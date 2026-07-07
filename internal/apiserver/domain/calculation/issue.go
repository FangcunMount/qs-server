package calculation

// Issue records a validation finding on calculation inputs or results.
type Issue struct {
	Code    string
	Message string
}

// NewIssue constructs a validation issue with code and message.
func NewIssue(code, message string) Issue {
	return Issue{Code: code, Message: message}
}
