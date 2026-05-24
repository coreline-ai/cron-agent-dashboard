package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/coreline-ai/cron-agent-dashboard/internal/app"
	backupops "github.com/coreline-ai/cron-agent-dashboard/internal/backup"
	"github.com/coreline-ai/cron-agent-dashboard/internal/config"
	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
	"github.com/coreline-ai/cron-agent-dashboard/internal/httpapi"
	"github.com/coreline-ai/cron-agent-dashboard/internal/scheduler"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
	"github.com/coreline-ai/cron-agent-dashboard/internal/worker"
	workerruntime "github.com/coreline-ai/cron-agent-dashboard/internal/worker/runtime"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	cmd := "serve"
	args := os.Args[1:]
	// Surface the linker-injected version (httpapi.Version) so release
	// pipelines can sanity-check the binary without booting the database.
	if len(args) > 0 {
		switch args[0] {
		case "--version", "-v", "version":
			fmt.Printf("cron-agent-dashboard %s\n", httpapi.Version)
			return
		}
	}
	if len(args) > 0 && args[0] != "--help" && args[0] != "-h" && args[0][0] != '-' {
		cmd = args[0]
		args = args[1:]
	}
	cfg, _, err := config.Load(args)
	if err != nil {
		log.Fatal(err)
	}
	if err := config.EnsureDirs(cfg); err != nil {
		log.Fatal(err)
	}
	if cmd == "import" {
		cmd = "restore"
	}
	if cmd == "export" {
		cmd = "backup"
	}
	if cmd == "restore" {
		if err := restoreDatabase(cfg); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("restored %s from %s\n", cfg.DBPath, cfg.RestoreFrom)
		return
	}
	database, err := db.OpenAndMigrate(cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	st := store.New(database, store.WithAutopilotFailureDisableThreshold(cfg.AutopilotFailureDisableThreshold))
	switch cmd {
	case "init":
		fmt.Printf("initialized %s\n", cfg.DBPath)
	case "serve":
		if err := serve(cfg, st); err != nil {
			log.Fatal(err)
		}
	case "backup", "export":
		path, err := backupDatabase(cfg, database)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("backup written to %s\n", path)
	case "seed":
		if err := seedExample(st); err != nil {
			log.Fatal(err)
		}
	case "seed-lab":
		if err := seedMultiAgentLab(cfg, st); err != nil {
			log.Fatal(err)
		}
	case "seed-dev-team":
		if err := seedDevTeam(cfg, st); err != nil {
			log.Fatal(err)
		}
	case "workspace-export":
		if err := workspaceExportCmd(cfg, st); err != nil {
			log.Fatal(err)
		}
	case "workspace-import":
		if err := workspaceImportCmd(cfg, st); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unknown command %q (expected serve, init, backup, restore, export, import, seed, seed-lab, seed-dev-team, workspace-export, or workspace-import)", cmd)
	}
}

func workspaceExportCmd(cfg config.Config, st *store.Store) error {
	if cfg.WorkspaceSlug == "" {
		return fmt.Errorf("workspace-export: --workspace <slug> is required")
	}
	if cfg.BackupTo == "" {
		return fmt.Errorf("workspace-export: --to <file.json> is required")
	}
	export, err := app.ExportWorkspaceWithOptions(context.Background(), st, cfg.WorkspaceSlug, app.ExportWorkspaceOptions{
		IncludeHistory: cfg.WorkspaceExportIncludeHistory,
		MaskPII:        cfg.WorkspaceExportMaskPII,
	})
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return fmt.Errorf("workspace-export: marshal: %w", err)
	}
	if err := os.WriteFile(cfg.BackupTo, data, 0o600); err != nil {
		return fmt.Errorf("workspace-export: write %s: %w", cfg.BackupTo, err)
	}
	fmt.Printf("workspace %q exported to %s (%d agents, %d skills, %d autopilot rules, %d issues, %d comments, %d runs, %d attachments)\n",
		export.Workspace.Slug, cfg.BackupTo,
		len(export.Agents), len(export.Skills), len(export.Autopilot),
		len(export.Issues), len(export.Comments), len(export.Runs), len(export.Attachments),
	)
	return nil
}

func workspaceImportCmd(cfg config.Config, st *store.Store) error {
	if cfg.RestoreFrom == "" {
		return fmt.Errorf("workspace-import: --from <file.json> is required")
	}
	data, err := os.ReadFile(cfg.RestoreFrom)
	if err != nil {
		return fmt.Errorf("workspace-import: read %s: %w", cfg.RestoreFrom, err)
	}
	var export app.WorkspaceExport
	if err := json.Unmarshal(data, &export); err != nil {
		return fmt.Errorf("workspace-import: parse %s: %w", cfg.RestoreFrom, err)
	}
	ws, err := app.ImportWorkspace(context.Background(), st, export, app.ImportOptions{
		DestSlug:       cfg.WorkspaceDestSlug,
		IncludeHistory: cfg.WorkspaceExportIncludeHistory,
	})
	if err != nil {
		return err
	}
	fmt.Printf("imported workspace %q (slug=%s) from %s\n", ws.Name, ws.Slug, cfg.RestoreFrom)
	return nil
}

func seedExample(st *store.Store) error {
	result, err := app.SeedExample(context.Background(), st)
	if err != nil {
		return err
	}
	if result.AlreadyHad {
		fmt.Printf("workspace %q already seeded (slug=%s) — nothing to do\n", result.Workspace.Name, result.Workspace.Slug)
		return nil
	}
	workerNames := make([]string, 0, len(result.Worker))
	for _, w := range result.Worker {
		workerNames = append(workerNames, w.Name)
	}
	fmt.Printf("seeded workspace %q (slug=%s) — main agent: %s, workers: %s, auto_chain_enabled=true\n",
		result.Workspace.Name,
		result.Workspace.Slug,
		result.MainAgent.Name,
		strings.Join(workerNames, ", "),
	)
	return nil
}

func seedMultiAgentLab(cfg config.Config, st *store.Store) error {
	workingDir := normalizeLabWorkingDir(cfg.LabWorkingDir)
	if workingDir == "" {
		workingDir = detectNearestGitRoot()
	}
	result, err := app.SeedMultiAgentLab(context.Background(), st, app.MultiAgentLabOptions{WorkingDir: workingDir})
	if err != nil {
		return err
	}

	createdWorkspaces := 0
	createdAgents := 0
	createdIssues := 0
	for _, lab := range result.Workspaces {
		if !lab.AlreadyHad {
			createdWorkspaces++
		}
		createdAgents += lab.CreatedAgentCount
		createdIssues += lab.CreatedIssueCount
	}
	fmt.Printf("seeded multi-agent lab — workspaces=%d created=%d existing=%d new_workers=%d new_issues=%d\n",
		len(result.Workspaces),
		createdWorkspaces,
		len(result.Workspaces)-createdWorkspaces,
		createdAgents,
		createdIssues,
	)
	if workingDir == "" {
		fmt.Println("lab working_dir: default per-workspace workdirs (pass --lab-working-dir /path/to/repo for real code runs)")
	} else {
		fmt.Printf("lab working_dir: %s\n", workingDir)
	}
	for _, lab := range result.Workspaces {
		workerNames := make([]string, 0, len(lab.Worker))
		for _, worker := range lab.Worker {
			workerNames = append(workerNames, worker.Name)
		}
		status := "created"
		if lab.AlreadyHad {
			status = "existing"
		}
		fmt.Printf("- %-24s [%s] main=%s workers=%s issue=%s\n",
			lab.Workspace.Slug,
			status,
			lab.MainAgent.Name,
			strings.Join(workerNames, ", "),
			firstIssueIdentifier(lab.Issues),
		)
	}
	return nil
}

func normalizeLabWorkingDir(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func detectNearestGitRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	current, err := filepath.Abs(cwd)
	if err != nil {
		current = cwd
	}
	for {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func firstIssueIdentifier(issues []store.Issue) string {
	if len(issues) == 0 {
		return "-"
	}
	return issues[0].Identifier
}

func seedDevTeam(cfg config.Config, st *store.Store) error {
	workingDir := normalizeLabWorkingDir(cfg.DevTeamWorkingDir)
	if workingDir == "" {
		workingDir = detectNearestGitRoot()
	}
	result, err := app.SeedDevTeam(context.Background(), st, cfg.DevTeamSlug, workingDir)
	if err != nil {
		return err
	}
	status := "created"
	if result.AlreadyHad {
		status = "existing"
	}
	fmt.Printf("seeded dev-team workspace %q (slug=%s, %s) — agents=%d new_agents=%d skills=%d assignments=%d\n",
		result.Workspace.Name,
		result.Workspace.Slug,
		status,
		len(result.Agents),
		result.CreatedAgentCount,
		len(result.Skills),
		result.AssignmentCount,
	)
	if workingDir == "" {
		fmt.Println("dev-team working_dir: default per-workspace workdir (pass --working-dir /path/to/repo for real code runs)")
	} else {
		fmt.Printf("dev-team working_dir: %s\n", workingDir)
	}
	return nil
}

func backupDatabase(cfg config.Config, database interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}) (string, error) {
	result, err := backupops.Database(context.Background(), database, cfg.DBPath, cfg.BackupTo, time.Now().UTC())
	return result.Path, err
}

func restoreDatabase(cfg config.Config) error {
	_, err := backupops.Restore(cfg.RestoreFrom, cfg.DBPath, time.Now().UTC())
	return err
}

func serve(cfg config.Config, st *store.Store) error {
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	report, err := app.RunStartupSelfCheck(runCtx, st)
	if err != nil {
		return err
	}
	log.Printf("startup self-check ok: integrity=%s journal_mode=%s foreign_keys=%t busy_timeout_ms=%d workspaces=%d foreign_key_violations=%d orphan_process_groups_terminated=%d orphan_process_groups_skipped=%d orphan_runs_recovered=%d migration_failures=%d",
		report.IntegrityCheck,
		report.JournalMode,
		report.ForeignKeysEnabled,
		report.BusyTimeoutMS,
		report.WorkspaceCount,
		report.ForeignKeyViolationCount,
		report.OrphanProcessGroupsTerminated,
		report.OrphanProcessGroupsSkipped,
		report.OrphanRunsRecovered,
		report.MigrationFailureCount,
	)

	workerStore := app.NewWorkerStore(st,
		app.WithDefaultWorkDir(filepath.Join(cfg.DataDir, "workdirs")),
		app.WithDataDir(cfg.DataDir),
	)
	executor := app.NewRuntimeExecutor(
		workerruntime.DefaultAdapters(),
		filepath.Join(cfg.DataDir, "runs"),
		app.WithRunProcessMarker(st),
	)
	pool := worker.NewPool(workerStore, executor, worker.WithPoolSize(cfg.Workers))
	if err := pool.Start(runCtx); err != nil {
		return err
	}
	loc, err := scheduler.LoadLocation(cfg.Timezone)
	if err != nil {
		return err
	}
	autopilot := app.NewAutopilotRunner(st, loc)
	if err := autopilot.Reload(runCtx); err != nil {
		return err
	}

	// Wire the in-process IssueEventBus so AppendRunEvent commits wake the
	// SSE handler directly instead of relying on the keep-alive poll. The
	// bus also drives the HTTP layer's optional WithIssueEventBus hook.
	// The workspace resolver lets workspace-scoped subscribers (Run feed,
	// chain dashboard) wake on any issue's event in the same workspace.
	issueEventBus := app.NewIssueEventBus(app.WithWorkspaceResolver(func(issueID string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var workspaceID string
		_ = st.DB().GetContext(ctx, &workspaceID, `SELECT workspace_id FROM issue WHERE id=?`, issueID)
		return workspaceID
	}))
	st.SetRunEventNotifier(issueEventBus)

	maintenance := app.NewMaintenanceRunner(st.DB(), app.MaintenanceConfig{
		DataDir:            cfg.DataDir,
		DBPath:             cfg.DBPath,
		AutoBackup:         cfg.AutoBackup,
		AutoBackupKeep:     cfg.AutoBackupKeep,
		AutoCleanupLogDays: cfg.AutoCleanupLogDays,
		WorktreeGCAfter:    cfg.WorktreeGCAfter,
		WorktreePruneGuard: st.IsRunWorktreeGCProtected,
		Interval:           cfg.MaintenanceInterval,
		OnReport: func(report app.MaintenanceReport, _ error) {
			// Persist the log-cleanup tally so the Settings UI can show
			// "마지막 log cleanup at <time> — <files>개 / <bytes>". A timeout
			// keeps the maintenance loop from stalling if the DB is busy.
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			at := time.Now().UTC().Format(time.RFC3339)
			_ = st.SetSystemState(ctx, store.SystemStateLastLogCleanupAt, at)
			_ = st.SetSystemState(ctx, store.SystemStateLastLogCleanupFiles, fmt.Sprintf("%d", report.DeletedLogFiles))
			_ = st.SetSystemState(ctx, store.SystemStateLastLogCleanupBytes, fmt.Sprintf("%d", report.FreedLogBytes))
			_ = st.SetSystemState(ctx, store.SystemStateWorktreeBytes, fmt.Sprintf("%d", report.WorktreeBytes))
			_ = st.SetSystemState(ctx, store.SystemStateWorktreeDirCount, fmt.Sprintf("%d", report.WorktreeDirCount))
			_ = st.SetSystemState(ctx, store.SystemStateWorktreePruned, fmt.Sprintf("%d", report.PrunedWorktrees))
			_ = st.SetSystemState(ctx, store.SystemStateWorktreeMeasuredAt, at)
		},
	})
	maintenance.Start(runCtx)

	webhooks := app.NewWebhookDispatcher(st)
	webhooks.Start(runCtx)

	srv := &http.Server{
		Addr:              cfg.Bind,
		Handler:           httpapi.New(st, cfg, httpapi.WithRunCanceller(pool), httpapi.WithAutopilotReloader(autopilot), httpapi.WithIssueEventBus(issueEventBus)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("cron-agent-dashboard listening on http://%s", cfg.Bind)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-runCtx.Done():
	case err := <-errCh:
		if err != nil {
			return err
		}
	}
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := autopilot.Stop(shutdownCtx); err != nil {
		return err
	}
	if err := maintenance.Stop(shutdownCtx); err != nil {
		return err
	}
	if err := webhooks.Stop(shutdownCtx); err != nil {
		return err
	}
	if err := pool.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return nil
}
