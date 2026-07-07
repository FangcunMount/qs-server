package run

// FailureKind classifies why an evaluation attempt failed.
type FailureKind string

const (
	FailureKindValidation  FailureKind = "validation"
	FailureKindCalculation FailureKind = "calculation"
	FailureKindTimeout     FailureKind = "timeout"
	FailureKindInternal    FailureKind = "internal"
)

func (k FailureKind) String() string { return string(k) }

// Failure captures a terminal execution failure for one attempt.
type Failure struct {
	Kind      FailureKind
	Message   string
	Retryable bool
}
