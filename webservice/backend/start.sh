#!/usr/bin/env bash
set -euo pipefail

# --- CONFIG ---
root_path="$(git rev-parse --show-toplevel)"
SUI_LOG="/tmp/sui.log"
SUI_PORT=9123                    # adjust if your Sui HTTP/RPC port differs
# health paths to probe (try each until one returns 2xx). Change/add if Sui exposes a different health endpoint.
SUI_HEALTH_PATHS=("/" "/health" "/metrics")
SUI_START_TIMEOUT=60             # seconds to wait for node to become ready
SUI_START_DELAY=0.5              # small delay before first probe
NETWORK="${LFS_NETWORK:-localnet}"
ENV_FILE="${ENV_FILE:-}"
NETWORK_OVERRIDE=0
SUI_PID=""
SUI_PGID=""

# --- helpers ---
log() { printf '%s %s\n' "$(date --iso-8601=seconds 2>/dev/null || date)" "$*"; }
usage() {
  cat <<'EOF'
Usage: ./start.sh [--network localnet|testnet] [--env-file path]

--network/-n   Choose which Sui network the backend should target (default: localnet)
--env-file/-e  Optional .env file to source before starting (helpful for testnet)
--help/-h      Show this help
EOF
}

# choose probing method (curl preferred, fallback to nc)
if command -v curl >/dev/null 2>&1; then
  probe_url() {
    local url="$1"
    # curl --fail returns non-zero for non-2xx codes
    curl --silent --fail --max-time 3 "$url" >/dev/null 2>&1
  }
else
  # fallback: only check TCP port
  probe_url() {
    nc -z localhost "${SUI_PORT}" >/dev/null 2>&1
  }
fi

cleanup() {
  if [ -z "${SUI_PID}" ]; then
    return
  fi

  log "Cleaning up: killing sui (pid ${SUI_PID}, pgid ${SUI_PGID})..."
  if [ -n "${SUI_PGID:-}" ] && [ "${SUI_PGID}" != "" ]; then
    # negative pid kills whole process group
    kill -TERM -"${SUI_PGID}" 2>/dev/null || true
    sleep 1
    kill -KILL -"${SUI_PGID}" 2>/dev/null || true
  else
    kill "${SUI_PID}" 2>/dev/null || true
  fi
  # wait for pid to terminate (avoid zombies)
  wait "${SUI_PID}" 2>/dev/null || true
}

parse_args() {
  while [ "$#" -gt 0 ]; do
    case "$1" in
      -n|--network)
        NETWORK="${2:-}"
        if [ -z "${NETWORK}" ]; then
          echo "--network requires a value" >&2
          exit 1
        fi
        NETWORK_OVERRIDE=1
        shift 2
        ;;
      -e|--env-file)
        ENV_FILE="${2:-}"
        if [ -z "${ENV_FILE}" ]; then
          echo "--env-file requires a path" >&2
          exit 1
        fi
        shift 2
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "Unknown option: $1" >&2
        usage
        exit 1
        ;;
    esac
  done
}

load_env_file() {
  if [ -z "${ENV_FILE}" ]; then
    return
  fi

  if [ ! -f "${ENV_FILE}" ]; then
    echo "Env file not found: ${ENV_FILE}" >&2
    exit 1
  fi

  log "Loading env file ${ENV_FILE}"
  # Parse manually instead of sourcing so values like coin types with '<' '>' work.
  while IFS= read -r line || [ -n "$line" ]; do
    # skip comments/blank
    case "$line" in
      ''|'#'*) continue ;;
    esac
    # only handle KEY=VALUE
    case "$line" in
      *=*) ;;
      *) continue ;;
    esac
    key="${line%%=*}"
    val="${line#*=}"
    # trim whitespace around key and value
    key="${key#"${key%%[![:space:]]*}"}"
    key="${key%"${key##*[![:space:]]}"}"
    val="${val#"${val%%[![:space:]]*}"}"
    val="${val%"${val##*[![:space:]]}"}"
    # strip matching surrounding quotes
    if [ "${val#\"}" != "$val" ] && [ "${val%\"}" != "$val" ]; then
      val="${val#\"}"
      val="${val%\"}"
    elif [ "${val#\'}" != "$val" ] && [ "${val%\'}" != "$val" ]; then
      val="${val#\'}"
      val="${val%\'}"
    fi
    if [ -n "$key" ]; then
      printf -v "$key" '%s' "$val"
      export "$key"
    fi
  done < "${ENV_FILE}"
}

wait_for_sui() {
  log "Waiting ${SUI_START_TIMEOUT}s for sui to become ready on port ${SUI_PORT}..."
  sleep "${SUI_START_DELAY}"

  end=$((SECONDS + SUI_START_TIMEOUT))
  ready=0
  while [ "$SECONDS" -lt "$end" ]; do
    # if sui process died, bail and print logs
    if ! kill -0 "${SUI_PID}" 2>/dev/null; then
      log "sui process ${SUI_PID} died unexpectedly. Last 200 lines of log:"
      tail -n 200 "${SUI_LOG}" || true
      exit 1
    fi

    for p in "${SUI_HEALTH_PATHS[@]}"; do
      url="http://localhost:${SUI_PORT}${p}"
      if probe_url "${url}"; then
        log "sui responded successfully at ${url}"
        ready=1
        break 2
      fi
    done

    sleep 1
  done

  if [ "${ready}" -ne 1 ]; then
    log "Timed out waiting for sui readiness (${SUI_START_TIMEOUT}s). Last 200 lines of log:"
    tail -n 200 "${SUI_LOG}" || true
    exit 1
  fi
}

start_sui_localnet() {
  log "Starting sui (logs -> ${SUI_LOG})..."
  # use setsid when available so we can kill whole session/group later
  if command -v setsid >/dev/null 2>&1; then
    setsid sui start --force-regenesis --with-faucet >"${SUI_LOG}" 2>&1 &
  else
    sui start --force-regenesis --with-faucet >"${SUI_LOG}" 2>&1 &
  fi
  SUI_PID=$!

  # get process group id (used to kill entire group)
  SUI_PGID="$(ps -o pgid= "${SUI_PID}" | tr -d ' ')"
  trap cleanup EXIT INT TERM

  wait_for_sui
}

run_initializer() {
  log "Running initializer..."
  cd "$root_path/webservice/backend/cmd/initializer"
  if ! go run main.go; then
    rc=$?
    log "initializer failed with exit code ${rc}. Tail of Sui log for debugging:"
    tail -n 400 "${SUI_LOG}" || true
    exit "${rc}"
  fi
}

start_api() {
  log "Starting api..."
  cd "$root_path/webservice/backend/cmd/api"
  go run main.go
  rc=$?
  log "api exited with code ${rc}"
  exit "${rc}"
}

main() {
  parse_args "$@"
  load_env_file

  if [ "${NETWORK_OVERRIDE}" -eq 0 ]; then
    NETWORK="${LFS_NETWORK:-${NETWORK}}"
  fi

  NETWORK="$(printf '%s' "${NETWORK}" | tr '[:upper:]' '[:lower:]')"
  if [ "${NETWORK}" != "localnet" ] && [ "${NETWORK}" != "testnet" ]; then
    echo "Invalid network '${NETWORK}'. Expected localnet or testnet." >&2
    exit 1
  fi
  export LFS_NETWORK="${NETWORK}"

  if [ "${NETWORK}" = "testnet" ] && [ -z "${LFS_ENABLE_BRIDGE_MINT:-}" ]; then
    export LFS_ENABLE_BRIDGE_MINT=1
    log "Enabling bridge mint handler for testnet (set LFS_ENABLE_BRIDGE_MINT=0 to disable)"
  fi

  log "Selected network: ${NETWORK}"
  if [ "${NETWORK}" = "localnet" ]; then
    start_sui_localnet
    run_initializer
  else
    log "Testnet mode selected: skipping local Sui start and initializer (expects testnet values in init.json and env)."
  fi

  start_api
}

main "$@"
