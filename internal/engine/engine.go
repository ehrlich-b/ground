package engine

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/ehrlich-b/ground/internal/db"
	"github.com/ehrlich-b/ground/internal/model"
)

// Config holds the tunable parameters for the EigenTrust computation.
type Config struct {
	MaxIterations      int
	Epsilon            float64 // convergence threshold
	DampingFactor      float64 // d in accuracy/contribution smoothing
	Prior              float64 // default accuracy/contribution
	Alpha              float64 // contest scaling factor in groundedness
	RefineWeight       float64 // partial support from refine assertions
	GroundedThreshold  float64 // minimum groundedness for "grounded" status
	RefutedThreshold   float64 // below this = "refuted"
	ContestedThreshold float64 // above this contestation = "contested"
}

func DefaultConfig() Config {
	return Config{
		MaxIterations:      100,
		Epsilon:            0.001,
		DampingFactor:      0.85,
		Prior:              1.0,
		Alpha:              1.0,
		RefineWeight:       0.3,
		GroundedThreshold:  0.8,
		RefutedThreshold:   0.2,
		ContestedThreshold: 0.3,
	}
}

// EpochResult contains the output of a single epoch computation.
type EpochResult struct {
	EpochID                int
	AccuracyIterations     int
	ContributionIterations int
	AccuracyDelta          float64
	ContributionDelta      float64
}

// RunEpoch executes one full dual EigenTrust epoch.
func RunEpoch(store *db.Store, cfg Config) (*EpochResult, error) {
	epoch, err := store.CreateEpoch()
	if err != nil {
		return nil, fmt.Errorf("create epoch: %w", err)
	}

	// Load all data
	agents, err := store.ListAgents()
	if err != nil {
		return nil, fmt.Errorf("load agents: %w", err)
	}
	claims, err := store.ListAllClaims()
	if err != nil {
		return nil, fmt.Errorf("load claims: %w", err)
	}
	assertions, err := store.ListAllAssertions()
	if err != nil {
		return nil, fmt.Errorf("load assertions: %w", err)
	}
	reviews, err := store.ListAllReviews()
	if err != nil {
		return nil, fmt.Errorf("load reviews: %w", err)
	}
	allDeps, err := store.ListAllDependencies()
	if err != nil {
		return nil, fmt.Errorf("load dependencies: %w", err)
	}

	// Build indexes
	agentMap := make(map[string]*agentState)
	for i := range agents {
		agentMap[agents[i].ID] = &agentState{
			agent:        &agents[i],
			accuracy:     agents[i].Accuracy,
			contribution: agents[i].Contribution,
			weight:       agents[i].Weight,
		}
	}

	claimMap := make(map[string]*claimState)
	for i := range claims {
		cs := &claimState{
			claim:                  &claims[i],
			groundedness:           claims[i].Groundedness,
			effectiveGroundedness:  claims[i].EffectiveGroundedness,
		}
		if claims[i].Status == "adjudicated" && claims[i].AdjudicatedValue != nil {
			cs.groundedness = *claims[i].AdjudicatedValue
			cs.effectiveGroundedness = *claims[i].AdjudicatedValue
			cs.adjudicated = true
		}
		claimMap[claims[i].ID] = cs
	}

	// Assertions indexed by claim and by agent
	assertionsByClaim := make(map[string][]model.Assertion)
	assertionsByAgent := make(map[string][]model.Assertion)
	assertionMap := make(map[string]*model.Assertion)
	for i := range assertions {
		a := assertions[i]
		assertionsByClaim[a.ClaimID] = append(assertionsByClaim[a.ClaimID], a)
		assertionsByAgent[a.AgentID] = append(assertionsByAgent[a.AgentID], a)
		assertionMap[a.ID] = &assertions[i]
	}

	// Reviews indexed by assertion and by reviewer
	reviewsByAssertion := make(map[string][]model.Review)
	reviewsByReviewer := make(map[string][]model.Review)
	for i := range reviews {
		r := reviews[i]
		reviewsByAssertion[r.AssertionID] = append(reviewsByAssertion[r.AssertionID], r)
		reviewsByReviewer[r.ReviewerID] = append(reviewsByReviewer[r.ReviewerID], r)
	}

	// Dependencies indexed by claim
	depsByClaim := make(map[string][]model.Dependency)
	for _, d := range allDeps {
		depsByClaim[d.ClaimID] = append(depsByClaim[d.ClaimID], d)
	}

	// Topological order for dependency DAG
	topoOrder := topologicalSort(claims, depsByClaim)

	// Helpfulness state
	helpfulness := make(map[string]float64) // assertion ID -> helpfulness

	// --- Run Contribution Graph ---
	contIter, contDelta := runContributionGraph(cfg, agentMap, helpfulness, reviewsByAssertion, reviewsByReviewer, assertionMap)
	log.Printf("contribution graph: %d iterations, delta=%.6f", contIter, contDelta)

	// --- Run Accuracy Graph (outer loop) ---
	accIter, accDelta := runAccuracyGraph(cfg, agentMap, claimMap, assertionsByClaim, assertionsByAgent, depsByClaim, topoOrder)
	log.Printf("accuracy graph: %d iterations, delta=%.6f", accIter, accDelta)

	// Final weight computation
	for _, as := range agentMap {
		as.weight = as.contribution * (1 + as.accuracy)
	}

	// Compute contestation and status for each claim
	for _, cs := range claimMap {
		if cs.adjudicated {
			cs.status = "adjudicated"
			continue
		}
		cs.contestation = computeContestation(assertionsByClaim[cs.claim.ID], agentMap)
		cs.status = computeStatus(cs.groundedness, cs.effectiveGroundedness, cs.contestation, cfg)
	}

	// Persist results
	now := time.Now().UTC()
	for _, as := range agentMap {
		if err := store.UpdateAgentScores(as.agent.ID, as.accuracy, as.contribution, as.weight); err != nil {
			return nil, fmt.Errorf("update agent %s: %w", as.agent.ID, err)
		}
	}
	for _, cs := range claimMap {
		if err := store.UpdateClaimScores(cs.claim.ID, cs.groundedness, cs.effectiveGroundedness, cs.contestation, cs.status, now); err != nil {
			return nil, fmt.Errorf("update claim %s: %w", cs.claim.ID, err)
		}
	}
	for id, h := range helpfulness {
		if err := store.UpdateAssertionHelpfulness(id, h); err != nil {
			return nil, fmt.Errorf("update helpfulness %s: %w", id, err)
		}
	}

	if err := store.CompleteEpoch(epoch.ID, accIter, contIter, accDelta, contDelta); err != nil {
		return nil, fmt.Errorf("complete epoch: %w", err)
	}

	return &EpochResult{
		EpochID:                epoch.ID,
		AccuracyIterations:     accIter,
		ContributionIterations: contIter,
		AccuracyDelta:          accDelta,
		ContributionDelta:      contDelta,
	}, nil
}

// --- Internal state ---

type agentState struct {
	agent        *model.Agent
	accuracy     float64
	contribution float64
	weight       float64
}

type claimState struct {
	claim                 *model.Claim
	groundedness          float64
	effectiveGroundedness float64
	contestation          float64
	status                string
	adjudicated           bool
}

// --- Accuracy Graph ---

func runAccuracyGraph(
	cfg Config,
	agentMap map[string]*agentState,
	claimMap map[string]*claimState,
	assertionsByClaim map[string][]model.Assertion,
	assertionsByAgent map[string][]model.Assertion,
	depsByClaim map[string][]model.Dependency,
	topoOrder []string,
) (int, float64) {
	var iter int
	var delta float64

	for iter = 0; iter < cfg.MaxIterations; iter++ {
		delta = 0

		// Step 1: Intrinsic groundedness from weight-adjusted assertions
		for claimID, cs := range claimMap {
			if cs.adjudicated {
				continue
			}
			assertions := assertionsByClaim[claimID]
			var support, partial, contest float64
			for _, a := range assertions {
				as := agentMap[a.AgentID]
				if as == nil {
					continue
				}
				switch a.Stance {
				case "support":
					support += a.Confidence * as.weight
				case "refine":
					partial += cfg.RefineWeight * a.Confidence * as.weight
				case "contest":
					contest += a.Confidence * as.weight
				}
			}
			denom := support + partial + cfg.Alpha*contest + cfg.Epsilon
			raw := (support + partial - cfg.Alpha*contest) / denom
			newG := clamp(raw, 0, 1)
			delta = math.Max(delta, math.Abs(newG-cs.groundedness))
			cs.groundedness = newG
		}

		// Step 2: Effective groundedness via dependency DAG (topological order)
		for _, claimID := range topoOrder {
			cs := claimMap[claimID]
			if cs == nil {
				continue
			}
			if cs.adjudicated {
				continue
			}
			eff := cs.groundedness
			for _, dep := range depsByClaim[claimID] {
				depCS := claimMap[dep.DependsOnID]
				if depCS == nil {
					continue
				}
				eff *= math.Pow(depCS.effectiveGroundedness, dep.Strength)
			}
			delta = math.Max(delta, math.Abs(eff-cs.effectiveGroundedness))
			cs.effectiveGroundedness = eff
		}

		// Step 3: Accuracy from effective groundedness
		for agentID, as := range agentMap {
			assertions := assertionsByAgent[agentID]
			if len(assertions) == 0 {
				continue
			}
			var score, totalConf float64
			for _, a := range assertions {
				cs := claimMap[a.ClaimID]
				if cs == nil {
					continue
				}
				switch a.Stance {
				case "support":
					score += a.Confidence * cs.effectiveGroundedness
				case "contest":
					score += a.Confidence * (1 - cs.effectiveGroundedness)
				case "refine":
					if a.RefinementClaimID != nil {
						refCS := claimMap[*a.RefinementClaimID]
						if refCS != nil {
							score += a.Confidence * refCS.effectiveGroundedness
						}
					}
				}
				totalConf += a.Confidence
			}

			var rawAcc float64
			if totalConf > 0 {
				rawAcc = score / totalConf
			}
			newAcc := cfg.DampingFactor*rawAcc + (1-cfg.DampingFactor)*cfg.Prior
			delta = math.Max(delta, math.Abs(newAcc-as.accuracy))
			as.accuracy = newAcc
		}

		// Step 4: Recompute weight
		for _, as := range agentMap {
			as.weight = as.contribution * (1 + as.accuracy)
		}

		if delta < cfg.Epsilon {
			iter++
			break
		}
	}

	return iter, delta
}

// --- Contribution Graph ---

func runContributionGraph(
	cfg Config,
	agentMap map[string]*agentState,
	helpfulness map[string]float64,
	reviewsByAssertion map[string][]model.Review,
	reviewsByReviewer map[string][]model.Review,
	assertionMap map[string]*model.Assertion,
) (int, float64) {
	var iter int
	var delta float64

	for iter = 0; iter < cfg.MaxIterations; iter++ {
		delta = 0

		// Step 1: Helpfulness from contribution-weighted reviews
		for assertionID, reviews := range reviewsByAssertion {
			var weightedSum, totalWeight float64
			for _, r := range reviews {
				as := agentMap[r.ReviewerID]
				if as == nil {
					continue
				}
				weightedSum += r.Helpfulness * as.contribution
				totalWeight += as.contribution
			}
			newH := weightedSum / (totalWeight + cfg.Epsilon)
			old := helpfulness[assertionID]
			delta = math.Max(delta, math.Abs(newH-old))
			helpfulness[assertionID] = newH
		}

		// Step 2: Contribution-credibility from review alignment
		for agentID, as := range agentMap {
			reviews := reviewsByReviewer[agentID]
			if len(reviews) == 0 {
				continue
			}
			var score float64
			for _, r := range reviews {
				consensus := helpfulness[r.AssertionID]
				agreement := 1.0 - math.Abs(r.Helpfulness-consensus)
				score += agreement
			}
			rawContrib := score / float64(len(reviews))
			newContrib := cfg.DampingFactor*rawContrib + (1-cfg.DampingFactor)*cfg.Prior
			delta = math.Max(delta, math.Abs(newContrib-as.contribution))
			as.contribution = newContrib
		}

		if delta < cfg.Epsilon {
			iter++
			break
		}
	}

	return iter, delta
}

// --- Contestation ---

func computeContestation(assertions []model.Assertion, agentMap map[string]*agentState) float64 {
	if len(assertions) == 0 {
		return 0
	}

	// Contestation = variance of weighted stances
	var values []float64
	for _, a := range assertions {
		as := agentMap[a.AgentID]
		if as == nil {
			continue
		}
		var stanceSign float64
		switch a.Stance {
		case "support":
			stanceSign = 1.0
		case "contest":
			stanceSign = -1.0
		case "refine":
			stanceSign = 0.5
		}
		values = append(values, stanceSign*a.Confidence*as.weight)
	}

	if len(values) < 2 {
		return 0
	}

	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))

	var variance float64
	for _, v := range values {
		d := v - mean
		variance += d * d
	}
	variance /= float64(len(values))

	return clamp(variance, 0, 1)
}

// --- Status Transitions ---

func computeStatus(groundedness, effectiveGroundedness, contestation float64, cfg Config) string {
	if effectiveGroundedness >= cfg.GroundedThreshold && contestation < cfg.ContestedThreshold {
		return "grounded"
	}
	if effectiveGroundedness < cfg.RefutedThreshold && contestation < cfg.ContestedThreshold {
		return "refuted"
	}
	if contestation >= cfg.ContestedThreshold {
		return "contested"
	}
	if groundedness > 0.3 {
		return "emerging"
	}
	return "active"
}

// --- Topological Sort ---

func topologicalSort(claims []model.Claim, depsByClaim map[string][]model.Dependency) []string {
	// Kahn's algorithm
	inDegree := make(map[string]int)
	for _, c := range claims {
		inDegree[c.ID] = 0
	}
	for _, deps := range depsByClaim {
		for range deps {
			// The claim depends on something, so it has incoming edges
		}
	}
	// Actually: for each dep edge (claim depends_on dep), claim has in-degree += 1
	for claimID, deps := range depsByClaim {
		inDegree[claimID] += len(deps)
	}

	var queue []string
	for _, c := range claims {
		if inDegree[c.ID] == 0 {
			queue = append(queue, c.ID)
		}
	}

	// Build reverse index: which claims depend on this claim?
	dependents := make(map[string][]string)
	for claimID, deps := range depsByClaim {
		for _, d := range deps {
			dependents[d.DependsOnID] = append(dependents[d.DependsOnID], claimID)
		}
	}

	var order []string
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		order = append(order, cur)

		for _, dep := range dependents[cur] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	return order
}

// --- Helpers ---

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
