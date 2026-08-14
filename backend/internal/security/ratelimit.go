package security

import (
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*tokenBucket
	rate     int
	burst    int
	interval time.Duration
}

type tokenBucket struct {
	tokens   float64
	lastTime time.Time
}

func NewRateLimiter(rate int, burst int, interval time.Duration) *RateLimiter {
	return &RateLimiter{
		buckets:  make(map[string]*tokenBucket),
		rate:     rate,
		burst:    burst,
		interval: interval,
	}
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, ok := rl.buckets[key]
	if !ok {
		bucket = &tokenBucket{
			tokens:   float64(rl.burst),
			lastTime: time.Now(),
		}
		rl.buckets[key] = bucket
	}

	now := time.Now()
	elapsed := now.Sub(bucket.lastTime).Seconds()
	bucket.tokens += elapsed * float64(rl.rate) / rl.interval.Seconds()
	if bucket.tokens > float64(rl.burst) {
		bucket.tokens = float64(rl.burst)
	}
	bucket.lastTime = now

	if bucket.tokens < 1 {
		return false
	}

	bucket.tokens--
	return true
}

func (rl *RateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.buckets, key)
}

func (rl *RateLimiter) Cleanup(maxAge time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	for key, bucket := range rl.buckets {
		if bucket.lastTime.Before(cutoff) {
			delete(rl.buckets, key)
		}
	}
}

type RateLimitConfig struct {
	Enabled  bool
	Requests int
	Burst    int
	Interval time.Duration
}

func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Enabled:  true,
		Requests: 100,
		Burst:    20,
		Interval: time.Minute,
	}
}

func AbuseDetectorRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Enabled:  true,
		Requests: 5,
		Burst:    2,
		Interval: time.Minute,
	}
}
