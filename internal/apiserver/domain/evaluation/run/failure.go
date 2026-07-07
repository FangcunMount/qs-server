package run

// FailureKind 划分why 评估 尝试 失败。
type FailureKind string

const (
	FailureKindValidation  FailureKind = "validation"
	FailureKindCalculation FailureKind = "calculation"
	FailureKindTimeout     FailureKind = "timeout"
	FailureKindInternal    FailureKind = "internal"
)

func (k FailureKind) String() string { return string(k) }

// Failure 记录终态执行失败 用于 一个尝试。
type Failure struct {
	Kind      FailureKind
	Message   string
	Retryable bool
}
