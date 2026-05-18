package store

import (
	"context"
	"testing"
)

func TestSkillRegistryAssignAndResolve(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, agent, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name: "Skill Workspace", Slug: "skill-workspace", IdentifierPrefix: "SKL",
		MainAgent: CreateAgentInput{Name: "Researcher", Runtime: "codex", Instructions: "research"},
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	skill, err := st.UpsertSkill(ctx, ws.ID, UpsertSkillInput{
		SkillMD: `---
name: reddit-ai-brief
description: Reddit AI discussion summarizer
triggers: [reddit, AI]
---
Summarize discussions as Korean markdown.
`,
	})
	if err != nil {
		t.Fatalf("upsert skill: %v", err)
	}
	if skill.Content == "" || len(skill.Triggers) != 2 || skill.ContentHash == "" {
		t.Fatalf("bad skill: %#v", skill)
	}

	assignment, err := st.AssignAgentSkill(ctx, agent.ID, AssignAgentSkillInput{SkillID: skill.ID, ActivationMode: "trigger", Priority: 10})
	if err != nil {
		t.Fatalf("assign skill: %v", err)
	}
	if assignment.Skill == nil || assignment.Skill.Name != skill.Name {
		t.Fatalf("bad assignment: %#v", assignment)
	}

	resolved, err := st.ResolvePromptSkills(ctx, agent.ID, "Reddit AI 주요 논의", "body", "", nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(resolved) != 1 || !resolved[0].Active || resolved[0].TriggerReason == "" {
		t.Fatalf("bad resolved skills: %#v", resolved)
	}

	_, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "Reddit AI 주요 논의"})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	if err := st.AppendSkillsLoadedEvent(ctx, run.ID, resolved); err != nil {
		t.Fatalf("append skills event: %v", err)
	}
	events, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if got := events[len(events)-1].EventType; got != RunEventSkillsLoaded {
		t.Fatalf("last event=%s, want %s", got, RunEventSkillsLoaded)
	}
}

func TestManualSkillActivation(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, agent, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{Name: "Manual Skills", Slug: "manual-skills", IdentifierPrefix: "MSK", MainAgent: CreateAgentInput{Name: "Writer", Runtime: "codex", Instructions: "write"}})
	if err != nil {
		t.Fatal(err)
	}
	skill, err := st.UpsertSkill(ctx, ws.ID, UpsertSkillInput{Name: "editorial-style", Description: "Editorial style guide", Content: "Use concise prose."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AssignAgentSkill(ctx, agent.ID, AssignAgentSkillInput{SkillID: skill.ID, ActivationMode: "manual"}); err != nil {
		t.Fatal(err)
	}
	resolved, err := st.ResolvePromptSkills(ctx, agent.ID, "Title", "#skills: editorial-style\n본문", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 || !resolved[0].Active || resolved[0].TriggerReason != "manual" {
		t.Fatalf("bad manual activation: %#v", resolved)
	}
}
