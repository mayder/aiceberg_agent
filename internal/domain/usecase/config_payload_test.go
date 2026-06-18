package usecase

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/data/local/prefs"
)

func TestApplyConfigPayload_QueuesSelfUpdateOnSameVersion(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	cmds := make(chan ControlCommand, 1)

	base := config.CollectPrefs{Version: "cfg-v1", CPU: true}
	if err := store.Update(base); err != nil {
		t.Fatalf("seed prefs: %v", err)
	}

	payload := ConfigPayload{
		Version: "cfg-v1",
		Collect: config.CollectPrefs{CPU: true},
		Update: &UpdatePayload{
			Version: "7.0.6",
			URL:     "https://example.org/agent.bin",
		},
	}

	version, applied, err := ApplyConfigPayload(log, store, cmds, payload)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if version != "cfg-v1" {
		t.Fatalf("expected version cfg-v1, got %q", version)
	}
	if !applied {
		t.Fatalf("expected applied=true")
	}

	select {
	case cmd := <-cmds:
		if cmd.Name != "self_update" {
			t.Fatalf("expected self_update, got %q", cmd.Name)
		}
		if cmd.Update == nil || cmd.Update.Version != "7.0.6" {
			t.Fatalf("expected update payload version 7.0.6")
		}
	default:
		t.Fatalf("expected self_update command in channel")
	}
}

func TestApplyConfigPayload_QueuesSelfUpdatePolicy(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	cmds := make(chan ControlCommand, 1)

	base := config.CollectPrefs{Version: "cfg-v1", CPU: true}
	if err := store.Update(base); err != nil {
		t.Fatalf("seed prefs: %v", err)
	}

	enabled := true
	command := "echo update"
	payload := ConfigPayload{
		Version: "cfg-v1",
		Collect: config.CollectPrefs{CPU: true},
		AutoUpdate: &AutoUpdatePayload{
			Enabled: &enabled,
			Command: &command,
		},
	}

	_, applied, err := ApplyConfigPayload(log, store, cmds, payload)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !applied {
		t.Fatalf("expected applied=true")
	}

	select {
	case cmd := <-cmds:
		if cmd.Name != "self_update_policy" {
			t.Fatalf("expected self_update_policy, got %q", cmd.Name)
		}
		if cmd.AutoUpdate == nil || cmd.AutoUpdate.Enabled == nil || !*cmd.AutoUpdate.Enabled {
			t.Fatalf("expected auto update enabled override")
		}
	default:
		t.Fatalf("expected self_update_policy command in channel")
	}
}

func TestApplyConfigPayload_QueuesSelfUpdatePolicyWhenPayloadHasNullFields(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	cmds := make(chan ControlCommand, 1)

	base := config.CollectPrefs{Version: "cfg-v1", CPU: true}
	if err := store.Update(base); err != nil {
		t.Fatalf("seed prefs: %v", err)
	}

	payload := ConfigPayload{
		Version:    "cfg-v1",
		Collect:    config.CollectPrefs{CPU: true},
		AutoUpdate: &AutoUpdatePayload{},
	}

	_, applied, err := ApplyConfigPayload(log, store, cmds, payload)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !applied {
		t.Fatalf("expected applied=true")
	}

	select {
	case cmd := <-cmds:
		if cmd.Name != "self_update_policy" {
			t.Fatalf("expected self_update_policy, got %q", cmd.Name)
		}
		if cmd.AutoUpdate == nil {
			t.Fatalf("expected auto update payload")
		}
	default:
		t.Fatalf("expected self_update_policy command in channel")
	}
}

func TestApplyConfigPayload_QueuesPolicyBeforeSelfUpdate(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	cmds := make(chan ControlCommand, 2)

	base := config.CollectPrefs{Version: "cfg-v1", CPU: true}
	if err := store.Update(base); err != nil {
		t.Fatalf("seed prefs: %v", err)
	}

	enabled := true
	payload := ConfigPayload{
		Version: "cfg-v1",
		Collect: config.CollectPrefs{CPU: true},
		Update: &UpdatePayload{
			Version: "7.0.99",
			URL:     "https://example.org/agent.bin",
		},
		AutoUpdate: &AutoUpdatePayload{
			Enabled: &enabled,
		},
	}

	if _, _, err := ApplyConfigPayload(log, store, cmds, payload); err != nil {
		t.Fatalf("apply payload: %v", err)
	}

	first := <-cmds
	second := <-cmds
	if first.Name != "self_update_policy" {
		t.Fatalf("expected first command self_update_policy, got %q", first.Name)
	}
	if second.Name != "self_update" {
		t.Fatalf("expected second command self_update, got %q", second.Name)
	}
}

func TestApplyConfigPayloadAppliesLogLocalTransportAddresses(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}

	payload := ConfigPayload{
		Version: "cfg-logs-local",
		Collect: config.CollectPrefs{
			OSLogFiles: true,
		},
	}
	payload.Logs.UDPAddr = "127.0.0.1:1514"
	payload.Logs.TCPAddr = "127.0.0.1:1515"

	_, applied, err := ApplyConfigPayload(log, store, nil, payload)
	if err != nil {
		t.Fatalf("apply payload: %v", err)
	}
	if !applied {
		t.Fatalf("expected applied")
	}
	got := store.Get()
	if got.OSLogUDPAddr != "127.0.0.1:1514" || got.OSLogTCPAddr != "127.0.0.1:1515" {
		t.Fatalf("expected local log addresses persisted, got udp=%q tcp=%q", got.OSLogUDPAddr, got.OSLogTCPAddr)
	}
}

func TestApplyConfigPayloadAppliesJournaldFilters(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}

	enabled := true
	payload := ConfigPayload{
		Version: "cfg-logs-journald",
		Collect: config.CollectPrefs{
			OSLogFiles: true,
		},
	}
	payload.Logs.Journald = &enabled
	payload.Logs.JournalUnits = []string{"nginx.service", "sshd.service"}
	payload.Logs.JournalPrio = []string{"warning", "error"}

	_, applied, err := ApplyConfigPayload(log, store, nil, payload)
	if err != nil {
		t.Fatalf("apply payload: %v", err)
	}
	if !applied {
		t.Fatalf("expected applied")
	}
	got := store.Get()
	if !got.OSLogJournaldEnabled {
		t.Fatalf("expected journald enabled")
	}
	if strings.Join(got.OSLogJournaldUnits, ",") != "nginx.service,sshd.service" {
		t.Fatalf("expected journald units persisted, got %#v", got.OSLogJournaldUnits)
	}
	if strings.Join(got.OSLogJournaldPriorities, ",") != "warning,error" {
		t.Fatalf("expected journald priorities persisted, got %#v", got.OSLogJournaldPriorities)
	}
}

func TestApplyConfigPayloadAppliesCustomMetricsUDSPath(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}

	enabled := true
	payload := ConfigPayload{
		Version: "cfg-custom-metrics",
		Collect: config.CollectPrefs{
			CustomMetricsEnabled: true,
		},
	}
	payload.CustomMetrics.Enabled = &enabled
	payload.CustomMetrics.UDSPath = "/tmp/aiceberg-custommetrics.sock"

	_, applied, err := ApplyConfigPayload(log, store, nil, payload)
	if err != nil {
		t.Fatalf("apply payload: %v", err)
	}
	if !applied {
		t.Fatalf("expected applied")
	}
	got := store.Get()
	if !got.CustomMetricsEnabled || got.CustomMetricsUDSPath != "/tmp/aiceberg-custommetrics.sock" {
		t.Fatalf("expected custom metrics uds path persisted, got enabled=%v uds=%q", got.CustomMetricsEnabled, got.CustomMetricsUDSPath)
	}
}

func TestApplyConfigPayloadAppliesAPMSamplingPolicy(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}

	rate := 0.25
	preserveErrors := true
	payload := ConfigPayload{
		Version: "cfg-apm",
		Collect: config.CollectPrefs{
			OTLPEnabled: true,
		},
	}
	payload.APM.TraceSampleRate = &rate
	payload.APM.TraceSlowThresholdMs = 750
	payload.APM.TracePreserveErrors = &preserveErrors

	_, applied, err := ApplyConfigPayload(log, store, nil, payload)
	if err != nil {
		t.Fatalf("apply payload: %v", err)
	}
	if !applied {
		t.Fatalf("expected applied")
	}
	got := store.Get()
	if got.APMTraceSampleRate != 0.25 || got.APMTraceSlowThresholdMs != 750 || !got.APMTracePreserveErrors {
		t.Fatalf("expected apm sampling policy persisted, got rate=%v slow=%d preserve=%v", got.APMTraceSampleRate, got.APMTraceSlowThresholdMs, got.APMTracePreserveErrors)
	}
}

func TestApplyConfigPayloadAppliesContainerFilters(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}

	payload := ConfigPayload{
		Version: "cfg-containers",
		Collect: config.CollectPrefs{
			ContainerEnabled: true,
		},
	}
	payload.Containers.IncludeRegex = "prod|backend"
	payload.Containers.ExcludeRegex = "secret|root"

	_, applied, err := ApplyConfigPayload(log, store, nil, payload)
	if err != nil {
		t.Fatalf("apply payload: %v", err)
	}
	if !applied {
		t.Fatalf("expected applied")
	}
	got := store.Get()
	if got.ContainerIncludeRegex != "prod|backend" || got.ContainerExcludeRegex != "secret|root" {
		t.Fatalf("expected container filters persisted, got include=%q exclude=%q", got.ContainerIncludeRegex, got.ContainerExcludeRegex)
	}
}

func TestApplyConfigPayloadAppliesContainerRuntimeSettings(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}

	payload := ConfigPayload{
		Version: "cfg-containers-runtime",
		Collect: config.CollectPrefs{
			ContainerEnabled: true,
		},
	}
	payload.Containers.Runtime = "containerd"
	payload.Containers.ContainerdSocket = "/run/containerd/containerd.sock"
	payload.Containers.ContainerdNamespace = "k8s.io"
	payload.Containers.CtrPath = "/usr/bin/ctr"

	_, applied, err := ApplyConfigPayload(log, store, nil, payload)
	if err != nil {
		t.Fatalf("apply payload: %v", err)
	}
	if !applied {
		t.Fatalf("expected applied")
	}
	got := store.Get()
	if got.ContainerRuntime != "containerd" || got.ContainerContainerdSocket != "/run/containerd/containerd.sock" {
		t.Fatalf("expected container runtime persisted, got runtime=%q socket=%q", got.ContainerRuntime, got.ContainerContainerdSocket)
	}
	if got.ContainerContainerdNS != "k8s.io" || got.ContainerCtrPath != "/usr/bin/ctr" {
		t.Fatalf("expected containerd namespace and ctr path, got ns=%q ctr=%q", got.ContainerContainerdNS, got.ContainerCtrPath)
	}
}

func TestApplyConfigPayloadAppliesContainerLogSettings(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}

	logsEnabled := true
	payload := ConfigPayload{
		Version: "cfg-container-logs",
		Collect: config.CollectPrefs{
			ContainerEnabled: true,
		},
	}
	payload.Containers.LogsEnabled = &logsEnabled
	payload.Containers.LogsMaxLines = 50
	payload.Containers.LogsMaxBytes = 4096

	_, applied, err := ApplyConfigPayload(log, store, nil, payload)
	if err != nil {
		t.Fatalf("apply payload: %v", err)
	}
	if !applied {
		t.Fatalf("expected applied")
	}
	got := store.Get()
	if !got.ContainerLogsEnabled || got.ContainerLogsMaxLines != 50 || got.ContainerLogsMaxBytes != 4096 {
		t.Fatalf("expected container log settings persisted, got enabled=%v lines=%d bytes=%d", got.ContainerLogsEnabled, got.ContainerLogsMaxLines, got.ContainerLogsMaxBytes)
	}
}

func TestApplyConfigPayloadAppliesKubernetesLogSettings(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}

	logsEnabled := true
	payload := ConfigPayload{
		Version: "cfg-kubernetes-logs",
		Collect: config.CollectPrefs{
			KubernetesEnabled: true,
		},
	}
	payload.Kubernetes.LogsEnabled = &logsEnabled
	payload.Kubernetes.LogsCursorPath = "/var/lib/aiceberg/kubernetes_logs.cursor"
	payload.Kubernetes.LogsMaxLines = 75
	payload.Kubernetes.LogsMaxBytes = 8192
	payload.Kubernetes.LogsIncludeRegex = "prod|backend"
	payload.Kubernetes.LogsExcludeRegex = "secret|debug"

	_, applied, err := ApplyConfigPayload(log, store, nil, payload)
	if err != nil {
		t.Fatalf("apply payload: %v", err)
	}
	if !applied {
		t.Fatalf("expected applied")
	}
	got := store.Get()
	if !got.KubernetesLogsEnabled || got.KubernetesLogsCursorPath != "/var/lib/aiceberg/kubernetes_logs.cursor" {
		t.Fatalf("expected Kubernetes log enable/cursor persisted, got enabled=%v cursor=%q", got.KubernetesLogsEnabled, got.KubernetesLogsCursorPath)
	}
	if got.KubernetesLogsMaxLines != 75 || got.KubernetesLogsMaxBytes != 8192 {
		t.Fatalf("expected Kubernetes log limits persisted, got lines=%d bytes=%d", got.KubernetesLogsMaxLines, got.KubernetesLogsMaxBytes)
	}
	if got.KubernetesLogsIncludeRegex != "prod|backend" || got.KubernetesLogsExcludeRegex != "secret|debug" {
		t.Fatalf("expected Kubernetes log filters persisted, got include=%q exclude=%q", got.KubernetesLogsIncludeRegex, got.KubernetesLogsExcludeRegex)
	}
}

func TestApplyConfigPayloadAppliesLocalCheckManifestDirs(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}

	payload := ConfigPayload{
		Version: "cfg-local-check-manifests",
		Collect: config.CollectPrefs{
			LocalChecksEnabled: true,
		},
	}
	payload.LocalChecks.ManifestDirs = []string{"/etc/aiceberg/integrations.d", "/opt/aiceberg/localchecks"}

	_, applied, err := ApplyConfigPayload(log, store, nil, payload)
	if err != nil {
		t.Fatalf("apply payload: %v", err)
	}
	if !applied {
		t.Fatalf("expected applied")
	}
	got := store.Get()
	if len(got.LocalCheckManifestDirs) != 2 || got.LocalCheckManifestDirs[0] != "/etc/aiceberg/integrations.d" {
		t.Fatalf("expected manifest dirs persisted, got %#v", got.LocalCheckManifestDirs)
	}
}

func TestApplyConfigPayloadWithSecurityRequiresSignatureForSensitivePayload(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	cmds := make(chan ControlCommand, 1)

	payload := ConfigPayload{
		Version: "cfg-sec",
		Collect: config.CollectPrefs{CPU: true},
		Update:  &UpdatePayload{Version: "99.0.0", URL: "https://example.org/agent.bin"},
	}

	_, _, err := ApplyConfigPayloadWithSecurity(log, store, cmds, payload, ConfigSecurityOptions{
		SignatureSecret:   "secret",
		SignatureRequired: true,
	})
	if err == nil {
		t.Fatalf("expected missing signature error")
	}
}

func TestApplyConfigPayloadWithSecurityAcceptsSignedSensitivePayload(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	cmds := make(chan ControlCommand, 1)

	payload := ConfigPayload{
		Version: "cfg-sec",
		Collect: config.CollectPrefs{CPU: true},
		Update:  &UpdatePayload{Version: "99.0.0", URL: "https://example.org/agent.bin"},
	}
	payload.Signature = PayloadSignature{Algorithm: "hmac-sha256"}
	sig, err := SignConfigPayload(payload, "secret")
	if err != nil {
		t.Fatal(err)
	}
	payload.Signature.Value = sig

	_, applied, err := ApplyConfigPayloadWithSecurity(log, store, cmds, payload, ConfigSecurityOptions{
		SignatureSecret:   "secret",
		SignatureRequired: true,
	})
	if err != nil {
		t.Fatalf("expected signed payload accepted, got %v", err)
	}
	if !applied {
		t.Fatalf("expected payload applied")
	}
}

func TestApplyConfigPayloadBlocksUnsignedTokenRotation(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	cmds := make(chan ControlCommand, 1)

	payload := ConfigPayload{
		Version:       "cfg-token",
		Collect:       config.CollectPrefs{CPU: true},
		TokenRotation: &TokenRotationPayload{NewToken: "new-token"},
	}

	_, _, err := ApplyConfigPayloadWithSecurity(log, store, cmds, payload, ConfigSecurityOptions{
		SignatureSecret: "secret",
	})
	if err == nil {
		t.Fatalf("expected unsigned sensitive payload blocked")
	}
}

func TestApplyConfigPayloadQueuesTokenRotationWhenSigned(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	cmds := make(chan ControlCommand, 1)

	payload := ConfigPayload{
		Version:       "cfg-token",
		Collect:       config.CollectPrefs{CPU: true},
		TokenRotation: &TokenRotationPayload{NewToken: "new-token", Reason: "rotation"},
	}
	payload.Signature = PayloadSignature{Algorithm: "hmac-sha256"}
	sig, err := SignConfigPayload(payload, "secret")
	if err != nil {
		t.Fatal(err)
	}
	payload.Signature.Value = sig

	_, _, err = ApplyConfigPayloadWithSecurity(log, store, cmds, payload, ConfigSecurityOptions{SignatureSecret: "secret"})
	if err != nil {
		t.Fatalf("expected signed token rotation accepted, got %v", err)
	}
	cmd := <-cmds
	if cmd.Name != "rotate_agent_token" || cmd.TokenRotation == nil || cmd.TokenRotation.NewToken != "new-token" {
		t.Fatalf("unexpected token rotation command: %#v", cmd)
	}
}

func TestApplyConfigPayloadBlocksDowngradeWithoutForce(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	cmds := make(chan ControlCommand, 1)

	payload := ConfigPayload{
		Version: "cfg-downgrade",
		Collect: config.CollectPrefs{CPU: true},
		Update:  &UpdatePayload{Version: "0.0.0", URL: "https://example.org/agent.bin"},
	}

	_, _, err := ApplyConfigPayload(log, store, cmds, payload)
	if err == nil {
		t.Fatalf("expected downgrade blocked")
	}
}
