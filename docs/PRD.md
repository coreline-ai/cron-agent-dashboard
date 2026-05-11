# PRD — corn-agent-dashboard

> Product Requirements Document
> Version: 0.1
> Date: 2026-05-11
> Status: Draft

---

## 1. Vision (비전)

**"내가 CLI 에이전트에게 작업을 시키고 결과를 한 곳에 추적할 수 있는 가장 가벼운 도구."**

기존 Multica는 팀/조직을 가정한 풀스택 시스템이라 단일 사용자에게는 과한 토큰 소비와 복잡도가 발생한다.
corn-agent-dashboard는 Multica의 검증된 UX(보드/댓글/위임)를 보존하면서 단일 사용자에게 불필요한 모든 기능을 제거한다.

---

## 2. Problem Statement (해결할 문제)

### 2.1 현재 Multica의 문제 (single user 관점)

| 문제 | 영향 |
|---|---|
| 멤버/Inbox/Subscriber/WS Hub 등 멀티 유저 기능 | 항상 켜져 있음 → 토큰/메모리 낭비 |
| Daemon + Desktop + Backend + Frontend 4 프로세스 | 운영 복잡, dev memory leak (Turbopack 6.1GB cache 사례) |
| Postgres + Docker compose 5 services | 가벼운 실험에 과한 인프라 |
| i18n 21 namespace × 3 locale | 단일 한국어 사용자에게 불필요 |
| Autopilot/Quick-Create/PAT 등 다층 인증 흐름 | 단일 사용자엔 진입장벽만 |

### 2.2 단일 사용자가 진짜 원하는 것

1. **작업 지시**: "오늘 뉴스 정리해줘" 한 줄 입력
2. **자동 실행**: 메인 에이전트가 자동으로 작업 수행
3. **결과 추적**: stdout/결과물을 시간순으로 댓글로 확인
4. **위임 체인**: 결과가 만족스러우면 다음 에이전트에게 넘김
5. **반복 자동화**: 매일 9시처럼 정기 작업
6. **워크스페이스 분리**: 뉴스/코딩/문서 작업을 분리해서 관리

---

## 3. Target User (대상 사용자)

### 3.1 Primary
- **본인 1명** (single-user, single-machine)
- 한국어 사용자, macOS/Linux
- CLI 도구(codex/claude/gemini) 기 설치
- AI 에이전트로 반복 작업 자동화에 관심

### 3.2 Non-target
- 팀/조직 사용 ❌
- 모바일 ❌
- 비CLI 사용자 ❌
- 클라우드 멀티테넌트 ❌

---

## 4. Goals (목표)

### 4.1 Must-have (M)
- M1. **워크스페이스 CRUD** — 여러 작업 영역 분리
- M2. **메인 에이전트 + 추가 에이전트** — 워크스페이스당 메인 1개 필수, 추가 N개 선택
- M3. **이슈(작업) 트래커** — 제목/본문/상태/담당 에이전트, 자동 식별자(`NEWS-12`)
- M4. **에이전트 실행 → 댓글 결과 기록** — stdout을 시간순 댓글로 기록
- M5. **`@AgentName` 멘션 위임** — 댓글에서 다른 에이전트 트리거 → **같은 이슈에 새 run 추가**
- M6. **Autopilot (cron 자동 이슈 생성)** — 정기 작업 자동화
- M7. **현재 Multica 테마/스타일 유지** — 다크모드, 한국어, shadcn 컴포넌트
- M8. **단일 바이너리 + SQLite 파일** — 외부 의존 0
- M9. **재실행/취소** — 실패한 이슈 rerun, 진행 중 cancel (cancel은 `cancelled` 상태로 별도 표시)
- M10. **Durable queue** — 프로세스 재시작 후에도 queued 이슈가 자동 재개됨 (DB-backed)

### 4.2 Should-have (S)
- S1. **이슈 검색** — 제목/본문 텍스트 검색
- S2. **상태 필터** — queued/running/done/failed 별 보기
- S3. **에이전트별 필터** — 특정 에이전트 작업만 보기
- S4. **로그 다운로드** — 개별 run의 stdout 전체 다운로드
- S5. **수동 트리거** — Autopilot 룰을 "지금 실행" 버튼으로 즉시 실행

### 4.3 Won't (W) — 명시적 제외
- W1. ❌ 멤버/팀/조직 (멀티 유저 가정 일체)
- W2. ❌ Inbox / Subscriber / 알림 채널
- W3. ❌ 데스크톱 앱 (Electron) — 웹 only
- W4. ❌ WebSocket 실시간 (polling으로 충분)
- W5. ❌ Daemon 별도 프로세스 (백엔드 단일 프로세스에 통합)
- W6. ❌ 다국어 (한국어 only, 영어 기본 fallback 정도)
- W7. ❌ OAuth/PAT/Daemon Token 다중 인증 — 무인증 또는 단일 토큰
- W8. ❌ Priority/Label/Attachment (Phase 2 후보)
- W9. ❌ pgvector/embedding/semantic search
- W10. ❌ **부모/서브 이슈 트리** (Phase 2 후보) — MVP에서 위임 체인은 같은 이슈의 댓글/run 누적으로 표현. `parent_issue_id` 컬럼은 스키마에 예약하되 UI/API 노출은 Phase 2.

---

## 5. User Stories (사용자 스토리)

### US-1. 작업 지시
> "AI 뉴스" 워크스페이스의 보드에서 `[+ 새 이슈]`를 누르고 "오늘 r/MachineLearning 상위 5개 정리해줘" 라고 입력하면, 메인 에이전트 NewsLead가 자동으로 실행되어 결과를 댓글로 남긴다.

**수용 기준**:
- 이슈 생성 후 3초 이내에 **시스템 댓글로 "NewsLead 실행을 시작했습니다"** 표시 (실행 시작 신호)
- 결과 댓글은 에이전트 실행 종료 시점에 1회 INSERT하되, run log는 10MB까지 파일로 보존하고 comment.content는 64KB cap 적용 + 전체 로그 링크를 제공
- 완료되면 status가 `done`으로 자동 변경
- 결과 댓글에 markdown 표/링크/코드 블록이 렌더링됨

### US-2. 위임 체인
> NewsLead의 결과 댓글 아래에 사용자가 "@Writer 이걸로 블로그 글 써줘" 라고 댓글을 달면 Writer 에이전트가 **같은 이슈에서** 자동 실행되어 다음 댓글에 결과를 남긴다.

**수용 기준**:
- 댓글 본문의 `@Writer` (첫 멘션만) 을 백엔드가 파싱 → **같은 이슈에 새 run 생성** (sub-issue 아님)
- **이슈의 `assignee_agent_id`는 변경되지 않음** (멘션은 일회성 위임). UI는 "담당: NewsLead / 최근 실행: Writer" 분리 표시.
- 멘션 매칭은 대소문자 무시 (`@writer`도 동일)
- 동일 이슈 내에서 댓글 순서가 보존됨 (시간순)
- 같은 댓글에 멘션이 둘 이상이면 첫 번째만 dispatch + "추가 멘션 무시" 경고
- 멘션된 에이전트가 워크스페이스에 없으면 댓글은 저장, 시스템 경고 댓글 추가
- 이미 같은 (이슈, 에이전트)에 queued run이 있으면 중복 dispatch 차단 + 경고

### US-3. 정기 작업
> Autopilot 페이지에서 "매일 09:00, @NewsLead, 제목=`{{date}} AI 뉴스`" 룰을 만들어두면, 매일 09:00에 자동으로 이슈가 생성되고 실행된다.

**수용 기준**:
- cron 표현식 입력 또는 preset(매시간/매일/매주)
- `enabled` 토글로 즉시 on/off
- `[지금 실행]` 버튼으로 수동 트리거 가능
- 다음 실행 시각이 룰 카드에 표시됨

### US-4. 결과 회수
> 어제 만든 이슈 `NEWS-12`를 다시 열어서 댓글 스레드에서 결과를 그대로 확인할 수 있다.

**수용 기준**:
- 이슈 상세 페이지가 댓글 영속 (저장된 SQLite + run stdout 파일)
- URL이 안정적 (`/w/:slug/issues/NEWS-12`) — 북마크 가능

### US-5. 실패 재시도
> 어떤 이슈의 마지막 run이 `failed`로 끝나면 사이드바에서 `[재실행]` 버튼으로 **마지막 실행한 에이전트**에게 다시 보낼 수 있다.

**수용 기준**:
- [재실행] 대상은 **가장 최근 run의 agent** (예: 마지막이 멘션으로 Writer가 실행했다면 Writer로). issue.assignee_agent_id가 NewsLead여도 마찬가지.
- 명시적으로 다른 agent로 재실행하려면 모달에서 선택 (Phase 1에서는 마지막 agent 자동, 선택 UI는 Phase 2 후보)
- rerun 시 새로운 `run` row 생성 (`trigger_type='rerun'`, 기존 run 보존)
- 댓글 스레드는 누적 (이전 시도 결과 그대로 유지)
- 동일 이슈에서 run 1, run 2, run 3 식별 가능
- 이슈가 `done`이었어도 rerun 시 `open`으로 자동 되돌림

---

## 6. Out of Scope (범위 외)

다음은 명시적으로 이 프로젝트에서 **하지 않는다**:

| 항목 | 이유 |
|---|---|
| 멀티 사용자 권한/RBAC | 단일 사용자 가정 |
| 클라우드 호스팅 / SaaS | 로컬 머신 전용 |
| 모바일 앱 | 작업 환경 = 데스크톱 |
| 실시간 협업 (cursor share 등) | 단일 사용자 |
| 첨부 파일 업로드 | Phase 1 제외 (필요 시 v2) |
| 에이전트 마켓플레이스 | 사용자가 직접 instructions 작성 |
| 결제/구독 | 단일 사용자 도구 |
| 분석 대시보드 | 로그는 SQLite에서 직접 쿼리 |

---

## 7. Success Criteria (성공 기준)

### 7.1 정량적
- **콜드 부팅**: 단일 바이너리 실행 → UI 접속 가능까지 **3초 이내**
- **메모리**: 백엔드 idle 시 RAM **100MB 이하**
- **응답 시간**: API p95 **100ms 이하** (SQLite 로컬)
- **코드 규모**: Go ≤ 1,500 LOC, Frontend ≤ 3,000 LOC
- **외부 의존 서비스 수**: **0** (Postgres/Redis/Docker 없음)

### 7.2 정성적
- 본인이 매일 1회 이상 자연스럽게 사용하게 된다
- "Multica 켤까 말까" 고민 없이 바로 켜진다
- Autopilot 룰을 설정해두고 일주일 동안 정상 동작

---

## 8. Open Questions (열린 질문)

> 개발 진행하면서 결정해야 할 사항

1. **인증**: 완전 무인증 vs 단일 토큰(`CORN_AGENT_DASHBOARD_TOKEN`)? → 기본 무인증, 토큰은 옵션
2. **에이전트 결과 markdown 렌더링**: 기존 Multica에서 추출 vs `react-markdown` 신규? → 추출
3. **Cron preset 종류**: 매시간/매일/매주/매월/Cron직접 — 어디까지 제공? → 우선 4개 + advanced
4. **이슈 자동 보관**: done 후 7일 지나면 archive? → Phase 2 결정
5. **에이전트 instructions 버전 관리**: 변경 이력 보존? → Phase 1 제외
6. **bot.py 같은 외부 봇 통합**: REST API로 외부 프로세스가 작업 dispatch 가능해야 하는가? → API는 처음부터 외부 호출 가능하게 설계 (CORS/토큰)

---

## 9. Glossary (용어)

| 용어 | 정의 |
|---|---|
| **워크스페이스** | 작업 도메인 분리 단위 (예: "AI 뉴스", "코딩 보조") |
| **메인 에이전트** | 워크스페이스당 1개, 새 이슈의 기본 담당 |
| **이슈** | 작업 단위. 제목 + 본문 + 상태 + 담당 에이전트 + 댓글 스레드 |
| **댓글** | 이슈에 달리는 시간순 메시지. 작성자는 사용자 또는 에이전트 |
| **Run** | 이슈에 대한 1회 에이전트 실행. **status (queued/running/done/failed/cancelled)** + trigger_type + exit code + stdout 파일 보유. Durable queue의 단위. |
| **Autopilot 룰** | cron 기반 자동 이슈 생성 규칙. 시스템 timezone(기본 `Asia/Seoul`) 기준. |
| **멘션** | 댓글 내 `@AgentName` — 같은 이슈에 새 run 트리거. 첫 멘션만 사용. **이슈의 assignee는 변경되지 않음**. |
| **Identifier** | 사람이 읽는 이슈 ID (예: `NEWS-12`). 워크스페이스 prefix + 순번 |
| **Durable queue** | 큐를 메모리 channel이 아니라 `run` 테이블의 `status='queued'` 행으로 표현. 재시작 후 worker가 다시 claim. |
| **issue.status** | 사용자 의도 (`open / done / cancelled`). 실행 상태와 분리. |
| **execution_status** | API 응답에서 derived. 가장 최근 run의 상태 (`running / queued / done / failed / cancelled / idle`). |
| **assignee_agent_id** | 이슈의 기본 담당. 멘션이 들어와도 변경되지 않음. |
| **last_run_agent_id** | 가장 최근 run의 agent. UI에서 "최근 실행"으로 표시. [재실행] 대상. |
| **워크스페이스 직렬화** | MVP는 같은 워크스페이스 안에서 동시 1개 run만 실행 (working_dir 충돌 방지). 다른 워크스페이스끼리는 병렬. |
