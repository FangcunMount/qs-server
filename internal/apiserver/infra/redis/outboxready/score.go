package outboxready

import "time"

// scoreTieFactor multiplies due millis; dueMs*scoreTieFactor + tie must stay within float64 exact-int range (≤ 2^53).
const scoreTieFactor = 1000

// scoreTieMask keeps 22 low bits of created_at millis for FIFO tie-break among equal due times.
const scoreTieMask = (1 << 22) - 1

// ReadyScore builds a Redis ZSET score ordered by nextAttemptAt, then createdAt (FIFO).
// Layout: dueUnixMilli * scoreTieFactor + (createdAtUnixMilli & scoreTieMask).
// Millisecond due * 1e6 would exceed float64 precision; scoreTieFactor=1000 is the safe equivalent.
func ReadyScore(nextAttemptAt, createdAt time.Time) float64 {
	if createdAt.IsZero() {
		createdAt = nextAttemptAt
	}
	dueMs := nextAttemptAt.UnixMilli()
	tie := createdAt.UnixMilli() & scoreTieMask
	return float64(dueMs)*scoreTieFactor + float64(tie)
}
