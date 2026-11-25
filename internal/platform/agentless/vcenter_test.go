package agentless

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/types"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestVCenterReadsOneVMFromTLSModels(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("private credential resolution requires a supported Unix platform")
	}
	for name, makeModel := range map[string]func() *simulator.Model{"vCenter": simulator.VPX, "ESXi": simulator.ESX} {
		t.Run(name, func(t *testing.T) {
			job, profile := vcenterSimulatorFixture(t, makeModel(), nil)
			obs := RunJob(context.Background(), job)
			if obs.Status != "ok" || obs.ProbeReport == nil || obs.ProbeReport.Protocol != "vcenter" ||
				obs.MonitoringSamples == nil || len(obs.MonitoringSamples.Samples) != 5 {
				t.Fatalf("collector did not read the model: %s", obs.Code)
			}
			want := []float64{1, 0, 64, 31, 4096}
			wantUnits := []string{"none", "megahertz", "megabytes", "seconds", "bytes"}
			for i, sample := range obs.MonitoringSamples.Samples {
				if sample.Value == nil || *sample.Value != want[i] || sample.Unit != wantUnits[i] {
					t.Fatalf("sample %d: wrong reading or unit: %+v", i, sample)
				}
			}
			assertVCenterRedacted(t, obs, profile)
			verifyVCenterRestrictions(t, job, profile)
		})
	}
}

func TestVCenterClosesAuthenticatedSessionAfterCancellation(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("private credential resolution requires a supported Unix platform")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logout := make(chan int, 1)
	job, _ := vcenterSimulatorFixture(t, simulator.VPX(), func(sim http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch vcenterSOAPMethod(r) {
			case "RetrieveServiceContent", "Login":
				sim.ServeHTTP(w, r)
			case "RetrievePropertiesEx":
				cancel()
				<-r.Context().Done()
			case "Logout":
				response := httptest.NewRecorder()
				sim.ServeHTTP(response, r)
				logout <- response.Code
				w.WriteHeader(response.Code)
				_, _ = w.Write(response.Body.Bytes())
			default:
				w.WriteHeader(http.StatusBadRequest)
			}
		})
	})
	obs := RunJob(ctx, job)
	if obs.ProbeReport == nil || obs.ProbeReport.Outcome != "cancelled" || len(obs.Payload) != 0 {
		t.Fatalf("invalid result after cancellation: %s %+v", obs.Code, obs.ProbeReport)
	}
	if obs.MonitoringSamples != nil {
		for _, sample := range obs.MonitoringSamples.Samples {
			if sample.Value != nil {
				t.Fatalf("cancelled probe published a value: %+v", sample)
			}
		}
	}
	select {
	case code := <-logout:
		if code != http.StatusOK {
			t.Fatalf("simulator rejected logout: %d", code)
		}
	default:
		t.Fatal("authenticated session was left open")
	}
}

func TestVCenterReservesTimeToLogoutAfterQueryTimeout(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("private credential resolution requires a supported Unix platform")
	}
	logout := make(chan int, 1)
	job, _ := vcenterSimulatorFixture(t, simulator.VPX(), func(sim http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch vcenterSOAPMethod(r) {
			case "RetrievePropertiesEx":
				<-r.Context().Done()
			case "Logout":
				response := httptest.NewRecorder()
				sim.ServeHTTP(response, r)
				logout <- response.Code
				w.WriteHeader(response.Code)
				_, _ = w.Write(response.Body.Bytes())
			default:
				sim.ServeHTTP(w, r)
			}
		})
	})
	job.ProbePolicy.BudgetMS = 1800
	start := time.Now()
	observation := RunJob(context.Background(), job)
	if observation.Code != "vcenter_query_timeout" || observation.ProbeReport == nil ||
		observation.ProbeReport.Outcome != "timeout" || observation.ProbeReport.Reason != "budget" ||
		len(observation.Payload) != 0 || time.Since(start) > 2300*time.Millisecond {
		t.Fatalf("query timeout did not preserve the overall budget or receipt: %s", observation.Code)
	}
	select {
	case code := <-logout:
		if code != http.StatusOK {
			t.Fatalf("simulator rejected logout: %d", code)
		}
	default:
		t.Fatal("the timed-out query left its authenticated session open")
	}
}

func vcenterSOAPMethod(request *http.Request) string {
	body, err := io.ReadAll(request.Body)
	defer request.Body.Close()
	var envelope struct {
		Body struct {
			Method struct{ XMLName xml.Name } `xml:",any"`
		}
	}
	if err != nil || xml.Unmarshal(body, &envelope) != nil {
		return ""
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	return envelope.Body.Method.XMLName.Local
}

func TestVCenterClassifiesFailuresDuringSession(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("private credential resolution requires a supported Unix platform")
	}
	for _, test := range []struct {
		method, code, reason string
		fail                 http.HandlerFunc
	}{
		{"Login", "vcenter_connection_failed", "connection", vcenterHTTPFailure(http.StatusServiceUnavailable)},
		{"RetrievePropertiesEx", "vcenter_connection_failed", "connection", func(http.ResponseWriter, *http.Request) {
			panic(http.ErrAbortHandler)
		}},
		{"Login", "vcenter_access_denied", "configuration", vcenterHTTPFailure(http.StatusUnauthorized)},
		{"RetrievePropertiesEx", "vcenter_access_denied", "configuration", vcenterPermissionFault},
		{"Login", "invalid_vcenter_response", "response", vcenterHTTPFailure(http.StatusBadRequest)},
	} {
		t.Run(test.method+"/"+test.code, func(t *testing.T) {
			var inject atomic.Bool
			inject.Store(true)
			job, profile := vcenterSimulatorFixture(t, simulator.VPX(), func(sim http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if vcenterSOAPMethod(r) == test.method && inject.Load() {
						test.fail(w, r)
						return
					}
					sim.ServeHTTP(w, r)
				})
			})
			got := RunJob(context.Background(), job)
			if got.Status != "fail" || got.Code != test.code || got.ProbeReport == nil || got.ProbeReport.Reason != test.reason ||
				len(got.Payload) != 0 {
				t.Fatalf("unexpected failure: %s %s %+v", got.Status, got.Code, got.ProbeReport)
			}
			assertVCenterRedacted(t, got, profile)
			inject.Store(false)
			if resumed := RunJob(context.Background(), job); resumed.Status != "ok" {
				t.Fatalf("collector did not recover: %s", resumed.Code)
			}
		})
	}
}

func vcenterHTTPFailure(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fixture-password", status)
	}
}

func vcenterPermissionFault(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = io.WriteString(w, `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"
  xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:vim="urn:vim25">
  <soap:Body><soap:Fault><faultcode>ServerFaultCode</faultcode><faultstring>fixture-password</faultstring>
  <detail><NoPermissionFault xmlns="urn:vim25" xsi:type="NoPermission"><privilegeId>System.Read</privilegeId>
  </NoPermissionFault></detail></soap:Fault></soap:Body></soap:Envelope>`)
}

func vcenterSimulatorFixture(t *testing.T, model *simulator.Model, wrap func(http.Handler) http.Handler) (entities.AgentlessJob, vcenterProfile) {
	t.Helper()
	model.Machine = 1
	if err := model.Create(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(model.Remove)
	vm := model.Map().Any("VirtualMachine").(*simulator.VirtualMachine)
	vm.Runtime.PowerState = types.VirtualMachinePowerStatePoweredOn
	vm.Summary.QuickStats = types.VirtualMachineQuickStats{OverallCpuUsage: 0, GuestMemoryUsage: 64, UptimeSeconds: 31}
	vm.Summary.Storage = &types.VirtualMachineStorageSummary{Committed: 4096}
	model.Service.Listen = &url.URL{Host: "127.0.0.1:0", User: url.UserPassword("fixture-observer", "fixture-password")}
	model.Service.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	server := model.Service.NewServer()
	t.Cleanup(server.Close)
	endpoint := *server.URL
	endpoint.User = nil
	certificate := server.Certificate()
	if wrap != nil {
		wrapped := httptest.NewTLSServer(wrap(model.Service.ServeMux))
		t.Cleanup(wrapped.Close)
		endpoint.Host = strings.TrimPrefix(wrapped.URL, "https://")
		certificate = wrapped.Certificate()
	}
	const ref = "vcenter_profile:21"
	profile := vcenterProfile{
		Reference: ref, ClientID: 1, AssetID: 2, CheckID: 3, EndpointID: 4, URL: endpoint.String(),
		VMID: vm.Reference().Value, VMUUID: vm.Config.Uuid, Username: "fixture-observer", Password: "fixture-password",
		CAPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})),
	}
	writeVCenterProfiles(t, profile)
	job := entities.AgentlessJob{ClienteID: 1, AtivoID: 2, CheckID: 3, Tipo: "vcenter", BaseIntervalSec: 60,
		Endpoint: &entities.AgentlessEndpoint{ID: 4, Tipo: "url", Endereco: profile.URL}, Config: vcenterTestConfig(profile),
		ProbePolicy: &monitoring.ProbePolicy{Contract: monitoring.ProbePolicyContract, BudgetMS: 3000, FreshnessSec: 180, CredentialRef: ref}}
	return job, profile
}

func vcenterTestConfig(profile vcenterProfile) entities.AgentlessConfig {
	return entities.AgentlessConfig{"credential_ref": profile.Reference, "vm_id": profile.VMID, "vm_uuid": profile.VMUUID,
		"metrics": []any{
			map[string]any{"id": "powered", "preset": "powered_on"},
			map[string]any{"id": "cpu", "preset": "cpu_usage_mhz"},
			map[string]any{"id": "memory", "preset": "active_memory_mb"},
			map[string]any{"id": "uptime", "preset": "uptime_seconds"},
			map[string]any{"id": "storage", "preset": "committed_storage_bytes"},
		}}
}

func writeVCenterProfiles(t *testing.T, profile vcenterProfile) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"profiles": []vcenterProfile{profile}})
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "profiles.json")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AICEBERG_VCENTER_PROFILES_FILE", file)
}

func assertVCenterRedacted(t *testing.T, obs entities.AgentlessObservation, profile vcenterProfile) {
	t.Helper()
	wire, err := json.Marshal(obs)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{profile.Username, profile.Password, profile.CAPEM, profile.VMUUID, profile.URL} {
		if bytes.Contains(wire, []byte(value)) {
			t.Fatal("private fixture details in observation")
		}
	}
}

func verifyVCenterRestrictions(t *testing.T, job entities.AgentlessJob, profile vcenterProfile) {
	t.Helper()
	foreign := job
	foreign.ClienteID++
	if obs := RunJob(context.Background(), foreign); obs.Code != "vcenter_credential_unavailable" {
		t.Fatalf("cross-tenant reference accepted: %s", obs.Code)
	}
	wrongVM := profile
	wrongVM.VMUUID = "9df1b81b-4e00-4d5b-9088-d348e4aa39c0"
	job.Config = vcenterTestConfig(wrongVM)
	writeVCenterProfiles(t, wrongVM)
	if obs := RunJob(context.Background(), job); obs.Code != "vcenter_scope_mismatch" || len(obs.Payload) != 0 {
		t.Fatalf("unexpected VM attributes accepted: %s", obs.Code)
	}
	rejected := profile
	rejected.Password = "fixture-incorrect"
	job.Config = vcenterTestConfig(rejected)
	writeVCenterProfiles(t, rejected)
	if obs := RunJob(context.Background(), job); obs.Code != "vcenter_access_denied" {
		t.Fatalf("incorrect credentials accepted: %s", obs.Code)
	}
	untrusted := profile
	untrusted.CAPEM = ""
	job.Config = vcenterTestConfig(untrusted)
	writeVCenterProfiles(t, untrusted)
	if obs := RunJob(context.Background(), job); obs.Code != "vcenter_connection_failed" {
		t.Fatalf("untrusted certificate used: %s", obs.Code)
	}
}

func TestVCenterResponseLimitAndCancellation(t *testing.T) {
	r := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/large":
			_, _ = w.Write([]byte(strings.Repeat("x", maxVCenterReply+1)))
		default:
			<-r.Context().Done()
		}
	}))
	defer r.Close()
	for _, path := range []string{"/large", "/slow"} {
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		address := r.URL + path
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, nil)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		transport := vcenterTransport{delegate: http.DefaultTransport, target: address}
		response, err := transport.RoundTrip(req)
		cancel()
		if err == nil || response != nil {
			t.Fatal("out-of-budget reply accepted", path)
		}
	}
}

func TestVCenterPoweredOffDoesNotPublishStaleLiveMetrics(t *testing.T) {
	config := vcenterTestConfig(vcenterProfile{Reference: "vcenter_profile:7", VMID: "vm-42",
		VMUUID: "235d152e-d85a-43dc-b39e-dbaf8774d116"})
	selection, err := monitoring.VCenterSelectors(config)
	if err != nil {
		t.Fatal(err)
	}
	result := vcenterValues(selection, map[string]any{
		vmPowerState: types.VirtualMachinePowerStatePoweredOff, vcenterProperty("cpu_usage_mhz"): int32(250),
		vcenterProperty("active_memory_mb"): int32(64), vcenterProperty("committed_storage_bytes"): int64(4096),
	})
	power, ok := result[selection.Reference(selection.Metrics[0])].(float64)
	if !ok || power != 0 || len(result) != 2 || result[selection.Reference(selection.Metrics[4])] != float64(4096) {
		t.Fatal("powered off VM fabricated metrics or lost storage and power state")
	}
}
