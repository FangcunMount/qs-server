package testee

import (
	"context"
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

// assessmentAttentionService 同步测评结果后置关注状态，不维护风险标签。
type assessmentAttentionService struct {
	repo   domain.Repository
	editor domain.Editor
	uow    apptransaction.Runner
}

// NewAssessmentAttentionService 创建测评后置关注同步服务。
func NewAssessmentAttentionService(
	repo domain.Repository,
	editor domain.Editor,
	uow apptransaction.Runner,
) TesteeAssessmentAttentionService {
	return &assessmentAttentionService{
		repo:   repo,
		editor: editor,
		uow:    uow,
	}
}

func (s *assessmentAttentionService) SyncAssessmentAttention(
	ctx context.Context,
	testeeID uint64,
	riskLevel string,
	markKeyFocus bool,
) (*AssessmentAttentionResult, error) {
	l := logger.L(ctx)
	l.Infow("同步测评后置关注状态",
		"action", "sync_assessment_attention",
		"testee_id", testeeID,
		"risk_level", riskLevel,
		"mark_key_focus", markKeyFocus,
	)

	targetTesteeID, err := testeeIDFromUint64("testee_id", testeeID)
	if err != nil {
		return nil, err
	}

	result := &AssessmentAttentionResult{}
	if !shouldAutoMarkKeyFocus(riskLevel, markKeyFocus) {
		l.Debugw("测评结果无需自动同步重点关注",
			"action", "sync_assessment_attention",
			"testee_id", testeeID,
			"risk_level", riskLevel,
		)
		return result, nil
	}

	err = s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		testee, err := s.repo.FindByID(txCtx, targetTesteeID)
		if err != nil {
			return errors.Wrap(err, "failed to get testee")
		}

		wasKeyFocus := testee.IsKeyFocus()
		if err := s.editor.MarkAsKeyFocus(txCtx, testee); err != nil {
			return err
		}
		result.KeyFocusMarked = testee.IsKeyFocus()

		if !wasKeyFocus {
			if err := s.repo.Update(txCtx, testee); err != nil {
				return errors.Wrap(err, "failed to update testee")
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	l.Infow("测评后置关注状态同步完成",
		"action", "sync_assessment_attention",
		"testee_id", testeeID,
		"risk_level", riskLevel,
		"key_focus_marked", result.KeyFocusMarked,
	)

	return result, nil
}

func shouldAutoMarkKeyFocus(riskLevel string, markKeyFocus bool) bool {
	if !markKeyFocus {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(riskLevel)) {
	case "high", "severe":
		return true
	default:
		return false
	}
}
