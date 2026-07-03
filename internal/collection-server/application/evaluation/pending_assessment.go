package evaluation

import (
	"context"
	"errors"
	"time"

	answersheetapp "github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrAnswerSheetNotFound indicates the answer sheet does not exist for pending fallback.
var ErrAnswerSheetNotFound = errors.New("answer sheet not found")

// AnswerSheetLookup 查询答卷是否存在。
type AnswerSheetLookup interface {
	Get(ctx context.Context, id uint64) (*answersheetapp.AnswerSheetResponse, error)
}

// PendingAssessmentResolver 在测评尚未生成时，根据答卷存在性返回 pending 状态。
type PendingAssessmentResolver struct {
	answerSheets AnswerSheetLookup
}

func NewPendingAssessmentResolver(answerSheets AnswerSheetLookup) *PendingAssessmentResolver {
	return &PendingAssessmentResolver{answerSheets: answerSheets}
}

// PendingStatus 若答卷存在则返回 pending 状态；答卷不存在返回 ErrAnswerSheetNotFound。
func (r *PendingAssessmentResolver) PendingStatus(ctx context.Context, answerSheetID uint64) (*AssessmentStatusResponse, error) {
	exists, err := r.answerSheetExists(ctx, answerSheetID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrAnswerSheetNotFound
	}
	return &AssessmentStatusResponse{
		Status:    "pending",
		UpdatedAt: time.Now().Unix(),
	}, nil
}

func (r *PendingAssessmentResolver) answerSheetExists(ctx context.Context, answerSheetID uint64) (bool, error) {
	if r == nil || r.answerSheets == nil {
		return true, nil
	}
	result, err := r.answerSheets.Get(ctx, answerSheetID)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return false, nil
		}
		return false, err
	}
	return result != nil, nil
}
