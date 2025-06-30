package port

import "context"

type QuestionnaireCreateRequest struct {
	Code    string `json:"code" valid:"required"`
	Title   string `json:"title" valid:"required"`
	ImgUrl  string `json:"img_url" valid:"required"`
	Version uint8  `json:"version" valid:"required"`
}

type QuestionnaireIDRequest struct {
	ID uint64 `json:"id" valid:"required"`
}

type QuestionnaireResponse struct {
	ID        uint64 `json:"id"`
	Code      string `json:"code"`
	Title     string `json:"title"`
	ImgUrl    string `json:"img_url"`
	Version   uint8  `json:"version"`
	Status    uint8  `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type QuestionnaireListResponse struct {
	Questionnaires []*QuestionnaireResponse `json:"questionnaires"`
	TotalCount     int64                    `json:"total_count"`
	Page           int                      `json:"page"`
	PageSize       int                      `json:"page_size"`
}

type QuestionnaireEditRequest struct {
	ID      uint64 `json:"id" valid:"required"`
	Title   string `json:"title" valid:"required"`
	ImgUrl  string `json:"img_url" valid:"required"`
	Version uint8  `json:"version" valid:"required"`
}

type QuestionnairePublishRequest struct {
	ID uint64 `json:"id" valid:"required"`
}

type QuestionnaireUnpublishRequest struct {
	ID uint64 `json:"id" valid:"required"`
}

type QuestionnaireCreator interface {
	CreateQuestionnaire(ctx context.Context, req QuestionnaireCreateRequest) (*QuestionnaireResponse, error)
}

type QuestionnaireQueryer interface {
	GetQuestionnaire(ctx context.Context, req QuestionnaireIDRequest) (*QuestionnaireResponse, error)
	GetQuestionnaireByCode(ctx context.Context, code string) (*QuestionnaireResponse, error)
	ListQuestionnaires(ctx context.Context, page, pageSize int) (*QuestionnaireListResponse, error)
}

type QuestionnaireEditor interface {
	EditBasicInfo(ctx context.Context, req QuestionnaireEditRequest) (*QuestionnaireResponse, error)
}

type QuestionnairePublisher interface {
	PublishQuestionnaire(ctx context.Context, req QuestionnairePublishRequest) (*QuestionnaireResponse, error)
	UnpublishQuestionnaire(ctx context.Context, req QuestionnaireUnpublishRequest) (*QuestionnaireResponse, error)
}
