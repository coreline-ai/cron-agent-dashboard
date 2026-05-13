package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreline-ai/corn-agent-dashboard/internal/store"
)

type StartupCheckReport struct {
	IntegrityCheck           string   `json:"integrity_check"`
	JournalMode              string   `json:"journal_mode"`
	ForeignKeysEnabled       bool     `json:"foreign_keys_enabled"`
	BusyTimeoutMS            int      `json:"busy_timeout_ms"`
	WorkspaceCount           int      `json:"workspace_count"`
	ForeignKeyViolationCount int      `json:"foreign_key_violation_count"`
	MainAgentIssues          []string `json:"main_agent_issues"`
	OrphanRunsRecovered      int64    `json:"orphan_runs_recovered"`
}

func (r StartupCheckReport) LogFields() []any {
	return []any{
		"integrity", r.IntegrityCheck,
		"journal_mode", r.JournalMode,
		"foreign_keys", r.ForeignKeysEnabled,
		"busy_timeout_ms", r.BusyTimeoutMS,
		"workspaces", r.WorkspaceCount,
		"foreign_key_violations", r.ForeignKeyViolationCount,
		"orphan_runs_recovered", r.OrphanRunsRecovered,
	}
}

func RunStartupSelfCheck(ctx context.Context, st *store.Store) (StartupCheckReport, error) {
	if st == nil {
		return StartupCheckReport{}, fmt.Errorf("startup self-check: store is nil")
	}
	var report StartupCheckReport
	recovered, err := st.RecoverOrphanRuns(ctx)
	if err != nil {
		return report, fmt.Errorf("recover orphan runs: %w", err)
	}
	report.OrphanRunsRecovered = recovered

	if err := st.DB().GetContext(ctx, &report.IntegrityCheck, `PRAGMA integrity_check`); err != nil {
		return report, fmt.Errorf("pragma integrity_check: %w", err)
	}
	if strings.ToLower(strings.TrimSpace(report.IntegrityCheck)) != "ok" {
		return report, fmt.Errorf("pragma integrity_check failed: %s", report.IntegrityCheck)
	}

	if err := st.DB().GetContext(ctx, &report.JournalMode, `PRAGMA journal_mode`); err != nil {
		return report, fmt.Errorf("pragma journal_mode: %w", err)
	}
	var foreignKeys int
	if err := st.DB().GetContext(ctx, &foreignKeys, `PRAGMA foreign_keys`); err != nil {
		return report, fmt.Errorf("pragma foreign_keys: %w", err)
	}
	report.ForeignKeysEnabled = foreignKeys == 1
	if !report.ForeignKeysEnabled {
		return report, fmt.Errorf("pragma foreign_keys is disabled")
	}
	if err := st.DB().GetContext(ctx, &report.BusyTimeoutMS, `PRAGMA busy_timeout`); err != nil {
		return report, fmt.Errorf("pragma busy_timeout: %w", err)
	}
	if report.BusyTimeoutMS <= 0 {
		return report, fmt.Errorf("pragma busy_timeout is disabled")
	}

	violations, err := countRows(ctx, st, `PRAGMA foreign_key_check`)
	if err != nil {
		return report, fmt.Errorf("pragma foreign_key_check: %w", err)
	}
	report.ForeignKeyViolationCount = violations
	if violations > 0 {
		return report, fmt.Errorf("foreign_key_check reported %d violation(s)", violations)
	}

	if err := st.DB().GetContext(ctx, &report.WorkspaceCount, `SELECT COUNT(*) FROM workspace`); err != nil {
		return report, fmt.Errorf("count workspaces: %w", err)
	}
	report.MainAgentIssues, err = mainAgentIssues(ctx, st)
	if err != nil {
		return report, err
	}
	if len(report.MainAgentIssues) > 0 {
		return report, fmt.Errorf("workspace main agent invariant failed: %s", strings.Join(report.MainAgentIssues, "; "))
	}
	return report, nil
}

func countRows(ctx context.Context, st *store.Store, query string) (int, error) {
	rows, err := st.DB().QueryxContext(ctx, query)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var n int
	for rows.Next() {
		n++
	}
	return n, rows.Err()
}

func mainAgentIssues(ctx context.Context, st *store.Store) ([]string, error) {
	rows := []struct {
		Slug      string `db:"slug"`
		MainCount int    `db:"main_count"`
	}{}
	err := st.DB().SelectContext(ctx, &rows, `
SELECT w.slug, COUNT(a.id) AS main_count
FROM workspace w
LEFT JOIN agent a ON a.workspace_id = w.id AND a.is_main = 1
GROUP BY w.id, w.slug
HAVING COUNT(a.id) != 1
ORDER BY w.slug`)
	if err != nil {
		return nil, fmt.Errorf("main agent invariant query: %w", err)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, fmt.Sprintf("%s has %d main agents", row.Slug, row.MainCount))
	}
	return out, nil
}
