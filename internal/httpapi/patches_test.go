package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coreline-ai/cron-agent-dashboard/internal/config"
	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
	"github.com/coreline-ai/cron-agent-dashboard/internal/worker/runtime"
)

// F1: workspace create auto-populates working_dir when empty.
func TestCreateWorkspaceAutoPopulatesWorkingDir(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	// working_dir omitted on purpose
	body := `{"name":"Auto WD","slug":"auto-wd","identifier_prefix":"AWD","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`
	res := do(t, h, http.MethodPost, "/api/workspaces", body)
	if res.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", res.Code, res.Body.String())
	}
	var created struct {
		Workspace store.Workspace `json:"workspace"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	wantPath := filepath.Join(dir, "workdirs", "auto-wd")
	if created.Workspace.WorkingDir != wantPath {
		t.Fatalf("working_dir = %q, want %q", created.Workspace.WorkingDir, wantPath)
	}
	if info, err := os.Stat(wantPath); err != nil || !info.IsDir() {
		t.Fatalf("expected directory at %s, stat err=%v", wantPath, err)
	}
}

// F1: explicit working_dir is preserved and created if missing.
func TestCreateWorkspaceCreatesExplicitWorkingDir(t *testing.T) {
	dir := t.TempDir()
	explicit := filepath.Join(dir, "custom", "explicit-wd")
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	body := `{"name":"Explicit WD","slug":"explicit-wd","identifier_prefix":"EXP","working_dir":"` + explicit + `","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`
	res := do(t, h, http.MethodPost, "/api/workspaces", body)
	if res.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", res.Code, res.Body.String())
	}
	if info, err := os.Stat(explicit); err != nil || !info.IsDir() {
		t.Fatalf("explicit dir not created: %v", err)
	}
}

// F2: codex/claude/gemini adapters surface a sentinel error when working_dir
// is missing instead of letting the CLI fail with `os error 2`.
func TestRuntimeAdaptersReturnSentinelOnMissingWorkingDir(t *testing.T) {
	cases := []struct {
		name    string
		adapter runtime.RuntimeAdapter
	}{
		{"codex", runtime.CodexAdapter{Executable: "codex"}},
		{"claude", runtime.ClaudeAdapter{Executable: "claude"}},
		{"gemini", runtime.GeminiAdapter{Executable: "gemini"}},
	}
	for _, tc := range cases {
		_, _, err := tc.adapter.BuildCommand(t.Context(), runtime.RunContext{RunID: "r1", Prompt: "p"})
		if err == nil {
			t.Fatalf("%s: expected error when working_dir missing", tc.name)
		}
		if !errors.Is(err, runtime.ErrWorkspaceWorkingDirMissing) {
			t.Fatalf("%s: error is not ErrWorkspaceWorkingDirMissing: %v", tc.name, err)
		}
		if !strings.Contains(err.Error(), "run_id=r1") {
			t.Fatalf("%s: error should mention run id, got: %v", tc.name, err)
		}
	}
}

// F4: createWorkspace fills default retry_policy_json when omitted.
func TestCreateWorkspaceMainAgentGetsRetryPolicyDefault(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	body := `{"name":"RP","slug":"retry-default","identifier_prefix":"RPD","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`
	res := do(t, h, http.MethodPost, "/api/workspaces", body)
	if res.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", res.Code, res.Body.String())
	}
	var created struct {
		MainAgent store.Agent `json:"main_agent"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(created.MainAgent.RetryPolicyJSON, `"max_attempts":3`) {
		t.Fatalf("main_agent.retry_policy_json should default to 3 attempts, got %q", created.MainAgent.RetryPolicyJSON)
	}
	if !strings.Contains(created.MainAgent.RetryPolicyJSON, `"backoff_seconds":[10,60,300]`) {
		t.Fatalf("main_agent.retry_policy_json missing backoff: %q", created.MainAgent.RetryPolicyJSON)
	}
}

// F4: createAgent fills default retry_policy_json when omitted.
func TestCreateAgentGetsRetryPolicyDefault(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	wsBody := `{"name":"AG","slug":"agent-default","identifier_prefix":"AGD","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`
	if res := do(t, h, http.MethodPost, "/api/workspaces", wsBody); res.Code != http.StatusCreated {
		t.Fatalf("seed workspace: %s", res.Body.String())
	}

	agentBody := `{"name":"Writer","runtime":"claude","instructions":"write"}`
	res := do(t, h, http.MethodPost, "/api/workspaces/agent-default/agents", agentBody)
	if res.Code != http.StatusCreated {
		t.Fatalf("create agent status=%d body=%s", res.Code, res.Body.String())
	}
	var got struct {
		Agent store.Agent `json:"agent"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Agent.RetryPolicyJSON, `"max_attempts":3`) {
		t.Fatalf("agent retry_policy_json should default to 3 attempts, got %q", got.Agent.RetryPolicyJSON)
	}
}

// F4: explicit retry_policy_json is preserved (no overwrite by default).
func TestCreateAgentPreservesExplicitRetryPolicy(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	wsBody := `{"name":"AG","slug":"agent-explicit","identifier_prefix":"AGE","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`
	if res := do(t, h, http.MethodPost, "/api/workspaces", wsBody); res.Code != http.StatusCreated {
		t.Fatalf("seed workspace: %s", res.Body.String())
	}

	explicit := `{"max_attempts":1,"retry_on":["timeout"]}`
	agentBody := `{"name":"Tight","runtime":"claude","instructions":"x","retry_policy_json":` + jsonEscape(explicit) + `}`
	res := do(t, h, http.MethodPost, "/api/workspaces/agent-explicit/agents", agentBody)
	if res.Code != http.StatusCreated {
		t.Fatalf("create agent: %s", res.Body.String())
	}
	var got struct {
		Agent store.Agent `json:"agent"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Agent.RetryPolicyJSON != explicit {
		t.Fatalf("explicit retry_policy_json overwritten: %q", got.Agent.RetryPolicyJSON)
	}
}

// F3: new workspaces default to auto_close_on_run_done=false so multi-stage
// collaboration does not auto-close the parent issue when one agent finishes.
func TestCreateWorkspaceDefaultsAutoCloseFalse(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	body := `{"name":"NoAutoClose","slug":"no-auto-close","identifier_prefix":"NAC","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`
	res := do(t, h, http.MethodPost, "/api/workspaces", body)
	if res.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", res.Code, res.Body.String())
	}
	var created struct {
		Workspace store.Workspace `json:"workspace"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Workspace.AutoCloseOnRunDone {
		t.Fatalf("new workspaces should default to auto_close_on_run_done=false, got true")
	}
}

// F3: explicit auto_close_on_run_done=true respected.
func TestCreateWorkspaceRespectsAutoCloseTrue(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	body := `{"name":"AutoCloseOpt","slug":"auto-close-opt","identifier_prefix":"ACO","auto_close_on_run_done":true,"main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`
	res := do(t, h, http.MethodPost, "/api/workspaces", body)
	if res.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", res.Code, res.Body.String())
	}
	var created struct {
		Workspace store.Workspace `json:"workspace"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if !created.Workspace.AutoCloseOnRunDone {
		t.Fatalf("auto_close_on_run_done=true should be honored")
	}
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// TestUpdateAgentIsFullReplacePinsContract pins the published behavior of
// PUT /api/agents/:id: the handler treats the request body as a complete
// agent replacement, so any field omitted from the payload is reset to its
// zero value. docs/API.md §2.5 must reflect this; clients that need a
// partial update must read-modify-write the full resource.
func TestUpdateAgentIsFullReplacePinsContract(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	wsBody := `{"name":"AG","slug":"agent-put","identifier_prefix":"AGP","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`
	if res := do(t, h, http.MethodPost, "/api/workspaces", wsBody); res.Code != http.StatusCreated {
		t.Fatalf("seed workspace: %s", res.Body.String())
	}

	createBody := `{"name":"Writer","runtime":"claude","instructions":"write","summary":"initial summary","tags":"alpha,beta"}`
	res := do(t, h, http.MethodPost, "/api/workspaces/agent-put/agents", createBody)
	if res.Code != http.StatusCreated {
		t.Fatalf("create agent: %s", res.Body.String())
	}
	var created struct {
		Agent store.Agent `json:"agent"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Agent.Summary != "initial summary" || created.Agent.Tags != "alpha,beta" {
		t.Fatalf("seed agent did not carry summary/tags: %#v", created.Agent)
	}

	// PUT payload omits summary and tags on purpose. Under partial-update
	// semantics summary/tags would survive; under full-replace semantics they
	// are reset to "". The current handler is full-replace.
	putBody := `{"name":"Writer","runtime":"claude","instructions":"write"}`
	res = do(t, h, http.MethodPut, "/api/agents/"+created.Agent.ID, putBody)
	if res.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", res.Code, res.Body.String())
	}
	var after struct {
		Agent store.Agent `json:"agent"`
	}
	if err := json.NewDecoder(res.Body).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if after.Agent.Summary != "" {
		t.Fatalf("PUT contract drift: omitted summary should be cleared to \"\" (full replace), got %q", after.Agent.Summary)
	}
	if after.Agent.Tags != "" {
		t.Fatalf("PUT contract drift: omitted tags should be cleared to \"\" (full replace), got %q", after.Agent.Tags)
	}
}
