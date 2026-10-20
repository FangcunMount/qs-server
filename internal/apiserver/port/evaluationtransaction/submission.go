// Package evaluationtransaction owns the original-root evaluation write ports.
package evaluationtransaction

import (
	"context"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"time"
)

// Submitter freezes a copy and returns only a confirmed original submission.
// It does not mutate the caller's pending aggregate before root commit.
type Submitter interface {
	Submit(context.Context, *assessment.Assessment, time.Time) (*assessment.Assessment, error)
}
