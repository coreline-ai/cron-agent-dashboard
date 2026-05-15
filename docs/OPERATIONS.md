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
  | jq '.rules[] | {name, enabled, snooze_until, consecutive_failures, last_error, last_triggered_issue_id, next_run_at}'
```

Notes:

- A rule auto-disables after five consecutive trigger failures by default.
- 운영 환경별로 `--autopilot-failure-disable-threshold` 또는 `CORN_AGENT_DASHBOARD_AUTOPILOT_FAILURE_DISABLE_THRESHOLD`로 기준을 조정할 수 있다.
- `snooze_until`이 미래면 scheduled/manual trigger 모두 no-op이며 failure count를 증가시키지 않는다.
- Manual `지금 실행` uses the same trigger path, so a failure there should be treated like a scheduled failure unless the rule is snoozed.
- In token mode, add `-H "Authorization: Bearer $CORN_AGENT_DASHBOARD_TOKEN"` to curl requests.

## Run event and failure investigation

UI path:

1. Open the issue detail page.
2. In `Run 이력`, inspect status, trigger type, heartbeat, exit code, terminal reason, failure kind, cancel reason, and `instructions vN`.
3. Open `이벤트 타임라인` for the run.
4. Use `전체 로그 보기` only when the stdout body was truncated or when CLI output details are needed.

API path:

```bash
# Get runs for an issue UUID.
curl -fsS http://127.0.0.1:8080/api/issues/<issue-id>/runs | jq '.runs[] | {id, status, trigger_type, terminal_reason, failure_kind, cancel_reason}'

# Get the audit timeline for one run.
curl -fsS http://127.0.0.1:8080/api/runs/<run-id>/events | jq '.events[] | {seq, event_type, severity, message, details}'

# Get agent instructions history when reproducing why an old run behaved a certain way.
curl -fsS http://127.0.0.1:8080/api/agents/<agent-id>/instructions \
  | jq '.versions[] | {version, created_at, instructions}'
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


## Usage and retry check

`GET /api/settings`의 `usage_7d`는 최근 7일 run count, measured run count, token 합계, cost micros를 반환한다.

```bash
curl -fsS http://127.0.0.1:8080/api/settings \
  | jq '.usage_7d'
```

Retry 대기 중인 run은 `status=queued`와 `next_retry_at`을 함께 가진다. timeout/executor_error만 자동 retry 대상이며, max attempts 이후에는 일반 failed run으로 마감된다.

## Automatic maintenance

서버는 기본값으로 자동 유지보수를 수행한다.

- `--auto-backup=true`: 실행 직후 및 `--maintenance-interval`마다 SQLite DB를 `data_dir/backups/data-<timestamp>.db`로 백업한다.
- `--auto-backup-keep=7`: 자동 백업 파일은 최신 N개만 보존한다.
- `--auto-cleanup-log-days=90`: 지정 일수보다 오래된 `runs/*.log` 파일을 삭제한다. DB의 issue/comment/run 기록은 유지된다.
- `/settings`에서 현재 자동 유지보수 설정과 workspace별 기본 run timeout을 확인할 수 있다.

자동 백업을 끄려면 `CORN_AGENT_DASHBOARD_AUTO_BACKUP=false` 또는 `--auto-backup=false`로 실행한다.

## Migration failure visibility

마이그레이션은 파일 단위 트랜잭션으로 적용된다. 실패하면 해당 migration은 rollback되고 `schema_migration_failures`에 버전, 이름, 오류 메시지, 실패 시각이 기록된다.

확인 방법:

```bash
curl -fsS http://127.0.0.1:8080/api/settings \
  | jq '.migration_failures'
```

실패 이력이 있으면 먼저 DB를 백업한 뒤 서버 로그와 migration 파일을 확인한다.

## Phase 2 collaboration operations

### Auto-chain

자동 체이닝은 workspace별 opt-in이다. `/settings`에서 “agent 결과 @mention 자동 체이닝 허용”을 켜면 완료된 agent 결과 댓글의 첫 `@AgentName`이 다음 run으로 등록된다.

운영 가드:

- 최대 depth 5
- 같은 chain 내 동일 agent 재호출 차단
- dispatch 결과 system comment + run_event 기록
- 문제가 생기면 workspace toggle을 OFF로 전환

### Run log context sharing

각 run은 stdout 로그를 workspace 내부 `.corn-runs/<run-id>.log`에 symlink한다. symlink가 불가능한 파일시스템에서는 같은 경로에 실제 로그 파일 경로를 담은 pointer file을 쓴다. Prompt에도 이 상대 경로가 안내된다.

### Retry policy

Agent 상세에서 max attempts, backoff seconds, retry 대상 failure kind를 조정할 수 있다. 기본은 timeout/executor_error만 재시도하며 backoff는 10초 → 60초 → 5분이다.

## OSS adoption operations

### React error boundary

웹앱 route tree는 `react-error-boundary`로 감싸져 있다. 특정 페이지 렌더 오류가 발생하면 전체 white screen 대신 복구 화면을 표시한다.

운영 확인:

- fallback에 “화면 렌더 실패”와 “다시 시도” 버튼이 표시되어야 한다.
- 이벤트 핸들러/비동기 mutation 오류는 기존 Toast/MutationErrorAlert 흐름으로 처리한다. ErrorBoundary는 render/lifecycle 오류 방어용이다.

### Process probe

Startup self-check는 running run에 기록된 sample PID를 `gopsutil/v4/process`로 best-effort 확인한다.

정책:

- probe가 “PID 없음”을 확인하면 해당 process group kill은 건너뛰고 orphan run recovery만 수행한다.
- probe가 실패하거나 권한/OS 차이로 확인할 수 없으면 기존 process group cleanup 경로로 fallback한다.
- PID/PGID는 운영 safety hint이며, run 상태 전이의 source of truth는 SQLite `run` row다.

### Token/cost display

Issue Detail 작업 콘솔은 현재 이슈의 run token/cost 합계를 표시한다. Settings의 7일 사용량과 함께 비용 이상 징후를 확인한다.

### Deferred OSS candidates

- `@xyflow/react`: lineage/sub-issue graph UI를 실제 구현할 때 lazy import로 도입한다.
- MCP: readonly tool + workspace root 제한 + run_event audit 설계가 끝난 뒤 workspace opt-in으로 도입한다.
- 외부 queue, OpenTelemetry, Sentry, testcontainers는 현재 단일 사용자/SQLite/단일 바이너리 철학에는 과하다.

## Phase 2+ graph, guard, export operations

### Lineage graph

Issue Detail의 “흐름 그래프”는 parent issue, sub-issue, run, chained run을 한 화면에 표시한다. 데이터는 `issue.parent_issue_id`, `run.parent_run_id`, `run.chain_id`, `run.chain_depth`에서 온다. 그래프가 깨져도 기존 Run 이력과 이벤트 타임라인이 source of truth다.

### Auto-chain cost guard

`/settings`에서 workspace별로 다음 값을 조정한다.

- 최대 chain depth
- 24시간 자동 chain run 제한
- 24시간 자동 chain 비용 제한
- dry-run 모드

제한 초과 시 run은 생성되지 않고 system comment에 차단 사유가 남는다. 비용 제한은 runtime adapter가 보고한 `total_cost_micros` 기준이므로 CLI가 metric을 출력하지 않으면 0으로 집계될 수 있다.

### Export / import aliases

`backup`/`restore`와 같은 DB snapshot 동작을 더 명확한 이름으로 실행할 수 있다.

```bash
corn-agent-dashboard export --to ./data-export.db
corn-agent-dashboard import --from ./data-export.db
```

`import`는 기존 `restore`와 동일하게 현재 DB를 pre-restore 백업으로 보존한 뒤 복구한다.
