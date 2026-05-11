# API — corn-agent-dashboard

> REST API 명세
> Version: 0.1
> Date: 2026-05-11
> Base URL: `http://127.0.0.1:8080/api`

---

## 0. 공통 규약

### 0.1 Content-Type
- 모든 요청/응답: `application/json; charset=utf-8`

### 0.2 인증
- 기본: 인증 없음 (localhost)
- 토큰 모드: 환경변수 `CORN_AGENT_DASHBOARD_TOKEN` 또는 `--token` 플래그
  - 요청 헤더: `Authorization: Bearer <CORN_AGENT_DASHBOARD_TOKEN>`
- 누락 시 401 (토큰 모드일 때만)
- `--bind 0.0.0.0` 으로 외부 노출 시 토큰 필수 강제 (미설정이면 부팅 거부)

### 0.3 에러 응답 형식
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "워크스페이스 슬러그는 영숫자와 하이픈만 허용됩니다",
    "details": { "field": "slug" }
  }
}
```

**에러 코드**:
| 코드 | HTTP | 의미 |
|---|---|---|
| `VALIDATION_ERROR` | 400 | 입력 검증 실패 |
| `UNAUTHORIZED` | 401 | 토큰 누락/잘못됨 |
| `FORBIDDEN` | 403 | 작업 권한 없음 (예: agent/system 댓글 삭제 시도) |
| `NOT_FOUND` | 404 | 리소스 없음 |
| `CONFLICT` | 409 | 유일성 위반 (slug 중복 등) |
| `STATE_ERROR` | 409 | 현재 상태에서 불가능한 작업 (예: queued 이슈에 cancel) |
| `INTERNAL_ERROR` | 500 | 시스템 오류 |

### 0.4 시간 형식
- 모든 timestamp: RFC 3339 UTC (예: `2026-05-11T09:00:00Z`)

### 0.5 페이지네이션
- 쿼리 파라미터: `limit` (default 50, max 200), `cursor` (불투명 문자열)
- 응답에 `next_cursor` 포함 (없으면 null)

### 0.6 ID 형식
- 모든 ID: UUID v4

### 0.7 워크스페이스 / 이슈 식별 (다형 라우팅)

**워크스페이스**: 경로의 `:workspace` 자리에 **UUID 또는 slug** 모두 허용
- UUID 정규식 (`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-...$`) 매칭이면 id, 아니면 slug 조회

**이슈**: 경로의 `:issue` 자리에 **UUID 또는 identifier** (예: `NEWS-12`) 모두 허용
- identifier 정규식 (`^[A-Z]+-\d+$`) 매칭이면 identifier, 아니면 UUID 조회
- nested 경로에서는 워크스페이스 컨텍스트 안에서 identifier 조회 (`WHERE workspace_id=? AND identifier=?`)

프론트엔드는 URL에 slug + identifier 사용. API client는 응답 받은 UUID 사용. 둘 다 동작.

### 0.8 응답 envelope
- 성공 응답: 리소스 종류별 키로 감싼다 (`{ "workspace": {...} }`, `{ "issues": [...] }`)
- 단일 객체 변경(PUT/POST/DELETE/promote/rerun/cancel/trigger): 해당 리소스 또는 새 리소스를 반환
- DELETE 성공 시: `{ "deleted": true, "id": "uuid" }` (204 대신 200)

---

## 1. Workspaces

### 1.1 `GET /api/workspaces`
워크스페이스 목록.

**Response 200**:
```json
{
  "workspaces": [
    {
      "id": "uuid",
      "name": "AI 뉴스 큐레이션",
      "slug": "ai-news",
      "description": "Reddit 핫이슈 정리",
      "identifier_prefix": "NEWS",
      "agent_count": 4,
      "open_issue_count": 3,
      "created_at": "2026-05-11T00:00:00Z"
    }
  ]
}
```

### 1.2 `POST /api/workspaces`
워크스페이스 생성 + 메인 에이전트 1개 동시 생성 (트랜잭션).

**Request**:
```json
{
  "name": "AI 뉴스 큐레이션",
  "slug": "ai-news",
  "description": "Reddit 핫이슈 정리",
  "identifier_prefix": "NEWS",
  "working_dir": "",
  "output_dir": "",
  "main_agent": {
    "name": "NewsLead",
    "runtime": "codex",
    "model": "",
    "instructions": "Reddit r/MachineLearning에서..."
  }
}
```

**Response 201**:
```json
{
  "workspace": { "id": "...", ... },
  "main_agent": { "id": "...", ... }
}
```

**Errors**:
- 400: slug 형식 위반, name 빈값, instructions 빈값
- 409: slug 중복

### 1.3 `GET /api/workspaces/:id`
단일 워크스페이스 상세.

**Response 200**: `{ "workspace": {...} }`

### 1.4 `PUT /api/workspaces/:id`
워크스페이스 수정 (name/description/working_dir/output_dir만).
`slug`와 `identifier_prefix`는 변경 불가 (이슈 ID 일관성).

**Request**: `{ "name": "...", "description": "...", "working_dir": "...", "output_dir": "..." }`

### 1.5 `DELETE /api/workspaces/:id`
워크스페이스 + 산하 모든 데이터 삭제 (CASCADE).
- 확인 절차는 UI 책임.
- 진행 중 run이 있으면 409 `STATE_ERROR`.

---

## 2. Agents

### 2.1 `GET /api/workspaces/:id/agents`
워크스페이스 에이전트 목록.

**Response 200**:
```json
{
  "agents": [
    {
      "id": "uuid",
      "workspace_id": "uuid",
      "name": "NewsLead",
      "runtime": "codex",
      "model": "",
      "instructions": "...",
      "is_main": true,
      "created_at": "2026-05-11T00:00:00Z"
    }
  ]
}
```

### 2.2 `POST /api/workspaces/:id/agents`
에이전트 추가 (추가 에이전트, `is_main`은 자동 false).

**Request**:
```json
{
  "name": "Writer",
  "runtime": "codex",
  "model": "",
  "instructions": "..."
}
```

**Errors**:
- 400: name 중복(워크스페이스 내, case-insensitive — `Writer`와 `writer`는 같음), 빈 instructions
- 400: `is_main=true`로 보내면 거부 (메인은 워크스페이스 생성 시에만 자동, 변경은 `/promote`)
- 409 CONFLICT: 같은 이름 다른 대소문자 이미 존재

### 2.3 `GET /api/agents/:id`
에이전트 상세.

### 2.4 `PUT /api/agents/:id`
에이전트 수정 (`name`, `runtime`, `model`, `instructions`).

**Request**: 위 필드 일부 또는 전체
- `is_main` 변경 X (별도 엔드포인트)

### 2.5 `POST /api/agents/:id/promote`
이 에이전트를 메인으로 승격. 기존 메인은 자동 강등.

**Errors**:
- 409: 이미 메인

### 2.6 `DELETE /api/agents/:id`
- 메인 에이전트는 삭제 불가 (409)
- 관련 run이 있으면 사용자 확인 필요 → 응답에 `run_count` 포함된 409
- `?force=true`로 강제 삭제 시 run.agent_id는 SET NULL이 아니라 차단 (RESTRICT) → 운영상 안전을 위해 force도 막음. 대신 archive 플래그(Phase 2) 후보.

---

## 3. Issues

### 3.1 `GET /api/workspaces/:workspace/issues`
이슈 목록 (보드용).

**Query**:
- `status=open,done,cancelled` (이슈 의도 필터, 콤마 OR)
- `execution=queued,running,done,failed,cancelled,idle` (실행 상태 derived 필터, 콤마 OR)
- `assignee=<agent_id>` (기본 담당 필터)
- `q=<text>` (제목/본문 LIKE 검색)
- `limit`, `cursor`
- (sub-issue 필터 `parent`는 Phase 2)

**Response 200**:
```json
{
  "issues": [
    {
      "id": "uuid",
      "identifier": "NEWS-12",
      "title": "오늘 뉴스 정리",
      "body": "...",
      "status": "open",
      "execution_status": "running",
      "assignee_agent_id": "uuid",
      "assignee_agent_name": "NewsLead",
      "last_run_agent_id": "uuid",
      "last_run_agent_name": "Writer",
      "created_by": "user",
      "comment_count": 3,
      "created_at": "...",
      "updated_at": "..."
    }
  ],
  "next_cursor": null
}
```

**필드**:
- `status`: 사용자 의도 (`open | done | cancelled`)
- `execution_status`: derived from runs (`running | queued | done | failed | cancelled | idle`). DATA_MODEL §5.1 계산식
- `assignee_agent_*`: 기본 담당 (멘션으로 바뀌지 않음)
- `last_run_agent_*`: 가장 최근 run의 agent (멘션에 의해 바뀐 실제 마지막 실행자, [재실행] 대상)
- `parent_issue_id` 필드는 응답에서 제외 (Phase 2)

### 3.2 `POST /api/workspaces/:workspace/issues`
이슈 생성 + 즉시 dispatch (run row INSERT, status='queued').

**Request**:
```json
{
  "title": "오늘 뉴스 정리해줘",
  "body": "Reddit r/MachineLearning 상위 5개",
  "assignee_agent_id": "uuid"
}
```

- `assignee_agent_id` 누락 시 메인 에이전트 자동 할당.
- `parent_issue_id` 필드는 받지 않음 (Phase 2).

**Response 201**: 이슈 객체 (status='open', execution_status='queued')
- 생성과 동시에 run row INSERT (trigger_type='issue_created').

**Errors**:
- 400 VALIDATION_ERROR: title 빈값
- 404 NOT_FOUND: assignee_agent_id 없음

### 3.3 `GET /api/workspaces/:workspace/issues/:issue`
이슈 상세 (댓글/run은 별도 endpoint).
경로의 `:issue`는 **UUID 또는 identifier** (예: `NEWS-12`).

`GET /api/issues/:id` (workspace 없이 UUID 직접)도 동시 지원.

### 3.4 `PUT /api/issues/:id`
이슈 수정.

**Request 필드 (모두 선택)**:
- `title`, `body`, `assignee_agent_id`
- `status`: `done` 또는 `cancelled` 만 허용 (사용자 명시적 닫기/취소)
  - `done` → 명시적으로 이슈 완료 처리. 진행 중 run 있으면 409.
  - `cancelled` → 이슈 취소 + queued run 자동 cancel.
  - `open`으로 되돌리기는 [재실행] (3.5)을 사용.

### 3.5 `POST /api/issues/:id/rerun`
**가장 최근 run의 agent**로 새 run dispatch.

**Request (선택)**:
```json
{ "agent_id": "uuid" }
```
- `agent_id` 명시: 그 agent로 dispatch (마지막 run의 agent와 다를 수 있음)
- `agent_id` 누락: 마지막 run의 agent 자동 선택 (기본 동작)

**동작**:
- 새 run row 생성 (`trigger_type='rerun'`, `trigger_content_snapshot='[rerun of run <id>]'`)
- issue.status='open'으로 되돌림 (이전이 'done'이었어도)
- 워커가 polling으로 claim

**Errors**:
- 409 STATE_ERROR: 현재 `execution_status`가 `running` 또는 `queued`
- 409 CONFLICT: 같은 (issue, agent)에 이미 queued run 존재 (`idx_run_one_queued_per_issue_agent` 위반)

### 3.6 `POST /api/issues/:id/cancel`
**진행 중 run 한 개만** 취소. 이슈 자체를 닫는 게 아님.

**동작**:
- worker가 cmd.Process를 SIGTERM (child process group)
- 30초 후에도 살아있으면 SIGKILL
- run.status='cancelled', exit_code=-1, error_message='user cancelled'
- **issue.status는 'open' 유지** (이슈는 살아있고 [재실행] 가능)
- system 댓글 "사용자가 실행을 취소했습니다" INSERT

이슈 자체를 취소하려면 `PUT /api/issues/:id { status: 'cancelled' }` (3.4) 사용.

**Errors**:
- 409 STATE_ERROR: `execution_status`가 `running`이 아님

### 3.7 `DELETE /api/issues/:id`
이슈 + 댓글 + run 모두 삭제 (CASCADE).
- run.stdout_path 파일도 삭제 (best-effort, 실패해도 DB row는 지움)
- 진행 중이면 먼저 cancel 요구 (409 STATE_ERROR)
- 응답: `{ "deleted": true, "id": "..." }`

---

## 4. Comments

### 4.1 `GET /api/issues/:id/comments`
이슈의 댓글 스레드 (시간순).

**Response 200**:
```json
{
  "comments": [
    {
      "id": "uuid",
      "issue_id": "uuid",
      "author_type": "user",
      "author_agent_id": null,
      "author_agent_name": null,
      "run_id": null,
      "content": "@Writer 이걸로 블로그 글 써줘",
      "truncated": false,
      "log_url": null,
      "created_at": "..."
    },
    {
      "id": "uuid",
      "issue_id": "uuid",
      "author_type": "agent",
      "author_agent_id": "uuid",
      "author_agent_name": "Writer",
      "run_id": "uuid",
      "content": "# 블로그 글\n\n... (앞 60KB)",
      "truncated": true,
      "log_url": "/api/runs/<run_id>/log",
      "created_at": "..."
    }
  ]
}
```

**Comment 표시 cap (보안/성능)**:
- `comment.content` 최대 64KB (UTF-8 기준)
- 에이전트 결과가 초과하면 앞 60KB만 저장 + 본문 끝에 `\n\n---\n전체 로그(<size> KB)는 [로그 보기](<log_url>)에서 확인하세요.\n` append
- `truncated=true` 플래그로 클라이언트가 표시
- **markdown 렌더링은 `react-markdown` + `remark-gfm`만**. `rehype-raw` 금지 (HTML/script 인젝션 차단).

### 4.2 `POST /api/issues/:id/comments`
사용자 댓글 추가. 본문에 `@AgentName` 멘션이 있으면 자동 dispatch.

**Request**:
```json
{
  "content": "@Writer 다듬어줘"
}
```

**Response 201**:
```json
{
  "comment": { ... },
  "mention_warnings": ["..."],
  "dispatched_run": { "id": "uuid", "agent_id": "uuid", "agent_name": "Writer" }
}
```

**멘션 처리 규칙 (MVP)**:
- 정규식 `@([A-Za-z0-9_\-가-힣]+)` 으로 본문 스캔
- 매칭 0개: 단순 댓글 INSERT 만, dispatch 없음
- 매칭 1개: 워크스페이스 내 agent 매칭(`lower(name)` 일치)
  - 매칭 성공: 새 run row INSERT (`agent_id=matched`, `trigger_type='mention'`, `trigger_comment_id=댓글.id`)
  - **issue.assignee_agent_id는 변경하지 않음** (멘션은 일회성 위임)
  - issue.status가 'done'이었다면 'open'으로 되돌림
  - 매칭 실패: 댓글은 저장, system 댓글 "에이전트 @Foo를 찾을 수 없습니다" INSERT, `mention_warnings: ["@Foo not found"]`
- 매칭 2개 이상: **첫 매칭만 사용**, system 댓글 "@First만 적용됩니다" INSERT, `mention_warnings: ["multiple mentions, only @First applied"]`

**중복 dispatch**: 같은 (issue, 멘션된 agent) 조합에 이미 queued run이 있으면 unique index 위반 → 댓글은 저장하되 dispatch는 skip, `mention_warnings: ["already queued for @<agent>"]`.

**현재 execution_status가 `running`일 때 멘션**: 새 run은 queued로 INSERT만 되고 현재 running run이 끝난 후 워커가 claim. UI에는 "대기 중: Writer run #N" 표시.

### 4.3 `DELETE /api/comments/:id`
댓글 삭제. agent/system 댓글은 삭제 불가 (audit 무결성).

**Errors**:
- 403 FORBIDDEN: author_type ≠ 'user'
- 409 STATE_ERROR: 이 댓글이 `queued`/`running` run의 `trigger_comment_id`로 참조 중 (먼저 run cancel/완료 필요)

---

## 5. Runs

### 5.1 `GET /api/issues/:id/runs`
이슈의 실행 이력.

**Response 200**:
```json
{
  "runs": [
    {
      "id": "uuid",
      "issue_id": "uuid",
      "agent_id": "uuid",
      "agent_name": "NewsLead",
      "status": "running",
      "trigger_type": "mention",
      "trigger_comment_id": "uuid",
      "enqueued_at": "...",
      "claimed_at": "...",
      "started_at": "...",
      "finished_at": null,
      "exit_code": null,
      "stdout_size_bytes": 4321,
      "log_url": "/api/runs/<id>/log",
      "error_message": ""
    }
  ]
}
```

**필드 설명**:
- `status`: `queued | running | done | failed | cancelled`
- `trigger_type`: `issue_created | mention | autopilot | rerun`
- `stdout_size_bytes`: 응답 시점에 백엔드가 stdout 파일 stat으로 계산. 파일 없으면 0.
- `log_url`: 다운로드 경로. **`stdout_path` 절대 경로는 응답에 포함하지 않음** (보안).

### 5.2 `GET /api/runs/:id/log`
run의 stdout 전체 파일.

- Content-Type: `text/plain; charset=utf-8`
- 헤더: `Content-Disposition: attachment; filename="<run-id>.log"` 옵션
- 응답 크기 cap: 단일 run 10MB (executor 단계에서 이미 cap, 초과분은 truncation 표시)

**Errors**:
- 404 NOT_FOUND: stdout_path 파일 없음 (러닝 중이지만 아직 로그 쓰기 전 등)

---

## 6. Autopilot

### 6.1 `GET /api/workspaces/:id/autopilot`
워크스페이스 룰 목록.

**Response 200**:
```json
{
  "rules": [
    {
      "id": "uuid",
      "workspace_id": "uuid",
      "name": "매일 9시 뉴스",
      "cron_expr": "0 9 * * *",
      "issue_title_template": "{{date}} AI 뉴스",
      "issue_body_template": "",
      "assignee_agent_id": "uuid",
      "assignee_agent_name": "NewsLead",
      "enabled": true,
      "last_run_at": "2026-05-11T09:00:00Z",
      "next_run_at": "2026-05-12T09:00:00Z",
      "created_at": "..."
    }
  ]
}
```

### 6.2 `POST /api/workspaces/:id/autopilot`
룰 생성.

**Request**:
```json
{
  "name": "매일 9시 뉴스",
  "cron_expr": "0 9 * * *",
  "issue_title_template": "{{date}} AI 뉴스",
  "issue_body_template": "",
  "assignee_agent_id": null,
  "enabled": true
}
```

**Response 201**: 룰 객체

**Errors**:
- 400: cron_expr 파싱 실패
- 400: 알 수 없는 템플릿 변수

### 6.3 `PUT /api/autopilot/:id`
룰 수정. 가능한 필드: name, cron_expr, issue_title_template, issue_body_template, assignee_agent_id, enabled.

### 6.4 `DELETE /api/autopilot/:id`
룰 삭제. 이 룰로 생성된 기존 이슈는 보존.

### 6.5 `POST /api/autopilot/:id/trigger`
지금 즉시 실행 (수동 트리거).

**Response 201**:
```json
{
  "issue": { ... }   // 새로 생성된 이슈
}
```

---

## 7. System

### 7.1 `GET /healthz`
헬스 체크.

**Response 200**:
```json
{
  "status": "ok",
  "version": "0.1.0",
  "uptime_seconds": 12345,
  "db_ok": true,
  "available_runtimes": ["codex", "claude"]
}
```

### 7.2 `GET /api/settings`
시스템 설정/상태.

**Response 200**:
```json
{
  "version": "0.1.0",
  "data_dir": "/Users/.../.corn-agent-dashboard",
  "available_runtimes": [
    { "name": "codex", "version": "1.2.3", "path": "/usr/local/bin/codex" },
    { "name": "claude", "version": "0.9", "path": "..." }
  ],
  "worker_pool_size": 3,
  "auth_mode": "none",
  "timezone": "Asia/Seoul"
}
```

### 7.3 `POST /api/system/backup`
SQLite DB 백업 (online backup API).

**Request**: `{ "to": "/path/to/backup.db" }` (선택, 미지정 시 `~/.corn-agent-dashboard/data.db.<timestamp>`)

**Response 200**: `{ "backup_path": "/...", "size_bytes": 1234567 }`

### 7.4 `POST /api/system/vacuum`
`PRAGMA incremental_vacuum` 실행.

**Response 200**: `{ "reclaimed_bytes": 12345 }`

### 7.5 `POST /api/system/cleanup-logs`
지정한 일수보다 오래된 run의 stdout 파일 삭제.

**Request**: `{ "days": 30 }`

**Response 200**: `{ "deleted_files": 42, "freed_bytes": 12345678 }`

---

## 8. 엔드포인트 총괄

| # | Method | Path | 설명 |
|---|---|---|---|
| 1 | GET | `/api/workspaces` | 워크스페이스 목록 |
| 2 | POST | `/api/workspaces` | 워크스페이스 생성 (+메인 에이전트) |
| 3 | GET | `/api/workspaces/:workspace` | 워크스페이스 상세 (id 또는 slug) |
| 4 | PUT | `/api/workspaces/:workspace` | 워크스페이스 수정 |
| 5 | DELETE | `/api/workspaces/:workspace` | 워크스페이스 삭제 |
| 6 | GET | `/api/workspaces/:workspace/agents` | 에이전트 목록 |
| 7 | POST | `/api/workspaces/:workspace/agents` | 에이전트 추가 |
| 8 | GET | `/api/agents/:id` | 에이전트 상세 |
| 9 | PUT | `/api/agents/:id` | 에이전트 수정 |
| 10 | POST | `/api/agents/:id/promote` | 메인 승격 |
| 11 | DELETE | `/api/agents/:id` | 에이전트 삭제 |
| 12 | GET | `/api/workspaces/:workspace/issues` | 이슈 목록 |
| 13 | POST | `/api/workspaces/:workspace/issues` | 이슈 생성 + dispatch |
| 14 | GET | `/api/workspaces/:workspace/issues/:issue` | 이슈 상세 (id 또는 identifier) |
| 14b | GET | `/api/issues/:id` | 이슈 상세 (UUID 직접) |
| 15 | PUT | `/api/issues/:id` | 이슈 수정 (status 변경 포함) |
| 16 | POST | `/api/issues/:id/rerun` | 재실행 (마지막 run의 agent) |
| 17 | POST | `/api/issues/:id/cancel` | 진행 중 run 취소 |
| 18 | DELETE | `/api/issues/:id` | 이슈 삭제 |
| 19 | GET | `/api/issues/:id/comments` | 댓글 목록 |
| 20 | POST | `/api/issues/:id/comments` | 댓글 작성 (멘션 트리거) |
| 21 | DELETE | `/api/comments/:id` | 댓글 삭제 (user only) |
| 22 | GET | `/api/issues/:id/runs` | run 이력 |
| 23 | GET | `/api/runs/:id/log` | run 로그 다운로드 |
| 24 | GET | `/api/workspaces/:workspace/autopilot` | 룰 목록 |
| 25 | POST | `/api/workspaces/:workspace/autopilot` | 룰 생성 |
| 26 | PUT | `/api/autopilot/:id` | 룰 수정 |
| 27 | DELETE | `/api/autopilot/:id` | 룰 삭제 |
| 28 | POST | `/api/autopilot/:id/trigger` | 즉시 실행 |
| 29 | GET | `/healthz` | 헬스 체크 |
| 30 | GET | `/api/settings` | 시스템 설정 |
| 31 | POST | `/api/system/backup` | DB 백업 |
| 32 | POST | `/api/system/vacuum` | DB vacuum |
| 33 | POST | `/api/system/cleanup-logs` | run 로그 정리 |

**총 33 엔드포인트** (14b는 14의 alias로 카운트 X).

---

## 9. polling 가이드 (Frontend용)

- 이슈 보드: 5초마다 GET `/api/workspaces/:workspace/issues`
- 이슈 상세 (`execution_status` ∈ {`queued`, `running`}): 3초마다 댓글 + runs 동시 fetch
- 이슈 상세 (`execution_status` ∈ {`done`, `failed`, `cancelled`, `idle`}): polling 정지
- 워크스페이스 헤더: 30초마다 `/healthz`

**중요**: polling 조건은 `issue.status`가 아니라 `execution_status`. 이슈가 'done'이어도 새 멘션으로 queued run이 추가되면 'open' + 'queued'가 되므로 polling 재개.

---

## 10. 향후 (Phase 2+)

- SSE/WebSocket으로 polling 대체 (선택)
- bulk endpoints (POST 여러 이슈 동시 생성)
- export/import (워크스페이스 단위 JSON)
- webhook 발신 (이슈 done 시 외부 알림)
