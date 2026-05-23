package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

func (s *Server) registerRunRoutes(api chi.Router) {
	api.Get("/api/issues/{id}/runs", s.listRuns)
	api.Get("/api/issues/{id}/events/stream", s.streamIssueRunEvents)
	api.Get("/api/runs/{id}/events", s.listRunEvents)
	api.Get("/api/runs/{id}/log", s.runLog)
	api.Get("/api/runs/{id}/log/stream", s.streamRunLog)
	api.Post("/api/runs/chain/{chain}/cancel", s.cancelChain)
	api.Post("/api/runs/chain/{chain}/retry", s.retryChain)
	api.Get("/api/workspaces/{workspace}/runs", s.listWorkspaceRuns)
	api.Get("/api/workspaces/{workspace}/runs/stream", s.streamWorkspaceRunEvents)
}

// streamWorkspaceRunEvents serves a coarse SSE wake-up stream for
// workspace-level pages (Run feed, chain dashboard). Each frame carries
// no payload — clients invalidate their query on receipt and re-fetch
// the workspace's run list. This trades the per-issue payload for a
// single stream that scales across the whole workspace.
func (s *Server) streamWorkspaceRunEvents(w http.ResponseWriter, r *http.Request) {
	ws, _, err := s.store.GetWorkspace(r.Context(), chi.URLParam(r, "workspace"))
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	var wake <-chan struct{}
	var unsubscribe func()
	if s.issueEventBus != nil {
		wake, unsubscribe = s.issueEventBus.SubscribeWorkspace(ws.ID)
		defer unsubscribe()
	}

	fmt.Fprint(w, ": stream open\n\n")
	flusher.Flush()

	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepAlive.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		case <-wake:
			fmt.Fprint(w, "event: wake\ndata: {}\n\n")
			flusher.Flush()
		}
	}
}

// streamIssueRunEvents serves a Server-Sent Events stream of run_event rows
// scoped to one issue. The handler polls the store once a second for new
// rows and writes each one as `event: run_event\ndata: <json>\n\n`. A
// keep-alive comment is sent every 15s so intermediaries do not close idle
// connections. Stream terminates when the client disconnects (ctx.Done()).
func (s *Server) streamIssueRunEvents(w http.ResponseWriter, r *http.Request) {
	issueID := chi.URLParam(r, "id")
	if _, err := s.store.GetIssue(r.Context(), issueID); err != nil {
		respond(w, nil, err, 0)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Optional ?since= primes the watermark for clients that already have
	// the historical rows from /api/runs/.../events.
	watermark := r.URL.Query().Get("since")

	// Emit a hello frame so the client knows the stream is alive even when
	// the issue has no run_events yet.
	fmt.Fprint(w, ": stream open\n\n")
	flusher.Flush()

	// Subscribe to the in-process notifier so AppendRunEvent commits wake
	// us up directly. When no bus is wired (e.g. test harness), the
	// fallback poll on the keep-alive timer still catches new rows.
	var wake <-chan struct{}
	var unsubscribe func()
	if s.issueEventBus != nil {
		wake, unsubscribe = s.issueEventBus.Subscribe(issueID)
		defer unsubscribe()
	}
	// Initial flush so subscribers that opened after a burst still get the
	// rows they missed before the first wake-up arrives.
	if err := s.flushPendingRunEvents(r.Context(), w, flusher, issueID, &watermark); err != nil {
		return
	}

	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepAlive.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
			// Tx-batched paths (cancel / complete / orphan recovery) fire
			// the notifier after their outer commit, but this idle tick
			// double-checks at low frequency so the handler converges even
			// when the bus was nil (test) or a notifier call was dropped
			// due to a buffer race.
			if err := s.flushPendingRunEvents(r.Context(), w, flusher, issueID, &watermark); err != nil {
				return
			}
		case <-wake:
			if err := s.flushPendingRunEvents(r.Context(), w, flusher, issueID, &watermark); err != nil {
				return
			}
		}
	}
}

// flushPendingRunEvents drains every run_event row newer than the
// watermark and writes them to the SSE stream. The watermark is updated
// in-place so the next call only picks up rows beyond what we just sent.
// A store error is surfaced as a single SSE `event: error` frame and the
// caller is expected to terminate the stream (EventSource reconnects with
// its remembered watermark).
func (s *Server) flushPendingRunEvents(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, issueID string, watermark *string) error {
	events, err := s.store.ListIssueRunEventsSince(ctx, issueID, *watermark, 100)
	if err != nil {
		payload, _ := json.Marshal(map[string]string{"error": err.Error()})
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", payload)
		flusher.Flush()
		return err
	}
	for _, e := range events {
		payload, err := json.Marshal(e)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: run_event\ndata: %s\n\n", payload)
		flusher.Flush()
		*watermark = e.CreatedAt
	}
	return nil
}

// listWorkspaceRuns serves the workspace-wide chain dashboard. The handler
// returns up to `limit` recent runs (default 500, capped at 5000) so the
// browser can group them client-side via summarizeChains.
func (s *Server) listWorkspaceRuns(w http.ResponseWriter, r *http.Request) {
	ws, _, err := s.store.GetWorkspace(r.Context(), chi.URLParam(r, "workspace"))
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	limit := 500
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconvAtoiPositive(raw); err == nil {
			limit = n
		}
	}
	runs, err := s.store.ListRecentRunsByWorkspace(r.Context(), ws.ID, limit)
	respond(w, map[string]any{"runs": runs}, err, http.StatusOK)
}

// strconvAtoiPositive parses a positive int; returns an error for any other
// input. Locally scoped to this file so we do not collide with usage
// elsewhere in the package.
func strconvAtoiPositive(raw string) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, http.ErrAbortHandler
	}
	return n, nil
}

// retryChain finds the most recent failed run in the chain and enqueues a
// new run on the same agent with the same chain_id / chain_depth. Returns
// 404 when the chain has no failed run, 409 when the chain still has
// queued/running runs (the operator must cancel first).
func (s *Server) retryChain(w http.ResponseWriter, r *http.Request) {
	chainID := chi.URLParam(r, "chain")
	run, err := s.store.RetryFailedRunInChain(r.Context(), chainID)
	respond(w, map[string]any{"run": run}, err, http.StatusCreated)
}

func (s *Server) cancelChain(w http.ResponseWriter, r *http.Request) {
	chainID := chi.URLParam(r, "chain")
	// Wake the worker pool first for any in-flight run on this chain so the
	// process group exits before the store marks the row cancelled. Failures
	// here are non-fatal — the store sweep below still cancels the row.
	if s.runCanceller != nil {
		runs, listErr := s.store.ListRunsByChain(r.Context(), chainID)
		if listErr == nil {
			for _, run := range runs {
				if run.Status == "running" {
					s.runCanceller.CancelRun(run.ID)
				}
			}
		}
	}
	cancelled, err := s.store.CancelRunsByChain(r.Context(), chainID, store.CancelReasonInput{
		Message:        "user cancelled the chain",
		TerminalReason: store.TerminalReasonUserCancelled,
		CancelReason:   store.CancelReasonUser,
	})
	respond(w, map[string]any{"chain_id": chainID, "cancelled": cancelled}, err, http.StatusOK)
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	xs, err := s.store.ListRuns(r.Context(), chi.URLParam(r, "id"))
	respond(w, map[string]any{"runs": xs}, err, http.StatusOK)
}

func (s *Server) listRunEvents(w http.ResponseWriter, r *http.Request) {
	xs, err := s.store.ListRunEvents(r.Context(), chi.URLParam(r, "id"))
	respond(w, map[string]any{"events": xs}, err, http.StatusOK)
}

func (s *Server) runLog(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetRunLogPath(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(p)+"\"")
	http.ServeFile(w, r, p)
}

// streamRunLog tails the run's stdout file. Each frame is `event: chunk`
// with the raw text encoded per the SSE multi-line `data:` convention so
// the client EventSource receives `e.data` joined by newlines. When the
// run reaches a terminal status, the handler sends a final flush, an
// `event: done` frame, and closes the stream — the client can switch to
// the existing /log download for the complete archive.
func (s *Server) streamRunLog(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": stream open\n\n")
	flusher.Flush()

	var offset int64
	if raw := r.URL.Query().Get("offset"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n >= 0 {
			offset = n
		}
	}
	logPath := ""
	if run.StdoutPath.Valid {
		logPath = run.StdoutPath.String
	}

	flushChunk := func() error {
		if logPath == "" {
			updated, err := s.store.GetRun(r.Context(), runID)
			if err == nil && updated.StdoutPath.Valid {
				logPath = updated.StdoutPath.String
			}
		}
		if logPath == "" {
			return nil
		}
		f, err := os.Open(logPath)
		if err != nil {
			return nil
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil {
			return nil
		}
		if stat.Size() <= offset {
			return nil
		}
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return err
		}
		buf := make([]byte, stat.Size()-offset)
		n, _ := io.ReadFull(f, buf)
		if n == 0 {
			return nil
		}
		// SSE multi-line data: each \n inside the chunk starts a new
		// `data:` line. Trailing newline is dropped so the client gets a
		// frame whose .data ends without a stray empty line.
		text := strings.TrimRight(string(buf[:n]), "\n")
		fmt.Fprint(w, "event: chunk\n")
		for _, line := range strings.Split(text, "\n") {
			fmt.Fprintf(w, "data: %s\n", line)
		}
		fmt.Fprint(w, "\n")
		flusher.Flush()
		offset += int64(n)
		return nil
	}
	terminalNow := func() bool {
		upd, err := s.store.GetRun(r.Context(), runID)
		if err != nil {
			return true
		}
		switch upd.Status {
		case "done", "failed", "cancelled":
			return true
		}
		return false
	}
	finish := func() {
		_ = flushChunk() // best-effort final flush
		fmt.Fprintf(w, "event: done\ndata: {\"offset\":%d}\n\n", offset)
		flusher.Flush()
	}

	if err := flushChunk(); err != nil {
		return
	}
	if terminalNow() {
		finish()
		return
	}
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepAlive.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		case <-tick.C:
			if err := flushChunk(); err != nil {
				return
			}
			if terminalNow() {
				finish()
				return
			}
		}
	}
}
