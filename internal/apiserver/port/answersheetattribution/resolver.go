package answersheetattribution

import (
	"context"
	"time"

	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
)

type ResolveRequest struct {
	OriginRef            domainanswersheet.OriginRef
	OrgID                uint64
	TesteeID             uint64
	QuestionnaireCode    string
	QuestionnaireVersion string
	Admission            domainanswersheet.Admission
	CapturedAt           time.Time
}

type Resolver interface {
	Resolve(ctx context.Context, request ResolveRequest) (domainanswersheet.AttributionSnapshot, error)
}
