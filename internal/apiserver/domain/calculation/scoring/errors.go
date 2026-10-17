package scoring

import "fmt"

// inputError identifies failures determined entirely by the frozen scoring
// input. Replaying that same input cannot resolve them.
type inputError struct {
	message string
}

func (e *inputError) Error() string { return e.message }

func (*inputError) Retryable() bool { return false }

func invalidInput(format string, args ...any) error {
	return &inputError{message: fmt.Sprintf(format, args...)}
}
