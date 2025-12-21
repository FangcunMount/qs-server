package testee

import (
	"context"
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
)

// taggingService 受试者标签服务实现
type taggingService struct {
	repo              domain.Repository
	managementService TesteeManagementService
	queryService      TesteeQueryService
	uow               *mysql.UnitOfWork
}

// NewTaggingService 创建受试者标签服务
func NewTaggingService(
	repo domain.Repository,
	managementService TesteeManagementService,
	queryService TesteeQueryService,
	uow *mysql.UnitOfWork,
) TesteeTaggingService {
	return &taggingService{
		repo:              repo,
		managementService: managementService,
		queryService:      queryService,
		uow:               uow,
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
		"high_risk_factors_count", len(highRiskFactors),
		"mark_key_focus", markKeyFocus,
	)

	result := &TaggingResult{
		TagsAdded:   make([]string, 0),
		TagsRemoved: make([]string, 0),
	}

	// 查询当前 testee 的标签，用于智能更新
	currentTestee, err := s.queryService.GetByID(ctx, testeeID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get testee")
	}
	currentTags := currentTestee.Tags

	// 在事务中执行标签更新
	err = s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		// 1. 移除旧的状态标签（风险等级标签）
		removedRiskTags, err := s.removeOldRiskTags(txCtx, testeeID, currentTags)
		if err != nil {
			return errors.Wrap(err, "failed to remove old risk tags")
		}
		result.TagsRemoved = append(result.TagsRemoved, removedRiskTags...)

		// 2. 移除旧的因子风险标签
		removedFactorTags, err := s.removeOldFactorRiskTags(txCtx, testeeID, currentTags)
		if err != nil {
			return errors.Wrap(err, "failed to remove old factor risk tags")
		}
		result.TagsRemoved = append(result.TagsRemoved, removedFactorTags...)

		// 3. 根据风险等级添加新标签
		addedRiskTags, err := s.addRiskLevelTags(txCtx, testeeID, riskLevel)
		if err != nil {
			return errors.Wrap(err, "failed to add risk level tags")
		}
		result.TagsAdded = append(result.TagsAdded, addedRiskTags...)

		// 4. 根据量表类型添加标签（历史标签，保留不删除）
		if scaleCode != "" {
			addedScaleTag, err := s.addScaleTag(txCtx, testeeID, scaleCode, currentTags)
			if err != nil {
				return errors.Wrap(err, "failed to add scale tag")
			}
			if addedScaleTag != "" {
				result.TagsAdded = append(result.TagsAdded, addedScaleTag)
			}
		}

		// 5. 根据高风险因子添加标签
		addedFactorTags, err := s.addFactorRiskTags(txCtx, testeeID, highRiskFactors)
		if err != nil {
			return errors.Wrap(err, "failed to add factor risk tags")
		}
		result.TagsAdded = append(result.TagsAdded, addedFactorTags...)

		// 6. 更新重点关注状态
		keyFocusMarked, err := s.updateKeyFocusStatus(txCtx, testeeID, riskLevel, markKeyFocus, currentTestee.IsKeyFocus)
		if err != nil {
			return errors.Wrap(err, "failed to update key focus status")
		}
		result.KeyFocusMarked = keyFocusMarked

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

// removeOldRiskTags 移除旧的风险等级标签
func (s *taggingService) removeOldRiskTags(ctx context.Context, testeeID uint64, currentTags []string) ([]string, error) {
	oldRiskTags := []string{"risk_high", "risk_severe", "risk_medium"}
	var removed []string

	for _, oldTag := range oldRiskTags {
		hasTag := false
		for _, currentTag := range currentTags {
			if currentTag == oldTag {
				hasTag = true
				break
			}
		}
		if hasTag {
			if err := s.managementService.RemoveTag(ctx, testeeID, oldTag); err != nil {
				logger.L(ctx).Warnw("移除旧风险标签失败",
					"testee_id", testeeID,
					"tag", oldTag,
					"error", err.Error(),
				)
				// 继续处理其他标签，不中断流程
			} else {
				removed = append(removed, oldTag)
			}
		}
	}

	return removed, nil
}

// removeOldFactorRiskTags 移除旧的因子风险标签
func (s *taggingService) removeOldFactorRiskTags(ctx context.Context, testeeID uint64, currentTags []string) ([]string, error) {
	var removed []string

	for _, currentTag := range currentTags {
		if strings.HasPrefix(currentTag, "factor_") && strings.HasSuffix(currentTag, "_high") {
			if err := s.managementService.RemoveTag(ctx, testeeID, currentTag); err != nil {
				logger.L(ctx).Warnw("移除旧因子风险标签失败",
					"testee_id", testeeID,
					"tag", currentTag,
					"error", err.Error(),
				)
				// 继续处理其他标签，不中断流程
			} else {
				removed = append(removed, currentTag)
			}
		}
	}

	return removed, nil
}

// addRiskLevelTags 根据风险等级添加标签
func (s *taggingService) addRiskLevelTags(ctx context.Context, testeeID uint64, riskLevel string) ([]string, error) {
	var added []string
	riskLevel = strings.ToLower(riskLevel)

	switch riskLevel {
	case "high", "severe":
		// 高风险标签
		tag := "risk_high"
		if err := s.managementService.AddTag(ctx, testeeID, tag); err != nil {
			logger.L(ctx).Warnw("添加风险标签失败",
				"testee_id", testeeID,
				"tag", tag,
				"error", err.Error(),
			)
		} else {
			added = append(added, tag)
		}

		// 严重风险额外标签
		if riskLevel == "severe" {
			tag := "risk_severe"
			if err := s.managementService.AddTag(ctx, testeeID, tag); err != nil {
				logger.L(ctx).Warnw("添加严重风险标签失败",
					"testee_id", testeeID,
					"tag", tag,
					"error", err.Error(),
				)
			} else {
				added = append(added, tag)
			}
		}

	case "medium":
		// 中等风险标签
		tag := "risk_medium"
		if err := s.managementService.AddTag(ctx, testeeID, tag); err != nil {
			logger.L(ctx).Warnw("添加中等风险标签失败",
				"testee_id", testeeID,
				"tag", tag,
				"error", err.Error(),
			)
		} else {
			added = append(added, tag)
		}
	}

	return added, nil
}

// addScaleTag 添加量表类型标签（历史标签，保留不删除）
func (s *taggingService) addScaleTag(ctx context.Context, testeeID uint64, scaleCode string, currentTags []string) (string, error) {
	tag := "scale_" + strings.ToLower(scaleCode)

	// 检查是否已有该标签（历史标签，不重复添加）
	for _, currentTag := range currentTags {
		if currentTag == tag {
			return "", nil // 已存在，不重复添加
		}
	}

	if err := s.managementService.AddTag(ctx, testeeID, tag); err != nil {
		logger.L(ctx).Warnw("添加量表标签失败",
			"testee_id", testeeID,
			"tag", tag,
			"error", err.Error(),
		)
		return "", err
	}

	return tag, nil
}

// addFactorRiskTags 添加因子风险标签
func (s *taggingService) addFactorRiskTags(ctx context.Context, testeeID uint64, highRiskFactors []string) ([]string, error) {
	var added []string

	for _, factorCode := range highRiskFactors {
		if factorCode != "" {
			// 标签格式：factor_{factor_code}_high
			tag := "factor_" + strings.ToLower(factorCode) + "_high"
			if err := s.managementService.AddTag(ctx, testeeID, tag); err != nil {
				logger.L(ctx).Warnw("添加因子风险标签失败",
					"testee_id", testeeID,
					"factor_code", factorCode,
					"tag", tag,
					"error", err.Error(),
				)
				// 继续处理其他因子，不中断流程
			} else {
				added = append(added, tag)
			}
		}
	}

	return added, nil
}

// updateKeyFocusStatus 更新重点关注状态
func (s *taggingService) updateKeyFocusStatus(
	ctx context.Context,
	testeeID uint64,
	riskLevel string,
	markKeyFocus bool,
	currentIsKeyFocus bool,
) (bool, error) {
	riskLevel = strings.ToLower(riskLevel)
	isHighRisk := riskLevel == "high" || riskLevel == "severe"

	// 高风险时标记为重点关注
	if isHighRisk && markKeyFocus {
		if err := s.managementService.MarkAsKeyFocus(ctx, testeeID); err != nil {
			logger.L(ctx).Warnw("标记重点关注失败",
				"testee_id", testeeID,
				"error", err.Error(),
			)
			return false, err
		}
		return true, nil
	}

	// 如果风险等级不是高风险，且之前是重点关注，则取消重点关注
	if !isHighRisk && currentIsKeyFocus && !markKeyFocus {
		if err := s.managementService.UnmarkKeyFocus(ctx, testeeID); err != nil {
			logger.L(ctx).Warnw("取消重点关注失败",
				"testee_id", testeeID,
				"error", err.Error(),
			)
			return false, err
		}
		return false, nil
	}

	// 状态未变化
	return currentIsKeyFocus, nil
}
