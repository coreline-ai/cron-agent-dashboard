package app

import (
	"sync"
	"testing"
	"time"
)

func TestIssueEventBusWakesSubscribers(t *testing.T) {
	bus := NewIssueEventBus()
	ch, unsubscribe := bus.Subscribe("issue-1")
	defer unsubscribe()

	bus.OnRunEvent("issue-1", "run-1")
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("subscriber did not wake within 1s")
	}
}

func TestIssueEventBusIgnoresUnrelatedIssues(t *testing.T) {
	bus := NewIssueEventBus()
	ch, unsubscribe := bus.Subscribe("issue-1")
	defer unsubscribe()

	bus.OnRunEvent("issue-2", "run-x")
	select {
	case <-ch:
		t.Fatalf("subscriber for issue-1 woken on issue-2 event")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestIssueEventBusCoalescesBursts(t *testing.T) {
	bus := NewIssueEventBus()
	ch, unsubscribe := bus.Subscribe("issue-1")
	defer unsubscribe()
	// Fire 10 events in quick succession; the buffered (size 1) channel
	// should collapse them into a single pending wake-up.
	for i := 0; i < 10; i++ {
		bus.OnRunEvent("issue-1", "run-A")
	}
	count := 0
	timeout := time.After(50 * time.Millisecond)
loop:
	for {
		select {
		case <-ch:
			count++
		case <-timeout:
			break loop
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 coalesced wake-up, got %d", count)
	}
}

func TestIssueEventBusUnsubscribeRemovesEntry(t *testing.T) {
	bus := NewIssueEventBus()
	ch, unsubscribe := bus.Subscribe("issue-1")
	unsubscribe()
	bus.OnRunEvent("issue-1", "run-1")
	select {
	case <-ch:
		t.Fatalf("unsubscribed channel still received a wake-up")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestIssueEventBusWorkspaceSubscribersWakeOnAnyIssue(t *testing.T) {
	resolver := func(issueID string) string {
		if issueID == "issue-1" || issueID == "issue-2" {
			return "ws-A"
		}
		return ""
	}
	bus := NewIssueEventBus(WithWorkspaceResolver(resolver))
	workspaceCh, unsubscribeWS := bus.SubscribeWorkspace("ws-A")
	defer unsubscribeWS()
	issueCh, unsubscribeIss := bus.Subscribe("issue-1")
	defer unsubscribeIss()

	bus.OnRunEvent("issue-2", "run-x")
	select {
	case <-workspaceCh:
	case <-time.After(time.Second):
		t.Fatalf("workspace subscriber did not wake on sibling issue event")
	}
	select {
	case <-issueCh:
		t.Fatalf("issue-1 subscriber woken on issue-2 event")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestIssueEventBusWorkspaceResolverCaches(t *testing.T) {
	var lookupCalls int
	resolver := func(issueID string) string {
		lookupCalls++
		return "ws-CACHE"
	}
	bus := NewIssueEventBus(WithWorkspaceResolver(resolver))
	_, unsub := bus.SubscribeWorkspace("ws-CACHE")
	defer unsub()
	for i := 0; i < 5; i++ {
		bus.OnRunEvent("issue-cached", "run-x")
	}
	if lookupCalls != 1 {
		t.Fatalf("expected resolver to be called exactly once after cache warm, got %d", lookupCalls)
	}
}

func TestIssueEventBusWorkspaceSubscribersWithoutResolverNoOp(t *testing.T) {
	bus := NewIssueEventBus()
	ch, unsub := bus.SubscribeWorkspace("ws-orphan")
	defer unsub()
	bus.OnRunEvent("issue-detached", "run-x")
	select {
	case <-ch:
		t.Fatalf("workspace channel should not wake without a resolver")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestIssueEventBusConcurrentSubscribers(t *testing.T) {
	bus := NewIssueEventBus()
	var wg sync.WaitGroup
	const N = 10
	channels := make([]<-chan struct{}, N)
	cancels := make([]func(), N)
	for i := 0; i < N; i++ {
		channels[i], cancels[i] = bus.Subscribe("issue-X")
	}
	defer func() {
		for _, c := range cancels {
			c()
		}
	}()
	for i := 0; i < N; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-channels[i]:
			case <-time.After(time.Second):
				t.Errorf("subscriber %d did not wake", i)
			}
		}()
	}
	bus.OnRunEvent("issue-X", "run-Z")
	wg.Wait()
}
