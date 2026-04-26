// Package engine runs the per-epoch baseline computation:
//
//   - source credibility from anchors + source-source citation graph + audit aggregates
//   - agent reliability from audit verdicts (uphold vs reject), with mechanical-fail = hard reject
//   - claim intrinsic groundedness (linear in source credibility)
//   - effective groundedness in topological order over the dependency DAG
//
// All three iterations are bounded EigenTrust-style power iterations damped toward priors.
package engine

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/ehrlich-b/ground/internal/db"
	"github.com/ehrlich-b/ground/internal/lens"
	"github.com/ehrlich-b/ground/internal/model"
)

// Config holds tunable weights for one epoch.
type Config struct {
	SourceMaxIter   int
	SourceTolerance float64
	SourceDamping   float64
	SourceWAnchor   float64
	SourceWGraph    float64
	SourceWAudit    float64

	AgentMaxIter   int
	AgentTolerance float64
	AgentDamping   float64
	AgentPrior     float64

	GroundedThreshold float64
	RefutedThreshold  float64
	ContestedThresh   float64
}

func DefaultConfig() Config {
	return Config{
		SourceMaxIter:     50,
		SourceTolerance:   1e-4,
		SourceDamping:     0.15,
		SourceWAnchor:     0.5,
		SourceWGraph:      0.3,
		SourceWAudit:      0.2,
		AgentMaxIter:      50,
		AgentTolerance:    1e-4,
		AgentDamping:      0.15,
		AgentPrior:        0.5,
		GroundedThreshold: 0.7,
		RefutedThreshold:  0.3,
		ContestedThresh:   0.5,
	}
}

// EpochResult is what RunEpoch returns to callers.
type EpochResult struct {
	EpochID          int
	SourceIterations int
	AgentIterations  int
	SourceDelta      float64
	AgentDelta       float64
}

// RunEpoch runs one full v2 epoch and persists results.
func RunEpoch(store *db.Store, cfg Config) (*EpochResult, error) {
	epoch, err := store.CreateEpoch()
	if err != nil {
		return nil, fmt.Errorf("create epoch: %w", err)
	}

	srcCreds, srcIter, srcDelta, err := computeSourceCredibility(store, cfg)
	if err != nil {
		return nil, fmt.Errorf("source credibility: %w", err)
	}
	for sid, val := range srcCreds {
		comps, _ := json.Marshal(map[string]float64{"value": val})
		s := string(comps)
		if err := store.UpsertSourceCredibility(&model.SourceCredibility{
			SourceID:   sid,
			EpochID:    epoch.ID,
			Value:      val,
			Components: &s,
		}); err != nil {
			return nil, fmt.Errorf("save source credibility: %w", err)
		}
	}
	log.Printf("source credibility: %d sources, %d iterations, delta=%.6f", len(srcCreds), srcIter, srcDelta)

	if err := refreshCitationAuditFactors(store); err != nil {
		return nil, fmt.Errorf("audit factor: %w", err)
	}

	relMap, agentIter, agentDelta, err := computeAgentReliability(store, cfg)
	if err != nil {
		return nil, fmt.Errorf("agent reliability: %w", err)
	}
	for aid, rel := range relMap {
		productivity := computeProductivity(store, aid)
		if err := store.UpdateAgentScores(aid, rel, productivity); err != nil {
			return nil, fmt.Errorf("save agent scores: %w", err)
		}
	}
	log.Printf("agent reliability: %d agents, %d iterations, delta=%.6f", len(relMap), agentIter, agentDelta)

	snap, err := lens.LoadSnapshot(store)
	if err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}
	scores := lens.Render(snap, &lens.LensSpec{})
	now := time.Now().UTC()
	for cid, s := range scores {
		status := classifyStatus(s, cfg)
		if isAdjudicated(snap, cid) {
			status = "adjudicated"
		}
		if err := store.UpdateClaimScores(cid, s.Groundedness, s.EffectiveGroundedness, s.Contestation, status, now); err != nil {
			return nil, fmt.Errorf("update claim %s: %w", cid, err)
		}
	}

	if err := store.CompleteEpoch(epoch.ID, srcIter, agentIter, srcDelta, agentDelta); err != nil {
		return nil, fmt.Errorf("complete epoch: %w", err)
	}

	return &EpochResult{
		EpochID:          epoch.ID,
		SourceIterations: srcIter,
		AgentIterations:  agentIter,
		SourceDelta:      srcDelta,
		AgentDelta:       agentDelta,
	}, nil
}

func computeSourceCredibility(store *db.Store, cfg Config) (map[string]float64, int, float64, error) {
	srcs, err := store.ListAllSources()
	if err != nil {
		return nil, 0, 0, err
	}
	if len(srcs) == 0 {
		return map[string]float64{}, 0, 0, nil
	}

	anchors, err := store.ListSourceAnchors()
	if err != nil {
		return nil, 0, 0, err
	}
	anchorMap := map[string]float64{}
	for _, a := range anchors {
		anchorMap[a.SourceID] = a.Credibility
	}

	edges, err := store.ListSourceCitationEdges()
	if err != nil {
		return nil, 0, 0, err
	}

	citations, err := store.ListAllCitations()
	if err != nil {
		return nil, 0, 0, err
	}
	auditAgg := map[string][]float64{}
	for _, c := range citations {
		auditAgg[c.SourceID] = append(auditAgg[c.SourceID], c.AuditFactor)
	}
	auditByID := map[string]float64{}
	for sid, factors := range auditAgg {
		var sum float64
		for _, f := range factors {
			sum += f
		}
		auditByID[sid] = sum / float64(len(factors))
	}

	prev := map[string]float64{}
	for _, s := range srcs {
		if v, ok := anchorMap[s.ID]; ok {
			prev[s.ID] = v
		} else {
			prev[s.ID] = 0.5
		}
	}

	var iter int
	var delta float64
	for iter = 0; iter < cfg.SourceMaxIter; iter++ {
		next := map[string]float64{}
		for _, s := range srcs {
			anchor, hasAnchor := anchorMap[s.ID]
			if !hasAnchor {
				anchor = 0.5
			}
			audit := auditByID[s.ID]

			var inSum, inCount float64
			for _, e := range edges {
				if e.ToSourceID == s.ID {
					inSum += prev[e.FromSourceID]
					inCount++
				}
			}
			graph := 0.5
			if inCount > 0 {
				graph = inSum / inCount
			}

			combined := cfg.SourceWAnchor*anchor + cfg.SourceWGraph*graph + cfg.SourceWAudit*audit
			combined = (1-cfg.SourceDamping)*combined + cfg.SourceDamping*anchor
			if combined < 0 {
				combined = 0
			}
			if combined > 1 {
				combined = 1
			}
			next[s.ID] = combined
		}

		delta = maxDelta(prev, next)
		prev = next
		if delta < cfg.SourceTolerance {
			iter++
			break
		}
	}
	return prev, iter, delta, nil
}

func refreshCitationAuditFactors(store *db.Store) error {
	citations, err := store.ListAllCitations()
	if err != nil {
		return err
	}
	for _, c := range citations {
		audits, err := store.ListAuditsByCitation(c.ID)
		if err != nil {
			return err
		}
		if len(audits) == 0 {
			if c.AuditFactor != 1.0 {
				if err := store.UpdateCitationAuditFactor(c.ID, 1.0); err != nil {
					return err
				}
			}
			continue
		}
		anyMechFail := false
		var uphold, total float64
		for _, a := range audits {
			if a.Mechanical == "fail" {
				anyMechFail = true
				break
			}
			total++
			if a.Verdict == "uphold" {
				uphold++
			}
		}
		factor := 0.0
		if !anyMechFail {
			if total > 0 {
				factor = uphold / total
			} else {
				factor = 1.0
			}
		}
		if err := store.UpdateCitationAuditFactor(c.ID, factor); err != nil {
			return err
		}
		if anyMechFail {
			if err := store.UpdateCitationStatus(c.ID, "rejected"); err != nil {
				return err
			}
		}
	}
	return nil
}

func computeAgentReliability(store *db.Store, cfg Config) (map[string]float64, int, float64, error) {
	agents, err := store.ListAgents()
	if err != nil {
		return nil, 0, 0, err
	}
	if len(agents) == 0 {
		return map[string]float64{}, 0, 0, nil
	}

	citations, err := store.ListAllCitations()
	if err != nil {
		return nil, 0, 0, err
	}
	audits, err := store.ListAllAudits()
	if err != nil {
		return nil, 0, 0, err
	}
	auditsByCitation := map[string][]model.Audit{}
	for _, a := range audits {
		auditsByCitation[a.CitationID] = append(auditsByCitation[a.CitationID], a)
	}

	prev := map[string]float64{}
	for _, a := range agents {
		prev[a.ID] = cfg.AgentPrior
	}

	var iter int
	var delta float64
	for iter = 0; iter < cfg.AgentMaxIter; iter++ {
		next := map[string]float64{}

		agentScore := map[string]struct{ num, den float64 }{}
		for _, c := range citations {
			audList := auditsByCitation[c.ID]
			if len(audList) == 0 {
				continue
			}
			for _, ad := range audList {
				w := prev[ad.AuditorID]
				if ad.Mechanical == "fail" {
					w = 1.0
					s := agentScore[c.ExtractorID]
					s.den += w
					agentScore[c.ExtractorID] = s
					continue
				}
				s := agentScore[c.ExtractorID]
				s.den += w
				if ad.Verdict == "uphold" {
					s.num += w
				}
				agentScore[c.ExtractorID] = s
			}
		}

		for _, a := range agents {
			s, ok := agentScore[a.ID]
			score := cfg.AgentPrior
			if ok && s.den > 0 {
				score = s.num / s.den
			}
			score = (1-cfg.AgentDamping)*score + cfg.AgentDamping*cfg.AgentPrior
			if score < 0 {
				score = 0
			}
			if score > 1 {
				score = 1
			}
			next[a.ID] = score
		}

		delta = maxDelta(prev, next)
		prev = next
		if delta < cfg.AgentTolerance {
			iter++
			break
		}
	}
	return prev, iter, delta, nil
}

func computeProductivity(store *db.Store, agentID string) float64 {
	citations, err := store.ListCitationsByExtractor(agentID, 1000, 0)
	if err != nil {
		return 0
	}
	var upheld float64
	for _, c := range citations {
		if c.Status == "active" && c.AuditFactor >= 0.5 {
			upheld++
		}
	}
	return math.Log1p(upheld)
}

func classifyStatus(s lens.ClaimScore, cfg Config) string {
	if s.Contestation >= cfg.ContestedThresh {
		return "contested"
	}
	if s.EffectiveGroundedness >= cfg.GroundedThreshold {
		return "grounded"
	}
	if s.EffectiveGroundedness <= cfg.RefutedThreshold {
		return "refuted"
	}
	return "active"
}

func isAdjudicated(snap *lens.Snapshot, claimID string) bool {
	for _, c := range snap.Claims {
		if c.ID == claimID {
			return c.Status == "adjudicated"
		}
	}
	return false
}

func maxDelta(a, b map[string]float64) float64 {
	var d float64
	for k, va := range a {
		vb, ok := b[k]
		if !ok {
			continue
		}
		x := va - vb
		if x < 0 {
			x = -x
		}
		if x > d {
			d = x
		}
	}
	return d
}
