package app

import (
	"context"
	"strings"
	"testing"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

func TestSeedMultiAgentLabCreatesSevenWorkspacesAndProjectIssues(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	workingDir := t.TempDir()

	result, err := SeedMultiAgentLab(ctx, st, MultiAgentLabOptions{WorkingDir: workingDir})
	if err != nil {
		t.Fatalf("seed multi-agent lab: %v", err)
	}
	if len(result.Workspaces) != 7 {
		t.Fatalf("expected 7 lab workspaces, got %d", len(result.Workspaces))
	}

	seen := map[string]SeededLabWorkspace{}
	for _, lab := range result.Workspaces {
		seen[lab.Workspace.Slug] = lab
		if lab.AlreadyHad {
			t.Fatalf("first seed should create workspace %s", lab.Workspace.Slug)
		}
		if lab.Workspace.WorkingDir != workingDir {
			t.Fatalf("workspace %s working_dir drift: %q", lab.Workspace.Slug, lab.Workspace.WorkingDir)
		}
		if !lab.Workspace.PerRunWorktree {
			t.Fatalf("workspace %s should enable per_run_worktree", lab.Workspace.Slug)
		}
		if !lab.Workspace.AutoChainEnabled || lab.Workspace.AutoChainMaxDepth != 3 || lab.Workspace.AutoChainDailyRunLimit != 12 {
			t.Fatalf("workspace %s has unexpected chain guard config: %#v", lab.Workspace.Slug, lab.Workspace)
		}
		if lab.Workspace.AutoCloseOnRunDone {
			t.Fatalf("workspace %s should keep auto_close_on_run_done=false for multi-step work", lab.Workspace.Slug)
		}
		if lab.MainAgent.Runtime != "codex" || !lab.MainAgent.IsMain {
			t.Fatalf("workspace %s malformed main agent: %#v", lab.Workspace.Slug, lab.MainAgent)
		}
		if len(lab.Worker) < 2 {
			t.Fatalf("workspace %s should have at least 2 worker agents, got %d", lab.Workspace.Slug, len(lab.Worker))
		}
		if len(lab.Issues) != 1 || !strings.HasPrefix(lab.Issues[0].Title, "[LAB]") {
			t.Fatalf("workspace %s should have one seeded [LAB] issue, got %#v", lab.Workspace.Slug, lab.Issues)
		}
		if !strings.Contains(lab.Issues[0].Body, "branch: lab/"+lab.Workspace.Slug) {
			t.Fatalf("workspace %s issue body missing lab branch: %s", lab.Workspace.Slug, lab.Issues[0].Body)
		}

		agents, err := st.ListAgents(ctx, lab.Workspace.ID)
		if err != nil {
			t.Fatalf("list agents for %s: %v", lab.Workspace.Slug, err)
		}
		if len(agents) != len(lab.Worker)+1 {
			t.Fatalf("workspace %s agent count mismatch: list=%d result workers=%d", lab.Workspace.Slug, len(agents), len(lab.Worker))
		}
		issues, err := st.ListIssues(ctx, lab.Workspace.ID, store.ListIssuesFilter{Query: "[LAB]", Limit: 20})
		if err != nil {
			t.Fatalf("list issues for %s: %v", lab.Workspace.Slug, err)
		}
		if len(issues) != 1 || issues[0].ExecutionStatus != "queued" {
			t.Fatalf("workspace %s seeded issue should have one queued run, got %#v", lab.Workspace.Slug, issues)
		}
	}

	if seen["pm-command-hub"].MainAgent.Name != "ProgramLead" {
		t.Fatalf("pm-command-hub should use ProgramLead main, got %#v", seen["pm-command-hub"].MainAgent)
	}
	if seen["dashboard-design-lab"].MainAgent.Name != "DesignLead" {
		t.Fatalf("dashboard-design-lab should use DesignLead main, got %#v", seen["dashboard-design-lab"].MainAgent)
	}
	if len(seen["auth-realtime-lab"].Worker) != 3 {
		t.Fatalf("auth-realtime-lab should have 3 workers, got %d", len(seen["auth-realtime-lab"].Worker))
	}
}

func TestSeedMultiAgentLabIsIdempotentAndDoesNotDuplicateIssues(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	first, err := SeedMultiAgentLab(ctx, st, MultiAgentLabOptions{WorkingDir: t.TempDir(), Runtime: "fake"})
	if err != nil {
		t.Fatalf("first seed: %v", err)
	}
	second, err := SeedMultiAgentLab(ctx, st, MultiAgentLabOptions{WorkingDir: t.TempDir(), Runtime: "fake"})
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if len(second.Workspaces) != len(first.Workspaces) {
		t.Fatalf("workspace count drifted: %d -> %d", len(first.Workspaces), len(second.Workspaces))
	}

	workspaces, err := st.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(workspaces) != 7 {
		t.Fatalf("idempotency should leave exactly 7 workspaces, got %d", len(workspaces))
	}
	for _, lab := range second.Workspaces {
		if !lab.AlreadyHad {
			t.Fatalf("second seed should report workspace %s already existed", lab.Workspace.Slug)
		}
		if lab.CreatedAgentCount != 0 || lab.CreatedIssueCount != 0 {
			t.Fatalf("second seed should not create rows for %s: agents=%d issues=%d", lab.Workspace.Slug, lab.CreatedAgentCount, lab.CreatedIssueCount)
		}
		issues, err := st.ListIssues(ctx, lab.Workspace.ID, store.ListIssuesFilter{Query: "[LAB]", Limit: 20})
		if err != nil {
			t.Fatalf("list issues for %s: %v", lab.Workspace.Slug, err)
		}
		if len(issues) != 1 {
			t.Fatalf("workspace %s should still have one lab issue, got %d", lab.Workspace.Slug, len(issues))
		}
	}
}

func TestSeedMultiAgentLabRejectsNilStore(t *testing.T) {
	if _, err := SeedMultiAgentLab(context.Background(), nil, MultiAgentLabOptions{}); err == nil {
		t.Fatalf("expected nil store error")
	}
}
