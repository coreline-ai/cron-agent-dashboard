package store

import (
	"context"
	"regexp"
	"strings"
)

var mentionRE = regexp.MustCompile(`@([\p{L}\p{N}_\-]+)`)

const commentSelectBase = `
SELECT c.id, c.issue_id, c.author_type,
       COALESCE(c.author_agent_id,'') AS author_agent_id,
       COALESCE(a.name,'') AS author_agent_name,
       COALESCE(c.run_id,'') AS run_id,
       c.content,
       c.truncated,
       CASE WHEN c.run_id IS NOT NULL THEN '/api/runs/' || c.run_id || '/log' ELSE '' END AS log_url,
       c.created_at
FROM comment c
LEFT JOIN agent a ON a.id = c.author_agent_id`

type AddCommentResult struct {
	Comment         Comment  `json:"comment"`
	MentionWarnings []string `json:"mention_warnings"`
	DispatchedRun   *Run     `json:"dispatched_run,omitempty"`
}

func (s *Store) ListComments(ctx context.Context, issueID string) ([]Comment, error) {
	var out []Comment
	err := s.db.SelectContext(ctx, &out, commentSelectBase+` WHERE c.issue_id=? ORDER BY c.created_at ASC`, issueID)
	return out, normalizeErr(err)
}

func (s *Store) AddUserComment(ctx context.Context, issueID, content string) (AddCommentResult, error) {
	if strings.TrimSpace(content) == "" {
		return AddCommentResult{}, ErrValidation
	}
	iss, err := s.GetIssue(ctx, issueID)
	if err != nil {
		return AddCommentResult{}, err
	}
	mentions := mentionRE.FindAllStringSubmatch(content, -1)
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return AddCommentResult{}, err
	}
	defer tx.Rollback()
	t := now()
	commentID := newID()
	_, err = tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,content,created_at) VALUES(?,?,'user',?,?)`, commentID, issueID, content, t)
	if err != nil {
		return AddCommentResult{}, normalizeErr(err)
	}
	warnings := []string{}
	var runID string
	if len(mentions) > 0 {
		name := mentions[0][1]
		if len(mentions) > 1 {
			warnings = append(warnings, "multiple mentions, only @"+name+" applied")
			if _, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,content,created_at) VALUES(?,?,'system',?,?)`, newID(), issueID, "멘션이 둘 이상이라 @"+name+"만 적용됩니다", t); err != nil {
				return AddCommentResult{}, normalizeErr(err)
			}
		}
		var agent Agent
		err := tx.GetContext(ctx, &agent, agentSelectBase+` WHERE workspace_id=? AND lower(name)=lower(?)`, iss.WorkspaceID, name)
		if err != nil {
			warnings = append(warnings, "@"+name+" not found")
			if _, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,content,created_at) VALUES(?,?,'system',?,?)`, newID(), issueID, "에이전트 @"+name+"을 찾을 수 없습니다", t); err != nil {
				return AddCommentResult{}, normalizeErr(err)
			}
		} else {
			var existingQueued int
			if err := tx.GetContext(ctx, &existingQueued, `SELECT COUNT(*) FROM run WHERE issue_id=? AND agent_id=? AND status='queued'`, issueID, agent.ID); err != nil {
				return AddCommentResult{}, normalizeErr(err)
			}
			if existingQueued > 0 {
				warnings = append(warnings, "already queued for @"+agent.Name)
			} else {
				runID = newID()
				maxAttempts, err := retryMaxAttemptsForAgent(ctx, tx, agent.ID)
				if err != nil {
					return AddCommentResult{}, err
				}
				instructionsVersion := agent.InstructionsVersion
				if instructionsVersion <= 0 {
					instructionsVersion = 1
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO run(id,issue_id,agent_id,status,trigger_type,trigger_comment_id,trigger_content_snapshot,enqueued_at,max_attempts,agent_instructions_version,chain_id,chain_depth) VALUES(?,?,?,'queued','mention',?,?,?,?,?,?,0)`, runID, issueID, agent.ID, commentID, capSnapshot(content), t, maxAttempts, instructionsVersion, runID); err != nil {
					return AddCommentResult{}, normalizeErr(err)
				}
				if _, err := appendRunEventTx(ctx, tx, RunEventInput{
					RunID:     runID,
					IssueID:   issueID,
					EventType: RunEventQueued,
					Message:   "Run queued by mention",
					Details: map[string]any{
						"trigger_type":       "mention",
						"trigger_comment_id": commentID,
					},
				}); err != nil {
					return AddCommentResult{}, err
				}
				if _, err := tx.ExecContext(ctx, `UPDATE issue SET status='open', updated_at=? WHERE id=?`, t, issueID); err != nil {
					return AddCommentResult{}, normalizeErr(err)
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return AddCommentResult{}, err
	}
	if runID != "" {
		s.notifyRunEvent(issueID, runID)
	}
	comments, err := s.ListComments(ctx, issueID)
	if err != nil {
		return AddCommentResult{}, err
	}
	res := AddCommentResult{MentionWarnings: warnings}
	for _, c := range comments {
		if c.ID == commentID {
			res.Comment = c
			break
		}
	}
	if runID != "" {
		r, err := s.GetRun(ctx, runID)
		if err != nil {
			return AddCommentResult{}, err
		}
		res.DispatchedRun = &r
	}
	return res, nil
}

func (s *Store) DeleteComment(ctx context.Context, id string) error {
	var c Comment
	if err := s.db.GetContext(ctx, &c, commentSelectBase+` WHERE c.id=?`, id); err != nil {
		return normalizeErr(err)
	}
	if c.AuthorType != "user" {
		return ErrState
	}
	var refs int
	if err := s.db.GetContext(ctx, &refs, `SELECT COUNT(*) FROM run WHERE trigger_comment_id=? AND status IN ('queued','running')`, id); err != nil {
		return err
	}
	if refs > 0 {
		return ErrState
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM comment WHERE id=?`, id)
	if err != nil {
		return normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}
