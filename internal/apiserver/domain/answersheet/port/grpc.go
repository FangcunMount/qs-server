package port

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
)

// AnswerSheetGRPCService 答卷 GRPC 服务接口
type AnswerSheetGRPCService interface {
	// SaveAnswerSheet 保存答卷
	SaveAnswerSheet(ctx context.Context, req *dto.AnswerSheetDTO) (*dto.AnswerSheetDTO, error)
	// GetAnswerSheet 获取答卷
	GetAnswerSheet(ctx context.Context, id uint64) (*dto.AnswerSheetDetailDTO, error)
	// ListAnswerSheets 获取答卷列表
	ListAnswerSheets(ctx context.Context, filter *dto.AnswerSheetDTO, page, pageSize int) ([]*dto.AnswerSheetDTO, int64, error)
}
