package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreline-ai/cron-agent-dashboard/internal/app"
	backupops "github.com/coreline-ai/cron-agent-dashboard/internal/backup"
	dbmeta "github.com/coreline-ai/cron-agent-dashboard/internal/db"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
	"github.com/coreline-ai/cron-agent-dashboard/internal/worker"
	"github.com/go-chi/chi/v5"
)

var errBackupDestinationOutsideDataDir = errors.New("backup destination must be a file inside data-dir/backups unless allow-arbitrary-backup-paths is enabled")

func (s *Server) registerSystemRoutes(api chi.Router) {
	api.Get("/api/settings", s.settings)
	api.Get("/api/usage/summary", s.usageSummary)
	api.Post("/api/system/backup", s.backup)
	api.Post("/api/system/vacuum", s.vacuum)
	api.Post("/api/system/cleanup-logs", s.cleanupLogs)
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	dbOK := s.store.DB().PingContext(r.Context()) == nil
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": Version, "uptime_seconds": int64(time.Since(s.startedAt).Seconds()), "db_ok": dbOK})
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	usage, _ := s.store.RunUsageSummary(r.Context(), time.Now().Add(-7*24*time.Hour).UTC().Format(time.RFC3339Nano))
	migrationFailures, _ := dbmeta.RecentMigrationFailures(r.Context(), s.store.DB(), 5)
	lastCleanupAt, _ := s.store.GetSystemState(r.Context(), store.SystemStateLastLogCleanupAt)
	lastCleanupFiles, _ := s.store.GetSystemState(r.Context(), store.SystemStateLastLogCleanupFiles)
	lastCleanupBytes, _ := s.store.GetSystemState(r.Context(), store.SystemStateLastLogCleanupBytes)
	worktreeBytes, _ := s.store.GetSystemState(r.Context(), store.SystemStateWorktreeBytes)
	worktreeDirs, _ := s.store.GetSystemState(r.Context(), store.SystemStateWorktreeDirCount)
	worktreePruned, _ := s.store.GetSystemState(r.Context(), store.SystemStateWorktreePruned)
	worktreeAt, _ := s.store.GetSystemState(r.Context(), store.SystemStateWorktreeMeasuredAt)
	writeJSON(w, http.StatusOK, map[string]any{
		"version":              Version,
		"data_dir":             s.cfg.DataDir,
		"available_runtimes":   availableRuntimes(),
		"worker_pool_size":     s.cfg.Workers,
		"auth_mode":            s.cfg.AuthMode(),
		"timezone":             s.cfg.Timezone,
		"usage_7d":             usage,
		"migration_failures":   migrationFailures,
		"migration_fail_count": len(migrationFailures),
		"maintenance": map[string]any{
			"auto_backup":                         s.cfg.AutoBackup,
			"auto_backup_keep":                    s.cfg.AutoBackupKeep,
			"auto_cleanup_log_days":               s.cfg.AutoCleanupLogDays,
			"interval_seconds":                    int64(s.cfg.MaintenanceInterval / time.Second),
			"autopilot_failure_disable_threshold": s.cfg.AutopilotFailureDisableThreshold,
			"last_log_cleanup_at":                 lastCleanupAt,
			"last_log_cleanup_files":              lastCleanupFiles,
			"last_log_cleanup_bytes":              lastCleanupBytes,
			"worktree_bytes":                      worktreeBytes,
			"worktree_dir_count":                  worktreeDirs,
			"worktree_pruned_last_pass":           worktreePruned,
			"worktree_measured_at":                worktreeAt,
		},
		"run_lifecycle": map[string]any{
			"heartbeat_interval_seconds":  int(worker.DefaultHeartbeatInterval / time.Second),
			"stale_after_seconds":         int(worker.DefaultStaleAfter / time.Second),
			"stale_scan_interval_seconds": int(worker.DefaultStaleScanInterval / time.Second),
		},
	})
}

func (s *Server) usageSummary(w http.ResponseWriter, r *http.Request) {
	days := 7
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > 365 {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "days must be between 1 and 365", nil)
			return
		}
		days = parsed
	}
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	usage, err := s.store.RunUsageSummary(r.Context(), since)
	respond(w, map[string]any{"usage": usage, "days": days}, err, http.StatusOK)
}

func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		To string `json:"to"`
	}
	if !decodeOptional(w, r, &req) {
		return
	}
	destPath := strings.TrimSpace(req.To)
	if destPath != "" && !s.cfg.AllowArbitraryBackupPaths {
		var err error
		destPath, err = constrainedBackupDestination(s.cfg.DataDir, destPath)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
			return
		}
	}
	result, err := backupops.Database(r.Context(), s.store.DB(), s.cfg.DBPath, destPath, time.Now().UTC())
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"backup_path": result.Path, "size_bytes": result.SizeBytes})
}

func constrainedBackupDestination(dataDir, requested string) (string, error) {
	backupDir := filepath.Join(dataDir, "backups")
	if !filepath.IsAbs(requested) {
		requested = filepath.Join(backupDir, requested)
	}
	baseAbs, err := filepath.Abs(filepath.Clean(backupDir))
	if err != nil {
		return "", err
	}
	destAbs, err := filepath.Abs(filepath.Clean(requested))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(baseAbs, destAbs)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return "", errBackupDestinationOutsideDataDir
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errBackupDestinationOutsideDataDir
	}
	return destAbs, nil
}

func (s *Server) vacuum(w http.ResponseWriter, r *http.Request) {
	before, _ := sqliteDBSizeBytes(r.Context(), s.store)
	_, err := s.store.DB().ExecContext(r.Context(), `VACUUM`)
	after, _ := sqliteDBSizeBytes(r.Context(), s.store)
	reclaimed := before - after
	if reclaimed < 0 {
		reclaimed = 0
	}
	respond(w, map[string]any{"reclaimed_bytes": reclaimed}, err, http.StatusOK)
}

func sqliteDBSizeBytes(ctx context.Context, st *store.Store) (int64, error) {
	var pageCount, pageSize int64
	if err := st.DB().GetContext(ctx, &pageCount, `PRAGMA page_count`); err != nil {
		return 0, err
	}
	if err := st.DB().GetContext(ctx, &pageSize, `PRAGMA page_size`); err != nil {
		return 0, err
	}
	return pageCount * pageSize, nil
}

func (s *Server) cleanupLogs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Days int `json:"days"`
	}
	if !decodeOptional(w, r, &req) {
		return
	}
	if req.Days <= 0 {
		req.Days = 30
	}
	cutoff := time.Now().Add(-time.Duration(req.Days) * 24 * time.Hour)
	report, err := app.CleanupRunLogs(s.cfg.DataDir, cutoff)
	// Mirror the persistent state writes so manual cleanup also updates the
	// Settings UI's "last log cleanup at" card. Failures are non-fatal; the
	// caller already sees the report in the HTTP response.
	now := time.Now().UTC().Format(time.RFC3339)
	_ = s.store.SetSystemState(r.Context(), store.SystemStateLastLogCleanupAt, now)
	_ = s.store.SetSystemState(r.Context(), store.SystemStateLastLogCleanupFiles, strconv.Itoa(report.DeletedFiles))
	_ = s.store.SetSystemState(r.Context(), store.SystemStateLastLogCleanupBytes, strconv.FormatInt(report.FreedBytes, 10))
	respond(w, map[string]any{"deleted_files": report.DeletedFiles, "freed_bytes": report.FreedBytes}, err, http.StatusOK)
}

type RuntimeInfo struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Path      string `json:"path"`
	Supported bool   `json:"supported"`
	Warning   string `json:"warning,omitempty"`
}

var runtimeInfoCache = struct {
	sync.Mutex
	infos     []RuntimeInfo
	expiresAt time.Time
}{}

func availableRuntimes() []RuntimeInfo {
	now := time.Now()
	runtimeInfoCache.Lock()
	if now.Before(runtimeInfoCache.expiresAt) {
		infos := cloneRuntimeInfos(runtimeInfoCache.infos)
		runtimeInfoCache.Unlock()
		return infos
	}
	runtimeInfoCache.Unlock()

	names := []string{"codex", "claude", "gemini"}
	out := []RuntimeInfo{}
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			version := runtimeVersion(context.Background(), p)
			out = append(out, RuntimeInfo{Name: n, Path: p, Version: version, Supported: runtimeVersionSupported(version), Warning: runtimeCompatibilityWarning(version)})
		}
	}

	runtimeInfoCache.Lock()
	runtimeInfoCache.infos = cloneRuntimeInfos(out)
	runtimeInfoCache.expiresAt = now.Add(5 * time.Second)
	runtimeInfoCache.Unlock()
	return out
}

func runtimeVersionSupported(version string) bool {
	return strings.TrimSpace(version) != ""
}

func runtimeCompatibilityWarning(version string) string {
	if strings.TrimSpace(version) == "" {
		return "--version 확인 실패: CLI 설치와 비대화형 실행 인자 호환성을 확인하세요."
	}
	return ""
}

func cloneRuntimeInfos(in []RuntimeInfo) []RuntimeInfo {
	out := make([]RuntimeInfo, len(in))
	copy(out, in)
	return out
}

func runtimeVersion(ctx context.Context, path string) string {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	b, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
