package agent

import (
	"strings"
	"testing"
)

func TestParseAxiomsFACTSFormat(t *testing.T) {
	src := `# Axioms

## Mathematics

### MATH-01: Gödel's First

**Proposition**: Some statement.

**Adjudication**: TRUE

**Citations**:
- ` + "`https://example.org/g1`" + ` — *"the verbatim quote here"* (supports)
- ` + "`https://example.org/g1-alt`" + ` — *"second quote"*

**Topic anchors**: math, logic

## Adjudicated FALSE

### FALSE-01: Bogus

**Proposition**: A wrong claim.

**Adjudication**: FALSE

**Citations**:
- ` + "`https://example.org/false`" + ` — *"refuting quote"* (supports the FALSE adjudication)
- See MATH-01 — same evidence, opposite polarity
- Original publication: Smith (1900) — citation pending ingestion
`

	axioms := ParseAxioms(src)
	if len(axioms) != 2 {
		t.Fatalf("got %d axioms, want 2", len(axioms))
	}

	a := axioms[0]
	if a.Code != "MATH-01" {
		t.Errorf("code = %q, want MATH-01", a.Code)
	}
	if !strings.Contains(a.Proposition, "Some statement") {
		t.Errorf("proposition = %q", a.Proposition)
	}
	if a.Value != 1.0 {
		t.Errorf("value = %f, want 1.0", a.Value)
	}
	if len(a.Citations) != 2 {
		t.Fatalf("MATH-01 citations = %d, want 2", len(a.Citations))
	}
	if a.Citations[0].URL != "https://example.org/g1" {
		t.Errorf("citation url = %q", a.Citations[0].URL)
	}
	if a.Citations[0].VerbatimQuote != "the verbatim quote here" {
		t.Errorf("citation quote = %q", a.Citations[0].VerbatimQuote)
	}
	if a.Citations[0].Polarity != "supports" {
		t.Errorf("citation polarity = %q", a.Citations[0].Polarity)
	}
	// second citation: no explicit polarity, axiom is TRUE → supports
	if a.Citations[1].Polarity != "supports" {
		t.Errorf("citation[1] polarity = %q, want supports", a.Citations[1].Polarity)
	}
	if got := a.Anchors; len(got) != 2 || got[0] != "math" || got[1] != "logic" {
		t.Errorf("anchors = %v", got)
	}

	b := axioms[1]
	if b.Value != 0.0 {
		t.Errorf("FALSE axiom value = %f, want 0.0", b.Value)
	}
	if len(b.Citations) != 1 {
		t.Fatalf("FALSE-01 citations = %d, want 1 (placeholder lines should be skipped)", len(b.Citations))
	}
	if b.Citations[0].Polarity != "supports" {
		// "supports the FALSE adjudication" — first word is supports
		t.Errorf("FALSE citation polarity = %q", b.Citations[0].Polarity)
	}
}

func TestParseAxiomsRealFACTS(t *testing.T) {
	// Sanity check against the actual FACTS.md format: load a small slice and
	// verify we extract at least one citation per parseable axiom.
	src := `### MATH-06: Bayes' Theorem

**Proposition**: For events A and B with P(B) > 0, P(A|B) = P(B|A)P(A)/P(B) is a valid theorem of probability theory.

**Adjudication**: TRUE

**Citations**:
- ` + "`https://en.wikipedia.org/wiki/Bayes%27_theorem`" + ` — *"In probability theory and statistics, Bayes' theorem (alternatively Bayes' law or Bayes' rule) describes the probability of an event, based on prior knowledge of conditions that might be related to the event."* (supports)

**Topic anchors**: frameworks-for-reasoning-under-uncertainty
`
	axioms := ParseAxioms(src)
	if len(axioms) != 1 {
		t.Fatalf("got %d axioms, want 1", len(axioms))
	}
	if len(axioms[0].Citations) != 1 {
		t.Fatalf("got %d citations, want 1", len(axioms[0].Citations))
	}
	if !strings.HasPrefix(axioms[0].Citations[0].VerbatimQuote, "In probability theory") {
		t.Errorf("quote not extracted: %q", axioms[0].Citations[0].VerbatimQuote)
	}
}
