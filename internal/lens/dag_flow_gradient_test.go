package lens

import (
	"testing"

	"github.com/ehrlich-b/ground/internal/model"
)

const floatEps = 1e-9

func nearlyEqual(a, b float64) bool {
	return a-b < floatEps && b-a < floatEps
}

func TestPowApproxVerifiedCases(t *testing.T) {
	cases := []struct {
		name string
		a, b float64
		want float64
	}{
		{"b<=0 returns 1", 0.5, 0, 1},
		{"b>=1 returns a", 0.5, 1, 0.5},
		{"linear interpolation at b=0.5", 0.5, 0.5, 0.75},
		{"a clamped down from above 1", 1.5, 0.5, 1},
		{"a clamped up from below 0", -0.5, 0.5, 0.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := powApprox(tc.a, tc.b); !nearlyEqual(got, tc.want) {
				t.Errorf("powApprox(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestPowApproxClampMonotonic(t *testing.T) {
	as := []float64{-1, 0, 0.25, 0.5, 0.75, 1, 2}
	const b = 0.5
	prev := powApprox(as[0], b)
	for _, a := range as[1:] {
		got := powApprox(a, b)
		if got < prev-1e-12 {
			t.Errorf("powApprox not non-decreasing in a: powApprox(%v, %v) = %v < previous %v", a, b, got, prev)
		}
		prev = got
	}
}

func TestApplyDAGFlowNoDepsUntouched(t *testing.T) {
	snap := &Snapshot{
		Claims: []model.Claim{
			{ID: "isolated"},
			{ID: "dep-a"},
			{ID: "dep-b"},
		},
		Dependencies: []model.Dependency{
			{ClaimID: "dep-b", DependsOnID: "dep-a", Strength: 0.5},
		},
	}
	scores := map[string]ClaimScore{
		"isolated": {ClaimID: "isolated", Groundedness: 0.8, EffectiveGroundedness: 0.8},
		"dep-a":    {ClaimID: "dep-a", Groundedness: 0.5, EffectiveGroundedness: 0.5},
		"dep-b":    {ClaimID: "dep-b", Groundedness: 0.6, EffectiveGroundedness: 0.6},
	}
	applyDAGFlow(snap, scores)

	if got := scores["isolated"].EffectiveGroundedness; !nearlyEqual(got, 0.8) {
		t.Errorf("claim with no dep edges was modified: EffectiveGroundedness = %v, want 0.8", got)
	}
	wantDamped := 0.6 * powApprox(0.5, 0.5)
	if got := scores["dep-b"].EffectiveGroundedness; !nearlyEqual(got, wantDamped) {
		t.Errorf("sanity: dep-b should have been dampened to %v, got %v (proves the loop ran)", wantDamped, got)
	}
}

func TestApplyDAGFlowSingleDependency(t *testing.T) {
	snap := &Snapshot{
		Claims: []model.Claim{
			{ID: "child"},
			{ID: "parent"},
		},
		Dependencies: []model.Dependency{
			{ClaimID: "child", DependsOnID: "parent", Strength: 0.25},
		},
	}
	scores := map[string]ClaimScore{
		"child":  {ClaimID: "child", Groundedness: 0.8, EffectiveGroundedness: 0.8},
		"parent": {ClaimID: "parent", Groundedness: 0.6, EffectiveGroundedness: 0.6},
	}
	applyDAGFlow(snap, scores)

	want := 0.8 * (1 - 0.25*(1-0.6))
	if got := scores["child"].EffectiveGroundedness; !nearlyEqual(got, want) {
		t.Errorf("EffectiveGroundedness = %v, want %v (0.8 * powApprox(0.6, 0.25))", got, want)
	}
}

func TestApplyDAGFlowTwoDepsCompound(t *testing.T) {
	snap := &Snapshot{
		Claims: []model.Claim{
			{ID: "claim"},
			{ID: "dep-a"},
			{ID: "dep-b"},
		},
		Dependencies: []model.Dependency{
			{ClaimID: "claim", DependsOnID: "dep-a", Strength: 0.5},
			{ClaimID: "claim", DependsOnID: "dep-b", Strength: 0.5},
		},
	}
	scores := map[string]ClaimScore{
		"claim": {ClaimID: "claim", Groundedness: 0.7, EffectiveGroundedness: 0.7},
		"dep-a": {ClaimID: "dep-a", Groundedness: 0.5, EffectiveGroundedness: 0.5},
		"dep-b": {ClaimID: "dep-b", Groundedness: 0.8, EffectiveGroundedness: 0.8},
	}
	applyDAGFlow(snap, scores)

	factorA := powApprox(0.5, 0.5)
	factorB := powApprox(0.8, 0.5)
	want := 0.7 * factorA * factorB
	if got := scores["claim"].EffectiveGroundedness; !nearlyEqual(got, want) {
		t.Errorf("EffectiveGroundedness = %v, want %v (product of both dep factors, not one of them or their sum)", got, want)
	}
}

func TestApplyDAGFlowZeroEvidenceCannotBorrow(t *testing.T) {
	// A claim with zero citations of its own has Groundedness == 0 (computeIntrinsic
	// leaves it untouched). Depending on a well-grounded claim (EffectiveGroundedness
	// 0.516) cannot lift it: EffectiveGroundedness = Groundedness * factor, and 0
	// times any factor is 0. This is correct per the README's design — "a claim
	// resting on shaky foundations is weaker than it looks...": dependencies can
	// only dampen groundedness downward, never grant it. A claim cannot borrow
	// groundedness from something it depends on.
	snap := &Snapshot{
		Claims: []model.Claim{
			{ID: "child"},
			{ID: "parent"},
		},
		Dependencies: []model.Dependency{
			{ClaimID: "child", DependsOnID: "parent", Strength: 0.5},
		},
	}
	scores := map[string]ClaimScore{
		"child":  {ClaimID: "child", Groundedness: 0, EffectiveGroundedness: 0},
		"parent": {ClaimID: "parent", Groundedness: 0.516, EffectiveGroundedness: 0.516},
	}
	applyDAGFlow(snap, scores)

	if got := scores["child"].EffectiveGroundedness; !nearlyEqual(got, 0) {
		t.Errorf("zero-evidence claim borrowed groundedness: EffectiveGroundedness = %v, want 0", got)
	}
}

func TestApplyDAGFlowSkipsAdjudicated(t *testing.T) {
	adjVal := 0.9
	snap := &Snapshot{
		Claims: []model.Claim{
			{ID: "pinned", Status: "adjudicated", AdjudicatedValue: &adjVal},
			{ID: "shaky"},
		},
		Dependencies: []model.Dependency{
			{ClaimID: "pinned", DependsOnID: "shaky", Strength: 0.5},
		},
	}
	scores := map[string]ClaimScore{
		"pinned": {ClaimID: "pinned", Groundedness: 0.9, EffectiveGroundedness: 0.9},
		"shaky":  {ClaimID: "shaky", Groundedness: 0.1, EffectiveGroundedness: 0.1},
	}
	applyDAGFlow(snap, scores)

	if got := scores["pinned"].EffectiveGroundedness; !nearlyEqual(got, 0.9) {
		t.Errorf("adjudicated claim was dampened by its dependency: EffectiveGroundedness = %v, want 0.9", got)
	}
}

func TestGradientSupportsAndContradicts(t *testing.T) {
	snap := &Snapshot{
		Citations: []model.Citation{
			{ClaimID: "c1", SourceID: "src-a", Polarity: "supports", ExtractorID: "ext-1", AuditFactor: 1.0, Status: "active"},
			{ClaimID: "c1", SourceID: "src-b", Polarity: "contradicts", ExtractorID: "ext-1", AuditFactor: 1.0, Status: "active"},
		},
		AgentReliab: map[string]float64{"ext-1": 0.9},
	}
	got := Gradient(snap, nil, "c1", 0)
	if len(got) != 2 {
		t.Fatalf("Gradient returned %d impacts, want 2", len(got))
	}
	bySource := map[string]float64{}
	for _, imp := range got {
		bySource[imp.SourceID] = imp.Delta
	}
	// Both deltas have equal |delta| so sort order between them is not
	// guaranteed; verify the values as a set. Each contribution is
	// polarity_coef * AuditFactor * extractor reliability, with no source
	// credibility term — Gradient is ∂(intrinsic)/∂credibility.
	if !nearlyEqual(bySource["src-a"], 0.9) {
		t.Errorf("src-a delta = %v, want 0.9", bySource["src-a"])
	}
	if !nearlyEqual(bySource["src-b"], -0.9) {
		t.Errorf("src-b delta = %v, want -0.9", bySource["src-b"])
	}
}

func gradientSnapshot() *Snapshot {
	return &Snapshot{
		Citations: []model.Citation{
			{ClaimID: "c1", SourceID: "src-a", Polarity: "supports", ExtractorID: "ext-a", AuditFactor: 1.0, Status: "active"},
			{ClaimID: "c1", SourceID: "src-b", Polarity: "contradicts", ExtractorID: "ext-b", AuditFactor: 1.0, Status: "active"},
			{ClaimID: "c1", SourceID: "src-c", Polarity: "supports", ExtractorID: "ext-c", AuditFactor: 1.0, Status: "active"},
		},
		AgentReliab: map[string]float64{
			"ext-a": 0.9,
			"ext-b": 0.7,
			"ext-c": 0.5,
		},
	}
}

func TestGradientTopNTruncation(t *testing.T) {
	got := Gradient(gradientSnapshot(), nil, "c1", 1)
	if len(got) != 1 {
		t.Fatalf("topN=1 returned %d impacts, want 1", len(got))
	}
	if got[0].SourceID != "src-a" || !nearlyEqual(got[0].Delta, 0.9) {
		t.Errorf("topN=1 returned %+v, want the single largest-|delta| source src-a (+0.9)", got[0])
	}

	// Fewer impacts than topN: returns all of them.
	gotAll := Gradient(gradientSnapshot(), nil, "c1", 5)
	if len(gotAll) != 3 {
		t.Fatalf("topN=5 with 3 sources returned %d impacts, want 3", len(gotAll))
	}
}

func TestGradientTopNUnbounded(t *testing.T) {
	for _, topN := range []int{0, -1} {
		got := Gradient(gradientSnapshot(), nil, "c1", topN)
		if len(got) != 3 {
			t.Fatalf("topN=%d returned %d impacts, want all 3", topN, len(got))
		}
	}
}
