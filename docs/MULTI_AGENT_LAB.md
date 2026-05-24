# MULTI_AGENT_LAB — 5+ 워크스페이스 멀티 에이전트 실개발 테스트

> 목적: Cron Agent Dashboard 자체를 테스트베드로 사용해 5개 이상 워크스페이스가 병렬 또는 리더 지시 방식으로 실제 개발 프로젝트를 수행할 수 있는지 검증한다.

## 빠른 시작

```bash
# repo root에서 실행하면 seed-lab이 가장 가까운 git root를 자동 working_dir로 사용한다.
make build
./cron-agent-dashboard seed-lab

# 명시적으로 대상 repo를 지정하려면:
./cron-agent-dashboard seed-lab --lab-working-dir /path/to/cron-agent-dashboard

# 병렬 실행 확인용 서버 시작
./cron-agent-dashboard serve --workers 3
```

`seed-lab`은 idempotent하다. 다시 실행하면 기존 lab 워크스페이스를 재사용하고, 누락된 worker agent 또는 `[LAB]` 초기 issue만 보강한다.

## 생성되는 워크스페이스

| 워크스페이스 | 역할 | 할당 프로젝트 | 메인 에이전트 | Worker |
|---|---|---|---|---|
| `pm-command-hub` | 리더/통합 | 전체 분배, triage, 통합 체크리스트 | `@ProgramLead` | `@Integrator`, `@ReleaseCaptain` |
| `dashboard-design-lab` | UI 실행 | CoreMCP 스타일 데모/대시보드 레이아웃 최신화 | `@DesignLead` | `@FrontendPolish`, `@VisualQA` |
| `auth-realtime-lab` | API/realtime 실행 | SSE token mode/realtime stream 회귀 검증 | `@RealtimeLead` | `@BackendAuth`, `@FrontendSSE`, `@QAEngineer` |
| `worktree-ops-lab` | 운영 실행 | per-run worktree disk report/GC 스트레스 검증 | `@OpsLead` | `@BackendGC`, `@OpsQA` |
| `history-import-lab` | 데이터 실행 | workspace history import materialization round-trip 강화 | `@DataLead` | `@ImportTester`, `@CLIQA` |
| `release-ci-lab` | 릴리스 실행 | Homebrew tap dry-run + e2e-full CI 리허설 | `@ReleaseLead` | `@CIEngineer`, `@DocsQA` |
| `agent-orchestration-lab` | 오케스트레이션 실행 | 5+ workspace seed/runbook 유지보수 | `@OrchestrationLead` | `@SeedBuilder`, `@RunbookWriter` |

모든 lab workspace는 다음 기본값으로 생성된다.

- `auto_chain_enabled=true`
- `auto_chain_max_depth=3`
- `auto_chain_daily_run_limit=12`
- `auto_close_on_run_done=false`
- `per_run_worktree=true`
- `default_timeout_seconds=900`

## 권장 실험 순서

### 1. Baseline 확인

```bash
git status --short
git rev-parse HEAD
go test ./...
pnpm --filter web test
```

### 2. Lab seed

```bash
./cron-agent-dashboard seed-lab --lab-working-dir "$(pwd)"
```

생성 후 UI에서 다음을 확인한다.

- `/` 홈에서 7개 workspace가 표시된다.
- 각 workspace에 main agent 1개와 worker agent 2~3개가 있다.
- 각 workspace에 `[LAB] ...` 초기 issue가 1개 있다.
- `/w/:slug/runs`에서 초기 queued run이 분리되어 보인다.

### 3. 병렬 모드

1. 서버를 `--workers 3` 이상으로 실행한다.
2. 6개 실행 workspace의 `[LAB]` issue를 동시에 진행시킨다.
3. `Run feed`, `Chain dashboard`, `Settings`의 worktree disk usage를 확인한다.
4. 실패 run은 terminal reason, failure kind, stdout log를 먼저 요약한 뒤 재실행한다.

### 4. 리더 지시 모드

1. `pm-command-hub`의 `@ProgramLead`가 workspace별 결과를 리뷰한다.
2. 수정 요청은 대상 workspace issue 댓글에 acceptance criteria로 남긴다.
3. 각 workspace lead가 한 댓글에 worker 한 명만 `@WorkerName`으로 멘션해 재작업을 위임한다.
4. `@Integrator`가 충돌 파일과 통합 순서를 정리한다.

### 5. 통합 검증

권장 통합 순서:

1. `dashboard-design-lab`
2. `auth-realtime-lab`
3. `worktree-ops-lab`
4. `history-import-lab`
5. `release-ci-lab`
6. `agent-orchestration-lab`

최소 검증:

```bash
git diff --check
go test ./...
go test -race ./...
go vet ./...
pnpm --filter web build
pnpm --filter web test
make e2e-smoke
```

가능하면 추가 검증:

```bash
make e2e-full
make screenshots
make release-smoke
```

## 안전 규칙

- `main` 직접 수정/force-push 금지.
- 각 workspace는 `lab/<workspace-slug>` branch를 기준으로 보고한다.
- 파일 소유 경계를 넘는 수정은 `pm-command-hub`의 `@Integrator` 승인 후 진행한다.
- queued/running worktree는 GC 대상이 아니어야 한다.
- 실제 Homebrew tap publish와 GitHub release 업로드는 이 lab 범위에서 dry-run까지만 수행한다.

## 종료 보고 템플릿

```md
## Workspace result — <slug>

- Goal:
- Branch:
- Changed files:
- Tests:
- Run/chain observations:
- Worktree disk/GC observations:
- Token/cost summary:
- Risks:
- Merge recommendation: merge / revise / discard
```
