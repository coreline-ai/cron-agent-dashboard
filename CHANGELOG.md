# Changelog

All notable changes to this project are documented in this file.

This project follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) style sections and uses semantic versioning for release tags.

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
