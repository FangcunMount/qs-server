package writer

import (
	"context"

	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationcompat"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type Generation struct {
	Report *domainreport.InterpretReport
	Events []event.DomainEvent
}

type Generator interface {
	Generate(ctx context.Context, outcome evaloutcome.Outcome) (Generation, error)
}
