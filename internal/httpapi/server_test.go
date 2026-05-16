package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coreline-ai/cron-agent-dashboard/internal/app"
	"github.com/coreline-ai/cron-agent-dashboard/internal/config"
	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

func TestHTTPAPISmoke(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	body := `{"name":"AI News","slug":"ai-news","identifier_prefix":"NEWS","main_agent":{"name":"NewsLead","runtime":"codex","instructions":"lead"}}`
	res := do(t, h, http.MethodPost, "/api/workspaces", body)
	if res.Code != http.StatusCreated {
		t.Fatalf("create workspace status=%d body=%s", res.Code, res.Body.String())
	}
	var created struct {
		Workspace store.Workspace `json:"workspace"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Workspace.Slug != "ai-news" {
		t.Fatalf("bad workspace: %#v", created.Workspace)
	}

	res = do(t, h, http.MethodPost, "/api/workspaces/ai-news/issues", `{"title":"오늘 뉴스","body":"body"}`)
	if res.Code != http.StatusCreated {
		t.Fatalf("create issue status=%d body=%s", res.Code, res.Body.String())
	}
	var issueResp struct {
		Issue store.Issue `json:"issue"`
		Run   store.Run   `json:"run"`
	}
	if err := json.NewDecoder(res.Body).Decode(&issueResp); err != nil {
		t.Fatal(err)
	}
	if issueResp.Issue.ExecutionStatus != "queued" || issueResp.Issue.Identifier != "NEWS-1" {
		t.Fatalf("bad issue: %#v", issueResp.Issue)
	}
	if issueResp.Run.Status != "queued" || issueResp.Run.IssueID != issueResp.Issue.ID {
		t.Fatalf("bad initial run: %#v", issueResp.Run)
	}

	res = do(t, h, http.MethodPut, "/api/issues/"+issueResp.Issue.ID, `{"body":""}`)
	if res.Code != http.StatusOK {
		t.Fatalf("clear issue body status=%d body=%s", res.Code, res.Body.String())
	}
	var updatedIssueResp struct {
		Issue store.Issue `json:"issue"`
	}
	if err := json.NewDecoder(res.Body).Decode(&updatedIssueResp); err != nil {
		t.Fatal(err)
	}
	if updatedIssueResp.Issue.Body != "" {
		t.Fatalf("body should be cleared, got %#v", updatedIssueResp.Issue)
	}

	res = do(t, h, http.MethodGet, "/api/workspaces/ai-news/issues/NEWS-1", "")
	if res.Code != http.StatusOK {
		t.Fatalf("get issue status=%d body=%s", res.Code, res.Body.String())
	}

	res = do(t, h, http.MethodGet, "/healthz", "")
	if res.Code != http.StatusOK {
		t.Fatalf("healthz status=%d", res.Code)
	}

	res = do(t, h, http.MethodGet, "/api/settings", "")
	if res.Code != http.StatusOK {
		t.Fatalf("settings status=%d body=%s", res.Code, res.Body.String())
	}
	var settingsResp struct {
		RunLifecycle struct {
			HeartbeatIntervalSeconds int `json:"heartbeat_interval_seconds"`
			StaleAfterSeconds        int `json:"stale_after_seconds"`
			StaleScanIntervalSeconds int `json:"stale_scan_interval_seconds"`
		} `json:"run_lifecycle"`
	}
	if err := json.NewDecoder(res.Body).Decode(&settingsResp); err != nil {
		t.Fatal(err)
	}
	if settingsResp.RunLifecycle.HeartbeatIntervalSeconds <= 0 || settingsResp.RunLifecycle.StaleAfterSeconds <= 0 || settingsResp.RunLifecycle.StaleScanIntervalSeconds <= 0 {
		t.Fatalf("bad lifecycle settings: %#v", settingsResp.RunLifecycle)
	}
}

func TestHTTPAPIAutopilotTriggerResponseIncludesResultAndRule(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := store.New(database)
	loc, _ := time.LoadLocation("Asia/Seoul")
	runner := app.NewAutopilotRunner(st, loc)
	h := New(st, config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"}, WithAutopilotReloader(runner))

	ctx := t.Context()
	ws, main, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "AI News",
		Slug:             "ai-news",
		IdentifierPrefix: "NEWS",
		MainAgent:        store.CreateAgentInput{Name: "NewsLead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	rule, err := st.CreateAutopilotRule(ctx, ws.ID, store.UpsertAutopilotInput{
		Name:               "daily",
		CronExpr:           "0 9 * * *",
		IssueTitleTemplate: "{{date}} 뉴스",
		IssueBodyTemplate:  "body",
		AssigneeAgentID:    main.ID,
		Enabled:            true,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	res := do(t, h, http.MethodPost, "/api/autopilot/"+rule.ID+"/trigger", `{}`)
	if res.Code != http.StatusCreated {
		t.Fatalf("trigger status=%d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		TriggerResult store.AutopilotTriggerResult `json:"trigger_result"`
		Rule          store.AutopilotRule          `json:"rule"`
		Issue         store.Issue                  `json:"issue"`
		Run           store.Run                    `json:"run"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.TriggerResult.OK || payload.TriggerResult.Issue == nil || payload.TriggerResult.Run == nil {
		t.Fatalf("bad trigger_result: %#v", payload.TriggerResult)
	}
	if payload.Issue.ID == "" || payload.Run.ID == "" || payload.Rule.LastTriggeredIssueID != payload.Issue.ID {
		t.Fatalf("response should include legacy issue/run and updated rule: %#v", payload)
	}
}

func TestHTTPAPIAutopilotTriggerFailurePersistsRuleState(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := store.New(database)
	loc, _ := time.LoadLocation("Asia/Seoul")
	runner := app.NewAutopilotRunner(st, loc)
	h := New(st, config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"}, WithAutopilotReloader(runner))

	ctx := t.Context()
	ws, main, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "AI News",
		Slug:             "ai-news",
		IdentifierPrefix: "NEWS",
		MainAgent:        store.CreateAgentInput{Name: "NewsLead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	rule, err := st.CreateAutopilotRule(ctx, ws.ID, store.UpsertAutopilotInput{
		Name:               "daily",
		CronExpr:           "0 9 * * *",
		IssueTitleTemplate: "{{date}} 뉴스",
		AssigneeAgentID:    main.ID,
		Enabled:            true,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, `UPDATE autopilot_rule SET issue_title_template='{{workspace}} 뉴스' WHERE id=?`, rule.ID); err != nil {
		t.Fatalf("corrupt template: %v", err)
	}

	res := do(t, h, http.MethodPost, "/api/autopilot/"+rule.ID+"/trigger", `{}`)
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("trigger failure status=%d body=%s", res.Code, res.Body.String())
	}
	if !bytes.Contains(res.Body.Bytes(), []byte(`"code":"AUTOPILOT_TRIGGER_FAILED"`)) || !bytes.Contains(res.Body.Bytes(), []byte(`"trigger_result"`)) {
		t.Fatalf("failure response missing trigger result: %s", res.Body.String())
	}
	reloaded, err := st.GetAutopilotRule(ctx, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ConsecutiveFailures != 1 || reloaded.LastError == "" {
		t.Fatalf("failure state not persisted: %#v", reloaded)
	}
}

func TestHTTPAPICancelQueuedRun(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	canceller := &recordingRunCanceller{}
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"}, WithRunCanceller(canceller))

	body := `{"name":"AI News","slug":"ai-news","identifier_prefix":"NEWS","main_agent":{"name":"NewsLead","runtime":"codex","instructions":"lead"}}`
	res := do(t, h, http.MethodPost, "/api/workspaces", body)
	if res.Code != http.StatusCreated {
		t.Fatalf("create workspace status=%d body=%s", res.Code, res.Body.String())
	}
	res = do(t, h, http.MethodPost, "/api/workspaces/ai-news/issues", `{"title":"오늘 뉴스","body":"body"}`)
	if res.Code != http.StatusCreated {
		t.Fatalf("create issue status=%d body=%s", res.Code, res.Body.String())
	}
	var issueResp struct {
		Issue store.Issue `json:"issue"`
		Run   store.Run   `json:"run"`
	}
	if err := json.NewDecoder(res.Body).Decode(&issueResp); err != nil {
		t.Fatal(err)
	}

	res = do(t, h, http.MethodPost, "/api/issues/"+issueResp.Issue.ID+"/cancel", `{}`)
	if res.Code != http.StatusOK {
		t.Fatalf("cancel queued status=%d body=%s", res.Code, res.Body.String())
	}
	var cancelResp struct {
		Run struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"run"`
		CancelRequested bool `json:"cancel_requested"`
	}
	if err := json.NewDecoder(res.Body).Decode(&cancelResp); err != nil {
		t.Fatal(err)
	}
	if cancelResp.CancelRequested {
		t.Fatal("queued run should be cancelled in-store without process cancel request")
	}
	if cancelResp.Run.ID != issueResp.Run.ID || cancelResp.Run.Status != "cancelled" {
		t.Fatalf("bad cancelled run: %#v", cancelResp.Run)
	}
	if got := canceller.cancelledRunIDs; len(got) != 1 || got[0] != issueResp.Run.ID {
		t.Fatalf("queued cancel should record worker cancel intent before DB fallback, got %#v", got)
	}
	if got := canceller.forgottenRunIDs; len(got) != 1 || got[0] != issueResp.Run.ID {
		t.Fatalf("unclaimed queued cancel should clean pending worker intent, got %#v", got)
	}
}

func TestHTTPAPICancelRunningFallbackKeepsPendingIntent(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := store.New(database)
	canceller := &recordingRunCanceller{}
	h := New(st, config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"}, WithRunCanceller(canceller))

	body := `{"name":"AI News","slug":"ai-news","identifier_prefix":"NEWS","main_agent":{"name":"NewsLead","runtime":"codex","instructions":"lead"}}`
	res := do(t, h, http.MethodPost, "/api/workspaces", body)
	if res.Code != http.StatusCreated {
		t.Fatalf("create workspace status=%d body=%s", res.Code, res.Body.String())
	}
	res = do(t, h, http.MethodPost, "/api/workspaces/ai-news/issues", `{"title":"오늘 뉴스","body":"body"}`)
	if res.Code != http.StatusCreated {
		t.Fatalf("create issue status=%d body=%s", res.Code, res.Body.String())
	}
	var issueResp struct {
		Issue store.Issue `json:"issue"`
		Run   store.Run   `json:"run"`
	}
	if err := json.NewDecoder(res.Body).Decode(&issueResp); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ClaimNextRun(t.Context(), "worker"); err != nil || !ok {
		t.Fatalf("claim run ok=%v err=%v", ok, err)
	}

	res = do(t, h, http.MethodPost, "/api/issues/"+issueResp.Issue.ID+"/cancel", `{}`)
	if res.Code != http.StatusOK {
		t.Fatalf("cancel running status=%d body=%s", res.Code, res.Body.String())
	}
	var cancelResp struct {
		Run struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"run"`
		CancelRequested bool `json:"cancel_requested"`
	}
	if err := json.NewDecoder(res.Body).Decode(&cancelResp); err != nil {
		t.Fatal(err)
	}
	if cancelResp.CancelRequested {
		t.Fatal("inactive fake canceller should force DB fallback")
	}
	if cancelResp.Run.ID != issueResp.Run.ID || cancelResp.Run.Status != "cancelled" {
		t.Fatalf("bad cancelled run: %#v", cancelResp.Run)
	}
	if got := canceller.cancelledRunIDs; len(got) != 1 || got[0] != issueResp.Run.ID {
		t.Fatalf("running fallback should still record worker cancel intent, got %#v", got)
	}
	if len(canceller.forgottenRunIDs) != 0 {
		t.Fatalf("claimed run pending intent must remain for worker registration race, got %#v", canceller.forgottenRunIDs)
	}
}

type recordingRunCanceller struct {
	cancelledRunIDs []string
	forgottenRunIDs []string
}

func (c *recordingRunCanceller) CancelRun(runID string) bool {
	c.cancelledRunIDs = append(c.cancelledRunIDs, runID)
	return false
}

func (c *recordingRunCanceller) ForgetPendingCancel(runID string) bool {
	c.forgottenRunIDs = append(c.forgottenRunIDs, runID)
	return true
}

func TestHTTPAPIAuthCORSAndStaticFallback(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cfg := config.Config{
		DataDir:  dir,
		DBPath:   filepath.Join(dir, "data.db"),
		Bind:     "127.0.0.1:0",
		Token:    "secret",
		CORS:     []string{"http://allowed.local"},
		Workers:  1,
		Timezone: "Asia/Seoul",
	}
	h := New(store.New(database), cfg)

	res := do(t, h, http.MethodGet, "/api/settings", "")
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status=%d body=%s", res.Code, res.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Origin", "http://evil.local")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("unexpected origin status=%d body=%s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Origin", "http://allowed.local")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("allowed origin status=%d body=%s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Access-Control-Allow-Origin"); got != "http://allowed.local" {
		t.Fatalf("Access-Control-Allow-Origin=%q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/w/foo/issues/NEWS-1", nil)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("static fallback status=%d body=%s", res.Code, res.Body.String())
	}
	if ct := res.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("content-type=%q", ct)
	}
	if got := res.Header().Get("Content-Security-Policy"); got != contentSecurityPolicy {
		t.Fatalf("static enforced CSP header=%q, want %q", got, contentSecurityPolicy)
	}

	req = httptest.NewRequest(http.MethodGet, "/assets/index.js", nil)
	req.Header.Set("Origin", "http://example.com")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code == http.StatusForbidden {
		t.Fatalf("same-origin static asset was blocked by CORS")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/not-found", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("unknown api status=%d body=%s", res.Code, res.Body.String())
	}
	if !bytes.Contains(res.Body.Bytes(), []byte(`"code":"NOT_FOUND"`)) {
		t.Fatalf("unknown api did not return JSON error: %s", res.Body.String())
	}
}

func TestHTTPAPICORSEmptyAllowlistAllowsOnlySameOrigin(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", CORS: nil, Workers: 1, Timezone: "Asia/Seoul"})

	req := httptest.NewRequest(http.MethodGet, "http://app.local/api/settings", nil)
	req.Header.Set("Origin", "http://evil.local")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("empty CORS allowlist should reject cross-origin request, status=%d body=%s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("rejected origin should not get Access-Control-Allow-Origin, got %q", got)
	}

	req = httptest.NewRequest(http.MethodOptions, "http://app.local/api/settings", nil)
	req.Header.Set("Origin", "http://evil.local")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("empty CORS allowlist should reject cross-origin preflight, status=%d body=%s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodOptions, "http://app.local/api/settings", nil)
	req.Header.Set("Origin", "http://app.local")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("same-origin preflight status=%d body=%s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Access-Control-Allow-Origin"); got != "http://app.local" {
		t.Fatalf("same-origin Access-Control-Allow-Origin=%q", got)
	}
	if got := res.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("missing security header X-Content-Type-Options=%q", got)
	}
	if got := res.Header().Get("Content-Security-Policy"); got != contentSecurityPolicy {
		t.Fatalf("enforced CSP header=%q, want %q", got, contentSecurityPolicy)
	}
	if got := res.Header().Get("Content-Security-Policy-Report-Only"); got != contentSecurityPolicy {
		t.Fatalf("report-only CSP header=%q, want %q", got, contentSecurityPolicy)
	}
}

func TestHTTPAPIBodyTooLarge(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	body := `{"name":"` + strings.Repeat("a", maxJSONBodyBytes) + `"}`
	res := do(t, h, http.MethodPost, "/api/workspaces", body)
	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large body status=%d body=%s", res.Code, res.Body.String())
	}
	if !bytes.Contains(res.Body.Bytes(), []byte(`"code":"REQUEST_TOO_LARGE"`)) {
		t.Fatalf("large body response missing code: %s", res.Body.String())
	}
}

func TestWriteStoreErrorHidesInternalDetails(t *testing.T) {
	res := httptest.NewRecorder()
	writeStoreError(res, errors.New("sqlite failed at /secret/path/data.db"))
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if !bytes.Contains(res.Body.Bytes(), []byte(`"message":"internal server error"`)) {
		t.Fatalf("generic message missing: %s", res.Body.String())
	}
	if bytes.Contains(res.Body.Bytes(), []byte("secret")) || bytes.Contains(res.Body.Bytes(), []byte("sqlite")) {
		t.Fatalf("internal detail leaked: %s", res.Body.String())
	}
}

func TestHTTPAPIHealthzDoesNotExposeRuntimeProbe(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	res := do(t, h, http.MethodGet, "/healthz", "")
	if res.Code != http.StatusOK {
		t.Fatalf("healthz status=%d body=%s", res.Code, res.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"status", "version", "uptime_seconds", "db_ok"} {
		if _, ok := payload[field]; !ok {
			t.Fatalf("healthz missing %q: %#v", field, payload)
		}
	}
	if _, ok := payload["available_runtimes"]; ok {
		t.Fatalf("healthz should not expose runtime probe field: %#v", payload)
	}
}

func TestHTTPAPIRunListDoesNotExposeStdoutPath(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	body := `{"name":"AI News","slug":"ai-news","identifier_prefix":"NEWS","main_agent":{"name":"NewsLead","runtime":"codex","instructions":"lead"}}`
	res := do(t, h, http.MethodPost, "/api/workspaces", body)
	if res.Code != http.StatusCreated {
		t.Fatalf("create workspace status=%d body=%s", res.Code, res.Body.String())
	}
	res = do(t, h, http.MethodPost, "/api/workspaces/ai-news/issues", `{"title":"오늘 뉴스","body":"body"}`)
	if res.Code != http.StatusCreated {
		t.Fatalf("create issue status=%d body=%s", res.Code, res.Body.String())
	}
	var issueResp struct {
		Issue store.Issue `json:"issue"`
	}
	if err := json.NewDecoder(res.Body).Decode(&issueResp); err != nil {
		t.Fatal(err)
	}
	res = do(t, h, http.MethodGet, "/api/issues/"+issueResp.Issue.ID+"/runs", "")
	if res.Code != http.StatusOK {
		t.Fatalf("list runs status=%d body=%s", res.Code, res.Body.String())
	}
	if bytes.Contains(res.Body.Bytes(), []byte("stdout_path")) {
		t.Fatalf("run list leaked stdout_path: %s", res.Body.String())
	}
	if bytes.Contains(res.Body.Bytes(), []byte(`"Valid"`)) || bytes.Contains(res.Body.Bytes(), []byte(`"Int64"`)) {
		t.Fatalf("run list leaked database nullable internals: %s", res.Body.String())
	}
}

func TestHTTPAPIRunEvents(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := store.New(database)
	h := New(st, config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	body := `{"name":"AI News","slug":"ai-news","identifier_prefix":"NEWS","main_agent":{"name":"NewsLead","runtime":"codex","instructions":"lead"}}`
	res := do(t, h, http.MethodPost, "/api/workspaces", body)
	if res.Code != http.StatusCreated {
		t.Fatalf("create workspace status=%d body=%s", res.Code, res.Body.String())
	}
	res = do(t, h, http.MethodPost, "/api/workspaces/ai-news/issues", `{"title":"오늘 뉴스","body":"body"}`)
	if res.Code != http.StatusCreated {
		t.Fatalf("create issue status=%d body=%s", res.Code, res.Body.String())
	}
	var issueResp struct {
		Run store.Run `json:"run"`
	}
	if err := json.NewDecoder(res.Body).Decode(&issueResp); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ClaimNextRun(t.Context(), "worker"); err != nil || !ok {
		t.Fatalf("claim run ok=%v err=%v", ok, err)
	}

	res = do(t, h, http.MethodGet, "/api/runs/"+issueResp.Run.ID+"/events", "")
	if res.Code != http.StatusOK {
		t.Fatalf("events status=%d body=%s", res.Code, res.Body.String())
	}
	var eventsResp struct {
		Events []store.RunEvent `json:"events"`
	}
	if err := json.NewDecoder(res.Body).Decode(&eventsResp); err != nil {
		t.Fatal(err)
	}
	if len(eventsResp.Events) < 2 || eventsResp.Events[0].Seq != 1 || eventsResp.Events[0].EventType != store.RunEventQueued || eventsResp.Events[1].EventType != store.RunEventClaimed {
		t.Fatalf("bad events response: %#v", eventsResp.Events)
	}

	res = do(t, h, http.MethodGet, "/api/runs/missing/events", "")
	if res.Code != http.StatusNotFound {
		t.Fatalf("missing run events status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestHTTPAPIBackupCreatesSQLiteCopy(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	database, err := db.OpenAndMigrate(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: dbPath, Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	backupPath := filepath.Join(dir, "backups", "nested", "backup.db")
	res := do(t, h, http.MethodPost, "/api/system/backup", jsonBody(t, map[string]string{"to": backupPath}))
	if res.Code != http.StatusOK {
		t.Fatalf("backup status=%d body=%s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file not created: %v", err)
	}
	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("backup file is empty")
	}
}

func TestHTTPAPIBackupWithoutDestinationKeepsDefaultBackupPath(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	database, err := db.OpenAndMigrate(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: dbPath, Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	res := do(t, h, http.MethodPost, "/api/system/backup", `{}`)
	if res.Code != http.StatusOK {
		t.Fatalf("backup status=%d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		BackupPath string `json:"backup_path"`
		SizeBytes  int64  `json:"size_bytes"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(payload.BackupPath, dbPath+".") || !strings.HasSuffix(payload.BackupPath, ".bak") {
		t.Fatalf("backup path=%q, want default path based on %q", payload.BackupPath, dbPath)
	}
	if payload.SizeBytes <= 0 {
		t.Fatalf("size_bytes=%d, want positive", payload.SizeBytes)
	}
	if _, err := os.Stat(payload.BackupPath); err != nil {
		t.Fatalf("default backup file not created: %v", err)
	}
}

func TestHTTPAPIBackupRejectsDestinationOutsideBackupDir(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	database, err := db.OpenAndMigrate(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: dbPath, Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	outsidePath := filepath.Join(dir, "outside.db")
	res := do(t, h, http.MethodPost, "/api/system/backup", jsonBody(t, map[string]string{"to": outsidePath}))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("outside backup status=%d body=%s", res.Code, res.Body.String())
	}
	if !bytes.Contains(res.Body.Bytes(), []byte(`"code":"VALIDATION_ERROR"`)) {
		t.Fatalf("outside backup response missing validation code: %s", res.Body.String())
	}
	if _, err := os.Stat(outsidePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside backup path should not be created, stat err=%v", err)
	}

	traversalPath := filepath.Join("..", "traversal.db")
	res = do(t, h, http.MethodPost, "/api/system/backup", jsonBody(t, map[string]string{"to": traversalPath}))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("traversal backup status=%d body=%s", res.Code, res.Body.String())
	}
	if !bytes.Contains(res.Body.Bytes(), []byte(`"code":"VALIDATION_ERROR"`)) {
		t.Fatalf("traversal backup response missing validation code: %s", res.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "traversal.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("traversal target should not be created, stat err=%v", err)
	}
}

func TestHTTPAPIBackupAllowsArbitraryDestinationWhenOptedIn(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	database, err := db.OpenAndMigrate(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{
		DataDir:                   dir,
		DBPath:                    dbPath,
		Bind:                      "127.0.0.1:0",
		Workers:                   1,
		Timezone:                  "Asia/Seoul",
		AllowArbitraryBackupPaths: true,
	})

	backupPath := filepath.Join(dir, "power-user", "backup.db")
	res := do(t, h, http.MethodPost, "/api/system/backup", jsonBody(t, map[string]string{"to": backupPath}))
	if res.Code != http.StatusOK {
		t.Fatalf("backup status=%d body=%s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file not created: %v", err)
	}
}

func TestHTTPAPIVacuum(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	database, err := db.OpenAndMigrate(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: dbPath, Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	res := do(t, h, http.MethodPost, "/api/system/vacuum", `{}`)
	if res.Code != http.StatusOK {
		t.Fatalf("vacuum status=%d body=%s", res.Code, res.Body.String())
	}
	if !bytes.Contains(res.Body.Bytes(), []byte("reclaimed_bytes")) {
		t.Fatalf("vacuum response missing reclaimed_bytes: %s", res.Body.String())
	}
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

func jsonBody(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRuntimeCompatibilityWarning(t *testing.T) {
	if runtimeVersionSupported("") {
		t.Fatal("empty version should be marked unsupported")
	}
	if runtimeCompatibilityWarning("") == "" {
		t.Fatal("empty version should return a user-facing warning")
	}
	if !runtimeVersionSupported("codex 1.2.3") {
		t.Fatal("non-empty version should be supported by best-effort sanity check")
	}
	if runtimeCompatibilityWarning("codex 1.2.3") != "" {
		t.Fatal("non-empty version should not warn without a known minimum version")
	}
}

func TestHTTPAPIUsageSummary(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := store.New(database)
	h := New(st, config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	ctx := t.Context()
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{Name: "Usage", Slug: "usage", IdentifierPrefix: "USG", MainAgent: store.CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"}})
	if err != nil {
		t.Fatal(err)
	}
	_, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, store.CreateIssueInput{Title: "usage"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "worker"); err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if _, err := st.CompleteRunWithReason(ctx, run.ID, store.FinishRunInput{ExitCode: 0, Content: "done", InputTokens: 100, OutputTokens: 50, TotalCostMicros: 1234, TerminalReason: store.TerminalReasonCompleted}); err != nil {
		t.Fatal(err)
	}

	res := do(t, h, http.MethodGet, "/api/usage/summary?days=30", "")
	if res.Code != http.StatusOK {
		t.Fatalf("usage status=%d body=%s", res.Code, res.Body.String())
	}
	var got struct {
		Days  int                   `json:"days"`
		Usage store.RunUsageSummary `json:"usage"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Days != 30 || got.Usage.TotalTokens != 150 || got.Usage.TotalCostMicros != 1234 || got.Usage.MeasuredRunCount != 1 {
		t.Fatalf("bad usage response: %#v", got)
	}

	res = do(t, h, http.MethodGet, "/api/usage/summary?days=0", "")
	if res.Code != http.StatusBadRequest {
		t.Fatalf("bad days should be rejected, status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestHTTPAPIAgentInstructionVersions(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := store.New(database)
	h := New(st, config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	ctx := t.Context()
	ws, main, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{Name: "Audit", Slug: "audit", IdentifierPrefix: "AUD", MainAgent: store.CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead v1"}})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := st.UpdateAgent(ctx, main.ID, store.CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead v2"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.WorkspaceID != ws.ID {
		t.Fatalf("bad update: %#v", updated)
	}

	res := do(t, h, http.MethodGet, "/api/agents/"+main.ID+"/instructions", "")
	if res.Code != http.StatusOK {
		t.Fatalf("list instructions status=%d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		Versions []store.AgentInstructionVersion `json:"versions"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Versions) != 2 || payload.Versions[0].Version != 2 || payload.Versions[0].Instructions != "lead v2" {
		t.Fatalf("bad versions payload: %#v", payload.Versions)
	}
}
