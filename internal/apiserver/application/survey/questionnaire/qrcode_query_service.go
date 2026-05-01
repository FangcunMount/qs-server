package questionnaire

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// QuestionnaireQRCodeGenerator is the narrow QR-code capability consumed by survey.
type QuestionnaireQRCodeGenerator interface {
	GenerateQuestionnaireQRCode(ctx context.Context, code, version string) (string, error)
}

// QuestionnaireQRCodeQueryService resolves questionnaire QR-code requests.
type QuestionnaireQRCodeQueryService interface {
	GetQRCode(ctx context.Context, code, version string) (string, error)
}

type questionnaireQRCodeQueryService struct {
	query     QuestionnaireQueryService
	generator QuestionnaireQRCodeGenerator
}

// NewQRCodeQueryService creates a questionnaire QR-code use case.
func NewQRCodeQueryService(query QuestionnaireQueryService, generator QuestionnaireQRCodeGenerator) QuestionnaireQRCodeQueryService {
	return &questionnaireQRCodeQueryService{query: query, generator: generator}
}

func (s *questionnaireQRCodeQueryService) GetQRCode(ctx context.Context, code, version string) (string, error) {
	if code == "" {
		return "", errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}
	if s == nil || s.generator == nil {
		return "", errors.WithCode(errorCode.ErrInternalServerError, "小程序码生成服务未配置")
	}
	if version == "" {
		if s.query == nil {
			return "", errors.WithCode(errorCode.ErrQuestionnaireNotFound, "问卷不存在或未发布")
		}
		result, err := s.query.GetPublishedByCode(ctx, code)
		if err != nil {
			return "", err
		}
		if result == nil {
			return "", errors.WithCode(errorCode.ErrQuestionnaireNotFound, "问卷不存在或未发布")
		}
		version = result.Version
	}
	return s.generator.GenerateQuestionnaireQRCode(context.Background(), code, version)
}
