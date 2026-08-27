package evaluation

import "errors"

var (
	ErrNotFound               = errors.New("AI explanation Prompt evaluation run not found")
	ErrAlreadyExists          = errors.New("AI explanation Prompt evaluation run already exists")
	ErrConflict               = errors.New("AI explanation Prompt evaluation run state conflict")
	ErrOrgConcurrencyExceeded = errors.New("AI explanation Prompt evaluation organization concurrency exceeded")
	ErrDailyBudgetExceeded    = errors.New("AI explanation Prompt evaluation daily Provider invocation budget exceeded")
)
