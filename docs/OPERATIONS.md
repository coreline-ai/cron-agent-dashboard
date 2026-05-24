# Operations — Daily Use Checklist

This checklist is for running `cron-agent-dashboard` as a local daily tool and for verifying v0.1.0 release artifacts.

## Default locations

| Item | Default |
|---|---|
| UI | `http://127.0.0.1:8080` |
| Data dir | `~/.cron-agent-dashboard` |
| SQLite DB | `~/.cron-agent-dashboard/data.db` |
| Run logs | `~/.cron-agent-dashboard/runs/<run-id>.log` |
| Workspace cwd fallback | `~/.cron-agent-dashboard/workdirs/<workspace-slug>/` |

## Daily start

```bash
# If this is a fresh machine or a fresh data directory:
cron-agent-dashboard init

# Start foreground server. Keep the terminal open for logs.
cron-agent-dashboard serve --workers 3 --timezone Asia/Seoul
```

Daily checks after startup:

- Open `http://127.0.0.1:8080` and confirm the Settings page shows `연결됨`.
- Confirm at least one runtime (`codex`, `claude`, or `gemini`) is available on `PATH`.
- Check the server log for `startup self-check ok`.
- If the log shows `orphan_process_groups_terminated > 0`, the previous server process likely exited before child agent processes were fully cleaned up.
- If the log shows `orphan_process_groups_skipped > 0`, the DB had stale or missing process metadata, so the server avoided killing a potentially reused OS process group and only performed DB orphan recovery.
- If binding outside localhost, start with `--token` and store the same token in Settings → API token.

## Configuration strictness

Startup configuration is intentionally strict. Invalid numeric, boolean, or duration environment variables fail startup instead of silently falling back to defaults.

Examples that fail fast:

- `CRON_AGENT_DASHBOARD_WORKERS=abc`
- `CRON_AGENT_DASHBOARD_AUTO_BACKUP=maybe`
- `CRON_AGENT_DASHBOARD_MAINTENANCE_INTERVAL=daily`

Environment variables are parsed before CLI flags. If an invalid environment variable is present, startup fails before a CLI flag for the same setting can override it. Clear or correct the environment variable first.

## Security operating policy

- **Strict env**: keep startup fail-fast behavior for numeric, boolean, and duration env values. Do not rely on CLI flags to override invalid env; clear the env first.
- **Token storage**: token-mode UI stores the Bearer token in the current browser, not on the server. The default is `localStorage`; choose "session only" in the UI to store it in `sessionStorage` for the current browser session. Use only trusted local browser profiles and clear the token on shared machines.
- **Agent OS permissions**: Codex/Claude/Gemini child processes run with the same OS user permissions as `cron-agent-dashboard` and the configured workspace cwd. Treat external issue/comment input, Autopilot, and auto-chain as code-execution triggers.
- **Data/log permissions**: DBs, run logs, backups, and exported snapshots can contain prompts, stdout, paths, and operational metadata. Keep them in user-owned local directories; target `0700` for directories and `0600` for files.
- **Backup API paths**: HTTP `/api/system/backup` keeps the legacy default `.bak` path when `to` is empty. If `to` is set, it must resolve inside `{data_dir}/backups` unless the server is explicitly started with `--allow-arbitrary-backup-paths` or `CRON_AGENT_DASHBOARD_ALLOW_ARBITRARY_BACKUP_PATHS=true`. Local shell `cron-agent-dashboard backup --to ...` remains a power-user command and can target arbitrary user-writable paths.
- **CORS**: an empty CORS allowlist means same-origin only. Add explicit origins only for trusted dev/proxy UIs; avoid wildcard origins, especially with token mode.
- **CSP**: backend-served production UI responses include an enforced `Content-Security-Policy` plus the same Report-Only header. The policy is same-origin only for scripts/connects, permits inline styles for the SPA UI, blocks object embedding, and denies framing.

## Daily stop

Preferred stop path:

1. Stop creating new issues or Autopilot manual triggers.
2. Let active runs finish, or cancel them from the issue detail operations rail.
3. Press `Ctrl-C` in the server terminal.
4. Wait for graceful shutdown. The server stops HTTP, Autopilot, and the worker pool with a 30-second shutdown window.

If the server is supervised by another process, send `SIGTERM` through that supervisor and wait for the same graceful shutdown window.

## Backup checklist

Back up the DB at least before upgrades and after important work sessions.

CLI backups are local shell operations and may write to any user-writable destination. For UI/API-triggered backups, prefer paths under `$DATA_DIR/backups`; outside paths require the explicit arbitrary-path opt-in described in the security policy.

```bash
DATA_DIR="${CRON_AGENT_DASHBOARD_DATA_DIR:-$HOME/.cron-agent-dashboard}"
BACKUP_DIR="$HOME/backups/cron-agent-dashboard"
mkdir -p "$BACKUP_DIR"

cron-agent-dashboard backup \
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
2. Run `cron-agent-dashboard restore --data-dir "$DATA_DIR" --from <backup-db-path>`.
3. Start `cron-agent-dashboard serve` again and verify `/healthz`.

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
- 운영 환경별로 `--autopilot-failure-disable-threshold` 또는 `CRON_AGENT_DASHBOARD_AUTOPILOT_FAILURE_DISABLE_THRESHOLD`로 기준을 조정할 수 있다.
- `snooze_until`이 미래면 scheduled/manual trigger 모두 no-op이며 failure count를 증가시키지 않는다.
- Manual `지금 실행` uses the same trigger path, so a failure there should be treated like a scheduled failure unless the rule is snoozed.
- In token mode, add `-H "Authorization: Bearer $CRON_AGENT_DASHBOARD_TOKEN"` to curl requests.

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
ls -lh dist/cron-agent-dashboard-*
```

CI additionally runs `govulncheck ./...` on the pinned Go toolchain. The browser smoke test verifies the enforced CSP response header and fails if the browser emits a CSP violation console message during the core workspace → issue → comment flow.

Checksum verification for downloaded GitHub Release assets:

```bash
shasum -a 256 -c SHA256SUMS
```

Smoke-test one artifact for your platform:

```bash
ARTIFACT=dist/cron-agent-dashboard-darwin-arm64  # adjust for your OS/arch
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
- `dist/SHA256SUMS` must include every uploaded `cron-agent-dashboard-*` binary.
- `dist/cron-agent-dashboard.rb` is rendered from `docs/homebrew/cron-agent-dashboard.rb.tmpl` and uploaded as a release artifact.
- Optional Homebrew tap PR publish is enabled by setting `HOMEBREW_TAP_REPO` (for example `coreline-ai/homebrew-tap`) and `HOMEBREW_TAP_TOKEN` secrets. Without those secrets the publish step is a no-op.
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
- `--worktree-gc-after=24h`: `data_dir/worktrees/<workspace>/<run>/` 중 지정 기간 이상 수정되지 않은 terminal/orphan per-run worktree를 정리한다. DB상 `queued`/`running` run row는 디렉터리 mtime이 오래되어도 보호하며, `0`이면 GC를 끄고 사용량 측정만 유지한다.
- `/settings`에서 현재 자동 유지보수 설정, 마지막 로그 정리 결과, worktree disk usage/GC 결과, workspace별 기본 run timeout을 확인할 수 있다.

자동 백업을 끄려면 `CRON_AGENT_DASHBOARD_AUTO_BACKUP=false` 또는 `--auto-backup=false`로 실행한다.
worktree GC 기준은 `CRON_AGENT_DASHBOARD_WORKTREE_GC_AFTER=48h` 또는 `--worktree-gc-after=48h`처럼 조정한다.

### Worktree disk/GC verification runbook

이 절차는 release 후보나 운영 설정 변경 후 per-run worktree GC가 디스크 압박을 줄이면서 active run을 삭제하지 않는지 확인할 때 사용한다. 가능하면 실제 운영 `data_dir`가 아닌 임시/staging data dir에서 먼저 수행한다.

#### 1) Settings 가시성 확인

1. 서버를 시작한 뒤 `/settings`를 연다.
2. `서버 설정`의 `worktree 디스크` 요약이 `디렉터리 수`, `총 사용량`, `직전 GC 정리 수`를 보여주는지 확인한다.
3. `Per-run worktree 디스크 / GC` 카드에서 다음 값이 운영자가 해석 가능해야 한다.
   - 현재 사용량: `<data_dir>/worktrees/<workspace>/<run>/` 아래 run-scoped 디렉터리 수와 파일 바이트 합계
   - 마지막 측정: Settings API의 `worktree_measured_at`
   - 직전 GC 결과: 직전 maintenance pass에서 삭제한 worktree 디렉터리 수
   - GC 정책: `--worktree-gc-after`가 켜졌는지, 꺼졌으면 사용량 측정만 하는지
   - 주기: `--maintenance-interval` 기준으로 다음 자동 갱신을 기다릴 대략적 간격
4. API로 같은 값을 대조한다.

```bash
curl -fsS http://127.0.0.1:8080/api/settings \
  | jq '.maintenance | {interval_seconds, worktree_gc_after_seconds, worktree_bytes, worktree_dir_count, worktree_pruned_last_pass, worktree_measured_at}'
```

#### 2) GC 스트레스 검증

임시 data dir에 오래된 orphan worktree를 대량 생성하고, 짧은 maintenance interval과 GC horizon으로 실행해 pruning 수치가 Settings/API에 반영되는지 확인한다.

```bash
DATA_DIR="$(mktemp -d)"
cron-agent-dashboard init --data-dir "$DATA_DIR"

# 오래된 orphan worktree 100개를 생성한다.
mkdir -p "$DATA_DIR/worktrees/stress"
for i in $(seq 1 100); do
  d="$DATA_DIR/worktrees/stress/orphan-$i"
  mkdir -p "$d"
  dd if=/dev/zero of="$d/blob.bin" bs=1024 count=64 status=none
  touch -t 202001010000 "$d"
done

cron-agent-dashboard serve \
  --data-dir "$DATA_DIR" \
  --bind 127.0.0.1:18080 \
  --maintenance-interval 5s \
  --worktree-gc-after 1s
```

다른 터미널에서 확인한다.

```bash
# 첫 maintenance pass 이후 pruned 수가 증가하고 dir count가 0으로 내려가는지 확인한다.
while true; do
  curl -fsS http://127.0.0.1:18080/api/settings | jq .maintenance
  sleep 2
done

# 파일시스템도 대조한다.
find "$DATA_DIR/worktrees" -mindepth 2 -maxdepth 2 -type d | wc -l
```

기대 결과:

- `worktree_pruned_last_pass`가 생성한 orphan 수에 가깝게 증가한다.
- `worktree_dir_count`와 `worktree_bytes`는 다음 measurement에서 낮아진다.
- `/settings`의 worktree 카드가 API 값과 같은 방향으로 갱신된다.

#### 3) queued/running 보호 확인

정확한 보호 로직은 DB의 `run.status`를 기준으로 `queued`/`running` run ID와 같은 이름의 worktree를 pruning에서 제외하는 것이다. release 검증에서는 먼저 타깃 테스트를 실행한다.

```bash
go test ./internal/app -run 'TestRunMaintenanceOncePrunesTerminalAndOrphanWorktreesButProtectsActiveRuns|TestPruneStaleWorktreesRemovesOldDirsOnly|TestRunMaintenanceOnceRecordsWorktreeFields'
```

운영/staging에서 수동으로 확인할 때는 다음 순서로 active run ID와 worktree 디렉터리를 대조한다.

```bash
DATA_DIR="${CRON_AGENT_DASHBOARD_DATA_DIR:-$HOME/.cron-agent-dashboard}"
DB="$DATA_DIR/data.db"

# active run ID 목록. 결과가 있으면 같은 이름의 worktree는 GC 후에도 남아야 한다.
sqlite3 "$DB" "SELECT id,status FROM run WHERE status IN ('queued','running') ORDER BY enqueued_at;"

# GC 기준보다 오래된 active worktree가 있는지 확인한다.
# 출력된 run id를 <run-id>에 넣어 stat으로 mtime/존재 여부를 확인한다.
find "$DATA_DIR/worktrees" -mindepth 2 -maxdepth 2 -type d -name '<run-id>' -print -exec stat {} \;
```

검증 기준:

- `queued`/`running` run ID와 일치하는 worktree 디렉터리는 mtime이 오래되어도 남아 있어야 한다.
- terminal 상태(`done`, `failed`, `cancelled`)이거나 DB에 없는 orphan run ID만 GC 대상이다.
- git 기반 workspace에서 강제 삭제 후 parent repository의 worktree registry가 남으면 해당 repo에서 `git worktree prune`을 별도로 실행한다.

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

각 run은 stdout 로그를 workspace 내부 `.cron-runs/<run-id>.log`에 symlink한다. symlink가 불가능한 파일시스템에서는 같은 경로에 실제 로그 파일 경로를 담은 pointer file을 쓴다. Prompt에도 이 상대 경로가 안내된다.

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
cron-agent-dashboard export --to ./data-export.db
cron-agent-dashboard import --from ./data-export.db
```

`import`는 기존 `restore`와 동일하게 현재 DB를 pre-restore 백업으로 보존한 뒤 복구한다.
