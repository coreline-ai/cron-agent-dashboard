# internal/store

`internal/store`는 SQLite 스키마를 도메인 API로 감싸는 영속성 계층입니다. `run` 테이블이 durable queue 역할도 하기 때문에, 큐 상태 전이와 비즈니스 상태 전이는 같은 트랜잭션 안에서 처리합니다.

## 파일 역할

| 파일 | 역할 |
|---|---|
| `store.go` | `Store` 생성, 공통 option, 에러 normalize, 공통 DB helper |
| `workspace.go` | workspace CRUD, auto-chain 설정, timeout 기본값 |
| `agents.go` | agent CRUD, main agent 승격, instruction version 관리 |
| `issues.go` | issue CRUD, sub-issue 생성/조회, issue 검색 |
| `runs.go` | run claim/complete/fail, usage 집계, log path 조회 |
| `cancellation.go` | run cancel, active run 조회, heartbeat/stale/orphan recovery, retry reschedule |
| `comments.go` | 사용자 댓글, mention dispatch, comment 삭제/조회 |
| `autopilot.go` | autopilot rule CRUD, trigger 결과, failure visibility/auto-disable |
| `auto_chain.go` | workspace opt-in auto-chain dispatch와 depth/run/cost/dry-run guard |
| `reasons.go` | terminal/failure/cancel reason normalize와 사용자 메시지 helper |
| `run_events.go` | run event audit trail append/list |
| `resource_controls.go` | retry policy, timeout resolve, token/cost metrics helper |
| `process_tracking.go` | process pid/pgid 기록과 startup/stale cleanup support |

## 안정성 원칙

- SQLite는 `SetMaxOpenConns(1)` 정책으로 write 직렬화를 우선합니다.
- 상태 전이는 가능한 한 `UPDATE ... WHERE status IN (...)` 가드를 둡니다.
- run terminal 처리(`CompleteRunWithReason`, `CancelRunWithReason`, `FailInfrastructureRun`)는 댓글과 `run_event`를 같은 트랜잭션에 기록합니다.
- late completion/cancel race는 terminal 상태를 덮어쓰지 않고 현재 DB 상태를 반환합니다.
- 사용자/agent 입력은 store에서 실행하지 않고, prompt 계층에서 fence로 구분합니다.

## 테스트 정책

store 테스트는 mock DB 대신 임시 SQLite 파일에 전체 migration을 적용합니다. CHECK/FK/partial unique index, WAL/PRAGMA, migration 호환성이 이 계층의 핵심 계약이므로 integration-style 테스트를 기본값으로 유지합니다.
