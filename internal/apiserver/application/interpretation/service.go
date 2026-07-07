package interpretation

import (
	"context"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
)

// Service 生成并持久化解释报告s 从 计分结果。
type Service interface {
	GenerateAndPersist(ctx context.Context, outcome evaloutcome.Outcome) error
}

type service struct {
	writer interpretationreporting.Writer
}

// NewService 创建interpretation orchestrator 基于 report writer。
func NewService(writer interpretationreporting.Writer) Service {
	return &service{writer: writer}
}

func (s *service) GenerateAndPersist(ctx context.Context, outcome evaloutcome.Outcome) error {
	if s == nil || s.writer == nil {
		return interpretationreporting.ErrWriterNotConfigured()
	}
	return s.writer.Write(ctx, outcome)
}
