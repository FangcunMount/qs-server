package interpretation

import (
	"context"

	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
)

// Service generates and persists interpretation reports from scoring outcomes.
type Service interface {
	GenerateAndPersist(ctx context.Context, outcome evaluationresult.Outcome) error
}

type service struct {
	writer evaluationresult.InterpretationWriter
}

// NewService creates an interpretation orchestrator backed by a report writer.
func NewService(writer evaluationresult.InterpretationWriter) Service {
	return &service{writer: writer}
}

func (s *service) GenerateAndPersist(ctx context.Context, outcome evaluationresult.Outcome) error {
	if s == nil || s.writer == nil {
		return evaluationresult.ErrInterpretationWriterNotConfigured()
	}
	return s.writer.Write(ctx, outcome)
}
