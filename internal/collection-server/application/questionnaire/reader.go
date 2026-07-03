package questionnaire

import "context"

// CatalogReader 问卷目录读端口（application-owned DTO）。
type CatalogReader interface {
	GetQuestionnaire(ctx context.Context, code, version string) (*QuestionnaireResponse, error)
	ListQuestionnaires(ctx context.Context, page, pageSize int32, status, title string) (*ListQuestionnairesResponse, error)
}
