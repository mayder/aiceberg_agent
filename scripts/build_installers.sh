#!/usr/bin/env bash
set -euo pipefail

# Gera pacotes zip/tar.gz com binários e arquivos de serviço para cada OS/arch.
# Saída: dist/aiceberg-agent-<os>-<arch>.{tar.gz,zip}

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST="$ROOT/dist"
BIN_NAME="aiceberg_agent"
LD_FLAGS="-s -w"

rm -rf "$DIST"
mkdir -p "$DIST"

write_unix_readme() {
  cat >"$1" <<'EOF'
AIceberg Agent - pacote standalone

Conteúdo:
- aiceberg_agent (binário)
- agent.env.example (variáveis de ambiente)
- service (systemd ou launchd) para instalar como serviço

Passos resumidos:
1) Crie diretórios: sudo mkdir -p /usr/local/bin /var/lib/aiceberg && sudo mkdir -p /var/lib/aiceberg/data
2) Copie o binário para /usr/local/bin ou /Library/AIceberg/agent (macOS).
3) Copie agent.env.example para /etc/aiceberg/agent.env e defina AGENT_TOKEN (e outras variáveis, se precisar). Garanta permissão 600.
4) Instale o serviço:
   - Linux (systemd): sudo cp service/aiceberg-agent.service /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl enable --now aiceberg-agent
   - macOS (launchd): sudo cp service/com.aiceberg.agent.plist /Library/LaunchDaemons/ && sudo launchctl load /Library/LaunchDaemons/com.aiceberg.agent.plist
5) Verifique: curl http://localhost:8081/health (se HEALTH_PORT habilitado).

API de produção já é padrão: https://api.aiceberg.com.br/v1
EOF
}

write_win_readme() {
  cat >"$1" <<'EOF'
AIceberg Agent - pacote standalone (Windows)

Conteúdo:
- agent.exe
- install-service.ps1 (cria o serviço)
- LEIA-ME: defina AGENT_TOKEN (variável de ambiente do sistema) e paths opcionais; ou crie C:\ProgramData\AIceberg\agent.token e sete AGENT_TOKEN_PATH.

Passos resumidos (PowerShell como Admin):
1) Copie agent.exe para "C:\Program Files\AIceberg\agent".
2) Crie dados: mkdir C:\ProgramData\AIceberg
3) Defina o token (recomendado com path explícito):
   - setx /M AGENT_TOKEN_PATH C:\ProgramData\AIceberg\agent.token
   - echo -n SEU_TOKEN_AQUI > C:\ProgramData\AIceberg\agent.token
   (ou setx /M AGENT_TOKEN SEU_TOKEN_AQUI)
4) Execute: .\install-service.ps1 -BinPath "C:\Program Files\AIceberg\agent\agent.exe"
5) Verifique: sc.exe query AIcebergAgent ou Event Viewer (Application).

API de produção já é padrão: https://api.aiceberg.com.br/v1
EOF
}

build_unix() {
  local os="$1" arch="$2" ext="$3" svc="$4"
  local outdir="$DIST/aiceberg-agent-${os}-${arch}"
  mkdir -p "$outdir/service"
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build -ldflags "$LD_FLAGS" -o "$outdir/$BIN_NAME" "$ROOT/cmd/agent"
  cp "$ROOT/configs/agent.env.example" "$outdir/agent.env.example"
  if [[ -n "$svc" ]]; then
    cp "$ROOT/$svc" "$outdir/service/"
  fi
  write_unix_readme "$outdir/README_INSTALL.txt"
  (cd "$DIST" && tar -czf "aiceberg-agent-${os}-${arch}.${ext}" "aiceberg-agent-${os}-${arch}")
}

build_windows() {
  local arch="$1"
  local outdir="$DIST/aiceberg-agent-windows-${arch}"
  mkdir -p "$outdir"
  GOOS=windows GOARCH="$arch" CGO_ENABLED=0 go build -ldflags "$LD_FLAGS" -o "$outdir/agent.exe" "$ROOT/cmd/agent"
  cp "$ROOT/scripts/windows/install-service.ps1" "$outdir/install-service.ps1"
  write_win_readme "$outdir/README_INSTALL.txt"
  (cd "$DIST" && zip -qr "aiceberg-agent-windows-${arch}.zip" "aiceberg-agent-windows-${arch}")
}

echo "Building installers into $DIST"

build_unix "linux" "amd64" "tar.gz" "scripts/linux/aiceberg-agent.service"
build_unix "linux" "arm64" "tar.gz" "scripts/linux/aiceberg-agent.service"
build_unix "darwin" "amd64" "tar.gz" ""
build_unix "darwin" "arm64" "tar.gz" ""
build_windows "amd64"

echo "Done. Files in dist/:"
ls -1 "$DIST"
