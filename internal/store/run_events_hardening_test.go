package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestRunEventLargeDetailsAreStoredAsValidTruncationObject(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	_, issue, run := newRunEventHardeningFixture(t, ctx, st)

	appended, err := st.AppendRunEvent(ctx, RunEventInput{
		RunID:     run.ID,
		IssueID:   issue.ID,
		EventType: RunEventStarting,
		Message:   "large detail",
		Details: map[string]any{
			"payload": strings.Repeat("x", runEventDetailJSONMaxBytes*3),
		},
	})
	if err != nil {
		t.Fatalf("append large event: %v", err)
	}

	var raw string
	if err := st.DB().GetContext(ctx, &raw, `SELECT detail_json FROM run_event WHERE id=?`, appended.ID); err != nil {
		t.Fatalf("read raw detail_json: %v", err)
	}
	if len(raw) > runEventDetailJSONMaxBytes {
		t.Fatalf("detail_json len=%d, want <= %d", len(raw), runEventDetailJSONMaxBytes)
	}
	if !json.Valid([]byte(raw)) {
		t.Fatalf("detail_json is not valid JSON: %q", raw)
	}

	events, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	event := findRunEventByID(t, events, appended.ID)
	if got, ok := event.Details["truncated"].(bool); !ok || !got {
		t.Fatalf("truncated flag=%#v, want true in details %#v", event.Details["truncated"], event.Details)
	}
	originalSize, ok := event.Details["original_size_bytes"].(float64)
	if !ok || int(originalSize) <= runEventDetailJSONMaxBytes {
		t.Fatalf("original_size_bytes=%#v, want number > %d", event.Details["original_size_bytes"], runEventDetailJSONMaxBytes)
	}
	preview, ok := event.Details["preview"].(string)
	if !ok || preview == "" {
		t.Fatalf("preview=%#v, want non-empty string", event.Details["preview"])
	}
}

func TestRunEventNormalDetailsRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	_, issue, run := newRunEventHardeningFixture(t, ctx, st)

	appended, err := st.AppendRunEvent(ctx, RunEventInput{
		RunID:     run.ID,
		IssueID:   issue.ID,
		EventType: RunEventStarting,
		Message:   "normal detail",
		Details: map[string]any{
			"runtime": "codex",
			"attempt": 2,
			"dry_run": true,
		},
	})
	if err != nil {
		t.Fatalf("append normal event: %v", err)
	}

	events, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	event := findRunEventByID(t, events, appended.ID)
	if _, ok := event.Details["truncated"]; ok {
		t.Fatalf("normal details should not be replaced by truncation object: %#v", event.Details)
	}
	if event.Details["runtime"] != "codex" || event.Details["attempt"] != float64(2) || event.Details["dry_run"] != true {
		t.Fatalf("normal details did not round-trip: %#v", event.Details)
	}
}

func TestRunEventAppendIssueMismatchAndMissingRunAreSafe(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	_, issue, run := newRunEventHardeningFixture(t, ctx, st)

	if _, err := st.AppendRunEvent(ctx, RunEventInput{
		RunID:     run.ID,
		IssueID:   "not-" + issue.ID,
		EventType: RunEventStarting,
	}); !errors.Is(err, ErrValidation) {
		t.Fatalf("append issue mismatch err=%v, want ErrValidation", err)
	}

	if _, err := st.AppendRunEvent(ctx, RunEventInput{
		RunID:     "missing-run",
		IssueID:   issue.ID,
		EventType: RunEventStarting,
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("append missing run with issue err=%v, want ErrNotFound", err)
	}

	if _, err := st.AppendRunEvent(ctx, RunEventInput{
		RunID:     "missing-run",
		EventType: RunEventStarting,
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("append missing run err=%v, want ErrNotFound", err)
	}
}

func newRunEventHardeningFixture(t *testing.T, ctx context.Context, st *Store) (Workspace, Issue, Run) {
	t.Helper()
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Run Events",
		Slug:             "run-events",
		IdentifierPrefix: "EVT",
		MainAgent: CreateAgentInput{
			Name:         "Runner",
			Runtime:      "codex",
			Instructions: "run",
		},
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	issue, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "event hardening"})
	if err != nil {
		t.Fatalf("create issue/run: %v", err)
	}
	return ws, issue, run
}

func findRunEventByID(t *testing.T, events []RunEvent, id string) RunEvent {
	t.Helper()
	for _, event := range events {
		if event.ID == id {
			return event
		}
	}
	t.Fatalf("run event %s not found in %#v", id, events)
	return RunEvent{}
}
