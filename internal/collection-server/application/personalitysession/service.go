package personalitysession

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
)

type personalityModelReader interface {
	Get(ctx context.Context, code string) (*typologymodel.TypologyModelResponse, error)
}

type questionnaireReader interface {
	Get(ctx context.Context, code, version string) (*questionnaire.QuestionnaireResponse, error)
}

// Service aggregates the stable mini-program entry for starting a personality assessment.
type Service struct {
	models        personalityModelReader
	questionnaire questionnaireReader
}

func NewService(models personalityModelReader, questionnaire questionnaireReader) *Service {
	return &Service{
		models:        models,
		questionnaire: questionnaire,
	}
}

func (s *Service) Start(ctx context.Context, req *StartSessionRequest) (*StartSessionResponse, error) {
	if req == nil || req.ModelCode == "" || req.TesteeID == 0 {
		return nil, nil
	}
	model, err := s.models.Get(ctx, req.ModelCode)
	if err != nil {
		return nil, err
	}
	if model == nil {
		return nil, nil
	}
	q, err := s.questionnaire.Get(ctx, model.QuestionnaireCode, model.QuestionnaireVersion)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, nil
	}
	summary := typologymodel.TypologyModelSummaryResponse{
		Code:                 model.Code,
		Version:              model.Version,
		Title:                model.Title,
		Algorithm:            model.Algorithm,
		Description:          model.Description,
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Status:               model.Status,
		QuestionCount:        model.QuestionCount,
	}
	return &StartSessionResponse{
		Model:          summary,
		Questionnaire:  *q,
		SubmitContract: buildSubmitContract(model, req.TesteeID),
		Endpoints:      buildEndpoints(req.TesteeID),
	}, nil
}
