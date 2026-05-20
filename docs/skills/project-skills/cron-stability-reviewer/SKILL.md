---
name: cron-stability-reviewer
description: Review cron-agent-dashboard changes for data consistency, SQLite transaction safety, run lifecycle idempotency, worker cancellation/retry behavior, and regression test coverage. Use when editing internal/store, internal/worker, internal/app, migrations, or API handlers that mutate runs/issues/comments/skills.
triggers: [store, worker, run lifecycle, migration, sqlite, retry, cancellation, heartbeat, stale recovery, idempotency]
---
# Cron Stability Reviewer

## 목적

이 skill은 cron-agent-dashboard의 핵심 안정성 경계를 지키기 위한 리뷰 지침이다. 새 기능을 늘리기보다 기존 run/issue/worker 흐름이 깨지지 않는지 확인한다.

## 반드시 확인할 것

1. **트랜잭션 경계**
   - 하나의 상태 전이는 하나의 명확한 트랜잭션 안에서 끝나는지 확인한다.
   - `tx.Rollback()` defer와 `Commit()` 순서가 안전한지 확인한다.
   - SQLite `MaxOpenConns(1)` 전제를 깨는 변경인지 확인한다.

2. **Run lifecycle invariant**
   - `queued → running → done|failed|cancelled` 외 전이를 만들지 않는다.
   - terminal run을 late writer가 덮어쓰지 못하게 `WHERE status='running'` 같은 guard가 있는지 확인한다.
   - retry는 `attempt`, `next_retry_at`, `failure_kind`, `run_event`가 일관되게 갱신되는지 확인한다.

3. **Cancellation / recovery**
   - active run 취소는 queued/running을 구분한다.
   - worker pending cancel, heartbeat, stale recovery, orphan recovery 경로가 서로 충돌하지 않는지 확인한다.
   - cancel-after-claim처럼 stdout path만 남는 부분 결과 회수 정책을 유지한다.

4. **Audit / observability**
   - 사용자에게 보이는 상태 변화는 comment 또는 `run_event`로 추적 가능한지 확인한다.
   - silent fail (`_, _ = ...`)이 새로 생기지 않았는지 확인한다.

5. **테스트**
   - store 변경은 SQLite integration test를 추가/수정한다.
   - worker 동시성 변경은 `go test -race ./...` 또는 최소 해당 패키지 race test로 검증한다.

## 출력 형식

리뷰 결과는 아래 순서로 작성한다.

1. `판정`: 안전 / 주의 / 차단
2. `핵심 리스크`: 최대 5개
3. `필수 수정`: 파일과 함수 단위로 구체화
4. `검증`: 실행한 테스트와 결과
