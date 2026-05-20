# RFP 협업 스튜디오 추천 Agent Skills

> 대상 workspace: `rfp-studio`  
> 작성일: 2026-05-20

## 목적

RFP 협업 스튜디오의 6개 에이전트가 동일한 제안서 품질 기준으로 협업하도록, 역할별 `SKILL.md` 지침을 workspace skill registry에 등록하고 agent별로 할당한다.

## Skill 목록

| Skill | 주요 Agent | 기본 mode | 목적 |
|---|---|---|---|
| `rfp-orchestrator` | Lead | always | 전체 RFP 흐름, 역할 분배, 평가 기준 정렬 |
| `rfp-sales-win-theme` | Sales | always | 고객 pain, win theme, 차별화, 반론 대응 |
| `rfp-scope-planner` | Planner | always | 요구사항, IA, 범위, 일정, 산출물 정리 |
| `rfp-ux-concept-designer` | Designer | always | UX 컨셉, 핵심 화면, 디자인 시스템 방향 |
| `rfp-technical-architect` | Architect | always | 기술 아키텍처, 통합, 보안, 성능, 운영 |
| `rfp-qa-acceptance-reviewer` | QA | always | 수용 기준, E2E, UAT, 출시 체크리스트 |
| `rfp-proposal-integrator` | Lead | trigger | 여러 agent 산출물 최종 통합 |
| `rfp-risk-red-team` | QA/Lead/Architect/Sales | trigger | 제출 전 리스크·누락·반론 점검 |

## Agent별 권장 할당

| Agent | Skills |
|---|---|
| Lead | `rfp-orchestrator(always)`, `rfp-proposal-integrator(trigger)`, `rfp-risk-red-team(trigger)` |
| Sales | `rfp-sales-win-theme(always)`, `rfp-risk-red-team(trigger)` |
| Planner | `rfp-scope-planner(always)` |
| Designer | `rfp-ux-concept-designer(always)` |
| Architect | `rfp-technical-architect(always)`, `rfp-risk-red-team(trigger)` |
| QA | `rfp-qa-acceptance-reviewer(always)`, `rfp-risk-red-team(trigger)` |

## GUI 확인 방법

1. `http://127.0.0.1:8080/w/rfp-studio/agents` 접속
2. 각 agent 상세 페이지 진입
3. 하단 `Agent Skills` 섹션에서 할당 카드 확인
4. `기존 Skill 할당` select에 workspace registry skill 목록 확인

## API 등록

```bash
python3 scripts/seed-rfp-skills.py \
  --base-url http://127.0.0.1:8080/api \
  --workspace rfp-studio
```

토큰 모드일 경우:

```bash
CRON_AGENT_DASHBOARD_TOKEN=<token> python3 scripts/seed-rfp-skills.py \
  --base-url http://127.0.0.1:8080/api \
  --workspace rfp-studio
```

## 운영 정책

- 모든 RFP skill은 `trust_level=local`로 시작한다.
- scripts 자동 실행은 사용하지 않는다.
- 역할별 핵심 skill은 `always`, 통합/리스크 검토 skill은 `trigger`로 둔다.
- prompt 비용이 과도하면 `rfp-risk-red-team`을 `manual`로 낮출 수 있다.
