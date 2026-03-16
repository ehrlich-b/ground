package embed

import (
	"math"
	"testing"
)

func TestMarshalUnmarshalVector(t *testing.T) {
	orig := []float32{1.0, -0.5, 0.0, 3.14, -2.718}
	b := MarshalVector(orig)
	got := UnmarshalVector(b)

	if len(got) != len(orig) {
		t.Fatalf("length mismatch: got %d, want %d", len(got), len(orig))
	}
	for i := range orig {
		if got[i] != orig[i] {
			t.Errorf("index %d: got %f, want %f", i, got[i], orig[i])
		}
	}
}

func TestUnmarshalEmpty(t *testing.T) {
	got := UnmarshalVector(nil)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	got = UnmarshalVector([]byte{})
	if got != nil {
		t.Errorf("expected nil for empty, got %v", got)
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a, b []float32
		want float64
	}{
		{"identical", []float32{1, 0, 0}, []float32{1, 0, 0}, 1.0},
		{"opposite", []float32{1, 0, 0}, []float32{-1, 0, 0}, -1.0},
		{"orthogonal", []float32{1, 0, 0}, []float32{0, 1, 0}, 0.0},
		{"similar", []float32{1, 1, 0}, []float32{1, 0, 0}, 1.0 / math.Sqrt(2)},
		{"empty", []float32{}, []float32{}, 0.0},
		{"length mismatch", []float32{1, 0}, []float32{1, 0, 0}, 0.0},
		{"zero vector", []float32{0, 0, 0}, []float32{1, 0, 0}, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CosineSimilarity(tt.a, tt.b)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("got %f, want %f", got, tt.want)
			}
		})
	}
}

func TestCosineDistance(t *testing.T) {
	d := CosineDistance([]float32{1, 0, 0}, []float32{1, 0, 0})
	if math.Abs(d) > 1e-9 {
		t.Errorf("identical vectors should have distance 0, got %f", d)
	}
	d = CosineDistance([]float32{1, 0, 0}, []float32{-1, 0, 0})
	if math.Abs(d-2.0) > 1e-9 {
		t.Errorf("opposite vectors should have distance 2, got %f", d)
	}
}

func TestFindNearestTopics(t *testing.T) {
	query := []float32{1, 0, 0}
	topics := []TopicWithEmbedding{
		{ID: "a", Slug: "a", Title: "A", Embedding: MarshalVector([]float32{1, 0, 0})},
		{ID: "b", Slug: "b", Title: "B", Embedding: MarshalVector([]float32{0, 1, 0})},
		{ID: "c", Slug: "c", Title: "C", Embedding: MarshalVector([]float32{0.9, 0.1, 0})},
	}

	matches := FindNearestTopics(query, topics, 2)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].TopicID != "a" {
		t.Errorf("expected first match to be 'a', got %q", matches[0].TopicID)
	}
	if matches[1].TopicID != "c" {
		t.Errorf("expected second match to be 'c', got %q", matches[1].TopicID)
	}
}

func TestCheckExclusions(t *testing.T) {
	query := []float32{1, 0, 0}

	// Within threshold (distance < 0.3)
	anchors := []ExclusionAnchor{
		{ID: "ex1", Description: "bad topic", Embedding: MarshalVector([]float32{0.95, 0.05, 0}), Threshold: 0.3},
	}
	got := CheckExclusions(query, anchors)
	if got == nil {
		t.Fatal("expected exclusion match, got nil")
	}
	if got.ID != "ex1" {
		t.Errorf("expected ex1, got %q", got.ID)
	}

	// Outside threshold
	anchors = []ExclusionAnchor{
		{ID: "ex2", Description: "far topic", Embedding: MarshalVector([]float32{0, 1, 0}), Threshold: 0.3},
	}
	got = CheckExclusions(query, anchors)
	if got != nil {
		t.Errorf("expected no exclusion match, got %v", got)
	}
}

func TestFindDuplicates(t *testing.T) {
	query := []float32{1, 0, 0}
	claims := []ClaimWithEmbedding{
		{ID: "c1", Proposition: "same", Embedding: MarshalVector([]float32{1, 0, 0})},
		{ID: "c2", Proposition: "close", Embedding: MarshalVector([]float32{0.98, 0.02, 0})},
		{ID: "c3", Proposition: "different", Embedding: MarshalVector([]float32{0, 1, 0})},
	}

	dupes := FindDuplicates(query, claims, 0.95)
	if len(dupes) != 2 {
		t.Fatalf("expected 2 duplicates, got %d", len(dupes))
	}
	if dupes[0].ClaimID != "c1" {
		t.Errorf("expected first dupe to be c1, got %q", dupes[0].ClaimID)
	}
}
