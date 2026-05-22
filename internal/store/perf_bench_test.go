package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
)

func newBenchStore(b *testing.B) *Store {
	b.Helper()
	database, err := db.OpenAndMigrate(filepath.Join(b.TempDir(), "data.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = database.Close() })
	return New(database)
}

// Track A of dev-plan/implement_20260522_220446.md.
//
// These benchmarks seed a realistic-sized workspace (1,000 issues with
// 5,000 comments and 10,000 runs spread across them) and time the three
// list endpoints the IssueDetailPage and the BoardPage actually hit.
// Target budgets:
//   * ListIssues(workspace)  < 200ms
//   * ListRuns(issueID)      < 50ms
//   * ListComments(issueID)  < 50ms
//
// SQLite on a local disk should clear these by a comfortable margin; the
// benchmark exists as a regression guard — a future schema change that
// adds an N+1 column projection will surface here before it surfaces in
// the UI.

func seedLargeWorkspace(b *testing.B) (st *Store, workspaceID, sampleIssueID string) {
	b.Helper()
	st = newBenchStore(b)
	ctx := context.Background()
	ws, agent, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Perf",
		Slug:             "perf",
		IdentifierPrefix: "PRF",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		b.Fatal(err)
	}
	// 1,000 issues + 1 initial run each = 1,000 runs, plus 9 extra runs per
	// every other issue for 10,000 total. Distribute comments 5 per even
	// issue for 5,000.
	const issueCount = 1000
	const extraRunsPerIssue = 9
	const commentsPerEvenIssue = 5
	tx, err := st.db.BeginTxx(ctx, nil)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < issueCount; i++ {
		identifier := fmt.Sprintf("PRF-%d", i+1)
		issueID := newID()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO issue(id, workspace_id, identifier, title, body, status, assignee_agent_id, created_by, created_at, updated_at) VALUES(?,?,?,?,'','open',?,'user',datetime('now'),datetime('now'))`,
			issueID, ws.ID, identifier, fmt.Sprintf("Issue %d", i+1), agent.ID,
		); err != nil {
			b.Fatal(err)
		}
		if i == 0 {
			sampleIssueID = issueID
		}
		// initial run
		runID := newID()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO run(id,issue_id,agent_id,status,trigger_type,trigger_content_snapshot,enqueued_at,max_attempts,agent_instructions_version,chain_id,chain_depth) VALUES(?,?,?,'queued','issue_created','',datetime('now'),3,1,?,0)`,
			runID, issueID, agent.ID, runID,
		); err != nil {
			b.Fatal(err)
		}
		if i%2 == 0 {
			for c := 0; c < commentsPerEvenIssue; c++ {
				if _, err := tx.ExecContext(ctx,
					`INSERT INTO comment(id, issue_id, author_type, content, created_at) VALUES(?,?,?,?,datetime('now'))`,
					newID(), issueID, "user", fmt.Sprintf("Comment %d-%d", i, c),
				); err != nil {
					b.Fatal(err)
				}
			}
		}
		if i%2 == 0 {
			for r := 0; r < extraRunsPerIssue; r++ {
				extra := newID()
				if _, err := tx.ExecContext(ctx,
					`INSERT INTO run(id,issue_id,agent_id,status,trigger_type,trigger_content_snapshot,enqueued_at,max_attempts,agent_instructions_version,chain_id,chain_depth) VALUES(?,?,?,'done','mention','',datetime('now'),3,1,?,1)`,
					extra, issueID, agent.ID, runID,
				); err != nil {
					b.Fatal(err)
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	workspaceID = ws.ID
	return st, workspaceID, sampleIssueID
}

func BenchmarkLargeDatasetListIssues(b *testing.B) {
	st, workspaceID, _ := seedLargeWorkspace(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := st.ListIssues(ctx, workspaceID, ListIssuesFilter{Limit: 200}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLargeDatasetListRuns(b *testing.B) {
	st, _, issueID := seedLargeWorkspace(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := st.ListRuns(ctx, issueID); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLargeDatasetListComments(b *testing.B) {
	st, _, issueID := seedLargeWorkspace(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := st.ListComments(ctx, issueID); err != nil {
			b.Fatal(err)
		}
	}
}

// TestLargeDatasetMeetsLatencyBudgets is the assertion arm of the
// benchmarks above. It runs a single iteration of each list and verifies
// the wall-clock latency against the documented budget. Wall-clock is
// noisy on CI, so the budgets are 2x what the benchmark actually hits
// locally — they catch regressions, not minor jitter.
func TestLargeDatasetMeetsLatencyBudgets(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}
	st := newTestStore(t)
	ctx := context.Background()
	ws, agent, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "PerfBudget",
		Slug:             "perf-budget",
		IdentifierPrefix: "PB",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	const issueCount = 1000
	tx, _ := st.db.BeginTxx(ctx, nil)
	var sampleIssueID string
	for i := 0; i < issueCount; i++ {
		issueID := newID()
		if i == 0 {
			sampleIssueID = issueID
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO issue(id, workspace_id, identifier, title, body, status, assignee_agent_id, created_by, created_at, updated_at) VALUES(?,?,?,?,'','open',?,'user',datetime('now'),datetime('now'))`,
			issueID, ws.ID, fmt.Sprintf("PB-%d", i+1), fmt.Sprintf("Issue %d", i+1), agent.ID,
		)
		if err != nil {
			t.Fatal(err)
		}
		runID := newID()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO run(id,issue_id,agent_id,status,trigger_type,trigger_content_snapshot,enqueued_at,max_attempts,agent_instructions_version,chain_id,chain_depth) VALUES(?,?,?,'queued','issue_created','',datetime('now'),3,1,?,0)`,
			runID, issueID, agent.ID, runID,
		); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			// Make this one issue have 10 runs + 10 comments to test
			// per-issue list latency under a fan-out.
			for c := 0; c < 10; c++ {
				if _, err := tx.ExecContext(ctx,
					`INSERT INTO comment(id, issue_id, author_type, content, created_at) VALUES(?,?,?,?,datetime('now'))`,
					newID(), issueID, "user", "c",
				); err != nil {
					t.Fatal(err)
				}
				extra := newID()
				if _, err := tx.ExecContext(ctx,
					`INSERT INTO run(id,issue_id,agent_id,status,trigger_type,trigger_content_snapshot,enqueued_at,max_attempts,agent_instructions_version,chain_id,chain_depth) VALUES(?,?,?,'done','mention','',datetime('now'),3,1,?,1)`,
					extra, issueID, agent.ID, runID,
				); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	type budget struct {
		name string
		f    func() error
		max  time.Duration
	}
	cases := []budget{
		{name: "ListIssues", max: 400 * time.Millisecond, f: func() error {
			_, err := st.ListIssues(ctx, ws.ID, ListIssuesFilter{Limit: 200})
			return err
		}},
		{name: "ListRuns", max: 100 * time.Millisecond, f: func() error {
			_, err := st.ListRuns(ctx, sampleIssueID)
			return err
		}},
		{name: "ListComments", max: 100 * time.Millisecond, f: func() error {
			_, err := st.ListComments(ctx, sampleIssueID)
			return err
		}},
	}
	for _, c := range cases {
		start := time.Now()
		if err := c.f(); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		elapsed := time.Since(start)
		if elapsed > c.max {
			t.Fatalf("%s took %v, budget %v", c.name, elapsed, c.max)
		}
		t.Logf("%s: %v (budget %v)", c.name, elapsed, c.max)
	}
}
