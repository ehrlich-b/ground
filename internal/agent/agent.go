// Package agent provides helpers for registering Ground agents and seeding axioms.
//
// v2 dropped the 12-personality-EigenTrust seed orchestration. Phase 12 will rebuild
// search/extract/audit seed rounds against the v2 API. Until then, this package
// only exposes the lower-level helpers used by `ground bootstrap-axioms` and
// `ground seed-agent`.
package agent

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/ehrlich-b/ground/internal/api"
	"github.com/ehrlich-b/ground/internal/db"
	"github.com/ehrlich-b/ground/internal/model"
	"github.com/ehrlich-b/ground/internal/sources"
)

// Axiom represents a parsed axiom from FACTS.md.
type Axiom struct {
	Code        string
	Proposition string
	Basis       string
	Anchors     []string
	Value       float64 // 1.0 = true, 0.0 = false
	Citations   []AxiomCitation
}

// AxiomCitation is a verbatim-quote citation backing an axiom.
type AxiomCitation struct {
	URL           string
	VerbatimQuote string
	Polarity      string
	Reasoning     string
}

// citationLine matches the FACTS.md citation format:
//   - `URL` — *"VERBATIM QUOTE"* (polarity)
//
// The em-dash and en-dash are both accepted; the polarity in parens is optional
// (defaults to supports/contradicts based on the section). Lines that don't have
// both a backticked URL and a quoted span are placeholders ("ingestion target",
// cross-references) and are skipped silently.
var citationLine = regexp.MustCompile("`([^`]+)`[^*]*\\*\"([^\"]+)\"\\*(?:\\s*\\(([^)]+)\\))?")

// ParseAxioms parses FACTS.md content into structured axioms.
//
// Format expected:
//
//	### MATH-01: <title>
//	**Proposition**: ...
//	**Adjudication**: TRUE | FALSE
//	**Citations**:
//	- `URL` — *"VERBATIM QUOTE"* (supports)
//	- `URL` — *"VERBATIM QUOTE"* (contradicts)
//	**Topic anchors**: tag-a, tag-b
//
// All axioms under `## Adjudicated FALSE` default to Value=0. Citations whose
// polarity is omitted default to supports for TRUE axioms, contradicts for FALSE.
// Lines without a backticked URL and a quoted span are skipped (placeholders).
func ParseAxioms(content string) []Axiom {
	var axioms []Axiom
	lines := strings.Split(content, "\n")
	var current *Axiom
	inFalse := false

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "## Adjudicated FALSE" {
			inFalse = true
			continue
		}
		if strings.HasPrefix(line, "### ") {
			code := strings.TrimPrefix(line, "### ")
			if idx := strings.Index(code, ":"); idx > 0 {
				code = strings.TrimSpace(code[:idx])
			}
			ax := Axiom{Code: code, Value: 1.0}
			if inFalse {
				ax.Value = 0.0
			}
			axioms = append(axioms, ax)
			current = &axioms[len(axioms)-1]
			continue
		}
		if current == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "**Proposition**:"):
			current.Proposition = strings.TrimSpace(strings.TrimPrefix(line, "**Proposition**:"))
		case strings.HasPrefix(line, "**Basis**:"):
			current.Basis = strings.TrimSpace(strings.TrimPrefix(line, "**Basis**:"))
		case strings.HasPrefix(line, "**Topic anchors**:"), strings.HasPrefix(line, "**Anchors**:"):
			body := strings.TrimPrefix(line, "**Topic anchors**:")
			body = strings.TrimPrefix(body, "**Anchors**:")
			for _, a := range strings.Split(body, ",") {
				a = strings.TrimSpace(a)
				if a != "" {
					current.Anchors = append(current.Anchors, a)
				}
			}
		case strings.HasPrefix(line, "**Adjudication**:"), strings.HasPrefix(line, "**Adjudicated**:"):
			val := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "**Adjudication**:"), "**Adjudicated**:"))
			if strings.EqualFold(val, "FALSE") {
				current.Value = 0.0
			} else if strings.EqualFold(val, "TRUE") {
				current.Value = 1.0
			}
		default:
			if m := citationLine.FindStringSubmatch(line); m != nil {
				cit := AxiomCitation{
					URL:           strings.TrimSpace(m[1]),
					VerbatimQuote: strings.TrimSpace(m[2]),
				}
				polarity := strings.ToLower(strings.TrimSpace(m[3]))
				switch {
				case strings.HasPrefix(polarity, "supports"):
					cit.Polarity = "supports"
				case strings.HasPrefix(polarity, "contradicts"):
					cit.Polarity = "contradicts"
				case strings.HasPrefix(polarity, "qualifies"):
					cit.Polarity = "qualifies"
				default:
					if current.Value == 0.0 {
						cit.Polarity = "contradicts"
					} else {
						cit.Polarity = "supports"
					}
				}
				current.Citations = append(current.Citations, cit)
			}
		}
	}
	return axioms
}

// SeedAxioms creates and adjudicates each axiom as a claim, then ingests every
// declared citation source and persists a citation row for each verbatim quote
// that survives the mechanical containment check. Citations whose source fails
// to fetch or whose quote is not contained in the cached body are logged and
// skipped — the bare adjudicated claim still lands so downstream queries work,
// but the operator should treat any unsupported axiom as a TODO.
//
// Pass a nil ingester to seed bare claims only (used in tests where network
// fetching isn't available).
func SeedAxioms(store *db.Store, ing *sources.Ingester, axioms []Axiom) error {
	if _, err := EnsureAdminAgent(store); err != nil {
		return fmt.Errorf("ensure admin agent: %w", err)
	}
	for _, ax := range axioms {
		id := fmt.Sprintf("axiom-%s", strings.ToLower(ax.Code))
		existed := false
		if _, err := store.GetClaim(id); err == nil {
			existed = true
		} else {
			claim := &model.Claim{
				ID:          id,
				Proposition: ax.Proposition,
				Status:      "active",
			}
			if err := store.CreateClaim(claim); err != nil {
				return fmt.Errorf("create axiom %s: %w", ax.Code, err)
			}
			reasoning := fmt.Sprintf("Axiomatic node. %s", ax.Basis)
			if err := store.AdjudicateClaim(id, ax.Value, "seed", reasoning); err != nil {
				return fmt.Errorf("adjudicate axiom %s: %w", ax.Code, err)
			}
		}
		status := "TRUE"
		if ax.Value == 0.0 {
			status = "FALSE"
		}
		verb := "seeded"
		if existed {
			verb = "exists"
		}
		log.Printf("  %s [%s] (%s): %s", ax.Code, status, verb, truncate(ax.Proposition, 70))

		if ing == nil {
			continue
		}
		seedAxiomCitations(store, ing, id, ax)
	}
	return nil
}

// seedAxiomCitations ingests each declared citation source and persists a
// citation row for each verbatim quote that passes the mechanical check.
// Failures are logged and the loop continues; one bad citation must not block
// the rest of the axiom.
func seedAxiomCitations(store *db.Store, ing *sources.Ingester, claimID string, ax Axiom) {
	for _, ac := range ax.Citations {
		res, err := ing.Ingest(ac.URL)
		if err != nil {
			log.Printf("    skip citation %s: ingest failed: %v", ac.URL, err)
			continue
		}
		if !db.HasSourceQuote(string(res.Body), ac.VerbatimQuote) {
			log.Printf("    skip citation %s: quote not in cached body", ac.URL)
			continue
		}
		existing, _ := store.ListCitationsByClaim(claimID)
		dup := false
		for _, c := range existing {
			if c.SourceID == res.Source.ID && c.VerbatimQuote == ac.VerbatimQuote {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		reason := nullable(ac.Reasoning)
		cit := &model.Citation{
			ID:            db.GenerateID(),
			ClaimID:       claimID,
			SourceID:      res.Source.ID,
			VerbatimQuote: ac.VerbatimQuote,
			Polarity:      ac.Polarity,
			Reasoning:     reason,
			ExtractorID:   "admin",
			AuditFactor:   1.0,
			Status:        "active",
		}
		if err := store.CreateCitation(cit); err != nil {
			log.Printf("    skip citation %s: create failed: %v", ac.URL, err)
			continue
		}
	}
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// EnsureAdminAgent creates the admin agent (idempotent) and returns it.
func EnsureAdminAgent(store *db.Store) (*model.Agent, error) {
	if existing, err := store.GetAgent("admin"); err == nil {
		return existing, nil
	}
	a := &model.Agent{
		ID:           "admin",
		Name:         "admin",
		Role:         "admin",
		Reliability:  1.0,
		Productivity: 0.0,
	}
	if err := store.CreateAgent(a); err != nil {
		return nil, fmt.Errorf("create admin: %w", err)
	}
	return a, nil
}

// IssueTokenFor a registered agent, hooking through the api package's helpers.
func IssueTokenFor(store *db.Store, jwtSecret []byte, agentID, role string) (string, error) {
	return api.IssueToken(store, jwtSecret, agentID, role)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
