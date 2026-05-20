# Changelog

All notable changes to this project are documented in this file.

This project follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) style sections and uses semantic versioning for release tags.

## [Unreleased]

### Added

- **F1** `POST /api/workspaces` now auto-populates `working_dir` to `<data_dir>/workdirs/<slug>` and creates the directory when the request omits an explicit path. Prevents `os error 2` failures observed with the codex CLI when workspaces were created without a working directory (RFP-1 incident).
- **F2** Runtime adapters (codex/claude/gemini) now return the new sentinel `runtime.ErrWorkspaceWorkingDirMissing` with a clear message (`configure workspace working_dir before running agent`) instead of letting the CLI exit with the unhelpful `No such file or directory (os error 2)`.
- **F3** New workspace column `auto_close_on_run_done` (migration `0016`) and matching field on `Workspace`, `CreateWorkspaceInput`, `UpdateWorkspaceInput`. Multi-step collaboration workspaces (RFP-style) can now opt out so that a single successful agent run does not auto-close the parent issue. **New workspaces default to `false`** (preserves design principle `issue.status ≠ run.status`); existing rows keep the previous behavior (`true`) via migration default.
- **F4** Workspace and agent create handlers default `retry_policy_json` to `{"max_attempts":3,"backoff_seconds":[10,60,300],"retry_on":["timeout","executor_error"]}` when callers omit it, so transient runtime errors auto-recover.

### Changed

- `maybeMarkIssueDoneTx` (`internal/store/runs.go`) now reads `workspace.auto_close_on_run_done` before marking the parent issue done. Existing flows that depended on auto-close are unaffected when the workspace keeps the legacy default.

### Migration

- `0016_workspace_auto_close.sql` — `ALTER TABLE workspace ADD COLUMN auto_close_on_run_done INTEGER NOT NULL DEFAULT 1`. The application layer still defaults newly-created workspaces to `false`.

- Startup orphan process cleanup now uses `process_recorded_at` freshness checks before sending signals, reducing OS process group reuse risk.
- Executor process metadata recording now retries short transient failures before falling back to best-effort logging.
- React Query now refetches on window focus, with explicit refresh buttons on board, issue detail, and Autopilot pages.
- Issue comments now include lightweight agent mention autocomplete for `@AgentName` delegation.
- Autopilot rules now support `snooze_until` temporary pause, UI quick actions, and no-op trigger handling without increasing failure counts.
- Added run resource-control foundation: best-effort token/cost/model metrics capture, timeout resolution, limited transient retry, and Unicode mention matching.
- Added agent instructions version history and run-level instruction-version snapshots for reproducibility and audit.
- Added workspace-level Agent Skills registry with `SKILL.md` parsing, agent skill assignment, `always` / `trigger` / `manual` activation, fenced prompt injection, and `skills_loaded` run events. Registered scripts are not auto-executed.
- README, architecture, TRD, data model, roadmap, and operations docs now describe the shipped startup cleanup and release state more accurately.

## [0.1.0] - 2026-05-14

### Added

- Initial local MVP for a single-user AI agent dashboard: workspaces, agents, issues, comments, explicit `@AgentName` delegation, rerun/cancel controls, and DB-backed Autopilot rules.
- Lifecycle taxonomy for run operations with `heartbeat_at`, `terminal_reason`, `failure_kind`, and `cancel_reason` so completed, failed, cancelled, stale, orphaned, and shutdown paths can be diagnosed consistently.
- Run event timeline backed by the `run_event` audit table and `GET /api/runs/:id/events`, with UI timeline rendering on the issue detail page.
- Prompt safety and trigger snapshot model: explicit-only user mentions, `trigger_type`, `trigger_comment_id`, and 4KB `trigger_content_snapshot` fields preserve dispatch context without enabling automatic agent-result chaining.
- Autopilot failure visibility with `last_error`, `consecutive_failures`, `last_triggered_issue_id`, and automatic rule disablement after five consecutive trigger failures.
- UI standardization pass with shared status pills, confirmation dialogs, toast feedback, mutation error alerts, date/time formatting, board filters, and the issue operations rail.
- Operations endpoints and UI actions for SQLite backup, `VACUUM`, and run log cleanup.
- Vitest component tests for shared frontend UI primitives, alongside Go tests and Playwright smoke coverage.
- Release automation for cross-platform artifact builds, GitHub Release upload, and checksum generation.

### Changed

- The release build now embeds the Vite SPA into the Go binary through `embed.FS`, enabling the dashboard to run as one local process.
- Startup now performs self-checks for SQLite pragmas, foreign keys, main-agent invariants, and orphan run recovery before serving traffic.
- Board and issue-detail views now emphasize operational state through URL filters, execution-status polling, run history, and visible terminal/failure/cancel reasons.

### Security

- Raw HTML/script execution remains disabled in rendered markdown.
- Binding outside localhost requires token mode, preventing accidental unauthenticated LAN exposure.
- Agent result comments and run stdout are capped to prevent browser lockups and pipe blocking.

### Known limitations

- Auto-chain from agent result mentions is not implemented; only direct user comments can dispatch a mention run.
- Run stdout cleanup is manual through settings or `POST /api/system/cleanup-logs`.
- Homebrew distribution, workspace import/export, per-run worktrees, and realtime streaming are future work.
