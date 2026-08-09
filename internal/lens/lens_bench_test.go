package lens

import (
	"fmt"
	"testing"

	"github.com/ehrlich-b/ground/internal/model"
)

// benchSnapshot builds a snapshot with n claims, one supporting citation each,
// and a dependency chain claim-i -> claim-(i-1) so the DAG flow visits every
// claim in topological order (each claim carries exactly one dep).
func benchSnapshot(n int) *Snapshot {
	claims := make([]model.Claim, 0, n)
	citations := make([]model.Citation, 0, n)
	deps := make([]model.Dependency, 0, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("claim-%05d", i)
		claims = append(claims, model.Claim{ID: id, Status: "active"})
		citations = append(citations, model.Citation{
			ClaimID:     id,
			SourceID:    "src-1",
			ExtractorID: "ext-1",
			Polarity:    "supports",
			Status:      "active",
			AuditFactor: 1,
		})
		if i > 0 {
			deps = append(deps, model.Dependency{
				ClaimID:     id,
				DependsOnID: fmt.Sprintf("claim-%05d", i-1),
				Strength:    1,
			})
		}
	}
	return &Snapshot{
		Sources:      map[string]*model.Source{"src-1": {ID: "src-1"}},
		BaseCred:     map[string]float64{"src-1": 0.9},
		AgentReliab:  map[string]float64{"ext-1": 0.9},
		Claims:       claims,
		Citations:    citations,
		Dependencies: deps,
	}
}

func BenchmarkRender(b *testing.B) {
	for _, n := range []int{1000, 10000, 50000} {
		snap := benchSnapshot(n)
		b.Run(fmt.Sprintf("claims=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				Render(snap, &LensSpec{})
			}
		})
	}
}
