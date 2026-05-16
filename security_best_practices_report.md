# Security Best Practices Review — Cron Agent Dashboard

- 대상: `/Users/hwanchoi/projects/core-agent-dashboard/`
- 일자: 2026-05-16
- 범위: Go HTTP API / SQLite store / worker executor / React SPA / CI dependency posture
- 기준: 로컬 우선 단일 사용자 앱이지만, 토큰 모드·비로컬 바인딩·CLI 에이전트 실행까지 고려

## 실행한 점검

| 점검 | 결과 |
|---|---|
| `git status --short` | 병렬 작업 통합 전 `security_best_practices_report.md` 초안 untracked, 통합 후 hardening diff 검토 |
| `pnpm audit --prod` | ✅ No known vulnerabilities found |
| `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` | ✅ 로컬 Go `1.26.3` 업데이트 후 호출 경로 취약점 0건 |
| 정적 점검 | SQL placeholder, XSS sink, CORS/Auth, 파일 권한, HTTP timeout, 로그/백업 경로, CI 설치 정책 확인 |
| Worker C 검증 | ✅ YAML parse/grep 수준 확인: CI Go version, `pnpm install --ignore-scripts`, 문서 정책 문구 |
| 통합 검증 | ✅ `go test ./...`, `go test -race ./...`, `go vet ./...`, `pnpm web:build`, `make check`, `make e2e-smoke` 통과 |
| CI 재발 방지 | ✅ `go1.26.3` 기반 GitHub Actions에 `govulncheck ./...` step 추가 |
| 운영 검증 | ✅ 임시 data-dir 실제 서버 기동 후 healthz, CSP/security headers, workspace 생성, Backup API 경로 제한, backup/data/log 권한, 외부 bind token 강제 확인 |

## 종합 판정

현재 코드는 **로컬 우선 개인용 에이전트 대시보드로는 보안 기본기가 상당히 좋습니다.**
특히 외부 바인딩 시 토큰 강제, SQL placeholder 사용, raw HTML 미사용 Markdown 렌더, process group 기반 종료, stdout/comment cap, strict env parse는 강점입니다.

병렬 워커 A/B/C/D/E와 통합 패치에서 CI/문서 hardening, HTTP/API hardening, 파일 권한 hardening, backup path hardening, token storage hardening을 반영했습니다. 이후 로컬 Go toolchain도 `1.26.3`으로 업데이트했고 `govulncheck ./...` 재실행 결과 호출 경로 취약점 0건을 확인했습니다.

## 반영 현황

| 항목 | 상태 | 반영 파일 |
|---|---|---|
| CI/Release Go toolchain | ✅ Worker C: `actions/setup-go@v5`에서 `go-version: '1.26.3'`로 명시, CI `govulncheck` step 추가 | `.github/workflows/ci.yml`, `.github/workflows/release.yml` |
| pnpm install scripts 차단 | ✅ Worker C: 모든 소유 파일 내 `pnpm install`에 `--ignore-scripts` 적용 | `.github/workflows/ci.yml`, `README.md` |
| 보안 운영 원칙 문서화 | ✅ Worker C/E: strict env, token local/session storage, agent OS 권한, data/log permission, CORS 원칙 문서화 | `README.md`, `docs/OPERATIONS.md` |
| Token storage defense-in-depth | ✅ Worker E: `sessionStorage` 저장 옵션 추가, read 우선순위는 session → local, clear는 양쪽 삭제 | `web/src/api/client.ts`, `web/src/components/AuthTokenPanel.tsx`, `web/src/pages/SettingsPage.tsx` |
| HTTP/body/CORS/header/error/healthz/token hardening | ✅ Worker A + 통합 패치 반영 및 테스트 통과 | `cmd/`, `internal/httpapi` |
| 파일 권한 hardening | ✅ Worker B 반영 및 테스트 통과 | `internal/config`, `internal/backup`, `internal/worker`, `internal/app` |
| Backup API 경로 정책 | ✅ Worker D: HTTP `to` 지정 시 기본 `{data_dir}/backups` 내부만 허용, 임의 경로는 명시 opt-in | `internal/config`, `internal/httpapi`, `README.md`, `docs/OPERATIONS.md` |

## Critical

현재 코드 기준 즉시 원격 임의 코드 실행이나 인증 우회로 판단되는 Critical 이슈는 발견하지 못했습니다.

## High

### H-1. Go 표준 라이브러리 취약점 감지: GO-2026-4971 — 해결됨

- 위치: `cmd/cron-agent-dashboard/main.go:151` → `http.Server.ListenAndServe` → `net.Listen`
- 최초 점검 결과: `govulncheck`가 `net@go1.26.2`의 취약 호출 경로를 감지했습니다.
- 취약점: `GO-2026-4971`, Windows의 `net` 패키지 NUL byte 처리 panic. Fixed in `go1.26.3`.
- 영향: 릴리스 빌드가 취약 Go toolchain으로 수행되면 표준 라이브러리 취약점을 포함할 수 있습니다. 실제 영향은 OS/입력면에 따라 제한적이지만, 공개 취약점이므로 릴리스 전 조치가 안전합니다.
- Worker C 상태: CI/Release workflow는 `go-version: '1.26.3'`로 업데이트 완료.
- 로컬 상태: Homebrew Go를 `1.26.3`으로 업데이트했고, `govulncheck ./...` 재실행 결과 `Your code is affected by 0 vulnerabilities.`를 확인했습니다.

남은 조치: 없음.

## Medium

### M-1. HTTP 서버 timeout이 부족함 — 해결됨

- 이전 위치: `cmd/cron-agent-dashboard/main.go:142-146`
- 현재 작업트리: `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`, `MaxHeaderBytes`가 설정됨 (`cmd/cron-agent-dashboard/main.go:142-150`).
- 남은 조치: timeout 값이 실제 장시간 agent/API 응답 흐름과 충돌하지 않는지 smoke 검증합니다.

### M-2. JSON request body size 제한 없음 — 해결됨

- 이전 위치: `internal/httpapi/response.go:10-16`
- 현재 작업트리: 공통 decode에서 `http.MaxBytesReader`와 2 MiB cap을 적용하고 초과 시 `413 REQUEST_TOO_LARGE`를 반환함 (`internal/httpapi/response.go:12-20`).
- 통합 보강: optional JSON endpoint인 rerun/backup/cleanup도 `decodeOptional`을 통해 같은 body cap을 적용함.
- 남은 조치: endpoint별로 2 MiB보다 큰 payload가 필요한지 제품 정책을 확인합니다.

### M-3. Empty CORS allowlist가 모든 Origin을 반사함 — 해결됨

- 이전 위치: `internal/httpapi/server.go:77-100`
- 현재 작업트리: CORS 허용 조건이 explicit allowlist 또는 same-origin으로 제한됨 (`internal/httpapi/server.go:80-104`). 빈 allowlist도 cross-origin 반사를 하지 않습니다.
- Worker C 상태: README/OPERATIONS에 “empty CORS allowlist는 same-origin only” 운영 원칙 문서화 완료.
- 남은 조치: wildcard origin이 필요하다는 요구가 생기면 token mode 전제와 함께 별도 명시 옵션으로 설계합니다.

### M-4. 보안 헤더 미설정 — 해결됨

- 이전 위치: `internal/httpapi/server.go:60-74`, `internal/httpapi/static.go:14-38`
- 현재 작업트리: 중앙 middleware가 `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, `Permissions-Policy`, enforced `Content-Security-Policy`, 동일 policy의 `Content-Security-Policy-Report-Only`를 설정함 (`internal/httpapi/server.go`).
- Worker F 상태: backend가 서빙하는 production SPA는 same-origin script/connect만 허용하는 enforced CSP로 승격됨. SPA 호환성을 위해 inline style은 허용하지만 inline script는 허용하지 않습니다.
- 통합 검증: `make e2e-smoke` 통과로 backend-served production SPA의 기본 브라우저 흐름을 확인했고, smoke test가 enforced CSP header 존재와 CSP violation console 메시지 부재를 검증합니다. 향후 inline script가 필요하면 `unsafe-inline` 대신 nonce/hash 기반으로 설계합니다.

### M-5. 데이터/런 로그 파일 권한이 넓음 — 해결됨

- 이전 위치:
  - `internal/config/config.go:146-153` — data/runs dir `0755`
  - `internal/worker/executor.go:238-251` — log dir `0755`, log file `0644`
- 현재 작업트리:
  - data/db/runs 디렉터리는 `0700` 생성 및 best-effort chmod (`internal/config/config.go:147-172`)
  - backup output 디렉터리/파일은 `0700`/`0600` (`internal/backup/backup.go:21-24`, `internal/backup/backup.go:81-134`)
  - run log 디렉터리/파일은 `0700`/`0600` (`internal/worker/executor.go:240-266`)
- Worker C 상태: README/OPERATIONS에 data/log/backup 권한 운영 원칙 문서화 완료.

### M-6. 내부 500 에러 메시지 노출 — 해결됨

- 이전 위치: `internal/httpapi/response.go:35-47`
- 현재 작업트리: default 500은 `internal server error`만 반환하고 상세 오류는 `slog`에 기록함 (`internal/httpapi/response.go:45-58`).

## Low / Defense-in-depth

### L-1. Bearer token 비교가 일반 문자열 비교 — 해결됨

- 이전 위치: `internal/httpapi/server.go:114-124`
- 현재 작업트리: SHA-256 digest와 length를 constant-time 비교함 (`internal/httpapi/server.go:130-148`).

### L-2. `/healthz`가 unauthenticated runtime probe를 트리거함 — 해결됨

- 이전 위치: `internal/httpapi/handlers_system.go:29-31`, runtime detection helpers
- 현재 작업트리: `/healthz`는 status/version/uptime/db_ok만 반환하고 runtime subprocess probe를 호출하지 않음 (`internal/httpapi/handlers_system.go:29-31`).

### L-3. Backup API가 임의 writable path를 허용 — 해결됨

- 이전 위치: `internal/httpapi/handlers_system.go:77-87`
- 현재 작업트리: HTTP `/api/system/backup`은 `to`가 비어 있으면 기존 기본 `.bak` 경로를 유지하고, `to`가 지정되면 기본적으로 `{data_dir}/backups` 내부로만 제한합니다.
- Power-user opt-in: `--allow-arbitrary-backup-paths` 또는 `CRON_AGENT_DASHBOARD_ALLOW_ARBITRARY_BACKUP_PATHS=true`를 명시하면 HTTP API 임의 경로를 허용합니다. 로컬 shell `cron-agent-dashboard backup --to ...`는 기존 임의 경로 동작을 유지합니다.

### L-4. CI dependency install scripts 정책이 job별로 다름 — 해결됨

- 이전 위치: `.github/workflows/ci.yml:32-36`, `.github/workflows/ci.yml:61-65`
- Worker C 조치: `check`, `e2e-smoke`, `release` workflow의 `pnpm install`이 모두 `--frozen-lockfile --ignore-scripts`를 사용합니다.
- 영향: lockfile 기반 설치에 더해 lifecycle script 실행을 막아 supply-chain defense-in-depth를 강화했습니다.

남은 조치:

- 꼭 필요한 postinstall이 생기면 CI에서 별도 명시 step으로 허용하고 문서화합니다.

### L-5. Token을 브라우저 저장소에 저장 — 완화 강화됨

- 위치: `web/src/api/client.ts:23-66`, `web/src/api/client.ts:128-134`
- 영향: XSS가 발생하면 token 탈취 가능
- 현재 완화:
  - `dangerouslySetInnerHTML` 미사용
  - `ReactMarkdown`에서 `rehype-raw` 미사용
  - cookie auth가 아니므로 CSRF 위험은 낮음
- Worker E 상태: `localStorage` 기본 동작은 유지하되 `sessionStorage` 저장 옵션을 추가했습니다. token read는 `sessionStorage`를 우선하고 없으면 `localStorage`로 fallback하며, clear는 양쪽 저장소를 모두 삭제합니다.
- 평가: `sessionStorage`는 재시작 후 잔존 위험을 줄이는 defense-in-depth입니다. 단, 실행 중 XSS가 발생하면 두 브라우저 저장소 모두 접근 가능하므로 raw HTML 미사용 및 신뢰된 브라우저 프로필 전제는 유지합니다.
- 문서화 상태: README/OPERATIONS에 token 저장 위치와 브라우저 프로필 신뢰 전제를 local/session 선택 표현으로 갱신했습니다.

남은 조치:

- 추가 코드 조치는 없습니다. 공유 장비에서는 작업 후 토큰 삭제를 운영 절차로 유지합니다.

## Accepted Risks / 설계상 한계

### A-1. CLI agent는 사용자 OS 권한으로 실행됨

- Codex/Claude/Gemini CLI process가 앱과 같은 OS user 권한으로 실행됩니다.
- sandbox/RBAC는 현재 장기 후보로 분류되어 있고, 로컬 단일 사용자 신뢰 모델에서는 수용 가능한 결정입니다.
- 단, 외부에서 가져온 콘텐츠를 issue/comment로 넣고 agent가 실행하는 흐름은 prompt injection과 OS 권한 문제가 연결될 수 있습니다.

문서화 상태:

- README/OPERATIONS에 “agent process는 dashboard와 같은 OS 사용자 권한 및 workspace cwd에서 실행된다”는 전제를 명시했습니다.
- 외부 입력을 Autopilot/auto-chain으로 자동 실행하기 전 비용·권한·prompt injection 리스크를 검토하도록 문서화했습니다.

## 잘 되어 있는 보안 통제

- 외부 바인딩 시 token 필수: `internal/config/config.go:133-135`
- SQL query는 대부분 placeholder 기반이며, 사용자 입력의 raw SQL 삽입 흔적 없음
- React Markdown은 raw HTML 미사용: `web/src/components/MarkdownText.tsx:10-12`
- cookie auth가 없어 CSRF 공격면이 작음
- stdout cap/comment cap/snapshot cap과 pipe drain 구현
- process group kill/timeout/stale recovery/panic cooldown 존재
- strict env parse: 잘못된 env는 startup fail
- backup output 파일은 `0600` 생성: `internal/backup/backup.go`
- `pnpm audit --prod`: known vulnerability 없음
- CI/Release workflow는 Go `1.26.3`을 명시하고, CI는 `govulncheck ./...`를 실행하며, `pnpm install --ignore-scripts`를 일관 적용
- README/OPERATIONS에 strict env, token local/session storage, agent OS 권한, data/log permission, CORS 운영 원칙 문서화
- 현재 작업트리에는 HTTP timeouts, body cap, CORS same-origin default, security headers, private file permissions, generic 500, healthz probe 축소, constant-time token 비교가 반영됨

## 우선순위별 수정 권장

### 즉시 권장

필수 보안 수정/검증 항목은 없습니다.

### 다음 안정화 사이클

1. 실제 운영 1주 동안 Backup API 차단 오탐, session-only token UX 피드백, 예상치 못한 CSP violation을 관찰합니다.

## 결론

보안 관점에서 현재 프로젝트는 **로컬 단일 사용자 앱 기준으로 양호**하며, 이번 병렬 작업과 로컬 Go 업데이트로 주요 hardening 항목과 취약점 재검증까지 완료되었습니다. 남은 작업은 기능 구현이 아니라 실제 운영 1주 관찰입니다.
