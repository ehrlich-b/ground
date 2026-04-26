// Package web is the server-rendered HTML UI for Ground v2.
//
// All page routes are minimal during the v2 rebuild; the rich UI work lives in Phase 9.
package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ehrlich-b/ground/internal/db"
	"github.com/ehrlich-b/ground/internal/lens"
	"github.com/ehrlich-b/ground/internal/model"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// BodyLoader returns the cached body bytes for a source. The web layer uses it
// only for the source-body viewer; nil is fine if you only want JSON pages.
type BodyLoader func(src *model.Source) ([]byte, error)

type Server struct {
	store     *db.Store
	loadBody  BodyLoader
	templates map[string]*template.Template
}

func NewServer(store *db.Store, loadBody BodyLoader) *Server {
	s := &Server{store: store, loadBody: loadBody}
	s.loadTemplates()
	return s
}

func (s *Server) Mount(mux *http.ServeMux) {
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("GET /topics", s.handleTopics)
	mux.HandleFunc("GET /topic/{slug}", s.handleTopic)
	mux.HandleFunc("GET /agents", s.handleAgents)
	mux.HandleFunc("GET /agent/{id}", s.handleAgent)
	mux.HandleFunc("GET /sources", s.handleSources)
	mux.HandleFunc("GET /source/{id}", s.handleSource)
	mux.HandleFunc("GET /source/{id}/body", s.handleSourceBody)
	mux.HandleFunc("GET /claim/{id}", s.handleClaim)
	mux.HandleFunc("GET /lenses", s.handleLenses)
	mux.HandleFunc("GET /lens/{slug}", s.handleLens)
	mux.HandleFunc("GET /about", s.handleAbout)
	mux.HandleFunc("GET /graph", s.handleGraph)
}

var funcMap = template.FuncMap{
	"pct":           func(f float64) string { return fmt.Sprintf("%.0f", f*100) },
	"pctf":          func(f float64) string { return fmt.Sprintf("%.1f", f*100) },
	"printf":        fmt.Sprintf,
	"deref":         func(s *string) string { if s != nil { return *s }; return "" },
	"truncate":      truncate,
	"statusClass":   statusClass,
	"barClass":      barClass,
	"colorForValue": colorForValue,
	"timeAgo":       timeAgo,
}

func (s *Server) loadTemplates() {
	s.templates = map[string]*template.Template{}
	pages := []string{"home", "topic", "topics", "agent", "agents", "claim", "about", "graph", "sources", "source", "lenses", "lens"}
	for _, page := range pages {
		t := template.New("").Funcs(funcMap)
		// Each page is allowed to be missing during the rebuild; fall back to a generic page.
		t, err := t.ParseFS(templateFS, "templates/base.html", "templates/"+page+".html")
		if err != nil {
			log.Printf("warning: template %s missing, using fallback: %v", page, err)
			t = template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/base.html", "templates/about.html"))
		}
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

type homeData struct {
	AgentCount  int
	ClaimCount  int
	TopicCount  int
	SourceCount int
	EpochCount  int
	Contested   []model.Claim
	Grounded    []model.Claim
	Agents      []model.Agent
	Topics      []model.Topic
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	agentCount, _ := s.store.CountAgents()
	claimCount, _ := s.store.CountClaims()
	topicCount, _ := s.store.CountTopics()
	srcCount, _ := s.store.CountSources()
	epochCount, _ := s.store.CountEpochs()
	contested, _ := s.store.MostContestedClaims(10)
	grounded, _ := s.store.ListGroundedClaims(10)
	agents, _ := s.store.TopAgentsByReliability(10)
	topics, _ := s.store.ListTopics()
	if len(topics) > 12 {
		topics = topics[:12]
	}
	s.render(w, "home", homeData{
		AgentCount:  agentCount,
		ClaimCount:  claimCount,
		TopicCount:  topicCount,
		SourceCount: srcCount,
		EpochCount:  epochCount,
		Contested:   contested,
		Grounded:    grounded,
		Agents:      agents,
		Topics:      topics,
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
	claims, _ := s.store.ListClaimsByTopic(slug, 50)
	s.render(w, "topic", struct {
		Topic  *model.Topic
		Claims []model.Claim
	}{topic, claims})
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	agents, _ := s.store.ListAgents()
	s.render(w, "agents", struct{ Agents []model.Agent }{agents})
}

type citationView struct {
	model.Citation
	SourceURL   string
	SourceTitle string
}

func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent, err := s.store.GetAgent(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	citations, _ := s.store.ListCitationsByExtractor(id, 50, 0)
	audits, _ := s.store.ListAuditsByAuditor(id, 50, 0)
	s.render(w, "agent", struct {
		Agent     *model.Agent
		Citations []model.Citation
		Audits    []model.Audit
	}{agent, citations, audits})
}

func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	srcs, _ := s.store.ListSources(100, 0)
	creds, _ := s.store.LatestSourceCredibility()
	s.render(w, "sources", struct {
		Sources     []model.Source
		Credibility map[string]float64
	}{srcs, creds})
}

func (s *Server) handleSourceBody(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	src, err := s.store.GetSource(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if s.loadBody == nil {
		http.Error(w, "body viewer not configured", http.StatusNotImplemented)
		return
	}
	body, err := s.loadBody(src)
	if err != nil {
		http.Error(w, "load body: "+err.Error(), http.StatusInternalServerError)
		return
	}
	citations, _ := s.store.ListCitationsBySource(id)
	highlighted := highlightQuotes(string(body), citations)
	s.render(w, "source_body", struct {
		Source     *model.Source
		Citations  []model.Citation
		Highlights template.HTML
	}{src, citations, highlighted})
}

func (s *Server) handleSource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	src, err := s.store.GetSource(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	tags, _ := s.store.ListSourceTags(id)
	anchor, _ := s.store.GetSourceAnchor(id)
	citations, _ := s.store.ListCitationsBySource(id)
	creds, _ := s.store.LatestSourceCredibility()
	s.render(w, "source", struct {
		Source      *model.Source
		Tags        []string
		Anchor      *model.SourceAnchor
		Citations   []model.Citation
		Credibility float64
	}{src, tags, anchor, citations, creds[id]})
}

func (s *Server) handleClaim(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	claim, err := s.store.GetClaim(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	citations, _ := s.store.ListCitationsByClaim(id)
	deps, _ := s.store.ListDependenciesByClaim(id)
	dependents, _ := s.store.ListDependents(id)

	// Lens-aware re-render if requested.
	var lensScore *lens.ClaimScore
	lensSlug := r.URL.Query().Get("lens")
	if lensSlug != "" {
		if l, _ := s.store.GetLensBySlug(lensSlug); l != nil {
			snap, err := lens.LoadSnapshot(s.store)
			if err == nil {
				spec, _ := lens.LoadLensSpec(s.store, l.ID)
				score := lens.RenderClaim(snap, spec, id)
				lensScore = &score
			}
		}
	}

	s.render(w, "claim", struct {
		Claim        *model.Claim
		Citations    []model.Citation
		Dependencies []model.Dependency
		Dependents   []model.Dependency
		LensSlug     string
		LensScore    *lens.ClaimScore
	}{claim, citations, deps, dependents, lensSlug, lensScore})
}

func (s *Server) handleLenses(w http.ResponseWriter, r *http.Request) {
	lenses, _ := s.store.ListLenses()
	s.render(w, "lenses", struct{ Lenses []model.Lens }{lenses})
}

func (s *Server) handleLens(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	l, err := s.store.GetLensBySlug(slug)
	if err != nil || l == nil {
		http.NotFound(w, r)
		return
	}
	overrides, _ := s.store.ListLensOverrides(l.ID)
	tagOverrides, _ := s.store.ListLensTagOverrides(l.ID)
	s.render(w, "lens", struct {
		Lens         *model.Lens
		Overrides    []model.LensOverride
		TagOverrides []model.LensTagOverride
	}{l, overrides, tagOverrides})
}

func (s *Server) handleAbout(w http.ResponseWriter, r *http.Request) {
	s.render(w, "about", nil)
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	s.render(w, "graph", nil)
}

// highlightQuotes wraps every citation's verbatim quote inside the source body
// in <mark> tags with a polarity-coded class. Overlaps are resolved
// left-to-right: the earliest-starting citation wins, and any later citation
// whose span overlaps it is dropped. This is good enough for the viewer; the
// underlying citations remain in the sidebar list.
func highlightQuotes(body string, citations []model.Citation) template.HTML {
	type span struct {
		start, end int
		polarity   string
		citID      string
	}
	var spans []span
	for _, c := range citations {
		if c.VerbatimQuote == "" {
			continue
		}
		idx := strings.Index(body, c.VerbatimQuote)
		if idx < 0 {
			continue
		}
		spans = append(spans, span{idx, idx + len(c.VerbatimQuote), c.Polarity, c.ID})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })

	var b strings.Builder
	cursor := 0
	for _, sp := range spans {
		if sp.start < cursor {
			continue
		}
		b.WriteString(template.HTMLEscapeString(body[cursor:sp.start]))
		fmt.Fprintf(&b, `<mark class="cite cite-%s" data-citation-id="%s">`,
			template.HTMLEscapeString(sp.polarity),
			template.HTMLEscapeString(sp.citID))
		b.WriteString(template.HTMLEscapeString(body[sp.start:sp.end]))
		b.WriteString(`</mark>`)
		cursor = sp.end
	}
	b.WriteString(template.HTMLEscapeString(body[cursor:]))
	return template.HTML(b.String())
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
