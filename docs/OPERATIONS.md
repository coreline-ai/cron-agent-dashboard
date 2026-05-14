# Operations — Daily Use Checklist

This checklist is for running `corn-agent-dashboard` as a local daily tool and for verifying v0.1.0 release artifacts.

## Default locations

| Item | Default |
|---|---|
| UI | `http://127.0.0.1:8080` |
| Data dir | `~/.corn-agent-dashboard` |
| SQLite DB | `~/.corn-agent-dashboard/data.db` |
| Run logs | `~/.corn-agent-dashboard/runs/<run-id>.log` |
| Workspace cwd fallback | `~/.corn-agent-dashboard/workdirs/<workspace-slug>/` |

## Daily start

```bash
# If this is a fresh machine or a fresh data directory:
corn-agent-dashboard init

# Start foreground server. Keep the terminal open for logs.
corn-agent-dashboard serve --workers 3 --timezone Asia/Seoul
```

Daily checks after startup:

- Open `http://127.0.0.1:8080` and confirm the Settings page shows `연결됨`.
- Confirm at least one runtime (`codex`, `claude`, or `gemini`) is available on `PATH`.
- Check the server log for `startup self-check ok`.
- If the log shows `orphan_process_groups_terminated > 0`, the previous server process likely exited before child agent processes were fully cleaned up.
- If the log shows `orphan_process_groups_skipped > 0`, the DB had stale or missing process metadata, so the server avoided killing a potentially reused OS process group and only performed DB orphan recovery.
- If binding outside localhost, start with `--token` and store the same token in Settings → API token.

## Daily stop

Preferred stop path:

1. Stop creating new issues or Autopilot manual triggers.
2. Let active runs finish, or cancel them from the issue detail operations rail.
3. Press `Ctrl-C` in the server terminal.
4. Wait for graceful shutdown. The server stops HTTP, Autopilot, and the worker pool with a 30-second shutdown window.

If the server is supervised by another process, send `SIGTERM` through that supervisor and wait for the same graceful shutdown window.

## Backup checklist

Back up the DB at least before upgrades and after important work sessions.

```bash
DATA_DIR="${CORN_AGENT_DASHBOARD_DATA_DIR:-$HOME/.corn-agent-dashboard}"
BACKUP_DIR="$HOME/backups/corn-agent-dashboard"
mkdir -p "$BACKUP_DIR"

corn-agent-dashboard backup \
  --data-dir "$DATA_DIR" \
  --to "$BACKUP_DIR/data.db.$(date +%Y%m%d-%H%M%S)"
```

For a full local-state snapshot, also archive the data directory so run logs and workdirs are preserved:

```bash
tar -C "$(dirname "$DATA_DIR")" \
  -czf "$BACKUP_DIR/data-dir.$(date +%Y%m%d-%H%M%S).tgz" \
  "$(basename "$DATA_DIR")"
```

Restore sequence:

1. Stop the server.
2. Run `corn-agent-dashboard restore --data-dir "$DATA_DIR" --from <backup-db-path>`.
3. Start `corn-agent-dashboard serve` again and verify `/healthz`.

## Autopilot failure check

UI path:

1. Open `/w/<workspace-slug>/autopilot`.
2. Review each rule's enabled state, next run time, failure count, last error, and last triggered issue.
3. If a rule is disabled, fix the workspace/agent/template/runtime issue, then re-enable the rule.

API check:

```bash
curl -fsS http://127.0.0.1:8080/api/workspaces/<workspace-slug>/autopilot \
  | jq '.rules[] | {name, enabled, consecutive_failures, last_error, last_triggered_issue_id, next_run_at}'
```

Notes:

- A rule auto-disables after five consecutive trigger failures.
- Manual `지금 실행` uses the same trigger path, so a failure there should be treated like a scheduled failure.
- In token mode, add `-H "Authorization: Bearer $CORN_AGENT_DASHBOARD_TOKEN"` to curl requests.

## Run event and failure investigation

UI path:

1. Open the issue detail page.
2. In `Run 이력`, inspect status, trigger type, heartbeat, exit code, terminal reason, failure kind, and cancel reason.
3. Open `이벤트 타임라인` for the run.
4. Use `전체 로그 보기` only when the stdout body was truncated or when CLI output details are needed.

API path:

```bash
# Get runs for an issue UUID.
curl -fsS http://127.0.0.1:8080/api/issues/<issue-id>/runs | jq '.runs[] | {id, status, trigger_type, terminal_reason, failure_kind, cancel_reason}'

# Get the audit timeline for one run.
curl -fsS http://127.0.0.1:8080/api/runs/<run-id>/events | jq '.events[] | {seq, event_type, severity, message, details}'
```

Common interpretations:

| Signal | Meaning | First action |
|---|---|---|
| `failure_kind=exit_nonzero` | CLI process exited unsuccessfully | Open run log and inspect CLI output |
| `failure_kind=executor_error` | Runtime command could not be started or managed | Confirm CLI is installed and on `PATH` |
| `terminal_reason=stale_recovered` | Running heartbeat went stale and was recovered | Check host sleep/restart and rerun if needed |
| `terminal_reason=orphan_recovered` | Startup found a previously running row without a live worker | Review server restart timing, then rerun if needed |
| `cancel_reason=user` | User requested cancellation | Rerun if the issue still needs work |

## Cleanup logs

Run logs are separate files under `runs/`. Cleaning them does not delete issues, comments, or run DB rows, but old `로그 보기` links may no longer resolve after cleanup.

UI path: Settings → 운영 작업 → Run 로그 정리.

API path:

```bash
curl -fsS -X POST http://127.0.0.1:8080/api/system/cleanup-logs \
  -H 'Content-Type: application/json' \
  -d '{"days":30}'
```

Optional DB compaction after large deletes:

```bash
curl -fsS -X POST http://127.0.0.1:8080/api/system/vacuum \
  -H 'Content-Type: application/json' \
  -d '{}'
```

## Release artifact verification

Local build verification:

```bash
make check
pnpm --filter web test
make e2e-smoke
make release-build VERSION=v0.1.0
ls -lh dist/corn-agent-dashboard-*
```

Checksum verification for downloaded GitHub Release assets:

```bash
shasum -a 256 -c SHA256SUMS
```

Smoke-test one artifact for your platform:

```bash
ARTIFACT=dist/corn-agent-dashboard-darwin-arm64  # adjust for your OS/arch
TMP_DATA_DIR="$(mktemp -d)"

"$ARTIFACT" init --data-dir "$TMP_DATA_DIR"
"$ARTIFACT" serve --data-dir "$TMP_DATA_DIR" --bind 127.0.0.1:18080 &
PID=$!
sleep 2
curl -fsS http://127.0.0.1:18080/healthz
kill -TERM "$PID"
wait "$PID"
rm -rf "$TMP_DATA_DIR"
```

Release workflow expectation:

- Tag push or manual workflow dispatch builds `darwin/arm64`, `darwin/amd64`, `linux/amd64`, and `linux/arm64` artifacts.
- `dist/SHA256SUMS` must include every uploaded `corn-agent-dashboard-*` binary.
- The release body can use [RELEASE_NOTES_v0.1.0](RELEASE_NOTES_v0.1.0.md), and changes are summarized in [CHANGELOG](../CHANGELOG.md).
