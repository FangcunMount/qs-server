package questionnairecatalog

import "context"

// Item contains the questionnaire facts required by cross-module consumers.
type Item struct {
	Code    string
	Version string
	Type    string
}

// Catalog exposes questionnaire facts without leaking questionnaire repositories.
type Catalog interface {
	FindQuestionnaire(ctx context.Context, code string) (*Item, error)
	FindQuestionnaireVersion(ctx context.Context, code, version string) (*Item, error)
	FindPublishedQuestionnaire(ctx context.Context, code string) (*Item, error)
}
