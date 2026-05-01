package testee

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

// taggingService 受试者标签服务实现
type taggingService struct {
	repo          domain.Repository
	riskTagPolicy domain.RiskTagPolicy
	uow           apptransaction.Runner
}

// NewTaggingService 创建受试者标签服务
func NewTaggingService(
	repo domain.Repository,
	riskTagPolicy domain.RiskTagPolicy,
	uow apptransaction.Runner,
) TesteeTaggingService {
	if riskTagPolicy == nil {
		riskTagPolicy = domain.NewRiskTagPolicy()
	}
	return &taggingService{
		repo:          repo,
		riskTagPolicy: riskTagPolicy,
		uow:           uow,
	}
}

// TagByAssessmentResult 根据测评结果给受试者打标签
func (s *taggingService) TagByAssessmentResult(
	ctx context.Context,
	testeeID uint64,
	riskLevel string,
	scaleCode string,
	highRiskFactors []string,
	markKeyFocus bool,
) (*TaggingResult, error) {
	l := logger.L(ctx)

	l.Infow("根据测评结果给受试者打标签",
		"action", "tag_by_assessment_result",
		"testee_id", testeeID,
		"risk_level", riskLevel,
		"scale_code", scaleCode,
		"mark_key_focus", markKeyFocus,
	)

	if len(highRiskFactors) > 0 {
		// 因子标签已弃用，保留日志便于排查调用方是否仍在传递
		l.Debugw("高风险因子标签已弃用，输入参数已忽略",
			"testee_id", testeeID,
			"high_risk_factors_count", len(highRiskFactors),
		)
	}

	result := &TaggingResult{
		TagsAdded:   make([]string, 0),
		TagsRemoved: make([]string, 0),
	}

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		targetTesteeID, err := testeeIDFromUint64("testee_id", testeeID)
		if err != nil {
			return err
		}

		testee, err := s.repo.FindByID(txCtx, targetTesteeID)
		if err != nil {
			return errors.Wrap(err, "failed to get testee")
		}

		decision, err := s.riskTagPolicy.ApplyAssessmentResult(testee, riskLevel, markKeyFocus)
		if err != nil {
			return err
		}
		result.TagsAdded = tagsToStrings(decision.TagsAdded)
		result.TagsRemoved = tagsToStrings(decision.TagsRemoved)
		result.KeyFocusMarked = decision.KeyFocusMarked

		if err := s.repo.Update(txCtx, testee); err != nil {
			return errors.Wrap(err, "failed to update testee")
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	l.Infow("标签更新完成",
		"action", "tag_by_assessment_result",
		"testee_id", testeeID,
		"tags_added", result.TagsAdded,
		"tags_removed", result.TagsRemoved,
		"key_focus_marked", result.KeyFocusMarked,
	)

	return result, nil
}

func tagsToStrings(tags []domain.Tag) []string {
	values := make([]string, len(tags))
	for i, tag := range tags {
		values[i] = tag.String()
	}
	return values
}
