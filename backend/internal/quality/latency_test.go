package quality

import (
	"math"
	"testing"
)

func TestLatencySeriesP95NearestRank(t *testing.T) {
	// Nearest-rank p95 for 20 samples [1..20]: rank = ceil(0.95*20) = 19,
	// so p95 must equal the 19th smallest value = 19.
	s := NewLatencySeries()
	for i := 1; i <= 20; i++ {
		s.Observe(float64(i))
	}
	if got := s.P95(); got != 19 {
		t.Errorf("p95 of 1..20 = %v, want 19", got)
	}
	if got := s.Count(); got != 20 {
		t.Errorf("count = %d, want 20", got)
	}
}

func TestLatencySeriesPercentileEdgeCases(t *testing.T) {
	s := NewLatencySeries()
	if got := s.P95(); got != 0 {
		t.Errorf("empty series p95 = %v, want 0", got)
	}
	if got := s.Mean(); got != 0 {
		t.Errorf("empty series mean = %v, want 0", got)
	}
	if got := s.Max(); got != 0 {
		t.Errorf("empty series max = %v, want 0", got)
	}

	s.Observe(7)
	if got := s.P95(); got != 7 {
		t.Errorf("single sample p95 = %v, want 7", got)
	}
}

func TestLatencySeriesP95IndependentOfInsertionOrder(t *testing.T) {
	a := NewLatencySeries()
	for _, v := range []float64{100, 1, 50, 3, 200, 150, 2, 90} {
		a.Observe(v)
	}
	b := NewLatencySeries()
	for _, v := range []float64{2, 100, 150, 1, 90, 3, 200, 50} {
		b.Observe(v)
	}
	if a.P95() != b.P95() {
		t.Errorf("p95 must be order-independent: %v vs %v", a.P95(), b.P95())
	}
}

func TestLatencySeriesSummaryWithinTarget(t *testing.T) {
	s := NewLatencySeries()
	for i := 0; i < 40; i++ {
		s.Observe(float64(120 + i)) // p95 well below 500ms
	}
	sum := s.Summary("/v1/properties", P95TargetMilliseconds)
	if !sum.WithinTarget {
		t.Errorf("expected p95 %v <= target %v", sum.P95MS, sum.TargetMS)
	}
	if sum.Samples != 40 {
		t.Errorf("summary samples = %d, want 40", sum.Samples)
	}
	if math.Abs(sum.P95MS-s.P95()) > 0.011 {
		t.Errorf("summary p95 %v does not match series p95 %v", sum.P95MS, s.P95())
	}
}

func TestLatencySeriesSummaryOutsideTarget(t *testing.T) {
	s := NewLatencySeries()
	for i := 0; i < 40; i++ {
		s.Observe(700 + float64(i)) // p95 well above 500ms
	}
	sum := s.Summary("/v1/tickets", P95TargetMilliseconds)
	if sum.WithinTarget {
		t.Errorf("expected p95 %v to exceed target %v", sum.P95MS, sum.TargetMS)
	}
}
