package app

import "sync"

// IssueEventBus is the in-process notifier the SSE handler subscribes to.
// Store.AppendRunEvent (and the transactional run lifecycle paths in
// internal/store) call OnRunEvent after their commit succeeds; each
// subscriber registered for that issue gets a non-blocking wake-up. The
// bus deliberately avoids carrying the event payload itself — subscribers
// re-query ListIssueRunEventsSince with their own watermark on wake.
//
// Buffered (size 1) channels coalesce bursts: if a subscriber has not
// drained yet, additional wake-ups are dropped silently. The next wake-up
// (or the SSE handler's idle keep-alive) catches the missed delta because
// the watermark-based query still returns every row newer than the last
// one seen.
type IssueEventBus struct {
	mu   sync.Mutex
	subs map[string]map[chan struct{}]struct{}
}

// NewIssueEventBus constructs an empty bus. One instance is shared across
// the whole serve() lifecycle.
func NewIssueEventBus() *IssueEventBus {
	return &IssueEventBus{subs: make(map[string]map[chan struct{}]struct{})}
}

// Subscribe registers a wake-up channel for the given issue. The caller
// must invoke the returned unsubscribe function to release the slot
// (typically via defer on the SSE handler) — leaving subscriptions in the
// map after the client disconnects leaks goroutines.
func (b *IssueEventBus) Subscribe(issueID string) (<-chan struct{}, func()) {
	if b == nil || issueID == "" {
		ch := make(chan struct{}, 1)
		close(ch)
		return ch, func() {}
	}
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	set, ok := b.subs[issueID]
	if !ok {
		set = make(map[chan struct{}]struct{})
		b.subs[issueID] = set
	}
	set[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if set, ok := b.subs[issueID]; ok {
			delete(set, ch)
			if len(set) == 0 {
				delete(b.subs, issueID)
			}
		}
	}
}

// OnRunEvent satisfies store.RunEventNotifier. Sends a non-blocking
// wake-up to every subscriber currently registered for the issue. RunID
// is accepted for symmetry with the store interface but is unused — the
// bus is issue-scoped because SSE subscribers stream all events for an
// issue regardless of which run produced them.
func (b *IssueEventBus) OnRunEvent(issueID, runID string) {
	_ = runID
	if b == nil || issueID == "" {
		return
	}
	b.mu.Lock()
	set := b.subs[issueID]
	channels := make([]chan struct{}, 0, len(set))
	for ch := range set {
		channels = append(channels, ch)
	}
	b.mu.Unlock()
	for _, ch := range channels {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
