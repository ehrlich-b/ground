package db_test

import (
	"path/filepath"
	"testing"

	"github.com/ehrlich-b/ground/internal/db"
	"github.com/ehrlich-b/ground/internal/model"
)

func newStore(t *testing.T) *db.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestAgentCRUD(t *testing.T) {
	store := newStore(t)

	a := &model.Agent{
		ID:           "agent-a",
		Name:         "Alice",
		Role:         "extractor",
		Reliability:  0.5,
		Productivity: 0.0,
	}
	if err := store.CreateAgent(a); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := store.GetAgent("agent-a")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Role != "extractor" || got.Reliability != 0.5 {
		t.Errorf("roundtrip mismatch: %+v", got)
	}

	if err := store.UpdateAgentScores("agent-a", 0.9, 1.5); err != nil {
		t.Fatalf("update: %v", err)
	}
	got2, _ := store.GetAgent("agent-a")
	if got2.Reliability != 0.9 || got2.Productivity != 1.5 {
		t.Errorf("scores not updated: %+v", got2)
	}

	count, _ := store.CountAgents()
	if count != 1 {
		t.Errorf("expected 1 agent, got %d", count)
	}
}

func TestSourceContentHashDedup(t *testing.T) {
	store := newStore(t)
	src1 := &model.Source{
		ID:          "s1",
		URL:         "https://example.com/a",
		ContentHash: "abc123",
		BodyBlobID:  "abc123",
		FetchedAt:   nowUTC(),
		Type:        "html",
	}
	if err := store.CreateSource(src1); err != nil {
		t.Fatalf("create source: %v", err)
	}

	// Same content hash, different URL -> should fail UNIQUE.
	src2 := *src1
	src2.ID = "s2"
	src2.URL = "https://example.com/b"
	if err := store.CreateSource(&src2); err == nil {
		t.Errorf("expected dedup failure on same content_hash")
	}

	// Lookup by hash returns existing.
	got, err := store.GetSourceByContentHash("abc123")
	if err != nil || got == nil || got.ID != "s1" {
		t.Errorf("dedup lookup mismatch: %+v err=%v", got, err)
	}
}

func TestSourceAnchorAndTags(t *testing.T) {
	store := newStore(t)
	src := &model.Source{
		ID: "s1", URL: "https://nature.com/x", ContentHash: "h1",
		BodyBlobID: "h1", FetchedAt: nowUTC(), Type: "html",
	}
	store.CreateSource(src)

	anchor := &model.SourceAnchor{
		SourceID: "s1", Tier: 1, Credibility: 0.92,
		SetBy: "admin",
	}
	if err := store.UpsertSourceAnchor(anchor); err != nil {
		t.Fatalf("anchor: %v", err)
	}
	got, _ := store.GetSourceAnchor("s1")
	if got == nil || got.Tier != 1 || got.Credibility != 0.92 {
		t.Errorf("anchor mismatch: %+v", got)
	}

	store.AddSourceTag("s1", "peer-reviewed")
	store.AddSourceTag("s1", "tier-1-journal")
	tags, _ := store.ListSourceTags("s1")
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %v", tags)
	}
}

func TestCitationsAndAudits(t *testing.T) {
	store := newStore(t)
	store.CreateAgent(&model.Agent{ID: "ext", Name: "Ext", Role: "extractor", Reliability: 0.5})
	store.CreateAgent(&model.Agent{ID: "aud", Name: "Aud", Role: "auditor", Reliability: 0.5})
	store.CreateSource(&model.Source{
		ID: "src", URL: "https://x.com", ContentHash: "h",
		BodyBlobID: "h", FetchedAt: nowUTC(), Type: "html",
	})
	store.CreateClaim(&model.Claim{ID: "c1", Proposition: "X is true", Status: "active"})

	cit := &model.Citation{
		ID:            "cit1",
		ClaimID:       "c1",
		SourceID:      "src",
		VerbatimQuote: "X is true",
		Polarity:      "supports",
		ExtractorID:   "ext",
		AuditFactor:   1.0,
		Status:        "active",
	}
	if err := store.CreateCitation(cit); err != nil {
		t.Fatalf("create citation: %v", err)
	}

	// Auditor != extractor allowed.
	audit := &model.Audit{
		ID:         "a1",
		CitationID: "cit1",
		AuditorID:  "aud",
		Mechanical: "pass",
		Semantic:   "confirm",
		Verdict:    "uphold",
	}
	if err := store.CreateAudit(audit); err != nil {
		t.Fatalf("create audit: %v", err)
	}

	audits, _ := store.ListAuditsByCitation("cit1")
	if len(audits) != 1 || audits[0].Verdict != "uphold" {
		t.Errorf("audit mismatch: %+v", audits)
	}
}

func TestCitationsNeedingAuditExcludesSelf(t *testing.T) {
	store := newStore(t)
	store.CreateAgent(&model.Agent{ID: "ext", Name: "E", Role: "extractor", Reliability: 0.5})
	store.CreateAgent(&model.Agent{ID: "aud", Name: "A", Role: "auditor", Reliability: 0.5})
	store.CreateSource(&model.Source{
		ID: "s", URL: "u", ContentHash: "h", BodyBlobID: "h", FetchedAt: nowUTC(), Type: "html",
	})
	store.CreateClaim(&model.Claim{ID: "c1", Proposition: "x", Status: "active"})
	store.CreateClaim(&model.Claim{ID: "c2", Proposition: "y", Status: "active"})
	store.CreateCitation(&model.Citation{
		ID: "cit1", ClaimID: "c1", SourceID: "s", VerbatimQuote: "x",
		Polarity: "supports", ExtractorID: "ext", AuditFactor: 1.0, Status: "active",
	})
	store.CreateCitation(&model.Citation{
		ID: "cit2", ClaimID: "c2", SourceID: "s", VerbatimQuote: "y",
		Polarity: "supports", ExtractorID: "aud", AuditFactor: 1.0, Status: "active",
	})

	// aud auditing — should see cit1 (not own), not cit2 (own).
	queue, err := store.CitationsNeedingAudit("aud", 3, 10)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	for _, q := range queue {
		if q.ExtractorID == "aud" {
			t.Errorf("queue contains own citation: %+v", q)
		}
	}
}

func TestDependencyCycleDetection(t *testing.T) {
	store := newStore(t)
	store.CreateClaim(&model.Claim{ID: "a", Proposition: "a", Status: "active"})
	store.CreateClaim(&model.Claim{ID: "b", Proposition: "b", Status: "active"})
	store.CreateClaim(&model.Claim{ID: "c", Proposition: "c", Status: "active"})

	// a -> b
	store.CreateDependency(&model.Dependency{ID: "d1", ClaimID: "a", DependsOnID: "b", Strength: 1.0})
	// b -> c
	store.CreateDependency(&model.Dependency{ID: "d2", ClaimID: "b", DependsOnID: "c", Strength: 1.0})
	// c -> a would close the cycle.
	cycle, _ := store.HasCycle("c", "a")
	if !cycle {
		t.Errorf("expected cycle a→b→c→a")
	}
	cycle, _ = store.HasCycle("a", "c")
	if cycle {
		t.Errorf("did not expect cycle for a→c")
	}
}

func TestLensOverridesAndTagOverrides(t *testing.T) {
	store := newStore(t)
	store.CreateAgent(&model.Agent{ID: "u1", Name: "u", Role: "both", Reliability: 0.5})
	store.CreateSource(&model.Source{
		ID: "s1", URL: "u", ContentHash: "h", BodyBlobID: "h", FetchedAt: nowUTC(), Type: "html",
	})

	owner := "u1"
	lens := &model.Lens{
		ID: "lens1", Slug: "primary-only", OwnerID: &owner, Public: true,
	}
	if err := store.CreateLens(lens); err != nil {
		t.Fatalf("create lens: %v", err)
	}

	if err := store.UpsertLensOverride(&model.LensOverride{
		LensID: "lens1", SourceID: "s1", Mode: "exclude", Value: 0,
	}); err != nil {
		t.Fatalf("upsert override: %v", err)
	}
	if err := store.UpsertLensTagOverride(&model.LensTagOverride{
		LensID: "lens1", Tag: "preprint", Multiplier: 0,
	}); err != nil {
		t.Fatalf("upsert tag override: %v", err)
	}

	overrides, _ := store.ListLensOverrides("lens1")
	if len(overrides) != 1 || overrides[0].Mode != "exclude" {
		t.Errorf("override mismatch: %+v", overrides)
	}
	tagOverrides, _ := store.ListLensTagOverrides("lens1")
	if len(tagOverrides) != 1 || tagOverrides[0].Multiplier != 0 {
		t.Errorf("tag override mismatch: %+v", tagOverrides)
	}
}

func TestHasSourceQuote(t *testing.T) {
	body := "We measure the heat dissipated during a logically irreversible memory erasure procedure and verify that it approaches the Landauer limit."
	if !db.HasSourceQuote(body, "approaches the Landauer limit") {
		t.Errorf("expected match")
	}
	if db.HasSourceQuote(body, "exceeds the Landauer limit") {
		t.Errorf("expected miss")
	}
	// Empty quote is technically a substring (always true) - documented behavior.
	if !db.HasSourceQuote(body, "") {
		t.Errorf("empty quote should be substring")
	}
}

func TestEpochRoundtrip(t *testing.T) {
	store := newStore(t)
	e, err := store.CreateEpoch()
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.CompleteEpoch(e.ID, 5, 3, 0.001, 0.0005); err != nil {
		t.Fatalf("complete: %v", err)
	}
	got, _ := store.GetLatestEpoch()
	if got == nil || got.ID != e.ID {
		t.Errorf("get latest mismatch")
	}
	if got.SourceIterations == nil || *got.SourceIterations != 5 {
		t.Errorf("source iterations not persisted")
	}
}
