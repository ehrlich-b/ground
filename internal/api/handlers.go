package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ehrlich-b/ground/internal/embed"
	"github.com/ehrlich-b/ground/internal/model"
)

// --- Agents ---

func (s *Server) handleRegisterAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string  `json:"name"`
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

	agentID := generateID()
	agent := &model.Agent{
		ID:           agentID,
		Name:         req.Name,
		Accuracy:     1.0,
		Contribution: 1.0,
		Weight:       2.0,
		Metadata:     req.Metadata,
	}
	if err := s.store.CreateAgent(agent); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to create agent", nil)
		return
	}

	// Create JWT
	tokenStr, err := createJWT(s.jwtSecret, agentID, "agent")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to create token", nil)
		return
	}

	// Store token hash
	tok := &model.APIToken{
		ID:        generateID(),
		AgentID:   agentID,
		TokenHash: hashToken(tokenStr),
		Role:      "agent",
		ExpiresAt: time.Now().Add(90 * 24 * time.Hour),
	}
	if err := s.store.CreateAPIToken(tok); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to store token", nil)
		return
	}

	writeData(w, http.StatusCreated, map[string]any{
		"agent": agent,
		"token": tokenStr,
	})
}

func (s *Server) handleRotateToken(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentID(r.Context())

	// Revoke all existing tokens for this agent
	if err := s.store.RevokeAgentTokens(agentID); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to revoke old tokens", nil)
		return
	}

	tokenStr, err := createJWT(s.jwtSecret, agentID, "agent")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to create token", nil)
		return
	}

	tok := &model.APIToken{
		ID:        generateID(),
		AgentID:   agentID,
		TokenHash: hashToken(tokenStr),
		Role:      "agent",
		ExpiresAt: time.Now().Add(90 * 24 * time.Hour),
	}
	if err := s.store.CreateAPIToken(tok); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to store token", nil)
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
	writeData(w, http.StatusOK, agent)
}

func (s *Server) handleGetAgentAssertions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	limit := parseIntParam(r, "limit", 50)
	offset := parseIntParam(r, "offset", 0)

	assertions, err := s.store.ListAssertionsByAgent(id, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to list assertions", nil)
		return
	}
	writeData(w, http.StatusOK, assertions)
}

func (s *Server) handleGetAgentReviews(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	limit := parseIntParam(r, "limit", 50)
	offset := parseIntParam(r, "offset", 0)

	reviews, err := s.store.ListReviewsByReviewer(id, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to list reviews", nil)
		return
	}
	writeData(w, http.StatusOK, reviews)
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

	// Generate embedding
	vec, err := s.embedder.Embed(req.Title + ": " + req.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EMBED_FAILED", "failed to generate embedding", nil)
		return
	}

	// Exclusion check
	exclusions, err := s.store.ListTopicExclusions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to check exclusions", nil)
		return
	}
	var anchors []embed.ExclusionAnchor
	for _, e := range exclusions {
		anchors = append(anchors, embed.ExclusionAnchor{
			ID: e.ID, Description: e.Description, Embedding: e.Embedding, Threshold: e.Threshold,
		})
	}
	if hit := embed.CheckExclusions(vec, anchors); hit != nil {
		writeError(w, http.StatusForbidden, "EXCLUDED_TOPIC", "topic is too close to an excluded category", map[string]any{
			"exclusion": hit.Description,
		})
		return
	}

	// Duplicate topic check
	topicEmbeddings, err := s.store.ListTopicEmbeddings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to check duplicates", nil)
		return
	}
	var topicsForCheck []embed.TopicWithEmbedding
	for _, t := range topicEmbeddings {
		topicsForCheck = append(topicsForCheck, embed.TopicWithEmbedding{
			ID: t.ID, Slug: t.Slug, Title: t.Title, Embedding: t.Embedding,
		})
	}
	nearest := embed.FindNearestTopics(vec, topicsForCheck, 1)
	if len(nearest) > 0 && nearest[0].Similarity > 0.95 {
		writeError(w, http.StatusConflict, "DUPLICATE_TOPIC", "a very similar topic already exists", map[string]any{
			"existing_topic": nearest[0].Slug,
			"similarity":     nearest[0].Similarity,
		})
		return
	}

	slug := slugify(req.Title)
	desc := req.Description
	topic := &model.Topic{
		ID:          generateID(),
		Title:       req.Title,
		Slug:        slug,
		Description: &desc,
		Embedding:   embed.MarshalVector(vec),
	}
	if err := s.store.CreateTopic(topic); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to create topic", nil)
		return
	}

	// Return with proximity to existing topics
	var proximity []map[string]any
	allNearest := embed.FindNearestTopics(vec, topicsForCheck, 5)
	for _, n := range allNearest {
		proximity = append(proximity, map[string]any{
			"topic_slug": n.Slug,
			"similarity": n.Similarity,
		})
	}

	writeData(w, http.StatusCreated, map[string]any{
		"topic":     topic,
		"proximity": proximity,
	})
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

	// Include assertions and dependencies
	assertions, _ := s.store.ListAssertionsByClaim(id)
	deps, _ := s.store.ListDependenciesByClaim(id)
	dependents, _ := s.store.ListDependents(id)

	writeData(w, http.StatusOK, map[string]any{
		"claim":        claim,
		"assertions":   assertions,
		"dependencies": deps,
		"dependents":   dependents,
	})
}

func (s *Server) handleCreateClaim(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentID(r.Context())

	if !s.limiters.checkBurst(agentID) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests per second", nil)
		return
	}
	if !s.limiters.checkDaily(agentID, "claims", 100) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "daily claim limit reached (100/day)", nil)
		return
	}

	var req struct {
		Proposition string  `json:"proposition"`
		TopicSlug   string  `json:"topic_slug"`
		Confidence  float64 `json:"confidence"`
		Reasoning   string  `json:"reasoning"`
		Sources     string  `json:"sources,omitempty"`
		DependsOn   []struct {
			ClaimID   string  `json:"claim_id"`
			Strength  float64 `json:"strength"`
			Reasoning string  `json:"reasoning"`
		} `json:"depends_on,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	if req.Proposition == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "proposition is required", nil)
		return
	}
	if req.Confidence < 0 || req.Confidence > 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "confidence must be between 0 and 1", nil)
		return
	}

	// Generate embedding
	vec, err := s.embedder.Embed(req.Proposition)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EMBED_FAILED", "failed to generate embedding", nil)
		return
	}

	// Duplicate check
	claimEmbeddings, err := s.store.ListClaimEmbeddings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to check duplicates", nil)
		return
	}
	var claimsForCheck []embed.ClaimWithEmbedding
	for _, c := range claimEmbeddings {
		claimsForCheck = append(claimsForCheck, embed.ClaimWithEmbedding{
			ID: c.ID, Proposition: c.Proposition, Embedding: c.Embedding,
		})
	}
	dupes := embed.FindDuplicates(vec, claimsForCheck, 0.95)
	if len(dupes) > 0 {
		writeError(w, http.StatusConflict, "DUPLICATE_CLAIM", "a claim with very similar proposition already exists", map[string]any{
			"existing_claim_id": dupes[0].ClaimID,
			"similarity":        dupes[0].Similarity,
		})
		return
	}

	// Exclusion check
	exclusions, err := s.store.ListTopicExclusions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to check exclusions", nil)
		return
	}
	var anchors []embed.ExclusionAnchor
	for _, e := range exclusions {
		anchors = append(anchors, embed.ExclusionAnchor{
			ID: e.ID, Description: e.Description, Embedding: e.Embedding, Threshold: e.Threshold,
		})
	}
	if hit := embed.CheckExclusions(vec, anchors); hit != nil {
		writeError(w, http.StatusForbidden, "EXCLUDED_CONTENT", "claim is too close to an excluded category", map[string]any{
			"exclusion": hit.Description,
		})
		return
	}

	// Topic proximity
	topicEmbeddings, err := s.store.ListTopicEmbeddings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to check topic proximity", nil)
		return
	}
	var topicsForCheck []embed.TopicWithEmbedding
	for _, t := range topicEmbeddings {
		topicsForCheck = append(topicsForCheck, embed.TopicWithEmbedding{
			ID: t.ID, Slug: t.Slug, Title: t.Title, Embedding: t.Embedding,
		})
	}
	nearestTopics := embed.FindNearestTopics(vec, topicsForCheck, 5)

	// Create claim
	claimID := generateID()
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

	// Auto-assert (the submitter supports their own claim)
	var reasoning *string
	if req.Reasoning != "" {
		reasoning = &req.Reasoning
	}
	var sources *string
	if req.Sources != "" {
		sources = &req.Sources
	}
	assertion := &model.Assertion{
		ID:         generateID(),
		AgentID:    agentID,
		ClaimID:    claimID,
		Stance:     "support",
		Confidence: req.Confidence,
		Reasoning:  reasoning,
		Sources:    sources,
	}
	if err := s.store.CreateAssertion(assertion); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to create auto-assertion", nil)
		return
	}

	// Create dependencies
	for _, dep := range req.DependsOn {
		cycle, err := s.store.HasCycle(claimID, dep.ClaimID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to check dependency cycle", nil)
			return
		}
		if cycle {
			continue // skip cyclic dependencies silently
		}
		var depReasoning *string
		if dep.Reasoning != "" {
			depReasoning = &dep.Reasoning
		}
		d := &model.Dependency{
			ID:          generateID(),
			ClaimID:     claimID,
			DependsOnID: dep.ClaimID,
			Strength:    dep.Strength,
			Reasoning:   depReasoning,
		}
		s.store.CreateDependency(d)
	}

	// Build response with topic proximity
	var proximity []map[string]any
	for _, n := range nearestTopics {
		proximity = append(proximity, map[string]any{
			"topic_slug": n.Slug,
			"similarity": n.Similarity,
		})
	}

	// Find near-duplicates to suggest (0.7 - 0.95 similarity)
	nearDupes := embed.FindDuplicates(vec, claimsForCheck, 0.7)
	var similar []map[string]any
	for _, d := range nearDupes {
		if d.Similarity < 0.95 {
			similar = append(similar, map[string]any{
				"claim_id":    d.ClaimID,
				"proposition": d.Proposition,
				"similarity":  d.Similarity,
			})
		}
	}

	writeData(w, http.StatusCreated, map[string]any{
		"claim":            claim,
		"assertion":        assertion,
		"topic_proximity":  proximity,
		"similar_claims":   similar,
	})
}

// --- Assertions ---

func (s *Server) handleGetAssertion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	assertion, err := s.store.GetAssertion(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "assertion not found", nil)
		return
	}

	reviews, _ := s.store.ListReviewsByAssertion(id)

	writeData(w, http.StatusOK, map[string]any{
		"assertion": assertion,
		"reviews":   reviews,
	})
}

func (s *Server) handleCreateAssertion(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentID(r.Context())

	if !s.limiters.checkBurst(agentID) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests per second", nil)
		return
	}
	if !s.limiters.checkDaily(agentID, "assertions", 500) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "daily assertion limit reached (500/day)", nil)
		return
	}

	var req struct {
		ClaimID             string  `json:"claim_id"`
		Stance              string  `json:"stance"`
		Confidence          float64 `json:"confidence"`
		Reasoning           string  `json:"reasoning,omitempty"`
		Sources             string  `json:"sources,omitempty"`
		RefinedProposition  string  `json:"refined_proposition,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	if req.ClaimID == "" || req.Stance == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "claim_id and stance are required", nil)
		return
	}
	if req.Stance != "support" && req.Stance != "contest" && req.Stance != "refine" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "stance must be support, contest, or refine", nil)
		return
	}
	if req.Confidence < 0 || req.Confidence > 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "confidence must be between 0 and 1", nil)
		return
	}
	if req.Stance == "refine" && req.RefinedProposition == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "refined_proposition required for refine stance", nil)
		return
	}

	// Check claim exists
	_, err := s.store.GetClaim(req.ClaimID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "claim not found", nil)
		return
	}

	var reasoning, sources *string
	if req.Reasoning != "" {
		reasoning = &req.Reasoning
	}
	if req.Sources != "" {
		sources = &req.Sources
	}

	// Check if assertion already exists (update flow)
	existing, err := s.store.GetAssertionByAgentClaim(agentID, req.ClaimID)
	if err == nil && existing != nil {
		// Update existing assertion
		existing.Stance = req.Stance
		existing.Confidence = req.Confidence
		existing.Reasoning = reasoning
		existing.Sources = sources
		if err := s.store.UpdateAssertion(existing); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to update assertion", nil)
			return
		}
		writeData(w, http.StatusOK, map[string]any{"assertion": existing, "updated": true})
		return
	}

	assertion := &model.Assertion{
		ID:         generateID(),
		AgentID:    agentID,
		ClaimID:    req.ClaimID,
		Stance:     req.Stance,
		Confidence: req.Confidence,
		Reasoning:  reasoning,
		Sources:    sources,
	}

	// Handle refine — create child claim
	if req.Stance == "refine" {
		vec, err := s.embedder.Embed(req.RefinedProposition)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "EMBED_FAILED", "failed to embed refined proposition", nil)
			return
		}

		childID := generateID()
		parentID := req.ClaimID
		child := &model.Claim{
			ID:            childID,
			Proposition:   req.RefinedProposition,
			Embedding:     embed.MarshalVector(vec),
			Status:        "active",
			ParentClaimID: &parentID,
		}
		if err := s.store.CreateClaim(child); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to create refined claim", nil)
			return
		}

		assertion.RefinementClaimID = &childID
		if err := s.store.CreateAssertion(assertion); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to create assertion", nil)
			return
		}

		// Auto-support the refined claim
		childAssertion := &model.Assertion{
			ID:         generateID(),
			AgentID:    agentID,
			ClaimID:    childID,
			Stance:     "support",
			Confidence: req.Confidence,
			Reasoning:  reasoning,
			Sources:    sources,
		}
		s.store.CreateAssertion(childAssertion)

		writeData(w, http.StatusCreated, map[string]any{
			"assertion":     assertion,
			"refined_claim": child,
		})
		return
	}

	if err := s.store.CreateAssertion(assertion); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to create assertion", nil)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"assertion": assertion})
}

// --- Reviews ---

func (s *Server) handleGetAssertionReviews(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	reviews, err := s.store.ListReviewsByAssertion(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to list reviews", nil)
		return
	}
	writeData(w, http.StatusOK, reviews)
}

func (s *Server) handleCreateReview(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentID(r.Context())

	if !s.limiters.checkBurst(agentID) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests per second", nil)
		return
	}
	if !s.limiters.checkDaily(agentID, "reviews", 1000) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "daily review limit reached (1000/day)", nil)
		return
	}

	var req struct {
		AssertionID string  `json:"assertion_id"`
		Helpfulness float64 `json:"helpfulness"`
		Reasoning   string  `json:"reasoning,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	if req.AssertionID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "assertion_id is required", nil)
		return
	}
	if req.Helpfulness < 0 || req.Helpfulness > 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "helpfulness must be between 0 and 1", nil)
		return
	}

	// Check assertion exists
	assertion, err := s.store.GetAssertion(req.AssertionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "assertion not found", nil)
		return
	}

	// Can't review your own assertions
	if assertion.AgentID == agentID {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "cannot review your own assertion", nil)
		return
	}

	var reasoning *string
	if req.Reasoning != "" {
		reasoning = &req.Reasoning
	}

	// Check for existing review (update flow)
	existing, err := s.store.GetReviewByReviewerAssertion(agentID, req.AssertionID)
	if err == nil && existing != nil {
		existing.Helpfulness = req.Helpfulness
		existing.Reasoning = reasoning
		if err := s.store.UpdateReview(existing); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to update review", nil)
			return
		}
		writeData(w, http.StatusOK, map[string]any{"review": existing, "updated": true})
		return
	}

	review := &model.Review{
		ID:          generateID(),
		ReviewerID:  agentID,
		AssertionID: req.AssertionID,
		Helpfulness: req.Helpfulness,
		Reasoning:   reasoning,
	}
	if err := s.store.CreateReview(review); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to create review", nil)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"review": review})
}

// --- Dependencies ---

func (s *Server) handleGetClaimDependencies(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	deps, err := s.store.ListDependenciesByClaim(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to list dependencies", nil)
		return
	}
	dependents, err := s.store.ListDependents(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to list dependents", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"depends_on": deps,
		"depended_by": dependents,
	})
}

func (s *Server) handleCreateDependency(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentID(r.Context())

	if !s.limiters.checkBurst(agentID) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests per second", nil)
		return
	}

	var req struct {
		ClaimID     string  `json:"claim_id"`
		DependsOnID string  `json:"depends_on_id"`
		Strength    float64 `json:"strength"`
		Reasoning   string  `json:"reasoning,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	if req.ClaimID == "" || req.DependsOnID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "claim_id and depends_on_id are required", nil)
		return
	}
	if req.Strength < 0 || req.Strength > 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "strength must be between 0 and 1", nil)
		return
	}

	// Cycle check
	cycle, err := s.store.HasCycle(req.ClaimID, req.DependsOnID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to check for cycles", nil)
		return
	}
	if cycle {
		writeError(w, http.StatusBadRequest, "CYCLE_DETECTED", "this dependency would create a cycle", nil)
		return
	}

	var reasoning *string
	if req.Reasoning != "" {
		reasoning = &req.Reasoning
	}

	dep := &model.Dependency{
		ID:          generateID(),
		ClaimID:     req.ClaimID,
		DependsOnID: req.DependsOnID,
		Strength:    req.Strength,
		Reasoning:   reasoning,
	}
	if err := s.store.CreateDependency(dep); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to create dependency", nil)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"dependency": dep})
}

// --- Discovery ---

func (s *Server) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 25)
	agents, err := s.store.TopAgentsByWeight(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to get leaderboard", nil)
		return
	}
	writeData(w, http.StatusOK, agents)
}

func (s *Server) handleContested(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 25)
	claims, err := s.store.MostContestedClaims(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to get contested claims", nil)
		return
	}
	writeData(w, http.StatusOK, claims)
}

func (s *Server) handleFrontier(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 25)
	claims, err := s.store.FrontierClaims(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to get frontier", nil)
		return
	}
	writeData(w, http.StatusOK, claims)
}

func (s *Server) handleListEpochs(w http.ResponseWriter, r *http.Request) {
	epochs, err := s.store.ListEpochs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to list epochs", nil)
		return
	}
	writeData(w, http.StatusOK, epochs)
}

func (s *Server) handleLatestEpoch(w http.ResponseWriter, r *http.Request) {
	epoch, err := s.store.GetLatestEpoch()
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "no epochs yet", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to get latest epoch", nil)
		return
	}
	writeData(w, http.StatusOK, epoch)
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

	agentID := getAgentID(r.Context())
	if err := s.store.AdjudicateClaim(req.ClaimID, req.Value, agentID, req.Reasoning); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to adjudicate claim", nil)
		return
	}

	claim, _ := s.store.GetClaim(req.ClaimID)
	writeData(w, http.StatusOK, map[string]any{"claim": claim})
}

func (s *Server) handleCascade(w http.ResponseWriter, r *http.Request) {
	// Cascade analysis: find claims whose effective groundedness is threatened by
	// low-groundedness dependencies
	claims, err := s.store.ListAllClaims()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to list claims", nil)
		return
	}

	var threatened []map[string]any
	for _, c := range claims {
		if c.Status == "adjudicated" {
			continue
		}
		deps, _ := s.store.ListDependenciesByClaim(c.ID)
		for _, d := range deps {
			dep, err := s.store.GetClaim(d.DependsOnID)
			if err != nil {
				continue
			}
			if dep.EffectiveGroundedness < 0.5 && d.Strength > 0.5 {
				threatened = append(threatened, map[string]any{
					"claim_id":              c.ID,
					"proposition":           c.Proposition,
					"dependency_id":         d.DependsOnID,
					"dependency_groundedness": dep.EffectiveGroundedness,
					"strength":              d.Strength,
				})
			}
		}
	}

	writeData(w, http.StatusOK, map[string]any{"threatened_claims": threatened})
}

// --- Helpers ---

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

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
	// Collapse multiple dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}
