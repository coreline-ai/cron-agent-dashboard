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
//
// The bus also tracks workspace-level subscribers. When OnRunEvent fires
// for an issue, the bus resolves the owning workspace via the lookup
// function set on construction and wakes every workspace subscriber for
// that workspace. The lookup result is cached so a busy issue does not
// hit the DB per run_event.
type IssueEventBus struct {
	mu               sync.Mutex
	issueSubs        map[string]map[chan struct{}]struct{}
	workspaceSubs    map[string]map[chan struct{}]struct{}
	workspaceLookup  func(issueID string) string
	workspaceByIssue map[string]string
}

// IssueEventBusOption configures the bus at construction time.
type IssueEventBusOption func(*IssueEventBus)

// WithWorkspaceResolver registers a lookup function the bus uses to map
// issueID → workspaceID. Without a resolver, workspace subscribers
// receive no wake-ups even when OnRunEvent fires.
func WithWorkspaceResolver(fn func(issueID string) string) IssueEventBusOption {
	return func(b *IssueEventBus) {
		b.workspaceLookup = fn
	}
}

// NewIssueEventBus constructs an empty bus. One instance is shared across
// the whole serve() lifecycle.
func NewIssueEventBus(opts ...IssueEventBusOption) *IssueEventBus {
	b := &IssueEventBus{
		issueSubs:        make(map[string]map[chan struct{}]struct{}),
		workspaceSubs:    make(map[string]map[chan struct{}]struct{}),
		workspaceByIssue: make(map[string]string),
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
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
	set, ok := b.issueSubs[issueID]
	if !ok {
		set = make(map[chan struct{}]struct{})
		b.issueSubs[issueID] = set
	}
	set[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if set, ok := b.issueSubs[issueID]; ok {
			delete(set, ch)
			if len(set) == 0 {
				delete(b.issueSubs, issueID)
			}
		}
	}
}

// SubscribeWorkspace registers a wake-up channel for the given workspace.
// The workspace SSE endpoint uses this so /w/:slug/runs and /w/:slug/chains
// can converge in real-time instead of polling every 8 seconds. The same
// buffered (size 1) coalescing semantics apply.
func (b *IssueEventBus) SubscribeWorkspace(workspaceID string) (<-chan struct{}, func()) {
	if b == nil || workspaceID == "" {
		ch := make(chan struct{}, 1)
		close(ch)
		return ch, func() {}
	}
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	set, ok := b.workspaceSubs[workspaceID]
	if !ok {
		set = make(map[chan struct{}]struct{})
		b.workspaceSubs[workspaceID] = set
	}
	set[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if set, ok := b.workspaceSubs[workspaceID]; ok {
			delete(set, ch)
			if len(set) == 0 {
				delete(b.workspaceSubs, workspaceID)
			}
		}
	}
}

// OnRunEvent satisfies store.RunEventNotifier. Sends a non-blocking
// wake-up to every issue subscriber and, if the workspace resolver is
// wired, every workspace subscriber for the owning workspace.
func (b *IssueEventBus) OnRunEvent(issueID, runID string) {
	_ = runID
	if b == nil || issueID == "" {
		return
	}
	b.mu.Lock()
	issueChannels := snapshot(b.issueSubs[issueID])
	workspaceID := b.workspaceByIssue[issueID]
	resolver := b.workspaceLookup
	b.mu.Unlock()

	if workspaceID == "" && resolver != nil {
		workspaceID = resolver(issueID)
		if workspaceID != "" {
			b.mu.Lock()
			b.workspaceByIssue[issueID] = workspaceID
			b.mu.Unlock()
		}
	}

	var workspaceChannels []chan struct{}
	if workspaceID != "" {
		b.mu.Lock()
		workspaceChannels = snapshot(b.workspaceSubs[workspaceID])
		b.mu.Unlock()
	}

	for _, ch := range issueChannels {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	for _, ch := range workspaceChannels {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// snapshot copies the channel set into a slice while the mutex is held so
// the send loop can run without blocking other subscribe/unsubscribe
// operations.
func snapshot(set map[chan struct{}]struct{}) []chan struct{} {
	out := make([]chan struct{}, 0, len(set))
	for ch := range set {
		out = append(out, ch)
	}
	return out
}
