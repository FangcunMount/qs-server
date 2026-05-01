package scale

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// ScaleQRCodeGenerator is the narrow QR-code capability consumed by scale.
type ScaleQRCodeGenerator interface {
	GenerateScaleQRCode(ctx context.Context, code string) (string, error)
}

// ScaleQRCodeQueryService resolves scale QR-code requests.
type ScaleQRCodeQueryService interface {
	GetQRCode(ctx context.Context, code string) (string, error)
}

type scaleQRCodeQueryService struct {
	generator ScaleQRCodeGenerator
}

// NewQRCodeQueryService creates a scale QR-code use case.
func NewQRCodeQueryService(generator ScaleQRCodeGenerator) ScaleQRCodeQueryService {
	return &scaleQRCodeQueryService{generator: generator}
}

func (s *scaleQRCodeQueryService) GetQRCode(_ context.Context, code string) (string, error) {
	if code == "" {
		return "", errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if s == nil || s.generator == nil {
		return "", errors.WithCode(errorCode.ErrInternalServerError, "小程序码生成服务未配置")
	}
	return s.generator.GenerateScaleQRCode(context.Background(), code)
}
