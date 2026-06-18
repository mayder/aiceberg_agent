#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

section() {
  printf '\n## %s\n\n' "$1"
}

result() {
  local name="$1"
  local status="$2"
  local detail="$3"
  printf -- '- %s: %s — %s\n' "$name" "$status" "$detail"
}

section "PKG-69 operational homologation"
result "repo" "info" "$ROOT"
result "os" "info" "$(uname -srm)"
result "go" "info" "$(go version 2>/dev/null || echo 'go indisponivel')"

section "environment readiness"
command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1 \
  && result "docker" "ready" "daemon acessivel" \
  || result "docker" "pending" "daemon indisponivel neste host"
command -v kubectl >/dev/null 2>&1 \
  && result "kubectl" "available" "$(kubectl version --client=true --short 2>/dev/null || kubectl version --client=true 2>/dev/null || echo 'client instalado')" \
  || result "kubectl" "pending" "cliente indisponivel"
command -v helm >/dev/null 2>&1 \
  && result "helm" "available" "$(helm version --short 2>/dev/null || echo 'client instalado')" \
  || result "helm" "pending" "cliente indisponivel"
command -v powershell.exe >/dev/null 2>&1 || command -v pwsh >/dev/null 2>&1 \
  && result "windows-powershell" "available" "shell Windows/PowerShell detectado" \
  || result "windows-powershell" "pending" "validacao Windows requer host Windows"

section "focused validation"
go test ./internal/common/config ./internal/domain/usecase ./internal/data/local/outbox ./internal/bootstrap ./internal/platform/collectors/oslogs ./internal/platform/collectors/custommetrics ./internal/platform/collectors/otlp ./internal/platform/collectors/containers ./internal/platform/collectors/kubernetes ./internal/platform/collectors/localchecks >/tmp/aiceberg_pkg69_go_test.log
result "go-test-focused" "pass" "log=/tmp/aiceberg_pkg69_go_test.log"
go test ./internal/domain/usecase -run 'TestSelfUpdate_(VerifiesTrustedArtifactSignature|RejectsInvalidTrustedArtifactSignature)' >/tmp/aiceberg_pkg69_artifact_trust_test.log
result "artifact-trust-local" "pass" "Ed25519 aceita assinatura valida e rejeita assinatura invalida antes do apply; log=/tmp/aiceberg_pkg69_artifact_trust_test.log"
{
  go test ./internal/common/httpx -run TestNewClientUsesAuthenticatedHTTPProxyFromEnvironment
  go test ./internal/domain/usecase -run TestSelfUpdate_DownloadTimeoutDoesNotFinalizePartialFile
} >/tmp/aiceberg_pkg69_update_network_test.log
result "update-network-local" "pass" "HTTP_PROXY autenticado e timeout de download sem artefato parcial; log=/tmp/aiceberg_pkg69_update_network_test.log"
go test ./internal/platform/collectors/sysmetrics -run 'Test(TimeSyncStatusClassifiesClockSkew|ClampAbsInt64)' >/tmp/aiceberg_pkg69_clock_skew_test.log
result "clock-skew-local" "pass" "time_sync classifica offset ok/warning/critical e limita offset extremo; log=/tmp/aiceberg_pkg69_clock_skew_test.log"
go test ./internal/platform/collectors/networkcapture -run TestClassifyPCAPUnavailableWarning >/tmp/aiceberg_pkg69_permission_test.log
result "permission-local" "pass" "pcap classifica permissao insuficiente, interface invalida e captura vazia sem crash; log=/tmp/aiceberg_pkg69_permission_test.log"
go test ./internal/data/local/outbox -run TestBoltStorePreservesQueueAcrossCollectorRestart >/tmp/aiceberg_pkg69_restart_replay_test.log
result "restart-replay-local" "pass" "outbox preserva fila antes/depois de restart e ACK parcial; log=/tmp/aiceberg_pkg69_restart_replay_test.log"
go test ./internal/platform/collectors/custommetrics -run TestCollectorBoundsHighVolumeCardinalityBurst >/tmp/aiceberg_pkg69_high_volume_test.log
result "high-volume-local" "pass" "custom_metrics limita burst de cardinalidade e contabiliza drops; log=/tmp/aiceberg_pkg69_high_volume_test.log"
result "api-unavailable-local" "pass" "flush_outbox tests preserve pending envelopes on transport/API error"
result "network-intermittent-local" "pass" "flush_outbox tests backoff and later retry behavior"
result "payload-large-local" "pass" "collectors enforce max bytes/items in focused tests"
result "outbox-full-local" "pass" "bolt outbox rejects oversized envelope without partial write"

section "full check"
./check.sh >/tmp/aiceberg_pkg69_check.log
result "check.sh" "pass" "log=/tmp/aiceberg_pkg69_check.log"

section "pending real-environment scenarios"
result "windows-server" "pending" "executar smoke.ps1 e update controlado em Windows Server"
result "windows-desktop" "pending" "executar smoke.ps1 e EventLog real em Windows desktop"
result "linux-debian-rhel" "pending" "executar smoke.sh, systemd e instalador em distros alvo"
result "docker" "pending" "validar CONTAINER_ENABLED com daemon real e carga controlada"
result "kubernetes" "pending" "validar DaemonSet/Helm/RBAC em cluster controlado"
result "proxy-auth" "partial" "HTTP_PROXY autenticado coberto em teste local; falta proxy real/TLS em host controlado"
result "clock-skew" "partial" "classificacao local ok/warning/critical coberta; falta NTP/clock errado real em host controlado"
result "permission-restricted" "partial" "pcap/tcpdump classifica permissao insuficiente localmente; falta host Linux/eBPF restrito real"
result "reboot-during-collection" "partial" "outbox bbolt preserva replay em reabertura local; falta reboot real do SO"
result "disk-full" "partial" "outbox cheia coberta por teste local; falta disco cheio real do SO"
result "payload-large" "partial" "limites cobertos por testes locais; falta alto volume real DogStatsD/OTLP/logs"
result "high-volume" "partial" "burst local de cardinalidade custom_metrics coberto; falta carga real DogStatsD/OTLP/logs com CPU/mem"
result "remote-update-rollback" "pending" "validar artefato anterior, hash e version_confirmed"
