package store

import (
	"context"
	"errors"
	"fmt"
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
	// Hub-PM policy: main agent re-entry inherits the parent's chain_depth
	// (worker dispatch advanced from 0 -> 1, main re-entry keeps it at 1).
	if last.ChainDepth != 1 {
		t.Fatalf("expected main re-entry to inherit parent depth=1, got %d", last.ChainDepth)
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

// TestAutoChainHubPMOnlyWorkerDispatchesCountTowardMaxDepth pins the new
// hub-PM policy: main agent re-entry inherits the parent's chain_depth, so
// only worker dispatches advance toward max_depth. Linear worker chains
// still hit the gate as before.
func TestAutoChainHubPMOnlyWorkerDispatchesCountTowardMaxDepth(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	maxDepth := 2
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:              "Hub Depth Workers",
		Slug:              "hub-depth-workers",
		IdentifierPrefix:  "HDW",
		AutoChainEnabled:  true,
		AutoChainMaxDepth: maxDepth,
		MainAgent:         CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"W1", "W2", "W3"} {
		if _, err := st.CreateAgent(ctx, ws.ID, CreateAgentInput{Name: name, Runtime: "codex", Instructions: name}); err != nil {
			t.Fatal(err)
		}
	}

	issue, leadRun, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "hub-pm-depth"})
	if err != nil {
		t.Fatal(err)
	}

	advance := func(t *testing.T, runID, mention string) Run {
		t.Helper()
		if _, ok, err := st.ClaimNextRun(ctx, "w"); err != nil || !ok {
			t.Fatalf("claim for %s after %s: ok=%v err=%v", mention, runID, ok, err)
		}
		if _, err := st.CompleteRun(ctx, runID, 0, "", mention+" go", false, ""); err != nil {
			t.Fatalf("complete %s: %v", runID, err)
		}
		runs, err := st.ListRuns(ctx, issue.ID)
		if err != nil {
			t.Fatal(err)
		}
		return runs[len(runs)-1]
	}

	// Lead(d0) -> W1(d1): worker advances depth.
	w1 := advance(t, leadRun.ID, "@W1")
	if w1.AgentName != "W1" || w1.ChainDepth != 1 {
		t.Fatalf("W1 dispatch: %#v", w1)
	}
	// W1(d1) -> Lead(d1): hub re-entry keeps depth.
	leadReentry1 := advance(t, w1.ID, "@Lead")
	if leadReentry1.AgentName != "Lead" || leadReentry1.ChainDepth != 1 {
		t.Fatalf("Lead re-entry should preserve depth=1, got %#v", leadReentry1)
	}
	// Lead(d1) -> W2(d2): worker advances depth to the max.
	w2 := advance(t, leadReentry1.ID, "@W2")
	if w2.AgentName != "W2" || w2.ChainDepth != maxDepth {
		t.Fatalf("W2 should reach depth=%d, got %#v", maxDepth, w2)
	}
	// W2(d2) -> Lead(d2): hub re-entry still allowed even at the worker depth limit.
	leadReentry2 := advance(t, w2.ID, "@Lead")
	if leadReentry2.AgentName != "Lead" || leadReentry2.ChainDepth != maxDepth {
		t.Fatalf("Lead re-entry at max worker depth should be allowed, got %#v", leadReentry2)
	}
	// Lead(d2) -> W3 must now be blocked: parent.ChainDepth == max_depth.
	if _, ok, err := st.ClaimNextRun(ctx, "w"); err != nil || !ok {
		t.Fatalf("claim before W3 attempt: ok=%v err=%v", ok, err)
	}
	if _, err := st.CompleteRun(ctx, leadReentry2.ID, 0, "", "@W3 go", false, ""); err != nil {
		t.Fatal(err)
	}
	runs, err := st.ListRuns(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range runs {
		if r.AgentName == "W3" {
			t.Fatalf("W3 dispatch should be blocked by max_depth=%d; got run %#v", maxDepth, r)
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
		t.Fatalf("expected depth-limit system comment when worker dispatch reaches max_depth; comments=%#v", comments)
	}
}

// TestAutoChainHubPMFiveWorkersFitWithinMaxDepthFive ties the new policy to
// the real RFP-style use case: 5 worker agents cycling through the main
// agent fit inside the default-ish max_depth=5 without the operator having
// to raise the limit.
func TestAutoChainHubPMFiveWorkersFitWithinMaxDepthFive(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	maxDepth := 5
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:              "RFP-like",
		Slug:              "rfp-like",
		IdentifierPrefix:  "RFP",
		AutoChainEnabled:  true,
		AutoChainMaxDepth: maxDepth,
		MainAgent:         CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	workers := []string{"Sales", "Planner", "Designer", "Architect", "QA"}
	for _, name := range workers {
		if _, err := st.CreateAgent(ctx, ws.ID, CreateAgentInput{Name: name, Runtime: "codex", Instructions: name}); err != nil {
			t.Fatal(err)
		}
	}

	issue, leadRun, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "rfp"})
	if err != nil {
		t.Fatal(err)
	}

	// Each worker iteration does two dispatches: Lead -> worker, then worker
	// -> Lead (hub re-entry). For the very last worker we stop after the
	// worker dispatch since real workflows end with Lead consolidating
	// without a mention.
	leadRunID := leadRun.ID
	for i, worker := range workers {
		// Lead -> worker
		if _, ok, err := st.ClaimNextRun(ctx, "w"); err != nil || !ok {
			t.Fatalf("iter %d lead-claim: ok=%v err=%v", i, ok, err)
		}
		if _, err := st.CompleteRun(ctx, leadRunID, 0, "", "@"+worker+" go", false, ""); err != nil {
			t.Fatalf("iter %d lead-complete: %v", i, err)
		}
		runs, _ := st.ListRuns(ctx, issue.ID)
		workerRun := runs[len(runs)-1]
		if workerRun.AgentName != worker {
			t.Fatalf("iter %d expected dispatch to %s, got %#v", i, worker, workerRun)
		}

		// Worker -> Lead (skip for the last worker)
		if i == len(workers)-1 {
			break
		}
		if _, ok, err := st.ClaimNextRun(ctx, "w"); err != nil || !ok {
			t.Fatalf("iter %d worker-claim: ok=%v err=%v", i, ok, err)
		}
		if _, err := st.CompleteRun(ctx, workerRun.ID, 0, "", "@Lead go", false, ""); err != nil {
			t.Fatalf("iter %d worker-complete: %v", i, err)
		}
		runs, _ = st.ListRuns(ctx, issue.ID)
		latest := runs[len(runs)-1]
		if latest.AgentName != "Lead" {
			t.Fatalf("iter %d expected hub re-entry, got %#v", i, latest)
		}
		leadRunID = latest.ID
	}

	// All 5 workers should have been dispatched within max_depth=5 with the
	// hub-PM exemption — total runs = 1 initial + 5 workers + 4 hub re-entries.
	runs, _ := st.ListRuns(ctx, issue.ID)
	workerCount := 0
	maxObserved := 0
	for _, r := range runs {
		for _, w := range workers {
			if r.AgentName == w {
				workerCount++
				break
			}
		}
		if r.ChainDepth > maxObserved {
			maxObserved = r.ChainDepth
		}
	}
	if workerCount != len(workers) {
		t.Fatalf("expected %d worker dispatches, got %d; runs=%#v", len(workers), workerCount, runs)
	}
	if maxObserved != maxDepth {
		t.Fatalf("expected max observed chain_depth to be exactly max_depth=%d, got %d", maxDepth, maxObserved)
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

func TestAutoChainAgentLookupMessageDistinguishesNotFoundFromStoreError(t *testing.T) {
	notFoundMsg := autoChainAgentLookupMessage("Writer", ErrNotFound)
	if !strings.Contains(notFoundMsg, "@Writer") || !strings.Contains(notFoundMsg, "찾을 수 없습니다") {
		t.Fatalf("not-found message did not include mention + 찾을 수 없습니다: %q", notFoundMsg)
	}

	wrapped := fmt.Errorf("scan row: %w", ErrNotFound)
	if got := autoChainAgentLookupMessage("Writer", wrapped); got != notFoundMsg {
		t.Fatalf("wrapped ErrNotFound should still be treated as not-found, got=%q", got)
	}

	storeErr := errors.New("simulated connection reset")
	storeMsg := autoChainAgentLookupMessage("Writer", storeErr)
	if storeMsg == notFoundMsg {
		t.Fatalf("store error should produce a different message than not-found")
	}
	if !strings.Contains(storeMsg, "일시적 오류") {
		t.Fatalf("store error message did not flag transient failure: %q", storeMsg)
	}
	if strings.Contains(storeMsg, "simulated connection reset") {
		t.Fatalf("raw error details leaked into system comment: %q", storeMsg)
	}
}

func TestAutoChainMentionNotFoundProducesNotFoundComment(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "MissingMention",
		Slug:             "missing-mention",
		IdentifierPrefix: "MM",
		AutoChainEnabled: true,
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	issue, leadRun, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "w"); err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	// @Writer does not exist in this workspace.
	if _, err := st.CompleteRun(ctx, leadRun.ID, 0, "", "@Writer please pick this up", false, ""); err != nil {
		t.Fatal(err)
	}

	runs, err := st.ListRuns(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected no chained run; runs=%#v", runs)
	}

	comments, err := st.ListComments(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundNotFound := false
	foundTransient := false
	for _, c := range comments {
		if c.AuthorType != "system" {
			continue
		}
		if strings.Contains(c.Content, "@Writer을 찾을 수 없습니다") {
			foundNotFound = true
		}
		if strings.Contains(c.Content, "일시적 오류") {
			foundTransient = true
		}
	}
	if !foundNotFound {
		t.Fatalf("missing not-found system comment; comments=%#v", comments)
	}
	if foundTransient {
		t.Fatalf("expected only not-found message, but transient error message also surfaced; comments=%#v", comments)
	}
}
