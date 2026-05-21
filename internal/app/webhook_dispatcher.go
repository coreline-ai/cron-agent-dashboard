package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

// WebhookDispatcher polls webhook_delivery for pending rows and POSTs them
// to the subscriber URL. It runs as a single in-process goroutine (this is
// a local single-binary tool) so there is no per-row claim semaphore;
// concurrency is bounded by the polling loop itself.
type WebhookDispatcher struct {
	store         *store.Store
	httpClient    *http.Client
	pollInterval  time.Duration
	retryBackoff  time.Duration
	maxAttempts   int
	log           *slog.Logger
	cancel        context.CancelFunc
	done          chan struct{}
}

// WebhookDispatcherOption configures a WebhookDispatcher.
type WebhookDispatcherOption func(*WebhookDispatcher)

// WithWebhookPollInterval overrides the dispatcher poll interval.
func WithWebhookPollInterval(d time.Duration) WebhookDispatcherOption {
	return func(w *WebhookDispatcher) {
		if d > 0 {
			w.pollInterval = d
		}
	}
}

// WithWebhookRetryBackoff overrides the retry delay between attempts.
func WithWebhookRetryBackoff(d time.Duration) WebhookDispatcherOption {
	return func(w *WebhookDispatcher) {
		if d > 0 {
			w.retryBackoff = d
		}
	}
}

// WithWebhookMaxAttempts overrides the total attempts before a delivery is
// marked 'failed'. The default is 2 (initial + one retry).
func WithWebhookMaxAttempts(n int) WebhookDispatcherOption {
	return func(w *WebhookDispatcher) {
		if n > 0 {
			w.maxAttempts = n
		}
	}
}

// WithWebhookHTTPClient swaps the HTTP client (useful in tests so they can
// point a transport at httptest.NewServer without needing real DNS).
func WithWebhookHTTPClient(c *http.Client) WebhookDispatcherOption {
	return func(w *WebhookDispatcher) {
		if c != nil {
			w.httpClient = c
		}
	}
}

// NewWebhookDispatcher constructs a WebhookDispatcher with default polling
// every 30s, 10s per-request timeout, retry backoff 30s, and at most two
// attempts per delivery.
func NewWebhookDispatcher(st *store.Store, opts ...WebhookDispatcherOption) *WebhookDispatcher {
	d := &WebhookDispatcher{
		store:        st,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		pollInterval: 30 * time.Second,
		retryBackoff: 30 * time.Second,
		maxAttempts:  2,
		log:          slog.Default(),
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Start kicks off the polling loop. Calling Start twice without Stop is a
// no-op for subsequent invocations.
func (d *WebhookDispatcher) Start(ctx context.Context) {
	if d.cancel != nil {
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	d.cancel = cancel
	d.done = make(chan struct{})
	go d.loop(loopCtx)
}

// Stop signals the loop to exit and waits for it to drain. shutdownCtx may
// be used to cap the wait.
func (d *WebhookDispatcher) Stop(shutdownCtx context.Context) error {
	if d.cancel == nil {
		return nil
	}
	d.cancel()
	d.cancel = nil
	if d.done != nil {
		select {
		case <-d.done:
		case <-shutdownCtx.Done():
			return shutdownCtx.Err()
		}
		d.done = nil
	}
	return nil
}

// TickOnce processes at most one pending delivery synchronously. Exposed for
// tests so they don't have to wait for the polling tick.
func (d *WebhookDispatcher) TickOnce(ctx context.Context) error {
	return d.tick(ctx)
}

func (d *WebhookDispatcher) loop(ctx context.Context) {
	defer close(d.done)
	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()
	if err := d.tick(ctx); err != nil {
		d.log.Warn("webhook tick error", "error", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := d.tick(ctx); err != nil {
				d.log.Warn("webhook tick error", "error", err)
			}
		}
	}
}

func (d *WebhookDispatcher) tick(ctx context.Context) error {
	delivery, hook, ok, err := d.store.NextPendingWebhookDelivery(ctx, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	d.attempt(ctx, delivery, hook)
	return nil
}

func (d *WebhookDispatcher) attempt(ctx context.Context, delivery store.WebhookDelivery, hook store.Webhook) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, bytes.NewReader([]byte(delivery.PayloadJSON)))
	if err != nil {
		d.recordFailure(ctx, delivery.ID, 0, "", "build request: "+err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Cron-Agent-Event", delivery.EventType)
	req.Header.Set("X-Cron-Agent-Delivery", delivery.ID)
	if hook.Secret != "" {
		mac := hmac.New(sha256.New, []byte(hook.Secret))
		mac.Write([]byte(delivery.PayloadJSON))
		req.Header.Set("X-Cron-Agent-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		d.recordFailure(ctx, delivery.ID, 0, "", err.Error())
		return
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	body := strings.TrimSpace(string(bodyBytes))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := d.store.MarkWebhookDeliveryDelivered(ctx, delivery.ID, resp.StatusCode, body); err != nil && !errors.Is(err, store.ErrState) {
			d.log.Warn("mark webhook delivered failed", "delivery_id", delivery.ID, "error", err)
		}
		return
	}
	d.recordFailure(ctx, delivery.ID, resp.StatusCode, body, "non-2xx status")
}

func (d *WebhookDispatcher) recordFailure(ctx context.Context, deliveryID string, statusCode int, body, errMsg string) {
	retryAt := time.Now().UTC().Add(d.retryBackoff).Format(time.RFC3339Nano)
	if err := d.store.MarkWebhookDeliveryFailure(ctx, deliveryID, statusCode, body, errMsg, retryAt, d.maxAttempts); err != nil {
		d.log.Warn("mark webhook failure failed", "delivery_id", deliveryID, "error", err)
	}
}
