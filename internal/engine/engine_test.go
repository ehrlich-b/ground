package engine

import (
	"os"
	"testing"

	"github.com/ehrlich-b/ground/internal/db"
	"github.com/ehrlich-b/ground/internal/model"
)

func setupTestDB(t *testing.T) *db.Store {
	t.Helper()
	path := t.TempDir() + "/test.db"
	store, err := db.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { store.Close(); os.Remove(path) })
	return store
}

// TestBasicConvergence: two agents support the same claim. Should converge to high groundedness.
func TestBasicConvergence(t *testing.T) {
	store := setupTestDB(t)
	cfg := DefaultConfig()

	// Create two agents
	store.CreateAgent(&model.Agent{ID: "a1", Name: "Alice", Accuracy: 1.0, Contribution: 1.0, Weight: 2.0})
	store.CreateAgent(&model.Agent{ID: "a2", Name: "Bob", Accuracy: 1.0, Contribution: 1.0, Weight: 2.0})

	// Create a claim
	store.CreateClaim(&model.Claim{ID: "c1", Proposition: "Water is wet", Status: "active"})

	// Both agents support
	store.CreateAssertion(&model.Assertion{ID: "as1", AgentID: "a1", ClaimID: "c1", Stance: "support", Confidence: 0.9})
	store.CreateAssertion(&model.Assertion{ID: "as2", AgentID: "a2", ClaimID: "c1", Stance: "support", Confidence: 0.8})

	// Cross-review (mutual helpfulness)
	store.CreateReview(&model.Review{ID: "r1", ReviewerID: "a1", AssertionID: "as2", Helpfulness: 0.9})
	store.CreateReview(&model.Review{ID: "r2", ReviewerID: "a2", AssertionID: "as1", Helpfulness: 0.9})

	result, err := RunEpoch(store, cfg)
	if err != nil {
		t.Fatalf("run epoch: %v", err)
	}

	if result.AccuracyIterations == 0 {
		t.Error("expected at least 1 accuracy iteration")
	}

	// Check claim is now grounded
	claim, _ := store.GetClaim("c1")
	if claim.Groundedness < 0.5 {
		t.Errorf("expected high groundedness, got %.3f", claim.Groundedness)
	}
	if claim.Status != "grounded" && claim.Status != "emerging" {
		t.Errorf("expected grounded or emerging, got %s", claim.Status)
	}
}

// TestContestLowersGroundedness: one agent supports, one contests. Should lower groundedness.
func TestContestLowersGroundedness(t *testing.T) {
	store := setupTestDB(t)
	cfg := DefaultConfig()

	store.CreateAgent(&model.Agent{ID: "a1", Name: "Alice", Accuracy: 1.0, Contribution: 1.0, Weight: 2.0})
	store.CreateAgent(&model.Agent{ID: "a2", Name: "Bob", Accuracy: 1.0, Contribution: 1.0, Weight: 2.0})

	store.CreateClaim(&model.Claim{ID: "c1", Proposition: "Debatable thing", Status: "active"})

	store.CreateAssertion(&model.Assertion{ID: "as1", AgentID: "a1", ClaimID: "c1", Stance: "support", Confidence: 0.8})
	store.CreateAssertion(&model.Assertion{ID: "as2", AgentID: "a2", ClaimID: "c1", Stance: "contest", Confidence: 0.8})

	store.CreateReview(&model.Review{ID: "r1", ReviewerID: "a1", AssertionID: "as2", Helpfulness: 0.5})
	store.CreateReview(&model.Review{ID: "r2", ReviewerID: "a2", AssertionID: "as1", Helpfulness: 0.5})

	_, err := RunEpoch(store, cfg)
	if err != nil {
		t.Fatalf("run epoch: %v", err)
	}

	claim, _ := store.GetClaim("c1")
	// Equal support and contest should result in low groundedness
	if claim.Groundedness > 0.3 {
		t.Errorf("expected low groundedness with equal contest, got %.3f", claim.Groundedness)
	}
}

// TestAdjudicatedClaimPinned: adjudicated claims should not move.
func TestAdjudicatedClaimPinned(t *testing.T) {
	store := setupTestDB(t)
	cfg := DefaultConfig()

	store.CreateAgent(&model.Agent{ID: "a1", Name: "Alice", Accuracy: 1.0, Contribution: 1.0, Weight: 2.0})

	// Create adjudicated claim
	val := 1.0
	store.CreateClaim(&model.Claim{
		ID: "c1", Proposition: "2+2=4", Status: "adjudicated",
		Groundedness: 1.0, EffectiveGroundedness: 1.0, AdjudicatedValue: &val,
	})

	// Contest it (should not matter)
	store.CreateAssertion(&model.Assertion{ID: "as1", AgentID: "a1", ClaimID: "c1", Stance: "contest", Confidence: 1.0})

	_, err := RunEpoch(store, cfg)
	if err != nil {
		t.Fatalf("run epoch: %v", err)
	}

	claim, _ := store.GetClaim("c1")
	if claim.Groundedness != 1.0 {
		t.Errorf("adjudicated claim groundedness changed to %.3f", claim.Groundedness)
	}
}

// TestDependencyDiscounts: effective groundedness should be discounted by dependencies.
func TestDependencyDiscounts(t *testing.T) {
	store := setupTestDB(t)
	cfg := DefaultConfig()

	store.CreateAgent(&model.Agent{ID: "a1", Name: "Alice", Accuracy: 1.0, Contribution: 1.0, Weight: 2.0})

	// Create two claims: c2 depends on c1
	store.CreateClaim(&model.Claim{ID: "c1", Proposition: "Foundation", Status: "active"})
	store.CreateClaim(&model.Claim{ID: "c2", Proposition: "Depends on foundation", Status: "active"})
	store.CreateDependency(&model.Dependency{ID: "d1", ClaimID: "c2", DependsOnID: "c1", Strength: 1.0})

	// Support both at high confidence
	store.CreateAssertion(&model.Assertion{ID: "as1", AgentID: "a1", ClaimID: "c1", Stance: "support", Confidence: 0.9})
	store.CreateAssertion(&model.Assertion{ID: "as2", AgentID: "a1", ClaimID: "c2", Stance: "support", Confidence: 0.9})

	_, err := RunEpoch(store, cfg)
	if err != nil {
		t.Fatalf("run epoch: %v", err)
	}

	c1, _ := store.GetClaim("c1")
	c2, _ := store.GetClaim("c2")

	// c2's effective groundedness should be <= c1's groundedness
	// because c2 depends on c1
	if c2.EffectiveGroundedness > c1.Groundedness+0.01 {
		t.Errorf("dependent claim effective (%.3f) should not exceed dependency groundedness (%.3f)",
			c2.EffectiveGroundedness, c1.Groundedness)
	}
}

// TestContributionFromReviews: agents who review well should have higher contribution.
func TestContributionFromReviews(t *testing.T) {
	store := setupTestDB(t)
	cfg := DefaultConfig()

	// 4 reviewers: 3 agree on helpfulness, 1 outlier
	store.CreateAgent(&model.Agent{ID: "a1", Name: "Good Reviewer 1", Accuracy: 1.0, Contribution: 1.0, Weight: 2.0})
	store.CreateAgent(&model.Agent{ID: "a2", Name: "Good Reviewer 2", Accuracy: 1.0, Contribution: 1.0, Weight: 2.0})
	store.CreateAgent(&model.Agent{ID: "a3", Name: "Good Reviewer 3", Accuracy: 1.0, Contribution: 1.0, Weight: 2.0})
	store.CreateAgent(&model.Agent{ID: "a4", Name: "Bad Reviewer", Accuracy: 1.0, Contribution: 1.0, Weight: 2.0})
	store.CreateAgent(&model.Agent{ID: "a5", Name: "Asserter", Accuracy: 1.0, Contribution: 1.0, Weight: 2.0})

	store.CreateClaim(&model.Claim{ID: "c1", Proposition: "Test claim", Status: "active"})
	store.CreateAssertion(&model.Assertion{ID: "as1", AgentID: "a5", ClaimID: "c1", Stance: "support", Confidence: 0.8})

	// 3 reviewers agree on ~0.8, one outlier at 0.1
	store.CreateReview(&model.Review{ID: "r1", ReviewerID: "a1", AssertionID: "as1", Helpfulness: 0.8})
	store.CreateReview(&model.Review{ID: "r2", ReviewerID: "a2", AssertionID: "as1", Helpfulness: 0.8})
	store.CreateReview(&model.Review{ID: "r3", ReviewerID: "a3", AssertionID: "as1", Helpfulness: 0.8})
	store.CreateReview(&model.Review{ID: "r4", ReviewerID: "a4", AssertionID: "as1", Helpfulness: 0.1})

	_, err := RunEpoch(store, cfg)
	if err != nil {
		t.Fatalf("run epoch: %v", err)
	}

	a1, _ := store.GetAgent("a1")
	a4, _ := store.GetAgent("a4")

	// a1 should have higher contribution since their review aligns with majority
	if a1.Contribution <= a4.Contribution {
		t.Errorf("good reviewer contribution (%.3f) should exceed bad reviewer (%.3f)",
			a1.Contribution, a4.Contribution)
	}
}

// TestWeightFormula: weight = contribution * (1 + accuracy)
func TestWeightFormula(t *testing.T) {
	store := setupTestDB(t)
	cfg := DefaultConfig()

	store.CreateAgent(&model.Agent{ID: "a1", Name: "Alice", Accuracy: 1.0, Contribution: 1.0, Weight: 2.0})
	store.CreateClaim(&model.Claim{ID: "c1", Proposition: "Test", Status: "active"})
	store.CreateAssertion(&model.Assertion{ID: "as1", AgentID: "a1", ClaimID: "c1", Stance: "support", Confidence: 0.8})

	_, err := RunEpoch(store, cfg)
	if err != nil {
		t.Fatalf("run epoch: %v", err)
	}

	a, _ := store.GetAgent("a1")
	expected := a.Contribution * (1 + a.Accuracy)
	if abs(a.Weight-expected) > 0.001 {
		t.Errorf("weight (%.3f) != contribution * (1 + accuracy) = %.3f * (1 + %.3f) = %.3f",
			a.Weight, a.Contribution, a.Accuracy, expected)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
