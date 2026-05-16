package app

import (
	"context"
	"fmt"
	"time"

	"github.com/coreline-ai/cron-agent-dashboard/internal/scheduler"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
	cron "github.com/robfig/cron/v3"
)

type AutopilotRunner struct {
	store     *store.Store
	scheduler *scheduler.CronScheduler
	loc       *time.Location
	now       func() time.Time
}

func NewAutopilotRunner(st *store.Store, loc *time.Location) *AutopilotRunner {
	if loc == nil {
		loc = time.Local
	}
	return &AutopilotRunner{
		store:     st,
		scheduler: scheduler.NewCronScheduler(loc),
		loc:       loc,
		now:       time.Now,
	}
}

func (r *AutopilotRunner) Reload(ctx context.Context) error {
	rules, err := r.store.ListEnabledAutopilotRules(ctx)
	if err != nil {
		return err
	}
	schedulerRules := make([]scheduler.Rule, 0, len(rules))
	for _, rule := range rules {
		ruleID := rule.ID
		schedulerRules = append(schedulerRules, scheduler.Rule{
			ID:      rule.ID,
			Spec:    rule.CronExpr,
			Enabled: rule.Enabled,
			Run: func(ctx context.Context, _ scheduler.Rule) error {
				return r.TriggerRule(ctx, ruleID)
			},
		})
	}
	if err := r.scheduler.Reload(schedulerRules); err != nil {
		return err
	}
	if err := r.store.ClearDisabledAutopilotNextRuns(ctx); err != nil {
		return err
	}
	return r.syncNextRunAt(ctx, rules)
}

func (r *AutopilotRunner) Stop(ctx context.Context) error {
	return r.scheduler.Stop(ctx)
}

func (r *AutopilotRunner) TriggerRule(ctx context.Context, ruleID string) error {
	_, err := r.TriggerRuleResult(ctx, ruleID)
	return err
}

func (r *AutopilotRunner) TriggerRuleResult(ctx context.Context, ruleID string) (store.AutopilotTriggerResult, error) {
	rule, err := r.store.GetAutopilotRule(ctx, ruleID)
	if err != nil {
		return store.AutopilotTriggerResult{}, err
	}
	if !rule.Enabled {
		err := fmt.Errorf("%w: autopilot rule is disabled", store.ErrState)
		return store.AutopilotTriggerResult{
			Rule:  rule,
			Error: store.AutopilotTriggerErrorMessage(err),
		}, err
	}
	now := r.now().In(r.loc)
	nextRunAt := ""
	if next, ok := r.nextRunAtForRule(rule); ok {
		nextRunAt = next
	}
	if until, snoozed := store.AutopilotSnoozedUntil(rule, now); snoozed {
		err := fmt.Errorf("%w: autopilot rule is snoozed until %s", store.ErrState, until.UTC().Format(time.RFC3339Nano))
		return store.AutopilotTriggerResult{
			Rule:  rule,
			Error: store.AutopilotTriggerErrorMessage(err),
		}, err
	}
	title, err := scheduler.RenderTemplate(rule.IssueTitleTemplate, now)
	if err != nil {
		return r.recordTriggerFailure(ctx, rule, err, nextRunAt)
	}
	body, err := scheduler.RenderTemplate(rule.IssueBodyTemplate, now)
	if err != nil {
		return r.recordTriggerFailure(ctx, rule, err, nextRunAt)
	}
	return r.store.TriggerAutopilotRuleWithContentResult(ctx, ruleID, title, body, nextRunAt)
}

func (r *AutopilotRunner) recordTriggerFailure(ctx context.Context, rule store.AutopilotRule, triggerErr error, nextRunAt string) (store.AutopilotTriggerResult, error) {
	result := store.AutopilotTriggerResult{
		Rule:  rule,
		Error: store.AutopilotTriggerErrorMessage(triggerErr),
	}
	updated, err := r.store.RecordAutopilotTriggerFailure(ctx, rule.ID, triggerErr, nextRunAt)
	if err == nil {
		result.Rule = updated
	}
	return result, triggerErr
}

func (r *AutopilotRunner) syncNextRunAt(ctx context.Context, rules []store.AutopilotRule) error {
	for _, rule := range rules {
		next, ok := r.nextRunAtForRule(rule)
		if !ok {
			continue
		}
		if err := r.store.SetAutopilotNextRun(ctx, rule.ID, next); err != nil {
			return err
		}
	}
	return nil
}

func (r *AutopilotRunner) nextRunAtForRule(rule store.AutopilotRule) (string, bool) {
	schedule, err := cron.ParseStandard(rule.CronExpr)
	if err != nil {
		return "", false
	}
	base := r.now().In(r.loc)
	if until, snoozed := store.AutopilotSnoozedUntil(rule, base); snoozed && until.After(base) {
		base = until.In(r.loc)
	}
	return schedule.Next(base).UTC().Format(time.RFC3339Nano), true
}
