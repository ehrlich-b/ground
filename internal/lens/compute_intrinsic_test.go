package lens

import (
	"math"
	"testing"

	"github.com/ehrlich-b/ground/internal/model"
)

func fixtureSnapshot(claims []model.Claim, citations []model.Citation, reliab map[string]float64) *Snapshot {
	return &Snapshot{
		Claims:      claims,
		Citations:   citations,
		AgentReliab: reliab,
	}
}

func nearF(t *testing.T, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("got %v, want %v (within %v)", got, want, tol)
	}
}

func TestComputeIntrinsicZeroCitations(t *testing.T) {
	snap := fixtureSnapshot(
		[]model.Claim{{ID: "c-zero"}},
		nil,
		nil,
	)
	scores := computeIntrinsic(snap, map[string]float64{})
	s, ok := scores["c-zero"]
	if !ok {
		t.Fatal("claim missing from scores")
	}
	if s.Groundedness != 0 {
		t.Errorf("Groundedness = %v, want 0", s.Groundedness)
	}
	if s.Contestation != 0 {
		t.Errorf("Contestation = %v, want 0", s.Contestation)
	}
}

func TestComputeIntrinsicZeroWeightActiveCitation(t *testing.T) {
	snap := fixtureSnapshot(
		[]model.Claim{{ID: "c-zw"}},
		[]model.Citation{{
			ID:          "cit-zw",
			ClaimID:     "c-zw",
			SourceID:    "s-excluded",
			Polarity:    "supports",
			ExtractorID: "a-1",
			AuditFactor: 1.0,
			Status:      "active",
		}},
		map[string]float64{"a-1": 1.0},
	)
	cred := map[string]float64{"s-excluded": 0.0}
	s := computeIntrinsic(snap, cred)["c-zw"]
	if s.Groundedness == 0 {
		t.Fatal("zero-weight active citation must not produce the zero-citations fallback (0)")
	}
	if s.Groundedness != 0.5 {
		t.Errorf("Groundedness = %v, want 0.5 (uncertain fallback)", s.Groundedness)
	}
	if s.Contestation != 0 {
		t.Errorf("Contestation = %v, want 0", s.Contestation)
	}
}

func TestComputeIntrinsicBalancedEvidence(t *testing.T) {
	snap := fixtureSnapshot(
		[]model.Claim{{ID: "c-balanced"}},
		[]model.Citation{
			{ID: "cit-sup", ClaimID: "c-balanced", SourceID: "s-sup", Polarity: "supports", ExtractorID: "a-1", AuditFactor: 0.9375, Status: "active"},
			{ID: "cit-con", ClaimID: "c-balanced", SourceID: "s-con", Polarity: "contradicts", ExtractorID: "a-1", AuditFactor: 0.9375, Status: "active"},
		},
		map[string]float64{"a-1": 1.0},
	)
	cred := map[string]float64{"s-sup": 0.96, "s-con": 0.9}
	s := computeIntrinsic(snap, cred)["c-balanced"]
	nearF(t, s.Groundedness, 0.5161290322580645, 1e-9)
	nearF(t, s.Contestation, 0.967741935483871, 1e-9)
}

func TestComputeIntrinsicSupportsOnly(t *testing.T) {
	snap := fixtureSnapshot(
		[]model.Claim{{ID: "c-sup"}},
		[]model.Citation{{
			ID:          "cit-sup",
			ClaimID:     "c-sup",
			SourceID:    "s-sup",
			Polarity:    "supports",
			ExtractorID: "a-1",
			AuditFactor: 1.0,
			Status:      "active",
		}},
		map[string]float64{"a-1": 1.0},
	)
	cred := map[string]float64{"s-sup": 1.0}
	s := computeIntrinsic(snap, cred)["c-sup"]
	if s.Contestation != 0 {
		t.Errorf("Contestation = %v, want 0", s.Contestation)
	}
	if s.Groundedness <= 0.99 {
		t.Errorf("Groundedness = %v, want > 0.99", s.Groundedness)
	}
}

func TestComputeIntrinsicRejectedCitationIgnored(t *testing.T) {
	noCits := fixtureSnapshot(
		[]model.Claim{{ID: "c-zero"}},
		nil,
		map[string]float64{"a-1": 1.0},
	)
	rej := fixtureSnapshot(
		[]model.Claim{{ID: "c-rej"}},
		[]model.Citation{{
			ID:          "cit-rej",
			ClaimID:     "c-rej",
			SourceID:    "s-sup",
			Polarity:    "supports",
			ExtractorID: "a-1",
			AuditFactor: 1.0,
			Status:      "rejected",
		}},
		map[string]float64{"a-1": 1.0},
	)
	cred := map[string]float64{"s-sup": 1.0}
	want := computeIntrinsic(noCits, cred)["c-zero"]
	got := computeIntrinsic(rej, cred)["c-rej"]
	if got.Groundedness != want.Groundedness ||
		got.EffectiveGroundedness != want.EffectiveGroundedness ||
		got.Contestation != want.Contestation {
		t.Fatalf("rejected citation changed score: got %+v, want %+v", got, want)
	}
	if got.Groundedness != 0 || got.Contestation != 0 {
		t.Fatalf("expected zero-value score for rejected citation, got %+v", got)
	}
}

func TestComputeIntrinsicAdjudicatedPin(t *testing.T) {
	v := 0.8
	snap := fixtureSnapshot(
		[]model.Claim{{ID: "c-adjud", Status: "adjudicated", AdjudicatedValue: &v}},
		[]model.Citation{
			{ID: "cit-sup", ClaimID: "c-adjud", SourceID: "s-sup", Polarity: "supports", ExtractorID: "a-1", AuditFactor: 0.9375, Status: "active"},
			{ID: "cit-con", ClaimID: "c-adjud", SourceID: "s-con", Polarity: "contradicts", ExtractorID: "a-1", AuditFactor: 0.9375, Status: "active"},
		},
		map[string]float64{"a-1": 1.0},
	)
	cred := map[string]float64{"s-sup": 0.96, "s-con": 0.9}
	s := computeIntrinsic(snap, cred)["c-adjud"]
	if s.Groundedness != v {
		t.Errorf("Groundedness = %v, want adjudicated value %v", s.Groundedness, v)
	}
	if s.EffectiveGroundedness != v {
		t.Errorf("EffectiveGroundedness = %v, want adjudicated value %v", s.EffectiveGroundedness, v)
	}
	if s.Contestation == 0 {
		t.Error("Contestation should reflect citation evidence, got 0")
	}
	if s.Contestation == v {
		t.Error("Contestation must not be pinned to the adjudicated value")
	}
	nearF(t, s.Contestation, 30.0/31.0, 1e-9)
}

func TestIsAdjudicated(t *testing.T) {
	v := 0.8
	snap := fixtureSnapshot(
		[]model.Claim{
			{ID: "c-adjud", Status: "adjudicated", AdjudicatedValue: &v},
			{ID: "c-normal"},
		},
		nil,
		nil,
	)
	if !isAdjudicated(snap, "c-adjud") {
		t.Error("expected adjudicated claim to be detected")
	}
	if isAdjudicated(snap, "c-normal") {
		t.Error("expected non-adjudicated claim to return false")
	}
	if isAdjudicated(snap, "c-unknown") {
		t.Error("expected unknown claimID to return false")
	}
}
