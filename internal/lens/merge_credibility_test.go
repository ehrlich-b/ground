package lens

import (
	"math"
	"testing"

	"github.com/ehrlich-b/ground/internal/model"
)

const eps = 1e-9

func assertFloatEq(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > eps {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func assertCredEq(t *testing.T, got map[string]float64, want map[string]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("cred len = %d, want %d (got=%v want=%v)", len(got), len(want), got, want)
	}
	for k, v := range want {
		g, ok := got[k]
		if !ok {
			t.Errorf("cred[%q] missing, want %v", k, v)
			continue
		}
		if math.Abs(g-v) > eps {
			t.Errorf("cred[%q] = %v, want %v", k, g, v)
		}
	}
}

func TestMergeCredibilityNilSpecReturnsCopy(t *testing.T) {
	snap := &Snapshot{
		BaseCred: map[string]float64{"src-a": 0.8, "src-b": 0.4},
	}
	got := mergeCredibility(snap, nil)

	assertCredEq(t, got, map[string]float64{"src-a": 0.8, "src-b": 0.4})

	got["src-a"] = 99
	if snap.BaseCred["src-a"] != 0.8 {
		t.Errorf("mutating result leaked into snap.BaseCred: got %v, want 0.8", snap.BaseCred["src-a"])
	}
}

func TestMergeCredibilityTagMultiplierPlusOverrideDifferentSource(t *testing.T) {
	snap := &Snapshot{
		BaseCred:   map[string]float64{"src-a": 0.8, "src-b": 0.4},
		SourceTags: map[string][]string{"src-a": {"news"}},
	}
	spec := &LensSpec{
		TagOverrides: []model.LensTagOverride{{Tag: "news", Multiplier: 1.2}},
		Overrides:    []model.LensOverride{{SourceID: "src-b", Mode: "absolute", Value: 0.9}},
	}
	got := mergeCredibility(snap, spec)
	assertCredEq(t, got, map[string]float64{"src-a": 0.8 * 1.2, "src-b": 0.9})
}

func TestMergeCredibilityOverrideModes(t *testing.T) {
	tests := []struct {
		name string
		spec *LensSpec
		snap *Snapshot
		want map[string]float64
	}{
		{
			name: "absolute replaces",
			snap: &Snapshot{
				BaseCred:   map[string]float64{"src-a": 0.8, "src-b": 0.4},
				SourceTags: map[string][]string{},
			},
			spec: &LensSpec{Overrides: []model.LensOverride{{SourceID: "src-a", Mode: "absolute", Value: 0.9}}},
			want: map[string]float64{"src-a": 0.9, "src-b": 0.4},
		},
		{
			name: "multiplier scales",
			snap: &Snapshot{
				BaseCred:   map[string]float64{"src-a": 0.8, "src-b": 0.4},
				SourceTags: map[string][]string{},
			},
			spec: &LensSpec{Overrides: []model.LensOverride{{SourceID: "src-a", Mode: "multiplier", Value: 0.5}}},
			want: map[string]float64{"src-a": 0.4, "src-b": 0.4},
		},
		{
			name: "exclude zeroes target only",
			snap: &Snapshot{
				BaseCred:   map[string]float64{"src-a": 0.8, "src-b": 0.4},
				SourceTags: map[string][]string{},
			},
			spec: &LensSpec{Overrides: []model.LensOverride{{SourceID: "src-a", Mode: "exclude"}}},
			want: map[string]float64{"src-a": 0, "src-b": 0.4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeCredibility(tt.snap, tt.spec)
			assertCredEq(t, got, tt.want)
		})
	}
}

func TestMergeCredibilityOverrideOnAbsentBaseCredSource(t *testing.T) {
	snap := &Snapshot{
		BaseCred:   map[string]float64{"src-a": 0.8},
		SourceTags: map[string][]string{},
	}

	absolute := mergeCredibility(snap, &LensSpec{
		Overrides: []model.LensOverride{{SourceID: "ghost", Mode: "absolute", Value: 0.7}},
	})
	if _, ok := absolute["ghost"]; !ok {
		t.Fatalf("absolute override on absent source did not write key: got %v", absolute)
	}
	assertFloatEq(t, "absolute on absent", absolute["ghost"], 0.7)

	multiplier := mergeCredibility(snap, &LensSpec{
		Overrides: []model.LensOverride{{SourceID: "ghost", Mode: "multiplier", Value: 0.5}},
	})
	if _, ok := multiplier["ghost"]; !ok {
		t.Fatalf("multiplier override on absent source did not write key: got %v", multiplier)
	}
	assertFloatEq(t, "multiplier on absent", multiplier["ghost"], 0)
}

func TestMergeCredibilityOverrideBeatsTagMultiplierSameSource(t *testing.T) {
	snap := &Snapshot{
		BaseCred:   map[string]float64{"src-a": 0.8},
		SourceTags: map[string][]string{"src-a": {"news"}},
	}
	spec := &LensSpec{
		TagOverrides: []model.LensTagOverride{{Tag: "news", Multiplier: 1.2}},
		Overrides:    []model.LensOverride{{SourceID: "src-a", Mode: "absolute", Value: 0.9}},
	}
	got := mergeCredibility(snap, spec)
	assertCredEq(t, got, map[string]float64{"src-a": 0.9})
}

func TestMergeCredibilityMultipleTagsCompound(t *testing.T) {
	snap := &Snapshot{
		BaseCred:   map[string]float64{"src-a": 0.8},
		SourceTags: map[string][]string{"src-a": {"news", "state-funded"}},
	}
	spec := &LensSpec{
		TagOverrides: []model.LensTagOverride{
			{Tag: "news", Multiplier: 1.2},
			{Tag: "state-funded", Multiplier: 0.5},
		},
	}
	got := mergeCredibility(snap, spec)
	assertCredEq(t, got, map[string]float64{"src-a": 0.8 * 1.2 * 0.5})
}

func TestPolarityCoef(t *testing.T) {
	tests := []struct {
		pol  string
		want float64
	}{
		{"supports", CoefSupports},
		{"contradicts", CoefContradicts},
		{"qualifies", CoefQualifies},
		{"", 0},
		{"Supports", 0},
		{"supprt", 0},
		{"neither", 0},
	}

	for _, tt := range tests {
		got := polarityCoef(tt.pol)
		if got != tt.want {
			t.Errorf("polarityCoef(%q) = %v, want %v", tt.pol, got, tt.want)
		}
	}
}
