package port

import (
	"context"

	"github.com/fangcun-mount/qs-server/internal/apiserver/application/dto"
)

// InterpretReportCreator
type InterpretReportCreator interface {
	// CreateInterpretReport 创建解读报告
	CreateInterpretReport(ctx context.Context, report *dto.InterpretReportDTO) (*dto.InterpretReportDTO, error)
}

// InterpretReportEditor 解读报告编辑器接口
type InterpretReportEditor interface {
	// UpdateInterpretReport 更新解读报告
	UpdateInterpretReport(ctx context.Context, report *dto.InterpretReportDTO) (*dto.InterpretReportDTO, error)
}

// InterpretReportQueryer 解读报告查询器接口
type InterpretReportQueryer interface {
	// GetInterpretReportByAnswerSheetId 根据答卷ID获取解读报告
	GetInterpretReportByAnswerSheetId(ctx context.Context, answerSheetId uint64) (*dto.InterpretReportDTO, error)
}
