package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ehrlich-b/ground/internal/db"
	"github.com/ehrlich-b/ground/internal/embed"
	"github.com/ehrlich-b/ground/internal/lens"
	"github.com/ehrlich-b/ground/internal/model"
)

// --- Agents ---

func (s *Server) handleRegisterAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string  `json:"name"`
		Role     string  `json:"role"`
		Metadata *string `json:"metadata,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "name is required", nil)
		return
	}
	role := req.Role
	if role == "" {
		role = "both"
	}
	if !validRole(role) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid role", nil)
		return
	}

	agentID := db.GenerateID()
	agent := &model.Agent{
		ID:           agentID,
		Name:         req.Name,
		Role:         role,
		Reliability:  0.5,
		Productivity: 0.0,
		Metadata:     req.Metadata,
	}
	if err := s.store.CreateAgent(agent); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to create agent", nil)
		return
	}

	tokenStr, err := IssueToken(s.store, s.jwtSecret, agentID, "agent")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to create token", nil)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"agent": agent, "token": tokenStr})
}

func (s *Server) handleRotateToken(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentID(r.Context())
	if err := s.store.RevokeAgentTokens(agentID); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to revoke old tokens", nil)
		return
	}
	tokenStr, err := IssueToken(s.store, s.jwtSecret, agentID, "agent")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to create token", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"token": tokenStr})
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent, err := s.store.GetAgent(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "agent not found", nil)
		return
	}
	citations, _ := s.store.ListCitationsByExtractor(id, 50, 0)
	audits, _ := s.store.ListAuditsByAuditor(id, 50, 0)
	writeData(w, http.StatusOK, map[string]any{
		"agent":           agent,
		"recent_citations": citations,
		"recent_audits":    audits,
	})
}

func (s *Server) handleAgentLeaderboard(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 25)
	agents, err := s.store.TopAgentsByReliability(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "leaderboard failed", nil)
		return
	}
	writeData(w, http.StatusOK, agents)
}

func validRole(r string) bool {
	switch r {
	case "extractor", "auditor", "both", "observer":
		return true
	}
	return false
}

// --- Topics ---

func (s *Server) handleListTopics(w http.ResponseWriter, r *http.Request) {
	topics, err := s.store.ListTopics()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to list topics", nil)
		return
	}
	writeData(w, http.StatusOK, topics)
}

func (s *Server) handleGetTopic(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	topic, err := s.store.GetTopicBySlug(slug)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "topic not found", nil)
		return
	}
	writeData(w, http.StatusOK, topic)
}

func (s *Server) handleCreateTopic(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "title is required", nil)
		return
	}
	vec, err := s.embedder.Embed(req.Title + ": " + req.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EMBED_FAILED", "failed to generate embedding", nil)
		return
	}
	slug := slugify(req.Title)
	desc := req.Description
	topic := &model.Topic{
		ID:          db.GenerateID(),
		Title:       req.Title,
		Slug:        slug,
		Description: &desc,
		Embedding:   embed.MarshalVector(vec),
	}
	if err := s.store.CreateTopic(topic); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to create topic", nil)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"topic": topic})
}

func (s *Server) handleListClaimsByTopic(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	limit := parseIntParam(r, "limit", 100)
	claims, err := s.store.ListClaimsByTopic(slug, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to list claims by topic", nil)
		return
	}
	writeData(w, http.StatusOK, claims)
}

// --- Sources ---

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 50)
	offset := parseIntParam(r, "offset", 0)
	srcs, err := s.store.ListSources(limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to list sources", nil)
		return
	}
	writeData(w, http.StatusOK, srcs)
}

func (s *Server) handleGetSource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	src, err := s.store.GetSource(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "source not found", nil)
		return
	}
	tags, _ := s.store.ListSourceTags(id)
	anchor, _ := s.store.GetSourceAnchor(id)
	citations, _ := s.store.ListCitationsBySource(id)
	creds, _ := s.store.LatestSourceCredibility()
	writeData(w, http.StatusOK, map[string]any{
		"source":      src,
		"tags":        tags,
		"anchor":      anchor,
		"credibility": creds[id],
		"citations":   citations,
	})
}

func (s *Server) handleGetSourceBody(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	src, err := s.store.GetSource(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "source not found", nil)
		return
	}
	body, err := s.ingester.LoadBody(src)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to load body", nil)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(body)
}

func (s *Server) handleCandidateSources(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentID(r.Context())
	if !s.limiters.checkBurst(agentID) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests", nil)
		return
	}
	if !s.limiters.checkDaily(agentID, "source_candidates", 100) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "daily candidate limit reached", nil)
		return
	}
	var req struct {
		Candidates []struct {
			URL       string `json:"url"`
			Reasoning string `json:"reasoning"`
		} `json:"candidates"`
		TopicSlug string `json:"topic_slug,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	var ingested []map[string]any
	for _, c := range req.Candidates {
		if c.URL == "" {
			continue
		}
		res, err := s.ingester.Ingest(c.URL)
		if err != nil {
			ingested = append(ingested, map[string]any{"url": c.URL, "error": err.Error()})
			continue
		}
		ingested = append(ingested, map[string]any{
			"url":       c.URL,
			"source_id": res.Source.ID,
			"reused":    res.Reused,
		})
	}
	writeData(w, http.StatusCreated, map[string]any{"results": ingested})
}

// --- Claims ---

func (s *Server) handleListClaims(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit := parseIntParam(r, "limit", 50)
	offset := parseIntParam(r, "offset", 0)
	claims, err := s.store.ListClaims(status, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to list claims", nil)
		return
	}
	writeData(w, http.StatusOK, claims)
}

func (s *Server) handleGetClaim(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	claim, err := s.store.GetClaim(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "claim not found", nil)
		return
	}
	citations, _ := s.store.ListCitationsByClaim(id)
	deps, _ := s.store.ListDependenciesByClaim(id)
	dependents, _ := s.store.ListDependents(id)

	resp := map[string]any{
		"claim":        claim,
		"citations":    citations,
		"dependencies": deps,
		"dependents":   dependents,
	}

	// Lens-aware re-render if requested.
	lensSlug := r.URL.Query().Get("lens")
	if lensSlug != "" {
		if lensRow, _ := s.store.GetLensBySlug(lensSlug); lensRow != nil {
			snap, err := lens.LoadSnapshot(s.store)
			if err == nil {
				spec, err := lens.LoadLensSpec(s.store, lensRow.ID)
				if err == nil {
					score := lens.RenderClaim(snap, spec, id)
					resp["lens_score"] = score
				}
			}
		}
	}
	writeData(w, http.StatusOK, resp)
}

func (s *Server) handleClaimGradient(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	limit := parseIntParam(r, "limit", 5)
	snap, err := lens.LoadSnapshot(s.store)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "snapshot failed", nil)
		return
	}
	spec := &lens.LensSpec{}
	if lensSlug := r.URL.Query().Get("lens"); lensSlug != "" {
		if lensRow, _ := s.store.GetLensBySlug(lensSlug); lensRow != nil {
			spec, _ = lens.LoadLensSpec(s.store, lensRow.ID)
		}
	}
	g := lens.Gradient(snap, spec, id, limit)
	writeData(w, http.StatusOK, g)
}

func (s *Server) handleCreateClaim(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentID(r.Context())
	if !s.limiters.checkBurst(agentID) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests", nil)
		return
	}
	if !s.limiters.checkDaily(agentID, "claims", 50) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "daily claim limit reached", nil)
		return
	}

	var req struct {
		Proposition string                   `json:"proposition"`
		Citations   []citationInput          `json:"citations"`
		DependsOn   []dependencyInput        `json:"depends_on,omitempty"`
		TopicSlug   string                   `json:"topic_slug,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	if req.Proposition == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "proposition is required", nil)
		return
	}
	if len(req.Citations) == 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "at least one citation is required", nil)
		return
	}

	vec, err := s.embedder.Embed(req.Proposition)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EMBED_FAILED", "failed to embed", nil)
		return
	}
	// Duplicate check
	embeddings, err := s.store.ListClaimEmbeddings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "duplicate check failed", nil)
		return
	}
	var checks []embed.ClaimWithEmbedding
	for _, c := range embeddings {
		checks = append(checks, embed.ClaimWithEmbedding{ID: c.ID, Proposition: c.Proposition, Embedding: c.Embedding})
	}
	if dupes := embed.FindDuplicates(vec, checks, 0.95); len(dupes) > 0 {
		writeError(w, http.StatusConflict, "DUPLICATE_CLAIM", "near-duplicate claim exists", map[string]any{
			"dup_of":     dupes[0].ClaimID,
			"similarity": dupes[0].Similarity,
		})
		return
	}

	claimID := db.GenerateID()
	claim := &model.Claim{
		ID:          claimID,
		Proposition: req.Proposition,
		Embedding:   embed.MarshalVector(vec),
		Status:      "active",
	}
	if err := s.store.CreateClaim(claim); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to create claim", nil)
		return
	}

	var createdCitations []model.Citation
	for _, ci := range req.Citations {
		cit, errCode, errMsg, err := s.persistCitation(agentID, claimID, ci)
		if err != nil {
			writeError(w, http.StatusBadRequest, errCode, errMsg, nil)
			return
		}
		createdCitations = append(createdCitations, *cit)
	}

	for _, dep := range req.DependsOn {
		if dep.ClaimID == "" {
			continue
		}
		cycle, _ := s.store.HasCycle(claimID, dep.ClaimID)
		if cycle {
			continue
		}
		var reasoning *string
		if dep.Reasoning != "" {
			reasoning = &dep.Reasoning
		}
		strength := dep.Strength
		if strength == 0 {
			strength = 1.0
		}
		s.store.CreateDependency(&model.Dependency{
			ID:          db.GenerateID(),
			ClaimID:     claimID,
			DependsOnID: dep.ClaimID,
			Strength:    strength,
			Reasoning:   reasoning,
		})
	}

	writeData(w, http.StatusCreated, map[string]any{
		"claim":     claim,
		"citations": createdCitations,
	})
}

// --- Citations ---

type citationInput struct {
	SourceID      string  `json:"source_id,omitempty"`
	URL           string  `json:"url,omitempty"`
	VerbatimQuote string  `json:"verbatim_quote"`
	Locator       any     `json:"locator,omitempty"`
	Polarity      string  `json:"polarity"`
	Reasoning     string  `json:"reasoning,omitempty"`
}

type dependencyInput struct {
	ClaimID   string  `json:"claim_id"`
	Strength  float64 `json:"strength"`
	Reasoning string  `json:"reasoning,omitempty"`
}

func (s *Server) handleCreateCitation(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentID(r.Context())
	if !s.limiters.checkBurst(agentID) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests", nil)
		return
	}
	if !s.limiters.checkDaily(agentID, "citations", 100) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "daily citation limit reached", nil)
		return
	}

	var req struct {
		ClaimID string        `json:"claim_id"`
		citationInput
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	if req.ClaimID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "claim_id is required", nil)
		return
	}
	if _, err := s.store.GetClaim(req.ClaimID); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "claim not found", nil)
		return
	}

	cit, errCode, errMsg, err := s.persistCitation(agentID, req.ClaimID, req.citationInput)
	if err != nil {
		writeError(w, http.StatusBadRequest, errCode, errMsg, nil)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"citation": cit})
}

// persistCitation handles source resolution, the mechanical containment check, and DB insert.
func (s *Server) persistCitation(agentID, claimID string, in citationInput) (*model.Citation, string, string, error) {
	if in.VerbatimQuote == "" {
		return nil, "INVALID_REQUEST", "verbatim_quote is required", fmt.Errorf("missing quote")
	}
	if in.Polarity != "supports" && in.Polarity != "contradicts" && in.Polarity != "qualifies" {
		return nil, "INVALID_REQUEST", "polarity must be supports, contradicts, or qualifies", fmt.Errorf("bad polarity")
	}

	var src *model.Source
	if in.SourceID != "" {
		got, err := s.store.GetSource(in.SourceID)
		if err != nil {
			return nil, "NOT_FOUND", "source not found", err
		}
		src = got
	} else if in.URL != "" {
		got, err := s.store.GetSourceByURL(in.URL)
		if err != nil {
			return nil, "INTERNAL", "lookup failed", err
		}
		if got == nil {
			res, err := s.ingester.Ingest(in.URL)
			if err != nil {
				return nil, "FETCH_FAILED", fmt.Sprintf("could not ingest url: %v", err), err
			}
			src = res.Source
		} else {
			src = got
		}
	} else {
		return nil, "INVALID_REQUEST", "source_id or url required", fmt.Errorf("missing source")
	}

	body, err := s.ingester.LoadBody(src)
	if err != nil {
		return nil, "INTERNAL", "could not load source body", err
	}
	if !db.HasSourceQuote(string(body), in.VerbatimQuote) {
		return nil, "MECHANICAL_FAIL", "verbatim_quote not found in source body", fmt.Errorf("mechanical fail")
	}

	var locatorJSON *string
	if in.Locator != nil {
		bytes, _ := json.Marshal(in.Locator)
		s := string(bytes)
		locatorJSON = &s
	}
	var reasoning *string
	if in.Reasoning != "" {
		reasoning = &in.Reasoning
	}
	cit := &model.Citation{
		ID:            db.GenerateID(),
		ClaimID:       claimID,
		SourceID:      src.ID,
		VerbatimQuote: in.VerbatimQuote,
		Locator:       locatorJSON,
		Polarity:      in.Polarity,
		Reasoning:     reasoning,
		ExtractorID:   agentID,
		AuditFactor:   1.0,
		Status:        "active",
	}
	if err := s.store.CreateCitation(cit); err != nil {
		return nil, "INTERNAL", "failed to persist citation", err
	}
	return cit, "", "", nil
}

func (s *Server) handleGetCitation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cit, err := s.store.GetCitation(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "citation not found", nil)
		return
	}
	src, _ := s.store.GetSource(cit.SourceID)
	claim, _ := s.store.GetClaim(cit.ClaimID)
	audits, _ := s.store.ListAuditsByCitation(id)
	writeData(w, http.StatusOK, map[string]any{
		"citation": cit,
		"source":   src,
		"claim":    claim,
		"audits":   audits,
	})
}

func (s *Server) handleGetCitationAudits(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	audits, err := s.store.ListAuditsByCitation(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "list failed", nil)
		return
	}
	writeData(w, http.StatusOK, audits)
}

// --- Audits ---

func (s *Server) handleCreateAudit(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentID(r.Context())
	if !s.limiters.checkBurst(agentID) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests", nil)
		return
	}
	if !s.limiters.checkDaily(agentID, "audits", 500) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "daily audit limit reached", nil)
		return
	}

	var req struct {
		CitationID string `json:"citation_id"`
		Semantic   string `json:"semantic"`
		Reasoning  string `json:"reasoning"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	if req.CitationID == "" || req.Semantic == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "citation_id and semantic required", nil)
		return
	}
	if !validSemantic(req.Semantic) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid semantic verdict", nil)
		return
	}

	cit, err := s.store.GetCitation(req.CitationID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "citation not found", nil)
		return
	}
	if cit.ExtractorID == agentID {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "cannot audit your own citation", nil)
		return
	}

	src, err := s.store.GetSource(cit.SourceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "source missing", nil)
		return
	}
	body, err := s.ingester.LoadBody(src)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "body missing", nil)
		return
	}
	mech := "pass"
	if !db.HasSourceQuote(string(body), cit.VerbatimQuote) {
		mech = "fail"
	}

	verdict := "uphold"
	if mech == "fail" || req.Semantic == "misquote" || req.Semantic == "out_of_context" || req.Semantic == "broken_link" {
		verdict = "reject"
	} else if req.Semantic == "weak" {
		verdict = "reject" // weak counts as rejection of the citation as it stands
	}

	var reasoning *string
	if req.Reasoning != "" {
		reasoning = &req.Reasoning
	}
	audit := &model.Audit{
		ID:         db.GenerateID(),
		CitationID: req.CitationID,
		AuditorID:  agentID,
		Mechanical: mech,
		Semantic:   req.Semantic,
		Verdict:    verdict,
		Reasoning:  reasoning,
	}
	if err := s.store.CreateAudit(audit); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to persist audit", nil)
		return
	}
	if mech == "fail" {
		s.store.UpdateCitationStatus(cit.ID, "rejected")
		s.store.UpdateCitationAuditFactor(cit.ID, 0)
	}
	writeData(w, http.StatusCreated, map[string]any{"audit": audit})
}

func (s *Server) handleAuditQueue(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentID(r.Context())
	limit := parseIntParam(r, "limit", 10)
	minAudits := parseIntParam(r, "min_audits", 3)
	citations, err := s.store.CitationsNeedingAudit(agentID, minAudits, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "queue failed", nil)
		return
	}
	writeData(w, http.StatusOK, citations)
}

func validSemantic(v string) bool {
	switch v {
	case "confirm", "misquote", "out_of_context", "weak", "broken_link":
		return true
	}
	return false
}

// --- Dependencies ---

func (s *Server) handleGetClaimDependencies(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	deps, _ := s.store.ListDependenciesByClaim(id)
	dependents, _ := s.store.ListDependents(id)
	writeData(w, http.StatusOK, map[string]any{
		"depends_on":   deps,
		"depended_by":  dependents,
	})
}

func (s *Server) handleCreateDependency(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentID(r.Context())
	if !s.limiters.checkBurst(agentID) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests", nil)
		return
	}

	var req struct {
		ClaimID     string  `json:"claim_id"`
		DependsOnID string  `json:"depends_on_id"`
		Strength    float64 `json:"strength"`
		Reasoning   string  `json:"reasoning"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	if req.ClaimID == "" || req.DependsOnID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "claim_id and depends_on_id required", nil)
		return
	}
	cycle, _ := s.store.HasCycle(req.ClaimID, req.DependsOnID)
	if cycle {
		writeError(w, http.StatusBadRequest, "CYCLE_DETECTED", "would create a cycle", nil)
		return
	}
	strength := req.Strength
	if strength == 0 {
		strength = 1.0
	}
	dep := &model.Dependency{
		ID:          db.GenerateID(),
		ClaimID:     req.ClaimID,
		DependsOnID: req.DependsOnID,
		Strength:    strength,
		Reasoning:   nullableString(req.Reasoning),
	}
	if err := s.store.CreateDependency(dep); err != nil {
		// duplicate?
		if existing, lookupErr := s.store.GetDependencyByClaimPair(req.ClaimID, req.DependsOnID); lookupErr == nil && existing != nil {
			writeData(w, http.StatusOK, map[string]any{"dependency": existing})
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL", "create dep failed", nil)
		return
	}
	_ = agentID
	writeData(w, http.StatusCreated, map[string]any{"dependency": dep})
}

// --- Lenses ---

func (s *Server) handleCreateLens(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentID(r.Context())
	var req struct {
		Slug         string  `json:"slug"`
		Description  string  `json:"description"`
		ParentLensID *string `json:"parent_lens_id,omitempty"`
		Public       bool    `json:"public"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	if req.Slug == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "slug is required", nil)
		return
	}
	desc := req.Description
	owner := agentID
	l := &model.Lens{
		ID:           db.GenerateID(),
		Slug:         req.Slug,
		OwnerID:      &owner,
		ParentLensID: req.ParentLensID,
		Description:  &desc,
		Public:       req.Public,
	}
	if err := s.store.CreateLens(l); err != nil {
		writeError(w, http.StatusBadRequest, "DUPLICATE_OR_INVALID", "could not create lens", nil)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"lens": l})
}

func (s *Server) handleListLenses(w http.ResponseWriter, r *http.Request) {
	lenses, err := s.store.ListLenses()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "list lenses failed", nil)
		return
	}
	writeData(w, http.StatusOK, lenses)
}

func (s *Server) handleGetLens(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	l, err := s.store.GetLensBySlug(slug)
	if err != nil || l == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "lens not found", nil)
		return
	}
	overrides, _ := s.store.ListLensOverrides(l.ID)
	tagOverrides, _ := s.store.ListLensTagOverrides(l.ID)
	writeData(w, http.StatusOK, map[string]any{
		"lens":          l,
		"overrides":     overrides,
		"tag_overrides": tagOverrides,
	})
}

func (s *Server) handleSetLensOverrides(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentID(r.Context())
	slug := r.PathValue("slug")
	l, err := s.store.GetLensBySlug(slug)
	if err != nil || l == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "lens not found", nil)
		return
	}
	if l.OwnerID != nil && *l.OwnerID != agentID && getRole(r.Context()) != "admin" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "not lens owner", nil)
		return
	}
	var req struct {
		Overrides    []model.LensOverride    `json:"overrides"`
		TagOverrides []model.LensTagOverride `json:"tag_overrides"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	for _, o := range req.Overrides {
		o.LensID = l.ID
		s.store.UpsertLensOverride(&o)
	}
	for _, t := range req.TagOverrides {
		t.LensID = l.ID
		s.store.UpsertLensTagOverride(&t)
	}
	writeData(w, http.StatusOK, map[string]any{"updated": true})
}

func (s *Server) handleForkLens(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentID(r.Context())
	slug := r.PathValue("slug")
	parent, err := s.store.GetLensBySlug(slug)
	if err != nil || parent == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "parent lens not found", nil)
		return
	}
	var req struct {
		NewSlug string `json:"slug"`
		Desc    string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	if req.NewSlug == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "slug required", nil)
		return
	}
	owner := agentID
	desc := req.Desc
	child := &model.Lens{
		ID:           db.GenerateID(),
		Slug:         req.NewSlug,
		OwnerID:      &owner,
		ParentLensID: &parent.ID,
		Description:  &desc,
		Public:       parent.Public,
	}
	if err := s.store.CreateLens(child); err != nil {
		writeError(w, http.StatusBadRequest, "DUPLICATE_OR_INVALID", "could not fork", nil)
		return
	}
	// Copy overrides
	overrides, _ := s.store.ListLensOverrides(parent.ID)
	for _, o := range overrides {
		o.LensID = child.ID
		s.store.UpsertLensOverride(&o)
	}
	tagOverrides, _ := s.store.ListLensTagOverrides(parent.ID)
	for _, t := range tagOverrides {
		t.LensID = child.ID
		s.store.UpsertLensTagOverride(&t)
	}
	writeData(w, http.StatusCreated, map[string]any{"lens": child})
}

// --- Discovery ---

func (s *Server) handleSourceLeaderboard(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 25)
	creds, err := s.store.LatestSourceCredibility()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "leaderboard failed", nil)
		return
	}

	// Apply lens if requested.
	if lensSlug := r.URL.Query().Get("lens"); lensSlug != "" {
		if lensRow, _ := s.store.GetLensBySlug(lensSlug); lensRow != nil {
			snap, err := lens.LoadSnapshot(s.store)
			if err == nil {
				spec, err := lens.LoadLensSpec(s.store, lensRow.ID)
				if err == nil {
					_ = spec
					// Re-compute credibility view via snap. For now use baseline; full lens render of credibility is a Phase 7 follow-up.
					_ = snap
				}
			}
		}
	}

	type row struct {
		SourceID    string  `json:"source_id"`
		Credibility float64 `json:"credibility"`
	}
	var rows []row
	for sid, v := range creds {
		rows = append(rows, row{SourceID: sid, Credibility: v})
	}
	// Sort descending
	for i := range rows {
		for j := i + 1; j < len(rows); j++ {
			if rows[j].Credibility > rows[i].Credibility {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	writeData(w, http.StatusOK, rows)
}

func (s *Server) handleContested(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 25)
	claims, err := s.store.MostContestedClaims(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "contested failed", nil)
		return
	}
	writeData(w, http.StatusOK, claims)
}

func (s *Server) handleFrontier(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 25)
	claims, err := s.store.FrontierClaims(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "frontier failed", nil)
		return
	}
	writeData(w, http.StatusOK, claims)
}

func (s *Server) handleListEpochs(w http.ResponseWriter, r *http.Request) {
	epochs, err := s.store.ListEpochs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "list epochs failed", nil)
		return
	}
	writeData(w, http.StatusOK, epochs)
}

func (s *Server) handleLatestEpoch(w http.ResponseWriter, r *http.Request) {
	epoch, err := s.store.GetLatestEpoch()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "latest epoch failed", nil)
		return
	}
	if epoch == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "no epochs yet", nil)
		return
	}
	writeData(w, http.StatusOK, epoch)
}

// --- Graph ---

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	type node struct {
		ID    string  `json:"id"`
		Label string  `json:"label"`
		Type  string  `json:"type"`
		Value float64 `json:"value"`
	}
	type link struct {
		Source   string  `json:"source"`
		Target   string  `json:"target"`
		Type     string  `json:"type"`
		Polarity string  `json:"polarity,omitempty"`
		Value    float64 `json:"value"`
	}
	var nodes []node
	var links []link

	claims, _ := s.store.ListAllClaims()
	for _, c := range claims {
		nodes = append(nodes, node{ID: "c:" + c.ID, Label: c.Proposition, Type: "claim", Value: c.EffectiveGroundedness})
	}
	srcs, _ := s.store.ListAllSources()
	creds, _ := s.store.LatestSourceCredibility()
	for _, src := range srcs {
		title := src.URL
		if src.Title != nil {
			title = *src.Title
		}
		nodes = append(nodes, node{ID: "s:" + src.ID, Label: title, Type: "source", Value: creds[src.ID]})
	}
	citations, _ := s.store.ListAllCitations()
	for _, ct := range citations {
		links = append(links, link{
			Source: "c:" + ct.ClaimID, Target: "s:" + ct.SourceID,
			Type: "citation", Polarity: ct.Polarity, Value: ct.AuditFactor,
		})
	}
	deps, _ := s.store.ListAllDependencies()
	for _, d := range deps {
		links = append(links, link{
			Source: "c:" + d.ClaimID, Target: "c:" + d.DependsOnID, Type: "dependency", Value: d.Strength,
		})
	}
	edges, _ := s.store.ListSourceCitationEdges()
	for _, e := range edges {
		links = append(links, link{
			Source: "s:" + e.FromSourceID, Target: "s:" + e.ToSourceID, Type: "source_citation", Value: 1.0,
		})
	}
	writeData(w, http.StatusOK, map[string]any{"nodes": nodes, "links": links})
}

// --- Admin ---

func (s *Server) handleAdjudicate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClaimID   string  `json:"claim_id"`
		Value     float64 `json:"value"`
		Reasoning string  `json:"reasoning"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	if req.ClaimID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "claim_id is required", nil)
		return
	}
	citations, _ := s.store.ListCitationsByClaim(req.ClaimID)
	if len(citations) == 0 {
		writeError(w, http.StatusBadRequest, "REQUIRES_CITATIONS", "adjudicated claims require at least 1 citation", nil)
		return
	}
	agentID := getAgentID(r.Context())
	if err := s.store.AdjudicateClaim(req.ClaimID, req.Value, agentID, req.Reasoning); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "adjudicate failed", nil)
		return
	}
	claim, _ := s.store.GetClaim(req.ClaimID)
	writeData(w, http.StatusOK, map[string]any{"claim": claim})
}

// --- Helpers ---

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// generateID is kept for tests/IssueToken; prefer db.GenerateID.
func generateID() string {
	return db.GenerateID()
}

// IssueTokenAt is a wrapper retained for back-compat shape.
var _ = time.Now
