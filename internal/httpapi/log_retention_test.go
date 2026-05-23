package httpapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/coreline-ai/cron-agent-dashboard/internal/config"
	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

// Track E of dev-plan/implement_20260522_212332.md.
//
// /api/settings must surface last_log_cleanup_at / files / bytes so the
// Settings UI can render the "마지막 cleanup" stamp without a side-channel.
// /api/system/cleanup-logs must persist those values so the row updates the
// instant an operator triggers a manual sweep.
func TestSettingsExposesLastLogCleanupAfterManualSweep(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{
		DataDir:            dir,
		DBPath:             filepath.Join(dir, "data.db"),
		Bind:               "127.0.0.1:0",
		Workers:            1,
		Timezone:           "Asia/Seoul",
		AutoCleanupLogDays: 30,
		WorktreeGCAfter:    48 * time.Hour,
	})

	pristine := do(t, h, http.MethodGet, "/api/settings", "")
	if pristine.Code != http.StatusOK {
		t.Fatalf("settings pristine status=%d body=%s", pristine.Code, pristine.Body.String())
	}
	var pristineBody struct {
		Maintenance map[string]any `json:"maintenance"`
	}
	if err := json.NewDecoder(pristine.Body).Decode(&pristineBody); err != nil {
		t.Fatal(err)
	}
	if got, _ := pristineBody.Maintenance["auto_cleanup_log_days"].(float64); int(got) != 30 {
		t.Fatalf("auto_cleanup_log_days=%v want 30", pristineBody.Maintenance["auto_cleanup_log_days"])
	}
	if got, _ := pristineBody.Maintenance["worktree_gc_after_seconds"].(float64); int(got) != int((48*time.Hour)/time.Second) {
		t.Fatalf("worktree_gc_after_seconds=%v want %d", pristineBody.Maintenance["worktree_gc_after_seconds"], int((48*time.Hour)/time.Second))
	}
	if at, _ := pristineBody.Maintenance["last_log_cleanup_at"].(string); at != "" {
		t.Fatalf("pristine last_log_cleanup_at should be empty, got %q", at)
	}

	// Manual cleanup updates the system_state row.
	cleanup := do(t, h, http.MethodPost, "/api/system/cleanup-logs", `{"days":1}`)
	if cleanup.Code != http.StatusOK {
		t.Fatalf("cleanup status=%d body=%s", cleanup.Code, cleanup.Body.String())
	}

	after := do(t, h, http.MethodGet, "/api/settings", "")
	if after.Code != http.StatusOK {
		t.Fatalf("settings after-sweep status=%d body=%s", after.Code, after.Body.String())
	}
	var afterBody struct {
		Maintenance map[string]any `json:"maintenance"`
	}
	if err := json.NewDecoder(after.Body).Decode(&afterBody); err != nil {
		t.Fatal(err)
	}
	at, _ := afterBody.Maintenance["last_log_cleanup_at"].(string)
	if at == "" {
		t.Fatalf("settings should expose last_log_cleanup_at after a manual sweep, got empty")
	}
	files, _ := afterBody.Maintenance["last_log_cleanup_files"].(string)
	if files == "" {
		t.Fatalf("settings should expose last_log_cleanup_files (string), got empty")
	}
	bytes, _ := afterBody.Maintenance["last_log_cleanup_bytes"].(string)
	if bytes == "" {
		t.Fatalf("settings should expose last_log_cleanup_bytes (string), got empty")
	}
}
