package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	backupops "github.com/coreline-ai/cron-agent-dashboard/internal/backup"
)

type MaintenanceConfig struct {
	DataDir            string
	DBPath             string
	AutoBackup         bool
	AutoBackupKeep     int
	AutoCleanupLogDays int
	Interval           time.Duration
	Now                func() time.Time
	Log                *slog.Logger

	// OnReport is invoked after every successful or partial maintenance pass
	// so the caller can persist the report (e.g. into the system_state table
	// for the Settings UI). The hook is called even when the underlying run
	// returned an error so callers can still record what happened — the err
	// argument is the joined error from the pass.
	OnReport func(report MaintenanceReport, err error)
}

type MaintenanceReport struct {
	BackupPath      string
	BackupSizeBytes int64
	PrunedBackups   int
	DeletedLogFiles int
	FreedLogBytes   int64
}

type MaintenanceRunner struct {
	db      backupops.Checkpointer
	cfg     MaintenanceConfig
	mu      sync.Mutex
	started bool
	done    chan struct{}
}

func NewMaintenanceRunner(db backupops.Checkpointer, cfg MaintenanceConfig) *MaintenanceRunner {
	cfg = normalizeMaintenanceConfig(cfg)
	return &MaintenanceRunner{db: db, cfg: cfg, done: make(chan struct{})}
}

func (r *MaintenanceRunner) Start(ctx context.Context) {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	r.started = true
	r.done = make(chan struct{})
	done := r.done
	r.mu.Unlock()

	go func() {
		defer func() {
			r.mu.Lock()
			if r.done == done {
				r.started = false
			}
			r.mu.Unlock()
			close(done)
		}()
		r.runAndLog(ctx)
		ticker := time.NewTicker(r.cfg.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.runAndLog(ctx)
			}
		}
	}()
}

func (r *MaintenanceRunner) Stop(ctx context.Context) error {
	r.mu.Lock()
	if !r.started {
		r.mu.Unlock()
		return nil
	}
	done := r.done
	r.mu.Unlock()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *MaintenanceRunner) RunOnce(ctx context.Context) (MaintenanceReport, error) {
	return RunMaintenanceOnce(ctx, r.db, r.cfg)
}

func (r *MaintenanceRunner) runAndLog(ctx context.Context) {
	report, err := r.RunOnce(ctx)
	log := r.cfg.Log
	if log == nil {
		log = slog.Default()
	}
	if r.cfg.OnReport != nil {
		r.cfg.OnReport(report, err)
	}
	if err != nil {
		log.Warn("automatic maintenance failed", "error", err)
		return
	}
	log.Info("automatic maintenance completed",
		"backup_path", report.BackupPath,
		"backup_size_bytes", report.BackupSizeBytes,
		"pruned_backups", report.PrunedBackups,
		"deleted_log_files", report.DeletedLogFiles,
		"freed_log_bytes", report.FreedLogBytes,
	)
}

func RunMaintenanceOnce(ctx context.Context, db backupops.Checkpointer, cfg MaintenanceConfig) (MaintenanceReport, error) {
	cfg = normalizeMaintenanceConfig(cfg)
	report := MaintenanceReport{}
	var errs []error
	if cfg.AutoBackup {
		path := automaticBackupPath(cfg.DataDir, cfg.now())
		result, err := backupops.Database(ctx, db, cfg.DBPath, path, cfg.now())
		if err != nil {
			errs = append(errs, err)
		} else {
			report.BackupPath = result.Path
			report.BackupSizeBytes = result.SizeBytes
			pruned, err := PruneBackups(filepath.Join(cfg.DataDir, "backups"), cfg.AutoBackupKeep)
			report.PrunedBackups = pruned
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	if cfg.AutoCleanupLogDays > 0 {
		cutoff := cfg.now().Add(-time.Duration(cfg.AutoCleanupLogDays) * 24 * time.Hour)
		cleanup, err := CleanupRunLogs(cfg.DataDir, cutoff)
		report.DeletedLogFiles = cleanup.DeletedFiles
		report.FreedLogBytes = cleanup.FreedBytes
		if err != nil {
			errs = append(errs, err)
		}
	}
	return report, errors.Join(errs...)
}

type LogCleanupReport struct {
	DeletedFiles int
	FreedBytes   int64
}

func CleanupRunLogs(dataDir string, cutoff time.Time) (LogCleanupReport, error) {
	var report LogCleanupReport
	runsDir := filepath.Join(dataDir, "runs")
	if _, err := os.Stat(runsDir); errors.Is(err, os.ErrNotExist) {
		return report, nil
	}
	_ = os.Chmod(runsDir, 0o700)
	var errs []error
	err := filepath.Walk(runsDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if info == nil || info.IsDir() || !info.ModTime().Before(cutoff) {
			return nil
		}
		size := info.Size()
		if err := os.Remove(p); err != nil {
			errs = append(errs, err)
			return nil
		}
		report.DeletedFiles++
		report.FreedBytes += size
		return nil
	})
	return report, errors.Join(append(errs, err)...)
}

func PruneBackups(dir string, keep int) (int, error) {
	if keep <= 0 {
		keep = 1
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	type backupFile struct {
		path string
		mod  time.Time
	}
	files := []backupFile{}
	var errs []error
	for _, entry := range entries {
		if entry.IsDir() || !isAutoBackupName(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		files = append(files, backupFile{path: filepath.Join(dir, entry.Name()), mod: info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	deleted := 0
	if len(files) <= keep {
		return deleted, errors.Join(errs...)
	}
	for _, file := range files[keep:] {
		if err := os.Remove(file.path); err == nil {
			deleted++
		} else {
			errs = append(errs, err)
		}
	}
	return deleted, errors.Join(errs...)
}

func normalizeMaintenanceConfig(cfg MaintenanceConfig) MaintenanceConfig {
	if cfg.AutoBackupKeep <= 0 {
		cfg.AutoBackupKeep = 7
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 24 * time.Hour
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	return cfg
}

func (c MaintenanceConfig) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}

func automaticBackupPath(dataDir string, at time.Time) string {
	return filepath.Join(dataDir, "backups", "data-"+at.UTC().Format("20060102T150405Z")+".db")
}

func isAutoBackupName(name string) bool {
	return len(name) == len("data-20060102T150405Z.db") && name[:5] == "data-" && filepath.Ext(name) == ".db"
}

var _ backupops.Checkpointer = (*sql.DB)(nil)
