# TRD — corn-agent-dashboard

> Technical Requirements Document
> Version: 0.1
> Date: 2026-05-11
> Status: Draft

---

## 1. Tech Stack (기술 스택)

### 1.1 Backend
- **언어**: Go 1.24+
- **HTTP 라우터**: `chi/v5` (Corn Design Reference와 동일, 검증됨)
- **DB 드라이버**: `modernc.org/sqlite` (pure Go, CGo 없음 → 크로스 컴파일 용이)
- **SQL 빌더**: `sqlx` (가벼움, sqlc는 단일 프로젝트엔 과함)
- **Cron**: `github.com/robfig/cron/v3`
- **UUID**: `github.com/google/uuid`
- **로깅**: 표준 `log/slog`
- **마이그레이션**: `embed.FS`로 SQL 파일 임베드 + 부팅 시 적용

### 1.2 Frontend
- **프레임워크**: **Vite + React + React Router (SPA)** — Phase 0에서 확정
  - 결정 근거:
    - Next의 SSR/RSC 기능 미사용 (단일 사용자, API 전부 client fetch)
    - `output: 'export'` + 동적 라우트 `[id]`는 `generateStaticParams` 필요 → 모든 issue id를 빌드 시 알 수 없으므로 우회 필요
    - SPA + Go fallback handler (모든 비-API 경로 → index.html)가 단순하고 안정적
  - Corn Design Reference 컴포넌트는 client-only로 추출 가능한 것만 가져온다 (RSC/`'use server'` 의존 컴포넌트는 client 변형 작성)
- **라우팅**: `react-router-dom` v6+
- **UI 라이브러리**: shadcn/ui (Base UI는 선택)
- **스타일**: Tailwind CSS v4 (PostCSS plugin)
- **상태 관리**: TanStack Query v5
- **폼**: `react-hook-form` + `zod`
- **마크다운 렌더링**: MVP UI는 `react-markdown` + `remark-gfm`을 사용한다. `rehype-raw` / raw HTML 렌더링은 금지한다.
- **i18n**: 한국어 only (기본 문자열 inline, i18next 불필요)
- **다크모드**: `class` 기반 직접 토글 (next-themes 대체)

### 1.3 Storage
- **DB**: SQLite 파일 1개 (`~/.corn-agent-dashboard/data.db`)
- **Run logs**: `~/.corn-agent-dashboard/runs/<run-id>.log` (stdout 전체)
- **설정**: `~/.corn-agent-dashboard/config.toml` (선택)

### 1.4 Agent Runtime
- **방식**: `exec.Command` spawn (Corn Design Reference와 동일)
- **지원 CLI**: `codex`, `claude`, `gemini` (Phase 1)
- **Runtime adapter 인터페이스** — CLI마다 인자/stdin 방식이 달라 추상화 필요
  ```go
  type RuntimeAdapter interface {
      Name() string                          // "codex" | "claude" | "gemini"
      Detect(ctx context.Context) RuntimeInfo // PATH/version 검출
      BuildCommand(ctx context.Context, run RunContext) (*exec.Cmd, []byte /* stdin */, error)
  }
  type RunContext struct {
      WorkspaceWorkingDir string
      AgentInstructions   string
      AgentModel          string
      IssueTitle          string
      IssueBody           string
      RecentComments      []CommentSnippet  // 최근 댓글 (truncation 적용)
  }
  ```
- **동시 실행**: worker pool size = 3 (설정 가능, CLI 플래그)
  - **워크스페이스 직렬화**: claim 쿼리가 같은 워크스페이스의 동시 running을 차단 → 사실상 워크스페이스당 동시 1개, 다른 워크스페이스끼리만 병렬
  - 근거: 같은 `workspace.working_dir`에서 두 에이전트 동시 실행 시 파일 충돌. MVP 안전 우선.
- **타임아웃**: 600초 (설정 가능). 타임아웃 시 → run.status='failed', error_message='timeout'
- **취소**: `cmd.Process` SIGTERM → 30초 후 SIGKILL. **프로세스 그룹 단위 kill** (`setpgid` + `kill(-pgid)`) — 자식의 자식까지 정리
- **Working directory**: `workspace.working_dir` (빈값이면 `~/.corn-agent-dashboard/workdirs/<workspace-slug>` 자동 생성)
- **환경변수 정책**:
  - 기본 상속: `PATH`, `HOME`, `USER`, `LANG`, `LC_*`, `TZ`, `TMPDIR`
  - 추가 상속 (CLI별 API 키 등): adapter가 화이트리스트로 명시 (예: codex adapter는 `OPENAI_API_KEY` 추가)
  - 기타 모든 env는 자식에 전달 안 함 (안전)
- **stdout/stderr 캡처**:
  - stdout → `~/.corn-agent-dashboard/runs/<run-id>.log` (append, **단일 run 최대 10MB**)
  - 10MB 도달 시 파일 append 중단 + 마지막에 `\n[truncated by corn-agent-dashboard at 10MB]\n` 추가
  - **단, stdout pipe는 io.Discard로 계속 drain** — child process가 pipe buffer 가득 차서 blocking되지 않도록
  - stderr → 메모리 ring buffer (마지막 4KB) → 종료 시 run.error_message에 기록
  - **결과 댓글 INSERT 시 추가 cap**: comment.content는 64KB 한도. 초과 시 앞 60KB + "전체 로그는 [로그 보기](/api/runs/<id>/log)" 링크 append
  - **comment 렌더링은 raw HTML 허용 안 함** (`react-markdown` + `remark-gfm`, `rehype-raw` 금지)
- **prompt 렌더링** (truncation, **요약 없음**):
  ```
  {agent.instructions}

  # 작업
  {issue.title}

  {issue.body}

  # 최근 컨텍스트
  {최신순 댓글 3개, 총 4000자 cap, 초과 시 ...[truncated]}
  ```

---

## 2. Architecture (아키텍처)

### 2.1 단일 프로세스 모델

```
┌──────────────────────────────────────────────────┐
│  corn-agent-dashboard (single Go binary)                  │
│                                                  │
│  ┌─────────────────────────────────────────┐    │
│  │  HTTP Server (:8080)                    │    │
│  │  - REST API (/api/*)                    │    │
│  │  - Static (embed.FS: Vite SPA + fallback)│    │
│  └─────────────────────────────────────────┘    │
│                                                  │
│  ┌─────────────────────────────────────────┐    │
│  │  Worker Pool (goroutines × N)           │    │
│  │  - issue queue 폴링                      │    │
│  │  - exec.Command(codex/claude/...)       │    │
│  │  - stdout → log file + comment INSERT   │    │
│  └─────────────────────────────────────────┘    │
│                                                  │
│  ┌─────────────────────────────────────────┐    │
│  │  Cron Scheduler (robfig/cron)           │    │
│  │  - autopilot_rule 스캔                   │    │
│  │  - 시각 도래 시 issue INSERT             │    │
│  └─────────────────────────────────────────┘    │
│                                                  │
│           ↓ ↑                                   │
│  ┌─────────────────────────────────────────┐    │
│  │  SQLite (data.db)                       │    │
│  └─────────────────────────────────────────┘    │
└──────────────────────────────────────────────────┘
```

### 2.2 모듈 구성 (Go 패키지)

```
corn-agent-dashboard/
├─ cmd/
│  └─ corn-agent-dashboard/
│     └─ main.go            # 진입점, flag 파싱
├─ internal/
│  ├─ config/               # 설정 로딩
│  ├─ db/
│  │  ├─ migrations/        # *.sql, embed
│  │  └─ db.go              # connect, migrate, orphan recovery
│  ├─ store/                # sqlx 기반 CRUD
│  │  ├─ workspace.go
│  │  ├─ agent.go
│  │  ├─ issue.go
│  │  ├─ comment.go
│  │  ├─ run.go             # claim/UPDATE 트랜잭션 패턴 포함
│  │  └─ autopilot.go
│  ├─ http/
│  │  ├─ router.go
│  │  ├─ handler_workspace.go
│  │  ├─ handler_agent.go
│  │  ├─ handler_issue.go
│  │  ├─ handler_comment.go
│  │  ├─ handler_autopilot.go
│  │  ├─ handler_system.go  # backup / vacuum / cleanup-logs
│  │  └─ middleware.go
│  ├─ worker/
│  │  ├─ pool.go            # goroutine pool, DB-claim polling
│  │  ├─ executor.go        # exec.Command spawn (adapter 사용)
│  │  ├─ runtime/           # runtime adapter 구현체
│  │  │  ├─ adapter.go      # RuntimeAdapter 인터페이스
│  │  │  ├─ codex.go
│  │  │  ├─ claude.go
│  │  │  └─ gemini.go
│  │  ├─ mention.go         # @AgentName 파싱 (첫 매칭, lower 비교)
│  │  └─ prompt.go          # truncation 기반 prompt 렌더링
│  └─ scheduler/
│     └─ cron.go            # robfig/cron, CORN_AGENT_DASHBOARD_TIMEZONE 적용
├─ web/                     # Vite + React + React Router
│  ├─ src/
│  │  ├─ pages/
│  │  ├─ components/
│  │  └─ lib/
│  ├─ index.html
│  └─ vite.config.ts
├─ docs/
├─ scripts/
└─ go.mod
```

### 2.3 라이프사이클

**부팅**:
1. SQLite 연결 + 마이그레이션 적용 (idempotent)
2. Startup self-check 실행
   - SQLite `integrity_check`, WAL, foreign key, busy timeout 검증
   - DB상 `running`이며 최근 `process_recorded_at`이 있는 process group을 best-effort SIGTERM/SIGKILL 정리
   - 오래되었거나 누락된 process metadata는 kill하지 않고 skip
   - 남은 `running` run은 orphan recovery로 `cancelled` 처리
3. Worker pool 시작 (N goroutine)
4. Cron scheduler 시작 (DB에서 활성 룰 로드)
5. HTTP 서버 시작

**Graceful shutdown** (SIGINT/SIGTERM):
1. HTTP 서버 멈춤 (새 요청 거부)
2. Worker pool: 신규 claim 정지
3. 진행 중 run에 SIGTERM (자식 프로세스 그룹) → 최대 30초 대기 → 미종료 시 SIGKILL
4. 강제 종료된 run은 `status='cancelled', exit_code=-1, error_message='shutdown'`로 마감
5. 시스템 댓글 "재시작 중 진행 작업이 취소되었습니다" INSERT
6. Cron 정지
7. DB close
8. 정상 shutdown이면 다음 부팅 시 추가 정리는 보통 없음. 단, 프로세스가 `kill -9` 등으로 강제 종료되어 트랜잭션이나 child process cleanup이 누락된 경우, 다음 부팅 self-check가 최근 `process_pgid`를 best-effort 종료한 뒤 orphan recovery를 수행한다.

---

## 3. Non-Functional Requirements

### 3.1 성능
| 항목 | 목표 |
|---|---|
| 콜드 부팅 → UI 가용 | < 3초 |
| API p95 응답 | < 100ms |
| 보드 페이지 로드 | < 500ms (이슈 100개 기준) |
| 백엔드 idle RAM | < 100MB |
| 백엔드 idle CPU | < 0.1% |

### 3.2 안정성
- DB 트랜잭션으로 일관성 보장
- worker가 panic해도 pool 자체는 살아남기 (recover)
- exec timeout 시 process group kill + run.status='failed', error_message='timeout'
- 사용자 cancel 시 run.status='cancelled', exit_code=-1, error_message='user cancelled'
- 부팅 시 최근 process metadata가 남은 `running` run의 process group을 best-effort 종료하고, `running` 상태로 박힌 run을 `cancelled`로 정리 (orphan recovery, error_message='orphan recovered')
- **Durable queue**: 큐는 `run.status='queued'` row로 표현. HTTP 핸들러는 enqueue 대신 row INSERT만 함. 프로세스 재시작 시 작업 손실 0.

### 3.3 보안
- 단일 사용자, localhost only 기본
- `--bind` 플래그로 외부 노출 시 단일 토큰 인증 강제
- SQL injection: sqlx named parameter만 사용
- 멘션 파싱: 정규식 + 화이트리스트 (workspace 내 agent 이름만)
- 에이전트 stdout은 escape 없이 markdown 렌더링 (단일 사용자 신뢰)

### 3.4 운영
- 로그: stderr로 slog JSON
- 백업: `data.db` + `runs/` 디렉토리만 복사하면 끝
- 업그레이드: 새 바이너리 교체 + 부팅 시 자동 마이그레이션
- 로그 디스크 관리: 30일 지난 run log 자동 삭제 (옵션)

---

## 4. Auth (인증)

### 4.1 기본 모드: 무인증
- `127.0.0.1:8080` 바인딩 → 로컬 머신 전용
- 토큰 없이 API 호출 가능

### 4.2 옵션 모드: 단일 토큰
- `--token <SECRET>` 또는 `CORN_AGENT_DASHBOARD_TOKEN=...` 환경변수
- 모든 `/api/*` 요청에 `Authorization: Bearer <SECRET>` 필수
- 외부 노출(`--bind 0.0.0.0`) 시 토큰 미설정 → 부팅 거부

### 4.3 CORS / Origin
- 같은 origin이면 통과
- 무인증 모드라도 다른 origin의 fetch는 기본 거부 (CSRF 방어):
  - `Origin` 헤더 검사 → `http://127.0.0.1:8080` 또는 `--cors <origin>` 화이트리스트만 허용
- 다른 origin은 `--cors <origin>` 또는 `CORN_AGENT_DASHBOARD_CORS=<origin1>,<origin2>` 환경변수로 명시

---

## 5. Constraints (제약)

### 5.1 운영 환경
- OS: macOS 13+, Linux (Ubuntu 22.04+)
- Windows: best effort (Phase 2 검증)
- CPU: amd64 / arm64
- 디스크: 1GB 이상 여유 (run log 누적)

### 5.2 외부 의존
- 필수: `codex` 또는 `claude` 또는 `gemini` CLI 1개 이상 PATH에 있음
- 부팅 시 PATH 스캔 → 가용 CLI list를 settings에 표시
- 가용 CLI 0개여도 부팅은 성공 (UI에서 안내)

### 5.3 코드 규모 상한 (반복 강조)
- Go: 1,500 LOC 이하
- Frontend (web/, Vite + React): 3,000 LOC 이하
- 초과 시 기능 줄이거나 추상화 재검토

---

## 6. Data Lifecycle

### 6.1 영속화 위치
```
~/.corn-agent-dashboard/
├─ data.db           # SQLite (모든 메타데이터)
├─ runs/
│  ├─ <run-id>.log   # stdout 전체
│  └─ ...
└─ config.toml       # (선택)
```

### 6.2 보존 정책 (default)
- 이슈/댓글/run row: 영구 (사용자가 명시적으로 삭제할 때까지)
- run stdout log: 30일 후 자동 삭제 (옵션, default OFF)
- DB vacuum: 사용자가 Settings/API에서 요청할 때 SQLite `VACUUM` 실행

### 6.3 마이그레이션 정책
- 모든 스키마 변경은 `internal/db/migrations/NNNN_*.sql`
- 부팅 시 적용 이력은 `schema_migrations` 테이블
- DOWN 마이그레이션 없음 (forward-only)

---

## 7. API Surface (요약)

상세는 [API.md](API.md) 참조.

| 영역 | 엔드포인트 수 |
|---|---|
| Workspaces | 5 (list/get/create/update/delete) |
| Agents | 6 (list/get/create/update/promote/delete) |
| Issues | 7 (list/get/create/update/rerun/cancel/delete) |
| Comments | 3 |
| Runs | 2 |
| Autopilot | 5 |
| Health / Settings | 2 |
| System actions | 3 (backup/vacuum/cleanup-logs) |
| **총** | **33** |

---

## 8. Worker / Executor 상세

### 8.1 Dispatch 트리거 (모두 = run row INSERT)
1. 이슈 생성 시 → run INSERT (assignee 또는 메인 에이전트)
2. **사용자 댓글**에 `@AgentName` 명시 멘션 → 같은 issue에 새 run INSERT (`issue.assignee_agent_id` 변경 없음, `run.agent_id`만 멘션된 에이전트)
3. Autopilot cron 시각 도래 → issue + run INSERT
4. 수동 `/rerun` 호출 → 같은 issue에 새 run INSERT (동일 agent)

**모든 dispatch는 channel send가 아니라 DB row INSERT.** Worker가 polling으로 claim.

현재 체이닝 정책은 **explicit-only**다. agent 결과 댓글 안의 `@AgentName`은 자동 dispatch하지 않으며, 이는 무한 루프, 비용 폭주, hallucinated mention 실행을 방지하기 위한 MVP 안전 정책이다. Auto-chain은 Phase 2+ opt-in 후보이며 현재 기능이 아니다.

### 8.2 실행 흐름 (durable)
```
1. (HTTP) BEGIN; issue INSERT/UPDATE; run INSERT(status='queued'); COMMIT;
2. (Worker poll, 1s) BEGIN IMMEDIATE;
     SELECT id FROM run WHERE status='queued' AND <per-issue + per-workspace 직렬화> ORDER BY enqueued_at LIMIT 1;
     UPDATE run SET status='running', claimed_at, claimed_by, started_at WHERE id=? AND status='queued';
     INSERT system 댓글 "<agent> 실행을 시작했습니다 (run #N)";
   COMMIT;
   ※ 실행 시작 시 issue.status는 변경하지 않는다. UI 실행 상태는 derived execution_status로 표시한다.
3. adapter.BuildCommand(runContext) → *exec.Cmd
   - codex: `codex exec [--model <model>] [--cd <workspace>] -` + stdin prompt
   - claude: `claude --print [--model <model>]` + stdin prompt
   - gemini: `gemini --prompt <prompt> [--model <model>]`
4. cmd.SysProcAttr.Setpgid = true (process group)
5. cmd.Start()
6. stdout/stderr 동시 캡처:
   - stdout → 파일 append (10MB cap)
   - stderr → 메모리 ring buffer (마지막 4KB)
7. cmd.Wait() 또는 ctx 만료/취소 → process group kill
8. BEGIN;
     UPDATE run SET status=?, finished_at, exit_code, stdout_path, error_message;
     INSERT comment (author_type='agent', run_id=?, content=stdout 64KB cap + log link, truncated=?);
     성공(status='done')인 경우에만 issue.status='done'으로 전이;
     실패/취소(status='failed'|'cancelled')인 경우 issue.status는 'open' 유지;
   COMMIT;
```

### 8.3 Prompt 렌더링 (truncation, 요약 없음)
```
{agent.instructions}

# 작업
{issue.title}

{issue.body}

# 최근 컨텍스트
{최신순 댓글 최대 3개, 총 4000자 cap. 초과 시 ...[truncated]}
```
- **추가 LLM 호출 없음**. 단순 자르기.
- 4000자는 worker.prompt 상수 (Phase 2에서 조정 가능).

### 8.4 동시성
- worker pool size: default 3 (설정 가능, CLI 플래그)
- 같은 이슈에 동시 running run 불가 → claim 쿼리 `NOT EXISTS (SELECT 1 FROM run WHERE issue_id=r.issue_id AND status='running')`로 보장
- 다른 workspace는 병렬 가능, 같은 workspace는 직렬
- claim 충돌 (두 worker가 같은 row UPDATE 시도): SQLite `BEGIN IMMEDIATE`로 직렬화, `WHERE id=? AND status='queued'` 가드로 affected_rows=1 확인

### 8.5 취소
- `POST /api/issues/:id/cancel` 핸들러:
  1. active run 조회 (`status IN ('running','queued')`, running 우선)
  2. 상태와 무관하게 worker pool에 cancellation intent를 먼저 기록
     - 이미 실행 중이면 in-memory `map[run_id]context.CancelFunc` 로 워커에게 신호
     - 아직 cancel func가 없으면 pending-cancel set으로 queued→running 경계 race 보존
  3. HTTP fallback 트랜잭션에서 `run.status='cancelled', exit_code=-1, error_message='user cancelled'`
  4. fallback 결과 run이 claimed/started 전이면 pending-cancel 정리, claimed/started 흔적이 있으면 pending 유지
  5. 워커는 ctx.Done() 받으면 `syscall.Kill(-pgid, SIGTERM)` → 30초 후 SIGKILL
  6. **issue.status는 그대로 'open' 유지** (이슈는 살아있음)
  7. system 댓글 "사용자가 실행을 취소했습니다" INSERT

이슈 자체를 닫으려면 `PUT /api/issues/:id { status: 'cancelled' }` 사용 (별도 흐름, running run이 있으면 먼저 cancel).

### 8.6 Stdout cap (이중)
- **파일 cap**: 단일 run 10MB. 도달 시 파일 append 중단 + truncation 표시, **pipe는 io.Discard로 계속 drain**
- **comment cap**: 64KB. 초과 시 앞 60KB + 로그 다운로드 링크 append
- 에이전트가 너무 큰 결과를 토해내는 케이스(예: 코드 dump) 방어

### 8.7 Run trigger fields
- 모든 run row INSERT 시 `trigger_type` 필수: `issue_created | mention | autopilot | rerun`
- `mention`: `trigger_comment_id` 필수
- `trigger_content_snapshot`: 트리거 시점의 본문 스냅샷 (4KB cap)
  - `issue_created`: issue.body의 앞 4KB
  - `mention`: 댓글 content의 앞 4KB
  - `autopilot`: 룰 이름 + cron 표현식
  - `rerun`: `[rerun of run <id>]`
- `agent_mention` 또는 `auto_mention`은 auto-chain opt-in 구현 시 검토할 후보 enum이며 현재 schema/API에는 없다.

### 8.8 Run is stateless
- run 테이블에 `session_id`, `work_dir` 컬럼 없음 (의도적)
- 모든 run은 매번 `workspace.working_dir`을 cwd로 사용
- 에이전트 간 working_dir 격리는 Phase 2 후보 (per-run worktree)

### 8.9 Auto-chain opt-in 후보 (미구현)

Auto-chain은 agent 결과 댓글의 mention을 자동으로 다음 run으로 dispatch하는 후보 기능이다. 현재는 구현하지 않는다.

권장 설계:
- 기본값 off.
- lineage 후보 필드: `run.chain_id`, `run.parent_run_id`, `run.chain_depth`.
- 최대 depth 기본값 5.
- 같은 `chain_id` 안에서 동일 agent 재호출 차단.
- source run이 `failed` 또는 `cancelled`이면 chain 중단.
- 후보 `trigger_type`: `agent_mention` 또는 `auto_mention` 중 하나를 별도 migration/API 변경에서 선택.

후보 store 흐름:
1. agent 결과 comment 저장 후 opt-in 상태 확인.
2. off이면 mention parser를 실행하지 않고 종료.
3. on이면 agent comment의 첫 mention만 후보로 삼고 depth/중복/상태 guard를 통과할 때만 새 run INSERT.
4. dispatch 결과는 run_event 또는 system comment로 남긴다.

---

## 9. Cron / Autopilot 상세

### 9.1 라이브러리
- `robfig/cron/v3` (in-process)
- precision: 1분
- 등록 방식: `cron.AddFunc(spec, callback)` (DB scan 폴링 대신)

### 9.2 Timezone
- 시스템 전역: 환경변수 `CORN_AGENT_DASHBOARD_TIMEZONE` (기본 `Asia/Seoul`)
- 부팅 시 `cron.New(cron.WithLocation(loc))` 생성
- 룰별 timezone 컬럼은 두지 않음 — 단일 사용자 가정

### 9.3 룰 적용
- DB의 `autopilot_rule WHERE enabled=true` 부팅 시 전체 로드 → AddFunc로 등록
- 룰 변경(CRUD) 시 scheduler 전체 reload (단순함 우선)
- `next_run_at` 계산해서 룰 카드에 표시

### 9.4 꺼져 있는 동안의 cron
- **누락된 시각은 실행하지 않음** (robfig 기본 동작)
- 부팅 시 `next_run_at = cron.Next(time.Now())`로 재계산
- 사용자에게는 UI에서 "마지막 실행: <시각>" 표시로 누락 여부 식별 가능

### 9.5 cron expression
- 표준 5필드 (`* * * * *`)
- preset 매핑 (UI):
  - 매시간 → `0 * * * *`
  - 매일 09:00 → `0 9 * * *`
  - 매주 일요일 18:00 → `0 18 * * 0`
  - 직접 입력 → 임의 (백엔드 검증)

### 9.6 템플릿 변수 (issue_title_template / body_template)
- `{{date}}` → `2026-05-11` (시스템 timezone)
- `{{datetime}}` → `2026-05-11 09:00`
- `{{time}}` → `09:00`
- 알 수 없는 변수: 400 에러 (룰 저장 시 검증)

---

## 10. Frontend 기술 결정

### 10.1 Vite SPA 단일 바이너리 임베드 전략
- 빌드: `vite build` → `web/dist/` 정적 산출물
- Go에서 `embed.FS`로 `web/dist/` 묶음
- Go static handler:
  - `/api/*` → API 라우터
  - `/assets/*`, `*.js`, `*.css`, `*.svg` 등 → 파일 그대로 서빙
  - 그 외 모든 경로 → `index.html` (SPA fallback, React Router가 client routing)
- 데이터는 클라이언트에서 `/api/*` 호출

### 10.2 Corn Design Reference에서 재사용할 컴포넌트
- 사전 작업 (Phase 0): Corn Design Reference `web/`에서 RSC/`'use server'` 사용 여부 스캔
- 재사용 후보: `IssueBoard`, `IssueDetail`, `CommentThread`, `AgentEditor`의 client-only 추출
- 테마/스타일 토큰 그대로 (Tailwind config 복사)
- 한국어 namespace에서 필요한 string만 inline화 (i18next 의존 제거)

### 10.3 라우팅 구조 (React Router)
| Path | 페이지 |
|---|---|
| `/` | 워크스페이스 목록 (선택 화면) |
| `/w/:slug/board` | 이슈 보드 |
| `/w/:slug/issues/:identifier` | 이슈 상세 (URL은 identifier `NEWS-12`, API는 id) |
| `/w/:slug/agents` | 에이전트 목록 |
| `/w/:slug/agents/:id` | 에이전트 편집 |
| `/w/:slug/autopilot` | Autopilot 룰 |
| `/settings` | 전역 설정 (CLI 가용성, 토큰 등) |

7 페이지 fix.

### 10.4 Frontend ↔ Backend 시간 표시
- 모든 timestamp는 백엔드가 RFC3339 UTC 전송
- 프론트는 `Intl.DateTimeFormat('ko-KR', { timeZone })`로 로컬 표시
- timeZone은 `/api/settings`에서 받은 `timezone` 값 사용 (기본 `Asia/Seoul`)

---

## 11. Deployment (배포)

### 11.1 배포 단위
- 단일 정적 바이너리 (`corn-agent-dashboard` ~30MB 예상)
- macOS arm64/amd64, Linux amd64/arm64 4개 빌드

### 11.2 실행
```bash
corn-agent-dashboard serve \
  --db ~/.corn-agent-dashboard/data.db \
  --bind 127.0.0.1:8080 \
  --workers 3 \
  --timezone Asia/Seoul
```

환경변수 대체 가능:
- `CORN_AGENT_DASHBOARD_DB`, `CORN_AGENT_DASHBOARD_BIND`, `CORN_AGENT_DASHBOARD_WORKERS`, `CORN_AGENT_DASHBOARD_TIMEZONE`, `CORN_AGENT_DASHBOARD_TOKEN`, `CORN_AGENT_DASHBOARD_CORS`

### 11.3 init
```bash
corn-agent-dashboard init    # 디렉토리 생성, DB 마이그레이션
```

### 11.4 업그레이드
- 새 바이너리 교체 → 재시작
- 마이그레이션 자동 적용

---

## 12. Open Technical Decisions

| 항목 | 결정 | 결정 시점 |
|---|---|---|
| Frontend 스택 | **Vite + React Router SPA** (확정, Next.js/next-themes 미사용) | Phase 0 |
| `issue.status` vs `run.status` 분리 | **분리** (issue: open/done/cancelled, run: queued/running/done/failed/cancelled). execution_status는 API derived | Phase 0 |
| 워크스페이스 동시 실행 | **MVP는 워크스페이스당 1개** (per-run worktree는 Phase 2) | Phase 0 |
| 멘션 시 담당자 유지 | **issue.assignee_agent_id는 유지** (`run.agent_id`만 멘션된 agent) | Phase 0 |
| identifier URL 처리 | **`:idOrIdentifier` 다형 라우팅** | Phase 0 |
| stdout → comment | **64KB cap + raw HTML 금지 + pipe drain** | Phase 0 |
| run trigger fields | **`trigger_type / trigger_comment_id / trigger_content_snapshot` 추가** | Phase 0 |
| [재실행] 대상 agent | **마지막 run의 agent** (issue.assignee와 다를 수 있음) | Phase 0 |
| Durable queue | **`run.status='queued'` + DB claim** (확정) | Phase 0 |
| Cancel status | **`cancelled` 별도** (확정) | Phase 0 |
| Sub-issue 트리 | **Phase 2로 이동** (확정, MVP는 멘션 = 같은 issue 새 run) | Phase 0 |
| 멘션 정책 | **첫 멘션만 dispatch** (확정) | Phase 0 |
| Timezone | **시스템 전역 `CORN_AGENT_DASHBOARD_TIMEZONE`** (기본 `Asia/Seoul`) | Phase 0 |
| Stdout cap | **단일 run 10MB** (확정) | Phase 0 |
| Prompt 컨텍스트 | **truncation 4000자, 요약 없음** | Phase 0 |
| 체이닝 정책 | **explicit-only**. 사용자 댓글의 명시 멘션만 dispatch, agent 결과 멘션 auto-chain은 Phase 2+ opt-in 후보(기본 off) | Phase 0 |
| stdout 스트리밍 단위 | 종료 후 1번 (확정 MVP) — 라인 단위는 Phase 2 후보 | Phase 0 |
| run log 압축 | gzip vs raw — Phase 2에서 디스크 사용량 보고 후 결정 | Phase 2 |
| 워크스페이스 import/export | Phase 2 | Phase 2 |

---

## 9. 2026-05-14 구현 업데이트

이번 구현으로 개인용 AI agent 운영 콘솔에 필요한 5개 기반 기능이 추가되었다.

1. **Run Lifecycle Hardening**: heartbeat, structured terminal reason, stale/orphan/panic/shutdown recovery.
2. **Run Event / Audit Trail**: `run_event` 테이블과 `GET /api/runs/:id/events`.
3. **Autopilot Failure Visibility**: `last_error`, `consecutive_failures`, `last_triggered_issue_id`, trigger 결과 응답.
4. **Frontend Feedback Foundation**: `StatusPill`, `ConfirmDialog`, `ToastProvider`, `MutationErrorAlert`, `DateTimeText`.
5. **Issue Operations Console + Board Filters**: Board URL 기반 status/execution/agent/search 필터, Issue Detail 우측 운영 레일, run event timeline.

검증 기준은 `go test ./...`, `go vet ./...`, `pnpm --filter web build`, `make check` 통과다.
