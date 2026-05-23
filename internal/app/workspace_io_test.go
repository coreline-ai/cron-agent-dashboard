package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

func TestExportWorkspaceCapturesOperationalConfiguration(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	seeded, err := SeedExample(ctx, st)
	if err != nil {
		t.Fatal(err)
	}

	export, err := ExportWorkspace(ctx, st, seeded.Workspace.Slug)
	if err != nil {
		t.Fatal(err)
	}

	if export.FormatVersion != WorkspaceExportFormatVersion {
		t.Fatalf("format_version=%d want %d", export.FormatVersion, WorkspaceExportFormatVersion)
	}
	if export.Workspace.Slug != "demo-studio" || !export.Workspace.AutoChainEnabled {
		t.Fatalf("workspace mismatch: %#v", export.Workspace)
	}
	if len(export.Agents) != 3 {
		t.Fatalf("expected 3 agents (Lead + Writer + Reviewer), got %d", len(export.Agents))
	}
	var leadFound bool
	for _, a := range export.Agents {
		if a.IsMain {
			leadFound = true
			if a.Name != "Lead" || a.Runtime != "codex" {
				t.Fatalf("main agent mismatch: %#v", a)
			}
			if !strings.Contains(a.Instructions, "통합 문서를 작성하지 마라") {
				t.Fatalf("main agent instructions missing hub guard: %q", a.Instructions)
			}
		}
	}
	if !leadFound {
		t.Fatalf("no main agent in export")
	}

	// JSON round-trip is stable.
	encoded, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded WorkspaceExport
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Workspace.Slug != export.Workspace.Slug {
		t.Fatalf("roundtrip slug drift: %q != %q", decoded.Workspace.Slug, export.Workspace.Slug)
	}
}

func TestExportWorkspaceUnknownSlugReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	_, err := ExportWorkspace(ctx, st, "does-not-exist")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestExportImportRoundtripReproducesOperationalConfig(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seeded, err := SeedExample(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	perRunWorktree := true
	timeoutSeconds := seeded.Workspace.DefaultTimeoutSeconds
	autoChainMaxDepth := seeded.Workspace.AutoChainMaxDepth
	autoChainDailyRunLimit := seeded.Workspace.AutoChainDailyRunLimit
	autoChainDailyCostMicros := seeded.Workspace.AutoChainDailyCostMicros
	if _, err := st.UpdateWorkspace(ctx, seeded.Workspace.ID, store.UpdateWorkspaceInput{
		Name:                     seeded.Workspace.Name,
		Description:              seeded.Workspace.Description,
		WorkingDir:               seeded.Workspace.WorkingDir,
		OutputDir:                seeded.Workspace.OutputDir,
		DefaultTimeoutSeconds:    &timeoutSeconds,
		AutoChainEnabled:         &seeded.Workspace.AutoChainEnabled,
		AutoChainMaxDepth:        &autoChainMaxDepth,
		AutoChainDailyRunLimit:   &autoChainDailyRunLimit,
		AutoChainDailyCostMicros: &autoChainDailyCostMicros,
		AutoChainDryRun:          &seeded.Workspace.AutoChainDryRun,
		AutoCloseOnRunDone:       &seeded.Workspace.AutoCloseOnRunDone,
		PerRunWorktree:           &perRunWorktree,
	}); err != nil {
		t.Fatal(err)
	}

	export, err := ExportWorkspace(ctx, st, "demo-studio")
	if err != nil {
		t.Fatal(err)
	}
	if !export.Workspace.PerRunWorktree {
		t.Fatalf("export should preserve per_run_worktree=true")
	}

	imported, err := ImportWorkspace(ctx, st, export, ImportOptions{DestSlug: "demo-studio-clone"})
	if err != nil {
		t.Fatal(err)
	}
	if imported.Slug != "demo-studio-clone" {
		t.Fatalf("imported slug=%q want demo-studio-clone", imported.Slug)
	}
	if imported.AutoChainEnabled != export.Workspace.AutoChainEnabled {
		t.Fatalf("auto_chain_enabled drift: %v vs %v", imported.AutoChainEnabled, export.Workspace.AutoChainEnabled)
	}
	if imported.PerRunWorktree != export.Workspace.PerRunWorktree {
		t.Fatalf("per_run_worktree drift: %v vs %v", imported.PerRunWorktree, export.Workspace.PerRunWorktree)
	}

	agents, err := st.ListAgents(ctx, imported.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != len(export.Agents) {
		t.Fatalf("agent count drift: imported=%d export=%d", len(agents), len(export.Agents))
	}
	var foundMain bool
	for _, a := range agents {
		if a.IsMain {
			foundMain = true
			if a.Name != "Lead" {
				t.Fatalf("imported main agent name=%q want Lead", a.Name)
			}
			if !strings.Contains(a.Instructions, "통합 문서를 작성하지 마라") {
				t.Fatalf("hub guard instructions lost in roundtrip")
			}
		}
	}
	if !foundMain {
		t.Fatalf("no main agent after import")
	}
}

func TestImportWorkspaceSlugConflictReturnsErrConflict(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if _, err := SeedExample(ctx, st); err != nil {
		t.Fatal(err)
	}
	export, err := ExportWorkspace(ctx, st, "demo-studio")
	if err != nil {
		t.Fatal(err)
	}
	// Default DestSlug -> reuses "demo-studio" -> conflict.
	if _, err := ImportWorkspace(ctx, st, export, ImportOptions{}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("expected ErrConflict for duplicate slug, got %v", err)
	}
}

func TestImportWorkspaceRejectsUnsupportedFormatVersion(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if _, err := ImportWorkspace(ctx, st, WorkspaceExport{FormatVersion: 9999}, ImportOptions{DestSlug: "any"}); err == nil {
		t.Fatalf("expected error for future format_version, got nil")
	}
}
