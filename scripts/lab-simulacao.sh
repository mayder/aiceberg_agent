#!/usr/bin/env bash
set -euo pipefail

LAB_ROOT_DEFAULT="/Users/brenomayder/projects/simulacao/aiceberg_lab"
LAB_ROOT="${AICEBERG_LAB_ROOT:-$LAB_ROOT_DEFAULT}"
LAB_SCRIPT="$LAB_ROOT/scripts/lab.sh"
AGENTCTL_SCRIPT="$LAB_ROOT/scripts/agentctl.sh"
COPY_FROM_DIST_SCRIPT="$LAB_ROOT/scripts/copy_from_agent_dist.sh"

usage() {
  cat <<'EOF'
Uso:
  scripts/lab-simulacao.sh up
  scripts/lab-simulacao.sh down
  scripts/lab-simulacao.sh rollback
  scripts/lab-simulacao.sh ps
  scripts/lab-simulacao.sh guardrails
  scripts/lab-simulacao.sh validate
  scripts/lab-simulacao.sh acceptance
  scripts/lab-simulacao.sh logs [service]
  scripts/lab-simulacao.sh shell <service>
  scripts/lab-simulacao.sh restart [service]

  scripts/lab-simulacao.sh install <service> <artifact.tar.gz>
  scripts/lab-simulacao.sh configure <service> <mode> <token> [api_base_url] [hub_url] [hub_token]
  scripts/lab-simulacao.sh start <service>
  scripts/lab-simulacao.sh stop <service>
  scripts/lab-simulacao.sh status <service>
  scripts/lab-simulacao.sh agent-logs <service>

  scripts/lab-simulacao.sh copy-dist
  scripts/lab-simulacao.sh doctor

Exemplos:
  scripts/lab-simulacao.sh up
  scripts/lab-simulacao.sh configure node-hub hub TOKEN_HUB https://api.aiceberg.com.br
  scripts/lab-simulacao.sh configure node-relay relay TOKEN_RELAY https://api.aiceberg.com.br http://node-hub:9090 TOKEN_HUB
  scripts/lab-simulacao.sh start node-hub
  scripts/lab-simulacao.sh status node-hub
EOF
}

require_tools() {
  command -v docker >/dev/null 2>&1 || {
    echo "docker nao encontrado no PATH." >&2
    exit 1
  }
}

require_lab() {
  [[ -d "$LAB_ROOT" ]] || {
    echo "Lab nao encontrado em: $LAB_ROOT" >&2
    echo "Defina AICEBERG_LAB_ROOT para outro caminho, se necessario." >&2
    exit 1
  }
  [[ -x "$LAB_SCRIPT" ]] || {
    echo "Script do lab nao encontrado/executavel: $LAB_SCRIPT" >&2
    exit 1
  }
  [[ -x "$AGENTCTL_SCRIPT" ]] || {
    echo "Script do agentctl nao encontrado/executavel: $AGENTCTL_SCRIPT" >&2
    exit 1
  }
}

run_lab() {
  (
    cd "$LAB_ROOT"
    "$LAB_SCRIPT" "$@"
  )
}

run_agentctl() {
  (
    cd "$LAB_ROOT"
    "$AGENTCTL_SCRIPT" "$@"
  )
}

doctor() {
  echo "LAB_ROOT=$LAB_ROOT"
  echo "LAB_SCRIPT=$LAB_SCRIPT"
  echo "AGENTCTL_SCRIPT=$AGENTCTL_SCRIPT"
  docker info >/dev/null 2>&1 && echo "Docker: OK" || echo "Docker: indisponivel"
  run_lab ps || true
}

main() {
  local cmd="${1:-}"
  [[ -n "$cmd" ]] || {
    usage
    exit 1
  }

  require_tools
  require_lab

  case "$cmd" in
    up|down|rollback|ps|guardrails|validate|acceptance)
      run_lab "$cmd"
      ;;
    logs|restart)
      run_lab "$cmd" "${2:-}"
      ;;
    shell)
      local svc="${2:-}"
      [[ -n "$svc" ]] || {
        echo "Informe o service para shell." >&2
        usage
        exit 1
      }
      run_lab shell "$svc"
      ;;
    install)
      local svc="${2:-}"
      local artifact="${3:-}"
      [[ -n "$svc" && -n "$artifact" ]] || {
        echo "Uso: scripts/lab-simulacao.sh install <service> <artifact.tar.gz>" >&2
        exit 1
      }
      run_agentctl "$svc" install "$artifact"
      ;;
    configure)
      local svc="${2:-}"
      local mode="${3:-}"
      local token="${4:-}"
      local api_base_url="${5:-https://api.aiceberg.com.br}"
      local hub_url="${6:-}"
      local hub_token="${7:-}"
      [[ -n "$svc" && -n "$mode" && -n "$token" ]] || {
        echo "Uso: scripts/lab-simulacao.sh configure <service> <mode> <token> [api_base_url] [hub_url] [hub_token]" >&2
        exit 1
      }
      run_agentctl "$svc" configure "$mode" "$token" "$api_base_url" "$hub_url" "$hub_token"
      ;;
    start|stop|status)
      local svc="${2:-}"
      [[ -n "$svc" ]] || {
        echo "Uso: scripts/lab-simulacao.sh $cmd <service>" >&2
        exit 1
      }
      run_agentctl "$svc" "$cmd"
      ;;
    agent-logs)
      local svc="${2:-}"
      [[ -n "$svc" ]] || {
        echo "Uso: scripts/lab-simulacao.sh agent-logs <service>" >&2
        exit 1
      }
      run_agentctl "$svc" logs
      ;;
    copy-dist)
      [[ -x "$COPY_FROM_DIST_SCRIPT" ]] || {
        echo "Script nao encontrado/executavel: $COPY_FROM_DIST_SCRIPT" >&2
        exit 1
      }
      (
        cd "$LAB_ROOT"
        "$COPY_FROM_DIST_SCRIPT"
      )
      ;;
    doctor)
      doctor
      ;;
    *)
      usage
      exit 1
      ;;
  esac
}

main "$@"
