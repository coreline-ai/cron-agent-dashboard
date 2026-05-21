package main

import (
	"context"
	"database/sql"
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
	default:
		log.Fatalf("unknown command %q (expected serve, init, backup, restore, export, import, or seed)", cmd)
	}
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

	workerStore := app.NewWorkerStore(st, app.WithDefaultWorkDir(filepath.Join(cfg.DataDir, "workdirs")))
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

	maintenance := app.NewMaintenanceRunner(st.DB(), app.MaintenanceConfig{
		DataDir:            cfg.DataDir,
		DBPath:             cfg.DBPath,
		AutoBackup:         cfg.AutoBackup,
		AutoBackupKeep:     cfg.AutoBackupKeep,
		AutoCleanupLogDays: cfg.AutoCleanupLogDays,
		Interval:           cfg.MaintenanceInterval,
	})
	maintenance.Start(runCtx)

	srv := &http.Server{
		Addr:              cfg.Bind,
		Handler:           httpapi.New(st, cfg, httpapi.WithRunCanceller(pool), httpapi.WithAutopilotReloader(autopilot)),
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
	if err := pool.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return nil
}
