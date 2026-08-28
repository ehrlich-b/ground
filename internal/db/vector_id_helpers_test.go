package db

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestCosine(t *testing.T) {
	cases := []struct {
		name string
		a, b []float32
		want float64
	}{
		{"identical", []float32{1, 2, 3}, []float32{1, 2, 3}, 1},
		{"orthogonal", []float32{1, 0}, []float32{0, 1}, 0},
		{"opposite", []float32{1, 2, 3}, []float32{-1, -2, -3}, -1},
		{"mismatched lengths", []float32{1, 2}, []float32{1, 2, 3}, 0},
		{"both empty", []float32{}, []float32{}, 0},
		{"zero vector", []float32{0, 0, 0}, []float32{1, 2, 3}, 0},
	}
	for _, c := range cases {
		if got := cosine(c.a, c.b); got != c.want {
			t.Errorf("%s: cosine(%v, %v) = %v, want %v", c.name, c.a, c.b, got, c.want)
		}
	}
}

func TestUnmarshalVecRoundTrip(t *testing.T) {
	orig := []float32{1.5, -2.25, 0, 3.75}
	b := make([]byte, len(orig)*4)
	for i, f := range orig {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	got := unmarshalVec(b)
	if len(got) != len(orig) {
		t.Fatalf("unmarshalVec: len = %d, want %d", len(got), len(orig))
	}
	for i := range orig {
		if got[i] != orig[i] {
			t.Errorf("unmarshalVec: element %d = %v, want %v", i, got[i], orig[i])
		}
	}
}

func TestUnmarshalVecNilVsEmpty(t *testing.T) {
	if got := unmarshalVec([]byte{}); got != nil {
		t.Errorf("unmarshalVec(empty) = %v, want nil", got)
	}
	if got := unmarshalVec(nil); got != nil {
		t.Errorf("unmarshalVec(nil) = %v, want nil", got)
	}
}

func TestUnmarshalVecTruncates(t *testing.T) {
	want := float32(2.5)
	b := make([]byte, 5)
	binary.LittleEndian.PutUint32(b[:4], math.Float32bits(want))
	b[4] = 0xff
	got := unmarshalVec(b)
	if len(got) != 1 {
		t.Fatalf("unmarshalVec(5-byte input): len = %d, want exactly 1", len(got))
	}
	if got[0] != want {
		t.Errorf("unmarshalVec(5-byte input): element = %v, want %v (trailing byte must be dropped)", got[0], want)
	}
}

func TestGenerateID(t *testing.T) {
	id1 := GenerateID()
	if len(id1) != 32 {
		t.Fatalf("GenerateID: len = %d, want 32", len(id1))
	}
	for _, r := range id1 {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Errorf("GenerateID: non-lowercase-hex character %q in %q", r, id1)
		}
	}
	id2 := GenerateID()
	if id1 == id2 {
		t.Errorf("GenerateID: two calls returned the same value %q", id1)
	}
}
