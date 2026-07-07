package run

// Status 跟踪execution-phase progress 用于 一个评估 尝试。
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

func (s Status) String() string { return string(s) }

// Attempt 记录一个评估执行 try。
type Attempt struct {
	Number int
	Status Status
}

// NewAttempt 创建首个 执行尝试。
func NewAttempt() Attempt {
	return Attempt{Number: 1, Status: StatusPending}
}
