package app

import (
	"context"
	"testing"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

func TestSeedDevTeamCreatesSevenAgentsEightSkillsAndAssignments(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	workingDir := t.TempDir()

	result, err := SeedDevTeam(ctx, st, "dev-team-test", workingDir)
	if err != nil {
		t.Fatalf("seed dev team: %v", err)
	}
	if result.AlreadyHad {
		t.Fatalf("first seed should create workspace")
	}
	if result.Workspace.Slug != "dev-team-test" || result.Workspace.WorkingDir != workingDir {
		t.Fatalf("unexpected workspace: %#v", result.Workspace)
	}
	if !result.Workspace.AutoChainEnabled || result.Workspace.AutoChainMaxDepth != 8 || !result.Workspace.PerRunWorktree || result.Workspace.AutoCloseOnRunDone {
		t.Fatalf("unexpected workspace controls: %#v", result.Workspace)
	}
	if len(result.Agents) != 7 {
		t.Fatalf("expected 7 agents, got %d", len(result.Agents))
	}
	if len(result.Skills) != 8 {
		t.Fatalf("expected 8 skills, got %d", len(result.Skills))
	}
	if result.AssignmentCount != 14 {
		t.Fatalf("expected 14 skill assignments, got %d", result.AssignmentCount)
	}

	agentsByName := map[string]store.Agent{}
	for _, agent := range result.Agents {
		agentsByName[agent.Name] = agent
	}
	if !agentsByName["Lead"].IsMain || agentsByName["Lead"].Runtime != "codex" {
		t.Fatalf("Lead should be main codex agent: %#v", agentsByName["Lead"])
	}
	for _, name := range []string{"Lead", "Designer", "Backend", "Frontend", "DB", "QA", "DevOps"} {
		if agentsByName[name].Runtime != "codex" {
			t.Fatalf("%s should use codex runtime (project default): %#v", name, agentsByName[name])
		}
	}

	leadSkills, err := st.ListAgentSkills(ctx, agentsByName["Lead"].ID)
	if err != nil {
		t.Fatalf("list lead skills: %v", err)
	}
	if len(leadSkills) != 2 {
		t.Fatalf("Lead should have result-protocol + hub-routing, got %d", len(leadSkills))
	}
	qaSkills, err := st.ListAgentSkills(ctx, agentsByName["QA"].ID)
	if err != nil {
		t.Fatalf("list qa skills: %v", err)
	}
	if len(qaSkills) != 2 {
		t.Fatalf("QA should have result-protocol + qa-verdict, got %d", len(qaSkills))
	}
}

func TestSeedDevTeamIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	first, err := SeedDevTeam(ctx, st, "", t.TempDir())
	if err != nil {
		t.Fatalf("first seed: %v", err)
	}
	second, err := SeedDevTeam(ctx, st, "", t.TempDir())
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if !second.AlreadyHad || second.Workspace.ID != first.Workspace.ID {
		t.Fatalf("second seed should reuse workspace: first=%#v second=%#v", first.Workspace, second.Workspace)
	}
	if second.CreatedAgentCount != 0 || len(second.Agents) != 7 || len(second.Skills) != 8 || second.AssignmentCount != 14 {
		t.Fatalf("idempotency drift: %#v", second)
	}
}

func TestSeedDevTeamRejectsNilStore(t *testing.T) {
	if _, err := SeedDevTeam(context.Background(), nil, "", ""); err == nil {
		t.Fatalf("expected nil store error")
	}
}
