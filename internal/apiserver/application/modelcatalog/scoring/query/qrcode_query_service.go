package query

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/ports"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

type scaleQRCodeQueryService struct {
	generator ports.ScaleQRCodeGenerator
}

// NewQRCodeQueryService 创建scale 二维码 用例。
func NewQRCodeQueryService(generator ports.ScaleQRCodeGenerator) ports.ScaleQRCodeQueryService {
	return &scaleQRCodeQueryService{generator: generator}
}

func (s *scaleQRCodeQueryService) GetQRCode(ctx context.Context, code string) (string, error) {
	if code == "" {
		return "", errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if s == nil || s.generator == nil {
		return "", errors.WithCode(errorCode.ErrInternalServerError, "小程序码生成服务未配置")
	}
	return s.generator.GenerateScaleQRCode(ctx, code)
}
