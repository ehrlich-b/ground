package engine

import (
	"testing"

	"github.com/ehrlich-b/ground/internal/lens"
	"github.com/ehrlich-b/ground/internal/model"
)

func mustDefaultConfig(t *testing.T) Config {
	t.Helper()
	cfg := DefaultConfig()
	if cfg.ContestedThresh != 0.5 || cfg.GroundedThreshold != 0.7 || cfg.RefutedThreshold != 0.3 {
		t.Fatalf("unexpected default thresholds: contestation=%v grounded=%v refuted=%v",
			cfg.ContestedThresh, cfg.GroundedThreshold, cfg.RefutedThreshold)
	}
	return cfg
}

func TestClassifyStatus(t *testing.T) {
	cfg := mustDefaultConfig(t)

	tests := []struct {
		name  string
		score lens.ClaimScore
		want  string
	}{
		{
			// Contestation >= 0.5: contested fires before groundedness is ever consulted.
			// EffectiveGroundedness 0.9 would say "grounded" if the order were reversed,
			// so only the contestation branch can produce this result.
			name:  "contested",
			score: lens.ClaimScore{Contestation: 0.6, EffectiveGroundedness: 0.9},
			want:  "contested",
		},
		{
			// Below contestation threshold, groundedness >= 0.7 -> grounded.
			// Contestation 0.4 excludes "contested", groundedness 0.9 excludes refuted/active.
			name:  "grounded",
			score: lens.ClaimScore{Contestation: 0.4, EffectiveGroundedness: 0.9},
			want:  "grounded",
		},
		{
			// Below contestation threshold, groundedness <= 0.3 -> refuted.
			// Contestation 0.4 excludes "contested", groundedness 0.1 excludes grounded/active.
			name:  "refuted",
			score: lens.ClaimScore{Contestation: 0.4, EffectiveGroundedness: 0.1},
			want:  "refuted",
		},
		{
			// Strictly between 0.3 and 0.7 with contestation below threshold:
			// the only branch that can fire is the final fallthrough.
			name:  "active",
			score: lens.ClaimScore{Contestation: 0.4, EffectiveGroundedness: 0.5},
			want:  "active",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyStatus(tt.score, cfg); got != tt.want {
				t.Errorf("classifyStatus(%+v) = %q, want %q", tt.score, got, tt.want)
			}
		})
	}
}

func TestClassifyStatusContestationPrecedence(t *testing.T) {
	cfg := mustDefaultConfig(t)

	score := lens.ClaimScore{Contestation: 0.6, EffectiveGroundedness: 0.9}
	if got := classifyStatus(score, cfg); got != "contested" {
		t.Fatalf("classifyStatus(%+v) = %q, want %q: high contestation must override groundedness regardless of check order",
			score, got, "contested")
	}
}

func TestClassifyStatusGroundednessBoundariesInclusive(t *testing.T) {
	cfg := mustDefaultConfig(t)

	t.Run("grounded at exactly 0.7", func(t *testing.T) {
		score := lens.ClaimScore{Contestation: 0.4, EffectiveGroundedness: 0.7}
		if got := classifyStatus(score, cfg); got != "grounded" {
			t.Errorf("classifyStatus(%+v) = %q, want %q (>= boundary is inclusive)", score, got, "grounded")
		}
	})

	t.Run("refuted at exactly 0.3", func(t *testing.T) {
		score := lens.ClaimScore{Contestation: 0.4, EffectiveGroundedness: 0.3}
		if got := classifyStatus(score, cfg); got != "refuted" {
			t.Errorf("classifyStatus(%+v) = %q, want %q (<= boundary is inclusive)", score, got, "refuted")
		}
	})
}

func TestMaxDelta(t *testing.T) {
	t.Run("missing key in b is skipped", func(t *testing.T) {
		a := map[string]float64{"a": 1.0, "onlyA": 9.0}
		b := map[string]float64{"a": 1.5}
		if got := maxDelta(a, b); got != 0.5 {
			t.Errorf("maxDelta(%v, %v) = %v, want 0.5 (key present only in a must be ignored)", a, b, got)
		}
	})

	t.Run("all keys present in both maps", func(t *testing.T) {
		a := map[string]float64{"a": 1.0, "b": 2.0}
		b := map[string]float64{"a": 1.5, "b": 1.0}
		if got := maxDelta(a, b); got != 1.0 {
			t.Errorf("maxDelta(%v, %v) = %v, want 1.0", a, b, got)
		}
	})

	t.Run("empty maps", func(t *testing.T) {
		if got := maxDelta(map[string]float64{}, map[string]float64{}); got != 0 {
			t.Errorf("maxDelta(empty, empty) = %v, want 0", got)
		}
	})
}

func TestIsAdjudicated(t *testing.T) {
	snap := &lens.Snapshot{
		Claims: []model.Claim{
			{ID: "c-adjudicated", Status: "adjudicated"},
			{ID: "c-active", Status: "active"},
		},
	}

	t.Run("known adjudicated claim", func(t *testing.T) {
		if !isAdjudicated(snap, "c-adjudicated") {
			t.Error("isAdjudicated = false for an adjudicated claim, want true")
		}
	})

	t.Run("known non-adjudicated claim", func(t *testing.T) {
		if isAdjudicated(snap, "c-active") {
			t.Error("isAdjudicated = true for a non-adjudicated claim, want false")
		}
	})

	t.Run("unknown claimID absent from snapshot", func(t *testing.T) {
		if isAdjudicated(snap, "c-missing") {
			t.Error("isAdjudicated = true for an unknown claimID, want false")
		}
	})
}
