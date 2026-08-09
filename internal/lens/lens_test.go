package lens

import (
	"strings"
	"testing"

	"github.com/ehrlich-b/ground/internal/model"
)

func TestPowApprox(t *testing.T) {
	tests := []struct {
		name string
		a, b float64
		want float64
	}{
		{"quarter to the half power", 0.25, 0.5, 0.5},
		{"a equals zero", 0, 0.5, 0},
		{"a equals one", 1, 0.5, 1},
		{"b equals zero", 0.25, 0, 1},
		{"b equals one", 0.25, 1, 0.25},
		{"a negative clamps to zero", -2, 0.5, 0},
		{"a above one clamps to one", 3, 0.5, 1},
		{"a and b both zero", 0, 0, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := powApprox(tc.a, tc.b); absf(got-tc.want) > 1e-12 {
				t.Errorf("powApprox(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestRenderDetectsDependencyCycle(t *testing.T) {
	snap := &Snapshot{
		Claims: []model.Claim{
			{ID: "A", Status: "active"},
			{ID: "B", Status: "active"},
		},
		Dependencies: []model.Dependency{
			{ClaimID: "A", DependsOnID: "B", Strength: 1},
			{ClaimID: "B", DependsOnID: "A", Strength: 1},
		},
	}

	scores, err := RenderChecked(snap, &LensSpec{})
	if err == nil {
		t.Fatalf("RenderChecked(A<->B) returned nil error; expected dependency cycle detection")
	}
	for _, id := range []string{"A", "B"} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("cycle error %q does not name stuck claim %q", err.Error(), id)
		}
	}
	if _, ok := scores["A"]; !ok {
		t.Errorf("expected A present in scores even though it is stuck in a cycle")
	}
	if _, ok := scores["B"]; !ok {
		t.Errorf("expected B present in scores even though it is stuck in a cycle")
	}
}
