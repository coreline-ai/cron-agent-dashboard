# Recommended Project Skills for cron-agent-dashboard

> 작성일: 2026-05-20  
> 목적: 현재 프로젝트의 안정화 중심 운영에 바로 등록해서 쓸 수 있는 P0 Agent Skills를 정의한다.

## 결론

현재 `cron-agent-dashboard`에는 기능 확장형 skill보다 **안정성·보안·릴리스 검증 skill**이 가장 적합하다.

추천 P0 3개:

| 우선순위 | Skill | 기본 할당 | 목적 |
|---|---|---|---|
| P0-1 | `cron-stability-reviewer` | `trigger`, priority 10 | store/worker/run lifecycle 변경의 데이터 정합성·idempotency·race 위험 검토 |
| P0-2 | `cron-security-guard` | `trigger`, priority 20 | prompt/skill/runtime/log/auth 경계의 보안 리스크 검토 |
| P0-3 | `cron-release-operator` | `manual`, priority 30 | commit/push/release 전 문서·CI·migration·naming 정합성 검증 |

이 3개는 새 tool runtime을 만들지 않고, 기존 Agent Skills registry의 `SKILL.md` prompt module로 등록하는 것이 맞다.

## 파일 위치

```text
docs/skills/
├── recommended-project-skills.md
└── project-skills/
    ├── cron-stability-reviewer/SKILL.md
    ├── cron-security-guard/SKILL.md
    ├── cron-release-operator/SKILL.md
    └── payloads.json
```

## Dashboard 등록 방식

Agent Detail 화면에서 직접 등록할 수 있다.

1. `/w/{workspace_slug}/agents/{agent_id}` 이동
2. `Agent Skills` 섹션으로 이동
3. `새 Skill 등록` 폼에 SKILL.md frontmatter/body를 기준으로 입력
4. `기존 Skill 할당` 폼에서 agent에 할당
5. activation mode 선택
   - `always`: 모든 run에 주입
   - `trigger`: 제목/본문/댓글/trigger snapshot에 keyword가 있을 때 주입
   - `manual`: `#skills: skill-name`으로 명시했을 때만 주입

API로 등록하려면 `docs/skills/project-skills/payloads.json`을 사용한다.

```bash
python3 scripts/seed-project-skills.py \
  --base-url http://127.0.0.1:8080/api \
  --workspace <workspace-slug> \
  --agent-name <agent-name>
```

토큰 모드 서버라면:

```bash
CRON_AGENT_DASHBOARD_TOKEN=<token> python3 scripts/seed-project-skills.py \
  --base-url http://127.0.0.1:8080/api \
  --workspace <workspace-slug> \
  --agent-name <agent-name>
```

## P0 Skill 상세

### 1. cron-stability-reviewer

사용 시점:

- `internal/store/*`, `internal/worker/*`, `internal/app/*` 변경
- migration 추가
- run/cancel/retry/heartbeat/stale recovery 변경
- API 핸들러에서 issue/run/comment 상태를 바꾸는 변경

주요 검토:

- SQLite transaction guard
- terminal run late-write 방지
- retry/reset/heartbeat 필드 정합성
- `run_event` audit 누락 여부
- race test 필요 여부

### 2. cron-security-guard

사용 시점:

- prompt builder, skills registry, runtime adapter 변경
- auth/CORS/backup/log 관련 변경
- workspace working_dir/file exposure 관련 변경
- README/OPERATIONS 보안 문구 변경

주요 검토:

- prompt fence 유지
- skill trust/source/hash 보존
- token/instructions/stdout 로그 누출 방지
- 외부 bind token 강제
- scripts 자동 실행 금지

### 3. cron-release-operator

사용 시점:

- commit/push 전
- release tag 전
- README/CHANGELOG/docs 갱신
- CI/e2e 실패 원인 정리

주요 검토:

- `cron` naming consistency
- migration/docs/API 정합성
- `make check` / `go test -race` / `pnpm web build` 기준
- embedded web static 최신성

## 운영 정책

- P0 skill은 모두 `trust_level=local`로 시작한다.
- scripts는 등록하지 않는다. 등록하더라도 dashboard가 자동 실행하지 않는다.
- `cron-release-operator`는 기본 `manual`이 적절하다. release/checklist 지침이 모든 run에 들어가면 불필요한 prompt 비용이 생긴다.
- `cron-stability-reviewer`, `cron-security-guard`는 trigger 기반으로 둔다. 관련 keyword가 있을 때만 활성화한다.

## 다음 후보

P0 3개 운영 후 필요할 때만 추가한다.

- `cron-frontend-qa`: UI 변경이 잦아질 때
- `cron-docs-sync`: 문서만 별도 agent에 맡길 때
- `cron-sql-migration-reviewer`: migration 변경량이 늘어날 때
