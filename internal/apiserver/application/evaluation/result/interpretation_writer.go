package result

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
)

// InterpretationWriter persists reports and transitions Assessment to interpreted.
type InterpretationWriter interface {
	Write(ctx context.Context, outcome Outcome) error
}

// ErrInterpretationWriterNotConfigured reports a missing interpretation writer dependency.
func ErrInterpretationWriterNotConfigured() error {
	return apperrors.ModuleNotConfigured("interpretation writer is not configured")
}
