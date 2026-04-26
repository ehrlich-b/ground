package db

import (
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"log"
	"math"
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

func (s *Store) Close() error          { return s.db.Close() }
func (s *Store) DB() *sql.DB           { return s.db }

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

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

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
		`INSERT INTO agents (id, name, role, reliability, productivity, metadata) VALUES (?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.Role, a.Reliability, a.Productivity, a.Metadata,
	)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}
	return nil
}

func (s *Store) GetAgent(id string) (*model.Agent, error) {
	a := &model.Agent{}
	err := s.db.QueryRow(
		`SELECT id, name, role, reliability, productivity, metadata, created_at FROM agents WHERE id = ?`, id,
	).Scan(&a.ID, &a.Name, &a.Role, &a.Reliability, &a.Productivity, &a.Metadata, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	return a, nil
}

func (s *Store) GetAgentByName(name string) (*model.Agent, error) {
	a := &model.Agent{}
	err := s.db.QueryRow(
		`SELECT id, name, role, reliability, productivity, metadata, created_at FROM agents WHERE name = ?`, name,
	).Scan(&a.ID, &a.Name, &a.Role, &a.Reliability, &a.Productivity, &a.Metadata, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get agent by name: %w", err)
	}
	return a, nil
}

func (s *Store) ListAgents() ([]model.Agent, error) {
	rows, err := s.db.Query(
		`SELECT id, name, role, reliability, productivity, metadata, created_at FROM agents ORDER BY reliability DESC`)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()
	return scanAgents(rows)
}

func (s *Store) UpdateAgentScores(id string, reliability, productivity float64) error {
	_, err := s.db.Exec(
		`UPDATE agents SET reliability = ?, productivity = ? WHERE id = ?`,
		reliability, productivity, id,
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

func (s *Store) TopAgentsByReliability(limit int) ([]model.Agent, error) {
	rows, err := s.db.Query(
		`SELECT id, name, role, reliability, productivity, metadata, created_at
		 FROM agents WHERE role != 'observer' ORDER BY reliability DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("top agents: %w", err)
	}
	defer rows.Close()
	return scanAgents(rows)
}

func scanAgents(rows *sql.Rows) ([]model.Agent, error) {
	var agents []model.Agent
	for rows.Next() {
		var a model.Agent
		if err := rows.Scan(&a.ID, &a.Name, &a.Role, &a.Reliability, &a.Productivity, &a.Metadata, &a.CreatedAt); err != nil {
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

// --- Sources ---

func (s *Store) CreateSource(src *model.Source) error {
	_, err := s.db.Exec(
		`INSERT INTO sources (id, url, content_hash, body_blob_id, fetched_at, type, title, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		src.ID, src.URL, src.ContentHash, src.BodyBlobID, src.FetchedAt, src.Type, src.Title, src.Metadata,
	)
	if err != nil {
		return fmt.Errorf("create source: %w", err)
	}
	return nil
}

func (s *Store) GetSource(id string) (*model.Source, error) {
	src := &model.Source{}
	err := s.db.QueryRow(
		`SELECT id, url, content_hash, body_blob_id, fetched_at, type, title, metadata, created_at
		 FROM sources WHERE id = ?`, id,
	).Scan(&src.ID, &src.URL, &src.ContentHash, &src.BodyBlobID, &src.FetchedAt, &src.Type, &src.Title, &src.Metadata, &src.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get source: %w", err)
	}
	return src, nil
}

func (s *Store) GetSourceByContentHash(hash string) (*model.Source, error) {
	src := &model.Source{}
	err := s.db.QueryRow(
		`SELECT id, url, content_hash, body_blob_id, fetched_at, type, title, metadata, created_at
		 FROM sources WHERE content_hash = ?`, hash,
	).Scan(&src.ID, &src.URL, &src.ContentHash, &src.BodyBlobID, &src.FetchedAt, &src.Type, &src.Title, &src.Metadata, &src.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get source by hash: %w", err)
	}
	return src, nil
}

func (s *Store) GetSourceByURL(url string) (*model.Source, error) {
	src := &model.Source{}
	err := s.db.QueryRow(
		`SELECT id, url, content_hash, body_blob_id, fetched_at, type, title, metadata, created_at
		 FROM sources WHERE url = ? ORDER BY fetched_at DESC LIMIT 1`, url,
	).Scan(&src.ID, &src.URL, &src.ContentHash, &src.BodyBlobID, &src.FetchedAt, &src.Type, &src.Title, &src.Metadata, &src.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get source by url: %w", err)
	}
	return src, nil
}

func (s *Store) UpdateSourceFetched(id, contentHash, blobID string, fetchedAt time.Time) error {
	_, err := s.db.Exec(
		`UPDATE sources SET content_hash = ?, body_blob_id = ?, fetched_at = ? WHERE id = ?`,
		contentHash, blobID, fetchedAt, id,
	)
	if err != nil {
		return fmt.Errorf("update source fetched: %w", err)
	}
	return nil
}

func (s *Store) ListSources(limit, offset int) ([]model.Source, error) {
	rows, err := s.db.Query(
		`SELECT id, url, content_hash, body_blob_id, fetched_at, type, title, metadata, created_at
		 FROM sources ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	defer rows.Close()
	return scanSources(rows)
}

func (s *Store) ListAllSources() ([]model.Source, error) {
	rows, err := s.db.Query(
		`SELECT id, url, content_hash, body_blob_id, fetched_at, type, title, metadata, created_at FROM sources`)
	if err != nil {
		return nil, fmt.Errorf("list all sources: %w", err)
	}
	defer rows.Close()
	return scanSources(rows)
}

func (s *Store) CountSources() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sources`).Scan(&count)
	return count, err
}

func scanSources(rows *sql.Rows) ([]model.Source, error) {
	var srcs []model.Source
	for rows.Next() {
		var src model.Source
		if err := rows.Scan(&src.ID, &src.URL, &src.ContentHash, &src.BodyBlobID, &src.FetchedAt, &src.Type, &src.Title, &src.Metadata, &src.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan source: %w", err)
		}
		srcs = append(srcs, src)
	}
	return srcs, rows.Err()
}

// --- Source Anchors ---

func (s *Store) UpsertSourceAnchor(a *model.SourceAnchor) error {
	_, err := s.db.Exec(
		`INSERT INTO source_anchors (source_id, tier, credibility, set_by, reasoning)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(source_id) DO UPDATE SET
		     tier = excluded.tier,
		     credibility = excluded.credibility,
		     set_by = excluded.set_by,
		     reasoning = excluded.reasoning,
		     set_at = CURRENT_TIMESTAMP`,
		a.SourceID, a.Tier, a.Credibility, a.SetBy, a.Reasoning,
	)
	if err != nil {
		return fmt.Errorf("upsert source anchor: %w", err)
	}
	return nil
}

func (s *Store) GetSourceAnchor(sourceID string) (*model.SourceAnchor, error) {
	a := &model.SourceAnchor{}
	err := s.db.QueryRow(
		`SELECT source_id, tier, credibility, set_by, reasoning, set_at FROM source_anchors WHERE source_id = ?`, sourceID,
	).Scan(&a.SourceID, &a.Tier, &a.Credibility, &a.SetBy, &a.Reasoning, &a.SetAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get source anchor: %w", err)
	}
	return a, nil
}

func (s *Store) ListSourceAnchors() ([]model.SourceAnchor, error) {
	rows, err := s.db.Query(`SELECT source_id, tier, credibility, set_by, reasoning, set_at FROM source_anchors ORDER BY tier, credibility DESC`)
	if err != nil {
		return nil, fmt.Errorf("list source anchors: %w", err)
	}
	defer rows.Close()
	var anchors []model.SourceAnchor
	for rows.Next() {
		var a model.SourceAnchor
		if err := rows.Scan(&a.SourceID, &a.Tier, &a.Credibility, &a.SetBy, &a.Reasoning, &a.SetAt); err != nil {
			return nil, fmt.Errorf("scan source anchor: %w", err)
		}
		anchors = append(anchors, a)
	}
	return anchors, rows.Err()
}

func (s *Store) DeleteSourceAnchor(sourceID string) error {
	_, err := s.db.Exec(`DELETE FROM source_anchors WHERE source_id = ?`, sourceID)
	if err != nil {
		return fmt.Errorf("delete source anchor: %w", err)
	}
	return nil
}

// --- Source Tags ---

func (s *Store) AddSourceTag(sourceID, tag string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO source_tags (source_id, tag) VALUES (?, ?)`, sourceID, tag)
	if err != nil {
		return fmt.Errorf("add source tag: %w", err)
	}
	return nil
}

func (s *Store) RemoveSourceTag(sourceID, tag string) error {
	_, err := s.db.Exec(`DELETE FROM source_tags WHERE source_id = ? AND tag = ?`, sourceID, tag)
	if err != nil {
		return fmt.Errorf("remove source tag: %w", err)
	}
	return nil
}

func (s *Store) ListSourceTags(sourceID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT tag FROM source_tags WHERE source_id = ? ORDER BY tag`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("list source tags: %w", err)
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func (s *Store) ListAllSourceTags() (map[string][]string, error) {
	rows, err := s.db.Query(`SELECT source_id, tag FROM source_tags`)
	if err != nil {
		return nil, fmt.Errorf("list all source tags: %w", err)
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var src, tag string
		if err := rows.Scan(&src, &tag); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out[src] = append(out[src], tag)
	}
	return out, rows.Err()
}

// --- Source Credibility ---

func (s *Store) UpsertSourceCredibility(sc *model.SourceCredibility) error {
	_, err := s.db.Exec(
		`INSERT INTO source_credibility (source_id, epoch_id, value, components)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(source_id, epoch_id) DO UPDATE SET value = excluded.value, components = excluded.components`,
		sc.SourceID, sc.EpochID, sc.Value, sc.Components,
	)
	if err != nil {
		return fmt.Errorf("upsert source credibility: %w", err)
	}
	return nil
}

func (s *Store) LatestSourceCredibility() (map[string]float64, error) {
	rows, err := s.db.Query(
		`SELECT sc.source_id, sc.value
		 FROM source_credibility sc
		 INNER JOIN (SELECT source_id, MAX(epoch_id) AS me FROM source_credibility GROUP BY source_id) m
		     ON sc.source_id = m.source_id AND sc.epoch_id = m.me`)
	if err != nil {
		return nil, fmt.Errorf("latest source credibility: %w", err)
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var id string
		var v float64
		if err := rows.Scan(&id, &v); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out[id] = v
	}
	return out, rows.Err()
}

// --- Source Citation Edges ---

func (s *Store) AddSourceCitationEdge(from, to string, locator *string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO source_citation_edges (from_source_id, to_source_id, locator) VALUES (?, ?, ?)`,
		from, to, locator,
	)
	if err != nil {
		return fmt.Errorf("add source citation edge: %w", err)
	}
	return nil
}

func (s *Store) ListSourceCitationEdges() ([]model.SourceCitationEdge, error) {
	rows, err := s.db.Query(`SELECT from_source_id, to_source_id, locator, extracted_at FROM source_citation_edges`)
	if err != nil {
		return nil, fmt.Errorf("list source citation edges: %w", err)
	}
	defer rows.Close()
	var edges []model.SourceCitationEdge
	for rows.Next() {
		var e model.SourceCitationEdge
		if err := rows.Scan(&e.FromSourceID, &e.ToSourceID, &e.Locator, &e.ExtractedAt); err != nil {
			return nil, fmt.Errorf("scan source edge: %w", err)
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// --- Claims ---

func (s *Store) CreateClaim(c *model.Claim) error {
	_, err := s.db.Exec(
		`INSERT INTO claims (id, proposition, embedding, groundedness, effective_groundedness, contestation, status,
		                     adjudicated_value, adjudicated_at, adjudicated_by, adjudication_reasoning)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Proposition, c.Embedding, c.Groundedness, c.EffectiveGroundedness, c.Contestation, c.Status,
		c.AdjudicatedValue, c.AdjudicatedAt, c.AdjudicatedBy, c.AdjudicationReasoning,
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
		        adjudicated_value, adjudicated_at, adjudicated_by, adjudication_reasoning, created_at, computed_at
		 FROM claims WHERE id = ?`, id,
	).Scan(&c.ID, &c.Proposition, &c.Embedding, &c.Groundedness, &c.EffectiveGroundedness, &c.Contestation, &c.Status,
		&c.AdjudicatedValue, &c.AdjudicatedAt, &c.AdjudicatedBy, &c.AdjudicationReasoning, &c.CreatedAt, &c.ComputedAt)
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
			        adjudicated_value, adjudicated_at, adjudicated_by, adjudication_reasoning, created_at, computed_at
			 FROM claims WHERE status = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`, status, limit, offset)
	} else {
		rows, err = s.db.Query(
			`SELECT id, proposition, embedding, groundedness, effective_groundedness, contestation, status,
			        adjudicated_value, adjudicated_at, adjudicated_by, adjudication_reasoning, created_at, computed_at
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
		        adjudicated_value, adjudicated_at, adjudicated_by, adjudication_reasoning, created_at, computed_at
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
	_, err := s.db.Exec(
		`UPDATE claims SET adjudicated_value = ?, adjudicated_at = ?, adjudicated_by = ?, adjudication_reasoning = ?,
		        groundedness = ?, effective_groundedness = ?, status = 'adjudicated' WHERE id = ?`,
		value, now, by, reasoning, value, value, id,
	)
	if err != nil {
		return fmt.Errorf("adjudicate claim: %w", err)
	}
	return nil
}

func (s *Store) ListGroundedClaims(limit int) ([]model.Claim, error) {
	rows, err := s.db.Query(
		`SELECT id, proposition, embedding, groundedness, effective_groundedness, contestation, status,
		        adjudicated_value, adjudicated_at, adjudicated_by, adjudication_reasoning, created_at, computed_at
		 FROM claims WHERE status IN ('grounded', 'adjudicated')
		 ORDER BY COALESCE(computed_at, created_at) DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list grounded claims: %w", err)
	}
	defer rows.Close()
	return scanClaims(rows)
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
	counts := map[string]int{}
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
			&c.AdjudicationReasoning, &c.CreatedAt, &c.ComputedAt); err != nil {
			return nil, fmt.Errorf("scan claim: %w", err)
		}
		claims = append(claims, c)
	}
	return claims, rows.Err()
}

func (s *Store) ListClaimsByTopic(topicSlug string, limit int) ([]model.Claim, error) {
	topic, err := s.GetTopicBySlug(topicSlug)
	if err != nil {
		return nil, fmt.Errorf("get topic for claims: %w", err)
	}
	if len(topic.Embedding) == 0 {
		return nil, fmt.Errorf("topic %s has no embedding", topicSlug)
	}

	rows, err := s.db.Query(
		`SELECT id, proposition, embedding, groundedness, effective_groundedness, contestation, status,
		        adjudicated_value, adjudicated_at, adjudicated_by, adjudication_reasoning, created_at, computed_at
		 FROM claims WHERE embedding IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("list claims for topic: %w", err)
	}
	defer rows.Close()

	allClaims, err := scanClaims(rows)
	if err != nil {
		return nil, err
	}

	type scored struct {
		claim model.Claim
		sim   float64
	}
	topicVec := unmarshalVec(topic.Embedding)
	var matches []scored
	for _, c := range allClaims {
		if len(c.Embedding) == 0 {
			continue
		}
		sim := cosine(topicVec, unmarshalVec(c.Embedding))
		if sim > 0.3 {
			matches = append(matches, scored{claim: c, sim: sim})
		}
	}

	sort.Slice(matches, func(i, j int) bool { return matches[i].sim > matches[j].sim })
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	result := make([]model.Claim, len(matches))
	for i, m := range matches {
		result[i] = m.claim
	}
	return result, nil
}

func cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai, bi := float64(a[i]), float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func unmarshalVec(b []byte) []float32 {
	if len(b) == 0 {
		return nil
	}
	n := len(b) / 4
	v := make([]float32, n)
	for i := range n {
		bits := uint32(b[i*4]) | uint32(b[i*4+1])<<8 | uint32(b[i*4+2])<<16 | uint32(b[i*4+3])<<24
		v[i] = math.Float32frombits(bits)
	}
	return v
}

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

// --- Citations ---

func (s *Store) CreateCitation(c *model.Citation) error {
	_, err := s.db.Exec(
		`INSERT INTO citations (id, claim_id, source_id, verbatim_quote, locator, polarity, reasoning,
		                        extractor_id, audit_factor, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.ClaimID, c.SourceID, c.VerbatimQuote, c.Locator, c.Polarity, c.Reasoning,
		c.ExtractorID, c.AuditFactor, c.Status,
	)
	if err != nil {
		return fmt.Errorf("create citation: %w", err)
	}
	return nil
}

func (s *Store) GetCitation(id string) (*model.Citation, error) {
	c := &model.Citation{}
	err := s.db.QueryRow(
		`SELECT id, claim_id, source_id, verbatim_quote, locator, polarity, reasoning,
		        extractor_id, audit_factor, status, created_at
		 FROM citations WHERE id = ?`, id,
	).Scan(&c.ID, &c.ClaimID, &c.SourceID, &c.VerbatimQuote, &c.Locator, &c.Polarity, &c.Reasoning,
		&c.ExtractorID, &c.AuditFactor, &c.Status, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get citation: %w", err)
	}
	return c, nil
}

func (s *Store) ListCitationsByClaim(claimID string) ([]model.Citation, error) {
	rows, err := s.db.Query(
		`SELECT id, claim_id, source_id, verbatim_quote, locator, polarity, reasoning,
		        extractor_id, audit_factor, status, created_at
		 FROM citations WHERE claim_id = ? ORDER BY created_at`, claimID)
	if err != nil {
		return nil, fmt.Errorf("list citations by claim: %w", err)
	}
	defer rows.Close()
	return scanCitations(rows)
}

func (s *Store) ListCitationsBySource(sourceID string) ([]model.Citation, error) {
	rows, err := s.db.Query(
		`SELECT id, claim_id, source_id, verbatim_quote, locator, polarity, reasoning,
		        extractor_id, audit_factor, status, created_at
		 FROM citations WHERE source_id = ? ORDER BY created_at DESC`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("list citations by source: %w", err)
	}
	defer rows.Close()
	return scanCitations(rows)
}

func (s *Store) ListCitationsByExtractor(extractorID string, limit, offset int) ([]model.Citation, error) {
	rows, err := s.db.Query(
		`SELECT id, claim_id, source_id, verbatim_quote, locator, polarity, reasoning,
		        extractor_id, audit_factor, status, created_at
		 FROM citations WHERE extractor_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		extractorID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list citations by extractor: %w", err)
	}
	defer rows.Close()
	return scanCitations(rows)
}

func (s *Store) ListAllCitations() ([]model.Citation, error) {
	rows, err := s.db.Query(
		`SELECT id, claim_id, source_id, verbatim_quote, locator, polarity, reasoning,
		        extractor_id, audit_factor, status, created_at FROM citations ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list all citations: %w", err)
	}
	defer rows.Close()
	return scanCitations(rows)
}

func (s *Store) UpdateCitationAuditFactor(id string, factor float64) error {
	_, err := s.db.Exec(`UPDATE citations SET audit_factor = ? WHERE id = ?`, factor, id)
	if err != nil {
		return fmt.Errorf("update citation audit factor: %w", err)
	}
	return nil
}

func (s *Store) UpdateCitationStatus(id, status string) error {
	_, err := s.db.Exec(`UPDATE citations SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return fmt.Errorf("update citation status: %w", err)
	}
	return nil
}

func (s *Store) CountCitations() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM citations`).Scan(&count)
	return count, err
}

// CountCitationsByExtractorOnDay returns the citation count for an extractor in the past 24 hours,
// used for rate limiting.
func (s *Store) CountCitationsByExtractorOnDay(extractorID string) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM citations WHERE extractor_id = ? AND created_at >= datetime('now','-1 day')`,
		extractorID,
	).Scan(&count)
	return count, err
}

func scanCitations(rows *sql.Rows) ([]model.Citation, error) {
	var citations []model.Citation
	for rows.Next() {
		var c model.Citation
		if err := rows.Scan(&c.ID, &c.ClaimID, &c.SourceID, &c.VerbatimQuote, &c.Locator, &c.Polarity, &c.Reasoning,
			&c.ExtractorID, &c.AuditFactor, &c.Status, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan citation: %w", err)
		}
		citations = append(citations, c)
	}
	return citations, rows.Err()
}

// --- Audits ---

func (s *Store) CreateAudit(a *model.Audit) error {
	_, err := s.db.Exec(
		`INSERT INTO audits (id, citation_id, auditor_id, mechanical, semantic, verdict, reasoning)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.CitationID, a.AuditorID, a.Mechanical, a.Semantic, a.Verdict, a.Reasoning,
	)
	if err != nil {
		return fmt.Errorf("create audit: %w", err)
	}
	return nil
}

func (s *Store) GetAudit(id string) (*model.Audit, error) {
	a := &model.Audit{}
	err := s.db.QueryRow(
		`SELECT id, citation_id, auditor_id, mechanical, semantic, verdict, reasoning, created_at
		 FROM audits WHERE id = ?`, id,
	).Scan(&a.ID, &a.CitationID, &a.AuditorID, &a.Mechanical, &a.Semantic, &a.Verdict, &a.Reasoning, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get audit: %w", err)
	}
	return a, nil
}

func (s *Store) ListAuditsByCitation(citationID string) ([]model.Audit, error) {
	rows, err := s.db.Query(
		`SELECT id, citation_id, auditor_id, mechanical, semantic, verdict, reasoning, created_at
		 FROM audits WHERE citation_id = ? ORDER BY created_at`, citationID)
	if err != nil {
		return nil, fmt.Errorf("list audits by citation: %w", err)
	}
	defer rows.Close()
	return scanAudits(rows)
}

func (s *Store) ListAuditsByAuditor(auditorID string, limit, offset int) ([]model.Audit, error) {
	rows, err := s.db.Query(
		`SELECT id, citation_id, auditor_id, mechanical, semantic, verdict, reasoning, created_at
		 FROM audits WHERE auditor_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		auditorID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list audits by auditor: %w", err)
	}
	defer rows.Close()
	return scanAudits(rows)
}

func (s *Store) ListAllAudits() ([]model.Audit, error) {
	rows, err := s.db.Query(
		`SELECT id, citation_id, auditor_id, mechanical, semantic, verdict, reasoning, created_at FROM audits`)
	if err != nil {
		return nil, fmt.Errorf("list all audits: %w", err)
	}
	defer rows.Close()
	return scanAudits(rows)
}

// CitationsNeedingAudit returns citations with fewer than minAudits audits, excluding any
// citations the requesting agent extracted.
func (s *Store) CitationsNeedingAudit(auditorID string, minAudits, limit int) ([]model.Citation, error) {
	rows, err := s.db.Query(
		`SELECT c.id, c.claim_id, c.source_id, c.verbatim_quote, c.locator, c.polarity, c.reasoning,
		        c.extractor_id, c.audit_factor, c.status, c.created_at
		 FROM citations c
		 LEFT JOIN audits a ON a.citation_id = c.id AND a.auditor_id = ?
		 WHERE c.extractor_id != ? AND c.status = 'active' AND a.id IS NULL
		 GROUP BY c.id
		 HAVING (SELECT COUNT(*) FROM audits WHERE citation_id = c.id) < ?
		 ORDER BY c.created_at DESC
		 LIMIT ?`,
		auditorID, auditorID, minAudits, limit)
	if err != nil {
		return nil, fmt.Errorf("citations needing audit: %w", err)
	}
	defer rows.Close()
	return scanCitations(rows)
}

func (s *Store) CountAuditsByAuditorOnDay(auditorID string) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM audits WHERE auditor_id = ? AND created_at >= datetime('now','-1 day')`,
		auditorID,
	).Scan(&count)
	return count, err
}

func scanAudits(rows *sql.Rows) ([]model.Audit, error) {
	var audits []model.Audit
	for rows.Next() {
		var a model.Audit
		if err := rows.Scan(&a.ID, &a.CitationID, &a.AuditorID, &a.Mechanical, &a.Semantic, &a.Verdict, &a.Reasoning, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit: %w", err)
		}
		audits = append(audits, a)
	}
	return audits, rows.Err()
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

func (s *Store) HasCycle(claimID, dependsOnID string) (bool, error) {
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

func (s *Store) GetDependencyByClaimPair(claimID, dependsOnID string) (*model.Dependency, error) {
	var d model.Dependency
	err := s.db.QueryRow(
		`SELECT id, claim_id, depends_on_id, strength, reasoning, created_at
		 FROM dependencies WHERE claim_id = ? AND depends_on_id = ?`,
		claimID, dependsOnID,
	).Scan(&d.ID, &d.ClaimID, &d.DependsOnID, &d.Strength, &d.Reasoning, &d.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get dependency by claim pair: %w", err)
	}
	return &d, nil
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

// --- Lenses ---

func (s *Store) CreateLens(l *model.Lens) error {
	pub := 0
	if l.Public {
		pub = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO lenses (id, slug, owner_id, parent_lens_id, description, public)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		l.ID, l.Slug, l.OwnerID, l.ParentLensID, l.Description, pub,
	)
	if err != nil {
		return fmt.Errorf("create lens: %w", err)
	}
	return nil
}

func (s *Store) GetLens(id string) (*model.Lens, error) {
	l := &model.Lens{}
	var pub int
	err := s.db.QueryRow(
		`SELECT id, slug, owner_id, parent_lens_id, description, public, created_at FROM lenses WHERE id = ?`, id,
	).Scan(&l.ID, &l.Slug, &l.OwnerID, &l.ParentLensID, &l.Description, &pub, &l.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get lens: %w", err)
	}
	l.Public = pub != 0
	return l, nil
}

func (s *Store) GetLensBySlug(slug string) (*model.Lens, error) {
	l := &model.Lens{}
	var pub int
	err := s.db.QueryRow(
		`SELECT id, slug, owner_id, parent_lens_id, description, public, created_at FROM lenses WHERE slug = ?`, slug,
	).Scan(&l.ID, &l.Slug, &l.OwnerID, &l.ParentLensID, &l.Description, &pub, &l.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get lens by slug: %w", err)
	}
	l.Public = pub != 0
	return l, nil
}

func (s *Store) ListLenses() ([]model.Lens, error) {
	rows, err := s.db.Query(
		`SELECT id, slug, owner_id, parent_lens_id, description, public, created_at FROM lenses ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list lenses: %w", err)
	}
	defer rows.Close()
	var lenses []model.Lens
	for rows.Next() {
		var l model.Lens
		var pub int
		if err := rows.Scan(&l.ID, &l.Slug, &l.OwnerID, &l.ParentLensID, &l.Description, &pub, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan lens: %w", err)
		}
		l.Public = pub != 0
		lenses = append(lenses, l)
	}
	return lenses, rows.Err()
}

// --- Lens Overrides ---

func (s *Store) UpsertLensOverride(o *model.LensOverride) error {
	_, err := s.db.Exec(
		`INSERT INTO lens_overrides (lens_id, source_id, mode, value)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(lens_id, source_id) DO UPDATE SET mode = excluded.mode, value = excluded.value`,
		o.LensID, o.SourceID, o.Mode, o.Value,
	)
	if err != nil {
		return fmt.Errorf("upsert lens override: %w", err)
	}
	return nil
}

func (s *Store) DeleteLensOverride(lensID, sourceID string) error {
	_, err := s.db.Exec(`DELETE FROM lens_overrides WHERE lens_id = ? AND source_id = ?`, lensID, sourceID)
	if err != nil {
		return fmt.Errorf("delete lens override: %w", err)
	}
	return nil
}

func (s *Store) ListLensOverrides(lensID string) ([]model.LensOverride, error) {
	rows, err := s.db.Query(`SELECT lens_id, source_id, mode, value FROM lens_overrides WHERE lens_id = ?`, lensID)
	if err != nil {
		return nil, fmt.Errorf("list lens overrides: %w", err)
	}
	defer rows.Close()
	var out []model.LensOverride
	for rows.Next() {
		var o model.LensOverride
		if err := rows.Scan(&o.LensID, &o.SourceID, &o.Mode, &o.Value); err != nil {
			return nil, fmt.Errorf("scan lens override: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) UpsertLensTagOverride(o *model.LensTagOverride) error {
	_, err := s.db.Exec(
		`INSERT INTO lens_tag_overrides (lens_id, tag, multiplier)
		 VALUES (?, ?, ?)
		 ON CONFLICT(lens_id, tag) DO UPDATE SET multiplier = excluded.multiplier`,
		o.LensID, o.Tag, o.Multiplier,
	)
	if err != nil {
		return fmt.Errorf("upsert lens tag override: %w", err)
	}
	return nil
}

func (s *Store) ListLensTagOverrides(lensID string) ([]model.LensTagOverride, error) {
	rows, err := s.db.Query(`SELECT lens_id, tag, multiplier FROM lens_tag_overrides WHERE lens_id = ?`, lensID)
	if err != nil {
		return nil, fmt.Errorf("list lens tag overrides: %w", err)
	}
	defer rows.Close()
	var out []model.LensTagOverride
	for rows.Next() {
		var o model.LensTagOverride
		if err := rows.Scan(&o.LensID, &o.Tag, &o.Multiplier); err != nil {
			return nil, fmt.Errorf("scan lens tag override: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
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
	return &model.Epoch{ID: int(id), StartedAt: time.Now().UTC()}, nil
}

func (s *Store) CompleteEpoch(id, srcIter, agentIter int, srcDelta, agentDelta float64) error {
	_, err := s.db.Exec(
		`UPDATE epochs SET completed_at = ?, source_iterations = ?, agent_iterations = ?, source_delta = ?, agent_delta = ? WHERE id = ?`,
		time.Now().UTC(), srcIter, agentIter, srcDelta, agentDelta, id,
	)
	if err != nil {
		return fmt.Errorf("complete epoch: %w", err)
	}
	return nil
}

func (s *Store) GetLatestEpoch() (*model.Epoch, error) {
	e := &model.Epoch{}
	err := s.db.QueryRow(
		`SELECT id, started_at, completed_at, source_iterations, agent_iterations, source_delta, agent_delta
		 FROM epochs ORDER BY id DESC LIMIT 1`,
	).Scan(&e.ID, &e.StartedAt, &e.CompletedAt, &e.SourceIterations, &e.AgentIterations, &e.SourceDelta, &e.AgentDelta)
	if err == sql.ErrNoRows {
		return nil, nil
	}
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

func (s *Store) ListEpochs() ([]model.Epoch, error) {
	rows, err := s.db.Query(
		`SELECT id, started_at, completed_at, source_iterations, agent_iterations, source_delta, agent_delta
		 FROM epochs ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list epochs: %w", err)
	}
	defer rows.Close()
	var epochs []model.Epoch
	for rows.Next() {
		var e model.Epoch
		if err := rows.Scan(&e.ID, &e.StartedAt, &e.CompletedAt, &e.SourceIterations, &e.AgentIterations, &e.SourceDelta, &e.AgentDelta); err != nil {
			return nil, fmt.Errorf("scan epoch: %w", err)
		}
		epochs = append(epochs, e)
	}
	return epochs, rows.Err()
}

// --- Discovery ---

func (s *Store) MostContestedClaims(limit int) ([]model.Claim, error) {
	rows, err := s.db.Query(
		`SELECT id, proposition, embedding, groundedness, effective_groundedness, contestation, status,
		        adjudicated_value, adjudicated_at, adjudicated_by, adjudication_reasoning, created_at, computed_at
		 FROM claims WHERE status != 'adjudicated' ORDER BY contestation DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("most contested: %w", err)
	}
	defer rows.Close()
	return scanClaims(rows)
}

func (s *Store) FrontierClaims(limit int) ([]model.Claim, error) {
	rows, err := s.db.Query(
		`SELECT c.id, c.proposition, c.embedding, c.groundedness, c.effective_groundedness, c.contestation, c.status,
		        c.adjudicated_value, c.adjudicated_at, c.adjudicated_by, c.adjudication_reasoning,
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

// --- IDs ---

// GenerateID returns a 16-byte hex ID; usable as primary key for any table.
func GenerateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// fall back to nanosecond clock; should never hit
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// HasSourceQuote runs the mechanical containment check against a body provided by the caller
// (the caller already loaded the blob). Returns true if the quote is a literal substring.
func HasSourceQuote(body, quote string) bool {
	return strings.Contains(body, quote)
}
