package policy

// IdempotencyPolicy 控制重复执行抑制 用于 评估执行。
type IdempotencyPolicy struct {
	Key string
}

// 默认IdempotencyPolicy 返回策略 键ed 按 assessment ID。
func DefaultIdempotencyPolicy(assessmentID string) IdempotencyPolicy {
	return IdempotencyPolicy{Key: assessmentID}
}
