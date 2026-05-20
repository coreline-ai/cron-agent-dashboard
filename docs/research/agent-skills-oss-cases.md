# Agent Skills OSS 사례 연구와 cron-agent-dashboard 도입 판단

> 작성일: 2026-05-20  
> 목적: `cron-agent-dashboard`의 Agent Skills registry를 더 확장하기 전에, 공개 사례의 구조·활성화 방식·보안 리스크를 비교해 현재 구현의 적정성과 다음 단계 우선순위를 판단한다.

## 1. 결론 요약

현재 프로젝트에는 **Skill을 새 runtime이나 tool executor로 만들지 않고, agent instructions를 재사용 가능한 prompt module로 관리하는 registry 방식**이 가장 안정적이다.

이미 구현된 Phase 1 구조는 공개 사례와 비교해도 방향이 맞다.

- workspace별 `skill` registry
- agent별 `agent_skill` 할당
- `always` / `trigger` / `manual` activation
- `SKILL.md` frontmatter 파싱
- `SKILL_CONTEXT` fence 기반 prompt 주입
- `trust_level`, `source_type`, `content_hash`
- scripts 자동 실행 금지
- run event `skills_loaded` audit

다음 단계는 기능 확장보다 **validation, trust policy, import diff preview**가 우선이다.

## 2. 비교 대상

| 사례 | 유형 | 핵심 관찰 | 적용 판단 |
|---|---|---|---|
| Claude Code Skills | 공식 agent skill UX | `SKILL.md` + YAML frontmatter + personal/project/plugin scope. 설명 기반 자동 선택과 선택적 supporting files | 포맷/스코프 개념 참고. scripts 자동 실행은 현재 프로젝트에서는 금지 유지 |
| Vercel Labs `skills` CLI | multi-agent skill installer | GitHub/local path에서 skill을 설치하고 여러 agent target에 배포 | Phase 2 import UX 참고. 즉시 도입 X |
| OpenHands Skills | SDK/agent context 시스템 | always/trigger/progressive disclosure, public skills repository, git cache | 현재 activation 모델과 가장 유사. public auto-load는 보류 |
| Agent Skills 보안 연구 | security/supply chain | `SKILL.md` natural language metadata가 선택/주입/실행 경로를 바꾸는 공격면 | `trust_level`, hash, scripts 자동 실행 금지, diff preview 필요성 강화 |

## 3. Claude Code Skills 분석

Claude Code Skills는 skill을 `SKILL.md` 중심의 디렉토리로 다룬다.

핵심 구조:

```text
my-skill/
├── SKILL.md
├── reference.md
├── examples/
└── scripts/
```

관찰:

- `SKILL.md` frontmatter는 skill discovery의 핵심이다.
- `description`은 agent가 언제 skill을 사용할지 판단하는 주요 신호다.
- personal / project / plugin scope가 있다.
- supporting files와 scripts를 둘 수 있다.
- Claude Code 문맥에서는 scripts 실행이 가능한 사용 모델이지만, 이는 Claude Code의 permission model에 의존한다.

cron-agent-dashboard 적용 판단:

- `SKILL.md` 호환 파서와 `description`/`triggers` 저장은 적절하다.
- 현재 프로젝트는 CLI agent 실행을 이미 다루므로, skill scripts까지 자동 실행하면 권한 경계가 흐려진다.
- 따라서 scripts는 metadata/reference로만 보관하고 자동 실행 금지 정책을 유지해야 한다.
- scope는 Claude의 personal/project/plugin 대신 dashboard의 `workspace` / `agent assignment`로 매핑하는 것이 더 자연스럽다.

## 4. Vercel Labs `skills` CLI 분석

Vercel Labs `skills`는 여러 agent 생태계에 skill을 설치하는 CLI다.

관찰:

- GitHub shorthand, full URL, repo 내부 path, local path 등 다양한 source format을 지원한다.
- project/global install scope를 제공한다.
- symlink/copy 방식 선택과 agent target 선택이 있다.
- 설치 도구이지, runtime execution system은 아니다.

cron-agent-dashboard 적용 판단:

즉시 구현하지 말고 Phase 2 후보로 둔다.

권장 Phase 2 import UX:

```text
1. GitHub/local URL 입력
2. SKILL.md 파싱
3. source_url/source_ref/content_hash 저장
4. diff preview 표시
5. trust_level 기본값은 git 또는 untrusted
6. 사용자가 명시 승인해야 registry 반영
```

피해야 할 것:

- `--yes`에 해당하는 무확인 설치
- remote repo 전체 자동 신뢰
- update 시 silent overwrite
- scripts 자동 실행

## 5. OpenHands Skills 분석

OpenHands는 skill을 agent context injection 시스템으로 다룬다.

관찰:

- always-active inline skill과 trigger-loaded skill을 구분한다.
- keyword trigger에 따라 context를 주입한다.
- AgentSkills 표준 `SKILL.md`를 사용하면 progressive disclosure를 적용할 수 있다.
- public skills repo를 git cache로 자동 clone/update하는 모델도 있다.
- MCP tool 연결도 architecture상 고려한다.

cron-agent-dashboard 적용 판단:

현재 구현과 가장 유사하다.

매핑:

| OpenHands 개념 | cron-agent-dashboard 대응 |
|---|---|
| always loaded | `activation_mode='always'` |
| keyword trigger | `activation_mode='trigger'` + `triggers_json` |
| manual/task trigger | `activation_mode='manual'` + `#skills:` |
| context injection | `SKILL_CONTEXT` fence |
| public skills repo | Phase 2+ import 후보 |
| MCP tools | Phase 2+ readonly pilot 후보 |

다만 OpenHands의 public auto-load는 dashboard에 바로 넣지 않는 것이 좋다. 단일 사용자 로컬 도구라도 AI agent가 파일/CLI 작업을 수행하므로, 외부 skill 자동 유입은 supply-chain 위험이 크다.

## 6. 보안 연구 시사점

최근 Agent Skills 보안 연구는 `SKILL.md` 자체를 단순 문서가 아니라 agent 행동을 바꾸는 **semantic supply-chain artifact**로 본다.

핵심 리스크:

- prompt injection
- 데이터 유출 지시
- privilege escalation
- 악성 또는 과도한 trigger/description으로 skill 선택 유도
- source/update 경로 오염

현재 프로젝트의 좋은 방어선:

- scripts 자동 실행 금지
- `SKILL_CONTEXT` fence 주입
- `source_type`, `source_url`, `source_ref`, `local_path`, `content_hash`, `trust_level` 보유
- run event `skills_loaded`로 실제 주입 이력 audit
- agent별 assignment가 있어 무작위 전역 skill activation을 피함

추가 권장 방어:

- `trust_level='git' | 'untrusted'` skill은 UI에서 경고 표시
- skill update 시 before/after diff preview
- max active skills per run 제한
- skill content size cap을 UI/API 문서에 명시
- skill linter: frontmatter, description 길이, trigger 중복, 위험 문구 탐지
- remote import는 commit hash pinning 우선

## 7. 현재 구현 적합성 평가

| 기준 | 현재 상태 | 평가 |
|---|---|---|
| 표준 호환성 | `SKILL.md` frontmatter 파싱 | 양호 |
| 스코프 | workspace registry + agent assignment | dashboard 도메인에 적합 |
| activation | always / trigger / manual | 공개 사례와 정합 |
| 안전 주입 | `SKILL_CONTEXT` fence | 양호 |
| audit | `skills_loaded` run event | 양호 |
| scripts | 자동 실행 금지 | 매우 중요, 유지 권장 |
| remote import | 미구현 | 적절한 보류 |
| MCP | 미구현 | Phase 2+ readonly pilot 적절 |

## 8. 다음 개발 후보 우선순위

### P0 — 구현 확장 없이 안정화

1. Skill validation/lint 추가
   - frontmatter 필수값 검증
   - description 최소/최대 길이
   - triggers 중복/공백 검증
   - content size cap 검증
   - 위험 문구 heuristic warning

2. UI trust 표시
   - `trust_level` badge
   - `source_type` 표시
   - `content_hash` copy

3. max active skills per run
   - 기본 5개
   - `manual` > `always` > `trigger` 또는 priority 기준

### P1 — 안전한 import

1. local `SKILL.md` upload/import
2. GitHub URL import는 commit hash 또는 tag pin 권장
3. import 전 diff preview
4. update는 사용자 승인 필수

### P2 — MCP readonly pilot

- workspace filesystem read-only browsing
- run log read-only tool
- issue/comment read-only tool
- write/delete tool은 보류

## 9. 하지 말아야 할 것

- skill scripts 자동 실행
- public skill registry 자동 clone/update 기본 ON
- remote skill silent update
- trust_level 없이 외부 skill 등록
- 모든 workspace/agent에 전역 skill 자동 활성화
- MCP write tool을 초기 단계에서 제공

## 10. 권장 최종 방향

`cron-agent-dashboard`는 agent orchestration dashboard다. 따라서 skill은 runtime 확장이 아니라 다음 역할이어야 한다.

```text
Skill = reusable prompt/instruction module
Registry scope = workspace
Activation scope = agent assignment
Execution policy = prompt injection only, no automatic script execution
Audit = skills_loaded run_event
External source = explicit import + trust + diff preview
```

현재 Phase 1 구현은 이 방향과 잘 맞다. 다음은 GitHub import보다 **validation/lint + trust UI + active skill cap**을 먼저 하는 것이 가장 안정적이다.

## 참고 링크

- Claude Code Skills: https://code.claude.com/docs/en/skills
- Vercel Labs skills: https://github.com/vercel-labs/skills
- OpenHands Skills guide: https://docs.openhands.dev/sdk/guides/skill
- OpenHands Skill architecture: https://docs.openhands.dev/sdk/arch/skill
- SKILL.md supply-chain paper: https://arxiv.org/abs/2605.11418
- Agent Skills vulnerability study: https://arxiv.org/abs/2601.10338
