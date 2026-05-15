# ROADMAP — corn-agent-dashboard

> 페이즈별 개발 계획
> Version: 0.1
> Date: 2026-05-12
> Status: Local MVP integrated

---

## 0. 페이즈 개요

| Phase | 목표 | 산출물 | 예상 LOC |
|---|---|---|---|
| **P0** | 프로젝트 셋업 | go module + Vite skeleton + CI | ~100 |
| **P1** | 백엔드 코어 | DB + store + REST API (인증 없음) | ~800 |
| **P2** | 에이전트 실행 | Worker pool + executor + 멘션 파싱 | ~300 |
| **P3** | Autopilot | cron + 룰 CRUD + 트리거 | ~150 |
| **P4** | Frontend | 7 페이지 구현 | ~3000 |
| **P5** | 통합 / 임베드 | static export + embed.FS + 단일 binary | ~50 |
| **P6** | 품질 / 운영 | 부팅 검증, orphan 정리, 백업/복구 | ~150 |
| **P7** | 릴리스 | 크로스 컴파일, README, 데모 | ~50 |

**총 예상**: Go ~1,500 LOC + Frontend ~3,000 LOC.

**현재 구현 상태 (2026-05-12)**

- P0 골격, Go DB/API foundation, worker/runtime/scheduler foundation, Vite 7-route foundation 구현 완료.
- Worker/store/main 실행 연결, DB-backed Autopilot scheduler reload 연결, Frontend read/write API action 연결 완료.
- Go `embed.FS` static serving + SPA fallback, CLI backup/restore, startup self-check, release-build matrix 구현 완료.
- `go test ./...`, `pnpm --filter web build`, `make check`, `make e2e-smoke`, `make verify-clean-clone`, `make release-build`로 검증한다.
- 후속 polish: log retention 자동화, 성능 fixture, 스크린샷/데모 seed, 원격 CI green 확인.
- 남은 항목은 [`TODO.md`](../TODO.md)와 [`dev-plan/implement_20260512_180648.md`](../dev-plan/implement_20260512_180648.md)에서 우선순위별로 추적한다.

---

## Phase 0 — 프로젝트 셋업 + 핵심 의사결정 (1~2일)

### 목표
빈 저장소에서 빌드/포맷이 도는 최소 골격 + **구현 착수 전 결정해야 할 항목을 모두 확정**.

### 의사결정 (Phase 1 시작 전 필수)
- [x] **Frontend 스택 = Vite + React Router SPA (확정)**
  - 근거: SSR/RSC 미사용, static export 동적 라우트 우회 비용 없음, Corn Design Reference 컴포넌트는 client-only로 추출
  - Phase 0 액션: Corn Design Reference 프론트에서 재사용할 컴포넌트들의 RSC/`'use server'` 사용 여부 스캔 → client 변형 추출 계획 수립
- [x] **시스템 timezone 정책**: 환경변수 `CORN_AGENT_DASHBOARD_TIMEZONE` (기본 `Asia/Seoul`)
- [x] **꺼져 있는 동안의 cron**: 누락된 시각은 실행 안 함 (robfig 기본 동작)
- [x] **stdout cap**: 단일 run 최대 10MB (초과 시 truncation + 경고)
- [x] **Worker 병렬 처리 기준**: workspace 직렬화. worker pool 3은 다른 workspace 병렬 실행용

### Tasks
- [x] `go.mod` 생성: `github.com/coreline-ai/corn-agent-dashboard`
- [x] 디렉토리 구조 생성 (`cmd/`, `internal/`, `web/`, `docs/`)
- [x] `web/` Vite + React + TS skeleton init (Tailwind/shadcn은 후속)
- [x] `.gitignore`, `.editorconfig`, `Makefile` 추가
- [x] `cmd/corn-agent-dashboard/main.go` — `init`/`serve` skeleton
- [x] GitHub Actions: `make check` skeleton

### 완료 기준
- [x] `go build ./...` 성공
- [x] `pnpm --filter web build` 성공 (Vite 빌드)
- [x] CI workflow 구성 (`make check` + Playwright smoke), 원격 green은 push 후 확인
- [x] 위 의사결정 항목 모두 TRD에 반영됨

---

## Phase 1 — 백엔드 코어 (3~5일)

### 목표
DB 마이그레이션 + 6 테이블 store + 30 REST 엔드포인트 (worker 없이 mock).

### Sub-phases

#### P1.1 DB + Migration (1일)
- [x] `internal/db/db.go` — sqlite open, pragma, migrate
- [x] `internal/db/migrations/0001_init.sql` — 6 테이블 + meta
  - **run 테이블에 `status / claimed_at / claimed_by` 포함** (durable queue)
  - issue.status 도메인: `open | done | cancelled`
  - workspace.working_dir 컬럼 포함
  - agent 테이블 case-insensitive unique 인덱스
- [x] `internal/db/migrations/0002_indexes.sql`
- [x] 부팅 시 migration 자동 적용 + idempotent
- [x] 부팅 시 orphan 정리: `run.status='running' AND finished_at IS NULL` → `cancelled` + error_message
- [x] 단위 테스트: 두 번 부팅해도 멱등

#### P1.2 Store Layer (2일)
- [x] `internal/store/store.go` — Workspace / Agent CRUD + Promote
- [x] `internal/store/issues.go` — Issue CRUD + identifier 발급
- [x] `internal/store/runs.go` / `cancellation.go` / `reasons.go` — durable queue claim, run lifecycle, cancel/recovery reason helpers
- [x] `internal/store/comments.go` — Comment / mention dispatch
- [x] `internal/store/autopilot.go` — Autopilot CRUD / trigger visibility
- [x] `internal/store/auto_chain.go` — workspace opt-in auto-chain guard
- [x] Store 통합 테스트 (temporary SQLite)

#### P1.3 HTTP Layer (1.5일)
- [x] `internal/httpapi/server.go` — chi 라우팅 등록
- [x] API skeleton 핸들러 구현
- [x] 에러 응답 형식 통일
- [x] 입력 검증 (inline)
- [x] CORS / 토큰 미들웨어

#### P1.4 통합 테스트 (0.5일)
- [x] httptest로 핸들러 통합 테스트 (워크스페이스 생성 → 이슈 생성)
- [x] 에러 경로 일부 검증 (validation, conflict)

### 완료 기준
- 30개 엔드포인트 모두 응답 (worker 부분은 mock dispatch)
- `curl` 시나리오 통과:
  ```
  POST /api/workspaces        → 201
  POST /api/workspaces/:id/issues → 201 (status=queued)
  GET  /api/issues/:id        → status=queued
  GET  /api/issues/:id/comments → []
  ```

---

## Phase 2 — 에이전트 실행 (2~3일)

### 목표
Worker pool + executor + 멘션 파싱. P1에서 queued로 멈춰있던 이슈가 실제로 실행되고 댓글이 생성됨.

### Tasks

#### P2.1 Runtime Adapter + Executor (1.5일)
- [x] `internal/worker/runtime/adapter.go` — `RuntimeAdapter` 인터페이스 (TRD §1.4 참조)
  - [x] `Name() string`
  - [x] `Detect(ctx) RuntimeInfo` — PATH/version 검출
  - [x] `BuildCommand(ctx, RunContext) (*exec.Cmd, []byte stdin, error)` — CLI별 인자/stdin/non-interactive 모드 분기
- [x] `internal/worker/runtime/codex.go`, `claude.go`, `gemini.go` CLI별 non-interactive command 구현
- [x] `internal/worker/executor.go`
  - [x] `cmd.SysProcAttr.Setpgid = true` (process group)
  - [ ] stdout 캡처 → 파일 append + 10MB cap (cap 후에도 `io.Discard`로 drain — pipe blocking 방지)
  - [ ] stderr → 메모리 ring buffer (마지막 4KB) → error_message
  - [ ] 타임아웃 600초 (context 기반)
  - [ ] cancel 지원: `syscall.Kill(-pgid, SIGTERM)` → 30초 후 SIGKILL
- [ ] Phase 1 결정: stdout 결과 댓글은 종료 후 1번 INSERT (64KB cap, 초과 시 로그 링크 append)
- [ ] 단위 테스트:
  - [ ] echo 명령으로 정상 실행
  - [ ] 10MB 초과 stdout drain 확인 (sleep + 무한 출력 시나리오)
  - [x] cancel 시 process group kill (자식의 자식까지 정리)

#### P2.2 Worker Pool (durable queue, 1.5일)
- [x] `internal/worker/pool.go` skeleton
  - [ ] N goroutine이 각각 `BEGIN IMMEDIATE; SELECT FROM run WHERE status='queued' ORDER BY started_at LIMIT 1; UPDATE run SET status='running', claimed_at, claimed_by; COMMIT;` 패턴으로 claim
  - [ ] queue 비면 1초 sleep 후 재폴링 (1초 정밀도면 충분)
  - [ ] per-issue 직렬화: claim 시 같은 issue_id에 이미 `running` run 있으면 skip (자연스러운 직렬화)
  - [ ] panic recover (run 정리 + 에러 메시지 기록)
- [ ] HTTP 핸들러는 channel enqueue가 아니라 **run row INSERT(status='queued')** 만 하면 끝
- [ ] graceful shutdown:
  - [ ] 신규 claim 정지
  - [x] 진행 중 run에 SIGTERM (자식 프로세스 group kill)
  - [x] 최대 30초 대기, 미종료 시 SIGKILL
  - [x] run.status='cancelled' + error_message="shutdown" 기록

#### P2.3 Prompt 렌더링 (0.3일)
- [ ] 템플릿: agent.instructions + issue.title + issue.body + 최근 댓글 3개
- [ ] 댓글 컨텍스트 길이 제한 (4000 chars)

#### P2.4 멘션 파싱 (0.5일)
- [x] `internal/worker/mention.go` — 정규식 `@([\p{L}\p{N}_\-]+)`
- [ ] 댓글 POST 시 호출 → 에이전트 매칭 (대소문자 무시, `lower(name)`)
- [ ] **첫 멘션만 사용** (multiple → 첫 번째 + system warning)
- [ ] 매칭 성공: 새 run INSERT (`trigger_type='mention'`, `trigger_comment_id`, `trigger_content_snapshot`). **issue.assignee_agent_id는 변경하지 않음**.
- [ ] 매칭 실패: warning system 댓글 추가
- [ ] 중복 dispatch (같은 issue, agent에 queued 존재): `idx_run_one_queued_per_issue_agent` unique 위반 → warning

#### P2.5 부팅 시 orphan 복구 (0.2일) — P1.1과 중복, 정합 확인만
- [x] `run.status='running' AND finished_at IS NULL` → `cancelled` + error_message="orphan recovered"
- [ ] 해당 이슈 → 같은 trigger로 새 run을 자동 생성 (재개) 또는 사용자가 수동 재실행
- [ ] **결정**: MVP는 **자동 재개 안 함**. 사용자가 보드에서 confirm 후 [재실행] 클릭. (자동 재개는 무한 루프 위험)

### 완료 기준
- 이슈 생성 → 30초 내 댓글에 결과 등장 (실행 시작 시 system 댓글 즉시)
- @Writer 멘션 → Writer가 같은 이슈에서 실행되고 댓글 추가
- 동시 이슈 3개 → 병렬 처리 확인
- 같은 이슈에 동시 dispatch 2회 → 직렬화 확인
- **재시작 시나리오**: 진행 중 SIGINT → 재시작 → `cancelled` 마감 확인, queued였던 이슈는 자동 재개

---

## Phase 3 — Autopilot (1~2일)

### 목표
cron 기반 자동 이슈 생성.

### Tasks
- [x] `internal/scheduler/cron.go` skeleton
  - [x] robfig/cron 통합
  - [x] 부팅 시 DB에서 enabled 룰 전부 로드
  - [x] 룰 CRUD 시 scheduler reload (전체 reload, 단순)
- [ ] 시각 도래 시:
  - [x] 템플릿 변수 치환 (`{{date}}`, `{{datetime}}`, `{{time}}`)
  - [x] issue INSERT (created_by=autopilot, autopilot_rule_id 채움)
  - [x] worker enqueue
  - [x] rule.last_run_at / next_run_at 업데이트
- [x] 수동 트리거 엔드포인트 (`POST /autopilot/:id/trigger`)
- [x] cron expression 검증 (룰 생성/수정 시)
- [x] 룰별 `snooze_until` 일시정지 + 만료 이후 `next_run_at` 동기화

### 완료 기준
- 1분 cron (`* * * * *`) 룰 생성 → 1분 후 이슈 자동 생성 확인
- 룰 OFF → 다음 tick에 실행 안 됨
- 룰 변경 → 다음 tick부터 새 cron 적용
- 수동 트리거 → 즉시 이슈 생성

---

## Phase 4 — Frontend (5~7일)

### 목표
7개 페이지 구현. Corn Design Reference의 스타일 토큰/컴포넌트 추출.

### Sub-phases

#### P4.0 셋업 (0.5일)
- [ ] Tailwind config (Corn Design Reference에서 복사)
- [ ] shadcn 컴포넌트 초기 셋업 (button/dialog/input/textarea/select/badge)
- [ ] 다크모드 (class 기반 토글, next-themes 미사용)
- [x] API client (`web/src/api/client.ts`) — fetch wrapper + 에러 처리
- [x] TanStack Query 셋업

#### P4.1 레이아웃 (0.5일)
- [ ] 헤더 (로고 + 워크스페이스 드롭다운 + 설정 + 다크모드)
- [ ] 사이드바 (보드/에이전트/자동화)
- [x] 라우팅 구조

#### P4.2 워크스페이스 페이지 (1일)
- [x] `/` — 목록 + 빈 상태
- [x] 새 워크스페이스 form (메인 에이전트 동시 생성)

#### P4.3 이슈 보드 (1일)
- [x] `/w/:slug/board` — 카드 list
- [ ] status 필터 / agent 필터 / 검색
- [x] [+ 새 이슈] form
- [x] polling 5초

#### P4.4 이슈 상세 (1.5일) — **가장 중요**
- [x] `/w/:slug/issues/:id`
- [x] 본문 + 댓글 스레드 (safe markdown 렌더)
- [ ] 사이드바 (상태/담당/작업/run 이력)
- [x] 댓글 입력 (@AgentName 자동완성은 후속 nice-to-have)
- [x] polling 3초 (queued/running일 때만)
- [x] 재실행 / 실행 취소 / 완료 처리 / 이슈 취소

#### P4.5 에이전트 페이지 (0.5일)
- [x] `/w/:slug/agents` — list + 추가 form
- [x] `/w/:slug/agents/:id` — 편집/승격/삭제 폼

#### P4.6 Autopilot 페이지 (1일)
- [x] `/w/:slug/autopilot` — 룰 카드 + on/off
- [x] 룰 생성/편집 모달 + toggle update
- [x] [지금 실행] / [삭제]
- [x] 일시정지 quick action(1일/1주/1개월/해제) + snooze 상태 표시

#### P4.7 설정 페이지 (0.5일)
- [x] `/settings` — 버전 + 런타임 가용성 + 워커 수 + 인증 모드 + 운영 actions

### 완료 기준
- 모든 페이지에서 API 호출 → DB 반영 확인
- 다크모드 정상
- 빈 상태 / 에러 상태 시각화 정상
- Korean string inline (i18n 라이브러리 없음)

---

## Phase 5 — 통합 / 임베드 (1일)

### 목표
프론트엔드 정적 자산을 Go 바이너리에 임베드. 단일 파일 배포.

### Tasks
- [x] `vite build` → `web/dist/` 정적 산출물
- [x] `go:embed web_dist/*` 로 Go에 임베드
- [ ] HTTP 라우터에 정적 핸들러 등록:
  - [x] `/api/*` → API 핸들러
  - [x] 그 외 모든 경로 → `index.html` fallback (SPA client routing)
  - [x] 정적 자산(`*.js`, `*.css`, `assets/*`)은 그대로 서빙
- [x] 빌드 스크립트 (`make build` → vite build → go build)
- [x] 단일 바이너리에서 `/`, `/w/:slug/board`, `/api/*` 모두 동작 확인
- [x] **static routing smoke test**: `/w/foo/issues/NEWS-1` 직접 접근(=새로고침) 시 index.html fallback → client routing 작동 검증

### 완료 기준
- `./corn-agent-dashboard serve` 한 줄로 UI + API 가능
- 바이너리 크기 50MB 이하
- 새로고침/직접 URL 입력으로 모든 페이지 진입 가능

---

## Phase 6 — 품질 / 운영 (2일)

### 목표
릴리스 가능한 수준의 안정성 / 운영성.

### Tasks
- [x] **부팅 자가검진**
  - [x] 워크스페이스마다 메인 에이전트 정확히 1개 확인
  - [x] orphan run 정리
  - [x] DB pragma / integrity / foreign key check 확인
- [ ] **백업/복구**
  - [x] `corn-agent-dashboard backup --to <path>` 명령
  - [x] `corn-agent-dashboard restore --from <path>` 명령
- [ ] **로그 관리**
  - [ ] `--log-retention-days N` 옵션
  - [ ] 부팅 시 N일 이상 된 stdout 파일 삭제
- [ ] **에러 핸들링 강화**
  - [ ] DB lock 충돌 시 재시도
  - [x] worker panic recover
  - [ ] HTTP 500 시 stack trace 로깅
- [ ] **성능 검증**
  - [ ] 이슈 1000개로 보드 로드 < 500ms 확인
  - [ ] 동시 3 worker로 메모리 < 100MB 확인

### 완료 기준
- 일주일 동안 데모 환경에서 정지 없이 가동
- 백업 → 다른 머신에서 복구 성공

---

## Phase 7 — 릴리스 (1일)

### Tasks
- [x] 크로스 컴파일 (darwin/arm64, darwin/amd64, linux/amd64, linux/arm64)
- [x] GitHub Release CI
- [x] README 마무리 (빠른 시작/검증 명령 동기화)
- [x] Playwright browser smoke (`make e2e-smoke`)
- [x] clean clone quick start 검증 (`make verify-clean-clone`)
- [x] GitHub Release artifact upload workflow
- [ ] 데모 워크스페이스 seed 옵션 (`--seed example`)
- [ ] 첫 사용자 시나리오 영상 / 스크린샷

### 완료 기준
- 첫 사용자가 README만 보고 5분 안에 첫 이슈 실행 성공

---

## 전체 일정 합산

| Phase | 예상 일수 |
|---|---|
| P0 | 1 |
| P1 | 3~5 |
| P2 | 2~3 |
| P3 | 1~2 |
| P4 | 5~7 |
| P5 | 1 |
| P6 | 2 |
| P7 | 1 |
| **합** | **16~22일** (혼자, 풀타임 가정) |

---

## 페이즈 의존성

```
P0 ─▶ P1 ─▶ P2 ─┬─▶ P5 ─▶ P6 ─▶ P7
                │
                ├─▶ P3 (P2와 병렬 가능, P1 의존)
                │
                └─▶ P4 (P1 의존, P2/P3 mock으로 진행 가능)
```

**병렬 가능 조합**:
- P3 + P4 (P1만 끝나면 동시 진행)
- P4 안에서도 페이지별 독립 작업 가능

---

## 리스크 / 완화

| 리스크 | 영향 | 완화 |
|---|---|---|
| stdout 캡처 race condition | 댓글 깨짐 | P2에서 종료 후 1번 INSERT (단순화) |
| stdout 폭주 (수 GB) | 디스크 / 메모리 고갈 | 단일 run 10MB cap, 초과 시 truncation + 경고 |
| Vite SPA 라우팅 (직접 URL 접근) | 404 | Go fallback handler가 비-API 경로를 index.html로 |
| cron timezone | 한국 시간 vs UTC | 시스템 전역 `CORN_AGENT_DASHBOARD_TIMEZONE` (기본 Asia/Seoul) |
| 꺼져 있던 동안의 cron 누락 | 사용자가 기대한 시각에 실행 안 됨 | 명시: "꺼져 있는 동안의 시각은 실행 안 함". UI에 안내. |
| SQLite write 동시성 | lock 에러 | WAL + 짧은 트랜잭션 + busy_timeout 5초 |
| Worker가 같은 run을 두 번 claim | 중복 실행 | BEGIN IMMEDIATE + UPDATE에 status='queued' AND id 조건 |
| 진행 중 프로세스가 부팅 후 살아있음 | 좀비 프로세스 | `process_pgid` + `process_recorded_at` 기반 startup cleanup 후 orphan recovery |
| CLI 미설치 환경 | UI에서 실행 불가 | 부팅 시 PATH 스캔 + 설정 페이지 안내 |
| 단일 사용자 가정 깨짐 (외부 노출) | 보안 사고 | --bind 0.0.0.0 + 토큰 필수 강제 |
| Corn Design Reference 컴포넌트 RSC 의존 발견 | Phase 4 작업 폭증 | Phase 0에서 사전 스캔 후 결정 |

---

## 명시적 제외 (다시 강조)

이 로드맵은 **단일 사용자**를 가정한다. 아래는 Phase 1~7에서 다루지 **않는다**:

- 멤버 / 권한 / 초대
- 모바일 / 데스크톱 앱
- 알림 / 이메일 / 푸시
- pgvector / 임베딩 / 시맨틱 검색
- 결제 / 구독 / SaaS 운영
- 다국어 (한국어 only)
- WebSocket 실시간
- Postgres / Docker 의존

추가하려면 별도 새 PRD가 필요하다.

---

## 다음 액션 (이 문서 직후)

1. PRD / TRD / ARCHITECTURE / DATA_MODEL / API / UX_FLOW 검토 → 이의 있으면 수정
2. **Phase 0 시작 권장**
3. 또는 Phase 1.1만 먼저 구현해서 빠르게 검증

## v0.1.x Resource Controls Foundation

- [x] Run token/cost/model metrics columns + best-effort runtime parser
- [x] Settings 7-day usage summary
- [x] Timeout resolve foundation (`issue > agent > workspace > executor default`)
- [x] Limited transient retry for timeout/executor_error
- [x] Unicode mention regex for multilingual agent names
