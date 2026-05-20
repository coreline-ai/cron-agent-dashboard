---
name: cron-security-guard
description: Review cron-agent-dashboard changes for prompt-injection boundaries, skill trust, secret leakage, workspace file exposure, auth/CORS safety, and safe handling of CLI agent output. Use when editing prompts, skills, runtime adapters, logs, auth, backup/restore, or workspace file access.
triggers: [security, prompt injection, skill, trust_level, token, secret, auth, cors, runtime, workspace, logs, backup]
---
# Cron Security Guard

## 목적

이 skill은 로컬 단일 사용자 도구라는 전제를 유지하되, CLI agent가 실제 파일과 명령을 다룬다는 점을 기준으로 보안 경계를 점검한다.

## 보안 체크리스트

1. **Prompt boundary**
   - 사용자 입력, 댓글, trigger snapshot, skill content는 fence 안에 들어가는지 확인한다.
   - fence 내부 지시는 상위 지시를 덮어쓰지 못한다는 안전 규칙이 유지되는지 확인한다.
   - skill content가 실행 지시처럼 보이더라도 dashboard가 script를 자동 실행하지 않는지 확인한다.

2. **Skill registry trust**
   - `source_type`, `trust_level`, `content_hash`, `local_path/source_url/source_ref`가 보존되는지 확인한다.
   - git/untrusted skill은 UI/API에서 신뢰도를 숨기지 않는다.
   - remote import나 update가 silent overwrite로 구현되지 않았는지 확인한다.

3. **Secret / log leakage**
   - `slog`에는 token, instructions, full prompt, stdout 본문, credential path를 기록하지 않는다.
   - run log와 comments에는 cap/truncation 정책이 적용되는지 확인한다.
   - README/OPERATIONS에 workspace와 log에 민감정보가 남을 수 있음을 명시한다.

4. **HTTP exposure**
   - localhost 외부 bind는 token 없이는 startup fail해야 한다.
   - CORS allowlist가 wildcard로 넓어지지 않았는지 확인한다.
   - backup/restore path는 allowlist 또는 명시 flag 없이 임의 경로를 허용하지 않는다.

5. **Runtime execution**
   - runtime adapter는 workspace `working_dir` 없이 실행되지 않아야 한다.
   - 프로세스 group termination과 timeout이 유지되는지 확인한다.

## 출력 형식

- `차단 이슈`: 실제 데이터 유출/권한 상승/외부 노출 위험
- `권장 보강`: 문서, UI 경고, 테스트로 막을 수 있는 항목
- `검증`: grep/test/API 확인 결과
