package writer

import (
	"context"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type Generation struct {
	Report *domainreport.InterpretReport
	Events []event.DomainEvent
}

type Generator interface {
	Generate(ctx context.Context, outcome evaloutcome.Outcome) (Generation, error)
}
