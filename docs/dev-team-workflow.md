# Dev Team Workflow Seed

`cron-agent-dashboard seed-dev-team`은 하나의 workspace 안에 7개 역할 에이전트와 8개 Agent Skill을 생성한다. `seed-lab`이 여러 워크스페이스 병렬 실험용이라면, `seed-dev-team`은 한 프로젝트 안에서 PM hub가 순차 멘션으로 실제 개발팀처럼 굴리는 실험용이다.

## 생성 명령

두 가지 진입점이 있다 — 운영자 취향에 따라 골라서 사용한다.

### 옵션 A. 웹앱에서 한 번에 (권장 — 시뮬레이션 포함)

1. `./cron-agent-dashboard serve --workers 3` 으로 서버 띄움.
2. 브라우저에서 `/settings` 진입 → 최상단 **"AI Dev Team 워크스페이스"** 카드.
3. slug(기본 `ai-dev-team`) + working_dir(비우면 서버 `data_dir`) 입력 → **AI Dev Team 생성** 클릭.
4. 자동으로 `/w/<slug>/board`로 이동. 이슈가 0개면 **"샘플 이슈로 시작"** 버튼 노출 → 클릭 시 canned acceptance criteria가 채워진 새 이슈 모달.
5. 이슈 생성 즉시 `@Lead`가 자동 dispatch — chain dashboard(`/w/<slug>/chains`)에서 실시간 흐름 관찰.

### 옵션 B. CLI (CI 자동화 / 헤드리스)

```bash
make build
./cron-agent-dashboard seed-dev-team --slug ai-dev-team --working-dir "$(pwd)"
./cron-agent-dashboard serve --workers 3
```

옵션 B 동작 규칙:

- `--slug` 기본값: `ai-dev-team`
- `--working-dir` 생략 시 현재 위치에서 가장 가까운 git root를 자동 감지한다.
- workspace는 `auto_chain_enabled=true`, `auto_chain_max_depth=8`, `per_run_worktree=true`, `auto_close_on_run_done=false`로 생성된다.

HTTP 진입점 (옵션 A가 내부적으로 호출): `POST /api/system/seed-dev-team` body `{slug?, working_dir?}` — 응답에 workspace / agents / skills / assignment 카운트.

## 역할 구성

| Agent | Runtime | 역할 |
|---|---|---|
| `@Lead` | claude | hub PM, 작업 분해, 순차 멘션, QA verdict 기반 종료 |
| `@Designer` | gemini | UX spec, 레이아웃, 디자인 토큰 |
| `@Backend` | codex | API, 비즈니스 로직, Go 테스트 |
| `@Frontend` | codex | React UI, build/test |
| `@DB` | codex | migration, schema, idempotency |
| `@QA` | claude | 회귀 테스트, `## QA-PASS` / `## QA-FAIL` verdict |
| `@DevOps` | codex | CI, release, deployment automation |

## Skill 구성

- `result-protocol` — 전원 always
- `hub-routing` — Lead only
- `ux-spec` — Designer only
- `backend-patterns` — Backend only
- `frontend-patterns` — Frontend only
- `db-migrations` — DB only
- `qa-verdict` — QA only
- `devops-ci` — DevOps only

## 운영 패턴

1. 사용자가 issue를 만들고 assignee를 `@Lead`로 둔다.
2. `@Lead`가 목표를 분해하고 worker 한 명만 멘션한다.
3. worker는 `## RESULT: ...` 헤더로 결과를 남긴다.
4. `@Lead`가 다음 worker 또는 `@QA`를 멘션한다.
5. `@QA`가 `## QA-PASS`이면 Lead가 최종 요약 후 종료한다.
6. `## QA-FAIL`이면 QA가 마지막 줄에 재작업 대상 한 명만 멘션한다.

## 안전 규칙

- main branch 직접 push/force-push 금지.
- 각 run은 per-run worktree에서 실행되므로 결과 적용 전 diff 확인이 필요하다.
- skill은 prompt context로만 주입되며 스크립트를 자동 실행하지 않는다.
- 외부 release/publish는 dry-run까지만 수행한다.

## 첫 이슈로 검증하기

seed 직후 1 cycle이 실제로 7명을 다 거치는지 확인하는 가장 짧은 시나리오. 예시: **"Settings 페이지에 다크모드 토글 추가"**.

1. **이슈 생성** (`/w/ai-dev-team/board` → `+ 새 이슈`):
   - 제목: `Settings 페이지 다크모드 토글 추가`
   - 본문 (acceptance criteria 3-5줄):
     ```
     - settings 페이지에 다크/라이트 토글 버튼
     - localStorage 'theme' 키로 영속화
     - 페이지 로드 시 저장된 값 복원
     ```
   - assignee: `@Lead` (기본값).

2. **기대 흐름** (chain_depth 가드를 거치는 멘션 순서):
   - `@Lead` 초기 run → 분해 + `@Designer` 멘션.
   - `@Designer` → 토큰 결정 markdown + `@Frontend` 멘션. (Backend/DB 변경 불필요로 판단)
   - `@Frontend` → React 컴포넌트 + `pnpm --filter web build` PASS → `## RESULT: BUILD-PASS` + `@QA` 멘션.
   - `@QA` → 회귀 확인 → `## QA-PASS` (또는 실패면 `## QA-FAIL @Frontend <사유>`).
   - `@Lead` 재진입 → QA-PASS 감지 후 issue done 처리.

3. **운영자 체크포인트**:
   - chain dashboard(`/w/ai-dev-team/chains`)에서 chain 1개, depth ≤ 8 확인.
   - Run feed(`/w/ai-dev-team/runs`)에서 7개 run 정도 (`Lead → Designer → Lead → Frontend → Lead → QA → Lead`).
   - 각 worker run의 댓글에 `## RESULT:` 또는 `## QA-PASS` 헤더가 있는지.
   - worktree 디스크 사용량(Settings → worktree 카드)이 비정상적으로 폭증하지 않는지.

4. **실패 시 자주 보는 패턴**:
   - `@Lead`가 본인이 코드를 직접 작성하려고 함 → `hub-routing` skill instructions 보강 필요.
   - QA가 `## QA-PASS` 헤더를 빼먹음 → `qa-verdict` skill 강화.
   - 같은 chain 안에서 두 worker에게 동시 멘션 → `hub-routing`에 "한 번에 한 명만" 명시.
   - chain_depth 8 도달 → max_depth 늘리기보다 PM이 분해 못 한 신호로 보고 sub-issue로 쪼개기.

5. **운영 후 reflect**:
   - 일주일 운영 후 QA-PASS 자동 종결(Layer 2)이 정말 필요한지 판단.
   - 역할별 dispatch 빈도 분포에 따라 `agent.role` 컬럼(Layer 3) 필요성 판단.
