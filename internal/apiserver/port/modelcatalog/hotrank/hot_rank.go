package hotrank

import (
	"context"
	"time"
)

// SubmissionFact is one questionnaire submission projected into the catalog
// hot-rank read model.
type SubmissionFact struct {
	EventID           string
	QuestionnaireCode string
	SubmittedAt       time.Time
}

// Query constrains a catalog hot-rank read.
type Query struct {
	WindowDays int
	Limit      int
}

// Entry is one questionnaire-backed catalog rank item.
type Entry struct {
	QuestionnaireCode string
	Score             int64
}

// Projection maintains the catalog hot-rank read model.
type Projection interface {
	ProjectSubmission(context.Context, SubmissionFact) error
}

// ReadModel reads catalog hot-rank entries.
type ReadModel interface {
	Top(context.Context, Query) ([]Entry, error)
}
