package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/ehrlich-b/ground/internal/db"
	embedpkg "github.com/ehrlich-b/ground/internal/embed"
	"github.com/ehrlich-b/ground/internal/model"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

type Server struct {
	store     *db.Store
	templates map[string]*template.Template
}

func NewServer(store *db.Store) *Server {
	s := &Server{store: store}
	s.loadTemplates()
	return s
}

func (s *Server) Mount(mux *http.ServeMux) {
	// Static files
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Pages
	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("GET /topics", s.handleTopics)
	mux.HandleFunc("GET /topic/{slug}", s.handleTopic)
	mux.HandleFunc("GET /agents", s.handleAgents)
	mux.HandleFunc("GET /agent/{id}", s.handleAgent)
	mux.HandleFunc("GET /claim/{id}", s.handleClaim)
	mux.HandleFunc("GET /about", s.handleAbout)
	mux.HandleFunc("GET /graph", s.handleGraph)
}

// --- Template Loading ---

var funcMap = template.FuncMap{
	"pct":           func(f float64) string { return fmt.Sprintf("%.0f", f*100) },
	"pctf":          func(f float64) string { return fmt.Sprintf("%.1f", f*100) },
	"printf":        fmt.Sprintf,
	"deref":         func(s *string) string { if s != nil { return *s }; return "" },
	"derefStr":      func(s *string) string { if s != nil { return *s }; return "" },
	"truncate":      truncate,
	"statusClass":   statusClass,
	"barClass":      barClass,
	"colorForValue": colorForValue,
	"mul":           func(a, b float64) float64 { return a * b },
	"divBy":         func(a, b float64) float64 { if b == 0 { return 0 }; return a / b },
	"add":           func(a, b int) int { return a + b },
	"timeAgo":       timeAgo,
}

func (s *Server) loadTemplates() {
	s.templates = make(map[string]*template.Template)
	pages := []string{"home", "topic", "topics", "agent", "agents", "claim", "about", "graph"}
	for _, page := range pages {
		t := template.Must(
			template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/base.html", "templates/"+page+".html"),
		)
		s.templates[page] = t
	}
}

func (s *Server) render(w http.ResponseWriter, page string, data any) {
	t, ok := s.templates[page]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		log.Printf("render %s: %v", page, err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// --- Handlers ---

type homeData struct {
	AgentCount int
	ClaimCount int
	TopicCount int
	EpochCount int
	Contested  []model.Claim
	Grounded   []model.Claim
	Agents     []model.Agent
	Topics     []model.Topic
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	agentCount, _ := s.store.CountAgents()
	claimCount, _ := s.store.CountClaims()
	topicCount, _ := s.store.CountTopics()
	epochCount, _ := s.store.CountEpochs()
	contested, _ := s.store.MostContestedClaims(10)
	grounded, _ := s.store.ListGroundedClaims(10)
	agents, _ := s.store.TopAgentsByWeight(10)
	topics, _ := s.store.ListTopics()

	// Limit topics for the home page grid
	if len(topics) > 12 {
		topics = topics[:12]
	}

	s.render(w, "home", homeData{
		AgentCount: agentCount,
		ClaimCount: claimCount,
		TopicCount: topicCount,
		EpochCount: epochCount,
		Contested:  contested,
		Grounded:   grounded,
		Agents:     agents,
		Topics:     topics,
	})
}

func (s *Server) handleTopics(w http.ResponseWriter, r *http.Request) {
	topics, _ := s.store.ListTopics()
	s.render(w, "topics", struct{ Topics []model.Topic }{topics})
}

func (s *Server) handleTopic(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	topic, err := s.store.GetTopicBySlug(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Find claims nearest to this topic by embedding proximity
	claims := s.claimsForTopic(topic)
	agents, _ := s.store.TopAgentsByWeight(5)

	s.render(w, "topic", struct {
		Topic     *model.Topic
		Claims    []model.Claim
		TopAgents []model.Agent
	}{topic, claims, agents})
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	agents, _ := s.store.ListAgents()
	s.render(w, "agents", struct{ Agents []model.Agent }{agents})
}

type assertionView struct {
	model.Assertion
	AgentName string
	ClaimProp string
	DepProp   string
}

type depView struct {
	model.Dependency
	DepProp string
}

func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent, err := s.store.GetAgent(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	assertions, _ := s.store.ListAssertionsByAgent(id, 100, 0)

	// Enrich assertions with claim propositions
	var views []assertionView
	for _, a := range assertions {
		v := assertionView{Assertion: a}
		if claim, err := s.store.GetClaim(a.ClaimID); err == nil {
			v.ClaimProp = claim.Proposition
		}
		views = append(views, v)
	}

	s.render(w, "agent", struct {
		Agent      *model.Agent
		Assertions []assertionView
	}{agent, views})
}

func (s *Server) handleClaim(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	claim, err := s.store.GetClaim(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	assertions, _ := s.store.ListAssertionsByClaim(id)
	deps, _ := s.store.ListDependenciesByClaim(id)
	dependents, _ := s.store.ListDependents(id)

	// Enrich assertions with agent names
	var assertionViews []assertionView
	for _, a := range assertions {
		v := assertionView{Assertion: a}
		if ag, err := s.store.GetAgent(a.AgentID); err == nil {
			v.AgentName = ag.Name
		}
		assertionViews = append(assertionViews, v)
	}

	// Enrich dependencies with claim propositions
	var depViews []depView
	for _, d := range deps {
		v := depView{Dependency: d}
		if c, err := s.store.GetClaim(d.DependsOnID); err == nil {
			v.DepProp = c.Proposition
		}
		depViews = append(depViews, v)
	}

	var dependentViews []depView
	for _, d := range dependents {
		v := depView{Dependency: d}
		if c, err := s.store.GetClaim(d.ClaimID); err == nil {
			v.DepProp = c.Proposition
		}
		dependentViews = append(dependentViews, v)
	}

	s.render(w, "claim", struct {
		Claim        *model.Claim
		Assertions   []assertionView
		Dependencies []depView
		Dependents   []depView
	}{claim, assertionViews, depViews, dependentViews})
}

func (s *Server) handleAbout(w http.ResponseWriter, r *http.Request) {
	s.render(w, "about", nil)
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	s.render(w, "graph", nil)
}

// --- Helpers ---

// claimsForTopic finds claims with highest embedding proximity to the given topic.
func (s *Server) claimsForTopic(topic *model.Topic) []model.Claim {
	if len(topic.Embedding) == 0 {
		// No embedding on topic, return recent claims as fallback
		claims, _ := s.store.ListClaims("", 50, 0)
		return claims
	}

	topicVec := embedpkg.UnmarshalVector(topic.Embedding)
	if len(topicVec) == 0 {
		claims, _ := s.store.ListClaims("", 50, 0)
		return claims
	}

	claimEmbeddings, err := s.store.ListClaimEmbeddings()
	if err != nil {
		return nil
	}

	type scored struct {
		id  string
		sim float64
	}
	var results []scored
	for _, ce := range claimEmbeddings {
		vec := embedpkg.UnmarshalVector(ce.Embedding)
		if len(vec) == 0 {
			continue
		}
		sim := embedpkg.CosineSimilarity(topicVec, vec)
		if sim > 0.3 { // minimum relevance threshold
			results = append(results, scored{id: ce.ID, sim: sim})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].sim > results[j].sim
	})

	if len(results) > 50 {
		results = results[:50]
	}

	var claims []model.Claim
	for _, r := range results {
		if c, err := s.store.GetClaim(r.id); err == nil {
			claims = append(claims, *c)
		}
	}
	return claims
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func statusClass(status string) string {
	switch status {
	case "grounded":
		return "grounded"
	case "adjudicated":
		return "adjudicated"
	case "contested":
		return "contested"
	case "active":
		return "active"
	case "emerging":
		return "emerging"
	case "refuted":
		return "refuted"
	default:
		return "active"
	}
}

func barClass(v float64) string {
	if v >= 0.7 {
		return "high"
	}
	if v >= 0.4 {
		return "medium"
	}
	return "low"
}

func colorForValue(v float64) string {
	if v >= 0.7 {
		return "var(--accent)"
	}
	if v >= 0.4 {
		return "var(--yellow)"
	}
	return "var(--red)"
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
