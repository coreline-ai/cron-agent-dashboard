package store

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAutoChainMainAgentRevisitAllowed(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, main, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Hub Chain",
		Slug:             "hub-chain",
		IdentifierPrefix: "HUB",
		AutoChainEnabled: true,
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	writer, err := st.CreateAgent(ctx, ws.ID, CreateAgentInput{Name: "Writer", Runtime: "codex", Instructions: "write"})
	if err != nil {
		t.Fatal(err)
	}

	issue, leadRun1, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "rfp"})
	if err != nil {
		t.Fatal(err)
	}

	if _, ok, err := st.ClaimNextRun(ctx, "w"); err != nil || !ok {
		t.Fatalf("claim lead1 ok=%v err=%v", ok, err)
	}
	if _, err := st.CompleteRun(ctx, leadRun1.ID, 0, "", "@Writer draft this", false, ""); err != nil {
		t.Fatal(err)
	}

	runs, err := st.ListRuns(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[1].AgentID != writer.ID {
		t.Fatalf("expected Writer dispatched, runs=%#v", runs)
	}
	writerRun := runs[1]

	if _, ok, err := st.ClaimNextRun(ctx, "w"); err != nil || !ok {
		t.Fatalf("claim writer ok=%v err=%v", ok, err)
	}
	if _, err := st.CompleteRun(ctx, writerRun.ID, 0, "", "@Lead here is the draft, please delegate next", false, ""); err != nil {
		t.Fatal(err)
	}

	runs, err = st.ListRuns(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected main agent re-entry to be allowed, runs=%#v", runs)
	}
	last := runs[2]
	if last.AgentID != main.ID {
		t.Fatalf("expected re-entered run to be Lead, got agent_id=%s", last.AgentID)
	}
	if last.ChainID != leadRun1.ID {
		t.Fatalf("re-entered run should share chain_id=%s, got %s", leadRun1.ID, last.ChainID)
	}
	if last.ChainDepth != 2 {
		t.Fatalf("expected chain_depth=2, got %d", last.ChainDepth)
	}
}

func TestAutoChainNonMainAgentRevisitBlocked(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Worker Loop",
		Slug:             "worker-loop",
		IdentifierPrefix: "WL",
		AutoChainEnabled: true,
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	writer, err := st.CreateAgent(ctx, ws.ID, CreateAgentInput{Name: "Writer", Runtime: "codex", Instructions: "write"})
	if err != nil {
		t.Fatal(err)
	}
	reviewer, err := st.CreateAgent(ctx, ws.ID, CreateAgentInput{Name: "Reviewer", Runtime: "codex", Instructions: "review"})
	if err != nil {
		t.Fatal(err)
	}

	issue, leadRun, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "rfp"})
	if err != nil {
		t.Fatal(err)
	}

	if _, ok, err := st.ClaimNextRun(ctx, "w"); err != nil || !ok {
		t.Fatalf("claim lead ok=%v err=%v", ok, err)
	}
	if _, err := st.CompleteRun(ctx, leadRun.ID, 0, "", "@Writer draft this", false, ""); err != nil {
		t.Fatal(err)
	}

	runs, err := st.ListRuns(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[1].AgentID != writer.ID {
		t.Fatalf("writer not dispatched: %#v", runs)
	}
	writerRun := runs[1]

	if _, ok, err := st.ClaimNextRun(ctx, "w"); err != nil || !ok {
		t.Fatalf("claim writer ok=%v err=%v", ok, err)
	}
	if _, err := st.CompleteRun(ctx, writerRun.ID, 0, "", "@Reviewer review my draft", false, ""); err != nil {
		t.Fatal(err)
	}

	runs, err = st.ListRuns(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 || runs[2].AgentID != reviewer.ID {
		t.Fatalf("reviewer should be dispatched: %#v", runs)
	}
	reviewerRun := runs[2]

	if _, ok, err := st.ClaimNextRun(ctx, "w"); err != nil || !ok {
		t.Fatalf("claim reviewer ok=%v err=%v", ok, err)
	}
	if _, err := st.CompleteRun(ctx, reviewerRun.ID, 0, "", "@Writer please revise", false, ""); err != nil {
		t.Fatal(err)
	}

	runs, err = st.ListRuns(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 {
		t.Fatalf("non-main agent re-entry should be blocked, runs=%#v", runs)
	}

	comments, err := st.ListComments(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range comments {
		if strings.Contains(c.Content, "중복 방지를 위해 @Writer") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected duplicate-guard system comment for non-main agent revisit; comments=%#v", comments)
	}
}

func TestAutoChainMainAgentRevisitStopsAtMaxDepth(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	maxDepth := 2
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:              "Hub Depth",
		Slug:              "hub-depth",
		IdentifierPrefix:  "HD",
		AutoChainEnabled:  true,
		AutoChainMaxDepth: maxDepth,
		MainAgent:         CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAgent(ctx, ws.ID, CreateAgentInput{Name: "Writer", Runtime: "codex", Instructions: "write"}); err != nil {
		t.Fatal(err)
	}

	issue, leadRun, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "deep"})
	if err != nil {
		t.Fatal(err)
	}

	currentRunID := leadRun.ID
	mentions := []string{"@Writer", "@Lead", "@Writer", "@Lead"}
	for i, mention := range mentions {
		if _, ok, err := st.ClaimNextRun(ctx, "w"); err != nil || !ok {
			t.Fatalf("claim iteration %d ok=%v err=%v", i, ok, err)
		}
		if _, err := st.CompleteRun(ctx, currentRunID, 0, "", mention+" go", false, ""); err != nil {
			t.Fatalf("complete iteration %d: %v", i, err)
		}
		runs, err := st.ListRuns(ctx, issue.ID)
		if err != nil {
			t.Fatal(err)
		}
		latest := runs[len(runs)-1]
		if latest.Status != "queued" {
			break
		}
		currentRunID = latest.ID
	}

	runs, err := st.ListRuns(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range runs {
		if r.ChainDepth > maxDepth {
			t.Fatalf("chain_depth=%d exceeds max_depth=%d in run=%#v", r.ChainDepth, maxDepth, r)
		}
	}

	comments, err := st.ListComments(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range comments {
		if strings.Contains(c.Content, "자동 체이닝 깊이 제한") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected depth-limit system comment, comments=%#v", comments)
	}
}

func TestAutoChainDispatchSanitizesPromptSnapshot(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Snapshot Chain",
		Slug:             "snapshot-chain",
		IdentifierPrefix: "SNP",
		AutoChainEnabled: true,
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	writer, err := st.CreateAgent(ctx, ws.ID, CreateAgentInput{Name: "Writer", Runtime: "codex", Instructions: "write"})
	if err != nil {
		t.Fatal(err)
	}

	issue, leadRun, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "w"); err != nil || !ok {
		t.Fatalf("claim lead ok=%v err=%v", ok, err)
	}

	// Comment content contains an invalid UTF-8 byte sequence (\xc3\x28) plus a chain mention.
	rawContent := "draft please " + string([]byte{0xc3, 0x28}) + " — @Writer take it over"
	if _, err := st.CompleteRun(ctx, leadRun.ID, 0, "", rawContent, false, ""); err != nil {
		t.Fatal(err)
	}

	runs, err := st.ListRuns(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[1].AgentID != writer.ID {
		t.Fatalf("expected Writer dispatched via chain, runs=%#v", runs)
	}
	snap := runs[1].TriggerContentSnapshot
	if !utf8.ValidString(snap) {
		t.Fatalf("trigger_content_snapshot contains invalid UTF-8: bytes=%x", []byte(snap))
	}
}
