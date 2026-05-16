# Web App Test Plan: Cron Agent Dashboard

> Vite SPA 메뉴에서 도달 가능한 **모든 페이지/기능**을 단계별로 검증하기 위한 통합 테스트 계획서.
> Generated: 2026-05-16
> Project: cron-agent-dashboard (v0.1 stabilized)

---

## 1. Context (배경)

### 1.1 Why (왜 필요한가)
v0.1 안정화 단계에서 store/httpapi 리팩터, auto-chain guard, error boundary, lineage graph 등 광범위한 변경이 누적되어 있다. 기존 `tests/e2e/smoke.spec.ts`는 워크스페이스→이슈→댓글 1-line happy path만 검증하므로, 메뉴에서 도달 가능한 모든 기능을 회귀 검증할 수 있는 **재현 가능한 manual + Playwright 테스트 매트릭스**가 필요하다.

### 1.2 Current State (현재 상태)
- E2E 자동화: `tests/e2e/smoke.spec.ts` 1개 (workspace + issue + comment XSS 방지)
- Unit: 25개 Go test (`go test -race` clean), 6개 Vitest 컴포넌트 테스트
- 페이지: 7개 (Home, Board, IssueDetail, Agents, AgentDetail, Autopilot, Settings)
- 인터랙션: 워크스페이스 스위처, 테마 토글, 모달 4종(워크스페이스/에이전트/이슈/오토파일럿), 필터/검색/뷰토글, 상태 전이 버튼, lineage graph, 댓글 멘션 자동완성
- 백엔드: REST 39 엔드포인트, polling 3초

### 1.3 Target State (목표 상태)
- 페이지별 happy-path와 error-path 테스트 케이스가 **체크박스**로 정리되어 manual QA 또는 Playwright 추가 시 곧바로 참조 가능
- 각 Phase 종료 후 `pnpm exec playwright test`나 manual run이 **독립적으로 통과**해야 다음 Phase 진행
- 회귀 위험이 높은 인터랙션(워크스페이스 전환, 상태 머신, auto-chain dry-run, 백업/복구)은 별도 케이스로 보강

### 1.4 Scope Boundary (범위)
- **In scope**: 브라우저에서 사이드바를 통해 접근 가능한 모든 UI 동작, 모달 흐름, polling/refresh, error boundary, 토큰 인증 핸드오프
- **Out of scope**: CLI 명령(`backup`/`restore`/`init`)의 비-UI 동작, Playwright 외부의 SSE/WebSocket, 모바일 viewport, 다국어 (한국어 단일)

---

## 2. Architecture Overview (테스트 아키텍처)

### 2.1 테스트 계층

```
┌────────────────────────────────────────────────────────────────┐
│   Layer 0 — Foundation                                          │
│   - Playwright config + fresh data-dir 격리                      │
│   - 공통 fixture: workspace + main agent + runtime stub          │
└────────────────────────────────────────────────────────────────┘
            │
            ▼
┌────────────────────────────────────────────────────────────────┐
│   Layer 1 — Page-level (페이지별 happy path + edge)              │
│   Phase 1~8 (한 페이지당 1 Phase)                                │
└────────────────────────────────────────────────────────────────┘
            │
            ▼
┌────────────────────────────────────────────────────────────────┐
│   Layer 2 — Cross-page Integration                              │
│   - 이슈 생성→실행→댓글 멘션→sub-issue→Autopilot 트리거 등        │
└────────────────────────────────────────────────────────────────┘
            │
            ▼
┌────────────────────────────────────────────────────────────────┐
│   Layer 3 — Resilience / Polling / Error                        │
│   - error boundary, 토큰 인증, 백엔드 오류 표시                   │
└────────────────────────────────────────────────────────────────┘
```

### 2.2 Key Design Decisions

| 결정 사항 | 선택 | 근거 |
|---|---|---|
| 테스트 도구 | Playwright (E2E) + Vitest (component) | 이미 `playwright.config.ts` / `vitest.config` 존재 |
| 백엔드 격리 | tmp data-dir + 별도 binary | 기존 `e2e-smoke` Makefile 패턴 재사용 |
| Agent runtime | `missing-runtime` 또는 echo stub | 실제 codex/claude/gemini 호출 회피 |
| 시각 검증 | text/heading/aria-label 매칭 | 스크린샷 회귀는 v0.2+로 미룸 |
| polling 처리 | `expect(...).toBeVisible({ timeout: 15_000 })` | 기본 3초 polling 1~2 사이클 흡수 |

### 2.3 New Files (신규 파일)

| 파일 경로 | 용도 |
|---|---|
| `tests/e2e/workspace.spec.ts` | Phase 2 — workspace switcher / create / settings |
| `tests/e2e/board.spec.ts` | Phase 3 — BoardPage 필터/검색/뷰 |
| `tests/e2e/issue_detail.spec.ts` | Phase 4 — IssueDetailPage 댓글/멘션/sub-issue/lineage |
| `tests/e2e/agents.spec.ts` | Phase 5 — Agents + AgentDetail |
| `tests/e2e/autopilot.spec.ts` | Phase 6 — Autopilot CRUD + trigger + snooze |
| `tests/e2e/settings.spec.ts` | Phase 7 — Settings (workspace/system/token) |
| `tests/e2e/integration.spec.ts` | Phase 8 — cross-page 통합 시나리오 |
| `tests/e2e/fixtures.ts` | 공통 헬퍼: workspace seed, runtime stub, slug 생성 |

### 2.4 Modified Files (수정 파일)

| 파일 경로 | 변경 내용 |
|---|---|
| `playwright.config.ts` | `testDir` 패턴 확장, `globalSetup`/`globalTeardown` 추가 검토 |
| `Makefile` | `e2e-full` 타깃 추가 (`pnpm exec playwright test`) |
| `tests/e2e/smoke.spec.ts` | Phase 0/1 가이드대로 fixture 재사용으로 슬림화 (선택) |

---

## 3. Phase Dependencies (페이즈 의존성)

```
Phase 0 (Foundation)
        │
        ▼
Phase 1 (Shell & Layout)
        │
        ▼
Phase 2 (Workspace)
        │
        ├─▶ Phase 3 (Board)       ─┐
        │                           │
        ├─▶ Phase 5 (Agents)       ─┤  (Phase 3/5/6 병렬 가능)
        │                           │
        └─▶ Phase 6 (Autopilot)    ─┘
                  │
                  ▼
        Phase 4 (Issue Detail)  ← Board에서 진입하므로 Phase 3 의존
                  │
                  ▼
        Phase 7 (Settings)
                  │
                  ▼
        Phase 8 (Integration & Resilience)
```

- Phase 3 / 5 / 6은 워크스페이스 + 메인 에이전트만 있으면 **독립적**으로 실행 가능 → 병렬 작성/실행 허용.
- Phase 4(IssueDetail)는 Phase 3에서 생성한 이슈 fixture에 의존.
- Phase 8은 Phase 1~7의 fixture/페이지 객체를 재사용.

---

## 4. Implementation Phases (구현 페이즈)

### Phase 0: Test Foundation (기반 설정)
> 모든 spec 파일이 공유할 fixture/helper와 격리된 백엔드 부팅
> Dependencies: 없음

#### Tasks
- [ ] `tests/e2e/fixtures.ts`에 `createWorkspaceFixture(request, opts)` 함수 작성 — slug/prefix/main_agent 기본값과 cleanup 등록
- [ ] `tests/e2e/fixtures.ts`에 `seedSecondAgent(request, workspaceSlug, name)` 헬퍼 추가 (`POST /api/workspaces/{slug}/agents`)
- [ ] `playwright.config.ts`에 `webServer.command`가 `.tmp/e2e-data`를 별 dir로 사용하도록 환경변수 검토 (현 상태가 이미 격리됐는지 확인)
- [ ] `Makefile`에 `e2e-full: build\n\tpnpm exec playwright test --config playwright.config.ts` 타깃 추가
- [ ] README의 "검증 명령" 섹션에 `make e2e-full` 1줄 추가

#### Success Criteria
- `pnpm exec playwright test tests/e2e/smoke.spec.ts`가 fixture를 사용해도 그대로 통과
- 각 spec 파일이 `test.beforeEach`에서 fresh workspace를 생성하고 `test.afterEach`에서 삭제 (또는 격리 dir cleanup)

#### Test Cases
- [ ] TC-0.1: `createWorkspaceFixture`가 고유 slug를 만들고 `POST /api/workspaces` 201 응답을 반환한다
- [ ] TC-0.2: 동일 slug 충돌 시 fixture가 409 응답을 받는 케이스를 throw로 처리한다
- [ ] TC-0.E1: `missing-runtime` runtime으로 워크스페이스를 만든 뒤 worker pool이 panic 없이 run을 failed로 마감한다 (3초 이내)

#### Testing Instructions
```bash
make build
pnpm exec playwright test tests/e2e/smoke.spec.ts
```

**테스트 실패 시 워크플로우:**
1. 에러 출력 분석 → 근본 원인 식별
2. 원인 수정 → 재테스트
3. **모든 테스트가 통과할 때까지 다음 Phase 진행 금지**

---

### Phase 1: Shell & Layout (사이드바 / 테마 / 라우팅)
> 모든 페이지에 공통으로 등장하는 `DashboardLayout` 동작
> Dependencies: Phase 0

#### Tasks
- [ ] `tests/e2e/shell.spec.ts` 작성 — 다음 시나리오 한 파일에 묶음
- [ ] 사이드바 nav `대시보드` / `이슈 보드` / `에이전트` / `오토파일럿` / `설정` 클릭 시 URL 전환 검증
- [ ] WorkspaceSwitcher 드롭다운 열기 / 워크스페이스 변경 / 새 워크스페이스 모달 트리거
- [ ] `Light` ↔ `Dark` 테마 토글 후 `<html data-theme>` 속성과 `localStorage` 키 (`cron-agent-dashboard-theme`) 확인
- [ ] 마지막 워크스페이스 기억: `localStorage` 키 `cron-agent-dashboard-last-workspace` 확인
- [ ] 워크스페이스 없을 때 nav에서 `이슈 보드 / 에이전트 / 오토파일럿`이 `nav-disabled`로 비활성화

#### Success Criteria
- 사이드바의 모든 NavLink가 활성 상태에서 `aria-current`/CSS active class 적용
- 테마 변경이 새로고침 후에도 유지
- `/w/none-such/board` 같은 잘못된 slug 진입 시 첫 워크스페이스로 폴백 (DashboardLayout 코드 기준)

#### Test Cases
- [ ] TC-1.1: `/` 접속 시 HomePage 헤더가 보이고, 사이드바 `대시보드` 링크가 active
- [ ] TC-1.2: 워크스페이스 2개가 있을 때 스위처에서 두번째로 전환하면 `/w/<slug>/board`로 이동
- [ ] TC-1.3: 다크 → 라이트 토글 후 새 탭에서도 라이트 유지 (`localStorage` 확인)
- [ ] TC-1.4: 워크스페이스가 0개일 때 사이드바 `이슈 보드 / 에이전트 / 오토파일럿`이 `aria-disabled='true'`
- [ ] TC-1.E1: 존재하지 않는 `/w/zzz/board` 진입 시 첫 워크스페이스 또는 `/`로 리다이렉트

#### Testing Instructions
```bash
pnpm exec playwright test tests/e2e/shell.spec.ts
```

**테스트 실패 시 워크플로우:**
1. 에러 출력 분석 → 근본 원인 식별
2. 원인 수정 → 재테스트
3. **모든 테스트가 통과할 때까지 다음 Phase 진행 금지**

---

### Phase 2: Workspace Lifecycle (워크스페이스 생성/편집/삭제)
> 사이드바와 HomePage에서 진입하는 워크스페이스 CRUD
> Dependencies: Phase 1

#### Tasks
- [ ] `tests/e2e/workspace.spec.ts` 작성
- [ ] HomePage에서 `[+ 새 워크스페이스]` 버튼 → `CreateWorkspaceDialog` 열기
- [ ] 다이얼로그 필수 필드 검증: 이름 / slug / prefix / 메인 에이전트 이름·runtime·instructions
- [ ] 잘못된 slug (`UPPERCASE`, `한글`) 입력 시 서버 4xx 에러를 `MutationErrorAlert`로 표시
- [ ] HomePage 카드의 `더 보기` 토글 (이슈 5개 초과 시) — 토글로 추가 5줄 노출
- [ ] HomePage `최근 이슈가 없습니다` empty state 확인

#### Success Criteria
- 생성 직후 사이드바 스위처에 새 워크스페이스가 즉시 등장
- 생성 직후 URL이 `/w/<new-slug>/board`로 자동 이동 (DashboardLayout `onWorkspaceCreated`)
- 잘못된 입력은 다이얼로그를 닫지 않고 에러 메시지 표시

#### Test Cases
- [ ] TC-2.1: 정상 워크스페이스 생성 → 즉시 보드로 이동 + 사이드바 등장
- [ ] TC-2.2: 메인 에이전트 instructions를 비우면 다이얼로그가 검증 실패로 닫히지 않음
- [ ] TC-2.3: 동일 slug 중복 생성 시 `409` 메시지 표시
- [ ] TC-2.4: HomePage 카드 `더 보기` 토글이 5개 → 10개로 확장 후 다시 축소
- [ ] TC-2.E1: 네트워크 오류(서버 종료 상태) 시 `MutationErrorAlert`에 사용자 친화 메시지 노출

#### Testing Instructions
```bash
pnpm exec playwright test tests/e2e/workspace.spec.ts
```

**테스트 실패 시 워크플로우:**
1. 에러 출력 분석 → 근본 원인 식별
2. 원인 수정 → 재테스트
3. **모든 테스트가 통과할 때까지 다음 Phase 진행 금지**

---

### Phase 3: Board Page (이슈 보드)
> `/w/:slug/board` — 필터 / 검색 / 보기 토글 / 컬럼별 새 이슈 / 상태 전이
> Dependencies: Phase 2 (워크스페이스 fixture)

#### Tasks
- [ ] `tests/e2e/board.spec.ts` 작성
- [ ] `[+ 새 이슈]` 버튼 → `CreateIssueDialog` 제목/본문/담당자 입력 → 이슈 생성 후 보드에 등장
- [ ] 상태 필터 segmented (`전체 / open / done / cancelled`) URL `?status=` 동기화 확인
- [ ] 실행 상태 필터 select (`전체 / queued / running / done / failed / cancelled`) URL `?execution=` 동기화
- [ ] 에이전트 필터 select URL `?agent=` 동기화
- [ ] 보드 ↔ 리스트 뷰 토글 (`?view=board|list`)
- [ ] 검색 input (`?q=`) → debounced 입력 후 결과 필터링
- [ ] 컬럼 헤더 `+` 아이콘으로 해당 status로 이슈 생성
- [ ] 이슈 카드의 `복귀 / 완료 / 취소 / 강제취소` 버튼 disabled 조건 확인

#### Success Criteria
- URL 쿼리 파라미터가 새로고침 후에도 필터/뷰 상태를 복원
- 빈 컬럼 empty state 메시지 노출
- polling 3초 안에 새 이슈가 보드에 자동 반영

#### Test Cases
- [ ] TC-3.1: `[+ 새 이슈]` → `NEWS-1`이 생성되고 `running` 또는 `failed`(missing-runtime)로 자동 전이
- [ ] TC-3.2: 상태 필터 `done`만 선택 시 open/cancelled 이슈 숨김 + URL `?status=done`
- [ ] TC-3.3: 보기 토글을 `list`로 바꾸면 row 형태 렌더링 + URL `?view=list`
- [ ] TC-3.4: 검색 `Smoke`로 필터 → 일치하는 이슈만 표시
- [ ] TC-3.5: open 컬럼 헤더 `+` 클릭 시 `CreateIssueDialog`에 status가 미리 선택됨
- [ ] TC-3.E1: 백엔드 다운 상태에서 보드 헤더에 `MutationErrorAlert` 또는 fallback 표시
- [ ] TC-3.E2: 워크스페이스에 에이전트가 0개일 때 이슈 생성 다이얼로그가 적절한 안내 노출

#### Testing Instructions
```bash
pnpm exec playwright test tests/e2e/board.spec.ts
```

**테스트 실패 시 워크플로우:**
1. 에러 출력 분석 → 근본 원인 식별
2. 원인 수정 → 재테스트
3. **모든 테스트가 통과할 때까지 다음 Phase 진행 금지**

---

### Phase 4: Issue Detail Page (이슈 상세)
> `/w/:slug/issues/:identifier` — 댓글 / 멘션 / sub-issue / lineage graph / 우측 작업 콘솔
> Dependencies: Phase 3 (이슈 fixture)

#### Tasks
- [ ] `tests/e2e/issue_detail.spec.ts` 작성
- [ ] 헤더의 `재실행 / 취소 / 삭제` 버튼이 issue/run 상태에 따라 disabled
- [ ] 댓글 textarea에 `@`를 입력하면 `MentionAutocomplete` 드롭다운이 열림 → 키보드 탐색 + 선택
- [ ] XSS 본문(`<script>alert(1)</script>`)이 dialog를 띄우지 않고 plain text로 렌더 (smoke.spec.ts와 동일 보장)
- [ ] sub-issue 폼: 제목/본문 입력 → 생성 → 같은 페이지에 자식 목록 추가
- [ ] lineage graph (`IssueFlowGraph`)에 parent / current / sub / chained run 노드 등장 확인
- [ ] 우측 작업 콘솔: token / cost / model 사용량 표시 (run에 metrics가 있을 때)
- [ ] Run timeline `GET /api/runs/:id/events` 결과가 시간순으로 노출
- [ ] 댓글 64KB 초과 시 `truncated` 표시 + 로그 링크 노출

#### Success Criteria
- 멘션 자동완성은 `Esc` / blur로 닫히고, 선택 시 textarea에 `@AgentName ` 삽입
- sub-issue 생성 후 lineage graph에 즉시 노드가 추가됨 (polling 3초 이내)
- 댓글 삭제 버튼은 system 댓글에서 비활성화

#### Test Cases
- [ ] TC-4.1: 댓글 등록 후 댓글 스레드 가장 아래에 노출, polling 후 사라지지 않음
- [ ] TC-4.2: `@NewsLead 이거 처리해줘` 등록 시 새 run이 댓글 스레드에 system 댓글로 추가
- [ ] TC-4.3: sub-issue 생성 후 부모 페이지에 자식 목록 + lineage 노드 등장
- [ ] TC-4.4: 멘션 autocomplete에서 ArrowDown ↓ ↓ Enter로 두 번째 후보 선택
- [ ] TC-4.5: 64KB 초과 댓글이 `[truncated]` 라벨 + 로그 링크와 함께 표시
- [ ] TC-4.E1: `<script>alert(1)</script>` 본문이 dialog를 띄우지 않음 (기존 smoke 보장 재확인)
- [ ] TC-4.E2: `재실행`을 running 상태에서 클릭하면 버튼이 disabled, 에러 메시지 노출 없음

#### Testing Instructions
```bash
pnpm exec playwright test tests/e2e/issue_detail.spec.ts
```

**테스트 실패 시 워크플로우:**
1. 에러 출력 분석 → 근본 원인 식별
2. 원인 수정 → 재테스트
3. **모든 테스트가 통과할 때까지 다음 Phase 진행 금지**

---

### Phase 5: Agents & Agent Detail (에이전트 관리)
> `/w/:slug/agents` 목록·필터·검색·생성 + `/w/:slug/agents/:id` 편집·승격·삭제
> Dependencies: Phase 2

#### Tasks
- [ ] `tests/e2e/agents.spec.ts` 작성
- [ ] AgentsPage `[+ 새 에이전트]` → `CreateAgentDialog`로 추가 에이전트 생성
- [ ] 필터 segmented (예: `전체 / main / sub`) 및 검색 input 동작 확인
- [ ] AgentDetailPage 폼: 이름 / runtime / model (`ModelSelect`) / instructions / summary / tags / backoff_seconds(`10,60,300`) / max_attempts 저장
- [ ] `메인으로 승격` 버튼: 비-main 에이전트만 활성, 클릭 시 다른 에이전트가 main에서 해제
- [ ] `삭제` 버튼: main 에이전트는 disabled, 일반 에이전트는 ConfirmDialog 후 삭제
- [ ] `instructions` 변경 시 version history가 `GET /api/agents/:id/instructions` 응답으로 늘어남

#### Success Criteria
- 동일 이름(case-insensitive) 중복 생성 시 4xx 에러를 alert에 표시
- backoff_seconds `0,a,300` 같은 잘못된 입력은 저장 거부
- 승격된 에이전트는 사이드바의 `에이전트` 카운트에 영향 없음 (카운트는 total)

#### Test Cases
- [ ] TC-5.1: 신규 에이전트 생성 후 목록 카드에 등장 + `에이전트` 카운트 +1
- [ ] TC-5.2: 검색 `Writer`로 1개만 노출 + URL `?q=Writer` 동기화 (해당 기능이 있을 경우)
- [ ] TC-5.3: 비-main 에이전트를 `메인으로 승격` 후 원래 main이 sub로 강등
- [ ] TC-5.4: backoff_seconds에 `10,60,300` 입력 후 저장 → 페이지 새로고침해도 보존
- [ ] TC-5.5: instructions 수정 → version history 길이 +1
- [ ] TC-5.E1: main 에이전트 삭제 버튼이 disabled
- [ ] TC-5.E2: 동일 이름(소문자/대문자) 에이전트 생성 시 `409` 메시지

#### Testing Instructions
```bash
pnpm exec playwright test tests/e2e/agents.spec.ts
```

**테스트 실패 시 워크플로우:**
1. 에러 출력 분석 → 근본 원인 식별
2. 원인 수정 → 재테스트
3. **모든 테스트가 통과할 때까지 다음 Phase 진행 금지**

---

### Phase 6: Autopilot Page (오토파일럿)
> `/w/:slug/autopilot` — 룰 CRUD + 즉시 트리거 + snooze + 템플릿 + 필터
> Dependencies: Phase 2

#### Tasks
- [ ] `tests/e2e/autopilot.spec.ts` 작성
- [ ] `[+ 자동화 추가]` → `AutopilotDialog`에 이름 / cron / 제목 템플릿 / 담당 에이전트 입력
- [ ] cron 빠른 선택 템플릿 카드 클릭으로 사전 채움
- [ ] 잘못된 cron(`60 25 * * *`) → 4xx 에러 alert
- [ ] 룰 토글 (`ON ↔ OFF`) 후 polling 시 상태 유지
- [ ] `지금 실행` 버튼 → issue 생성 + Board에 노출
- [ ] `편집` / `삭제` 버튼 동작 + ConfirmDialog
- [ ] 필터 segmented (`전체 / 활성 / 일시정지 / 자동 정지`)와 검색 input 동기화
- [ ] 연속 실패 5회 도달 시 룰 자동 OFF 배지 노출

#### Success Criteria
- 트리거 후 3초 이내 새 이슈가 Board에 등장
- snooze된 룰은 `지금 실행` 버튼이 disabled
- 자동 정지된 룰은 `자동 정지` 배지 + 사유 표시

#### Test Cases
- [ ] TC-6.1: cron `0 9 * * *` 정상 입력 후 저장 → 룰 목록에 활성 상태로 등장
- [ ] TC-6.2: `지금 실행` 클릭 → Board에 새 이슈 등장 (제목 템플릿 `{{date}}` 치환 확인)
- [ ] TC-6.3: 룰 OFF 토글 → polling 후에도 OFF 유지
- [ ] TC-6.4: 룰 삭제 → ConfirmDialog 확인 후 목록에서 사라짐
- [ ] TC-6.5: 필터 `일시정지` 선택 시 활성 룰 숨김
- [ ] TC-6.E1: 잘못된 cron 입력 시 다이얼로그가 닫히지 않고 에러 표시
- [ ] TC-6.E2: 5회 연속 실패 후 룰이 자동 OFF + `자동 정지` 배지

#### Testing Instructions
```bash
pnpm exec playwright test tests/e2e/autopilot.spec.ts
```

**테스트 실패 시 워크플로우:**
1. 에러 출력 분석 → 근본 원인 식별
2. 원인 수정 → 재테스트
3. **모든 테스트가 통과할 때까지 다음 Phase 진행 금지**

---

### Phase 7: Settings Page (워크스페이스 + 시스템 설정)
> `/settings` — 워크스페이스 옵션 / DB 백업·정리 / 토큰 / 사용량
> Dependencies: Phase 2

#### Tasks
- [ ] `tests/e2e/settings.spec.ts` 작성
- [ ] 워크스페이스 섹션: 기본 timeout(`default_timeout_seconds`) 변경 후 저장 → 새로고침 시 유지
- [ ] auto-chain 토글 OFF→ON 시 max_depth / daily run limit / daily cost / dry-run 입력 노출
- [ ] auto-chain `dry-run` ON 상태에서 멘션 댓글 → 실제 dispatch 없이 system comment 기록만 (Phase 4와 연결)
- [ ] 시스템 섹션: `DB 백업` 버튼 → 성공 메시지 + 경로 표시
- [ ] `VACUUM` 버튼 → 성공 토스트
- [ ] `로그 정리` 버튼 + `cleanupDays` 입력 → 성공 메시지 (예: `n개 정리됨`)
- [ ] 토큰 입력 → `apiAuth.setToken` 저장 후 다른 페이지 요청에 `Authorization` 헤더 적용
- [ ] 7일/30일 사용량 대시보드 카드가 `/api/usage/summary?days=` 응답대로 렌더

#### Success Criteria
- 시스템 액션 버튼은 진행 중 disabled (`isPending` 상태)
- 잘못된 cleanupDays (음수, 문자) 입력 시 4xx alert
- 토큰 설정 후 다이얼로그/페이지 액션이 정상 동작 (요청에 Authorization 헤더 포함)

#### Test Cases
- [ ] TC-7.1: workspace timeout `600` → `1200` 변경 후 새로고침 시 유지
- [ ] TC-7.2: auto-chain ON 후 dry-run 입력칸 + 가드 칸들이 노출
- [ ] TC-7.3: `DB 백업` 클릭 → 응답 메시지에 백업 경로 포함
- [ ] TC-7.4: `로그 정리` 30일 → 응답 `removed_runs: N` 또는 성공 메시지
- [ ] TC-7.5: 토큰 설정 후 BoardPage 진입 시 정상 로드 (Network 패널에서 `Authorization` 확인)
- [ ] TC-7.6: 사용량 대시보드 카드에 `input_tokens` / `output_tokens` / `cost` 값 표시
- [ ] TC-7.E1: 잘못된 토큰 입력 후 보호 라우트가 401 응답 시 ErrorAlert 노출
- [ ] TC-7.E2: 백업 디렉토리가 없는 절대 경로 입력 시 4xx 메시지 표시

#### Testing Instructions
```bash
pnpm exec playwright test tests/e2e/settings.spec.ts
```

**테스트 실패 시 워크플로우:**
1. 에러 출력 분석 → 근본 원인 식별
2. 원인 수정 → 재테스트
3. **모든 테스트가 통과할 때까지 다음 Phase 진행 금지**

---

### Phase 8: Cross-page Integration & Resilience
> 여러 페이지를 거치는 핵심 사용자 여정 + 장애/에러 처리
> Dependencies: Phase 1~7

#### Tasks
- [ ] `tests/e2e/integration.spec.ts` 작성
- [ ] 시나리오 A — Workspace 생성 → 보조 Agent 추가 → Board 이슈 생성 → IssueDetail에서 `@Writer` 멘션 → 두번째 run 등장
- [ ] 시나리오 B — Autopilot 룰 등록 → `지금 실행` → Board에 신규 이슈 → IssueDetail에서 lineage 노드 확인
- [ ] 시나리오 C — Settings에서 auto-chain ON + dry-run → IssueDetail 멘션 → dispatch 없이 system comment 기록
- [ ] 시나리오 D — Sub-issue 생성 → 부모 lineage graph에서 자식 노드 클릭 → 자식 페이지 진입
- [ ] 회복 시나리오 E — 백엔드 강제 종료 후 polling 실패 → 페이지에 에러/스피너 + 재시작 후 자동 복구
- [ ] 회복 시나리오 F — IssueDetail 진입 중 잘못된 identifier로 이동 → `AppErrorBoundary` fallback 또는 404 메시지
- [ ] 회복 시나리오 G — `<script>` 멘션 자동완성이 dialog를 띄우지 않음 (XSS 방어 재확인)

#### Success Criteria
- 시나리오 A~D는 5분 안에 한 번씩 통과
- 시나리오 E에서 백엔드 복구 후 페이지가 새로고침 없이 정상 동작 (TanStack Query retry)
- ErrorBoundary fallback이 white screen 없이 노출

#### Test Cases
- [ ] TC-8.1 (A): NewsLead → Writer 멘션 → 두번째 run의 agent_id가 Writer
- [ ] TC-8.2 (B): Autopilot 트리거 후 lineage에 `autopilot_rule_id` 메타가 노드에 표시
- [ ] TC-8.3 (C): dry-run 활성 상태에서 멘션 → run table에 새 row 없음 + system comment에 `dry-run skipped` 기록
- [ ] TC-8.4 (D): sub-issue 노드 클릭 → 자식 IssueDetail로 라우팅
- [ ] TC-8.E1 (E): 백엔드 다운 시 `ErrorAlert` + 복구 후 polling 재개
- [ ] TC-8.E2 (F): 잘못된 identifier `NEWS-9999`에 대해 `not found` 안내 노출
- [ ] TC-8.E3 (G): 멘션 자동완성에서 `<script>` 입력 시 dialog 없이 plain text 처리

#### Testing Instructions
```bash
make build
pnpm exec playwright test
```

**테스트 실패 시 워크플로우:**
1. 에러 출력 분석 → 근본 원인 식별
2. 원인 수정 → 재테스트
3. **모든 테스트가 통과할 때까지 다음 Phase 진행 금지**

---

## 5. Integration & Verification (통합 검증)

### 5.1 Integration Test Plan (통합 테스트)
- [ ] 전체 spec 묶음 1회 실행: `pnpm exec playwright test` (8 spec, 약 60+ 케이스)
- [ ] CI(GitHub Actions)에서 `make e2e-full` 실행 — `playwright-report/` artifact 업로드 검토
- [ ] 매트릭: `pass / fail / flaky` 통계 → `test-results/` 산출물 확인
- [ ] 회귀 가드: 새 PR마다 최소 `smoke + integration` spec 통과 의무

### 5.2 Manual Verification Steps (수동 검증)
1. `make build && ./cron-agent-dashboard serve --data-dir /tmp/cad-manual`
2. 브라우저에서 `http://127.0.0.1:8080` 진입 → 워크스페이스 생성
3. Board에서 새 이슈 생성 → IssueDetail에서 댓글/멘션/sub-issue 추가
4. Agents 페이지에서 보조 에이전트 생성 → 메인으로 승격 후 원복
5. Autopilot 룰 등록 → `지금 실행` → Board에 새 이슈 노출 확인
6. Settings에서 백업 / VACUUM / 로그 정리 실행
7. 테마 토글, 워크스페이스 스위처 동작 확인
8. 백엔드 강제 종료 → ErrorAlert → 재시작 후 polling 자동 복구

### 5.3 Rollback Strategy (롤백 전략)
- 신규 spec 파일은 별도 커밋으로 추가하므로 spec 단위 revert로 되돌릴 수 있음
- Playwright config 변경은 `git revert <commit>`로 즉시 회복
- 테스트 실패가 product 코드의 회귀에서 비롯되면 product PR을 revert하고 spec은 유지

---

## 6. Edge Cases & Risks (엣지 케이스 및 위험)

| 위험 요소 | 영향도 | 완화 방안 |
|---|---|---|
| polling 3초로 인한 timing flake | 중간 | `expect().toBeVisible({ timeout: 15_000 })` + retry 1회 허용 |
| missing-runtime이 시간차로 panic할 가능성 | 낮음 | Phase 0 TC-0.E1로 사전 검증 |
| auto-chain dry-run 시나리오에서 system comment 순서 불안정 | 중간 | run_event timeline API로 검증 (`/api/runs/:id/events`) |
| 64KB 댓글 캡 테스트 시 메모리 부담 | 낮음 | 70KB 단일 문자열로 충분 |
| Playwright 헤드리스 환경에서 dialog 차단 동작 차이 | 낮음 | `page.on('dialog', d => d.dismiss())` 명시 |
| 토큰 설정 후 인증 안 풀리는 잔존 상태 | 중간 | `test.afterEach`에서 `localStorage.clear()` 또는 fresh context 사용 |
| `?q=` debounced 검색이 race로 빈 결과 | 낮음 | `page.waitForResponse(/api\/.*issues/)` 명시 대기 |

---

## 7. Execution Rules (실행 규칙)

1. **독립 모듈**: 각 spec 파일은 독립 fixture로 실행하여 다른 Phase에 의존하지 않는다
2. **완료 조건**: 모든 태스크 체크박스 체크 + 모든 테스트 케이스 통과
3. **테스트 실패 워크플로우**: 에러 분석 → 근본 원인 수정 → 재테스트 → 통과 후에만 다음 Phase 진행
4. **Phase 완료 기록**: 체크박스를 체크하여 이 문서에 진행 상황 기록
5. **병렬 실행**: Phase 3 / 5 / 6은 독립이므로 동시 작성·실행 허용
6. **변경 금지 영역**: 기존 `tests/e2e/smoke.spec.ts`는 회귀 보호용으로 유지, 슬림화는 Phase 0 후반에 선택적으로 진행
