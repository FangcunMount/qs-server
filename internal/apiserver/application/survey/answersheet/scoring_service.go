package answersheet

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// scoringService 答卷评分服务实现
// 行为者：评分系统/Assessment Domain
type scoringService struct {
	repo answersheet.Repository
}

// NewScoringService 创建答卷评分服务
func NewScoringService(
	repo answersheet.Repository,
) AnswerSheetScoringService {
	return &scoringService{
		repo: repo,
	}
}

// UpdateScore 更新答卷分数
// 场景：Assessment 域计算出各题分数后，调用此方法更新答卷
// 实现：1. 更新每个答案的分数  2. 自动计算总分  3. 保存答卷
func (s *scoringService) UpdateScore(ctx context.Context, dto UpdateScoreDTO) (*AnswerSheetResult, error) {
	// 1. 验证输入参数
	if dto.AnswerSheetID == 0 {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "答卷ID不能为空")
	}

	// 2. 获取答卷
	sheet, err := s.repo.FindByID(ctx, meta.ID(dto.AnswerSheetID))
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAnswerSheetNotFound, "获取答卷失败")
	}

	// 3. 更新每个答案的分数
	updatedAnswers := make([]answersheet.Answer, 0, len(sheet.Answers()))
	for _, answer := range sheet.Answers() {
		// 查找对应问题的分数
		score := float64(0)
		for _, answerScore := range dto.AnswerScores {
			if answerScore.QuestionCode == answer.QuestionCode() {
				score = answerScore.Score
				break
			}
		}
		// 使用 WithScore 创建新的答案（不可变性）
		updatedAnswers = append(updatedAnswers, answer.WithScore(score))
	}

	// 4. 创建新的答卷（包含更新后的答案）
	qCode, qVersion, qTitle := sheet.QuestionnaireInfo()
	scoredSheet, err := answersheet.NewAnswerSheet(
		answersheet.NewQuestionnaireRef(qCode, qVersion, qTitle),
		sheet.Filler(),
		updatedAnswers,
		sheet.FilledAt(),
	)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAnswerSheetInvalid, "创建答卷失败")
	}

	// 设置ID（保持原有ID）
	scoredSheet.AssignID(sheet.ID())

	// 5. 自动计算总分（调用领域方法）
	scoredSheet = scoredSheet.CalculateScore()

	// 6. 更新答卷
	if err := s.repo.Update(ctx, scoredSheet); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "更新答卷分数失败")
	}

	return toAnswerSheetResult(scoredSheet), nil
}
