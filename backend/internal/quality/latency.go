package quality

import (
	"math"
	"sort"
)

// LatencySeries records response latencies in milliseconds and computes
// percentile summaries (including p95) without external dependencies. It is
// used to verify the NFR-003 p95 server-response target for core server paths.
type LatencySeries struct {
	samples []float64
}

// NewLatencySeries returns an empty latency series.
func NewLatencySeries() *LatencySeries {
	return &LatencySeries{}
}

// Observe appends a single latency sample measured in milliseconds.
func (s *LatencySeries) Observe(ms float64) {
	s.samples = append(s.samples, ms)
}

// Count returns the number of recorded samples.
func (s *LatencySeries) Count() int {
	return len(s.samples)
}

// Mean returns the arithmetic mean latency in milliseconds.
func (s *LatencySeries) Mean() float64 {
	if len(s.samples) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range s.samples {
		sum += v
	}
	return sum / float64(len(s.samples))
}

// Max returns the largest recorded latency in milliseconds.
func (s *LatencySeries) Max() float64 {
	if len(s.samples) == 0 {
		return 0
	}
	max := s.samples[0]
	for _, v := range s.samples[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

// Percentile returns the p-th percentile latency in milliseconds using the
// nearest-rank method. It returns 0 for an empty series.
func (s *LatencySeries) Percentile(p float64) float64 {
	n := len(s.samples)
	if n == 0 {
		return 0
	}
	sorted := make([]float64, n)
	copy(sorted, s.samples)
	sort.Float64s(sorted)

	rank := int(math.Ceil(p / 100.0 * float64(n)))
	if rank < 1 {
		rank = 1
	}
	if rank > n {
		rank = n
	}
	return sorted[rank-1]
}

// P95 returns the p95 latency in milliseconds.
func (s *LatencySeries) P95() float64 {
	return s.Percentile(95)
}

// LatencySummary is the recorded shape of one measured server path.
type LatencySummary struct {
	Path         string  `json:"path"`
	Samples      int     `json:"samples"`
	MeanMS       float64 `json:"mean_ms"`
	P95MS        float64 `json:"p95_ms"`
	MaxMS        float64 `json:"max_ms"`
	TargetMS     float64 `json:"target_ms"`
	WithinTarget bool    `json:"within_target"`
}

// Summary renders a LatencySeries as the recorded LatencySummary for a path.
func (s *LatencySeries) Summary(path string, targetMS float64) LatencySummary {
	p95 := s.P95()
	return LatencySummary{
		Path:         path,
		Samples:      s.Count(),
		MeanMS:       round2(s.Mean()),
		P95MS:        round2(p95),
		MaxMS:        round2(s.Max()),
		TargetMS:     targetMS,
		WithinTarget: p95 <= targetMS,
	}
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
