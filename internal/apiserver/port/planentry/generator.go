package planentry

import (
	"context"
	"time"

	planDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
)

// Generator produces task entry tokens and URLs for opened plan tasks.
type Generator interface {
	GenerateEntry(ctx context.Context, task *planDomain.AssessmentTask) (token string, url string, expireAt time.Time, err error)
}
