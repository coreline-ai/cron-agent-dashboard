# CHAINING — corn-agent-dashboard

> Status: 정책 문서 + Phase 2+ 후보 설계
> Date: 2026-05-14

---

## 1. 현재 정책: explicit-only

현재 구현된 체이닝 정책은 **explicit-only**다.

- 사용자 댓글의 `@AgentName` 명시 멘션만 같은 issue에 새 run을 생성한다.
- agent 결과 댓글 안의 `@AgentName`은 markdown 텍스트로만 저장/표시하며 자동 dispatch하지 않는다.
- 이슈 담당자(`issue.assignee_agent_id`)는 멘션으로 바뀌지 않는다. 새 run의 `agent_id`만 멘션된 agent가 된다.

이 정책을 기본값으로 유지하는 이유:

1. agent 결과가 다시 agent를 부르는 무한 루프를 방지한다.
2. 예상치 못한 CLI 실행과 비용 폭주를 막는다.
3. agent가 hallucinated mention을 출력해도 실행 권한으로 승격되지 않게 한다.
4. 사용자가 각 위임 단계를 댓글로 승인하므로 로컬 MVP의 감사 가능성이 높다.

---

## 2. 현재 dispatch 흐름

현재 새 run을 만드는 경로는 다음 네 가지뿐이다.

| 경로 | 현재 `trigger_type` | 새 run 생성 조건 |
|---|---|---|
| 이슈 생성 | `issue_created` | 이슈 생성 트랜잭션에서 기본/선택 agent로 queued run 생성 |
| 사용자 댓글 명시 멘션 | `mention` | `POST /api/issues/:id/comments`의 사용자 댓글 본문에서 첫 `@AgentName` 매칭 |
| Autopilot | `autopilot` | cron/manual trigger가 issue + run 생성 |
| 재실행 | `rerun` | 사용자가 [재실행] 또는 API를 호출 |

중요한 제외 사항:

- agent comment 저장 경로는 mention parser를 호출하지 않는다.
- agent 결과 댓글에서 발견되는 `@AgentName`은 현재 자동 실행 조건이 아니다.
- 현재 DB에는 `chain_id`, `parent_run_id`, `chain_depth` 컬럼이 없다.

---

## 3. Phase 2+ 후보: auto-chain opt-in (미구현)

Auto-chain은 **후보 설계**이며 현재 기능이 아니다. 구현하더라도 기본값은 반드시 **off**로 둔다.

권장 정책:

- opt-in 단위: workspace 또는 issue 단위 중 선택.
- 기본값: off.
- 최대 depth: 기본 5 권장.
- 같은 chain 안에서 동일 agent 재호출 차단 권장.
- source run이 `failed` 또는 `cancelled`이면 chain 중단 권장.
- agent 결과에 여러 mention이 있을 때는 첫 번째만 실행하거나 명시 config 필요.
- dispatch 기록은 run_event 또는 system comment로 남긴다.

---

## 4. 후보 데이터 모델 (미구현)

`run` 후보 컬럼:

```sql
ALTER TABLE run ADD COLUMN chain_id TEXT NOT NULL DEFAULT '';
ALTER TABLE run ADD COLUMN parent_run_id TEXT REFERENCES run(id) ON DELETE SET NULL;
ALTER TABLE run ADD COLUMN chain_depth INTEGER NOT NULL DEFAULT 0;

CREATE INDEX idx_run_chain ON run(chain_id, chain_depth, enqueued_at);
CREATE INDEX idx_run_parent ON run(parent_run_id);
```

Opt-in 후보 컬럼 중 하나를 선택한다.

```sql
-- workspace 단위 opt-in 후보
ALTER TABLE workspace ADD COLUMN auto_chain_enabled INTEGER NOT NULL DEFAULT 0 CHECK (auto_chain_enabled IN (0,1));
ALTER TABLE workspace ADD COLUMN auto_chain_max_depth INTEGER NOT NULL DEFAULT 5;

-- 또는 issue 단위 opt-in 후보
ALTER TABLE issue ADD COLUMN auto_chain_enabled INTEGER NOT NULL DEFAULT 0 CHECK (auto_chain_enabled IN (0,1));
```

후보 의미:

| 필드 | 후보 의미 |
|---|---|
| `chain_id` | 같은 자동 체인에 속한 run을 묶는 ID. root run id 또는 별도 UUID 후보. |
| `parent_run_id` | agent 결과 comment를 만든 source run. |
| `chain_depth` | root explicit run은 0, auto-chain으로 만들어진 run은 parent + 1. |

---

## 5. 후보 trigger_type (미구현)

현재 `trigger_type` enum은 `issue_created | mention | autopilot | rerun`만 사용한다.

Auto-chain 구현 시 후보:

| 후보 | 의미 |
|---|---|
| `agent_mention` | agent 결과 댓글의 mention이 opt-in 정책을 통과해 생성한 run |
| `auto_mention` | 자동 mention dispatch임을 더 일반적으로 표현하는 대안 |

둘 중 하나를 schema/API/UI에서 일관되게 선택해야 한다. 현재 구현에는 둘 다 존재하지 않는다.

---

## 6. 후보 worker/store 흐름 (미구현)

1. `CompleteRunWithReason()`이 agent 결과 comment를 저장한다.
2. auto-chain opt-in이 꺼져 있으면 mention parse 없이 종료한다.
3. opt-in이 켜져 있으면 방금 저장한 agent comment에서 첫 mention을 파싱한다.
4. guard를 확인한다.
   - `chain_depth < max_depth` (기본 max 5)
   - 같은 `chain_id` 안에 동일 agent active/terminal run이 없음
   - 같은 issue/agent에 queued run이 없음
   - source run status가 `done`일 때만 진행하고 `failed/cancelled`면 중단
5. 새 run을 INSERT한다.
   - `trigger_type='agent_mention'` 또는 `auto_mention` 후보
   - `trigger_comment_id=<agent_comment_id>`
   - `trigger_content_snapshot=capSnapshot(agent_comment_content)`
   - `parent_run_id=<source_run_id>`
   - `chain_id=<source_chain_id or source_run_id>`
   - `chain_depth=source_depth+1`
6. system comment 또는 run_event로 auto-chain dispatch 결과를 기록한다.

---

## 7. 후보 API/UI (미구현)

API 후보:

- `GET /api/settings` 또는 workspace 응답에 auto-chain 정책 표시.
- workspace/issue update API에 `auto_chain_enabled`, `auto_chain_max_depth` 추가 후보.
- `GET /api/issues/:id/runs` 응답에 `chain_id`, `parent_run_id`, `chain_depth` 추가 후보.
- 기존 응답 호환성을 위해 필드 추가 방식만 사용한다.

UI 후보:

- Settings 또는 workspace settings에 “자동 체이닝” toggle.
- Issue Detail 작업 콘솔에 “자동 체이닝: OFF/ON, depth N/5” 표시.
- Run 이력은 우선 flat list를 유지하고, 후속으로 lineage tree를 검토한다.

---

## 8. 구현 전 체크리스트

- [ ] opt-in 단위를 workspace로 할지 issue로 할지 결정한다.
- [ ] `trigger_type` 후보 중 `agent_mention` 또는 `auto_mention` 하나를 선택한다.
- [ ] max depth 기본값 5와 사용자 변경 가능 범위를 결정한다.
- [ ] 같은 chain 내 동일 agent 재호출 차단 기준을 active만 볼지 terminal까지 볼지 결정한다.
- [ ] failed/cancelled 후 chain 중단 정책을 테스트 케이스로 고정한다.
- [ ] 여러 mention 처리 정책(첫 번째만 vs 모두)을 결정한다.
- [ ] run_event/system comment 메시지 형식을 정한다.
- [ ] migration 번호와 rollback 불가 forward-only 정책을 확인한다.

