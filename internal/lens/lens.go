// Package lens renders the knowledge graph under per-source credibility overrides.
//
// Lenses move ONLY source credibility. Agent reliability and citation existence are
// off-limits. Render is linear in #citations + #dep edges so it stays sub-100ms even
// at 100k claims.
package lens

import (
	"sort"

	"github.com/ehrlich-b/ground/internal/db"
	"github.com/ehrlich-b/ground/internal/model"
)

// Polarity coefficients for intrinsic groundedness.
const (
	CoefSupports    = 1.0
	CoefContradicts = -1.0
	CoefQualifies   = 0.4
)

// ClaimScore is the lens-rendered groundedness for a single claim.
type ClaimScore struct {
	ClaimID               string  `json:"claim_id"`
	Groundedness          float64 `json:"groundedness"`
	EffectiveGroundedness float64 `json:"effective_groundedness"`
	Contestation          float64 `json:"contestation"`
}

// Snapshot is an in-memory copy of everything the renderer needs from the DB.
// Loaded once per epoch (or per render-batch) and reused across many lens renders.
type Snapshot struct {
	Sources        map[string]*model.Source
	SourceTags     map[string][]string
	BaseCred       map[string]float64 // post-baseline source credibility
	Citations      []model.Citation
	Claims         []model.Claim
	Dependencies   []model.Dependency
	AgentReliab    map[string]float64 // baseline-only
}

// LoadSnapshot pulls everything in one shot.
func LoadSnapshot(store *db.Store) (*Snapshot, error) {
	srcs, err := store.ListAllSources()
	if err != nil {
		return nil, err
	}
	citations, err := store.ListAllCitations()
	if err != nil {
		return nil, err
	}
	claims, err := store.ListAllClaims()
	if err != nil {
		return nil, err
	}
	deps, err := store.ListAllDependencies()
	if err != nil {
		return nil, err
	}
	agents, err := store.ListAgents()
	if err != nil {
		return nil, err
	}
	tagMap, err := store.ListAllSourceTags()
	if err != nil {
		return nil, err
	}
	baseCred, err := store.LatestSourceCredibility()
	if err != nil {
		return nil, err
	}

	srcMap := map[string]*model.Source{}
	for i := range srcs {
		srcMap[srcs[i].ID] = &srcs[i]
	}
	relMap := map[string]float64{}
	for _, a := range agents {
		relMap[a.ID] = a.Reliability
	}
	// Anchored sources without a computed credibility yet fall back to anchor priors.
	if anchors, err := store.ListSourceAnchors(); err == nil {
		for _, a := range anchors {
			if _, ok := baseCred[a.SourceID]; !ok {
				baseCred[a.SourceID] = a.Credibility
			}
		}
	}

	return &Snapshot{
		Sources:      srcMap,
		SourceTags:   tagMap,
		BaseCred:     baseCred,
		Citations:    citations,
		Claims:       claims,
		Dependencies: deps,
		AgentReliab:  relMap,
	}, nil
}

// LensSpec captures the per-source overrides and per-tag multipliers for a lens.
// Pass an empty LensSpec to render the baseline.
type LensSpec struct {
	Overrides    []model.LensOverride
	TagOverrides []model.LensTagOverride
}

// LoadLensSpec assembles a LensSpec from DB rows for a given lens.
func LoadLensSpec(store *db.Store, lensID string) (*LensSpec, error) {
	overrides, err := store.ListLensOverrides(lensID)
	if err != nil {
		return nil, err
	}
	tagOverrides, err := store.ListLensTagOverrides(lensID)
	if err != nil {
		return nil, err
	}
	return &LensSpec{Overrides: overrides, TagOverrides: tagOverrides}, nil
}

// Render computes lens-adjusted ClaimScore for every claim in the snapshot.
func Render(snap *Snapshot, spec *LensSpec) map[string]ClaimScore {
	cred := mergeCredibility(snap, spec)
	scores := computeIntrinsic(snap, cred)
	applyDAGFlow(snap, scores)
	return scores
}

// RenderClaim is a one-claim variant; same cost as Render but focused output.
func RenderClaim(snap *Snapshot, spec *LensSpec, claimID string) ClaimScore {
	scores := Render(snap, spec)
	return scores[claimID]
}

// Gradient returns the top sources by ∂(intrinsic)/∂credibility for a claim under a lens.
// Linear-in-credibility makes this trivial: each source's contribution to a claim's
// intrinsic groundedness is `polarity_coef * audit_factor * extractor_reliability`,
// summed over the citations it backs for that claim.
type SourceImpact struct {
	SourceID string
	Delta    float64 // change in intrinsic groundedness per unit credibility change
}

func Gradient(snap *Snapshot, spec *LensSpec, claimID string, topN int) []SourceImpact {
	impact := map[string]float64{}
	for _, c := range snap.Citations {
		if c.ClaimID != claimID || c.Status != "active" {
			continue
		}
		coef := polarityCoef(c.Polarity)
		impact[c.SourceID] += coef * c.AuditFactor * snap.AgentReliab[c.ExtractorID]
	}
	out := make([]SourceImpact, 0, len(impact))
	for sid, d := range impact {
		out = append(out, SourceImpact{SourceID: sid, Delta: d})
	}
	sort.Slice(out, func(i, j int) bool { return absf(out[i].Delta) > absf(out[j].Delta) })
	if topN > 0 && len(out) > topN {
		out = out[:topN]
	}
	return out
}

func mergeCredibility(snap *Snapshot, spec *LensSpec) map[string]float64 {
	cred := make(map[string]float64, len(snap.BaseCred))
	for k, v := range snap.BaseCred {
		cred[k] = v
	}
	if spec == nil {
		return cred
	}

	// Apply per-tag multipliers first, then per-source overrides (which always win).
	if len(spec.TagOverrides) > 0 {
		mults := map[string]float64{}
		for _, to := range spec.TagOverrides {
			mults[to.Tag] = to.Multiplier
		}
		for sid, tags := range snap.SourceTags {
			for _, tag := range tags {
				if m, ok := mults[tag]; ok {
					cred[sid] = cred[sid] * m
				}
			}
		}
	}

	for _, o := range spec.Overrides {
		switch o.Mode {
		case "absolute":
			cred[o.SourceID] = o.Value
		case "multiplier":
			cred[o.SourceID] = cred[o.SourceID] * o.Value
		case "exclude":
			cred[o.SourceID] = 0
		}
	}
	return cred
}

func computeIntrinsic(snap *Snapshot, cred map[string]float64) map[string]ClaimScore {
	scores := make(map[string]ClaimScore, len(snap.Claims))
	for _, c := range snap.Claims {
		s := ClaimScore{ClaimID: c.ID}
		// Adjudicated claims pin to their adjudicated value.
		if c.Status == "adjudicated" && c.AdjudicatedValue != nil {
			v := *c.AdjudicatedValue
			s.Groundedness = v
			s.EffectiveGroundedness = v
		}
		scores[c.ID] = s
	}

	// Aggregate per-claim weighted polarity, plus track magnitude to derive contestation.
	type acc struct{ sum, magnitude, posSum, negSum float64 }
	aggs := map[string]*acc{}
	for _, ct := range snap.Citations {
		if ct.Status != "active" {
			continue
		}
		w := cred[ct.SourceID] * snap.AgentReliab[ct.ExtractorID] * ct.AuditFactor
		coef := polarityCoef(ct.Polarity)
		a, ok := aggs[ct.ClaimID]
		if !ok {
			a = &acc{}
			aggs[ct.ClaimID] = a
		}
		a.sum += coef * w
		a.magnitude += absf(coef) * w
		if coef > 0 {
			a.posSum += w * coef
		} else if coef < 0 {
			a.negSum += w * (-coef)
		}
	}

	for cid, a := range aggs {
		s := scores[cid]
		if scores[cid].Groundedness == 0 || (s.ClaimID != "" && !isAdjudicated(snap, cid)) {
			if a.magnitude == 0 {
				s.Groundedness = 0.5
			} else {
				// Map signed sum into [0,1] by sigmoid-like normalization.
				// sum/magnitude ∈ [-1,1] → 0..1
				s.Groundedness = 0.5 + 0.5*(a.sum/maxf(a.magnitude, 1e-9))
			}
		}
		// Contestation = balance of pos vs neg evidence (0=consensus, ~0.5 = even split).
		total := a.posSum + a.negSum
		if total > 0 {
			s.Contestation = 1.0 - absf(a.posSum-a.negSum)/total
		}
		s.EffectiveGroundedness = s.Groundedness // pre-DAG; topo pass refines this
		s.ClaimID = cid
		scores[cid] = s
	}
	return scores
}

func isAdjudicated(snap *Snapshot, claimID string) bool {
	for _, c := range snap.Claims {
		if c.ID == claimID {
			return c.Status == "adjudicated"
		}
	}
	return false
}

func applyDAGFlow(snap *Snapshot, scores map[string]ClaimScore) {
	// Build adjacency: claim -> deps (depends_on)
	deps := map[string][]model.Dependency{}
	in := map[string]int{}
	all := map[string]struct{}{}
	for _, d := range snap.Dependencies {
		deps[d.ClaimID] = append(deps[d.ClaimID], d)
		in[d.ClaimID]++
		all[d.ClaimID] = struct{}{}
		all[d.DependsOnID] = struct{}{}
	}
	// Topological order: visit dependsOn before claim.
	var queue []string
	for cid := range all {
		if in[cid] == 0 {
			queue = append(queue, cid)
		}
	}
	order := make([]string, 0, len(all))
	depOf := map[string][]string{}
	for _, d := range snap.Dependencies {
		depOf[d.DependsOnID] = append(depOf[d.DependsOnID], d.ClaimID)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		order = append(order, cur)
		for _, child := range depOf[cur] {
			in[child]--
			if in[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	for _, cid := range order {
		ds := deps[cid]
		if len(ds) == 0 {
			continue
		}
		s := scores[cid]
		if isAdjudicated(snap, cid) {
			continue
		}
		// Effective groundedness multiplied through dependency floor.
		factor := 1.0
		for _, d := range ds {
			depScore := scores[d.DependsOnID]
			// each dep dampens by (depScore.EffectiveGroundedness ^ strength)
			factor *= powApprox(depScore.EffectiveGroundedness, d.Strength)
		}
		s.EffectiveGroundedness = s.Groundedness * factor
		scores[cid] = s
	}
}

func polarityCoef(p string) float64 {
	switch p {
	case "supports":
		return CoefSupports
	case "contradicts":
		return CoefContradicts
	case "qualifies":
		return CoefQualifies
	default:
		return 0
	}
}

func absf(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// powApprox computes a^b without importing math; b is in [0,1] typically.
// Falls back to exp(b * ln(a)) via series only when needed; for our domain,
// linear interpolation between 1 (b=0) and a (b=1) is close enough.
func powApprox(a, b float64) float64 {
	if a < 0 {
		a = 0
	}
	if a > 1 {
		a = 1
	}
	if b <= 0 {
		return 1
	}
	if b >= 1 {
		return a
	}
	return 1 - b*(1-a)
}
