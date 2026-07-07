package policy

// IdempotencyPolicy controls duplicate execution suppression for an evaluation run.
type IdempotencyPolicy struct {
	Key string
}

// DefaultIdempotencyPolicy returns a policy keyed by assessment ID.
func DefaultIdempotencyPolicy(assessmentID string) IdempotencyPolicy {
	return IdempotencyPolicy{Key: assessmentID}
}
