<div align="center">

# 🧭 Corn Agent Dashboard

**혼자 쓰는 AI 에이전트 작업 트래커**
CLI 에이전트(`codex` · `claude` · `gemini`)에게 작업을 지시하고, 결과를 댓글로 추적하고, 정기 작업은 Autopilot으로 자동화한다.

[![Status](https://img.shields.io/badge/status-local%20MVP%20integrated-brightgreen?style=for-the-badge)](docs/ROADMAP.md)
[![License](https://img.shields.io/badge/license-MIT-green?style=for-the-badge)](#-라이선스)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-lightgrey?style=for-the-badge)](#requirements)
[![Single Binary](https://img.shields.io/badge/deploy-single%20binary-blueviolet?style=for-the-badge)](#)

[![Go](https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![SQLite](https://img.shields.io/badge/SQLite-3-003B57?style=flat-square&logo=sqlite&logoColor=white)](https://www.sqlite.org/)
[![chi](https://img.shields.io/badge/router-chi%2Fv5-007ACC?style=flat-square)](https://github.com/go-chi/chi)
[![sqlx](https://img.shields.io/badge/orm-sqlx-336791?style=flat-square)](https://github.com/jmoiron/sqlx)
[![Vite](https://img.shields.io/badge/Vite-5-646CFF?style=flat-square&logo=vite&logoColor=white)](https://vitejs.dev/)
[![React](https://img.shields.io/badge/React-18-61DAFB?style=flat-square&logo=react&logoColor=white)](https://react.dev/)
[![React Router](https://img.shields.io/badge/React%20Router-7-CA4245?style=flat-square&logo=reactrouter&logoColor=white)](https://reactrouter.com/)
[![TanStack Query](https://img.shields.io/badge/TanStack%20Query-5-FF4154?style=flat-square&logo=reactquery&logoColor=white)](https://tanstack.com/query)
[![CSS](https://img.shields.io/badge/CSS-dark%20skeleton-64748B?style=flat-square)](#)
[![Tailwind/shadcn](https://img.shields.io/badge/Tailwind%2Fshadcn-planned-38B2AC?style=flat-square)](#)

[![codex](https://img.shields.io/badge/agent-codex-FF6B6B?style=flat-square)](#)
[![claude](https://img.shields.io/badge/agent-claude-D97757?style=flat-square)](#)
[![gemini](https://img.shields.io/badge/agent-gemini-4285F4?style=flat-square&logo=google&logoColor=white)](#)

</div>

---

## ✨ 한눈에 보기

```text
┌────────────────────────────────────────────────────────────────┐
│  AI 뉴스 큐레이션 (워크스페이스)                                  │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  [NEWS-15] 오늘 뉴스 정리           🔵 실행 중 · @NewsLead       │
│  [NEWS-14] 주말 모아보기             🟢 완료   · @Publisher      │
│  [NEWS-13] 어제 실패한 작업           🔴 실패   · @NewsLead       │
│                                                                │
│  [+ 새 이슈]                                                    │
│                                                                │
├────────────────────────────────────────────────────────────────┤
│  ⏰ Autopilot: 매일 09:00 → NewsLead         ON · 다음 09:00     │
│  ⏰ Autopilot: 매주 일요일 18:00 → Publisher  OFF                │
└────────────────────────────────────────────────────────────────┘
```

> [!IMPORTANT]
> **이 프로젝트는 현재 로컬 MVP 통합 완료 단계입니다.** Go SQLite/API, worker/store/main 실행 연결, DB-backed Autopilot scheduler, Vite React read/write UI, Go `embed.FS` 단일 바이너리 서빙, CLI backup/restore, startup self-check, Playwright browser smoke, clean clone 검증 스크립트, GitHub Release 업로드 workflow까지 구현되었고 `make check` / `make e2e-smoke` / `make release-build`로 자체 검증합니다.

---

## 📑 목차

- [왜 만드는가](#-왜-만드는가)
- [핵심 가치](#-핵심-가치)
- [기능](#-기능)
- [아키텍처](#-아키텍처)
- [기술 스택](#-기술-스택)
- [빠른 시작](#-빠른-시작)
- [사용법](#-사용법)
- [설정](#-설정)
- [문서](#-문서)
- [로드맵](#-로드맵)
- [디자인 원칙](#-디자인-원칙)
- [기여](#-기여)
- [라이선스](#-라이선스)

---

## 🎯 왜 만드는가

기존 **Multica**는 팀/조직을 가정한 풀스택 시스템(Postgres + Daemon + Frontend + Desktop)이라 **단일 사용자에게는 토큰 낭비 + 운영 복잡도**가 발생한다.

**Corn Agent Dashboard**는 Multica의 검증된 UX(보드 / 댓글 / 멘션 위임)는 보존하면서 단일 사용자에게 불필요한 모든 기능을 제거한 **경량 단일 바이너리** 버전이다.

| 항목 | Multica | Corn Agent Dashboard |
|---|---|---|
| 프로세스 | Postgres + Daemon + Frontend + Desktop = **5** | **1** (단일 Go 바이너리) |
| DB | PostgreSQL 17 + pgvector | **SQLite 1 파일** |
| 인프라 | `docker-compose` × 5 services | **없음** |
| Frontend | Next.js 16 (SSR/RSC) | **Vite + React Router SPA** |
| Realtime | WebSocket Hub | **HTTP polling** (3s) |
| 인증 | OAuth + PAT + Daemon Token | **무인증** (옵션: 단일 토큰) |
| 멤버 / 권한 | RBAC + invite | **단일 사용자** |
| Go 코드 규모 | ~25,000 LOC | **목표 ≤ 1,500 LOC** |
| Frontend 규모 | 40+ 페이지 | **7 페이지** |

---

## 💎 핵심 가치

<table>
<tr>
<td width="33%" valign="top">

### 📋 작업 트래커
이슈 만들기 → 자동 실행 → 결과 댓글로 영구 기록.
사람이 읽는 식별자 `NEWS-12`로 북마크 가능.

</td>
<td width="33%" valign="top">

### 🔗 멘션 위임
`@Writer 이걸로 블로그 글 써줘` 한 줄이면 다음 에이전트로 위임.
같은 이슈에 시간순 누적.

</td>
<td width="33%" valign="top">

### ⏰ Autopilot
cron 기반 정기 이슈 자동 생성.
`매일 09:00 → NewsLead → 뉴스 정리`.

</td>
</tr>
<tr>
<td width="33%" valign="top">

### 🚀 단일 바이너리
`./corn-agent-dashboard serve` 한 줄.
SQLite 파일 옆에 둠. 외부 의존 0.

</td>
<td width="33%" valign="top">

### 💾 Durable Queue
DB-backed 큐. 프로세스 재시작에도 작업 손실 없음.

</td>
<td width="33%" valign="top">

### 🎨 한국어 다크모드
Multica의 검증된 디자인 토큰 그대로.
shadcn/ui · Tailwind v4 · 다크모드.

</td>
</tr>
</table>

---

## ✅ 기능

### MVP (Phase 1)

- [x] 📐 설계 완료 — PRD / TRD / ARCHITECTURE / DATA_MODEL / API / UX / ROADMAP
- [x] 🧱 초기 구현 스캐폴드 — Go module / SQLite migration / REST API skeleton / worker·scheduler skeleton / Vite React routes
- [x] ⚙️ Worker/Scheduler 연결 — queued run claim/execution 반영, active cancel 요청, DB-backed Autopilot reload
- [x] 🗂️ 워크스페이스 생성/조회/수정/삭제 API + 생성 UI (이슈 ID prefix · 기본 working_dir)
- [x] 🤖 메인 에이전트 1개 + 추가 에이전트 N개 CRUD UI/API (case-insensitive 이름 유일)
- [x] 🎫 이슈 트래커 (`identifier` `NEWS-12`, status: `open` / `done` / `cancelled`)
- [x] 💬 댓글 스레드 + system 댓글 (실행 시작 / 취소 / 경고)
- [x] 🔀 `@AgentName` 멘션 위임 — **assignee는 바꾸지 않음**, 같은 이슈에 새 run 추가
- [x] ⏰ Autopilot (robfig/cron, 시스템 timezone `Asia/Seoul` 기본)
- [x] 🔁 [재실행] — 마지막 run의 agent로 자동 dispatch
- [x] 🛑 [취소] — process group SIGTERM → 30초 후 SIGKILL
- [ ] 💾 Durable queue (`run.status='queued'` row 기반 DB claim)
- [ ] 🔐 옵션 토큰 인증 (`CORN_AGENT_DASHBOARD_TOKEN`)
- [ ] 📦 단일 바이너리 (Go embed.FS로 Vite SPA 포함)

### Phase 2+

- [ ] 🌲 부모 / 서브 이슈 트리
- [ ] 📎 첨부 파일
- [ ] 🌳 per-run worktree (워크스페이스 동시 실행 활성화)
- [ ] 📤 워크스페이스 import/export
- [ ] 🔔 외부 webhook

---

## 🧱 아키텍처

### 시스템 구성

```
┌────────────────────────────────────────────────────────────────┐
│                  Browser (127.0.0.1:8080)                      │
└────────────────────────────┬───────────────────────────────────┘
                             │ HTTP
                             ▼
┌────────────────────────────────────────────────────────────────┐
│        corn-agent-dashboard binary (single process)            │
│                                                                │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │              HTTP Server (chi router)                    │  │
│  │  /api/*  →  REST API (33 endpoints)                      │  │
│  │  /*      →  embed.FS (Vite SPA + index.html fallback)    │  │
│  └────────────────────┬─────────────────────┬───────────────┘  │
│                       │                     │                  │
│                       ▼                     ▼                  │
│         ┌─────────────────────┐  ┌───────────────────────┐     │
│         │   Store (sqlx)      │  │  Worker Pool (N=3)    │     │
│         │   ─ CRUD            │  │  ─ DB claim polling   │     │
│         │   ─ Transactions    │  │  ─ Runtime adapter    │     │
│         └──────────┬──────────┘  │  ─ Process group kill │     │
│                    │             └───────────┬───────────┘     │
│                    │                         │                 │
│                    ▼                         ▼                 │
│         ┌─────────────────────────────────────────────┐        │
│         │           SQLite (data.db, WAL)             │        │
│         │  workspace · agent · issue · comment ·      │        │
│         │  run · autopilot_rule · schema_migrations   │        │
│         └─────────────────────────────────────────────┘        │
│                                                                │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │       Cron Scheduler (robfig/cron, in-process)           │  │
│  │  ─ Autopilot 룰 등록 / 시각 도래 시 issue + run INSERT    │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────┬──────────────────────────────────┘
                              │ exec.Command (process group)
                              ▼
                ┌─────────────────────────────┐
                │  External CLI agents (PATH) │
                │   codex / claude / gemini   │
                └─────────────────────────────┘
```

### 상태 분리 모델

```
┌──────────────────────────────┐    ┌──────────────────────────────────────────┐
│  issue.status (사용자 의도)   │    │  run.status (실행 상태)                   │
│  open · done · cancelled     │    │  queued · running · done · failed ·       │
│                              │    │  cancelled                                │
└──────────────────────────────┘    └──────────────────────────────────────────┘
                ↕
     execution_status = derived from latest run
            (API 응답에서만 계산)
```

자세한 흐름은 [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

---

## 🛠️ 기술 스택

<table>
<tr><th>영역</th><th>기술</th><th>이유</th></tr>
<tr>
<td>Backend 언어</td>
<td><img src="https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat&logo=go&logoColor=white"/></td>
<td>단일 정적 바이너리, 가벼운 동시성</td>
</tr>
<tr>
<td>HTTP 라우터</td>
<td><img src="https://img.shields.io/badge/chi-v5-007ACC?style=flat"/></td>
<td>Multica에서 검증, 미들웨어 친화</td>
</tr>
<tr>
<td>DB 드라이버</td>
<td><img src="https://img.shields.io/badge/modernc.org%2Fsqlite-pure%20Go-003B57?style=flat&logo=sqlite&logoColor=white"/></td>
<td>CGo 없음 → 크로스 컴파일 단순</td>
</tr>
<tr>
<td>SQL 매핑</td>
<td><img src="https://img.shields.io/badge/sqlx-336791?style=flat"/></td>
<td>1.5k LOC에 sqlc는 과함</td>
</tr>
<tr>
<td>Cron</td>
<td><img src="https://img.shields.io/badge/robfig%2Fcron-v3-0B5394?style=flat"/></td>
<td>in-process · timezone 지원</td>
</tr>
<tr>
<td>Frontend 빌드</td>
<td><img src="https://img.shields.io/badge/Vite-5-646CFF?style=flat&logo=vite&logoColor=white"/></td>
<td>SPA + static export, Next의 RSC 회피</td>
</tr>
<tr>
<td>UI 라이브러리</td>
<td><img src="https://img.shields.io/badge/Plain%20CSS-skeleton-64748B?style=flat"/> <img src="https://img.shields.io/badge/Tailwind%2Fshadcn-planned-38B2AC?style=flat"/></td>
<td>초기 UI는 다크모드 CSS skeleton, 후속 Phase에서 Multica 디자인 토큰 이식</td>
</tr>
<tr>
<td>상태 관리</td>
<td><img src="https://img.shields.io/badge/TanStack%20Query-5-FF4154?style=flat&logo=reactquery&logoColor=white"/></td>
<td>polling · 캐시 · 낙관적 업데이트</td>
</tr>
<tr>
<td>라우팅</td>
<td><img src="https://img.shields.io/badge/React%20Router-7-CA4245?style=flat&logo=reactrouter&logoColor=white"/></td>
<td>SPA 동적 라우트</td>
</tr>
<tr>
<td>Markdown</td>
<td><img src="https://img.shields.io/badge/react--markdown%20%2B%20GFM-safe-9C27B0?style=flat"/></td>
<td>`react-markdown` + `remark-gfm` 렌더링. raw HTML/script 실행 금지</td>
</tr>
<tr>
<td>Storage</td>
<td><img src="https://img.shields.io/badge/SQLite-3%20(WAL)-003B57?style=flat&logo=sqlite&logoColor=white"/></td>
<td>1 파일, 무외부 의존</td>
</tr>
</table>

---

## 🚀 빠른 시작

> [!NOTE]
> 로컬에서는 `make build`로 단일 바이너리를 만들 수 있고, GitHub tag `v*.*.*` push 또는 수동 workflow dispatch로 Release artifact 업로드가 자동화됩니다.

### Requirements

- macOS 13+ 또는 Linux (Ubuntu 22.04+)
- `codex` 또는 `claude` 또는 `gemini` CLI 중 1개 이상 PATH에 설치됨

### 설치 (예정)

```bash
# Homebrew (예정)
brew install coreline-ai/tap/corn-agent-dashboard

# 또는 직접 다운로드
curl -L https://github.com/coreline-ai/corn-agent-dashboard/releases/latest/download/corn-agent-dashboard-$(uname -s)-$(uname -m) \
  -o /usr/local/bin/corn-agent-dashboard
chmod +x /usr/local/bin/corn-agent-dashboard
```

### 초기화 + 실행

```bash
# 의존성 설치
pnpm install

# 데이터 디렉토리 초기화 (~/.corn-agent-dashboard/)
go run ./cmd/corn-agent-dashboard init

# 단일 바이너리 빌드 (web/dist embed 포함)
make build

# 백엔드 + 내장 UI 서버 시작
./corn-agent-dashboard serve

# → http://127.0.0.1:8080 접속
# 부팅 시 DB pragma / foreign key / main agent invariant / orphan run recovery self-check가 실행됩니다.
```


### 검증 명령

```bash
# Go + frontend + embedded static build gate
make check

# 단일 바이너리 기반 브라우저 smoke: workspace → issue → detail → comment
make e2e-smoke

# fresh copy에서 README quick start 재현
make verify-clean-clone

# 릴리스 artifact 생성
make release-build VERSION=v0.1.0
```

### 프론트엔드 개발 서버

```bash
pnpm --filter web dev
# → http://127.0.0.1:5173
# Vite dev server는 /api, /healthz를 127.0.0.1:8080으로 프록시합니다.
```

### Shell alias 제안

binary 이름이 길어서 alias 권장:

```bash
# ~/.zshrc 또는 ~/.bashrc
alias cad='corn-agent-dashboard'

# 이후
cad serve --workers 3 --timezone Asia/Seoul
```

---

## 📖 사용법

### 1. 워크스페이스 + 메인 에이전트 생성

```
[+ 새 워크스페이스]
  이름:        AI 뉴스 큐레이션
  슬러그:       ai-news
  이슈 prefix:  NEWS
  ───── 메인 에이전트 ─────
  이름:        NewsLead
  런타임:       codex
  지시문:       Reddit r/MachineLearning에서 오늘 핫한 5개 정리...
```

### 2. 이슈 만들기 → 자동 실행

`/w/ai-news/board` 에서 `[+ 새 이슈]`:

```
제목:  오늘 뉴스 정리해줘
본문:  (선택)
담당:  NewsLead (기본)
```

→ 즉시 dispatch. 3초 이내 system 댓글로 "NewsLead 실행을 시작했습니다" 표시.
종료 시 결과 댓글로 markdown 결과 INSERT.

### 3. 멘션 위임

NewsLead 결과 아래 댓글:

```markdown
@Writer 이걸로 블로그 글 써줘
```

→ Writer가 같은 이슈에 새 run으로 실행. 댓글 스레드에 누적.

> [!TIP]
> 멘션은 이슈의 **담당자를 바꾸지 않습니다**. 일회성 위임일 뿐이며, [재실행] 버튼은 가장 최근 run의 agent로 동작합니다.

### 4. Autopilot 룰

`/w/ai-news/autopilot` → `[+ 자동화 추가]`:

```
이름:    매일 09:00 뉴스
주기:    매일 [09:00]
제목:    {{date}} AI 뉴스 정리
담당:    NewsLead
```

매일 09:00에 이슈 자동 생성 + 즉시 실행.

### 5. 결과 회수

이슈 상세 페이지에서 댓글 스레드로 결과 확인. 큰 결과(64KB 초과)는 본문에 일부 + "전체 로그는 [로그 보기](...)" 링크.

---

## ⚙️ 설정

### CLI 플래그

| 플래그 | 환경변수 | 기본값 | 설명 |
|---|---|---|---|
| `--db` | `CORN_AGENT_DASHBOARD_DB` | `~/.corn-agent-dashboard/data.db` | SQLite 파일 경로 |
| `--bind` | `CORN_AGENT_DASHBOARD_BIND` | `127.0.0.1:8080` | HTTP 바인딩 |
| `--workers` | `CORN_AGENT_DASHBOARD_WORKERS` | `3` | 전역 worker pool 크기 |
| `--timezone` | `CORN_AGENT_DASHBOARD_TIMEZONE` | `Asia/Seoul` | Autopilot cron timezone |
| `--token` | `CORN_AGENT_DASHBOARD_TOKEN` | (없음) | 단일 토큰 인증 (옵션) |
| `--cors` | `CORN_AGENT_DASHBOARD_CORS` | (없음) | 추가 허용 origin (콤마 구분) |
| `--to` | — | 자동 `.bak` 경로 | `backup` 명령의 백업 파일 경로 |
| `--from` | — | (필수) | `restore` 명령의 복구 원본 DB 경로 |

### 데이터 디렉토리 구조

```
~/.corn-agent-dashboard/
├── data.db                # SQLite (모든 메타데이터)
├── runs/
│   ├── <run-id>.log       # 각 run의 stdout (최대 10MB)
│   └── ...
├── workdirs/
│   └── <workspace-slug>/  # 에이전트 실행 cwd (자동 생성)
└── config.toml            # (선택)
```

### 백업 / 복구

```bash
# 백업 (UI에서도 가능: /settings → [DB 백업])
corn-agent-dashboard backup --to ~/backup/data.db.$(date +%Y%m%d)

# 복구 전 기존 DB는 data.db.pre-restore-<timestamp>로 자동 보존
corn-agent-dashboard restore --from ~/backup/data.db.20260512
corn-agent-dashboard serve   # 마이그레이션 자동 적용
```

---

## 📚 문서

| 문서 | 내용 |
|---|---|
| [📋 PRD](docs/PRD.md) | 제품 요구사항 — vision · 사용자 스토리 · M/S/W 목표 · 성공 기준 |
| [⚙️ TRD](docs/TRD.md) | 기술 요구사항 — 스택 · Runtime adapter · durable queue · 워크스페이스 직렬화 |
| [🧱 ARCHITECTURE](docs/ARCHITECTURE.md) | 컴포넌트 · 데이터 흐름 · 상태머신 (`issue.status` vs `run.status` 분리) |
| [🗃️ DATA MODEL](docs/DATA_MODEL.md) | 6 도메인 + 1 메타 테이블 DDL · claim 쿼리 · 트랜잭션 패턴 |
| [🔌 API](docs/API.md) | 33 REST 엔드포인트 · 멘션 규칙 · identifier resolve |
| [🎨 UX FLOW](docs/UX_FLOW.md) | 7 페이지 화면 · 배지 · 사이드바 · empty state |
| [🗺️ ROADMAP](docs/ROADMAP.md) | Phase 0~7 · 의존성 · 리스크 |
| [✅ TODO](TODO.md) | scaffold 이후 남은 항목 · P0/P1/P2/P3 우선순위 |
| [🧩 후속 개발 계획](dev-plan/implement_20260512_180648.md) | worker/scheduler/frontend/embed/release 남은 작업 분해 |
| [🤝 CLAUDE.md](CLAUDE.md) | LLM 코딩 어시스턴트용 작업 가이드라인 |

---

## 🗺️ 로드맵

```
Phase 0  ─▶ Phase 1 ─▶ Phase 2 ─┬─▶ Phase 5 ─▶ Phase 6 ─▶ Phase 7
                                │
                                ├─▶ Phase 3 (병렬)
                                │
                                └─▶ Phase 4 (병렬)
```

| Phase | 목표 | 산출물 | 예상 일수 | 상태 |
|---|---|---|---|---|
| **P0** | 프로젝트 셋업 + 핵심 의사결정 | go mod · Vite skeleton · CI · RSC 스캔 | 1~2 | ✅ 진행 완료 |
| **P1** | 백엔드 코어 | DB + store + REST API skeleton | 3~5 | ✅ 기반 구현 |
| **P2** | 에이전트 실행 | Runtime adapter · worker pool · 멘션 파싱 | 2~3 | ✅ 주요 연결 완료 |
| **P3** | Autopilot | cron + 룰 CRUD + 수동 트리거 | 1~2 | ✅ 주요 연결 완료 |
| **P4** | Frontend | 7 페이지 (Vite + React Router SPA) | 5~7 | ✅ read/write UI 연결 |
| **P5** | 통합 / 임베드 | static export + embed.FS + 단일 바이너리 | 1 | ✅ 구현 완료 |
| **P6** | 품질 / 운영 | 부팅 자가검진 · 백업 / 복구 · 성능 검증 | 2 | ✅ 주요 구현 완료 |
| **P7** | 릴리스 | 크로스 컴파일 · README 스크린샷 · 데모 | 1 | ✅ Release CI/검증 자동화 완료 |

**총 예상**: 16~22일 (혼자, 풀타임 기준)

상세는 [docs/ROADMAP.md](docs/ROADMAP.md).

---

## 🎯 디자인 원칙

> [!IMPORTANT]
> 이 프로젝트가 망가지지 않게 하는 핵심 원칙들. 구현 시 우선 참고.

1. **단일 사용자 가정** — 멤버 / 권한 / 멀티테넌트는 명시적 제외 (W1~W10)
2. **`issue.status` ≠ `run.status`** — 사용자 의도와 실행 상태는 별도 테이블 · `execution_status`는 API derived
3. **Durable queue** — channel 큐 사용 금지. 모든 작업은 `run.status='queued'` row로 표현
4. **워크스페이스당 동시 1개** — 같은 `working_dir` 충돌 방지 (per-run worktree는 Phase 2)
5. **멘션은 담당자 전환이 아님** — 멘션은 일회성 위임이며 `run.agent_id`만 다르게
6. **comment cap 64KB + raw HTML 금지** — 브라우저 멈춤 + 인젝션 방어
7. **stdout pipe drain** — cap 후에도 `io.Discard` 로 계속 read (child blocking 방지)
8. **process group kill** — `setpgid` + `kill(-pgid)` 로 자식의 자식까지 정리
9. **timestamp RFC3339 UTC** — `datetime('now')` 직접 의존 안 함
10. **Identifier resolve API** — URL은 `NEWS-12`, API는 UUID + identifier 둘 다 받음

---

## 🤝 기여

로컬 MVP 통합 단계에서는 소규모 이슈/문서 개선 중심으로 검토합니다. 대형 기능 추가는 ROADMAP 범위 조정 후 진행합니다.

### 보고 / 제안
- 설계 의견: GitHub Issues (예정)

---

## 📜 라이선스

MIT License © 2026 Coreline AI

전체 LICENSE 파일은 Phase 7 릴리스에 포함.

---

<div align="center">

**Built with focus on simplicity over scale.**

[⬆ 맨 위로](#-corn-agent-dashboard)

</div>
