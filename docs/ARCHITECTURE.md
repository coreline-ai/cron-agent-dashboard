# ARCHITECTURE — corn-agent-dashboard

> 아키텍처 개요
> Version: 0.1
> Date: 2026-05-11

---

## 1. 시스템 다이어그램

### 1.1 전체 구성

```
┌─────────────────────────────────────────────────────────────────┐
│                        브라우저 (사용자)                          │
│                  http://127.0.0.1:8080                          │
└─────────────────────────────┬───────────────────────────────────┘
                              │ HTTPS X / HTTP O (localhost)
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│              corn-agent-dashboard binary (단일 프로세스)                   │
│                                                                 │
│  ┌──────────────────────────────────────────────────────┐       │
│  │              HTTP Server (chi router)                │       │
│  │                                                      │       │
│  │  /api/workspaces/*    /api/agents/*                  │       │
│  │  /api/issues/*        /api/comments/*                │       │
│  │  /api/autopilot/*     /healthz                       │       │
│  │                                                      │       │
│  │  /  /w/:slug/*  →  embed.FS (Vite SPA) + fallback     │       │
│  └──────────────────┬──────────────┬────────────────────┘       │
│                     │              │                            │
│                     ▼              ▼                            │
│         ┌───────────────────┐  ┌─────────────────────┐          │
│         │   store (sqlx)    │  │  worker pool        │          │
│         │   CRUD layer      │  │  (N goroutines)     │          │
│         └────────┬──────────┘  └──────────┬──────────┘          │
│                  │                        │                     │
│                  │              ┌─────────▼──────────┐          │
│                  │              │  executor          │          │
│                  │              │  exec.Command      │          │
│                  │              │  (codex/claude/...)│          │
│                  │              └─────────┬──────────┘          │
│                  │                        │ stdout              │
│                  ▼                        ▼                     │
│         ┌────────────────────────────────────┐                  │
│         │       SQLite (data.db)             │                  │
│         │   workspace / agent / issue /      │                  │
│         │   comment / run / autopilot_rule   │                  │
│         └────────────────────────────────────┘                  │
│                                                                 │
│  ┌──────────────────────────────────────────────────────┐       │
│  │            Cron Scheduler (robfig/cron)              │       │
│  │  ─ autopilot_rule scan                               │       │
│  │  ─ 시각 도래 시 issue 자동 INSERT + dispatch         │       │
│  └──────────────────────────────────────────────────────┘       │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
                ┌─────────────────────────────┐
                │  외부 CLI agents (PATH)     │
                │   codex / claude / gemini   │
                └─────────────────────────────┘
```

---

## 2. 데이터 흐름

### 2.1 새 이슈 생성 → 결과까지 (Durable Queue 기반)

**핵심 차이 (이전 in-memory 큐 대비)**:
- HTTP 핸들러는 같은 트랜잭션 안에서 `issue` + `run(status='queued')` row만 INSERT — channel send 없음
- Worker는 **DB polling 기반 claim** (1초 주기): `BEGIN IMMEDIATE; SELECT ... WHERE status='queued' LIMIT 1; UPDATE → 'running'; COMMIT;`
- 같은 run을 두 워커가 동시에 claim 못 함 (BEGIN IMMEDIATE write lock + `WHERE id=? AND status='queued'` guard)
- 프로세스 재시작 → 큐 손실 0 (모든 작업이 DB row)
- Worker가 `running` 진입 직후 시스템 댓글 INSERT ("`<agent>` 실행을 시작했습니다 (run #N)")

```
User                    Frontend           Backend            Worker          CLI
 │                         │                  │                  │             │
 │ "+ 새 이슈" 클릭          │                  │                  │             │
 │ ───────────────────────▶│                  │                  │             │
 │                         │ POST /api/issues │                  │             │
 │                         │ ────────────────▶│                  │             │
 │                         │                  │ BEGIN            │             │
 │                         │                  │ issue INSERT     │             │
 │                         │                  │ (status=queued)  │             │
 │                         │                  │ run   INSERT     │             │
 │                         │                  │ (status=queued)  │             │
 │                         │                  │ COMMIT           │             │
 │                         │ 201 issue        │                  │             │
 │                         │◀──────────────── │                  │             │
 │ 이슈 상세 페이지 렌더링    │                  │                  │ 1s poll     │
 │                         │                  │                  │ claim run   │
 │                         │                  │                  │ status=     │
 │                         │                  │                  │ running     │
 │                         │                  │                  │ system      │
 │                         │                  │                  │ comment     │
 │                         │                  │                  │ "실행 시작"  │
 │                         │ (3초 polling)    │                  │             │
 │                         │ ────────────────▶│                  │             │
 │                         │                  │                  │ exec ──────▶│
 │                         │                  │                  │             │
 │                         │                  │                  │ stdout pipe │
 │                         │                  │                  │◀──────────  │
 │                         │                  │                  │ comment     │
 │                         │                  │                  │ INSERT      │
 │                         │ (3초 polling)    │                  │             │
 │                         │ ────────────────▶│                  │             │
 │ 댓글 화면 갱신            │◀──────────────── │                  │             │
 │                         │                  │                  │ Wait done   │
 │                         │                  │                  │ BEGIN       │
 │                         │                  │                  │ run UPDATE  │
 │                         │                  │                  │ comment INS │
 │                         │                  │                  │ issue UPD   │
 │                         │                  │                  │ COMMIT      │
 │                         │                  │ status=done      │             │
```

### 2.2 멘션 위임 흐름 (같은 이슈에 새 run, **assignee는 변경 X**)

```
User → 댓글 작성: "@Writer 이걸로 블로그 글 써줘"
  │
  ▼
POST /api/issues/:id/comments
  │
  ▼
BEGIN
  comment INSERT (author_type=user) → comment.id = C
  mention parser: 본문에서 @\w+ 정규식 추출
    - 0개: COMMIT 끝 (단순 댓글)
    - 1개: 아래로 진행
    - 2개 이상: 첫 멘션만 사용, system 댓글 "추가 멘션 무시" INSERT
  workspace 내 매칭 (lower(name) 기준)
    - 매칭 실패: system 댓글 "에이전트 @Foo 없음" INSERT, COMMIT 끝
    - 매칭 성공: 아래로 진행
  -- assignee_agent_id는 변경하지 않음 (멘션은 일회성 위임)
  -- 이슈가 done이었다면 open으로 되돌림 (멘션이 새 작업을 트리거)
  UPDATE issue SET status='open' WHERE id=? AND status='done';
  INSERT INTO run (
    issue_id, agent_id=matched, status='queued',
    trigger_type='mention',
    trigger_comment_id=C,
    trigger_content_snapshot=substr(comment.content, 0, 4000)
  );
  -- 중복 dispatch 차단: 같은 (issue, agent)에 이미 queued run 있으면
  -- idx_run_one_queued_per_issue_agent 위반 → 트랜잭션 rollback 후 응답에
  -- mention_warnings=["already queued for @<agent>"] 포함, 댓글만 저장
COMMIT

(이후 워커가 1초 polling으로 claim — 2.1 흐름으로 합류)
```

**중요**:
- sub-issue를 만들지 않는다. 동일 issue_id에 새 run이 추가될 뿐이며, 댓글 스레드에 시간순으로 누적된다.
- **`issue.assignee_agent_id`는 변경하지 않는다.** 기본 담당은 그대로. UI는 "담당: NewsLead / 최근 실행: Writer" 형태로 분리 표시.
- run에 `trigger_comment_id`를 채워두면 디버깅 / 추적에 유용. 댓글 삭제 시 SET NULL.
- 현재 정책은 **explicit-only**다. 이 흐름은 `author_type='user'` 댓글의 `@AgentName` 명시 멘션에서만 실행되며, agent 결과 댓글 안의 `@AgentName`은 자동 dispatch하지 않는다.
- agent 결과 멘션 자동 실행(auto-chain)은 무한 루프, 비용 폭주, hallucinated mention을 막기 위해 현재 미구현이다. Phase 2+에서 opt-in 후보로만 검토한다.

### 2.3 Autopilot 흐름

```
robfig/cron Scheduler (in-process, 등록 방식)
  - 부팅 시 enabled 룰을 cron.AddFunc(spec, callback)으로 전부 등록
  - 룰 CRUD/snooze 변경 시 scheduler 전체 reload (단순함 우선)
  - cron.Location = CORN_AGENT_DASHBOARD_TIMEZONE (기본 Asia/Seoul)
  - snooze_until > now 이면 callback/manual trigger는 issue/run 생성 없이 no-op
  │
  ▼ (룰의 시각 도래 시 callback 호출)
  │
  ▼
BEGIN
  issue INSERT
    - title = rule.issue_title_template (변수 치환: {{date}}, {{datetime}}, {{time}})
    - body  = rule.issue_body_template
    - assignee_agent_id = rule.assignee_agent_id (or workspace 메인 agent fallback)
    - status = 'queued'
    - created_by = 'autopilot'
    - autopilot_rule_id = rule.id
  run INSERT (status='queued')
  rule UPDATE: last_run_at = now, next_run_at = cron.Next()
  (snooze_until이 미래면 BEGIN 이전 no-op, failure count 증가 없음)
COMMIT

(이후 워커가 polling으로 claim — 2.1 흐름으로 합류)
```

**꺼져 있는 동안의 시각**:
- 서버가 꺼져 있던 시간에 도래했어야 할 시각은 **실행하지 않는다** (robfig 기본 동작)
- 부팅 시 `next_run_at`을 재계산 (cron.Next from now, snooze 중이면 만료 이후 첫 cron)
- 사용자 안내: Autopilot UI 카드에 "마지막 실행: ..." 표시로 누락 여부 확인 가능

---

## 3. 컴포넌트 책임 분리

### 3.1 HTTP Layer
- **책임**: 요청 파싱, 응답 직렬화, 인증/CORS 미들웨어
- **NOT 책임**: 비즈니스 로직, SQL 직접 호출
- 모든 핸들러는 `store` 메서드를 호출하고 결과를 변환만 함

### 3.2 Store Layer
- **책임**: SQL 실행 (sqlx), 트랜잭션 관리, 도메인 객체 매핑
- **NOT 책임**: HTTP, 외부 프로세스, 의사결정
- 함수 하나 = 트랜잭션 하나 원칙

### 3.3 Worker
- **책임**: 큐에서 이슈 pickup → executor 호출 → 결과 store에 기록
- 동시성: per-issue lock, pool 크기 제한
- 실패 격리: 한 goroutine panic이 pool 전체를 죽이지 않음

### 3.4 Executor
- **책임**: `exec.Command` spawn, stdout/stderr 캡처, 타임아웃, 취소
- **NOT 책임**: 큐잉, DB, HTTP

### 3.5 Scheduler
- **책임**: cron 평가, 시각 도래 시 store에 issue INSERT
- **NOT 책임**: 직접 exec

### 3.6 Frontend
- **책임**: UI, polling, 사용자 입력 검증
- **NOT 책임**: 비즈니스 규칙 (모두 백엔드에)

---

## 4. 상태 전이

### 4.1 issue.status vs run.status — **분리된 상태머신**

**핵심 원칙**: issue.status는 **사용자 의도**, run.status는 **실행 상태**. 두 도메인은 절대 합치지 않는다.

#### issue.status (사용자 의도, 3개)

```
            ┌──────┐
            │ open │  ← 생성 직후, 그리고 거의 항상
            └───┬──┘
                │
       ┌────────┴────────┐
       │                 │
   마지막 run         사용자 명시
   == 'done'          PUT status='done'
       │                 │
       └────────┬────────┘
                ▼
            ┌──────┐
            │ done │  ← 자동 또는 수동 닫기. [재실행] 누르면 다시 open으로.
            └──────┘

       open ─── 사용자 명시 PUT status='cancelled' ──▶ cancelled
                                                      (queued run도 자동 취소)
```

- `open` → 작업 진행 중 또는 진행 가능 (queued/running/failed run이 있어도, 단순히 run이 없어도)
- `done` → 마지막 run이 `done`이면 자동 전이. 또는 사용자가 명시적으로 PUT.
- `cancelled` → 사용자가 이슈 자체를 포기 (DELETE 대신 보존하고 닫음).

#### run.status (실행 상태, 5개)

```
            ┌────────┐
            │ queued │  ← INSERT (issue 생성/rerun/멘션/autopilot)
            └───┬────┘
                │ worker claim (workspace 직렬화)
                ▼
            ┌─────────┐
            │ running │
            └────┬────┘
                 │
   ┌─────────────┼─────────────┐
   │             │             │
exit=0       exit≠0/timeout  user cancel /
   │             │           shutdown / orphan
   ▼             ▼             ▼
┌──────┐    ┌────────┐    ┌───────────┐
│ done │    │ failed │    │ cancelled │
└──────┘    └────────┘    └───────────┘
```

#### issue.status 자동 전이 트리거
- 새 run이 `running` 진입 → issue.status는 그대로 (open 유지). UI는 `execution_status='running'`으로 표시.
- run이 `done`으로 종료 → 같은 트랜잭션에서 `UPDATE issue SET status='done'`.
- run이 `failed/cancelled`로 종료 → issue.status 그대로 (open 유지). 사용자가 [재실행] 또는 직접 close.
- [재실행] 호출 → issue.status='done'이었으면 'open'으로 되돌림 + 새 run INSERT.

### 4.2 execution_status (derived field, API 응답에만)

```
if any run.status == 'running' for this issue → 'running'
elif any run.status == 'queued'              → 'queued'
elif last run by enqueued_at                  → that run's status (done/failed/cancelled)
else                                          → 'idle'  (run 없음, 예외적)
```

DB 컬럼 없음. API 응답 시 LEFT JOIN으로 계산.
UI 보드 배지는 execution_status를 표시 (run의 현재 상황이 직관적).

### 4.3 상태별 종료 사유 (run)
| Status | 발생 케이스 |
|---|---|
| done | exit_code == 0 |
| failed | exit_code != 0, 타임아웃, executor 에러 (예: CLI not found) |
| cancelled | `POST /api/issues/:id/cancel`(queued/running active run), 재시작 시 orphan 정리, graceful shutdown 중 강제 종료, 이슈 자체 cancel 시 queued run 자동 취소 |

---

## 5. 동시성 모델

### 5.1 직렬화 (per-issue + **per-workspace**)
- **DB 레벨로 직렬화**: claim 쿼리에서 보장
  - **per-issue**: 같은 이슈에 동시 running 없음 (NOT EXISTS run r2)
  - **per-workspace (MVP)**: 같은 워크스페이스에 동시 running 없음 (NOT EXISTS run r3 JOIN issue i3)
- 워크스페이스 직렬화 근거: 같은 `workspace.working_dir`에서 두 에이전트가 동시 실행 시 파일 충돌, git 상태 꼬임 위험. MVP는 안전 우선.
- Phase 2 후보: per-run worktree (git worktree 또는 temp dir 분리) → 그때 가서 병렬화 활성화
- in-memory mutex 불필요. 부팅 시 DB에 남은 `running` row는 self-check가 `process_pgid`/`process_recorded_at` 기반 best-effort process cleanup 후 orphan recovery로 정리한다.

### 5.2 Worker pool (durable / DB-claim)
- **channel 없음**. N개 goroutine이 각각 1초 간격으로 polling
- claim 쿼리:
  ```sql
  BEGIN IMMEDIATE;
    SELECT r.id, r.issue_id FROM run r JOIN issue i ON i.id=r.issue_id
     WHERE r.status='queued'
       AND NOT EXISTS (SELECT 1 FROM run r2 WHERE r2.issue_id=r.issue_id AND r2.status='running')
       AND NOT EXISTS (SELECT 1 FROM run r3 JOIN issue i3 ON i3.id=r3.issue_id
                        WHERE i3.workspace_id=i.workspace_id AND r3.status='running')
     ORDER BY r.enqueued_at ASC, r.id ASC
     LIMIT 1;
    UPDATE run SET status='running', claimed_at=now, claimed_by=?
     WHERE id=? AND status='queued';
  COMMIT;
  ```
  - UPDATE에 `status='queued'` 조건이 있어서 두 워커가 동시에 같은 row를 잡아도 한쪽만 affected_rows=1
  - 같은 workspace의 다른 issue들이 모두 같은 시점에 NOT EXISTS 통과해도, UPDATE 단계의 BEGIN IMMEDIATE write lock + WHERE guard로 한쪽만 성공. 다른 워커는 다음 polling에 다시 SELECT (이때 NOT EXISTS 실패) → 자연스럽게 1개만 진행.
- pool size 변경: 재시작 필요 (Phase 1). default 3.
- 1초 polling은 SQLite WAL에 거의 부담 없음 (read-only SELECT, lock 없음)
- 워크스페이스 직렬화로 인해 worker pool size 3이라도 같은 워크스페이스에서는 사실상 직렬. 다른 워크스페이스끼리만 병렬.

### 5.3 SQLite WAL
- `_journal_mode=WAL` 설정 → 동시 read 다수 + write 1
- store의 모든 write는 짧은 트랜잭션

---

## 6. 에러 처리 원칙

### 6.1 사용자 에러 (4xx)
- 입력 검증 실패 → 400 + 한국어 메시지
- 존재하지 않는 리소스 → 404
- 권한 없음 (토큰 모드) → 401/403

### 6.2 시스템 에러 (5xx)
- DB 에러 → 500 + slog ERROR + 클라이언트엔 "잠시 후 다시 시도"
- exec 실패 → run.status='failed', issue.status는 유지(open). 5xx 아님, 정상 동작

### 6.3 부분 실패
- 멘션 파싱 실패 → 댓글은 저장, dispatch만 skip + 경고 댓글 추가
- Autopilot 룰 실행 실패 → 룰은 살아있고 run.status='failed'로 기록, issue.status는 유지

---

## 7. 확장 포인트 (Future)

> Phase 1 범위는 아니지만 설계가 막지 않도록 둔다.

| 포인트 | 어떻게 |
|---|---|
| 새 에이전트 CLI 추가 | `internal/worker/executor.go` 의 cli map에 추가 |
| 첨부 파일 | `attachment` 테이블 + `runs/` 옆에 파일 저장 |
| 외부 봇 통합 | 이미 REST API가 외부 호출 가능 → 토큰만 발급 |
| 이슈 webhook | `webhook` 테이블 + worker 후처리 hook |
| Auto-chain opt-in | 현재 미구현. 기본 off, `chain_id`/`parent_run_id`/`chain_depth`와 max depth guard가 필요 |
| 다국어 | 한국어 string을 i18n 파일로 추출 (지금은 inline) |
| 멀티 사용자 | 본 프로젝트 범위 외 (별도 fork) |

---

## 8. Corn Design Reference 대비 차이 (검증된 결정만 가져옴)

| Corn Design Reference 결정 | corn-agent-dashboard 채택? | 이유 |
|---|---|---|
| Polymorphic assignee (`type,id`) | ❌ | 멤버 없음 → agent_id 단일 컬럼 |
| WebSocket Hub | ❌ | polling 3s로 충분 (단일 사용자) |
| sqlc | ❌ | 1.5k LOC엔 sqlx가 충분 |
| Daemon 별도 프로세스 | ❌ | in-process worker로 통합 |
| pgvector / embedding | ❌ | 필요 시 v2 |
| in-memory queue (chan) | ❌ | **재시작 시 손실** → DB-backed durable queue |
| Sub-issue 트리 | ❌ (Phase 2) | 멘션 = 같은 issue 새 run으로 충분 |
| Next.js (Corn Design Reference) | ❌ → Vite | SSR/RSC 미사용 + static export 동적 라우트 회피 |
| `exec.Command` CLI spawn | ✅ | 검증된 방식 (runtime adapter로 추상화) |
| Identifier prefix (`NEWS-12`) | ✅ | 사람 기억하기 좋음 |
| `@AgentName` 멘션 | ✅ | 위임 체인의 핵심 (첫 멘션만 적용) |
| chi router | ✅ | 가벼움, 익숙함 |
| shadcn (Base UI는 선택) | ✅ | 검증된 컴포넌트 |
| 다크모드 | ✅ | UX 일관성 |
| 한국어 | ✅ | 대상 사용자 |

---

## 8. Run lifecycle / audit trail 확장 (2026-05-14)

Worker pool은 run을 claim한 뒤 `heartbeat_at`을 주기적으로 갱신한다. 정상 종료는 `terminal_reason=completed`, 비정상 종료는 `terminal_reason`/`failure_kind`, 취소는 `terminal_reason`/`cancel_reason`으로 구조화해서 저장한다.

추가 흐름:

1. run 생성 시 `run_event(run_queued)` 기록.
2. worker claim 시 `run_event(run_claimed)` 기록 및 heartbeat 시작.
3. executor process 시작 시 `process_pid`/`process_pgid`/`process_recorded_at`을 best-effort retry로 기록한다.
4. executor 시작/출력 절단/취소 요청/완료/실패/복구를 `run_event`에 append.
5. UI는 `GET /api/runs/:id/events`로 Issue Detail의 이벤트 타임라인을 표시한다.
6. stale scanner는 일정 시간 heartbeat가 멈춘 running run을 `cancelled + stale_recovered`로 복구한다.
7. startup self-check는 최근 process metadata만 kill 대상으로 삼아 OS process group id 재사용 리스크를 줄인다.

이 구조는 system comment를 대체하지 않는다. system comment는 사용자 작업 스레드 표시용, run_event는 운영/디버깅용이다.

---

## 9. 체이닝 정책과 auto-chain 후보

현재 제품 정책은 **사용자 명시 멘션 기반 위임(explicit-only)**이다.

- 사용자 댓글의 `@AgentName`만 같은 issue에 새 run을 만든다.
- agent 결과 댓글의 `@AgentName`은 markdown 텍스트로만 보존하고 자동 dispatch하지 않는다.
- 이 정책은 무한 루프, 예상치 못한 비용 폭주, agent hallucinated mention 실행을 방지하기 위한 기본 안전장치다.

Auto-chain은 현재 기능이 아니라 **Phase 2+ opt-in 후보**다. 구현 시 기본값은 off로 두고, 다음 guard가 필요하다.

| 후보 guard | 권장값 |
|---|---|
| 최대 chain depth | 5 |
| 같은 chain 내 동일 agent 재호출 | 차단 |
| source run이 `failed`/`cancelled` | chain 중단 |
| 후보 lineage 필드 | `chain_id`, `parent_run_id`, `chain_depth` |
| 후보 trigger_type | `agent_mention` 또는 `auto_mention` 중 하나 선택 |

상세 설계 초안은 [CHAINING.md](CHAINING.md)를 참조한다.

### Resource controls foundation

Run execution now carries a small resource-control envelope:

- Timeout is resolved as `issue.timeout_seconds_override → agent.timeout_seconds_override → workspace.default_timeout_seconds → executor default`.
- Runtime adapters can parse best-effort metrics from stdout/stderr. Captured values are stored on `run.input_tokens`, `run.output_tokens`, `run.total_cost_micros`, and `run.model_resolved`.
- Transient retry is intentionally narrow: only `timeout` and `executor_error` can be rescheduled, using `attempt`, `max_attempts`, and `next_retry_at`. Non-zero process exits and worker panics stay terminal.
