package binding

// ValidationLevel 划分校验问题 severity。
type ValidationLevel string

const (
	ValidationLevelError   ValidationLevel = "error"
	ValidationLevelWarning ValidationLevel = "warning"
)

// DomainValidationIssue 是structured 校验发现 at 领域层。
type DomainValidationIssue struct {
	Field   string
	Message string
	Code    string
	Level   ValidationLevel
}

// DomainValidationResult 聚合 领域校验发现s。
type DomainValidationResult struct {
	Issues []DomainValidationIssue
}

func (r DomainValidationResult) Passed() bool {
	if len(r.Issues) == 0 {
		return true
	}
	for _, issue := range r.Issues {
		if issue.Level == "" || issue.Level == ValidationLevelError {
			return false
		}
	}
	return true
}
