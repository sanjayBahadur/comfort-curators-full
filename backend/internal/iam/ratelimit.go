package iam

import (
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*tokenBucket
	rate     int
	interval time.Duration
}

type tokenBucket struct {
	tokens     int
	lastRefill time.Time
}

func NewRateLimiter(rate int, interval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		buckets:  make(map[string]*tokenBucket),
		rate:     rate,
		interval: interval,
	}
	go rl.cleanup(interval * 2)
	return rl
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, ok := rl.buckets[key]
	if !ok {
		rl.buckets[key] = &tokenBucket{
			tokens:     rl.rate - 1,
			lastRefill: time.Now(),
		}
		return true
	}

	now := time.Now()
	elapsed := now.Sub(bucket.lastRefill)
	refillTokens := int(elapsed/rl.interval) * rl.rate

	if refillTokens > 0 {
		bucket.tokens = rl.rate
		bucket.lastRefill = now
	}

	if bucket.tokens <= 0 {
		return false
	}

	bucket.tokens--
	return true
}

func (rl *RateLimiter) cleanup(maxAge time.Duration) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, bucket := range rl.buckets {
			if now.Sub(bucket.lastRefill) > maxAge {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}
