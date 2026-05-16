package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/coreline-ai/corn-agent-dashboard/internal/config"
	"github.com/coreline-ai/corn-agent-dashboard/internal/store"
)

var Version = "0.1.0"

type Server struct {
	store            *store.Store
	cfg              config.Config
	startedAt        time.Time
	runCanceller     RunCanceller
	autopilotManager AutopilotManager
}

type RunCanceller interface {
	CancelRun(runID string) bool
}

// PendingRunCancelCleaner is implemented by worker.Pool so the API can remove
// a pre-claim cancel marker when a queued run was cancelled directly in store.
type PendingRunCancelCleaner interface {
	ForgetPendingCancel(runID string) bool
}

type AutopilotManager interface {
	Reload(ctx context.Context) error
	TriggerRuleResult(ctx context.Context, ruleID string) (store.AutopilotTriggerResult, error)
}

type Option func(*Server)

func WithRunCanceller(c RunCanceller) Option {
	return func(s *Server) {
		s.runCanceller = c
	}
}

func WithAutopilotReloader(r AutopilotManager) Option {
	return func(s *Server) {
		s.autopilotManager = r
	}
}

func New(st *store.Store, cfg config.Config, opts ...Option) http.Handler {
	s := &Server{store: st, cfg: cfg, startedAt: time.Now()}
	for _, opt := range opts {
		opt(s)
	}
	r := chi.NewRouter()
	r.Use(s.cors)
	r.Get("/healthz", s.healthz)
	r.Group(func(api chi.Router) {
		api.Use(s.auth)
		s.registerSystemRoutes(api)
		s.registerWorkspaceRoutes(api)
		s.registerAgentRoutes(api)
		s.registerIssueRoutes(api)
		s.registerCommentRoutes(api)
		s.registerRunRoutes(api)
		s.registerAutopilotRoutes(api)
	})
	r.HandleFunc("/*", s.static)
	return r
}

func (s *Server) cors(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range s.cfg.CORS {
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		sameOrigin := isSameOrigin(r, origin)
		if origin != "" && len(allowed) > 0 && !allowed[origin] && !sameOrigin {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "origin not allowed", nil)
			return
		}
		if origin != "" && (allowed[origin] || len(allowed) == 0 || sameOrigin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isSameOrigin(r *http.Request, origin string) bool {
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && strings.EqualFold(u.Host, r.Host)
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Token != "" {
			want := "Bearer " + s.cfg.Token
			if r.Header.Get("Authorization") != want {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid token", nil)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
