package domain_test

import (
	"errors"
	"math"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/recommendation/domain"
)

// TestDefaultWeights pins the documented defaults so future
// migrations and the in-memory fallback don't silently drift.
func TestDefaultWeights(t *testing.T) {
	w := domain.DefaultWeights()
	if w.CoPurchase != 0.40 || w.Text != 0.20 || w.Category != 0.20 || w.Attributes != 0.10 || w.Price != 0.10 {
		t.Fatalf("unexpected defaults: %+v", w)
	}
	if math.Abs(w.Sum()-1.0) > 1e-9 {
		t.Fatalf("default sum should be 1, got %v", w.Sum())
	}
}

func TestNewWeights_RejectsNegative(t *testing.T) {
	_, err := domain.NewWeights(-0.1, 0, 0, 0, 0)
	if !errors.Is(err, domain.ErrInvalidWeights) {
		t.Fatalf("expected ErrInvalidWeights, got %v", err)
	}
}

func TestNewWeights_AcceptsArbitrarySum(t *testing.T) {
	// Sum is not forced to 1 — operators may use weights summing
	// to anything since only relative ordering matters.
	w, err := domain.NewWeights(2, 2, 2, 2, 2)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if w.Sum() != 10 {
		t.Fatalf("sum = %v, want 10", w.Sum())
	}
}

// TestScore_AllZeroInputs verifies that an all-zero ScoreInputs
// always scores 0 regardless of the weights (foot-gun guard).
func TestScore_AllZeroInputs(t *testing.T) {
	w := domain.DefaultWeights()
	if got := w.Score(domain.ScoreInputs{}); got != 0 {
		t.Fatalf("Score(zeros) = %v, want 0", got)
	}
}

// TestScore_AllOneInputs verifies that all-1 inputs with weights
// summing to 1 produce exactly 1 — the "every signal maxed out"
// reference point.
func TestScore_AllOneInputs(t *testing.T) {
	w := domain.DefaultWeights()
	in := domain.ScoreInputs{
		CoPurchaseNorm:   1,
		TextSimilarity:   1,
		SameCategory:     1,
		AttributeJaccard: 1,
		PriceProximity:   1,
	}
	if got := float64(w.Score(in)); math.Abs(got-1.0) > 1e-9 {
		t.Fatalf("Score(all=1) = %v, want 1", got)
	}
}

// TestScore_Linear pins the weighted-sum formula so a future
// refactor that subtly swaps two coefficients trips a test.
func TestScore_Linear(t *testing.T) {
	w, err := domain.NewWeights(0.5, 0.3, 0.1, 0.05, 0.05)
	if err != nil {
		t.Fatal(err)
	}
	in := domain.ScoreInputs{
		CoPurchaseNorm:   0.8,
		TextSimilarity:   0.6,
		SameCategory:     1.0,
		AttributeJaccard: 0.5,
		PriceProximity:   0.0,
	}
	got := float64(w.Score(in))
	want := 0.5*0.8 + 0.3*0.6 + 0.1*1.0 + 0.05*0.5 + 0.05*0.0
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("Score = %v, want %v", got, want)
	}
}

// TestNewPair_Symmetry verifies (a,b) and (b,a) collapse onto the
// same value object so the co-purchase map never double-counts.
func TestNewPair_Symmetry(t *testing.T) {
	p1 := domain.NewPair("apple", "banana")
	p2 := domain.NewPair("banana", "apple")
	if p1 != p2 {
		t.Fatalf("NewPair not symmetric: %v vs %v", p1, p2)
	}
	if p1.A != "apple" || p1.B != "banana" {
		t.Fatalf("NewPair did not sort lexically: %+v", p1)
	}
}

func TestPair_Other(t *testing.T) {
	p := domain.NewPair("apple", "banana")
	if got := p.Other("apple"); got != "banana" {
		t.Fatalf("Other(apple) = %q, want banana", got)
	}
	if got := p.Other("banana"); got != "apple" {
		t.Fatalf("Other(banana) = %q, want apple", got)
	}
	if got := p.Other("cherry"); got != "" {
		t.Fatalf("Other(unrelated) = %q, want empty string", got)
	}
}
