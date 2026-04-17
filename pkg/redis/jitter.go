package rediskit

import (
	"math/rand"
	"sync"
	"time"
)

var (
	jitterRand = rand.New(rand.NewSource(time.Now().UnixNano()))
	jitterMu   sync.Mutex
)

// JitterTTL adds symmetric jitter around the input TTL using the provided ratio.
func JitterTTL(ttl time.Duration, ratio float64) time.Duration {
	if ttl <= 0 || ratio <= 0 {
		return ttl
	}
	if ratio > 1 {
		ratio = 1
	}

	maxJitter := time.Duration(float64(ttl) * ratio)
	if maxJitter <= 0 {
		return ttl
	}

	jitterMu.Lock()
	delta := jitterRand.Int63n(int64(maxJitter*2)+1) - int64(maxJitter)
	jitterMu.Unlock()

	result := ttl + time.Duration(delta)
	if result <= 0 {
		return time.Second
	}
	return result
}
