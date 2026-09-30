package eventruntime

import "errors"

// ErrAutomaticRetryPaused marks a business retry event that must be durably
// held instead of consuming transport retry budget.
var ErrAutomaticRetryPaused = errors.New("automatic business retry paused by emergency switch")
