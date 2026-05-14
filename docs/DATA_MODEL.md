# DATA MODEL — corn-agent-dashboard

> SQLite 스키마 상세
> Version: 0.1
> Date: 2026-05-11

---

## 1. 개요

- 단일 SQLite 파일 (`~/.corn-agent-dashboard/data.db`)
- PRAGMA: `journal_mode=WAL`, `foreign_keys=ON`, `busy_timeout=5000`
- 모든 PK: UUID v4 (TEXT)
- 시간: **Go 레벨에서 `time.Now().UTC().Format(time.RFC3339Nano)`로 생성** — SQLite의 `datetime('now')`는 사용하지 않음 (RFC3339 호환 안 됨)
- DDL의 DEFAULT는 호환을 위해 `datetime('now')`로 두되, 애플리케이션 INSERT 시 명시적으로 RFC3339 값 전달
- 마이그레이션: forward-only, `internal/db/migrations/NNNN_*.sql`

총 **6 테이블** + 1 메타 테이블.

| 테이블 | 행 수 예상 | 목적 |
|---|---|---|
| `workspace` | 1~10 | 작업 도메인 분리 |
| `agent` | 워크스페이스당 1~5 | CLI 에이전트 정의 |
| `issue` | 누적 1k~10k | 작업 단위 |
| `comment` | 이슈당 2~20 | 결과/사용자 메시지 |
| `run` | 이슈당 1~3 | 실행 audit |
| `autopilot_rule` | 워크스페이스당 0~5 | cron 자동화 |
| `schema_migrations` | (메타) | 마이그레이션 이력 |

---

## 2. DDL

### 2.1 workspace
```sql
CREATE TABLE workspace (
  id          TEXT PRIMARY KEY,                    -- UUID
  name        TEXT NOT NULL,                       -- "AI 뉴스 큐레이션"
  slug        TEXT NOT NULL UNIQUE,                -- "ai-news" (URL용)
  description TEXT NOT NULL DEFAULT '',
  output_dir  TEXT NOT NULL DEFAULT '',            -- 결과물 저장 디렉토리 (에이전트가 글 쓰는 위치)
  working_dir TEXT NOT NULL DEFAULT '',            -- 에이전트 실행 cwd (빈값='~/.corn-agent-dashboard/workdirs/<slug>' 자동)
  identifier_prefix TEXT NOT NULL,                 -- "NEWS" (이슈 ID prefix)
  next_issue_seq INTEGER NOT NULL DEFAULT 1,       -- 다음 발급 번호 (트랜잭션 내 UPDATE...RETURNING)
  created_at  TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_workspace_slug ON workspace(slug);
```

**제약 / 규칙**:
- `slug`: 소문자 영숫자 + 하이픈만, 2~50자
- `identifier_prefix`: 대문자 영문 2~10자 (예: `NEWS`, `CODE`)
- `next_issue_seq`: 이슈 생성 시 `UPDATE workspace SET next_issue_seq=next_issue_seq+1 WHERE id=?` (트랜잭션 내)

### 2.2 agent
```sql
CREATE TABLE agent (
  id            TEXT PRIMARY KEY,                  -- UUID
  workspace_id  TEXT NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  name          TEXT NOT NULL,                     -- "NewsLead", "Writer"
  runtime       TEXT NOT NULL,                     -- "codex" | "claude" | "gemini"
  model         TEXT NOT NULL DEFAULT '',          -- 빈 문자열이면 runtime 기본 모델
  instructions  TEXT NOT NULL DEFAULT '',          -- 시스템 프롬프트
  is_main       INTEGER NOT NULL DEFAULT 0 CHECK (is_main IN (0, 1)),
  created_at    TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_agent_workspace ON agent(workspace_id);

-- 워크스페이스 내 이름 case-insensitive 유일 (멘션 매칭과 일관성)
CREATE UNIQUE INDEX idx_agent_name_ci
  ON agent(workspace_id, lower(name));

-- 워크스페이스당 메인 1개만 허용
CREATE UNIQUE INDEX idx_agent_main_unique
  ON agent(workspace_id) WHERE is_main = 1;
```

**제약 / 규칙**:
- `name`: 영숫자 + 한글 + 하이픈/언더스코어. `@AgentName` 멘션에 사용.
- 멘션 매칭은 `lower(name)` 기준 → `Writer`와 `writer`는 같은 에이전트로 취급. 두 행이 동시에 존재할 수 없음.
- `runtime`: 부팅 시 PATH 스캔으로 가용 목록 확인. 가용하지 않으면 UI에서 경고.
- 워크스페이스 생성 시 메인 에이전트도 같이 트랜잭션 INSERT (`is_main=1`)
- 메인 에이전트 삭제는 다른 에이전트를 메인으로 승격해야 가능 (애플리케이션 레벨 체크)

### 2.3 issue
```sql
CREATE TABLE issue (
  id                 TEXT PRIMARY KEY,             -- UUID
  workspace_id       TEXT NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  identifier         TEXT NOT NULL,                -- "NEWS-12" — workspace prefix + seq
  title              TEXT NOT NULL,
  body               TEXT NOT NULL DEFAULT '',
  status             TEXT NOT NULL DEFAULT 'open'
                     CHECK (status IN ('open','done','cancelled')),
                     -- 사용자 의도. 실행 상태(running/queued/failed)는 run.status에서 derive.
                     -- 자동 전이: 마지막 run이 done이면 issue도 done. 마지막 run이 failed/cancelled여도 issue는 open (사용자가 재실행 가능).
                     -- 'cancelled'는 사용자가 이슈 자체를 취소했을 때 (이슈 진행 의향 포기).
  assignee_agent_id  TEXT REFERENCES agent(id) ON DELETE SET NULL,
                     -- 기본 담당. 멘션이 들어와도 변경되지 않음.
  parent_issue_id    TEXT REFERENCES issue(id) ON DELETE SET NULL,
                     -- Phase 1: 예약 필드. UI/API 노출 X. Phase 2에서 sub-issue 기능 도입 시 사용.
  created_by         TEXT NOT NULL DEFAULT 'user'
                     CHECK (created_by IN ('user','autopilot')),
  autopilot_rule_id  TEXT REFERENCES autopilot_rule(id) ON DELETE SET NULL,
  created_at         TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at         TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE (workspace_id, identifier)
);

CREATE INDEX idx_issue_workspace_status   ON issue(workspace_id, status);
CREATE INDEX idx_issue_workspace_created  ON issue(workspace_id, created_at DESC);
-- Phase 2 sub-issue용 (현재 미사용이지만 미리 둠)
CREATE INDEX idx_issue_parent             ON issue(parent_issue_id);
CREATE INDEX idx_issue_assignee           ON issue(assignee_agent_id);
-- identifier 조회용 (URL `NEWS-12` → row)
CREATE INDEX idx_issue_workspace_identifier ON issue(workspace_id, identifier);
```

**제약 / 규칙**:
- `identifier`: 자동 생성. `<workspace.identifier_prefix>-<seq>`. seq는 workspace.next_issue_seq에서 발급.
- `assignee_agent_id`: 기본 담당. NULL 가능 → 메인 에이전트로 fallback. **멘션이 들어와도 변경되지 않는다** (멘션은 run.agent_id만 다르게 만듦).
- `parent_issue_id`: **Phase 1 예약 필드** — UI/API 노출 안 함. Phase 2에서 sub-issue 기능 도입 시 활성화.
- `created_by`: audit 용도. `'user' | 'autopilot'` 만. 멘션으로 만들어진 run의 트리거는 issue가 아니라 run의 `trigger_type`으로 기록.
- **`status`는 사용자 의도. 실행 상태는 run.status에서 derive (`execution_status`)**.
- 자동 전이: 마지막 run.status='done' → issue.status='done'. 그 외 (running/queued/failed/cancelled run) → issue.status는 'open' 유지.
- status 'cancelled'는 사용자가 이슈 자체를 취소했을 때 (DELETE 대신 보존하고 닫음). run cancel과는 다름.
- status 전이 규칙은 ARCHITECTURE.md §4.1 참조.

### 2.4 comment
```sql
CREATE TABLE comment (
  id            TEXT PRIMARY KEY,                  -- UUID
  issue_id      TEXT NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
  author_type   TEXT NOT NULL
                CHECK (author_type IN ('user','agent','system')),
  author_agent_id TEXT REFERENCES agent(id) ON DELETE SET NULL,
                     -- author_type='agent'일 때만 채움
  run_id        TEXT REFERENCES run(id) ON DELETE SET NULL,
                     -- agent/system 댓글이 특정 run에 속할 때
  content       TEXT NOT NULL,                     -- markdown
  truncated     INTEGER NOT NULL DEFAULT 0
                CHECK (truncated IN (0, 1)),
  created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_comment_issue_created ON comment(issue_id, created_at);
CREATE INDEX idx_comment_run           ON comment(run_id);
```

**제약 / 규칙**:
- `author_type='user'`: `author_agent_id = NULL`, `run_id = NULL`
- `author_type='agent'`: `author_agent_id` 필수, `run_id` 필수
- `author_type='system'`: `author_agent_id = NULL`, `run_id`는 선택 (특정 run의 시작/종료/취소 알림이면 채움)
- 표준 system 댓글:
  - `"<Agent> 실행을 시작했습니다 (run #N)"` — run_id 채움
  - `"사용자가 실행을 취소했습니다"` — run_id 채움
  - `"에이전트 @<Name>을 찾을 수 없습니다"` — run_id NULL
  - `"멘션이 둘 이상이라 @<First>만 적용됩니다"` — run_id NULL
  - `"재시작 중 진행 작업이 취소되었습니다 (orphan recovered)"` — run_id 채움
- `content`: markdown. 클라이언트에서 렌더링.
- `truncated`: 에이전트 결과 댓글이 64KB cap으로 축약됐는지 나타내는 명시 필드. 사용자 댓글 본문 문자열로 추정하지 않는다.
- 멘션은 `@AgentName` 형태로 본문에 포함됨 (별도 컬럼 없음, 백엔드가 정규식 파싱).
- 멘션 파싱: `/@([A-Za-z0-9_\-가-힣]+)/g`, 첫 매칭만 사용. 매칭은 `lower(name)` 비교.

### 2.5 run (Durable Queue의 단위)
```sql
CREATE TABLE run (
  id             TEXT PRIMARY KEY,                 -- UUID
  issue_id       TEXT NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
  agent_id       TEXT NOT NULL REFERENCES agent(id) ON DELETE RESTRICT,
  status         TEXT NOT NULL DEFAULT 'queued'
                 CHECK (status IN ('queued','running','done','failed','cancelled')),
  -- 트리거 정보 (어떤 사용 흐름이 이 run을 만들었는지)
  trigger_type   TEXT NOT NULL DEFAULT 'issue_created'
                 CHECK (trigger_type IN ('issue_created','mention','autopilot','rerun')),
  trigger_comment_id TEXT REFERENCES comment(id) ON DELETE SET NULL,
                 -- mention 트리거 시 출처 댓글
  trigger_content_snapshot TEXT NOT NULL DEFAULT '',
                 -- 트리거 시점의 본문 스냅샷 (댓글 본문 또는 issue body 일부, 4KB cap)
                 -- 댓글이 삭제되어도 추적 가능
  -- 실행 시각
  enqueued_at    TEXT NOT NULL DEFAULT (datetime('now')),  -- INSERT 시각
  claimed_at     TEXT,                             -- worker claim 시각 (status='running' 진입)
  claimed_by     TEXT NOT NULL DEFAULT '',         -- worker id (hostname+pid+goroutine#, 디버그용)
  started_at     TEXT,                             -- 실제 exec 시작 (claimed_at과 거의 동일)
  heartbeat_at   TEXT,                             -- worker alive timestamp
  process_pid    INTEGER,                          -- executor child process pid (내부 관측용)
  process_pgid   INTEGER,                          -- executor process group id (startup cleanup용)
  process_recorded_at TEXT,                        -- pid/pgid 기록 시각. 오래된 값은 startup kill skip
  finished_at    TEXT,                             -- NULL = 미종료
  exit_code      INTEGER,                          -- NULL = 미종료, -1 = user cancel, -2 = orphan
  stdout_path    TEXT,                             -- runs/<run-id>.log (백엔드 내부용, API 응답에 노출 X)
  error_message  TEXT NOT NULL DEFAULT ''
  -- 참고: session_id / work_dir 컬럼 없음.
  -- run은 stateless. cwd는 매번 workspace.working_dir 사용.
  -- per-run worktree는 Phase 2 후보.
);

-- 큐 polling 최적화 (status='queued'인 행만 인덱스)
CREATE INDEX idx_run_queue
  ON run(status, enqueued_at, id) WHERE status = 'queued';
-- 중복 dispatch 방지: 같은 issue/agent에 queued 1개만
CREATE UNIQUE INDEX idx_run_one_queued_per_issue_agent
  ON run(issue_id, agent_id) WHERE status = 'queued';
-- 같은 이슈에 동시 running 방지 (직렬화는 claim 트랜잭션으로, 인덱스는 보조)
CREATE INDEX idx_run_issue_running
  ON run(issue_id) WHERE status = 'running';
-- 같은 워크스페이스에 동시 running 방지 (워크스페이스 직렬화)
-- issue.workspace_id를 통해 JOIN으로 체크하므로 별도 인덱스 불필요 (idx_issue_workspace_status 활용)
CREATE INDEX idx_run_issue
  ON run(issue_id, enqueued_at);
CREATE INDEX idx_run_agent
  ON run(agent_id);
CREATE INDEX idx_run_trigger_comment
  ON run(trigger_comment_id) WHERE trigger_comment_id IS NOT NULL;
```

**제약 / 규칙**:
- `agent_id`: ON DELETE RESTRICT — 실행 이력은 에이전트 삭제 차단
  - 운영 정책: 에이전트 삭제 시 사용자에게 "관련 run이 N개 있습니다" 확인
- `stdout_path`: 백엔드 내부 경로. **API 응답에는 노출하지 않음**. 대신 `GET /api/runs/:id/log` 다운로드 URL만 제공.
- 단일 run 최대 10MB (executor가 cap). cap 도달 후에도 stdout pipe는 io.Discard로 계속 drain (child process pipe blocking 방지).
- `trigger_type='mention'` 이면 `trigger_comment_id` 채움. 그 외는 NULL 허용.
- 현재 체이닝 정책은 **explicit-only**다. `mention`은 사용자 댓글의 `@AgentName` 명시 멘션만 의미하며, agent 결과 댓글의 mention은 자동 dispatch하지 않는다.
- 현재 `run` 테이블에는 `chain_id`, `parent_run_id`, `chain_depth` 컬럼이 없다. 이 컬럼들은 auto-chain opt-in 후보 schema이며 미구현이다.
- `trigger_comment_id`가 `status IN ('queued','running')` run에 연결된 동안 해당 댓글 사용자 삭제 차단 (409).
- **Worker claim 쿼리** (durable queue + 워크스페이스 직렬화):
  ```sql
  BEGIN IMMEDIATE;
    SELECT r.id, r.issue_id FROM run r
     JOIN issue i ON i.id = r.issue_id
     WHERE r.status = 'queued'
       -- 같은 이슈에 이미 running 없음
       AND NOT EXISTS (
         SELECT 1 FROM run r2
          WHERE r2.issue_id = r.issue_id AND r2.status = 'running'
       )
       -- 같은 워크스페이스에 이미 running 없음 (MVP 직렬화)
       AND NOT EXISTS (
         SELECT 1 FROM run r3
          JOIN issue i3 ON i3.id = r3.issue_id
          WHERE i3.workspace_id = i.workspace_id AND r3.status = 'running'
       )
     ORDER BY r.enqueued_at ASC, r.id ASC
     LIMIT 1;
    -- 위에서 id를 얻으면:
    UPDATE run
       SET status = 'running',
           claimed_at = ?,        -- RFC3339 now
           claimed_by = ?,        -- worker id
           started_at = ?
     WHERE id = ? AND status = 'queued';
    -- affected = 1 이면 claim 성공, 0이면 다른 워커가 먼저 잡음
    -- 같은 트랜잭션 안에서 system 댓글 INSERT만 (issue.status는 변경 X — derived)
  COMMIT;
  ```
- **부팅 시 orphan 정리**:
  - 먼저 `process_pgid > 1`이고 `process_recorded_at`이 최근인 running run의 process group을 best-effort SIGTERM/SIGKILL한다.
  - `process_recorded_at`이 없거나 너무 오래된 값이면 OS PGID 재사용 가능성을 피하기 위해 kill하지 않고 skip한다.
  ```sql
  WITH orphan AS (
    UPDATE run SET status='cancelled', exit_code=-2, finished_at=?,
                   error_message='orphan recovered'
     WHERE status='running' AND finished_at IS NULL
     RETURNING id, issue_id
  )
  INSERT INTO comment (id, issue_id, author_type, run_id, content, created_at)
    SELECT ?, issue_id, 'system', id, '재시작 중 진행 작업이 취소되었습니다 (orphan recovered)', ?
      FROM orphan;
  ```
  - **issue.status는 건드리지 않음** — 이슈 의도와 무관하므로 사용자가 결정.
  - MVP는 자동 재개하지 않음. 사용자가 보드에서 [재실행] 클릭 시 새 run INSERT.
- **신규 dispatch** = 새 run row INSERT (status='queued'). 기존 run을 재사용하지 않음.
- **[재실행]의 대상 agent**: 가장 최근 run의 agent_id (issue.assignee_agent_id가 아님). 사용자가 마지막 결과 보고 [재실행] 누르는 흐름이 자연스러움.
- **중복 dispatch 차단**: `idx_run_one_queued_per_issue_agent` partial unique index. 같은 (issue, agent)에 queued 1개 초과 INSERT 시 SQLite unique 위반 → 409 `STATE_ERROR` 응답.

### 2.6 autopilot_rule
```sql
CREATE TABLE autopilot_rule (
  id                    TEXT PRIMARY KEY,
  workspace_id          TEXT NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  name                  TEXT NOT NULL,             -- "매일 9시 뉴스"
  cron_expr             TEXT NOT NULL,             -- "0 9 * * *"
  issue_title_template  TEXT NOT NULL,             -- "{{date}} AI 뉴스"
  issue_body_template   TEXT NOT NULL DEFAULT '',
  assignee_agent_id     TEXT REFERENCES agent(id) ON DELETE SET NULL,
                        -- NULL = 메인 에이전트
  enabled               INTEGER NOT NULL DEFAULT 1,
  last_run_at           TEXT,
  next_run_at           TEXT,
  created_at            TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at            TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_autopilot_workspace ON autopilot_rule(workspace_id);
CREATE INDEX idx_autopilot_enabled_next
  ON autopilot_rule(enabled, next_run_at) WHERE enabled = 1;
```

**제약 / 규칙**:
- `cron_expr`: 표준 5필드. 백엔드에서 robfig/cron 파싱 검증.
- `next_run_at`: 룰 생성/수정 시 cron 평가로 계산해서 저장 (시스템 timezone 기준).
- `enabled` 변경 시 scheduler 전체 reload (단순함 우선).
- 룰 실행이 issue 생성 실패하면 룰은 살아있고 다음 tick에 재시도.
- **시간대**:
  - 시스템 전역: 환경변수 `CORN_AGENT_DASHBOARD_TIMEZONE` (기본 `Asia/Seoul`)
  - 룰별 timezone 컬럼은 두지 않음 — 시스템 전역으로 단순화
  - 부팅 시 robfig/cron을 `cron.WithLocation(loc)`로 생성
- **꺼져 있는 동안의 누락**: 실행하지 않음. 부팅 시 `next_run_at`을 `cron.Next(now)`로 재계산.

### 2.7 schema_migrations (메타)
```sql
CREATE TABLE schema_migrations (
  version    INTEGER PRIMARY KEY,
  name       TEXT NOT NULL,
  applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```

부팅 시 `internal/db/migrations/NNNN_*.sql` 파일 순서대로 적용, 적용된 것만 INSERT.

---

## 3. ER 다이어그램

```
workspace 1 ─── N  agent
    │ 1
    │
    ├── N  issue ─── N  comment
    │        │ 1         │
    │        │           │
    │        ├── N  run ─┘  (comment.run_id)
    │        │
    │        └── self-ref (parent_issue_id)
    │
    └── N  autopilot_rule ─── N  issue (autopilot_rule_id)

agent 1 ─── N  comment (author_agent_id)
agent 1 ─── N  run (agent_id)
agent 1 ─── N  issue (assignee_agent_id)
agent 1 ─── N  autopilot_rule (assignee_agent_id)
```

---

## 4. 트랜잭션 패턴

### 4.1 워크스페이스 + 메인 에이전트 생성
```
BEGIN;
  INSERT INTO workspace (..., identifier_prefix, next_issue_seq=1);
  INSERT INTO agent (workspace_id, name, runtime, instructions, is_main=1);
COMMIT;
```

### 4.2 이슈 생성 + identifier 발급 + 첫 run 큐잉
```
BEGIN;
  UPDATE workspace SET next_issue_seq = next_issue_seq + 1
    WHERE id = ? RETURNING next_issue_seq;
  -- seq를 받은 후 (실제 발급 번호 = next_issue_seq - 1)
  INSERT INTO issue (..., identifier = prefix || '-' || (seq-1), status='open');
  INSERT INTO run (issue_id, agent_id, status='queued',
                   trigger_type='issue_created',
                   trigger_content_snapshot=substr(issue.body, 0, 4000),
                   enqueued_at=now);
COMMIT;
-- worker가 1초 polling으로 claim
```

### 4.3 Worker claim (running 진입)
```
BEGIN IMMEDIATE;
  -- 1) 다음 run 선택 (issue + workspace 직렬화)
  SELECT r.id, r.issue_id FROM run r
   JOIN issue i ON i.id = r.issue_id
   WHERE r.status='queued'
     AND NOT EXISTS (SELECT 1 FROM run r2 WHERE r2.issue_id=r.issue_id AND r2.status='running')
     AND NOT EXISTS (
       SELECT 1 FROM run r3 JOIN issue i3 ON i3.id=r3.issue_id
        WHERE i3.workspace_id=i.workspace_id AND r3.status='running'
     )
   ORDER BY r.enqueued_at ASC, r.id ASC
   LIMIT 1;
  -- 2) UPDATE는 status='queued' 가드로 race 방지
  UPDATE run SET status='running', claimed_at=now, claimed_by=?, started_at=now
   WHERE id=? AND status='queued';
  -- affected=1 확인. 0이면 다른 워커가 먼저 잡음 → 다음 polling.
  -- issue.status는 변경하지 않음 (execution_status는 derived)
  INSERT INTO comment (issue_id, author_type='system', run_id=?, content='<Agent> 실행을 시작했습니다 (run #N)');
COMMIT;
```

### 4.4 Agent 실행 종료
```
BEGIN;
  UPDATE run SET status=?, finished_at=now, exit_code=?, stdout_path=?, error_message=? WHERE id=?;
  -- status는 exit_code 따라: 0='done', !=0='failed', user cancel='cancelled'
  INSERT INTO comment (issue_id, author_type='agent', author_agent_id=?, run_id=?, content=?);
  -- comment.content는 64KB cap. 초과 시 앞 60KB + "전체 로그는 [로그 보기](...)"
  -- 만약 run.status='done':
  UPDATE issue SET status='done', updated_at=now WHERE id=?;
  -- 'failed'/'cancelled' run은 issue.status 그대로 (사용자가 [재실행]으로 재시도 가능)
COMMIT;
```

### 4.5 사용자 cancel (대기/진행 중 run 취소)
```
1. HTTP 핸들러: cancellable_run =
   SELECT id FROM run
    WHERE issue_id=? AND status IN ('running','queued')
    ORDER BY CASE status WHEN 'running' THEN 0 ELSE 1 END, enqueued_at ASC
    LIMIT 1;
2. 상태와 무관하게 먼저 worker pool에 cancellation intent를 기록한다.
   - 이미 실행 중이면 in-memory map[run_id]context.CancelFunc로 process group SIGTERM
   - 아직 cancel func가 없으면 pending-cancel set에 저장해 queued→running 경계 race를 보존
3. HTTP fallback이 DB에서 run.status='cancelled', exit_code=-1, error_message='user cancelled'로 전환한다.
4. fallback 결과 run이 아직 claimed/started 전이면 pending-cancel을 정리한다. claimed/started 흔적이 있으면 worker 등록 직후 적용되도록 pending을 유지한다.
5. worker의 종료 처리도 동일한 terminal 상태를 확인하며 이미 cancelled면 덮어쓰지 않는다.
6. issue.status는 그대로 'open' 유지 (사용자가 이슈를 닫은 게 아니라 한 번의 실행만 취소함)
```

### 4.5b 사용자가 이슈 자체를 취소 (보존하고 닫음)
```
1. HTTP: PUT /api/issues/:id  body={ status: 'cancelled' }
   - 단, 진행 중 run 있으면 409. 먼저 4.5 cancel 후 진행.
BEGIN;
  UPDATE issue SET status='cancelled', updated_at=now WHERE id=?;
  -- queued run이 있으면 자동으로 cancel:
  UPDATE run SET status='cancelled', exit_code=-1, finished_at=now,
                 error_message='issue cancelled'
   WHERE issue_id=? AND status='queued';
  INSERT INTO comment (issue_id, author_type='system', content='이슈가 취소되었습니다');
COMMIT;
```

### 4.6 멘션으로 위임 (같은 이슈 새 run, **assignee는 변경 X**)
```
BEGIN;
  INSERT INTO comment (id=C, issue_id=I, author_type='user', content=?);
  -- 멘션 파싱 결과:
  --   매칭 실패: INSERT system 댓글 "에이전트 @Foo 없음"
  --   매칭 둘 이상: 첫 매칭만 사용 + INSERT system 댓글 "추가 멘션 무시"
  --   매칭 1개:
  INSERT INTO run (issue_id=I, agent_id=matched, status='queued',
                   trigger_type='mention',
                   trigger_comment_id=C,
                   trigger_content_snapshot=substr(comment.content, 0, 4000),
                   enqueued_at=now);
  -- issue.assignee_agent_id는 변경하지 않음 (멘션은 일회성 위임)
  -- 중복 dispatch (같은 이슈/agent에 이미 queued)는 INSERT 단계에서 unique 위반 → 409
COMMIT;
-- worker가 polling으로 claim
```

### 4.7 부팅 시 orphan 정리
```sql
BEGIN;
WITH orphan AS (
  UPDATE run SET status='cancelled', exit_code=-2, finished_at=?,
                 error_message='orphan recovered'
   WHERE status='running' AND finished_at IS NULL
   RETURNING id, issue_id
)
INSERT INTO comment (id, issue_id, author_type, run_id, content, created_at)
  SELECT ?, issue_id, 'system', id, '재시작 중 진행 작업이 취소되었습니다 (orphan recovered)', ?
    FROM orphan;
-- issue.status는 건드리지 않음
COMMIT;
```

### 4.8 [재실행] (마지막 run의 agent로)
```
BEGIN;
  -- 마지막 run의 agent를 찾아서 새 run INSERT
  -- 단, 같은 (issue_id, agent_id, status='queued')가 이미 있으면 unique 위반 → 409
  SELECT agent_id FROM run WHERE issue_id=? ORDER BY enqueued_at DESC LIMIT 1;
  INSERT INTO run (issue_id, agent_id=마지막, status='queued',
                   trigger_type='rerun',
                   trigger_content_snapshot='[rerun of run <id>]',
                   enqueued_at=now);
  -- issue.status가 'done'이었다면 'open'으로 되돌리기
  UPDATE issue SET status='open', updated_at=now WHERE id=? AND status='done';
COMMIT;
```

---

## 5. 인덱스 사용 패턴

| 쿼리 | 사용 인덱스 |
|---|---|
| 워크스페이스 보드 (issue.status 필터) | `idx_issue_workspace_status` |
| 워크스페이스 보드 (최신순) | `idx_issue_workspace_created` |
| identifier로 이슈 찾기 (URL → row) | `idx_issue_workspace_identifier` |
| 이슈 상세 댓글 스레드 | `idx_comment_issue_created` |
| 이슈 상세 run 이력 | `idx_run_issue` |
| **Worker queue polling (claim)** | `idx_run_queue` (partial on status='queued') |
| **per-issue 직렬화 체크** | `idx_run_issue_running` (partial on status='running') |
| **중복 dispatch 차단** | `idx_run_one_queued_per_issue_agent` (UNIQUE partial) |
| trigger comment 역참조 | `idx_run_trigger_comment` |
| 에이전트 멘션 매칭 (case-insensitive) | `idx_agent_name_ci` |
| Sub-issue 조회 (Phase 2) | `idx_issue_parent` |
| 에이전트별 이슈 | `idx_issue_assignee` |

### 5.1 execution_status 계산 (issue별 derived field)

API 응답에서만 계산. DB 컬럼 없음.

```sql
-- 한 번에 issue + execution_status 계산 (LEFT JOIN 최근 run)
SELECT i.*,
  COALESCE(
    (SELECT status FROM run WHERE issue_id=i.id AND status='running' LIMIT 1),
    (SELECT status FROM run WHERE issue_id=i.id AND status='queued' ORDER BY enqueued_at LIMIT 1),
    (SELECT status FROM run WHERE issue_id=i.id ORDER BY enqueued_at DESC LIMIT 1),
    'idle'
  ) AS execution_status,
  (SELECT agent_id FROM run WHERE issue_id=i.id ORDER BY enqueued_at DESC LIMIT 1) AS last_run_agent_id
FROM issue i
WHERE i.workspace_id = ?;
```

execution_status 도메인: `running | queued | done | failed | cancelled | idle`
- `idle`: run이 한 번도 없는 경우 (예외적, 거의 발생 안 함)

---

## 6. 데이터 무결성 체크리스트

**Phase 1 부팅 시 필수**:
- [x] orphan run 정리 (§4.7)
- [x] 마이그레이션 적용 (idempotent)

**Phase 6 부팅 시 자가검진**:
- [x] 워크스페이스마다 메인 에이전트 정확히 1개 (`startup self-check`에서 검증)
- [ ] `issue.status='running'` 이슈는 진행 중 run (status='running') 보유
- [ ] `comment.author_type='agent'` → `author_agent_id IS NOT NULL AND run_id IS NOT NULL`
- [ ] run의 stdout_path 파일 존재 확인 (없으면 error_message에 "log file missing")

---

## 7. 초기 데이터 (seed)

`corn-agent-dashboard init` 실행 시:
- 빈 DB + 마이그레이션 적용만. 워크스페이스/에이전트는 사용자가 UI에서 생성.

선택적 `--seed example`:
- "playground" 워크스페이스 + 메인 에이전트 1개 (codex, 기본 instructions)

---

## 8. 마이그레이션 파일 (Phase 1 초기)

```
internal/db/migrations/
├── 0001_init.sql           # workspace, agent, issue, comment, run, autopilot_rule + schema_migrations
└── 0002_indexes.sql        # 모든 인덱스 (partial 포함)
```

이후 변경은 `0003_*.sql` 등으로 누적. forward-only.

### 0001_init.sql 핵심 포함 사항 체크리스트
- [ ] `workspace.working_dir` 컬럼
- [ ] `agent` 테이블에 `(workspace_id, lower(name))` UNIQUE INDEX
- [ ] `issue.status` CHECK 제약: **`open | done | cancelled` 3개 값만**
- [ ] `issue.created_by` CHECK: `user | autopilot` (mention 제외)
- [ ] `issue.parent_issue_id` 컬럼 (Phase 2용 예약)
- [ ] `idx_issue_workspace_identifier` 인덱스 (URL identifier 조회)
- [ ] `run.status` CHECK: `queued | running | done | failed | cancelled`
- [ ] `run.trigger_type` CHECK: `issue_created | mention | autopilot | rerun`
- [ ] `run.trigger_comment_id / trigger_content_snapshot` 컬럼
- [ ] `run.claimed_at / claimed_by / enqueued_at / started_at / finished_at` 컬럼
- [ ] `run` partial 인덱스 3개: queue / per-issue running / **중복 queued 차단(UNIQUE)**
- [ ] `comment.author_type` CHECK: `user | agent | system`

---

## 9. 백업 / 복구

### 9.1 백업
```bash
# 1. data.db 백업 (SQLite online backup)
corn-agent-dashboard backup --to ~/backup/data.db

# 2. runs/ 디렉토리 복사
cp -r ~/.corn-agent-dashboard/runs ~/backup/runs

# 3. config.toml (있다면)
cp ~/.corn-agent-dashboard/config.toml ~/backup/
```

### 9.2 복구
- `corn-agent-dashboard restore --from ~/backup/data.db` 실행 후 재시작
- 마이그레이션 자동 적용으로 스키마 호환

---

## 10. 향후 확장 (Phase 2+)

테이블 추가 후보:
- `attachment` (issue_id, file_path, mime_type)
- `webhook` (workspace_id, url, event, secret)
- `agent_token` (외부 봇 인증용)
- `audit_log` (관리자 감사용 — 단일 사용자엔 비필수)

지금은 추가하지 않음.


> 정책: `agent.model`은 사용자 선택값입니다. 빈 문자열은 runtime/CLI 기본 모델을 의미하며, 값이 있으면 해당 모델 ID를 adapter에 전달합니다.

---

## 7. 2026-05-14 운영 관측성 확장

### 7.1 run lifecycle 필드

`run` 테이블은 실행 복구와 UI 진단을 위해 아래 컬럼을 추가로 가진다.

| 컬럼 | 타입 | 의미 |
|---|---|---|
| `heartbeat_at` | TEXT | worker가 주기적으로 갱신하는 alive timestamp |
| `process_pid` | INTEGER | executor child process pid. API에는 노출하지 않음 |
| `process_pgid` | INTEGER | executor process group id. startup orphan process cleanup에 사용 |
| `process_recorded_at` | TEXT | pid/pgid 기록 시각. 오래된 metadata는 process kill skip |
| `terminal_reason` | TEXT | 완료/실패/취소/복구의 구조화된 최종 원인 |
| `failure_kind` | TEXT | 실패 run의 기술 분류 |
| `cancel_reason` | TEXT | 취소 run의 원인 분류 |

`status`는 여전히 `queued/running/done/failed/cancelled`만 표현하고, 위 reason 필드는 왜 terminal 상태가 되었는지 보조 설명한다.

### 7.2 run_event

`run_event`는 run별 기술 audit trail이다.

```sql
CREATE TABLE run_event (
  id          TEXT PRIMARY KEY,
  run_id      TEXT NOT NULL REFERENCES run(id) ON DELETE CASCADE,
  issue_id    TEXT NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
  seq         INTEGER NOT NULL,
  event_type  TEXT NOT NULL,
  severity    TEXT NOT NULL DEFAULT 'info',
  message     TEXT NOT NULL DEFAULT '',
  detail_json TEXT NOT NULL DEFAULT '{}',
  created_at  TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE(run_id, seq)
);
```

- `seq`는 run 내부 순번이다.
- `detail_json`에는 token, env, stdout path, prompt 전문 등 민감 정보를 넣지 않는다.
- 사용자에게 보여주는 comment와 분리한다. comment는 대화/결과, run_event는 실행 기술 로그다.

### 7.3 autopilot_rule failure 필드

`autopilot_rule`은 마지막 실패 및 마지막 생성 이슈 추적을 위해 `last_error`, `consecutive_failures`, `last_triggered_issue_id`를 가진다.

---

## 11. Auto-chain opt-in 후보 데이터 모델 (미구현)

현재 제품은 **사용자 댓글의 명시 멘션만 dispatch**한다. agent 결과 댓글 안의 `@AgentName`은 자동 실행하지 않는다.

Auto-chain을 Phase 2+에서 opt-in으로 구현하기로 결정할 경우의 후보 schema는 아래와 같다. 이 섹션은 설계 초안이며 현재 마이그레이션이 아니다.

### 11.1 run lineage 후보

```sql
ALTER TABLE run ADD COLUMN chain_id TEXT NOT NULL DEFAULT '';
ALTER TABLE run ADD COLUMN parent_run_id TEXT REFERENCES run(id) ON DELETE SET NULL;
ALTER TABLE run ADD COLUMN chain_depth INTEGER NOT NULL DEFAULT 0;

CREATE INDEX idx_run_chain ON run(chain_id, chain_depth, enqueued_at);
CREATE INDEX idx_run_parent ON run(parent_run_id);
```

| 후보 컬럼 | 의미 |
|---|---|
| `chain_id` | 같은 자동 chain에 속한 run 묶음. root run id 또는 별도 UUID 후보 |
| `parent_run_id` | agent 결과 comment를 만든 source run |
| `chain_depth` | root explicit run은 0, auto-chain으로 만든 run은 parent + 1 |

### 11.2 opt-in 설정 후보

workspace 단위:

```sql
ALTER TABLE workspace ADD COLUMN auto_chain_enabled INTEGER NOT NULL DEFAULT 0 CHECK (auto_chain_enabled IN (0,1));
ALTER TABLE workspace ADD COLUMN auto_chain_max_depth INTEGER NOT NULL DEFAULT 5;
```

issue 단위:

```sql
ALTER TABLE issue ADD COLUMN auto_chain_enabled INTEGER NOT NULL DEFAULT 0 CHECK (auto_chain_enabled IN (0,1));
```

둘 중 하나를 선택해야 하며, 기본값은 반드시 off다.

### 11.3 후보 제약

- max depth 기본값은 5를 권장한다.
- 같은 `chain_id` 안에서 동일 agent 재호출은 차단하는 것을 권장한다.
- source run이 `failed` 또는 `cancelled`이면 chain을 중단하는 것을 권장한다.
- 후보 `trigger_type`은 `agent_mention` 또는 `auto_mention` 중 하나를 선택한다. 현재 enum에는 둘 다 없다.
- agent 결과에 여러 mention이 있으면 첫 번째만 실행하거나 별도 설정을 둔다.
