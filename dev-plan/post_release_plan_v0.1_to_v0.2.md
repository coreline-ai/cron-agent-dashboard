# Post-release Plan: v0.1 → v0.2

> 단일 사용자 daily-use 운영을 가정한 시뮬레이션 기반 후속 개발 계획
> Generated: 2026-05-14
> Status: 가설 — 실측 후 검증 / 조정

---

## 0. 개요

### 0.1 배경

v0.1 릴리스 시점(2026-05-14)에 10차 리뷰를 마쳤다. 리뷰 사이클을 통해 코드 자체의 평균 품질 점수는 9.2 / 10. 그러나 *"코드가 깨지지 않는가"* 와 *"매일 쓸 때 답답하지 않은가"* 는 다른 축이다. 본 문서는 단일 사용자 daily-use를 1주 / 1개월 / 3개월 단위로 시뮬레이션한 결과를 토대로, **각 시점에 무엇을 우선해야 하는지**를 정리한다.

### 0.2 시뮬레이션 가정

- 단일 사용자, macOS 환경
- 워크스페이스 3~5개 (`ai-news` / `code-review` / `docs` / `autopilot-test`)
- 일일 5~10 이슈 생성, 멘션 위임 1~2회
- Autopilot 룰 3개 가동 (매일 09:00 / 매주 금요일 / 매주 월요일)
- codex / claude / gemini CLI 전체 PATH 설치
- 평균 run 시간 30초 ~ 5분

### 0.3 본 문서의 위치

- `docs/ROADMAP.md` 와 보완 관계 — ROADMAP은 Phase 단위 큰 그림, 본 문서는 출시 후 미세 조정
- 모든 항목은 **가설**로 표시. 실측치가 가설과 다르면 우선순위 재조정
- 실측 SQL은 §7 카탈로그 참조

---

## 1. 우선순위 매트릭스

| 항목 | 리뷰 시점 | 시뮬레이션 후 | 변경 사유 |
|---|---|---|---|
| 명시 refresh 버튼 + Tab focus refetch | 미언급 | 🔴 1순위 | polling 5초 답답함 |
| Agent name 자동완성 | 미언급 | 🔴 1순위 | 멘션 오타 주 2~3회 |
| Token / cost 추적 | 보류 | 🔴 1순위 | daily-use 첫 주 가장 절실 |
| Frontend 단위 테스트 (vitest) | 중간 | 🟡 2순위 | 첫 주 UI 회귀 5~6회 추정 |
| Autopilot snooze_until | 미언급 | 🟡 2순위 | 주 1회 정도 일시 정지 요구 |
| Sub-issue Phase 2.0 | 1개월 후 검토 | 🟢 조건부 | 사용자 피드백 3+ 시 시작 |
| HP-5 worker_panic 자동 차단 | 중간 | 🟢 보류 | 첫 주 0회 발생 확률 70% |
| HP-2 detail_json size cap | 낮음 | 🟢 보류 | 평균 100 bytes, 4KB까지 여유 |
| CR-3 stale zombie 처리 | 낮음 | 🟢 보류 | 한 달 zombie 0~1회 |
| HP-1 N+2 query 최적화 | 낮음 | 🟢 cleanup PR | 성능 영향 무의미, 깔끔성만 |

---

## 2. 운영 트랙 & 측정 인프라

### 2.1 사전 셋업 (출시 D-1)

- `run_event` 테이블 + `terminal_reason` / `failure_kind` / `cancel_reason` 분류 — 이미 v0.1 포함
- slog JSON handler — 이미 `main.go` 적용
- **추가 권장 (1시간)**: 로그 회전 + 외부 수집 위치 결정 (단일 사용자라면 `~/Library/Logs/corn-agent-dashboard.log` 로 redirect)

### 2.2 측정 지표 (§7 카탈로그 사용)

| 지표 | 측정 빈도 | 수집 방법 |
|---|---|---|
| 일일 이슈 생성 수 | 일 | SQL count |
| 멘션 위임 빈도 | 일 | run.trigger_type='mention' count |
| Autopilot 성공 / 실패 | 일 | run_event |
| run 평균 실행 시간 | 주 | finished_at - claimed_at |
| 에이전트 실패 분류 | 주 | failure_kind 그룹화 |
| token 사용량 (P0 도입 후) | 일 | run.input/output_tokens sum |

### 2.3 사용자 피드백 채널

- 단일 사용자 환경이므로 본인이 직접 기록
- `~/.corn-agent-dashboard/feedback.md` 또는 macOS Notes에 일자별 기록
- 권장 양식:
  ```
  ## 2026-05-15
  - 답답: 멘션 후 보드 새로고침까지 5초 기다림
  - 좋음: Autopilot 09:00 뉴스 자동 생성 정확
  - 버그?: 이슈 detail에서 [완료] 누르고 다른 카드 [완료] 누르려는데 비활성
  ```

---

## 3. v0.1.x 후속 (출시 직후 1~2주)

### 3.1 명시 refresh + Tab focus refetch

**목표**: polling 5초 답답함 해소.

**범위**:
- React Query `refetchOnWindowFocus: true` 활성화
- 페이지 우상단에 명시 `[새로고침]` 버튼 (보드 / 이슈 detail / autopilot)
- 새로고침 진행 중 spinner

**코드 위치**:
- `web/src/main.tsx` (queryClient default 옵션)
- `web/src/pages/BoardPage.tsx` / `IssueDetailPage.tsx` / `AutopilotPage.tsx`

**예상 LOC**: ~30
**의존성**: 없음
**완료 기준**:
- 사용자가 다른 탭/앱에서 다시 돌아왔을 때 자동 refetch
- 명시 새로고침 후 3초 이내 데이터 갱신

---

### 3.2 Agent name 자동완성

**목표**: 멘션 오타 차단 + 사용자 입력 시간 단축.

**범위**:
- 댓글 textarea에서 `@` 입력 시 워크스페이스 agent list 노출
- HTML5 `<datalist>` 활용 (가벼움, 라이브러리 무게 0)
- 키보드 화살표 + Enter 선택 가능

**코드 위치**:
- `web/src/components/MentionAutocomplete.tsx` (신규)
- `web/src/pages/IssueDetailPage.tsx` 댓글 입력 영역

**예상 LOC**: ~80
**의존성**: `useAgentsQuery(slug)` 이미 존재
**완료 기준**:
- 9개 agent 있는 워크스페이스에서 `@` 입력 시 list 즉시 표시
- 한글 agent 이름 매칭

---

### 3.3 Token / cost 추적

**목표**: *"어느 워크스페이스/agent가 토큰 많이 먹는지"* 가시화.

**범위 (P0)**:
- `run` 테이블에 `input_tokens / output_tokens / total_tokens / duration_ms / model_resolved` 컬럼 추가 (migration 0008)
- runtime adapter에 `ParseMetrics(stdout, stderr) RunMetrics` 메서드 추가
  - codex: `--print-metrics` 또는 stdout 마지막 JSON 라인
  - claude: stderr 또는 stdout 'usage' 키
  - gemini: stdout JSON envelope
- worker_store FinishRun에서 ParseMetrics 호출 후 store에 전달

**범위 (P1, 후속)**:
- `agent.model_cost_per_input_million / model_cost_per_output_million` 컬럼 + UI 단가 설정
- Workspace card에 7일 누적 token + 비용 표시
- Settings에 전체 워크스페이스 누적 비교

**코드 위치**:
- `internal/db/migrations/0008_run_token_usage.sql` (신규)
- `internal/store/models.go` (Run struct 확장)
- `internal/store/issues.go` `CompleteRun` SQL
- `internal/worker/runtime/{codex,claude,gemini}.go` (각 adapter ParseMetrics)
- `internal/app/worker_store.go` FinishRun

**예상 LOC**:
- P0: ~150 (migration + adapter ParseMetrics 3 × ~30 + store 30)
- P1: ~200 (UI + 단가 설정)

**의존성**: 각 CLI의 metrics 출력 format 확인 필요
**완료 기준 (P0)**:
- run 1회 완료 시 token 컬럼 채워짐 (NULL 아님)
- adapter 단위 테스트로 sample 출력 → 정확한 값 추출 검증

**완료 기준 (P1)**:
- 보드 카드에 "오늘 토큰 12.4k · $0.18" 표시
- Settings에 워크스페이스별 7일 누적

---

### 3.4 Frontend 단위 테스트 도입 (vitest)

**목표**: UI 회귀 가드 (첫 주 5~6회 추정).

**범위 (P0)**:
- `vitest` + `@testing-library/react` + `jsdom` 도입
- 핵심 4 컴포넌트 단위 테스트:
  - `Modal.test.tsx` (Esc / backdrop click / target check)
  - `StatusPill.test.tsx` (5 kind × 주요 status 라벨)
  - `ToastProvider.test.tsx` (mount/unmount 안전 + 자동 dismiss)
  - `MutationErrorAlert.test.tsx` (error body 파싱 + dismiss)

**범위 (P1)**:
- `useToast` hook 단위 테스트
- `ConfirmDialog` confirm/cancel 콜백
- `CreateIssueDialog` form 제출

**코드 위치**:
- `web/vitest.config.ts` (신규)
- `web/src/components/*.test.tsx`

**예상 LOC**: ~250 (4 컴포넌트 × ~60 + config + helpers)
**의존성**: 없음
**완료 기준**:
- `pnpm --filter web test` 실행 + 모든 통과
- CI에 vitest step 추가 (`.github/workflows/ci.yml`)
- 회귀 시나리오 5종 가드 (Modal target/currentTarget, StatusPill pulse, ToastProvider unmount, MutationErrorAlert dismiss, ConfirmDialog backdrop)

---

### 3.5 Autopilot snooze (일시 정지)

**목표**: 휴가 / 작업 중단 기간 동안 룰 자동 OFF.

**범위**:
- `autopilot_rule.snooze_until TEXT` 컬럼 추가 (migration 0009)
- cron callback 진입 시 `snooze_until > now()` 이면 skip
- UI: 룰 카드에 "1주일 정지 / 한 달 정지 / 직접 입력" 빠른 선택

**코드 위치**:
- `internal/db/migrations/0009_autopilot_snooze.sql`
- `internal/store/models.go` (AutopilotRule struct)
- `internal/scheduler/cron.go` 또는 callback wrapper
- `web/src/components/AutopilotDialog.tsx` + `AutopilotPage.tsx`

**예상 LOC**: ~80
**의존성**: 없음
**완료 기준**:
- snooze 설정 시 cron 시각이 와도 issue 생성 안 됨
- snooze 만료 시각 이후 첫 cron tick에 정상 트리거

---

## 4. v0.2 (출시 1~2개월)

### 4.1 보드 mutation isPending 카드별 분리

**가설**: 1주 사용 후 사용자 불편 피드백 빈도 ≥ 2회면 진행.

**문제**: 한 카드의 mutate 진행 중 다른 카드의 같은 mutation 버튼도 disable.

**범위**:
- `mutate.variables` 활용 — `isPending && variables?.issue.id === issue.id` 패턴
- `BoardPage.tsx` 의 cancel / done / cancelExecution mutations에 적용

**코드 위치**: `web/src/pages/BoardPage.tsx`
**예상 LOC**: ~20
**의존성**: 없음

---

### 4.2 Sub-issue Phase 2.0 (점진 활성화)

**조건**: 1개월 운영 후 사용자가 "수동으로 5개 이슈 생성" 시나리오 ≥ 3회 시 진행.

**범위 (P0)**:
- `POST /api/issues/:id/subissues` 엔드포인트
- 보드 카드에 sub-issue 개수만 표시 (트리 X)
- IssueDetail 페이지에 sub-issues list 섹션
- `idx_issue_parent` 인덱스 활용 (이미 v0.1 포함)

**코드 위치**:
- `internal/httpapi/server.go` (라우터 + 핸들러)
- `internal/store/issues.go` (`ListSubIssues`, `CreateSubIssue`)
- `web/src/pages/IssueDetailPage.tsx` (sub-issues section)
- `web/src/components/SubIssueList.tsx` (신규)

**예상 LOC**: ~250
**의존성**: 없음 (parent_issue_id 컬럼 이미 존재)
**완료 기준**:
- 부모 이슈에서 sub-issue 생성 가능
- IssueDetail에 sub-issues 진행률 (3/5 done) 표시

**Out of scope (Phase 2.1로 이연)**:
- 모든 sub-issues done 시 부모 자동 done
- 트리 시각화

---

### 4.3 CompleteRun stdout_path 회수 (CR-3 보강)

**조건**: 한 달 사용 후 cancel-after-claim 발생 ≥ 1회 + 부분 stdout 회수 요청 시.

**범위**:
- `CompleteRun` 의 `aff == 0` 분기에서 별도 `UPDATE run SET stdout_path = ? WHERE id = ? AND stdout_path IS NULL`
- cancel된 run의 부분 로그가 `/api/runs/:id/log` 로 다운로드 가능

**코드 위치**: `internal/store/issues.go:404-409`
**예상 LOC**: ~8
**의존성**: 없음
**완료 기준**:
- cancel-after-claim 시나리오 회귀 테스트 추가
- stdout 파일이 DB에서 참조 가능

---

## 5. v0.2.x / v0.3 (출시 3개월+)

### 5.1 HP-1 cleanup PR (N+2 query 최적화)

**조건**: 항상 진행 (코드 깔끔성 + Phase 2 대비).

**범위**:
- `appendRunEventTx(ctx, tx, issueID, in)` 시그니처에 issueID 직접 인자
- 호출처 11개 갱신
- 추가 SELECT 제거

**코드 위치**: `internal/store/run_events.go`, `internal/store/issues.go` 11곳
**예상 LOC**: ~30 (변경 + 호출처)
**의존성**: 없음

---

### 5.2 Sub-issue Phase 2.1 (자동 종합)

**조건**: Phase 2.0 안정화 후 4주 경과.

**범위**:
- 부모 이슈의 모든 sub-issue가 `done`이면 부모 자동 `done` 전이
- 종합 system 댓글 ("모든 sub-issue 완료")
- Run lineage 트리 UI (들여쓰기)

**예상 LOC**: ~200

---

### 5.3 멀티에이전트 분석 #C1 - Auto-chain opt-in (Phase 2.2)

**조건**: 매우 보수적. 사용자가 *"매번 같은 chain 패턴을 멘션으로 만들고 있다"* 명시 요청 시.

**범위**: `docs/CHAINING.md` §6 흐름 그대로 구현
- `chain_id` / `parent_run_id` / `chain_depth` 컬럼 추가
- workspace 단위 opt-in (`auto_chain_enabled` 기본 OFF)
- max depth 5
- `trigger_type='agent_mention'`

**예상 LOC**: ~400 + 회귀 테스트 ~150

---

### 5.4 Agent instructions 버전 관리

**상태**: 완료 (`0014_agent_instruction_history.sql`, `GET /api/agents/:id/instructions`, Agent Detail version history UI).

**조건**: 사용자가 한 달 이상 동일 agent instructions 자주 수정 + *"왜 이 run이 이렇게 답했지"* 추적 요청 시.

**범위**: `agent_instructions_history` 테이블 + run에 version snapshot
**예상 LOC**: ~150

---

## 6. 의도된 미구현 (Phase 3+ 또는 영원히 보류)

| 항목 | 보류 이유 |
|---|---|
| 멀티 사용자 / RBAC | 단일 사용자 가정 |
| Sandbox / 권한 격리 | 사용자 신뢰 모델 |
| MCP / tool 통합 | 각 CLI native 능력에 위임 |
| 클라우드 / SaaS | 로컬 우선 |
| 모바일 앱 | 데스크톱 환경 |
| 첨부 파일 | working_dir 의 파일로 충분 |

---

## 7. 측정 SQL 카탈로그

운영 중 실측에 사용. SQLite shell 또는 `cad-cli stats` (Phase 2 후보) 로 실행.

### 7.1 일일 활동
```sql
-- 오늘 생성된 이슈 수
SELECT COUNT(*) AS today_issues
FROM issue
WHERE date(created_at) = date('now');

-- 오늘 완료된 run 수
SELECT COUNT(*) AS today_runs
FROM run
WHERE date(finished_at) = date('now') AND status = 'done';
```

### 7.2 실패 분류 (주간)
```sql
SELECT failure_kind, terminal_reason, COUNT(*) AS n
FROM run
WHERE status = 'failed' AND created_at >= datetime('now', '-7 days')
GROUP BY failure_kind, terminal_reason
ORDER BY n DESC;
```

### 7.3 worker_panic 빈도 (HP-5)
```sql
SELECT COUNT(*) AS panic_count
FROM run
WHERE terminal_reason = 'worker_panic'
  AND created_at >= datetime('now', '-7 days');
-- 임계: ≥ 3건 / 1주 → 자동 차단 정책 검토
```

### 7.4 stale recovery 빈도 (CR-3)
```sql
SELECT
  SUM(CASE WHEN terminal_reason='orphan_recovered' THEN 1 ELSE 0 END) AS orphan,
  SUM(CASE WHEN terminal_reason='stale_recovered' THEN 1 ELSE 0 END) AS stale
FROM run
WHERE created_at >= datetime('now', '-30 days');
-- 임계: stale ≥ 5건 / 1개월 + 사용자 zombie 보고 → zombie cleanup 도입
```

### 7.5 detail_json size 분포 (HP-2)
```sql
SELECT
  AVG(length(detail_json)) AS avg_bytes,
  MAX(length(detail_json)) AS max_bytes,
  COUNT(CASE WHEN length(detail_json) > 1024 THEN 1 END) AS over_1kb
FROM run_event;
-- 임계: max ≥ 4096 또는 over_1kb ≥ 10 → cap 도입
```

### 7.6 claim throughput (HP-1)
```sql
-- 평균 claim → start latency
SELECT
  AVG(julianday(started_at) - julianday(enqueued_at)) * 86400 AS avg_seconds,
  MAX(julianday(started_at) - julianday(enqueued_at)) * 86400 AS max_seconds
FROM run
WHERE started_at != ''
  AND enqueued_at >= datetime('now', '-30 days');
-- 임계: avg ≥ 5초 → polling interval 단축 또는 N+2 query 제거
```

### 7.7 Sub-issue 시나리오 (Phase 2.0 트리거)
```sql
-- 같은 사용자가 1시간 내에 같은 워크스페이스에 N개 이슈 연속 생성
SELECT workspace_id, date(created_at) AS d, COUNT(*) AS bursty
FROM issue
WHERE created_by = 'user'
GROUP BY workspace_id, strftime('%Y-%m-%d %H', created_at)
HAVING bursty >= 4;
-- 자주 발생 (≥ 3회 / 1개월) → Sub-issue Phase 2.0 ROI 확인
```

### 7.8 Autopilot 실패 빈도
```sql
SELECT
  name,
  consecutive_failures,
  last_error,
  last_run_at
FROM autopilot_rule
WHERE consecutive_failures > 0
ORDER BY consecutive_failures DESC;
-- 임계: 한 룰이 자주 disable (consecutive_failures = 5) → cron expr 또는 instructions 검토
```

### 7.9 토큰 사용량 (v0.1.x 도입 후)
```sql
-- 워크스페이스별 7일 토큰
SELECT i.workspace_id, ws.name,
  SUM(r.input_tokens + r.output_tokens) AS total_tokens
FROM run r
JOIN issue i ON i.id = r.issue_id
JOIN workspace ws ON ws.id = i.workspace_id
WHERE r.finished_at >= datetime('now', '-7 days') AND r.status = 'done'
GROUP BY i.workspace_id
ORDER BY total_tokens DESC;
```

---

## 8. 의사결정 트리거 매트릭스

각 시점에 측정 → 다음 작업 선택.

| 시점 | 측정 | 임계 | 다음 작업 |
|---|---|---|---|
| 1주 | 사용자 피드백 | refresh 답답 ≥ 5회 / 1주 | §3.1 즉시 |
| 1주 | 사용자 피드백 | 멘션 오타 ≥ 3회 / 1주 | §3.2 즉시 |
| 1주 | 사용자 피드백 | token 추적 요청 1회+ | §3.3 P0 즉시 |
| 1주 | UI 회귀 | 5회+ 이상 발견 | §3.4 즉시 |
| 1개월 | Autopilot snooze 요청 | 1회+ | §3.5 진행 |
| 1개월 | §7.3 panic | ≥ 3건 | HP-5 자동 차단 도입 |
| 1개월 | §7.4 stale | stale ≥ 5건 + zombie 보고 | CR-3 cleanup |
| 1개월 | §7.5 detail_json | max ≥ 4096 또는 over_1kb ≥ 10 | HP-2 cap 도입 |
| 1개월 | §7.6 claim throughput | avg ≥ 5초 | HP-1 + polling 조정 |
| 1개월 | §7.7 sub-issue burst | ≥ 3회 / 1개월 | §4.2 Phase 2.0 |
| 3개월 | §7.8 autopilot | 자주 disabled | cron / instructions 개선 |
| 3개월 | 사용자 명시 요청 | auto-chain 1회+ | §5.3 검토 |

---

## 9. 작업 순서 요약

### v0.1.x 패치 (출시 후 1~2주)
1. §3.1 refresh + Tab focus (~30 LOC)
2. §3.2 agent 자동완성 (~80 LOC)
3. §3.3 P0 token 추적 (~150 LOC)
4. §3.4 vitest P0 (~250 LOC)
5. §3.5 autopilot snooze (~80 LOC)

### v0.2 (출시 1~2개월)
6. §3.3 P1 token UI (~200 LOC)
7. §4.1 mutation isPending 카드별 (~20 LOC)
8. §4.2 sub-issue Phase 2.0 (조건부) (~250 LOC)
9. §4.3 CompleteRun stdout_path 회수 (조건부) (~8 LOC)

### v0.2.x / v0.3 (출시 3개월+)
10. §5.1 HP-1 cleanup PR (~30 LOC)
11. §5.2 sub-issue Phase 2.1 (조건부) (~200 LOC)
12. §5.3 auto-chain Phase 2.2 (사용자 요청 시) (~400 LOC)

---

## 10. 사후 검토

본 문서는 v0.1 릴리스 시점의 가설. 출시 1개월 후 다음 영역 재평가:

1. 시뮬레이션 정확도 — 가설 항목별 실측 차이 기록
2. 우선순위 매트릭스 갱신
3. Phase 2 / Phase 3 로드맵 구체화

재평가 결과는 `docs/ROADMAP.md` 의 새 섹션 또는 `dev-plan/post_release_review_2026Q3.md` 로 분리.
