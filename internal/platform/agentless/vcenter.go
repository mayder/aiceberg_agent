package agentless

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/debug"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

const maxVCenterReply = 512 << 10
const vcenterLogoutTimeout = 250 * time.Millisecond

var (
	errVCenterConfig       = errors.New("invalid_vcenter_config")
	errVCenterReply        = errors.New("invalid_vcenter_response")
	errVCenterScope        = errors.New("vcenter_scope_mismatch")
	errVCenterAccess       = errors.New("vcenter_access_denied")
	errVCenterConnection   = errors.New("vcenter_connection_failed")
	errVCenterQueryTimeout = errors.New("vcenter_query_timeout")
)

func runVCenter(ctx context.Context, job entities.AgentlessJob) Result {
	target, valid := vcenterEndpoint(job.Endpoint)
	selection, err := monitoring.VCenterSelectors(job.Config)
	if !valid || err != nil || job.ProbePolicy == nil {
		return Result{Status: "fail", Code: "invalid_vcenter_config"}
	}
	profile, ok := loadVCenterProfile(config.AgentlessVCenterProfilesFile(), job, selection, target)
	if !ok {
		return Result{Status: "fail", Code: "vcenter_credential_unavailable"}
	}
	client, closeClient, err := newVCenterClient(ctx, target, profile)
	if err != nil {
		return failedVCenterResult(err)
	}
	defer closeClient()
	manager := session.NewManager(client)
	if err = manager.Login(ctx, url.UserPassword(profile.Username, profile.Password)); err != nil {
		return failedVCenterResult(err)
	}
	defer logoutVCenter(ctx, manager)
	properties, err := readVCenterVMBeforeLogout(ctx, client, selection)
	if err != nil {
		return failedVCenterResult(err)
	}
	values := vcenterValues(selection, properties)
	return Result{Status: "ok", Payload: map[string]any{"vcenter_values_v1": values}}
}

func failedVCenterResult(err error) Result {
	for _, classified := range []error{errVCenterConfig, errVCenterScope, errVCenterAccess, errVCenterConnection, errVCenterQueryTimeout} {
		if errors.Is(err, classified) {
			return Result{Status: "fail", Code: classified.Error(), failure: err}
		}
	}
	var fault interface{ Fault() types.BaseMethodFault }
	if errors.As(err, &fault) {
		switch fault.Fault().(type) {
		case *types.InvalidLogin, *types.NoPermission, *types.NotAuthenticated:
			return Result{Status: "fail", Code: errVCenterAccess.Error(), failure: err}
		case *types.ManagedObjectNotFound:
			return Result{Status: "fail", Code: errVCenterScope.Error(), failure: err}
		}
	}
	return Result{Status: "fail", Code: errVCenterReply.Error(), failure: err}
}

func readVCenterVMBeforeLogout(ctx context.Context, client *vim25.Client, selection monitoring.VCenterSelection) (map[string]any, error) {
	deadline, bounded := ctx.Deadline()
	if !bounded {
		return readVCenterVM(ctx, client, selection)
	}
	// Reserve at most a quarter of the remaining budget for session cleanup.
	reserve := min(vcenterLogoutTimeout, time.Until(deadline)/4)
	query, cancel := context.WithDeadlineCause(ctx, deadline.Add(-max(0, reserve)), errVCenterQueryTimeout)
	defer cancel()
	properties, err := readVCenterVM(query, client, selection)
	if errors.Is(context.Cause(query), errVCenterQueryTimeout) {
		return nil, errVCenterQueryTimeout
	}
	return properties, err
}

func logoutVCenter(ctx context.Context, manager *session.Manager) {
	deadline := time.Now().Add(vcenterLogoutTimeout)
	if probeDeadline, ok := ctx.Deadline(); ok && probeDeadline.Before(deadline) {
		deadline = probeDeadline
	}
	// The logged-in session can still be closed after cancellation if time remains.
	cleanup, cancel := context.WithDeadline(context.WithoutCancel(ctx), deadline)
	defer cancel()
	_ = manager.Logout(cleanup)
}

func newVCenterClient(ctx context.Context, target *url.URL, profile vcenterProfile) (*vim25.Client, func(), error) {
	// govmomi can trace SOAP messages. Never start a credentialed client if enabled.
	if debug.Enabled() {
		return nil, nil, errVCenterConfig
	}
	trust := &tls.Config{MinVersion: tls.VersionTLS12}
	if profile.CAPEM != "" {
		trust.RootCAs = x509.NewCertPool()
		if !trust.RootCAs.AppendCertsFromPEM([]byte(profile.CAPEM)) {
			return nil, nil, errVCenterConfig
		}
	}
	transport := probeHTTPTransport.Clone()
	transport.TLSClientConfig = trust
	client := soap.NewClient(target, false)
	client.Transport = vcenterTransport{delegate: transport, target: target.String()}
	client.CheckRedirect = probeHTTPClient.CheckRedirect
	vc, err := vim25.NewClient(ctx, client)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, nil, err
	}
	return vc, transport.CloseIdleConnections, nil
}

type vcenterTransport struct {
	delegate http.RoundTripper
	target   string
}

func (t vcenterTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method != http.MethodPost || request.URL.String() != t.target {
		return nil, errVCenterReply
	}
	response, err := t.delegate.RoundTrip(request)
	if err != nil {
		return nil, errors.Join(errVCenterConnection, err)
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, errVCenterAccess
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return nil, errVCenterConnection
	}
	if response.ContentLength > maxVCenterReply {
		return nil, errVCenterReply
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxVCenterReply+1))
	if err != nil {
		return nil, errors.Join(errVCenterConnection, err)
	}
	if len(body) > maxVCenterReply {
		return nil, errVCenterReply
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	return response, nil
}

const vmBIOSUUID = "config.uuid"
const vmPowerState = "runtime.powerState"

func vcenterProperty(preset string) string {
	switch preset {
	case "powered_on":
		return vmPowerState
	case "cpu_usage_mhz":
		return "summary.quickStats.overallCpuUsage"
	case "active_memory_mb":
		return "summary.quickStats.guestMemoryUsage"
	case "uptime_seconds":
		return "summary.quickStats.uptimeSeconds"
	case "committed_storage_bytes":
		return "summary.storage.committed"
	default:
		return ""
	}
}

func readVCenterVM(ctx context.Context, client *vim25.Client, selection monitoring.VCenterSelection) (map[string]any, error) {
	ref := types.ManagedObjectReference{Type: "VirtualMachine", Value: selection.VMID}
	paths := []string{vmBIOSUUID, vmPowerState}
	selected := map[string]bool{vmBIOSUUID: true, vmPowerState: true}
	for _, metric := range selection.Metrics {
		path := vcenterProperty(metric.Preset)
		if !selected[path] {
			paths = append(paths, path)
			selected[path] = true
		}
	}
	result, err := methods.RetrievePropertiesEx(ctx, client, &types.RetrievePropertiesEx{
		This: client.ServiceContent.PropertyCollector, Options: types.RetrieveOptions{MaxObjects: 1},
		SpecSet: []types.PropertyFilterSpec{{
			PropSet:   []types.PropertySpec{{Type: ref.Type, PathSet: paths}},
			ObjectSet: []types.ObjectSpec{{Obj: ref}},
		}},
	})
	if err != nil {
		return nil, err
	}
	if result.Returnval == nil || result.Returnval.Token != "" || len(result.Returnval.Objects) != 1 {
		return nil, errVCenterReply
	}
	resolved := result.Returnval.Objects[0]
	if resolved.Obj.Type != ref.Type || resolved.Obj.Value != ref.Value {
		return nil, errVCenterScope
	}
	return checkedVCenterProperties(resolved, selected, selection.VMUUID)
}

func checkedVCenterProperties(object types.ObjectContent, selected map[string]bool, uuid string) (map[string]any, error) {
	for _, missing := range object.MissingSet {
		switch missing.Fault.Fault.(type) {
		case *types.NoPermission, *types.NotAuthenticated:
			return nil, errVCenterAccess
		}
	}
	result := make(map[string]any, len(selected))
	for _, prop := range object.PropSet {
		if _, repeated := result[prop.Name]; !selected[prop.Name] || repeated {
			return nil, errVCenterReply
		}
		result[prop.Name] = prop.Val
	}
	if value, ok := result[vmBIOSUUID].(string); !ok || value != uuid {
		return nil, errVCenterScope
	}
	return result, nil
}

func vcenterValues(selection monitoring.VCenterSelection, properties map[string]any) map[string]any {
	values := make(map[string]any, len(selection.Metrics))
	state, _ := properties[vmPowerState].(types.VirtualMachinePowerState)
	if text, ok := properties[vmPowerState].(string); ok {
		state = types.VirtualMachinePowerState(text)
	}
	for _, metric := range selection.Metrics {
		var value float64
		var valid bool
		switch metric.Preset {
		case "powered_on":
			switch state {
			case types.VirtualMachinePowerStatePoweredOn:
				value, valid = 1, true
			case types.VirtualMachinePowerStatePoweredOff, types.VirtualMachinePowerStateSuspended:
				valid = true
			}
		case "committed_storage_bytes":
			value, valid = vcenterNumber(properties[vcenterProperty(metric.Preset)])
		default:
			if state == types.VirtualMachinePowerStatePoweredOn {
				value, valid = vcenterNumber(properties[vcenterProperty(metric.Preset)])
			}
		}
		if valid {
			values[selection.Reference(metric)] = value
		}
	}
	return values
}

func vcenterNumber(raw any) (float64, bool) {
	var number int64
	switch value := raw.(type) {
	case int32:
		number = int64(value)
	case int64:
		number = value
	default:
		return 0, false
	}
	return float64(number), number >= 0 && number <= 9007199254740991
}
