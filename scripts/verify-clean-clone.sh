#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
PORT="${CLEAN_CLONE_PORT:-18084}"
SERVER_PID=""

cleanup() {
  if [[ -n "${SERVER_PID}" ]] && kill -0 "${SERVER_PID}" 2>/dev/null; then
    kill "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
  fi
  rm -rf "${TMP}"
}
trap cleanup EXIT

command -v rsync >/dev/null || { echo "rsync is required" >&2; exit 1; }
command -v curl >/dev/null || { echo "curl is required" >&2; exit 1; }
command -v go >/dev/null || { echo "go is required" >&2; exit 1; }
command -v pnpm >/dev/null || { echo "pnpm is required" >&2; exit 1; }

WORK="${TMP}/repo"
mkdir -p "${WORK}"
rsync -a --delete \
  --exclude '.git' \
  --exclude 'node_modules' \
  --exclude 'web/node_modules' \
  --exclude 'web/dist' \
  --exclude 'dist' \
  --exclude '.tmp' \
  --exclude 'test-results' \
  --exclude 'playwright-report' \
  --exclude '/cron-agent-dashboard' \
  "${ROOT}/" "${WORK}/"

cd "${WORK}"
pnpm install --frozen-lockfile --ignore-scripts
make check
make build

DATA_DIR="${TMP}/data"
./cron-agent-dashboard init --data-dir "${DATA_DIR}"
./cron-agent-dashboard serve --data-dir "${DATA_DIR}" --bind "127.0.0.1:${PORT}" >"${TMP}/server.log" 2>&1 &
SERVER_PID="$!"

for _ in $(seq 1 100); do
  if curl -fsS "http://127.0.0.1:${PORT}/healthz" >/dev/null; then
    break
  fi
  sleep 0.1
done

curl -fsS "http://127.0.0.1:${PORT}/healthz" | grep '"status":"ok"' >/dev/null
curl -fsS "http://127.0.0.1:${PORT}/" | grep 'Cron Agent Dashboard' >/dev/null
curl -fsS "http://127.0.0.1:${PORT}/w/foo/issues/NEWS-1" | grep 'Cron Agent Dashboard' >/dev/null
curl -fsS "http://127.0.0.1:${PORT}/api/settings" | grep '"version"' >/dev/null

echo "clean clone quick start verified"
