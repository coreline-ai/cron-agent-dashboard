# cron-agent-dashboard v0.1.0

Release date: 2026-05-14

## Summary

v0.1.0 is the first local MVP release of Cron Agent Dashboard: a single-binary, SQLite-backed dashboard for running and tracking personal AI agent work. It focuses on a dependable daily loop: create an issue, dispatch a CLI agent, review results in comments, delegate with explicit mentions, and schedule recurring work with Autopilot.

## Install and run

### Option A — Download a release artifact

1. Download the artifact for your OS/architecture from the GitHub Release assets.
2. Make it executable and move it onto your PATH.

```bash
chmod +x cron-agent-dashboard-<os>-<arch>
sudo mv cron-agent-dashboard-<os>-<arch> /usr/local/bin/cron-agent-dashboard
```

### Option B — Build from source

```bash
pnpm install --frozen-lockfile --ignore-scripts
make build VERSION=v0.1.0
```

### First run

```bash
cron-agent-dashboard init
cron-agent-dashboard serve --workers 3 --timezone Asia/Seoul
```

Then open `http://127.0.0.1:8080`.

Default data directory: `~/.cron-agent-dashboard/`

## Highlights

- **Single local process**: Go API server, SQLite store, worker pool, scheduler, and embedded Vite React UI run from one binary.
- **Issue-based agent workflow**: create workspaces, define CLI agents, open issues, collect markdown results, and rerun or cancel active work.
- **Explicit mention delegation**: user comments such as `@Writer ...` enqueue a new run on the same issue without changing the issue assignee.
- **Run lifecycle hardening**: runs now expose heartbeat, terminal reason, failure kind, and cancel reason fields for clearer operations.
- **Run event timeline**: each run records an audit timeline, visible in the issue detail page and available through `GET /api/runs/:id/events`.
- **Prompt safety / trigger snapshots**: each run records why it was triggered, including a capped snapshot of the trigger content. Agent result mentions do not auto-chain in this release.
- **Autopilot visibility and safety**: rules show last error, consecutive failures, and last triggered issue; a rule auto-disables after five consecutive trigger failures.
- **Standardized UI feedback**: shared status pills, confirmation dialogs, toasts, error alerts, and date/time rendering improve consistency.
- **Operations tools**: settings page and API support DB backup, SQLite vacuum, and old run-log cleanup.
- **Release automation**: `make release-build` creates darwin/linux artifacts; GitHub Release workflow uploads binaries and `SHA256SUMS`.

## Operational notes

- Back up `~/.cron-agent-dashboard/data.db` before replacing the binary or restoring data.
- Use token mode when binding outside localhost:

```bash
cron-agent-dashboard serve --bind 0.0.0.0:8080 --token '<strong-token>'
```

- Check Autopilot rules after missed schedules. A rule with repeated trigger failures can be disabled automatically after the fifth consecutive failure.
- Use the issue detail run timeline to distinguish normal completion, CLI exit failures, user cancellation, stale recovery, and orphan recovery.
- Run logs live under `~/.cron-agent-dashboard/runs/`; clean old files from Settings or `POST /api/system/cleanup-logs`.

## Verification commands

```bash
make check
pnpm --filter web test
make e2e-smoke
make release-build VERSION=v0.1.0
```

The GitHub Release workflow runs the build/check path and uploads checksummed artifacts. Run the Vitest command locally when validating frontend component behavior.

## Known limitations in v0.1.0

- Homebrew installation was planned but not part of the initial v0.1.0 release. Current `[Unreleased]` builds add release formula rendering and secret-gated Homebrew tap PR automation.
- Realtime streaming was not implemented in the initial v0.1.0 release; the UI used polling. Current `[Unreleased]` builds add token-compatible fetch-based SSE subscribers for issue run events, run logs, and workspace run wake-ups.
- Auto-chain from agent-written mentions, per-run worktrees, workspace import/export history materialization, attachments, webhooks, SSE realtime streaming, and Homebrew release/tap automation were future work in the initial v0.1.0 release; they are now implemented in `[Unreleased]` for the single-user scope.
- Run stdout cleanup was manual in v0.1.0; current builds add automatic retention reporting/settings visibility while preserving logs after issue deletion for audit/debugging until explicit cleanup.

See also: [CHANGELOG](../CHANGELOG.md) and [Operations checklist](OPERATIONS.md).
