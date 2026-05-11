package scheduler

import (
	"context"
	"errors"
	"sync"
	"time"

	cron "github.com/robfig/cron/v3"
)

type Rule struct {
	ID      string
	Spec    string
	Enabled bool
	Run     func(context.Context, Rule) error
}

// CronScheduler is the narrow robfig/cron wrapper used by the app. Store code
// owns rule persistence; this type only manages in-process registrations.
type CronScheduler struct {
	loc     *time.Location
	cron    *cron.Cron
	mu      sync.Mutex
	entries map[string]cron.EntryID
}

func NewCronScheduler(loc *time.Location) *CronScheduler {
	if loc == nil {
		loc = time.Local
	}
	return &CronScheduler{
		loc:     loc,
		cron:    cron.New(cron.WithLocation(loc)),
		entries: make(map[string]cron.EntryID),
	}
}

func (s *CronScheduler) Start() { s.cron.Start() }

func (s *CronScheduler) Stop(ctx context.Context) error {
	stopCtx := s.cron.Stop()
	select {
	case <-stopCtx.Done():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *CronScheduler) AddRule(rule Rule) error {
	if rule.ID == "" {
		return errors.New("scheduler: rule ID is required")
	}
	if rule.Spec == "" {
		return errors.New("scheduler: cron spec is required")
	}
	if rule.Run == nil {
		return errors.New("scheduler: rule callback is required")
	}
	if !rule.Enabled {
		return nil
	}

	id, err := s.cron.AddFunc(rule.Spec, func() {
		_ = rule.Run(context.Background(), rule)
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.entries[rule.ID] = id
	s.mu.Unlock()
	return nil
}

func (s *CronScheduler) Reload(rules []Rule) error {
	next := cron.New(cron.WithLocation(s.loc))
	nextEntries := make(map[string]cron.EntryID)
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if rule.ID == "" || rule.Spec == "" || rule.Run == nil {
			return errors.New("scheduler: invalid rule")
		}
		ruleCopy := rule
		id, err := next.AddFunc(ruleCopy.Spec, func() {
			_ = ruleCopy.Run(context.Background(), ruleCopy)
		})
		if err != nil {
			return err
		}
		nextEntries[ruleCopy.ID] = id
	}

	s.mu.Lock()
	old := s.cron
	s.cron = next
	s.entries = nextEntries
	s.mu.Unlock()

	old.Stop()
	next.Start()
	return nil
}

func (s *CronScheduler) Next(ruleID string) (time.Time, bool) {
	s.mu.Lock()
	entryID, ok := s.entries[ruleID]
	cronRef := s.cron
	s.mu.Unlock()
	if !ok {
		return time.Time{}, false
	}
	entry := cronRef.Entry(entryID)
	if entry.ID == 0 {
		return time.Time{}, false
	}
	return entry.Next, true
}
