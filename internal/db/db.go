package db

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/ehrlich-b/ground/internal/model"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// WAL mode + foreign keys
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			return nil, fmt.Errorf("exec %s: %w", pragma, err)
		}
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		var applied int
		err := s.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", name).Scan(&applied)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied > 0 {
			continue
		}

		data, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", name, err)
		}

		if _, err := tx.Exec(string(data)); err != nil {
			tx.Rollback()
			return fmt.Errorf("exec migration %s: %w", name, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", name); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}

		log.Printf("applied migration: %s", name)
	}

	return nil
}

// --- Agents ---

func (s *Store) CreateAgent(a *model.Agent) error {
	_, err := s.db.Exec(
		`INSERT INTO agents (id, name, accuracy, contribution, weight, metadata) VALUES (?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.Accuracy, a.Contribution, a.Weight, a.Metadata,
	)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}
	return nil
}

func (s *Store) GetAgent(id string) (*model.Agent, error) {
	a := &model.Agent{}
	err := s.db.QueryRow(
		`SELECT id, name, accuracy, contribution, weight, metadata, created_at FROM agents WHERE id = ?`, id,
	).Scan(&a.ID, &a.Name, &a.Accuracy, &a.Contribution, &a.Weight, &a.Metadata, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	return a, nil
}

func (s *Store) ListAgents() ([]model.Agent, error) {
	rows, err := s.db.Query(`SELECT id, name, accuracy, contribution, weight, metadata, created_at FROM agents ORDER BY weight DESC`)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()
	return scanAgents(rows)
}

func (s *Store) UpdateAgentScores(id string, accuracy, contribution, weight float64) error {
	_, err := s.db.Exec(
		`UPDATE agents SET accuracy = ?, contribution = ?, weight = ? WHERE id = ?`,
		accuracy, contribution, weight, id,
	)
	if err != nil {
		return fmt.Errorf("update agent scores: %w", err)
	}
	return nil
}

func (s *Store) CountAgents() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM agents`).Scan(&count)
	return count, err
}

func scanAgents(rows *sql.Rows) ([]model.Agent, error) {
	var agents []model.Agent
	for rows.Next() {
		var a model.Agent
		if err := rows.Scan(&a.ID, &a.Name, &a.Accuracy, &a.Contribution, &a.Weight, &a.Metadata, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// --- Topics ---

func (s *Store) CreateTopic(t *model.Topic) error {
	_, err := s.db.Exec(
		`INSERT INTO topics (id, title, slug, description, embedding) VALUES (?, ?, ?, ?, ?)`,
		t.ID, t.Title, t.Slug, t.Description, t.Embedding,
	)
	if err != nil {
		return fmt.Errorf("create topic: %w", err)
	}
	return nil
}

func (s *Store) GetTopic(id string) (*model.Topic, error) {
	t := &model.Topic{}
	err := s.db.QueryRow(
		`SELECT id, title, slug, description, embedding, created_at FROM topics WHERE id = ?`, id,
	).Scan(&t.ID, &t.Title, &t.Slug, &t.Description, &t.Embedding, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get topic: %w", err)
	}
	return t, nil
}

func (s *Store) GetTopicBySlug(slug string) (*model.Topic, error) {
	t := &model.Topic{}
	err := s.db.QueryRow(
		`SELECT id, title, slug, description, embedding, created_at FROM topics WHERE slug = ?`, slug,
	).Scan(&t.ID, &t.Title, &t.Slug, &t.Description, &t.Embedding, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get topic by slug: %w", err)
	}
	return t, nil
}

func (s *Store) ListTopics() ([]model.Topic, error) {
	rows, err := s.db.Query(`SELECT id, title, slug, description, embedding, created_at FROM topics ORDER BY title`)
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	defer rows.Close()
	return scanTopics(rows)
}

func (s *Store) CountTopics() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM topics`).Scan(&count)
	return count, err
}

func scanTopics(rows *sql.Rows) ([]model.Topic, error) {
	var topics []model.Topic
	for rows.Next() {
		var t model.Topic
		if err := rows.Scan(&t.ID, &t.Title, &t.Slug, &t.Description, &t.Embedding, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan topic: %w", err)
		}
		topics = append(topics, t)
	}
	return topics, rows.Err()
}

// --- Topic Exclusions ---

func (s *Store) CreateTopicExclusion(e *model.TopicExclusion) error {
	_, err := s.db.Exec(
		`INSERT INTO topic_exclusions (id, description, embedding, threshold) VALUES (?, ?, ?, ?)`,
		e.ID, e.Description, e.Embedding, e.Threshold,
	)
	if err != nil {
		return fmt.Errorf("create topic exclusion: %w", err)
	}
	return nil
}

func (s *Store) ListTopicExclusions() ([]model.TopicExclusion, error) {
	rows, err := s.db.Query(`SELECT id, description, embedding, threshold, created_at FROM topic_exclusions ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list topic exclusions: %w", err)
	}
	defer rows.Close()
	var exclusions []model.TopicExclusion
	for rows.Next() {
		var e model.TopicExclusion
		if err := rows.Scan(&e.ID, &e.Description, &e.Embedding, &e.Threshold, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan topic exclusion: %w", err)
		}
		exclusions = append(exclusions, e)
	}
	return exclusions, rows.Err()
}

// --- Claims ---

func (s *Store) CreateClaim(c *model.Claim) error {
	_, err := s.db.Exec(
		`INSERT INTO claims (id, proposition, embedding, groundedness, effective_groundedness, contestation, status, adjudicated_value, adjudicated_at, adjudicated_by, adjudication_reasoning, parent_claim_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Proposition, c.Embedding, c.Groundedness, c.EffectiveGroundedness, c.Contestation, c.Status,
		c.AdjudicatedValue, c.AdjudicatedAt, c.AdjudicatedBy, c.AdjudicationReasoning, c.ParentClaimID,
	)
	if err != nil {
		return fmt.Errorf("create claim: %w", err)
	}
	return nil
}

func (s *Store) GetClaim(id string) (*model.Claim, error) {
	c := &model.Claim{}
	err := s.db.QueryRow(
		`SELECT id, proposition, embedding, groundedness, effective_groundedness, contestation, status,
		        adjudicated_value, adjudicated_at, adjudicated_by, adjudication_reasoning, parent_claim_id,
		        created_at, computed_at
		 FROM claims WHERE id = ?`, id,
	).Scan(&c.ID, &c.Proposition, &c.Embedding, &c.Groundedness, &c.EffectiveGroundedness, &c.Contestation, &c.Status,
		&c.AdjudicatedValue, &c.AdjudicatedAt, &c.AdjudicatedBy, &c.AdjudicationReasoning, &c.ParentClaimID,
		&c.CreatedAt, &c.ComputedAt)
	if err != nil {
		return nil, fmt.Errorf("get claim: %w", err)
	}
	return c, nil
}

func (s *Store) ListClaims(status string, limit, offset int) ([]model.Claim, error) {
	var rows *sql.Rows
	var err error
	if status != "" {
		rows, err = s.db.Query(
			`SELECT id, proposition, embedding, groundedness, effective_groundedness, contestation, status,
			        adjudicated_value, adjudicated_at, adjudicated_by, adjudication_reasoning, parent_claim_id,
			        created_at, computed_at
			 FROM claims WHERE status = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`, status, limit, offset)
	} else {
		rows, err = s.db.Query(
			`SELECT id, proposition, embedding, groundedness, effective_groundedness, contestation, status,
			        adjudicated_value, adjudicated_at, adjudicated_by, adjudication_reasoning, parent_claim_id,
			        created_at, computed_at
			 FROM claims ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("list claims: %w", err)
	}
	defer rows.Close()
	return scanClaims(rows)
}

func (s *Store) ListAllClaims() ([]model.Claim, error) {
	rows, err := s.db.Query(
		`SELECT id, proposition, embedding, groundedness, effective_groundedness, contestation, status,
		        adjudicated_value, adjudicated_at, adjudicated_by, adjudication_reasoning, parent_claim_id,
		        created_at, computed_at
		 FROM claims ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list all claims: %w", err)
	}
	defer rows.Close()
	return scanClaims(rows)
}

func (s *Store) UpdateClaimScores(id string, groundedness, effectiveGroundedness, contestation float64, status string, computedAt time.Time) error {
	_, err := s.db.Exec(
		`UPDATE claims SET groundedness = ?, effective_groundedness = ?, contestation = ?, status = ?, computed_at = ? WHERE id = ?`,
		groundedness, effectiveGroundedness, contestation, status, computedAt, id,
	)
	if err != nil {
		return fmt.Errorf("update claim scores: %w", err)
	}
	return nil
}

func (s *Store) AdjudicateClaim(id string, value float64, by, reasoning string) error {
	now := time.Now().UTC()
	status := "adjudicated"
	_, err := s.db.Exec(
		`UPDATE claims SET adjudicated_value = ?, adjudicated_at = ?, adjudicated_by = ?, adjudication_reasoning = ?,
		        groundedness = ?, effective_groundedness = ?, status = ? WHERE id = ?`,
		value, now, by, reasoning, value, value, status, id,
	)
	if err != nil {
		return fmt.Errorf("adjudicate claim: %w", err)
	}
	return nil
}

func (s *Store) CountClaims() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM claims`).Scan(&count)
	return count, err
}

func (s *Store) CountClaimsByStatus() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM claims GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("count claims by status: %w", err)
	}
	defer rows.Close()
	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scan claim count: %w", err)
		}
		counts[status] = count
	}
	return counts, rows.Err()
}

func scanClaims(rows *sql.Rows) ([]model.Claim, error) {
	var claims []model.Claim
	for rows.Next() {
		var c model.Claim
		if err := rows.Scan(&c.ID, &c.Proposition, &c.Embedding, &c.Groundedness, &c.EffectiveGroundedness,
			&c.Contestation, &c.Status, &c.AdjudicatedValue, &c.AdjudicatedAt, &c.AdjudicatedBy,
			&c.AdjudicationReasoning, &c.ParentClaimID, &c.CreatedAt, &c.ComputedAt); err != nil {
			return nil, fmt.Errorf("scan claim: %w", err)
		}
		claims = append(claims, c)
	}
	return claims, rows.Err()
}

// --- Claim/Topic Embedding Queries ---

// ListClaimEmbeddings returns lightweight claim data for duplicate detection.
func (s *Store) ListClaimEmbeddings() ([]struct {
	ID          string
	Proposition string
	Embedding   []byte
}, error) {
	rows, err := s.db.Query(`SELECT id, proposition, embedding FROM claims WHERE embedding IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("list claim embeddings: %w", err)
	}
	defer rows.Close()
	var results []struct {
		ID          string
		Proposition string
		Embedding   []byte
	}
	for rows.Next() {
		var r struct {
			ID          string
			Proposition string
			Embedding   []byte
		}
		if err := rows.Scan(&r.ID, &r.Proposition, &r.Embedding); err != nil {
			return nil, fmt.Errorf("scan claim embedding: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ListTopicEmbeddings returns lightweight topic data for proximity checks.
func (s *Store) ListTopicEmbeddings() ([]struct {
	ID        string
	Slug      string
	Title     string
	Embedding []byte
}, error) {
	rows, err := s.db.Query(`SELECT id, slug, title, embedding FROM topics WHERE embedding IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("list topic embeddings: %w", err)
	}
	defer rows.Close()
	var results []struct {
		ID        string
		Slug      string
		Title     string
		Embedding []byte
	}
	for rows.Next() {
		var r struct {
			ID        string
			Slug      string
			Title     string
			Embedding []byte
		}
		if err := rows.Scan(&r.ID, &r.Slug, &r.Title, &r.Embedding); err != nil {
			return nil, fmt.Errorf("scan topic embedding: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// --- Assertions ---

func (s *Store) CreateAssertion(a *model.Assertion) error {
	_, err := s.db.Exec(
		`INSERT INTO assertions (id, agent_id, claim_id, stance, confidence, reasoning, sources, helpfulness, refinement_claim_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.AgentID, a.ClaimID, a.Stance, a.Confidence, a.Reasoning, a.Sources, a.Helpfulness, a.RefinementClaimID,
	)
	if err != nil {
		return fmt.Errorf("create assertion: %w", err)
	}
	return nil
}

func (s *Store) UpdateAssertion(a *model.Assertion) error {
	// Preserve old version in history first
	_, err := s.db.Exec(
		`INSERT INTO assertion_history (id, assertion_id, agent_id, claim_id, stance, confidence, reasoning, sources)
		 SELECT ?, id, agent_id, claim_id, stance, confidence, reasoning, sources
		 FROM assertions WHERE id = ?`,
		generateID(), a.ID,
	)
	if err != nil {
		return fmt.Errorf("save assertion history: %w", err)
	}

	_, err = s.db.Exec(
		`UPDATE assertions SET stance = ?, confidence = ?, reasoning = ?, sources = ?, refinement_claim_id = ? WHERE id = ?`,
		a.Stance, a.Confidence, a.Reasoning, a.Sources, a.RefinementClaimID, a.ID,
	)
	if err != nil {
		return fmt.Errorf("update assertion: %w", err)
	}
	return nil
}

func (s *Store) GetAssertion(id string) (*model.Assertion, error) {
	a := &model.Assertion{}
	err := s.db.QueryRow(
		`SELECT id, agent_id, claim_id, stance, confidence, reasoning, sources, helpfulness, refinement_claim_id, created_at
		 FROM assertions WHERE id = ?`, id,
	).Scan(&a.ID, &a.AgentID, &a.ClaimID, &a.Stance, &a.Confidence, &a.Reasoning, &a.Sources, &a.Helpfulness, &a.RefinementClaimID, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get assertion: %w", err)
	}
	return a, nil
}

func (s *Store) GetAssertionByAgentClaim(agentID, claimID string) (*model.Assertion, error) {
	a := &model.Assertion{}
	err := s.db.QueryRow(
		`SELECT id, agent_id, claim_id, stance, confidence, reasoning, sources, helpfulness, refinement_claim_id, created_at
		 FROM assertions WHERE agent_id = ? AND claim_id = ?`, agentID, claimID,
	).Scan(&a.ID, &a.AgentID, &a.ClaimID, &a.Stance, &a.Confidence, &a.Reasoning, &a.Sources, &a.Helpfulness, &a.RefinementClaimID, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get assertion by agent/claim: %w", err)
	}
	return a, nil
}

func (s *Store) ListAssertionsByClaim(claimID string) ([]model.Assertion, error) {
	rows, err := s.db.Query(
		`SELECT id, agent_id, claim_id, stance, confidence, reasoning, sources, helpfulness, refinement_claim_id, created_at
		 FROM assertions WHERE claim_id = ? ORDER BY created_at`, claimID)
	if err != nil {
		return nil, fmt.Errorf("list assertions by claim: %w", err)
	}
	defer rows.Close()
	return scanAssertions(rows)
}

func (s *Store) ListAssertionsByAgent(agentID string, limit, offset int) ([]model.Assertion, error) {
	rows, err := s.db.Query(
		`SELECT id, agent_id, claim_id, stance, confidence, reasoning, sources, helpfulness, refinement_claim_id, created_at
		 FROM assertions WHERE agent_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`, agentID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list assertions by agent: %w", err)
	}
	defer rows.Close()
	return scanAssertions(rows)
}

func (s *Store) ListAllAssertions() ([]model.Assertion, error) {
	rows, err := s.db.Query(
		`SELECT id, agent_id, claim_id, stance, confidence, reasoning, sources, helpfulness, refinement_claim_id, created_at
		 FROM assertions ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list all assertions: %w", err)
	}
	defer rows.Close()
	return scanAssertions(rows)
}

func (s *Store) UpdateAssertionHelpfulness(id string, helpfulness float64) error {
	_, err := s.db.Exec(`UPDATE assertions SET helpfulness = ? WHERE id = ?`, helpfulness, id)
	if err != nil {
		return fmt.Errorf("update assertion helpfulness: %w", err)
	}
	return nil
}

func scanAssertions(rows *sql.Rows) ([]model.Assertion, error) {
	var assertions []model.Assertion
	for rows.Next() {
		var a model.Assertion
		if err := rows.Scan(&a.ID, &a.AgentID, &a.ClaimID, &a.Stance, &a.Confidence, &a.Reasoning, &a.Sources, &a.Helpfulness, &a.RefinementClaimID, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan assertion: %w", err)
		}
		assertions = append(assertions, a)
	}
	return assertions, rows.Err()
}

// --- Reviews ---

func (s *Store) CreateReview(r *model.Review) error {
	_, err := s.db.Exec(
		`INSERT INTO reviews (id, reviewer_id, assertion_id, helpfulness, reasoning) VALUES (?, ?, ?, ?, ?)`,
		r.ID, r.ReviewerID, r.AssertionID, r.Helpfulness, r.Reasoning,
	)
	if err != nil {
		return fmt.Errorf("create review: %w", err)
	}
	return nil
}

func (s *Store) UpdateReview(r *model.Review) error {
	_, err := s.db.Exec(
		`UPDATE reviews SET helpfulness = ?, reasoning = ? WHERE id = ?`,
		r.Helpfulness, r.Reasoning, r.ID,
	)
	if err != nil {
		return fmt.Errorf("update review: %w", err)
	}
	return nil
}

func (s *Store) GetReviewByReviewerAssertion(reviewerID, assertionID string) (*model.Review, error) {
	r := &model.Review{}
	err := s.db.QueryRow(
		`SELECT id, reviewer_id, assertion_id, helpfulness, reasoning, created_at
		 FROM reviews WHERE reviewer_id = ? AND assertion_id = ?`, reviewerID, assertionID,
	).Scan(&r.ID, &r.ReviewerID, &r.AssertionID, &r.Helpfulness, &r.Reasoning, &r.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get review: %w", err)
	}
	return r, nil
}

func (s *Store) ListReviewsByAssertion(assertionID string) ([]model.Review, error) {
	rows, err := s.db.Query(
		`SELECT id, reviewer_id, assertion_id, helpfulness, reasoning, created_at
		 FROM reviews WHERE assertion_id = ? ORDER BY created_at`, assertionID)
	if err != nil {
		return nil, fmt.Errorf("list reviews by assertion: %w", err)
	}
	defer rows.Close()
	return scanReviews(rows)
}

func (s *Store) ListReviewsByReviewer(reviewerID string, limit, offset int) ([]model.Review, error) {
	rows, err := s.db.Query(
		`SELECT id, reviewer_id, assertion_id, helpfulness, reasoning, created_at
		 FROM reviews WHERE reviewer_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`, reviewerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list reviews by reviewer: %w", err)
	}
	defer rows.Close()
	return scanReviews(rows)
}

func (s *Store) ListAllReviews() ([]model.Review, error) {
	rows, err := s.db.Query(
		`SELECT id, reviewer_id, assertion_id, helpfulness, reasoning, created_at FROM reviews ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list all reviews: %w", err)
	}
	defer rows.Close()
	return scanReviews(rows)
}

func scanReviews(rows *sql.Rows) ([]model.Review, error) {
	var reviews []model.Review
	for rows.Next() {
		var r model.Review
		if err := rows.Scan(&r.ID, &r.ReviewerID, &r.AssertionID, &r.Helpfulness, &r.Reasoning, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan review: %w", err)
		}
		reviews = append(reviews, r)
	}
	return reviews, rows.Err()
}

// --- Dependencies ---

func (s *Store) CreateDependency(d *model.Dependency) error {
	_, err := s.db.Exec(
		`INSERT INTO dependencies (id, claim_id, depends_on_id, strength, reasoning) VALUES (?, ?, ?, ?, ?)`,
		d.ID, d.ClaimID, d.DependsOnID, d.Strength, d.Reasoning,
	)
	if err != nil {
		return fmt.Errorf("create dependency: %w", err)
	}
	return nil
}

func (s *Store) ListDependenciesByClaim(claimID string) ([]model.Dependency, error) {
	rows, err := s.db.Query(
		`SELECT id, claim_id, depends_on_id, strength, reasoning, created_at
		 FROM dependencies WHERE claim_id = ? ORDER BY created_at`, claimID)
	if err != nil {
		return nil, fmt.Errorf("list dependencies by claim: %w", err)
	}
	defer rows.Close()
	return scanDependencies(rows)
}

func (s *Store) ListDependents(claimID string) ([]model.Dependency, error) {
	rows, err := s.db.Query(
		`SELECT id, claim_id, depends_on_id, strength, reasoning, created_at
		 FROM dependencies WHERE depends_on_id = ? ORDER BY created_at`, claimID)
	if err != nil {
		return nil, fmt.Errorf("list dependents: %w", err)
	}
	defer rows.Close()
	return scanDependencies(rows)
}

func (s *Store) ListAllDependencies() ([]model.Dependency, error) {
	rows, err := s.db.Query(
		`SELECT id, claim_id, depends_on_id, strength, reasoning, created_at FROM dependencies ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list all dependencies: %w", err)
	}
	defer rows.Close()
	return scanDependencies(rows)
}

// HasCycle checks if adding a dependency from claimID -> dependsOnID would create a cycle.
func (s *Store) HasCycle(claimID, dependsOnID string) (bool, error) {
	// Walk forward from dependsOnID. If we reach claimID, it's a cycle.
	visited := map[string]bool{}
	queue := []string{dependsOnID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == claimID {
			return true, nil
		}
		if visited[cur] {
			continue
		}
		visited[cur] = true
		deps, err := s.ListDependenciesByClaim(cur)
		if err != nil {
			return false, fmt.Errorf("cycle check: %w", err)
		}
		for _, d := range deps {
			queue = append(queue, d.DependsOnID)
		}
	}
	return false, nil
}

func scanDependencies(rows *sql.Rows) ([]model.Dependency, error) {
	var deps []model.Dependency
	for rows.Next() {
		var d model.Dependency
		if err := rows.Scan(&d.ID, &d.ClaimID, &d.DependsOnID, &d.Strength, &d.Reasoning, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan dependency: %w", err)
		}
		deps = append(deps, d)
	}
	return deps, rows.Err()
}

// --- API Tokens ---

func (s *Store) CreateAPIToken(t *model.APIToken) error {
	_, err := s.db.Exec(
		`INSERT INTO api_tokens (id, agent_id, token_hash, role, expires_at) VALUES (?, ?, ?, ?, ?)`,
		t.ID, t.AgentID, t.TokenHash, t.Role, t.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("create api token: %w", err)
	}
	return nil
}

func (s *Store) RevokeAPIToken(id string) error {
	_, err := s.db.Exec(`UPDATE api_tokens SET revoked_at = ? WHERE id = ?`, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("revoke api token: %w", err)
	}
	return nil
}

func (s *Store) RevokeAgentTokens(agentID string) error {
	_, err := s.db.Exec(`UPDATE api_tokens SET revoked_at = ? WHERE agent_id = ? AND revoked_at IS NULL`, time.Now().UTC(), agentID)
	if err != nil {
		return fmt.Errorf("revoke agent tokens: %w", err)
	}
	return nil
}

func (s *Store) GetAPITokenByHash(hash string) (*model.APIToken, error) {
	t := &model.APIToken{}
	err := s.db.QueryRow(
		`SELECT id, agent_id, token_hash, role, expires_at, revoked_at, created_at
		 FROM api_tokens WHERE token_hash = ?`, hash,
	).Scan(&t.ID, &t.AgentID, &t.TokenHash, &t.Role, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get api token: %w", err)
	}
	return t, nil
}

// --- Epochs ---

func (s *Store) CreateEpoch() (*model.Epoch, error) {
	result, err := s.db.Exec(`INSERT INTO epochs DEFAULT VALUES`)
	if err != nil {
		return nil, fmt.Errorf("create epoch: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get epoch id: %w", err)
	}
	e := &model.Epoch{ID: int(id), StartedAt: time.Now().UTC()}
	return e, nil
}

func (s *Store) CompleteEpoch(id int, accIter, contIter int, accDelta, contDelta float64) error {
	_, err := s.db.Exec(
		`UPDATE epochs SET completed_at = ?, accuracy_iterations = ?, contribution_iterations = ?, accuracy_delta = ?, contribution_delta = ? WHERE id = ?`,
		time.Now().UTC(), accIter, contIter, accDelta, contDelta, id,
	)
	if err != nil {
		return fmt.Errorf("complete epoch: %w", err)
	}
	return nil
}

func (s *Store) GetLatestEpoch() (*model.Epoch, error) {
	e := &model.Epoch{}
	err := s.db.QueryRow(
		`SELECT id, started_at, completed_at, accuracy_iterations, contribution_iterations, accuracy_delta, contribution_delta
		 FROM epochs ORDER BY id DESC LIMIT 1`,
	).Scan(&e.ID, &e.StartedAt, &e.CompletedAt, &e.AccuracyIterations, &e.ContributionIterations, &e.AccuracyDelta, &e.ContributionDelta)
	if err != nil {
		return nil, fmt.Errorf("get latest epoch: %w", err)
	}
	return e, nil
}

func (s *Store) CountEpochs() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM epochs`).Scan(&count)
	return count, err
}

// --- Leaderboard / Discovery ---

func (s *Store) TopAgentsByWeight(limit int) ([]model.Agent, error) {
	rows, err := s.db.Query(
		`SELECT id, name, accuracy, contribution, weight, metadata, created_at
		 FROM agents ORDER BY weight DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("top agents: %w", err)
	}
	defer rows.Close()
	return scanAgents(rows)
}

func (s *Store) MostContestedClaims(limit int) ([]model.Claim, error) {
	rows, err := s.db.Query(
		`SELECT id, proposition, embedding, groundedness, effective_groundedness, contestation, status,
		        adjudicated_value, adjudicated_at, adjudicated_by, adjudication_reasoning, parent_claim_id,
		        created_at, computed_at
		 FROM claims WHERE status != 'adjudicated' ORDER BY contestation DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("most contested: %w", err)
	}
	defer rows.Close()
	return scanClaims(rows)
}

// FrontierClaims returns claims with high contestation and high dependency fan-out.
func (s *Store) FrontierClaims(limit int) ([]model.Claim, error) {
	rows, err := s.db.Query(
		`SELECT c.id, c.proposition, c.embedding, c.groundedness, c.effective_groundedness, c.contestation, c.status,
		        c.adjudicated_value, c.adjudicated_at, c.adjudicated_by, c.adjudication_reasoning, c.parent_claim_id,
		        c.created_at, c.computed_at
		 FROM claims c
		 LEFT JOIN dependencies d ON d.depends_on_id = c.id
		 WHERE c.status != 'adjudicated'
		 GROUP BY c.id
		 ORDER BY c.contestation * (1 + COUNT(d.id)) DESC
		 LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("frontier claims: %w", err)
	}
	defer rows.Close()
	return scanClaims(rows)
}

func (s *Store) ListEpochs() ([]model.Epoch, error) {
	rows, err := s.db.Query(
		`SELECT id, started_at, completed_at, accuracy_iterations, contribution_iterations, accuracy_delta, contribution_delta
		 FROM epochs ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list epochs: %w", err)
	}
	defer rows.Close()
	var epochs []model.Epoch
	for rows.Next() {
		var e model.Epoch
		if err := rows.Scan(&e.ID, &e.StartedAt, &e.CompletedAt, &e.AccuracyIterations, &e.ContributionIterations, &e.AccuracyDelta, &e.ContributionDelta); err != nil {
			return nil, fmt.Errorf("scan epoch: %w", err)
		}
		epochs = append(epochs, e)
	}
	return epochs, rows.Err()
}

// --- ID Generation ---

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
