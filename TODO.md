# TODO

Cron Agent Dashboard 초기 scaffold 이후 남은 항목을 우선순위별로 추적한다.

## Backlog — Phase 2+ (2026-05-21 갱신)

MVP / P0~P3가 모두 완료된 상태에서 다음 사이클의 작업을 정리한다. 본 plan(`dev-plan/implement_20260521_191038.md`)에서 처리한 항목은 [x], 큰 기능·운영·잔여 후속은 [ ]로 남겨둔다.

### 품질 개선 (P1·P2)

- [x] `internal/store/ListIssues`: `execution` 필터를 SQL WHERE로 이동 (LIMIT 전 적용) — commit `27021cc`.
- [x] Gemini runtime adapter prompt를 argv가 아닌 stdin pipe로 전달해 `/proc/<pid>/cmdline` 노출 차단 — commit `13bc9ed`.
- [x] Settings UI에 `auto_close_on_run_done` workspace toggle 노출 — commit `9770c05`.
- [x] auto-chain agent lookup에서 ErrNotFound vs 일시적 store 오류를 시스템 코멘트로 구분 — commit `e458c05`.
- [x] `PUT /api/agents/:id` 계약을 full-replace로 명시하고 contract test로 못박음 — Phase 5.
- [x] `pnpm audit --prod` clean, dev advisory(vite/esbuild moderate ×2) 해소 — vite ^6.4.2 + vitest@latest 적용.

### 큰 기능 (별도 plan)

- [ ] **Auto-chain 고급 UI** — chain tree/sequence 편집, 중단/재개/재시도 stage 단위, main agent re-entry 정책 가시화.
- [ ] **첨부 파일** — issue / comment file attachment. 파일 크기·보존·보안 정책.
- [ ] **per-run worktree** — 같은 workspace 내 병렬 실행 허용 + git 상태 충돌 방지. 현재는 workspace당 running run 1개.
- [ ] **워크스페이스 import/export** — workspace + agents + skills + autopilot rule + issue history. 민감정보 마스킹 옵션.
- [ ] **외부 webhook** — issue / run 종료 이벤트 발신. retry / signing secret / delivery log.

### 운영·릴리스 (P2·P3)

- [ ] Homebrew tap 배포.
- [ ] 별도 LICENSE 파일 추가.
- [ ] demo seed 옵션 (`--seed example`).
- [ ] full release artifact smoke 자동화 강화.
- [ ] run log retention UX 고도화.
- [ ] 대량 데이터 성능 검증 (이슈 1,000개+ / run·comment·event 누적 시 detail page 성능).

### 잔여 후속 (별도 plan 검토)

- [ ] codex `--json` 전환 — JSON Lines 파서 + `ParseMetricsFromText` 재작성.
- [ ] claude `--print` adapter hang 정공 수정 (현재 Lead 운영을 codex로 우회).
- [ ] demo seed에 `auto_chain_enabled` / `auto_chain_max_depth` / `Lead.instructions` 운영 기본값 묶기.
- [ ] `auto_chain_max_depth` 기본 5 검토 — hub-PM 패턴에서 항상 ~10 depth 필요.
- [ ] `knownNoiseLines` 모니터링 — 새 runtime CLI 진단 패턴 발견 시 추가.
- [ ] `docs/ROADMAP.md` stale unchecked 항목 정리 (현재 plan 범위 밖).

## P0 — MVP 동작 Blocking

- [x] Worker pool을 `cmd/cron-agent-dashboard serve` lifecycle에 연결
- [x] Store ↔ Worker adapter 구현
- [x] queued run claim → executor 실행 → run/comment/issue 상태 반영
- [x] active run cancel API와 process cancel 연결
- [x] fake runtime 기반 worker integration test 추가
- [x] Frontend sample data 제거 및 실제 workspace/issue API 연동 시작

## P1 — MVP 핵심 완성

- [x] Autopilot scheduler를 DB rule과 연결
- [x] rule create/update/delete 후 scheduler reload 정책 구현
- [x] issue detail comment/run/rerun/cancel UI 연동
- [x] agent CRUD UI 연동
- [x] autopilot create/delete/toggle/manual trigger UI 연동
- [x] settings backup/vacuum/cleanup UI 연동
- [x] safe markdown renderer 구현 (`react-markdown` + `remark-gfm`, raw HTML 금지)
- [x] Static embed + SPA fallback 구현

## P2 — 운영/품질

- [x] API response DTO에서 absolute local path 노출 방지 테스트 강화
- [x] CORS/token auth regression test 추가
- [x] SQLite checkpoint 기반 안전 backup command 구현
- [x] CLI `backup --to` / `restore --from` 구현
- [x] startup self-check 추가: pragma, main agent uniqueness, orphan recovery summary 노출
- [x] release build matrix / artifact naming 확정
- [x] clean clone quick start 검증 자동화 및 로컬 검증
- [x] token mode UX 정교화: 초기 401 상태에서 토큰 안내/저장 흐름 개선

## P3 — Release Docs

- [x] README quick start를 실제 동작 기준으로 업데이트
- [x] ROADMAP checkbox와 구현 상태 동기화
- [x] API.md endpoint 수와 구현 route 수 동기화
- [x] TRD/ARCHITECTURE skeleton 표현 정리
- [x] final local smoke 결과 기록

## Completed

- [x] Initial design documentation committed
- [x] Foundation scaffold pushed to `main` — `97ba94f feat: scaffold cron agent dashboard`
- [x] Go/SQLite/API skeleton added
- [x] Worker/scheduler primitives added
- [x] Worker/store/main runtime wiring added
- [x] Autopilot scheduler DB rule wiring added
- [x] Frontend real API binding and create/update/action flows added
- [x] Vite SPA embedded into Go binary with direct-route fallback
- [x] CLI backup/restore and release packaging targets added
- [x] API hardening tests for auth, CORS, static fallback, backup, stdout path leak added
- [x] `make check` verification baseline established
- [x] Startup self-check, clean clone verification script, Playwright browser smoke, token 401 UX, GitHub Release upload workflow added
- [x] Expert review gap closure: explicit `comment.truncated`, Autopilot `next_run_at` sync, safe markdown renderer, real `VACUUM`, pointer-based issue update, CLI-specific runtime adapter arguments
- [x] Post-release v0.1.x polish: focus refetch, explicit refresh buttons, agent mention autocomplete, Autopilot snooze_until
- [x] Startup orphan process cleanup safety guard: `process_recorded_at` freshness check + process metadata retry

- [x] Multi-agent resource controls foundation: token/cost metrics capture, timeout resolve, transient retry, Unicode mention regex
- [x] Agent instructions version history: instructions 변경 이력 + run snapshot + Agent 상세 UI
