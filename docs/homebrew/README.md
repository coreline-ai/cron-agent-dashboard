# Homebrew tap publish

`docs/homebrew/cron-agent-dashboard.rb.tmpl` is rendered by the Release workflow into `dist/cron-agent-dashboard.rb`.

## 자동 publish 조건

Release workflow에서 아래 GitHub Actions secret을 설정하면 tap repo에 PR을 생성한다.

| Secret | 값 |
| --- | --- |
| `HOMEBREW_TAP_REPO` | 예: `coreline-ai/homebrew-tap` |
| `HOMEBREW_TAP_TOKEN` | tap repo에 push/PR 생성 권한이 있는 PAT |

secret이 없으면 publish step은 no-op이고, 렌더링된 formula는 GitHub Release artifact로만 업로드된다.

## 수동 렌더링

```bash
make release-build VERSION=v0.1.0
(
  cd dist
  sha256sum cron-agent-dashboard-* > SHA256SUMS
)
./scripts/render-homebrew-formula.sh v0.1.0 dist/SHA256SUMS dist/cron-agent-dashboard.rb
```

렌더러는 release download URL과 binary smoke test에는 입력 tag(`v0.1.0`)를 그대로 쓰고, Homebrew `version` 필드는 선행 `v`를 제거한 값(`0.1.0`)으로 채운다.
