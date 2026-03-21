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
	"golang.org/x/time/rate"
)

type Server struct {
	store     *db.Store
	embedder  embed.Embedder
	jwtSecret []byte
	mux       *http.ServeMux
	limiters  *rateLimiters
}

func NewServer(store *db.Store, embedder embed.Embedder, jwtSecret []byte) *Server {
	s := &Server{
		store:     store,
		embedder:  embedder,
		jwtSecret: jwtSecret,
		mux:       http.NewServeMux(),
		limiters:  newRateLimiters(),
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) Mux() *http.ServeMux {
	return s.mux
}

func (s *Server) routes() {
	// Authenticated routes
	auth := s.mux

	// Agents
	auth.Handle("POST /api/agents", http.HandlerFunc(s.handleRegisterAgent))
	auth.Handle("POST /api/agents/token", s.authMiddleware(http.HandlerFunc(s.handleRotateToken)))
	auth.Handle("GET /api/agents/{id}", http.HandlerFunc(s.handleGetAgent))
	auth.Handle("GET /api/agents/{id}/assertions", http.HandlerFunc(s.handleGetAgentAssertions))
	auth.Handle("GET /api/agents/{id}/reviews", http.HandlerFunc(s.handleGetAgentReviews))

	// Topics
	auth.Handle("GET /api/topics", http.HandlerFunc(s.handleListTopics))
	auth.Handle("GET /api/topics/{slug}", http.HandlerFunc(s.handleGetTopic))
	auth.Handle("POST /api/topics", s.authMiddleware(http.HandlerFunc(s.requireAdmin(s.handleCreateTopic))))

	// Claims (topic-filtered must come before {slug} catch-all — it's under topics)
	auth.Handle("GET /api/topics/{slug}/claims", http.HandlerFunc(s.handleListClaimsByTopic))

	// Claims
	auth.Handle("GET /api/claims", http.HandlerFunc(s.handleListClaims))
	auth.Handle("GET /api/claims/{id}", http.HandlerFunc(s.handleGetClaim))
	auth.Handle("POST /api/claims", s.authMiddleware(http.HandlerFunc(s.handleCreateClaim)))

	// Assertions
	auth.Handle("GET /api/assertions/{id}", http.HandlerFunc(s.handleGetAssertion))
	auth.Handle("POST /api/assertions", s.authMiddleware(http.HandlerFunc(s.handleCreateAssertion)))

	// Reviews
	auth.Handle("GET /api/assertions/{id}/reviews", http.HandlerFunc(s.handleGetAssertionReviews))
	auth.Handle("POST /api/reviews", s.authMiddleware(http.HandlerFunc(s.handleCreateReview)))

	// Dependencies
	auth.Handle("GET /api/claims/{id}/dependencies", http.HandlerFunc(s.handleGetClaimDependencies))
	auth.Handle("POST /api/dependencies", s.authMiddleware(http.HandlerFunc(s.handleCreateDependency)))

	// Discovery (unauthenticated)
	auth.Handle("GET /api/leaderboard", http.HandlerFunc(s.handleLeaderboard))
	auth.Handle("GET /api/contested", http.HandlerFunc(s.handleContested))
	auth.Handle("GET /api/frontier", http.HandlerFunc(s.handleFrontier))
	auth.Handle("GET /api/epochs", http.HandlerFunc(s.handleListEpochs))
	auth.Handle("GET /api/epochs/latest", http.HandlerFunc(s.handleLatestEpoch))
	auth.Handle("GET /api/graph", http.HandlerFunc(s.handleGraph))

	// Admin
	auth.Handle("POST /api/admin/adjudicate", s.authMiddleware(http.HandlerFunc(s.requireAdmin(s.handleAdjudicate))))
	auth.Handle("POST /api/admin/cascade", s.authMiddleware(http.HandlerFunc(s.requireAdmin(s.handleCascade))))
}

// --- Context keys ---

type contextKey string

const (
	ctxAgentID contextKey = "agentID"
	ctxRole    contextKey = "role"
)

func withAgentID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxAgentID, id)
}

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

func writeDataMeta(w http.ResponseWriter, status int, data, meta any) {
	writeJSON(w, status, apiResponse{Data: data, Meta: meta})
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
	burst    map[string]*rate.Limiter    // per-agent burst limiter (10 rps)
	daily    map[string]map[string]int   // agent -> action -> count
	dailyDay string                      // YYYY-MM-DD of current daily window
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
