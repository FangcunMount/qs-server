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

// HasValidationErrors reports whether issues contain a blocking validation error.
func HasValidationErrors(issues []DomainValidationIssue) bool {
	for _, issue := range issues {
		if issue.Level == "" || issue.Level == ValidationLevelError {
			return true
		}
	}
	return false
}

func (r DomainValidationResult) Passed() bool {
	return !HasValidationErrors(r.Issues)
}
