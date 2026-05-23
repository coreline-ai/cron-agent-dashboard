package app

import (
	"context"
	"testing"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

// Track C of dev-plan/implement_20260523_092408.md.
//
// ImportOptions.IncludeHistory rebuilds the issues / comments / runs that
// the v2 export carries so the operator can roll a workspace forward from
// an archive. Identifiers and timestamps survive the round-trip; runs that
// were still in flight at export time are forced to 'cancelled' so they
// do not become permanent ghosts.
func TestImportWorkspaceIncludeHistoryRestoresIssuesAndComments(t *testing.T) {
	ctx := context.Background()
	src := newTestStore(t)
	dst := newTestStore(t)
	seeded, err := SeedExample(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	issue, _, err := src.CreateIssueWithInitialRun(ctx, seeded.Workspace.ID, store.CreateIssueInput{
		Title:           "Roundtrip me",
		Body:            "body",
		AssigneeAgentID: seeded.MainAgent.ID,
		CreatedBy:       "user",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.AddUserComment(ctx, issue.ID, "second comment"); err != nil {
		t.Fatal(err)
	}

	export, err := ExportWorkspaceWithOptions(ctx, src, seeded.Workspace.Slug, ExportWorkspaceOptions{IncludeHistory: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(export.Issues) == 0 || len(export.Comments) == 0 || len(export.Runs) == 0 {
		t.Fatalf("export missing history: issues=%d comments=%d runs=%d", len(export.Issues), len(export.Comments), len(export.Runs))
	}

	imported, err := ImportWorkspace(ctx, dst, export, ImportOptions{DestSlug: "demo-roundtrip", IncludeHistory: true})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// Issue identifiers survive the round-trip.
	issues, err := dst.ListIssues(ctx, imported.ID, store.ListIssuesFilter{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != len(export.Issues) {
		t.Fatalf("issue count drift: imported=%d exported=%d", len(issues), len(export.Issues))
	}
	identifiers := map[string]bool{}
	for _, i := range issues {
		identifiers[i.Identifier] = true
	}
	for _, e := range export.Issues {
		if !identifiers[e.Identifier] {
			t.Fatalf("imported workspace missing identifier %q", e.Identifier)
		}
	}

	// Comments survive, attached to the correct issue.
	for _, i := range issues {
		comments, err := dst.ListComments(ctx, i.ID)
		if err != nil {
			t.Fatal(err)
		}
		if i.Identifier == issue.Identifier && len(comments) == 0 {
			t.Fatalf("expected comment on imported issue %s", i.Identifier)
		}
	}

	// Runs survive but anything non-terminal is now 'cancelled'.
	for _, i := range issues {
		runs, err := dst.ListRuns(ctx, i.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range runs {
			switch r.Status {
			case "done", "failed", "cancelled":
			default:
				t.Fatalf("imported run %s status=%q must be terminal", r.ID, r.Status)
			}
		}
	}

	// next_issue_seq is advanced past the highest imported identifier so a
	// fresh issue created on the destination workspace does not collide.
	freshIssue, _, err := dst.CreateIssueWithInitialRun(ctx, imported.ID, store.CreateIssueInput{Title: "fresh", CreatedBy: "user"})
	if err != nil {
		t.Fatalf("create fresh issue: %v", err)
	}
	if identifiers[freshIssue.Identifier] {
		t.Fatalf("fresh issue collided with restored identifier %q", freshIssue.Identifier)
	}
}

func TestImportWorkspaceWithoutHistoryFlagStillIgnoresExportedSlices(t *testing.T) {
	ctx := context.Background()
	src := newTestStore(t)
	dst := newTestStore(t)
	if _, err := SeedExample(ctx, src); err != nil {
		t.Fatal(err)
	}
	if _, _, err := src.CreateIssueWithInitialRun(ctx, "demo-studio-not-real", store.CreateIssueInput{Title: "x"}); err == nil {
		// Not expected; helper above is for type-checking only.
	}
	export, err := ExportWorkspaceWithOptions(ctx, src, "demo-studio", ExportWorkspaceOptions{IncludeHistory: true})
	if err != nil {
		t.Fatal(err)
	}
	imported, err := ImportWorkspace(ctx, dst, export, ImportOptions{DestSlug: "demo-no-history", IncludeHistory: false})
	if err != nil {
		t.Fatal(err)
	}
	issues, _ := dst.ListIssues(ctx, imported.ID, store.ListIssuesFilter{Limit: 50})
	if len(issues) != 0 {
		t.Fatalf("history should not be restored when IncludeHistory=false, got %d issues", len(issues))
	}
}
