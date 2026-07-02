package cacheutil

import (
	"math/rand"
	"time"
)

// JitterTTL 在 base TTL 上增加 [0, base*ratio] 的随机抖动，spread 集体过期尖刺。
func JitterTTL(base time.Duration, ratio float64) time.Duration {
	if base <= 0 {
		return base
	}
	if ratio <= 0 {
		return base
	}
	if ratio > 1 {
		ratio = 1
	}
	extra := time.Duration(rand.Float64() * float64(base) * ratio)
	return base + extra
}
