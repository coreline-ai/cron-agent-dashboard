# CHAINING — corn-agent-dashboard

> Status: 구현됨 — workspace opt-in 자동 체이닝
> Date: 2026-05-15

## 1. 기본 정책

자동 체이닝은 **기본 OFF**다. 사용자가 `/settings`에서 workspace별 `agent 결과 @mention 자동 체이닝 허용`을 켠 경우에만 agent 결과 댓글의 첫 `@AgentName`이 새 run으로 dispatch된다.

기본값을 OFF로 둔 이유:

1. 무한 루프와 비용 폭주 방지
2. hallucinated mention의 실행 권한 승격 방지
3. 단일 사용자 로컬 MVP의 감사 가능성 유지
4. workspace 단위로 필요한 도메인에서만 명시 opt-in

## 2. Dispatch 경로

| 경로 | `trigger_type` | 조건 |
|---|---|---|
| 이슈 생성 | `issue_created` | 이슈 생성 트랜잭션에서 기본/선택 agent queued |
| 사용자 댓글 명시 멘션 | `mention` | 사용자 댓글 본문 첫 `@AgentName` |
| Agent 결과 자동 체이닝 | `mention` | workspace opt-in ON + 완료 run 결과 댓글 첫 `@AgentName` |
| Autopilot | `autopilot` | cron/manual trigger |
| 재실행 | `rerun` | 사용자가 재실행 호출 |

기존 DB CHECK 호환성을 위해 auto-chain도 `trigger_type='mention'`을 사용하고, `run_event.details.auto_chain=true`, `parent_run_id`, `chain_id`, `chain_depth`로 구분한다.

## 3. 안전 가드

- workspace `auto_chain_enabled=false`면 agent 결과 mention은 텍스트로만 저장한다.
- source run이 `done`일 때만 체이닝한다.
- 한 agent 결과에 mention이 여러 개 있어도 첫 번째만 처리한다.
- `chain_depth >= auto_chain_max_depth`면 추가 실행하지 않고 system comment를 남긴다.
- 같은 `chain_id` 안에서 동일 agent는 재호출하지 않는다.
- dispatch 결과는 system comment와 run_event로 남긴다.

## 4. 데이터 모델

```sql
ALTER TABLE workspace ADD COLUMN auto_chain_enabled INTEGER NOT NULL DEFAULT 0 CHECK (auto_chain_enabled IN (0, 1));
ALTER TABLE run ADD COLUMN parent_run_id TEXT REFERENCES run(id) ON DELETE SET NULL;
ALTER TABLE run ADD COLUMN chain_id TEXT NOT NULL DEFAULT '';
ALTER TABLE run ADD COLUMN chain_depth INTEGER NOT NULL DEFAULT 0 CHECK (chain_depth >= 0 AND chain_depth <= 20);
CREATE INDEX idx_run_chain ON run(chain_id, chain_depth, enqueued_at);
CREATE INDEX idx_run_parent ON run(parent_run_id);
```

Root run은 `chain_id=<run_id>`, `chain_depth=0`이다. Auto-chain run은 `parent_run_id=<source_run_id>`, `chain_id=<root_run_id>`, `chain_depth=parent+1`이다.

## 5. 운영 권장

- 비용이 큰 workspace에서는 OFF 유지 권장.
- NewsLead → Writer → Publisher처럼 명확한 pipeline이 있는 workspace에서만 ON 권장.
- 예상치 못한 자동 체이닝이 발생하면 `/settings`에서 workspace toggle을 끄고 run event timeline에서 `auto_chain=true` 이벤트를 확인한다.

## Workspace guard policy

Auto-chain은 workspace별로 다음 guard를 가진다.

- `auto_chain_max_depth`: 기본 5, 최대 20
- `auto_chain_daily_run_limit`: 기본 20, `0`이면 run 수 제한 없음
- `auto_chain_daily_cost_micros`: 기본 0, `0`이면 비용 제한 없음
- `auto_chain_dry_run`: mention을 감지하고 system comment를 남기지만 run은 등록하지 않음

제한에 걸리면 새 run을 만들지 않고 system comment로 이유를 기록한다. 이 정책은 hallucinated mention, recursive chain, 예기치 못한 비용 증가를 막기 위한 workspace-local safety guard다.
