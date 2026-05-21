package app

import (
	"context"
	"strings"
	"testing"
)

func TestSeedExampleCreatesHubPMWorkspaceOnFirstCall(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	result, err := SeedExample(ctx, st)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if result.AlreadyHad {
		t.Fatalf("first call should not report AlreadyHad=true")
	}
	if result.Workspace.Slug != "demo-studio" {
		t.Fatalf("unexpected slug=%q", result.Workspace.Slug)
	}
	if !result.Workspace.AutoChainEnabled {
		t.Fatalf("seeded workspace should have auto_chain_enabled=true")
	}
	if result.MainAgent.Name != "Lead" || result.MainAgent.Runtime != "codex" || !result.MainAgent.IsMain {
		t.Fatalf("main agent malformed: %#v", result.MainAgent)
	}
	if !strings.Contains(result.MainAgent.Instructions, "통합 문서를 작성하지 마라") {
		t.Fatalf("Lead instructions missing hub guard text; got %q", result.MainAgent.Instructions)
	}
	if !strings.Contains(result.MainAgent.Instructions, "auto-chain은 결과 댓글의 첫 멘션만 dispatch") {
		t.Fatalf("Lead instructions missing chain-mention guard; got %q", result.MainAgent.Instructions)
	}

	if len(result.Worker) != 2 {
		t.Fatalf("expected 2 worker agents, got %d", len(result.Worker))
	}
	names := map[string]bool{}
	for _, w := range result.Worker {
		if w.IsMain {
			t.Fatalf("worker %q is marked main", w.Name)
		}
		names[w.Name] = true
	}
	if !names["Writer"] || !names["Reviewer"] {
		t.Fatalf("expected Writer + Reviewer workers, got %#v", names)
	}
}

func TestSeedExampleIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	first, err := SeedExample(ctx, st)
	if err != nil {
		t.Fatalf("first seed: %v", err)
	}
	second, err := SeedExample(ctx, st)
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if !second.AlreadyHad {
		t.Fatalf("second call should report AlreadyHad=true")
	}
	if second.Workspace.ID != first.Workspace.ID {
		t.Fatalf("idempotency broken: workspace ID drift %s -> %s", first.Workspace.ID, second.Workspace.ID)
	}
	if len(second.Worker) != len(first.Worker) {
		t.Fatalf("idempotency: worker count drifted %d -> %d", len(first.Worker), len(second.Worker))
	}
}
