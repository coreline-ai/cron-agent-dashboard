# TODO

Corn Agent Dashboard 초기 scaffold 이후 남은 항목을 우선순위별로 추적한다.

## P0 — MVP 동작 Blocking

- [x] Worker pool을 `cmd/corn-agent-dashboard serve` lifecycle에 연결
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
- [ ] TRD/ARCHITECTURE skeleton 표현 정리
- [x] final local smoke 결과 기록

## Completed

- [x] Initial design documentation committed
- [x] Foundation scaffold pushed to `main` — `97ba94f feat: scaffold corn agent dashboard`
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
