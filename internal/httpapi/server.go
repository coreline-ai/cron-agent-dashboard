package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/coreline-ai/cron-agent-dashboard/internal/config"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

var Version = "0.1.0"

const contentSecurityPolicy = "default-src 'self'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'"

type Server struct {
	store            *store.Store
	cfg              config.Config
	startedAt        time.Time
	runCanceller     RunCanceller
	autopilotManager AutopilotManager
	issueEventBus    IssueEventSubscriber
}

// IssueEventSubscriber lets the SSE handler park on a wake-up channel that
// fires when AppendRunEvent commits. The subscribe contract returns the
// wake-up channel and an unsubscribe callback the caller must defer.
// Production wiring uses internal/app.IssueEventBus; tests can swap a
// nil-safe fake.
type IssueEventSubscriber interface {
	Subscribe(issueID string) (<-chan struct{}, func())
	SubscribeWorkspace(workspaceID string) (<-chan struct{}, func())
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

// WithIssueEventBus wires the in-process notifier the SSE handler uses to
// stream run_event rows without polling. Optional — when absent the SSE
// handler falls back to its idle keep-alive cadence.
func WithIssueEventBus(bus IssueEventSubscriber) Option {
	return func(s *Server) {
		s.issueEventBus = bus
	}
}

func New(st *store.Store, cfg config.Config, opts ...Option) http.Handler {
	s := &Server{store: st, cfg: cfg, startedAt: time.Now()}
	for _, opt := range opts {
		opt(s)
	}
	r := chi.NewRouter()
	r.Use(securityHeaders)
	r.Use(s.cors)
	r.Get("/healthz", s.healthz)
	r.Group(func(api chi.Router) {
		api.Use(s.auth)
		s.registerSystemRoutes(api)
		s.registerWorkspaceRoutes(api)
		s.registerAgentRoutes(api)
		s.registerSkillRoutes(api)
		s.registerIssueRoutes(api)
		s.registerCommentRoutes(api)
		s.registerRunRoutes(api)
		s.registerAutopilotRoutes(api)
		s.registerWebhookRoutes(api)
		s.registerAttachmentRoutes(api)
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
		originAllowed := origin != "" && (allowed[origin] || sameOrigin)
		if origin != "" && !originAllowed {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "origin not allowed", nil)
			return
		}
		if originAllowed {
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

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("Content-Security-Policy-Report-Only", contentSecurityPolicy)
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
			if !constantTimeEqual(r.Header.Get("Authorization"), "Bearer "+s.cfg.Token) {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid token", nil)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func constantTimeEqual(got, want string) bool {
	gotHash := sha256.Sum256([]byte(got))
	wantHash := sha256.Sum256([]byte(want))
	hashesEqual := subtle.ConstantTimeCompare(gotHash[:], wantHash[:]) == 1
	lengthsEqual := subtle.ConstantTimeEq(int32(len(got)), int32(len(want))) == 1
	return hashesEqual && lengthsEqual
}
