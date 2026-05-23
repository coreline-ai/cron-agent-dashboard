# Changelog

All notable changes to this project are documented in this file.

This project follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) style sections and uses semantic versioning for release tags.

## [Unreleased]

### Added (2026-05-21 cycle)

- **2026-05-20 chain stabilization**: workspace main agent (PM hub) can now re-enter the same auto-chain ([`294fe2e`](https://github.com/coreline-ai/cron-agent-dashboard/commit/294fe2e)); UTF-8 safe truncation through `capSnapshot` + `CapCommentForLogWithStatus` and runtime CLI diagnostic noise (`MCP issues detected. Run /mcp list for status.`) strip applied across codex / claude / gemini ([`25c1ec2`](https://github.com/coreline-ai/cron-agent-dashboard/commit/25c1ec2)); stripped diagnostic lines recorded as `stdout_sanitized` run_event via migration `0017` ([`e619548`](https://github.com/coreline-ai/cron-agent-dashboard/commit/e619548)).
- **2026-05-21 quality cycle**: `ListIssues` execution filter moved to SQL WHERE so LIMIT does not drop matching rows ([`27021cc`](https://github.com/coreline-ai/cron-agent-dashboard/commit/27021cc)); Gemini adapter routes the prompt body through stdin (was argv) to keep prompts out of `/proc/<pid>/cmdline` ([`13bc9ed`](https://github.com/coreline-ai/cron-agent-dashboard/commit/13bc9ed)); Settings UI now exposes `auto_close_on_run_done` toggle per workspace ([`9770c05`](https://github.com/coreline-ai/cron-agent-dashboard/commit/9770c05)); auto-chain agent lookup splits `ErrNotFound` from transient store errors with distinct system comments ([`e458c05`](https://github.com/coreline-ai/cron-agent-dashboard/commit/e458c05)); `PUT /api/agents/:id` full-replace contract pinned in docs + test.
- **2026-05-21 hub-PM + UX cycle**: main agent auto-chain re-entry no longer advances `chain_depth` so hub-PM workflows fit inside the default `max_depth=5` ([`1ff2b93`](https://github.com/coreline-ai/cron-agent-dashboard/commit/1ff2b93)); agent runtime selectors show recommendation/warning badges sourced from `available_runtimes` ([`a22abdb`](https://github.com/coreline-ai/cron-agent-dashboard/commit/a22abdb)); workspace create form exposes `auto_chain_enabled` toggle inline ([`89260cc`](https://github.com/coreline-ai/cron-agent-dashboard/commit/89260cc)); new `cron-agent-dashboard seed` example workspace command for fresh clones ([`e220913`](https://github.com/coreline-ai/cron-agent-dashboard/commit/e220913)); IssueFlowGraph now renders `chain_depth` and hub re-entry annotations + legend ([`06d3ef8`](https://github.com/coreline-ai/cron-agent-dashboard/commit/06d3ef8)); claude `--print` adapter passes `--input-format text` so stdin prompts no longer hang in interactive mode (this cycle).
- **2026-05-22 Phase 2 closure cycle**: workspace chain dashboard at `/w/:slug/chains`, chain cancel/retry, depth/cost guard visualization, issue attachments with audit/image preview/comment linkage, workspace webhooks with HMAC-SHA256 + `mask_pii` + exponential retry/dead-letter counts, per-run worktree git integration, workspace export history + PII masking, release smoke automation, and run-log retention reporting are now implemented. README/TODO/API docs are synchronized so the active backlog shows no open implementation items.
- **2026-05-23 hardening closure cycle**: issue run_event SSE streaming, Homebrew tap PR publish workflow, workspace history import materialization, per-run worktree disk usage/GC, and required `e2e-full` CI are implemented. Worktree GC is configurable with `--worktree-gc-after` / `CRON_AGENT_DASHBOARD_WORKTREE_GC_AFTER`, and workspace export/import now round-trips `per_run_worktree`.

### Changed (2026-05-21 cycle)

- Bumped web dependencies — `vite ^6.4.2` (was `^5.4.14`) and `vitest@latest` — to clear `pnpm audit` moderate advisories `GHSA-67mh-4wv8-2f99` (esbuild dev server CORS) and `GHSA-4w7w-66w2-5vf9` (Vite `.map` path traversal). `pnpm audit --prod` and full `pnpm audit` are clean.
- README quality metrics refreshed: sentinel error count 6 → 14, automation test/spec files 49 → 50, migrations 16 → 17.
- `docs/API.md` §2.5 `PUT /api/agents/:id` is now explicit about full-replace semantics; partial updates require GET → modify → PUT.

### Migration (2026-05-21 cycle)

- `0017_run_event_stdout_sanitized.sql` — `run_event.event_type` CHECK enum extended with `stdout_sanitized` (table-rebuild pattern, matches `0015`).

### Current limitations / follow-up

- WebSocket hub is intentionally not implemented; the current single-user UI uses SSE for issue detail run_event refresh plus React Query polling fallback.
- Homebrew tap publishing is opt-in: set `HOMEBREW_TAP_REPO` and `HOMEBREW_TAP_TOKEN` secrets to create tap PRs automatically, otherwise the rendered formula remains a GitHub Release artifact.
- Workspace history import restores metadata only. Attachment binaries and run stdout payloads still need an external archive if operators want byte-for-byte restoration.

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

### Known limitations in 0.1.0

- Auto-chain from agent result mentions was not implemented in the initial `0.1.0` release; it is implemented in the current `[Unreleased]` line as workspace opt-in auto-chain.
- Run stdout cleanup was manual in `0.1.0`; the current line adds automatic retention reporting and settings visibility.
- Homebrew distribution, workspace import/export, per-run worktrees, attachments, webhooks, and realtime streaming were future work in `0.1.0`. The current line implements all of them for the single-user scope; Homebrew tap PR publishing remains secret-gated by the operator.
