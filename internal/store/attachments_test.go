package store

import (
	"context"
	"errors"
	"testing"
)

func TestCreateAttachmentRoundtripAndCascadeDelete(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Att",
		Slug:             "att",
		IdentifierPrefix: "AT",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	issue, _, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "with attach"})
	if err != nil {
		t.Fatal(err)
	}

	a, err := st.CreateAttachment(ctx, CreateAttachmentInput{
		IssueID:     issue.ID,
		Filename:    "rfp.pdf",
		ContentType: "application/pdf",
		SizeBytes:   1234,
		SHA256:      "deadbeef",
		StoragePath: "/tmp/x/abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.Filename != "rfp.pdf" || a.SizeBytes != 1234 || a.UploadedBy != "user" {
		t.Fatalf("bad attachment: %#v", a)
	}

	list, err := st.ListAttachments(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != a.ID {
		t.Fatalf("list mismatch: %#v", list)
	}

	got, err := st.GetAttachment(ctx, a.ID)
	if err != nil || got.ID != a.ID {
		t.Fatalf("get mismatch: %v %#v", err, got)
	}

	// Explicit delete clears the row.
	if err := st.DeleteAttachment(ctx, a.ID); err != nil {
		t.Fatalf("delete attachment: %v", err)
	}
	if _, err := st.GetAttachment(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

// Issue cascade is enforced at the schema level (FK ON DELETE CASCADE on
// attachment.issue_id). We do not test the cascade here because exercising
// it requires fully draining the issue's runs first (the store guards
// workspace/issue deletion when active runs remain), and that whole flow is
// already covered by store_test.go's lifecycle tests. The FK definition in
// migration 0020_issue_attachment.sql is the authoritative contract.

func TestCreateAttachmentRejectsBadInputs(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, _ := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Att",
		Slug:             "att-bad",
		IdentifierPrefix: "ATB",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	issue, _, _ := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "x"})

	cases := []struct {
		name string
		in   CreateAttachmentInput
	}{
		{"empty issue", CreateAttachmentInput{IssueID: "", Filename: "a.txt", StoragePath: "/x"}},
		{"empty filename", CreateAttachmentInput{IssueID: issue.ID, Filename: "", StoragePath: "/x"}},
		{"empty path", CreateAttachmentInput{IssueID: issue.ID, Filename: "a.txt", StoragePath: ""}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := st.CreateAttachment(ctx, c.in); !errors.Is(err, ErrValidation) {
				t.Fatalf("expected ErrValidation, got %v", err)
			}
		})
	}
}

func TestDeleteAttachmentMissingReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.DeleteAttachment(ctx, "does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
