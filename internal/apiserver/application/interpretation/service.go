package interpretation

import (
	"context"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
)

// Service generates and persists interpretation reports from scoring outcomes.
type Service interface {
	GenerateAndPersist(ctx context.Context, outcome evaloutcome.Outcome) error
}

type service struct {
	writer interpretationreporting.Writer
}

// NewService creates an interpretation orchestrator backed by a report writer.
func NewService(writer interpretationreporting.Writer) Service {
	return &service{writer: writer}
}

func (s *service) GenerateAndPersist(ctx context.Context, outcome evaloutcome.Outcome) error {
	if s == nil || s.writer == nil {
		return interpretationreporting.ErrWriterNotConfigured()
	}
	return s.writer.Write(ctx, outcome)
}
