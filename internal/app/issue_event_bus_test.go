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
