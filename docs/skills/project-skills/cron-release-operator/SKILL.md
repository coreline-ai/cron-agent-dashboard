---
name: cron-release-operator
description: Prepare cron-agent-dashboard changes for safe local release by checking docs, migrations, CI, embedded web build, naming consistency, and smoke-test readiness. Use before commit/push, release tagging, README updates, or when finishing a stabilization cycle.
triggers: [release, commit, push, changelog, README, migration, CI, e2e, make check, docs]
---
# Cron Release Operator

## 목적

이 skill은 코드가 이미 구현된 뒤 릴리스 가능한 상태인지 점검하는 운영 마무리 지침이다. 기능 확장보다 문서 정합성과 재현 가능한 검증을 우선한다.

## 릴리스 전 점검

1. **Working tree**
   - `git status --short`로 의도하지 않은 파일이 없는지 확인한다.
   - 기능 변경, 문서 변경, 생성 산출물은 의미 단위로 분리한다.

2. **Naming consistency**
   - 저장소 slug, Go module, binary, release URL, README 제목이 `cron-agent-dashboard` 기준으로 일치하는지 확인한다.
   - 과거 `corn` 표기가 재발하지 않았는지 grep한다.

3. **Docs sync**
   - API/DB/ARCHITECTURE/OPERATIONS/README가 실제 구현과 충돌하지 않는지 확인한다.
   - 새 migration이 있으면 DATA_MODEL 또는 운영 문서에 필요한 설명을 반영한다.

4. **Validation commands**
   - 기본: `go test ./...`, `go vet ./...`, `pnpm --filter web build`
   - 릴리스 후보: `make check`
   - 동시성 변경: `go test -race ./...`
   - UI 변경: Playwright smoke 또는 로컬 browser QA

5. **Release readiness**
   - embedded static이 최신인지 `make build` 또는 `make check` 흐름으로 확인한다.
   - GitHub Actions 실패 원인이 있으면 로컬 재현 로그를 남긴다.

## 출력 형식

1. `릴리스 판정`: 가능 / 보류
2. `변경 요약`: code/docs/tests
3. `검증 명령`: 성공/실패와 핵심 로그
4. `남은 항목`: 출시 차단 / 출시 후
