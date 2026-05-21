package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

type capturedRequest struct {
	method    string
	body      string
	signature string
	event     string
	delivery  string
}

func seedWebhookDelivery(t *testing.T, st *store.Store, hookURL, secret, payload string) (store.Webhook, store.WebhookDelivery) {
	t.Helper()
	ctx := context.Background()
	slug := fmt.Sprintf("wh-%d", time.Now().UnixNano())
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "Hook",
		Slug:             slug,
		IdentifierPrefix: "WH",
		MainAgent:        store.CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	hook, err := st.CreateWebhook(ctx, ws.ID, store.UpsertWebhookInput{URL: hookURL, Secret: secret})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := st.DB().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	n, err := st.EnqueueWebhookDeliveries(ctx, tx, ws.ID, "run.completed", []byte(payload))
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 queued delivery, got %d", n)
	}
	deliveries, err := st.ListWebhookDeliveries(ctx, hook.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery row, got %d", len(deliveries))
	}
	return hook, deliveries[0]
}

func TestWebhookDispatcherDelivers2xxResponses(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	var captured capturedRequest
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		captured = capturedRequest{
			method:    r.Method,
			body:      string(b),
			signature: r.Header.Get("X-Cron-Agent-Signature"),
			event:     r.Header.Get("X-Cron-Agent-Event"),
			delivery:  r.Header.Get("X-Cron-Agent-Delivery"),
		}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("thanks"))
	}))
	defer srv.Close()

	payload := `{"hello":"world"}`
	_, delivery := seedWebhookDelivery(t, st, srv.URL, "hush", payload)

	disp := NewWebhookDispatcher(st)
	if err := disp.TickOnce(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}

	mu.Lock()
	cap := captured
	mu.Unlock()
	if cap.method != http.MethodPost {
		t.Fatalf("method=%q want POST", cap.method)
	}
	if cap.body != payload {
		t.Fatalf("body=%q want %q", cap.body, payload)
	}
	if cap.event != "run.completed" || cap.delivery != delivery.ID {
		t.Fatalf("headers mismatch: %#v", cap)
	}

	// Verify HMAC signature explicitly so the secret format does not regress.
	mac := hmac.New(sha256.New, []byte("hush"))
	mac.Write([]byte(payload))
	wantSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if cap.signature != wantSig {
		t.Fatalf("signature=%q want %q", cap.signature, wantSig)
	}

	// Delivery row should be marked delivered with status_code=200.
	deliveries, _ := st.ListWebhookDeliveries(ctx, delivery.WebhookID, 10)
	if len(deliveries) != 1 || deliveries[0].Status != "delivered" || deliveries[0].StatusCode != 200 {
		t.Fatalf("post-tick delivery state: %#v", deliveries)
	}
}

func TestWebhookDispatcherRetriesAndEventuallyFails(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, delivery := seedWebhookDelivery(t, st, srv.URL, "", `{"x":1}`)

	disp := NewWebhookDispatcher(st,
		WithWebhookMaxAttempts(2),
		// retryBackoff small so the second tick is immediately eligible.
		WithWebhookRetryBackoff(time.Millisecond),
	)
	if err := disp.TickOnce(ctx); err != nil {
		t.Fatalf("first tick: %v", err)
	}
	// First attempt should not have terminated as failed yet.
	deliveries, _ := st.ListWebhookDeliveries(ctx, delivery.WebhookID, 10)
	if len(deliveries) != 1 || deliveries[0].Status != "pending" || deliveries[0].Attempt != 1 {
		t.Fatalf("post-first-tick state: %#v", deliveries)
	}

	// Sleep just long enough for the new next_attempt_at (now + 1ms) to be due.
	time.Sleep(20 * time.Millisecond)
	if err := disp.TickOnce(ctx); err != nil {
		t.Fatalf("second tick: %v", err)
	}
	deliveries, _ = st.ListWebhookDeliveries(ctx, delivery.WebhookID, 10)
	if len(deliveries) != 1 || deliveries[0].Status != "failed" || deliveries[0].Attempt != 2 {
		t.Fatalf("post-second-tick state: %#v", deliveries)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 HTTP calls, got %d", got)
	}
}

func TestWebhookDispatcherTickWithEmptyQueueNoops(t *testing.T) {
	st := newTestStore(t)
	disp := NewWebhookDispatcher(st)
	if err := disp.TickOnce(context.Background()); err != nil {
		t.Fatalf("empty tick returned err: %v", err)
	}
}
