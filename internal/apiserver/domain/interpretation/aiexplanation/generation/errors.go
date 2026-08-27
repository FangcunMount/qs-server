package generation

import "errors"

var (
	ErrNotFound                         = errors.New("AI explanation generation not found")
	ErrAlreadyExists                    = errors.New("AI explanation generation already exists")
	ErrConflict                         = errors.New("AI explanation generation state conflict")
	ErrOrgDailyBudgetExceeded           = errors.New("AI explanation participant organization daily Provider invocation budget exceeded")
	ErrUserDailyBudgetExceeded          = errors.New("AI explanation participant user daily Provider invocation budget exceeded")
	ErrAssessmentDailyBudgetExceeded    = errors.New("AI explanation participant Assessment daily Provider invocation budget exceeded")
	ErrOrgActiveCapacityExceeded        = errors.New("AI explanation participant organization active Provider execution capacity exceeded")
	ErrUserActiveCapacityExceeded       = errors.New("AI explanation participant user active Provider execution capacity exceeded")
	ErrAssessmentActiveCapacityExceeded = errors.New("AI explanation participant Assessment active Provider execution capacity exceeded")
)
