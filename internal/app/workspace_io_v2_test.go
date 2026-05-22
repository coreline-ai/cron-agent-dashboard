package app

import (
	"context"
	"strings"
	"testing"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

func TestExportWorkspaceLegacyOmitsHistory(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seeded, err := SeedExample(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateIssueWithInitialRun(ctx, seeded.Workspace.ID, store.CreateIssueInput{
		Title:           "Plain issue",
		Body:            "plain body",
		AssigneeAgentID: seeded.MainAgent.ID,
		CreatedBy:       "user",
	}); err != nil {
		t.Fatal(err)
	}
	export, err := ExportWorkspace(ctx, st, seeded.Workspace.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(export.Issues) != 0 || len(export.Comments) != 0 || len(export.Runs) != 0 {
		t.Fatalf("legacy export must omit history: issues=%d comments=%d runs=%d",
			len(export.Issues), len(export.Comments), len(export.Runs))
	}
}

func TestExportWorkspaceIncludeHistoryAndMaskPII(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seeded, err := SeedExample(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	issue, _, err := st.CreateIssueWithInitialRun(ctx, seeded.Workspace.ID, store.CreateIssueInput{
		Title:           "Reach customer@example.com about onboarding",
		Body:            "Call 010-1234-5678 or email jane.doe@company.com",
		AssigneeAgentID: seeded.MainAgent.ID,
		CreatedBy:       "user",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddUserComment(ctx, issue.ID, "Ping jane.doe@company.com today"); err != nil {
		t.Fatal(err)
	}

	export, err := ExportWorkspaceWithOptions(ctx, st, seeded.Workspace.Slug, ExportWorkspaceOptions{
		IncludeHistory: true,
		MaskPII:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(export.Issues) == 0 {
		t.Fatalf("history export must include at least one issue")
	}
	exportedIssue := export.Issues[0]
	if strings.Contains(exportedIssue.Title, "customer@example.com") {
		t.Fatalf("title PII not masked: %q", exportedIssue.Title)
	}
	if !strings.Contains(exportedIssue.Title, "[email]") {
		t.Fatalf("title missing [email] placeholder: %q", exportedIssue.Title)
	}
	if strings.Contains(exportedIssue.Body, "010-1234-5678") {
		t.Fatalf("body phone not masked: %q", exportedIssue.Body)
	}
	if strings.Contains(exportedIssue.Body, "jane.doe@company.com") {
		t.Fatalf("body email not masked: %q", exportedIssue.Body)
	}
	if !strings.Contains(exportedIssue.Body, "[email]") || !strings.Contains(exportedIssue.Body, "[phone]") {
		t.Fatalf("body must show placeholders for both kinds of PII: %q", exportedIssue.Body)
	}
	if len(export.Comments) == 0 {
		t.Fatalf("history export must include comments")
	}
	if strings.Contains(export.Comments[0].Content, "jane.doe@company.com") {
		t.Fatalf("comment email not masked: %q", export.Comments[0].Content)
	}
	if len(export.Runs) == 0 {
		t.Fatalf("history export must include the initial run created with the issue")
	}
	if export.Runs[0].IssueIdentifier == "" {
		t.Fatalf("run history must reference the issue by identifier (got empty)")
	}
}

func TestExportWorkspaceIncludeHistoryWithoutMaskPreservesContent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seeded, err := SeedExample(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateIssueWithInitialRun(ctx, seeded.Workspace.ID, store.CreateIssueInput{
		Title:           "Reach jane.doe@company.com",
		Body:            "phone 010-1234-5678",
		AssigneeAgentID: seeded.MainAgent.ID,
		CreatedBy:       "user",
	}); err != nil {
		t.Fatal(err)
	}
	export, err := ExportWorkspaceWithOptions(ctx, st, seeded.Workspace.Slug, ExportWorkspaceOptions{
		IncludeHistory: true,
		MaskPII:        false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(export.Issues[0].Title, "jane.doe@company.com") {
		t.Fatalf("mask=false must keep original PII: %q", export.Issues[0].Title)
	}
}

func TestImportWorkspaceIgnoresHistoryFieldsSilently(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if _, err := SeedExample(ctx, st); err != nil {
		t.Fatal(err)
	}
	full, err := ExportWorkspaceWithOptions(ctx, st, "demo-studio", ExportWorkspaceOptions{IncludeHistory: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ImportWorkspace(ctx, st, full, ImportOptions{DestSlug: "demo-history-target"}); err != nil {
		t.Fatalf("import must accept history-carrying export silently: %v", err)
	}
}
