package result

import (
	"context"

	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
)

// InterpretationWriter persists reports and transitions Assessment to interpreted.
type InterpretationWriter interface {
	Write(ctx context.Context, outcome Outcome) error
}

// ErrInterpretationWriterNotConfigured reports a missing interpretation writer dependency.
func ErrInterpretationWriterNotConfigured() error {
	return interpretationreporting.ErrWriterNotConfigured()
}
