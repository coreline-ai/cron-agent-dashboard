package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
	"github.com/google/uuid"
)

// WorkspaceExportFormatVersion bumps when the on-disk schema changes in a way
// importers must understand. Bumping is a deliberate decision — once a
// version ships, exporters should keep emitting it (or a higher one) so old
// import binaries can still read newer exports cleanly when they have to.
const WorkspaceExportFormatVersion = 1

// WorkspaceExport is the JSON envelope for a single workspace's operational
// configuration: workspace + agents + skills + agent skill assignments +
// autopilot rules. History (issues / runs / comments / attachment metadata)
// is included only when the exporter is invoked with
// ExportWorkspaceOptions.IncludeHistory; importers materialize those rows only
// when ImportOptions.IncludeHistory is enabled, so existing config-only import
// flows stay non-destructive without bumping FormatVersion.
type WorkspaceExport struct {
	FormatVersion int                         `json:"format_version"`
	ExportedAt    string                      `json:"exported_at"`
	Workspace     WorkspaceExportWorkspace    `json:"workspace"`
	Agents        []WorkspaceExportAgent      `json:"agents"`
	Skills        []WorkspaceExportSkill      `json:"skills,omitempty"`
	AgentSkills   []WorkspaceExportAgentSkill `json:"agent_skills,omitempty"`
	Autopilot     []WorkspaceExportAutopilot  `json:"autopilot_rules,omitempty"`

	// History payload. Present only when ExportWorkspaceOptions.IncludeHistory
	// is true. ImportWorkspace rematerializes these rows when
	// ImportOptions.IncludeHistory is true; otherwise they are intentionally
	// ignored for config-only imports.
	Issues      []WorkspaceExportIssue      `json:"issues,omitempty"`
	Comments    []WorkspaceExportComment    `json:"comments,omitempty"`
	Runs        []WorkspaceExportRun        `json:"runs,omitempty"`
	Attachments []WorkspaceExportAttachment `json:"attachments,omitempty"`
}

// WorkspaceExportWorkspace mirrors the operational settings on a workspace.
// Identifier sequence (next_issue_seq) and timestamps are not exported —
// imports always start fresh.
type WorkspaceExportWorkspace struct {
	Name                     string `json:"name"`
	Slug                     string `json:"slug"`
	Description              string `json:"description"`
	IdentifierPrefix         string `json:"identifier_prefix"`
	WorkingDir               string `json:"working_dir,omitempty"`
	OutputDir                string `json:"output_dir,omitempty"`
	DefaultTimeoutSeconds    int    `json:"default_timeout_seconds"`
	AutoChainEnabled         bool   `json:"auto_chain_enabled"`
	AutoChainMaxDepth        int    `json:"auto_chain_max_depth"`
	AutoChainDailyRunLimit   int    `json:"auto_chain_daily_run_limit"`
	AutoChainDailyCostMicros int64  `json:"auto_chain_daily_cost_micros"`
	AutoChainDryRun          bool   `json:"auto_chain_dry_run"`
	AutoCloseOnRunDone       bool   `json:"auto_close_on_run_done"`
	PerRunWorktree           bool   `json:"per_run_worktree"`
}

type WorkspaceExportAgent struct {
	Name                   string `json:"name"`
	Runtime                string `json:"runtime"`
	Model                  string `json:"model,omitempty"`
	Instructions           string `json:"instructions"`
	Summary                string `json:"summary,omitempty"`
	Tags                   string `json:"tags,omitempty"`
	TimeoutSecondsOverride *int   `json:"timeout_seconds_override,omitempty"`
	RetryPolicyJSON        string `json:"retry_policy_json,omitempty"`
	IsMain                 bool   `json:"is_main"`
}

type WorkspaceExportSkill struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Triggers    []string `json:"triggers,omitempty"`
	Content     string   `json:"content"`
	SourceType  string   `json:"source_type"`
	SourceURL   string   `json:"source_url,omitempty"`
	SourceRef   string   `json:"source_ref,omitempty"`
	LocalPath   string   `json:"local_path,omitempty"`
	ContentHash string   `json:"content_hash,omitempty"`
	TrustLevel  string   `json:"trust_level"`
	Enabled     bool     `json:"enabled"`
}

// WorkspaceExportAgentSkill references agents and skills by their names
// (case-insensitive within a workspace) rather than database IDs so a roundtrip
// to a new workspace stays valid.
type WorkspaceExportAgentSkill struct {
	AgentName      string `json:"agent_name"`
	SkillName      string `json:"skill_name"`
	ActivationMode string `json:"activation_mode"`
	Priority       int    `json:"priority"`
	Enabled        bool   `json:"enabled"`
}

type WorkspaceExportAutopilot struct {
	Name               string `json:"name"`
	CronExpr           string `json:"cron_expr"`
	IssueTitleTemplate string `json:"issue_title_template"`
	IssueBodyTemplate  string `json:"issue_body_template,omitempty"`
	AssigneeAgentName  string `json:"assignee_agent_name,omitempty"`
	Enabled            bool   `json:"enabled"`
}

// WorkspaceExportIssue is a frozen historical snapshot of a single issue.
// Fields are flattened (no nested runs/comments) so each slice in
// WorkspaceExport can be inspected independently and so very large workspaces
// stream cleanly when written to disk.
type WorkspaceExportIssue struct {
	Identifier        string `json:"identifier"`
	Title             string `json:"title"`
	Body              string `json:"body,omitempty"`
	Status            string `json:"status"`
	ExecutionStatus   string `json:"execution_status"`
	AssigneeAgentName string `json:"assignee_agent_name,omitempty"`
	CreatedBy         string `json:"created_by,omitempty"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at,omitempty"`
}

// WorkspaceExportComment carries the comment thread for an issue. IssueIdentifier
// is the foreign key field exports use to relate rows because Issue.ID values
// would not survive an import into a different database.
type WorkspaceExportComment struct {
	IssueIdentifier string `json:"issue_identifier"`
	AuthorType      string `json:"author_type"`
	AuthorAgentName string `json:"author_agent_name,omitempty"`
	Content         string `json:"content"`
	Truncated       bool   `json:"truncated,omitempty"`
	CreatedAt       string `json:"created_at"`
}

// WorkspaceExportRun captures the operational metadata of a finished or
// queued run. stdout file paths are NOT exported (they are local-disk only);
// callers who need the log payload should archive the run log directory
// separately.
type WorkspaceExportRun struct {
	IssueIdentifier string `json:"issue_identifier"`
	AgentName       string `json:"agent_name,omitempty"`
	Status          string `json:"status"`
	TriggerType     string `json:"trigger_type,omitempty"`
	ChainID         string `json:"chain_id,omitempty"`
	ChainDepth      int    `json:"chain_depth,omitempty"`
	EnqueuedAt      string `json:"enqueued_at,omitempty"`
	StartedAt       string `json:"started_at,omitempty"`
	FinishedAt      string `json:"finished_at,omitempty"`
	ModelResolved   string `json:"model_resolved,omitempty"`
	InputTokens     int64  `json:"input_tokens,omitempty"`
	OutputTokens    int64  `json:"output_tokens,omitempty"`
	TotalCostMicros int64  `json:"total_cost_micros,omitempty"`
	TerminalReason  string `json:"terminal_reason,omitempty"`
	FailureKind     string `json:"failure_kind,omitempty"`
	ErrorMessage    string `json:"error_message,omitempty"`
}

// WorkspaceExportAttachment carries metadata only — binary content is omitted
// (a separate bundling format will handle that). Operators who archive the
// run logs / attachments directory alongside the JSON export can recombine the
// payload by `filename` at restore time.
type WorkspaceExportAttachment struct {
	IssueIdentifier string `json:"issue_identifier"`
	Filename        string `json:"filename"`
	MIMEType        string `json:"mime_type,omitempty"`
	SizeBytes       int64  `json:"size_bytes,omitempty"`
	CreatedAt       string `json:"created_at"`
}

// ExportWorkspaceOptions controls v2-style history export and PII masking.
type ExportWorkspaceOptions struct {
	IncludeHistory bool
	MaskPII        bool
}

// ExportWorkspace gathers the operational configuration for a single
// workspace. History (issues/runs/comments/attachments) is excluded; use
// ExportWorkspaceWithOptions for v2-style snapshots.
func ExportWorkspace(ctx context.Context, st *store.Store, slug string) (WorkspaceExport, error) {
	return ExportWorkspaceWithOptions(ctx, st, slug, ExportWorkspaceOptions{})
}

// ExportWorkspaceWithOptions is the v2 entry point: caller chooses whether
// to include history and whether to mask PII in user-visible text fields.
func ExportWorkspaceWithOptions(ctx context.Context, st *store.Store, slug string, opts ExportWorkspaceOptions) (WorkspaceExport, error) {
	if st == nil {
		return WorkspaceExport{}, errors.New("export: store is nil")
	}
	ws, _, err := st.GetWorkspace(ctx, slug)
	if err != nil {
		return WorkspaceExport{}, fmt.Errorf("export: get workspace: %w", err)
	}
	agents, err := st.ListAgents(ctx, ws.ID)
	if err != nil {
		return WorkspaceExport{}, fmt.Errorf("export: list agents: %w", err)
	}
	skills, err := st.ListSkills(ctx, ws.ID)
	if err != nil {
		return WorkspaceExport{}, fmt.Errorf("export: list skills: %w", err)
	}
	rules, err := st.ListAutopilotRules(ctx, ws.ID)
	if err != nil {
		return WorkspaceExport{}, fmt.Errorf("export: list autopilot rules: %w", err)
	}

	agentByID := make(map[string]string, len(agents))
	exportAgents := make([]WorkspaceExportAgent, 0, len(agents))
	for _, a := range agents {
		agentByID[a.ID] = a.Name
		var override *int
		if a.TimeoutSecondsOverride.Valid {
			v := int(a.TimeoutSecondsOverride.Int64)
			override = &v
		}
		exportAgents = append(exportAgents, WorkspaceExportAgent{
			Name:                   a.Name,
			Runtime:                a.Runtime,
			Model:                  a.Model,
			Instructions:           a.Instructions,
			Summary:                a.Summary,
			Tags:                   a.Tags,
			TimeoutSecondsOverride: override,
			RetryPolicyJSON:        a.RetryPolicyJSON,
			IsMain:                 a.IsMain,
		})
	}

	skillByID := make(map[string]string, len(skills))
	exportSkills := make([]WorkspaceExportSkill, 0, len(skills))
	for _, s := range skills {
		skillByID[s.ID] = s.Name
		exportSkills = append(exportSkills, WorkspaceExportSkill{
			Name:        s.Name,
			Description: s.Description,
			Triggers:    s.Triggers,
			Content:     s.Content,
			SourceType:  s.SourceType,
			SourceURL:   s.SourceURL,
			SourceRef:   s.SourceRef,
			LocalPath:   s.LocalPath,
			ContentHash: s.ContentHash,
			TrustLevel:  s.TrustLevel,
			Enabled:     s.Enabled,
		})
	}

	var exportAgentSkills []WorkspaceExportAgentSkill
	for _, a := range agents {
		assignments, err := st.ListAgentSkills(ctx, a.ID)
		if err != nil {
			return WorkspaceExport{}, fmt.Errorf("export: list agent_skills for %s: %w", a.Name, err)
		}
		for _, as := range assignments {
			skillName := skillByID[as.SkillID]
			if skillName == "" {
				continue
			}
			exportAgentSkills = append(exportAgentSkills, WorkspaceExportAgentSkill{
				AgentName:      a.Name,
				SkillName:      skillName,
				ActivationMode: as.ActivationMode,
				Priority:       as.Priority,
				Enabled:        as.Enabled,
			})
		}
	}

	exportRules := make([]WorkspaceExportAutopilot, 0, len(rules))
	for _, r := range rules {
		exportRules = append(exportRules, WorkspaceExportAutopilot{
			Name:               r.Name,
			CronExpr:           r.CronExpr,
			IssueTitleTemplate: r.IssueTitleTemplate,
			IssueBodyTemplate:  r.IssueBodyTemplate,
			AssigneeAgentName:  agentByID[r.AssigneeAgentID],
			Enabled:            r.Enabled,
		})
	}

	out := WorkspaceExport{
		FormatVersion: WorkspaceExportFormatVersion,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Workspace: WorkspaceExportWorkspace{
			Name:                     ws.Name,
			Slug:                     ws.Slug,
			Description:              ws.Description,
			IdentifierPrefix:         ws.IdentifierPrefix,
			WorkingDir:               ws.WorkingDir,
			OutputDir:                ws.OutputDir,
			DefaultTimeoutSeconds:    ws.DefaultTimeoutSeconds,
			AutoChainEnabled:         ws.AutoChainEnabled,
			AutoChainMaxDepth:        ws.AutoChainMaxDepth,
			AutoChainDailyRunLimit:   ws.AutoChainDailyRunLimit,
			AutoChainDailyCostMicros: ws.AutoChainDailyCostMicros,
			AutoChainDryRun:          ws.AutoChainDryRun,
			AutoCloseOnRunDone:       ws.AutoCloseOnRunDone,
			PerRunWorktree:           ws.PerRunWorktree,
		},
		Agents:      exportAgents,
		Skills:      exportSkills,
		AgentSkills: exportAgentSkills,
		Autopilot:   exportRules,
	}

	if opts.IncludeHistory {
		if err := appendWorkspaceHistory(ctx, st, ws.ID, agentByID, opts.MaskPII, &out); err != nil {
			return WorkspaceExport{}, err
		}
	}

	return out, nil
}

// appendWorkspaceHistory walks every issue under the workspace and populates
// the v2 history fields. PII masking, if requested, is applied to user-visible
// text fields (issue title/body, comment content, run trigger_content_snapshot
// / error_message) using maskPII.
func appendWorkspaceHistory(ctx context.Context, st *store.Store, workspaceID string, agentByID map[string]string, mask bool, out *WorkspaceExport) error {
	issues, err := st.ListIssues(ctx, workspaceID, store.ListIssuesFilter{Limit: 1_000_000})
	if err != nil {
		return fmt.Errorf("export: list issues: %w", err)
	}
	for _, issue := range issues {
		out.Issues = append(out.Issues, WorkspaceExportIssue{
			Identifier:        issue.Identifier,
			Title:             maybeMaskPII(issue.Title, mask),
			Body:              maybeMaskPII(issue.Body, mask),
			Status:            issue.Status,
			ExecutionStatus:   issue.ExecutionStatus,
			AssigneeAgentName: issue.AssigneeAgentName,
			CreatedBy:         issue.CreatedBy,
			CreatedAt:         issue.CreatedAt,
			UpdatedAt:         issue.UpdatedAt,
		})

		comments, err := st.ListComments(ctx, issue.ID)
		if err != nil {
			return fmt.Errorf("export: list comments for %s: %w", issue.Identifier, err)
		}
		for _, c := range comments {
			out.Comments = append(out.Comments, WorkspaceExportComment{
				IssueIdentifier: issue.Identifier,
				AuthorType:      c.AuthorType,
				AuthorAgentName: c.AuthorAgentName,
				Content:         maybeMaskPII(c.Content, mask),
				Truncated:       c.Truncated,
				CreatedAt:       c.CreatedAt,
			})
		}

		runs, err := st.ListRuns(ctx, issue.ID)
		if err != nil {
			return fmt.Errorf("export: list runs for %s: %w", issue.Identifier, err)
		}
		for _, r := range runs {
			out.Runs = append(out.Runs, WorkspaceExportRun{
				IssueIdentifier: issue.Identifier,
				AgentName:       r.AgentName,
				Status:          r.Status,
				TriggerType:     r.TriggerType,
				ChainID:         r.ChainID,
				ChainDepth:      r.ChainDepth,
				EnqueuedAt:      r.EnqueuedAt,
				StartedAt:       r.StartedAt,
				FinishedAt:      r.FinishedAt,
				ModelResolved:   r.ModelResolved,
				InputTokens:     r.InputTokens,
				OutputTokens:    r.OutputTokens,
				TotalCostMicros: r.TotalCostMicros,
				TerminalReason:  r.TerminalReason,
				FailureKind:     r.FailureKind,
				ErrorMessage:    maybeMaskPII(r.ErrorMessage, mask),
			})
		}

		attachments, err := st.ListAttachments(ctx, issue.ID)
		if err != nil {
			return fmt.Errorf("export: list attachments for %s: %w", issue.Identifier, err)
		}
		for _, a := range attachments {
			out.Attachments = append(out.Attachments, WorkspaceExportAttachment{
				IssueIdentifier: issue.Identifier,
				Filename:        a.Filename,
				MIMEType:        a.ContentType,
				SizeBytes:       a.SizeBytes,
				CreatedAt:       a.CreatedAt,
			})
		}
	}
	// agentByID is reserved for future fields that need agent renaming
	// (e.g. mention rewriting); keep it in the signature so callers do not
	// reload the agent list when that lands.
	_ = agentByID
	return nil
}

// ImportOptions controls how an incoming WorkspaceExport is applied. DestSlug
// overrides the slug recorded in the export (so the same export can be
// imported into multiple environments with different slugs). When DestSlug
// is empty the export's slug is used.
type ImportOptions struct {
	DestSlug string
	// IncludeHistory rebuilds the issues / comments / runs / attachment
	// metadata that the v2 export emits. Identifiers and timestamps survive
	// the round-trip; runs that were still in flight at export time are
	// forced to 'cancelled' so they do not become permanent ghosts.
	IncludeHistory bool
}

// ImportWorkspace creates a fresh workspace from the WorkspaceExport snapshot.
// It is intentionally non-destructive: slug collisions return store.ErrConflict
// rather than overwriting. By default only operational configuration is
// restored; set ImportOptions.IncludeHistory to rematerialize exported issue /
// run / comment / attachment metadata.
func ImportWorkspace(ctx context.Context, st *store.Store, export WorkspaceExport, opts ImportOptions) (store.Workspace, error) {
	if st == nil {
		return store.Workspace{}, errors.New("import: store is nil")
	}
	if export.FormatVersion != WorkspaceExportFormatVersion {
		return store.Workspace{}, fmt.Errorf("import: unsupported format_version %d (this binary expects %d)", export.FormatVersion, WorkspaceExportFormatVersion)
	}

	slug := export.Workspace.Slug
	if opts.DestSlug != "" {
		slug = opts.DestSlug
	}
	if _, _, err := st.GetWorkspace(ctx, slug); err == nil {
		return store.Workspace{}, fmt.Errorf("import: slug %q: %w", slug, store.ErrConflict)
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.Workspace{}, fmt.Errorf("import: probe slug %q: %w", slug, err)
	}

	var mainSpec *WorkspaceExportAgent
	var workerSpecs []WorkspaceExportAgent
	for i := range export.Agents {
		a := export.Agents[i]
		if a.IsMain {
			if mainSpec != nil {
				return store.Workspace{}, errors.New("import: more than one main agent in export")
			}
			mainSpec = &a
		} else {
			workerSpecs = append(workerSpecs, a)
		}
	}
	if mainSpec == nil {
		return store.Workspace{}, errors.New("import: export has no main agent")
	}

	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:                     export.Workspace.Name,
		Slug:                     slug,
		Description:              export.Workspace.Description,
		IdentifierPrefix:         export.Workspace.IdentifierPrefix,
		WorkingDir:               export.Workspace.WorkingDir,
		OutputDir:                export.Workspace.OutputDir,
		DefaultTimeoutSeconds:    export.Workspace.DefaultTimeoutSeconds,
		AutoChainEnabled:         export.Workspace.AutoChainEnabled,
		AutoChainMaxDepth:        export.Workspace.AutoChainMaxDepth,
		AutoChainDailyRunLimit:   intPtrIfPositive(export.Workspace.AutoChainDailyRunLimit),
		AutoChainDailyCostMicros: export.Workspace.AutoChainDailyCostMicros,
		AutoChainDryRun:          export.Workspace.AutoChainDryRun,
		AutoCloseOnRunDone:       boolPtr(export.Workspace.AutoCloseOnRunDone),
		PerRunWorktree:           export.Workspace.PerRunWorktree,
		MainAgent: store.CreateAgentInput{
			Name:                   mainSpec.Name,
			Runtime:                mainSpec.Runtime,
			Model:                  mainSpec.Model,
			Instructions:           mainSpec.Instructions,
			Summary:                mainSpec.Summary,
			Tags:                   mainSpec.Tags,
			TimeoutSecondsOverride: mainSpec.TimeoutSecondsOverride,
			RetryPolicyJSON:        mainSpec.RetryPolicyJSON,
		},
	})
	if err != nil {
		return store.Workspace{}, fmt.Errorf("import: create workspace: %w", err)
	}

	agentIDByName := map[string]string{}
	// Look up the created main agent so we can resolve mentions of it.
	mainAgents, err := st.ListAgents(ctx, ws.ID)
	if err != nil {
		return store.Workspace{}, fmt.Errorf("import: list seeded agents: %w", err)
	}
	for _, a := range mainAgents {
		agentIDByName[strings.ToLower(a.Name)] = a.ID
	}

	for _, w := range workerSpecs {
		a, err := st.CreateAgent(ctx, ws.ID, store.CreateAgentInput{
			Name:                   w.Name,
			Runtime:                w.Runtime,
			Model:                  w.Model,
			Instructions:           w.Instructions,
			Summary:                w.Summary,
			Tags:                   w.Tags,
			TimeoutSecondsOverride: w.TimeoutSecondsOverride,
			RetryPolicyJSON:        w.RetryPolicyJSON,
		})
		if err != nil {
			return store.Workspace{}, fmt.Errorf("import: create worker %s: %w", w.Name, err)
		}
		agentIDByName[strings.ToLower(a.Name)] = a.ID
	}

	skillIDByName := map[string]string{}
	for _, s := range export.Skills {
		created, err := st.UpsertSkill(ctx, ws.ID, store.UpsertSkillInput{
			Name:        s.Name,
			Description: s.Description,
			Triggers:    s.Triggers,
			Content:     s.Content,
			SourceType:  s.SourceType,
			SourceURL:   s.SourceURL,
			SourceRef:   s.SourceRef,
			LocalPath:   s.LocalPath,
			ContentHash: s.ContentHash,
			TrustLevel:  s.TrustLevel,
			Enabled:     boolPtr(s.Enabled),
		})
		if err != nil {
			return store.Workspace{}, fmt.Errorf("import: upsert skill %s: %w", s.Name, err)
		}
		skillIDByName[strings.ToLower(created.Name)] = created.ID
	}

	for _, as := range export.AgentSkills {
		agentID, ok := agentIDByName[strings.ToLower(as.AgentName)]
		if !ok {
			return store.Workspace{}, fmt.Errorf("import: agent_skill references unknown agent %q", as.AgentName)
		}
		skillID, ok := skillIDByName[strings.ToLower(as.SkillName)]
		if !ok {
			return store.Workspace{}, fmt.Errorf("import: agent_skill references unknown skill %q", as.SkillName)
		}
		enabled := as.Enabled
		if _, err := st.AssignAgentSkill(ctx, agentID, store.AssignAgentSkillInput{
			SkillID:        skillID,
			ActivationMode: as.ActivationMode,
			Priority:       as.Priority,
			Enabled:        &enabled,
		}); err != nil {
			return store.Workspace{}, fmt.Errorf("import: assign skill %s -> %s: %w", as.SkillName, as.AgentName, err)
		}
	}

	for _, r := range export.Autopilot {
		assigneeAgentID := ""
		if r.AssigneeAgentName != "" {
			if id, ok := agentIDByName[strings.ToLower(r.AssigneeAgentName)]; ok {
				assigneeAgentID = id
			}
		}
		if _, err := st.CreateAutopilotRule(ctx, ws.ID, store.UpsertAutopilotInput{
			Name:               r.Name,
			CronExpr:           r.CronExpr,
			IssueTitleTemplate: r.IssueTitleTemplate,
			IssueBodyTemplate:  r.IssueBodyTemplate,
			AssigneeAgentID:    assigneeAgentID,
			Enabled:            r.Enabled,
		}); err != nil {
			return store.Workspace{}, fmt.Errorf("import: create autopilot rule %s: %w", r.Name, err)
		}
	}

	if opts.IncludeHistory {
		if err := restoreWorkspaceHistory(ctx, st, ws.ID, agentIDByName, export); err != nil {
			return store.Workspace{}, fmt.Errorf("import: restore history: %w", err)
		}
	}

	return ws, nil
}

// restoreWorkspaceHistory inserts the v2 export's issues / comments / runs
// straight into the destination workspace. The path bypasses
// CreateIssueWithInitialRun (and the trigger pipeline it carries) because
// the rows already represent historical state that needs to land verbatim,
// not new work that needs to fan out into the worker pool. Identifiers are
// preserved, and workspace.next_issue_seq is advanced past the highest
// imported suffix so fresh issues on the destination workspace do not
// collide.
func restoreWorkspaceHistory(ctx context.Context, st *store.Store, workspaceID string, agentIDByName map[string]string, export WorkspaceExport) error {
	if len(export.Issues) == 0 && len(export.Comments) == 0 && len(export.Runs) == 0 {
		return nil
	}
	db := st.DB()
	// Map exported identifier → newly minted issue id so comments / runs can
	// resolve their FK.
	issueIDByIdentifier := map[string]string{}
	maxSeq := 0
	for _, issue := range export.Issues {
		newIssueID := newImportID()
		assignee := agentIDByName[strings.ToLower(issue.AssigneeAgentName)]
		createdBy := issue.CreatedBy
		if createdBy == "" {
			createdBy = "user"
		}
		status := issue.Status
		if status == "" {
			status = "open"
		}
		if _, err := db.ExecContext(ctx,
			`INSERT INTO issue(id, workspace_id, identifier, title, body, status, assignee_agent_id, created_by, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			newIssueID, workspaceID, issue.Identifier, issue.Title, issue.Body, status,
			nilIfEmpty(assignee), createdBy,
			coalesceTS(issue.CreatedAt), coalesceTS(issue.UpdatedAt),
		); err != nil {
			return fmt.Errorf("insert issue %s: %w", issue.Identifier, err)
		}
		issueIDByIdentifier[issue.Identifier] = newIssueID
		if seq := identifierSuffix(issue.Identifier); seq > maxSeq {
			maxSeq = seq
		}
	}
	if maxSeq > 0 {
		if _, err := db.ExecContext(ctx, `UPDATE workspace SET next_issue_seq=? WHERE id=? AND next_issue_seq<=?`, maxSeq+1, workspaceID, maxSeq); err != nil {
			return fmt.Errorf("advance next_issue_seq: %w", err)
		}
	}
	for _, comment := range export.Comments {
		issueID := issueIDByIdentifier[comment.IssueIdentifier]
		if issueID == "" {
			continue
		}
		author := agentIDByName[strings.ToLower(comment.AuthorAgentName)]
		authorType := comment.AuthorType
		if authorType == "" {
			authorType = "user"
		}
		if _, err := db.ExecContext(ctx,
			`INSERT INTO comment(id, issue_id, author_type, author_agent_id, content, truncated, created_at) VALUES(?,?,?,?,?,?,?)`,
			newImportID(), issueID, authorType, nilIfEmpty(author), comment.Content, boolInt8(comment.Truncated), coalesceTS(comment.CreatedAt),
		); err != nil {
			return fmt.Errorf("insert comment for %s: %w", comment.IssueIdentifier, err)
		}
	}
	for _, run := range export.Runs {
		issueID := issueIDByIdentifier[run.IssueIdentifier]
		if issueID == "" {
			continue
		}
		agentID := agentIDByName[strings.ToLower(run.AgentName)]
		if agentID == "" {
			// An agent that was active at export time but never made it into
			// the agents slice cannot be re-materialized; skip the run to
			// avoid a dangling FK.
			continue
		}
		status := run.Status
		switch status {
		case "done", "failed", "cancelled":
		default:
			status = "cancelled"
		}
		terminal := run.TerminalReason
		if terminal == "" && status == "cancelled" {
			terminal = "user_cancelled"
		}
		failureKind := run.FailureKind
		if failureKind == "" && status == "failed" {
			failureKind = "unknown"
		}
		enqueued := coalesceTS(run.EnqueuedAt)
		finished := run.FinishedAt
		if finished == "" {
			finished = enqueued
		}
		if _, err := db.ExecContext(ctx,
			`INSERT INTO run(
                id, issue_id, agent_id, status, trigger_type, trigger_content_snapshot,
                chain_id, chain_depth, agent_instructions_version,
                enqueued_at, started_at, finished_at,
                input_tokens, output_tokens, total_cost_micros, model_resolved,
                max_attempts, attempt, error_message, terminal_reason, failure_kind, cancel_reason
            ) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			newImportID(), issueID, agentID, status,
			coalesceTrigger(run.TriggerType), "",
			coalesceString(run.ChainID), run.ChainDepth, 1,
			enqueued, coalesceString(run.StartedAt), finished,
			run.InputTokens, run.OutputTokens, run.TotalCostMicros, run.ModelResolved,
			1, 1, run.ErrorMessage, terminal, failureKind, "",
		); err != nil {
			return fmt.Errorf("insert run for %s: %w", run.IssueIdentifier, err)
		}
	}
	// Attachment binaries are not part of the export, but the metadata rows
	// are useful for audit. We insert them with a sentinel storage_path so
	// the download endpoint will 5xx (file absent on disk) without leaking
	// a real workspace path. Operators who archive the binaries separately
	// can rewrite storage_path back to a real path post-import.
	for _, a := range export.Attachments {
		issueID := issueIDByIdentifier[a.IssueIdentifier]
		if issueID == "" {
			continue
		}
		if _, err := db.ExecContext(ctx,
			`INSERT INTO attachment(id, issue_id, uploaded_by, filename, content_type, size_bytes, sha256, storage_path, created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			newImportID(), issueID, "imported", a.Filename, a.MIMEType, a.SizeBytes, "", "imported-no-binary", coalesceTS(a.CreatedAt),
		); err != nil {
			return fmt.Errorf("insert attachment for %s: %w", a.IssueIdentifier, err)
		}
	}
	return nil
}

func nilIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func boolInt8(b bool) int {
	if b {
		return 1
	}
	return 0
}

func coalesceTS(ts string) string {
	if strings.TrimSpace(ts) == "" {
		return time.Now().UTC().Format(time.RFC3339)
	}
	return ts
}

func coalesceString(s string) string {
	return strings.TrimSpace(s)
}

func coalesceTrigger(t string) string {
	switch t {
	case "issue_created", "mention", "autopilot", "rerun":
		return t
	}
	return "rerun"
}

func identifierSuffix(identifier string) int {
	idx := strings.LastIndex(identifier, "-")
	if idx < 0 || idx == len(identifier)-1 {
		return 0
	}
	n := 0
	for _, r := range identifier[idx+1:] {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// newImportID generates a UUIDv4 for history rows created during import.
// Reaching across packages would require exporting store.newID(); a local
// helper keeps the dependency direction one-way.
func newImportID() string {
	return uuid.NewString()
}

func intPtrIfPositive(v int) *int {
	if v <= 0 {
		return nil
	}
	return &v
}

func boolPtr(v bool) *bool {
	return &v
}
