package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	skilldoc "github.com/coreline-ai/cron-agent-dashboard/internal/skill"
)

const skillSelectBase = `SELECT id,workspace_id,name,description,triggers_json,content,source_type,source_url,source_ref,local_path,content_hash,trust_level,enabled,created_at,updated_at FROM skill`

func (s *Store) ListSkills(ctx context.Context, workspaceID string) ([]Skill, error) {
	var out []Skill
	if err := s.db.SelectContext(ctx, &out, skillSelectBase+` WHERE workspace_id=? ORDER BY enabled DESC, lower(name) ASC`, workspaceID); err != nil {
		return nil, normalizeErr(err)
	}
	return decodeSkills(out)
}

func (s *Store) GetSkill(ctx context.Context, id string) (Skill, error) {
	var out Skill
	if err := s.db.GetContext(ctx, &out, skillSelectBase+` WHERE id=?`, id); err != nil {
		return Skill{}, normalizeErr(err)
	}
	return decodeSkill(out)
}

func (s *Store) UpsertSkill(ctx context.Context, workspaceID string, in UpsertSkillInput) (Skill, error) {
	if _, _, err := s.GetWorkspace(ctx, workspaceID); err != nil {
		return Skill{}, err
	}
	normalized, err := normalizeSkillInput(in)
	if err != nil {
		return Skill{}, err
	}
	t := now()
	var skillID string
	err = s.db.GetContext(ctx, &skillID, `SELECT id FROM skill WHERE workspace_id=? AND lower(name)=lower(?)`, workspaceID, normalized.Name)
	if err != nil && err != sql.ErrNoRows {
		return Skill{}, normalizeErr(err)
	}
	if err == sql.ErrNoRows {
		skillID = newID()
		_, err = s.db.ExecContext(ctx, `INSERT INTO skill(id,workspace_id,name,description,triggers_json,content,source_type,source_url,source_ref,local_path,content_hash,trust_level,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			skillID, workspaceID, normalized.Name, normalized.Description, normalized.TriggersJSON, normalized.Content, normalized.SourceType, normalized.SourceURL, normalized.SourceRef, normalized.LocalPath, normalized.ContentHash, normalized.TrustLevel, boolInt(normalized.Enabled), t, t)
		if err != nil {
			return Skill{}, normalizeErr(err)
		}
	} else {
		_, err = s.db.ExecContext(ctx, `UPDATE skill SET name=?,description=?,triggers_json=?,content=?,source_type=?,source_url=?,source_ref=?,local_path=?,content_hash=?,trust_level=?,enabled=?,updated_at=? WHERE id=?`,
			normalized.Name, normalized.Description, normalized.TriggersJSON, normalized.Content, normalized.SourceType, normalized.SourceURL, normalized.SourceRef, normalized.LocalPath, normalized.ContentHash, normalized.TrustLevel, boolInt(normalized.Enabled), t, skillID)
		if err != nil {
			return Skill{}, normalizeErr(err)
		}
	}
	return s.GetSkill(ctx, skillID)
}

func (s *Store) UpdateSkill(ctx context.Context, id string, in UpsertSkillInput) (Skill, error) {
	existing, err := s.GetSkill(ctx, id)
	if err != nil {
		return Skill{}, err
	}
	normalized, err := normalizeSkillInput(in)
	if err != nil {
		return Skill{}, err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE skill SET name=?,description=?,triggers_json=?,content=?,source_type=?,source_url=?,source_ref=?,local_path=?,content_hash=?,trust_level=?,enabled=?,updated_at=? WHERE id=?`,
		normalized.Name, normalized.Description, normalized.TriggersJSON, normalized.Content, normalized.SourceType, normalized.SourceURL, normalized.SourceRef, normalized.LocalPath, normalized.ContentHash, normalized.TrustLevel, boolInt(normalized.Enabled), now(), existing.ID)
	if err != nil {
		return Skill{}, normalizeErr(err)
	}
	return s.GetSkill(ctx, id)
}

func (s *Store) DeleteSkill(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM skill WHERE id=?`, id)
	if err != nil {
		return normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListAgentSkills(ctx context.Context, agentID string) ([]AgentSkill, error) {
	if _, err := s.GetAgent(ctx, agentID); err != nil {
		return nil, err
	}
	var assignments []AgentSkill
	if err := s.db.SelectContext(ctx, &assignments, `SELECT agent_id,skill_id,activation_mode,priority,enabled,created_at,updated_at FROM agent_skill WHERE agent_id=? ORDER BY enabled DESC, priority ASC`, agentID); err != nil {
		return nil, normalizeErr(err)
	}
	for i := range assignments {
		skill, err := s.GetSkill(ctx, assignments[i].SkillID)
		if err != nil {
			return nil, err
		}
		assignments[i].Skill = &skill
	}
	return assignments, nil
}

func (s *Store) AssignAgentSkill(ctx context.Context, agentID string, in AssignAgentSkillInput) (AgentSkill, error) {
	agent, err := s.GetAgent(ctx, agentID)
	if err != nil {
		return AgentSkill{}, err
	}
	skill, err := s.GetSkill(ctx, in.SkillID)
	if err != nil {
		return AgentSkill{}, err
	}
	if skill.WorkspaceID != agent.WorkspaceID {
		return AgentSkill{}, ErrValidation
	}
	mode := normalizeActivationMode(in.ActivationMode)
	if mode == "" {
		return AgentSkill{}, ErrValidation
	}
	priority := in.Priority
	if priority == 0 {
		priority = 100
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	t := now()
	_, err = s.db.ExecContext(ctx, `INSERT INTO agent_skill(agent_id,skill_id,activation_mode,priority,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?)
ON CONFLICT(agent_id, skill_id) DO UPDATE SET activation_mode=excluded.activation_mode, priority=excluded.priority, enabled=excluded.enabled, updated_at=excluded.updated_at`,
		agentID, skill.ID, mode, priority, boolInt(enabled), t, t)
	if err != nil {
		return AgentSkill{}, normalizeErr(err)
	}
	return s.getAgentSkill(ctx, agentID, skill.ID)
}

func (s *Store) DeleteAgentSkill(ctx context.Context, agentID, skillID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM agent_skill WHERE agent_id=? AND skill_id=?`, agentID, skillID)
	if err != nil {
		return normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ResolvePromptSkills(ctx context.Context, agentID, title, body, triggerSnapshot string, comments []Comment) ([]PromptSkill, error) {
	assignments, err := s.ListAgentSkills(ctx, agentID)
	if err != nil {
		return nil, err
	}
	manual := manualSkillSet(title + "\n" + body + "\n" + triggerSnapshot + "\n" + commentsText(comments))
	haystack := strings.ToLower(title + "\n" + body + "\n" + triggerSnapshot + "\n" + commentsText(comments))
	out := make([]PromptSkill, 0, len(assignments))
	for _, assignment := range assignments {
		if !assignment.Enabled || assignment.Skill == nil || !assignment.Skill.Enabled {
			continue
		}
		skill := assignment.Skill
		active, reason := skillActive(*skill, assignment.ActivationMode, manual, haystack)
		out = append(out, PromptSkill{ID: skill.ID, Name: skill.Name, Description: skill.Description, ActivationMode: assignment.ActivationMode, Content: skill.Content, Active: active, TriggerReason: reason, ContentHash: skill.ContentHash})
	}
	return out, nil
}

func (s *Store) AppendSkillsLoadedEvent(ctx context.Context, runID string, skills []PromptSkill) error {
	loaded := make([]map[string]any, 0)
	for _, skill := range skills {
		if !skill.Active {
			continue
		}
		loaded = append(loaded, map[string]any{"id": skill.ID, "name": skill.Name, "activation_mode": skill.ActivationMode, "reason": skill.TriggerReason, "content_hash": skill.ContentHash})
	}
	if len(loaded) == 0 {
		return nil
	}
	_, err := s.AppendRunEvent(ctx, RunEventInput{RunID: runID, EventType: RunEventSkillsLoaded, Message: "Agent skills loaded", Details: map[string]any{"skills": loaded}})
	return err
}

func (s *Store) getAgentSkill(ctx context.Context, agentID, skillID string) (AgentSkill, error) {
	assignments, err := s.ListAgentSkills(ctx, agentID)
	if err != nil {
		return AgentSkill{}, err
	}
	for _, assignment := range assignments {
		if assignment.SkillID == skillID {
			return assignment, nil
		}
	}
	return AgentSkill{}, ErrNotFound
}

type normalizedSkillInput struct {
	Name         string
	Description  string
	Triggers     []string
	TriggersJSON string
	Content      string
	SourceType   string
	SourceURL    string
	SourceRef    string
	LocalPath    string
	ContentHash  string
	TrustLevel   string
	Enabled      bool
}

func normalizeSkillInput(in UpsertSkillInput) (normalizedSkillInput, error) {
	if strings.TrimSpace(in.SkillMD) != "" {
		doc, err := skilldoc.Parse(in.SkillMD)
		if err != nil {
			return normalizedSkillInput{}, ErrValidation
		}
		in.Name = doc.Name
		in.Description = doc.Description
		in.Triggers = doc.Triggers
		in.Content = doc.Body
		if in.SourceType == "" {
			in.SourceType = "manual"
		}
		in.ContentHash = doc.Hash
	}
	name := strings.TrimSpace(in.Name)
	description := strings.TrimSpace(in.Description)
	content := strings.TrimSpace(in.Content)
	if name == "" || description == "" || content == "" {
		return normalizedSkillInput{}, ErrValidation
	}
	triggers := normalizeStringList(in.Triggers)
	triggerJSON, err := json.Marshal(triggers)
	if err != nil {
		return normalizedSkillInput{}, err
	}
	sourceType := normalizeSkillSourceType(in.SourceType)
	trustLevel := normalizeSkillTrustLevel(in.TrustLevel, sourceType)
	if sourceType == "" || trustLevel == "" {
		return normalizedSkillInput{}, ErrValidation
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	hash := strings.TrimSpace(in.ContentHash)
	if hash == "" {
		hash = skilldoc.Hash(content)
	}
	return normalizedSkillInput{Name: name, Description: description, Triggers: triggers, TriggersJSON: string(triggerJSON), Content: content, SourceType: sourceType, SourceURL: strings.TrimSpace(in.SourceURL), SourceRef: strings.TrimSpace(in.SourceRef), LocalPath: strings.TrimSpace(in.LocalPath), ContentHash: hash, TrustLevel: trustLevel, Enabled: enabled}, nil
}

func decodeSkills(skills []Skill) ([]Skill, error) {
	for i := range skills {
		skill, err := decodeSkill(skills[i])
		if err != nil {
			return nil, err
		}
		skills[i] = skill
	}
	return skills, nil
}

func decodeSkill(skill Skill) (Skill, error) {
	if strings.TrimSpace(skill.TriggersJSON) != "" {
		if err := json.Unmarshal([]byte(skill.TriggersJSON), &skill.Triggers); err != nil {
			return Skill{}, err
		}
	}
	if skill.Triggers == nil {
		skill.Triggers = []string{}
	}
	return skill, nil
}

func normalizeStringList(xs []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		key := strings.ToLower(x)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, x)
	}
	return out
}

func normalizeSkillSourceType(sourceType string) string {
	trimmed := strings.TrimSpace(sourceType)
	switch trimmed {
	case "", "manual":
		return "manual"
	case "local", "git", "builtin":
		return trimmed
	default:
		return ""
	}
}

func normalizeSkillTrustLevel(trustLevel, sourceType string) string {
	trimmed := strings.TrimSpace(trustLevel)
	switch trimmed {
	case "builtin", "local", "git", "untrusted":
		return trimmed
	case "":
		if sourceType == "git" {
			return "git"
		}
		return "local"
	default:
		return ""
	}
}

func normalizeActivationMode(mode string) string {
	trimmed := strings.TrimSpace(mode)
	switch trimmed {
	case "", "trigger":
		return "trigger"
	case "always", "manual":
		return trimmed
	default:
		return ""
	}
}

func commentsText(comments []Comment) string {
	var b strings.Builder
	for _, comment := range comments {
		if strings.TrimSpace(comment.Content) == "" {
			continue
		}
		b.WriteString(comment.Content)
		b.WriteByte('\n')
	}
	return b.String()
}

func manualSkillSet(text string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if !strings.HasPrefix(lower, "#skills:") && !strings.HasPrefix(lower, "skills:") {
			continue
		}
		_, list, _ := strings.Cut(trimmed, ":")
		for _, part := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
			part = strings.TrimSpace(part)
			if part != "" {
				out[strings.ToLower(part)] = true
			}
		}
	}
	return out
}

func skillActive(skill Skill, mode string, manual map[string]bool, haystack string) (bool, string) {
	name := strings.ToLower(skill.Name)
	switch mode {
	case "always":
		return true, "always"
	case "manual":
		if manual[name] {
			return true, "manual"
		}
		return false, ""
	default:
		if manual[name] {
			return true, "manual"
		}
		for _, trigger := range append(skill.Triggers, skill.Name) {
			trigger = strings.ToLower(strings.TrimSpace(trigger))
			if trigger != "" && strings.Contains(haystack, trigger) {
				return true, "trigger:" + trigger
			}
		}
		return false, ""
	}
}
