package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/ehrlich-b/ground/internal/db"
	"github.com/ehrlich-b/ground/internal/embed"
	"github.com/ehrlich-b/ground/internal/sources"
	"golang.org/x/time/rate"
)

type Server struct {
	store     *db.Store
	embedder  embed.Embedder
	jwtSecret []byte
	mux       *http.ServeMux
	limiters  *rateLimiters
	ingester  *sources.Ingester
}

func NewServer(store *db.Store, embedder embed.Embedder, jwtSecret []byte) *Server {
	blobs, err := sources.NewFileBlobStore()
	if err != nil {
		log.Fatalf("init blob store: %v", err)
	}
	s := &Server{
		store:     store,
		embedder:  embedder,
		jwtSecret: jwtSecret,
		mux:       http.NewServeMux(),
		limiters:  newRateLimiters(),
		ingester: &sources.Ingester{
			Store:   store,
			Fetcher: sources.NewHTTPFetcher(),
			Blobs:   blobs,
		},
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }
func (s *Server) Mux() *http.ServeMux   { return s.mux }

func (s *Server) routes() {
	mux := s.mux

	// Agents
	mux.Handle("POST /api/agents", http.HandlerFunc(s.handleRegisterAgent))
	mux.Handle("POST /api/agents/token", s.authMiddleware(http.HandlerFunc(s.handleRotateToken)))
	mux.Handle("GET /api/agents/{id}", http.HandlerFunc(s.handleGetAgent))
	mux.Handle("GET /api/agents/leaderboard", http.HandlerFunc(s.handleAgentLeaderboard))

	// Topics
	mux.Handle("GET /api/topics", http.HandlerFunc(s.handleListTopics))
	mux.Handle("GET /api/topics/{slug}", http.HandlerFunc(s.handleGetTopic))
	mux.Handle("GET /api/topics/{slug}/claims", http.HandlerFunc(s.handleListClaimsByTopic))
	mux.Handle("POST /api/topics", s.authMiddleware(http.HandlerFunc(s.requireAdmin(s.handleCreateTopic))))

	// Sources
	mux.Handle("GET /api/sources", http.HandlerFunc(s.handleListSources))
	mux.Handle("GET /api/sources/{id}", http.HandlerFunc(s.handleGetSource))
	mux.Handle("GET /api/sources/{id}/body", http.HandlerFunc(s.handleGetSourceBody))
	mux.Handle("POST /api/sources/candidates", s.authMiddleware(http.HandlerFunc(s.handleCandidateSources)))

	// Claims
	mux.Handle("GET /api/claims", http.HandlerFunc(s.handleListClaims))
	mux.Handle("GET /api/claims/{id}", http.HandlerFunc(s.handleGetClaim))
	mux.Handle("GET /api/claims/{id}/gradient", http.HandlerFunc(s.handleClaimGradient))
	mux.Handle("GET /api/claims/{id}/dependencies", http.HandlerFunc(s.handleGetClaimDependencies))
	mux.Handle("POST /api/claims", s.authMiddleware(http.HandlerFunc(s.handleCreateClaim)))

	// Citations
	mux.Handle("POST /api/citations", s.authMiddleware(http.HandlerFunc(s.handleCreateCitation)))
	mux.Handle("GET /api/citations/{id}", http.HandlerFunc(s.handleGetCitation))
	mux.Handle("GET /api/citations/{id}/audits", http.HandlerFunc(s.handleGetCitationAudits))

	// Audits
	mux.Handle("POST /api/audits", s.authMiddleware(http.HandlerFunc(s.handleCreateAudit)))
	mux.Handle("GET /api/audits/queue", s.authMiddleware(http.HandlerFunc(s.handleAuditQueue)))

	// Dependencies
	mux.Handle("POST /api/dependencies", s.authMiddleware(http.HandlerFunc(s.handleCreateDependency)))

	// Lenses
	mux.Handle("POST /api/lenses", s.authMiddleware(http.HandlerFunc(s.handleCreateLens)))
	mux.Handle("GET /api/lenses", http.HandlerFunc(s.handleListLenses))
	mux.Handle("GET /api/lenses/{slug}", http.HandlerFunc(s.handleGetLens))
	mux.Handle("PUT /api/lenses/{slug}/overrides", s.authMiddleware(http.HandlerFunc(s.handleSetLensOverrides)))
	mux.Handle("POST /api/lenses/{slug}/fork", s.authMiddleware(http.HandlerFunc(s.handleForkLens)))

	// Discovery
	mux.Handle("GET /api/leaderboard", http.HandlerFunc(s.handleSourceLeaderboard))
	mux.Handle("GET /api/contested", http.HandlerFunc(s.handleContested))
	mux.Handle("GET /api/frontier", http.HandlerFunc(s.handleFrontier))
	mux.Handle("GET /api/epochs", http.HandlerFunc(s.handleListEpochs))
	mux.Handle("GET /api/epochs/latest", http.HandlerFunc(s.handleLatestEpoch))
	mux.Handle("GET /api/graph", http.HandlerFunc(s.handleGraph))

	// Admin
	mux.Handle("POST /api/admin/adjudicate", s.authMiddleware(http.HandlerFunc(s.requireAdmin(s.handleAdjudicate))))
}

// --- Context keys ---

type contextKey string

const (
	ctxAgentID contextKey = "agentID"
	ctxRole    contextKey = "role"
)

func withAgentID(ctx context.Context, id string) context.Context { return context.WithValue(ctx, ctxAgentID, id) }
func getAgentID(ctx context.Context) string {
	v, _ := ctx.Value(ctxAgentID).(string)
	return v
}

func withRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, ctxRole, role)
}
func getRole(ctx context.Context) string {
	v, _ := ctx.Value(ctxRole).(string)
	return v
}

// --- Response helpers ---

type apiResponse struct {
	Data any `json:"data,omitempty"`
	Meta any `json:"meta,omitempty"`
}

type apiError struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write json: %v", err)
	}
}

func writeData(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, apiResponse{Data: data})
}

func writeError(w http.ResponseWriter, status int, code, message string, details any) {
	writeJSON(w, status, apiError{Error: errorBody{Code: code, Message: message, Details: details}})
}

func parseIntParam(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return defaultVal
	}
	return v
}

// --- Rate Limiting ---

type rateLimiters struct {
	mu       sync.Mutex
	burst    map[string]*rate.Limiter
	daily    map[string]map[string]int
	dailyDay string
}

func newRateLimiters() *rateLimiters {
	return &rateLimiters{
		burst: make(map[string]*rate.Limiter),
		daily: make(map[string]map[string]int),
	}
}

func (rl *rateLimiters) checkBurst(agentID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	l, ok := rl.burst[agentID]
	if !ok {
		l = rate.NewLimiter(10, 10)
		rl.burst[agentID] = l
	}
	return l.Allow()
}

func (rl *rateLimiters) checkDaily(agentID, action string, limit int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	today := time.Now().UTC().Format("2006-01-02")
	if rl.dailyDay != today {
		rl.daily = make(map[string]map[string]int)
		rl.dailyDay = today
	}

	agent, ok := rl.daily[agentID]
	if !ok {
		agent = make(map[string]int)
		rl.daily[agentID] = agent
	}

	if agent[action] >= limit {
		return false
	}
	agent[action]++
	return true
}
