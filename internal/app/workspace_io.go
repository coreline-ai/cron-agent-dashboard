package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

// WorkspaceExportFormatVersion bumps when the on-disk schema changes in a way
// importers must understand. Bumping is a deliberate decision — once a
// version ships, exporters should keep emitting it (or a higher one) so old
// import binaries can still read newer exports cleanly when they have to.
const WorkspaceExportFormatVersion = 1

// WorkspaceExport is the JSON envelope for a single workspace's operational
// configuration: workspace + agents + skills + agent skill assignments +
// autopilot rules. Issues, runs, comments, and run_events are deliberately
// out of scope for this format — those have a separate plan because their
// data volume and PII surface need different masking policy.
type WorkspaceExport struct {
	FormatVersion int                          `json:"format_version"`
	ExportedAt    string                       `json:"exported_at"`
	Workspace     WorkspaceExportWorkspace     `json:"workspace"`
	Agents        []WorkspaceExportAgent       `json:"agents"`
	Skills        []WorkspaceExportSkill       `json:"skills,omitempty"`
	AgentSkills   []WorkspaceExportAgentSkill  `json:"agent_skills,omitempty"`
	Autopilot     []WorkspaceExportAutopilot   `json:"autopilot_rules,omitempty"`
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

// ExportWorkspace gathers the operational configuration for a single
// workspace into a portable WorkspaceExport snapshot.
func ExportWorkspace(ctx context.Context, st *store.Store, slug string) (WorkspaceExport, error) {
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

	return WorkspaceExport{
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
		},
		Agents:      exportAgents,
		Skills:      exportSkills,
		AgentSkills: exportAgentSkills,
		Autopilot:   exportRules,
	}, nil
}

// ImportOptions controls how an incoming WorkspaceExport is applied. DestSlug
// overrides the slug recorded in the export (so the same export can be
// imported into multiple environments with different slugs). When DestSlug
// is empty the export's slug is used.
type ImportOptions struct {
	DestSlug string
}

// ImportWorkspace creates a fresh workspace from the WorkspaceExport snapshot.
// It is intentionally non-destructive: slug collisions return store.ErrConflict
// rather than overwriting. Issue / run / comment / run_event history is out
// of scope for this format — only operational configuration is restored.
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

	return ws, nil
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

