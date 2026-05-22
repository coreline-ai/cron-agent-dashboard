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

### 큰 기능 (Phase 2+ 첫 라운드 — 2026-05-21/22 사이클로 모두 닫힘)

- [x] **Auto-chain 고급 UI** — `ChainSummaryPanel`로 chain_id 단위 요약(run 수 / max depth / token / cost / 마지막 상태) 가시화. 운영 액션(stage cancel/retry)은 별도 plan 후속. — commits `06d3ef8`, `2c8f773` / plan `implement_20260521_214433.md`, `implement_20260522_182604.md`.
- [x] **첨부 파일** — schema + store + file storage helper + multipart HTTP + IssueDetailPage 패널까지 end-to-end. — commits `d8dadef`, `8647ad7`, `259951e` / plan `implement_20260522_174204.md`.
- [x] **per-run worktree** — `workspace.per_run_worktree` opt-in + ClaimNextRun 가드 완화 + worktree allocate/cleanup + UI 토글. — commits `453f201`, `252ea48` / plan `implement_20260521_222623.md`.
- [x] **워크스페이스 import/export** — `WorkspaceExport` JSON 포맷 v1 + `workspace-export`/`workspace-import` CLI. (이슈/run/comment history는 별도 plan으로 명시 이관) — commit `5d972f7` / plan `implement_20260521_221719.md`.
- [x] **외부 webhook** — workspace 구독 + HMAC-SHA256 dispatcher + Settings UI + 1회 retry. — commits `064b011`, `2fa9c65`, `9225d9b`, `54bc72a` / plan `implement_20260521_224221.md`.

### 운영·릴리스 (P2·P3)

- [x] demo seed — `cron-agent-dashboard seed` 명령 + Lead/Writer/Reviewer + hub guard instructions + `auto_chain_enabled=true` 자동 적용. — commit `e220913` / plan `implement_20260521_214433.md`.
- [x] 별도 LICENSE 파일 추가 — MIT 전문 `LICENSE` 추가, README 라이선스 절 정리. — plan `implement_20260522_183344.md` (Track D).
- [x] Homebrew tap formula 템플릿 — `docs/homebrew/cron-agent-dashboard.rb.tmpl` (darwin/linux × arm/amd 4-vendor 분기, sha256 placeholder). 실제 tap publish는 운영자 own-repo에서 진행. — Track D.
- [x] full release artifact smoke 자동화 강화 — `scripts/release-smoke.sh` + `make release-smoke` + `.github/workflows/release.yml` 단계. `cron-agent-dashboard version` subcommand 추가. — Track D.
- [ ] run log retention UX 고도화 (`--auto-cleanup-log-days` 기반 운영 안내 + UI 노출 정밀화).
- [ ] 대량 데이터 성능 검증 (이슈 1,000개+ / run·comment·event 누적 시 detail page 성능).

### 잔여 후속

- [x] codex `--json` 전환 — `ParseCodexJSONL` + `StdoutFileMetricsParser` opt-in 인터페이스 + `readRunComment` codex 분기. MCP noise는 자연스럽게 무시. — commit `c41a24e` / plan `implement_20260522_183344.md` Track A.
- [x] claude `--print` adapter hang 정공 수정 — `--input-format text` 명시로 해결. — commit `fd3461e` / plan `implement_20260521_221108.md`.
- [x] `auto_chain_max_depth` 기본 5 검토 — hub-PM 재진입을 depth 카운트에서 제외해 기본 5 유지로도 RFP 풀체인 가능. — commit `1ff2b93` / plan `implement_20260521_211716.md`.
- [x] `knownNoiseLines` 모니터링 — ROADMAP "로그 관리" 절에 monitoring 메모 명시. 새 패턴 발견 시 `internal/worker/runtime/sanitize.go::knownNoiseLines` 맵에 한 줄 추가하면 끝. — plan `implement_20260522_212332.md` Track A.
- [x] `docs/ROADMAP.md` stale 정리 1차 — 상단 callout으로 v0.1 출시 후 작업은 TODO/CHANGELOG 참조 + demo seed 항목 [x]. — commit `5245892`.
- [x] `docs/ROADMAP.md` 본문 unchecked 항목 재분류 — 50여 줄을 done/deferred/superseded 라벨로 정리. 남은 unchecked 3건은 성능 검증 deferred만 명시. — Track A.
- [x] workspace import/export 확장 (export side) — 이슈/run/comment/attachment 메타 직렬화 + email/phone PII 마스킹 + HTTP `GET /api/workspaces/:slug/export?include_history=1&mask_pii=1` 라우트 + CLI `--include-history` / `--mask-pii`. 히스토리 import 복원은 별도 plan으로 남김. — commit `51d7e0b` / Track B.
- [x] Auto-chain UI 후속 (1차) — chain 단위 cancel 액션. `POST /api/runs/chain/{chain}/cancel` + ChainSummaryPanel "체인 취소" 버튼. chain retry / depth·cost guard 시각화 / workspace 차원 chain dashboard는 별도 plan. — Track D.
- [x] 첨부 파일 후속 (1차) — MIME deep-sniff(`http.DetectContentType` 첫 512B) + 다운로드 audit log(`attachment_audit` 테이블 + `GET /api/attachments/{id}/audit`). comment 첨부 / image inline preview / 바이러스 스캔 / S3·MinIO storage는 별도 plan. — Track C.
- [x] webhook 후속 (1차) — `issue.cancelled` 이벤트 dispatch + per-subscription `mask_pii` 옵션 + Settings UI 노출. 다회 retry / exponential backoff / dead-letter는 별도 plan. — Track B.
- [x] per-run worktree 후속 — `git worktree add --detach`/`git worktree remove --force` 통합. workspace.working_dir이 git repo이면 자동 분기, 아니면 plain mkdir fallback. — commit `1c537b5` / Track C. (worktree 파일 자동 시드 / 디스크 사용량 관측·GC는 별도 plan)

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
