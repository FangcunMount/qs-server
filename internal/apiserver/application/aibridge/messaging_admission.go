package aibridge

import (
	"errors"
	"github.com/FangcunMount/qs-server/internal/pkg/aicommand"
)

// ErrRuntimeAdmissionClosed is a definitive local refusal before new submission.
// Storage failures must never be mapped to this error.
var ErrRuntimeAdmissionClosed = errors.New(RuntimeAdmissionClosedReason)

const RuntimeAdmissionClosedReason = aicommand.AdmissionClosedReason
