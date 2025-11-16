package answersheet

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/dto"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/mapper"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/answersheet/port"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/user"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/user/role"
	errCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/errors"
	"github.com/FangcunMount/qs-server/pkg/log"
)

// Saver 答卷保存器
type Saver struct {
	aRepoMongo port.AnswerSheetRepositoryMongo
	mapper     mapper.AnswerMapper
}

// NewSaver 创建答卷保存器
func NewSaver(aRepoMongo port.AnswerSheetRepositoryMongo) *Saver {
	return &Saver{
		aRepoMongo: aRepoMongo,
		mapper:     mapper.NewAnswerMapper(),
	}
}

// SaveOriginalAnswerSheet 保存原始答卷
func (s *Saver) SaveOriginalAnswerSheet(ctx context.Context, answerSheetDTO dto.AnswerSheetDTO) (*dto.AnswerSheetDTO, error) {
	// 1. 参数校验
	if err := s.validateAnswerSheet(answerSheetDTO); err != nil {
		return nil, err
	}

	// 2. 转换为领域对象
	writer := role.NewWriter(user.NewUserID(answerSheetDTO.WriterID), "")
	testee := role.NewTestee(user.NewUserID(answerSheetDTO.TesteeID), "")
	answers := s.mapper.ToBOs(answerSheetDTO.Answers)

	asBO := answersheet.NewAnswerSheet(
		answerSheetDTO.QuestionnaireCode,
		answerSheetDTO.QuestionnaireVersion,
		answersheet.WithTitle(answerSheetDTO.Title),
		answersheet.WithWriter(writer),
		answersheet.WithTestee(testee),
		answersheet.WithAnswers(answers),
	)

	// 3. 保存到 MongoDB
	if err := s.aRepoMongo.Create(ctx, asBO); err != nil {
		return nil, errors.WrapC(err, errCode.ErrDatabase, "保存答卷失败")
	}

	// 4. 转换为 DTO 并返回
	return &dto.AnswerSheetDTO{
		ID:                   asBO.GetID(),
		QuestionnaireCode:    asBO.GetQuestionnaireCode(),
		QuestionnaireVersion: asBO.GetQuestionnaireVersion(),
		Title:                asBO.GetTitle(),
		Score:                asBO.GetScore(),
		WriterID:             asBO.GetWriter().GetUserID().Value(),
		TesteeID:             asBO.GetTestee().GetUserID().Value(),
		Answers:              s.mapper.ToDTOs(asBO.GetAnswers()),
	}, nil
}

// SaveAnswerSheetScores 保存答卷得分
func (s *Saver) SaveAnswerSheetScores(ctx context.Context, id uint64, totalScore float64, answers []dto.AnswerDTO) (*dto.AnswerSheetDTO, error) {
	log.Infof("开始保存答卷分数，答卷ID: %d, 总分: %d, 答案数量: %d", id, totalScore, len(answers))

	// 1. 获取现有答卷
	aDomain, err := s.aRepoMongo.FindByID(ctx, id)
	if err != nil {
		log.Errorf("查找答卷失败，ID: %d, 错误: %v", id, err)
		return nil, errors.WrapC(err, errCode.ErrAnswerSheetNotFound, "答卷不存在")
	}

	if aDomain == nil {
		log.Errorf("答卷不存在，ID: %d", id)
		return nil, errors.WithCode(errCode.ErrAnswerSheetNotFound, "答卷不存在")
	}

	log.Infof("找到现有答卷，ID: %d, 当前分数: %d", id, aDomain.GetScore())

	// 2. 转换答案
	answerBOs := s.mapper.ToBOs(answers)
	log.Infof("转换答案完成，答案数量: %d", len(answerBOs))

	// 3. 更新分数
	aDomain = answersheet.NewAnswerSheet(
		aDomain.GetQuestionnaireCode(),
		aDomain.GetQuestionnaireVersion(),
		answersheet.WithID(aDomain.GetID()),
		answersheet.WithTitle(aDomain.GetTitle()),
		answersheet.WithScore(totalScore),
		answersheet.WithWriter(aDomain.GetWriter()),
		answersheet.WithTestee(aDomain.GetTestee()),
		answersheet.WithAnswers(answerBOs),
		answersheet.WithCreatedAt(aDomain.GetCreatedAt()),
	)

	log.Infof("创建新的答卷对象完成，新分数: %d", aDomain.GetScore())

	// 4. 保存到 MongoDB
	if err := s.aRepoMongo.Update(ctx, aDomain); err != nil {
		log.Errorf("更新MongoDB失败，ID: %d, 错误: %v", id, err)
		return nil, errors.WrapC(err, errCode.ErrDatabase, "更新答卷分数失败")
	}

	log.Infof("MongoDB更新成功，ID: %d", id)

	// 5. 转换为 DTO 并返回
	result := &dto.AnswerSheetDTO{
		ID:                   aDomain.GetID(),
		QuestionnaireCode:    aDomain.GetQuestionnaireCode(),
		QuestionnaireVersion: aDomain.GetQuestionnaireVersion(),
		Title:                aDomain.GetTitle(),
		Score:                aDomain.GetScore(),
		WriterID:             aDomain.GetWriter().GetUserID().Value(),
		TesteeID:             aDomain.GetTestee().GetUserID().Value(),
		Answers:              s.mapper.ToDTOs(aDomain.GetAnswers()),
	}

	log.Infof("保存答卷分数完成，ID: %d, 最终分数: %d", id, result.Score)
	return result, nil
}

// validateAnswerSheet 验证答卷数据
func (s *Saver) validateAnswerSheet(answerSheet dto.AnswerSheetDTO) error {
	if answerSheet.QuestionnaireCode == "" {
		return errors.WithCode(errCode.ErrValidation, "问卷代码不能为空")
	}
	if answerSheet.QuestionnaireVersion == "" {
		return errors.WithCode(errCode.ErrValidation, "问卷版本不能为空")
	}
	if answerSheet.Title == "" {
		return errors.WithCode(errCode.ErrValidation, "答卷标题不能为空")
	}
	if answerSheet.WriterID == 0 {
		return errors.WithCode(errCode.ErrValidation, "填写人ID不能为空")
	}
	if answerSheet.TesteeID == 0 {
		return errors.WithCode(errCode.ErrValidation, "被试者ID不能为空")
	}
	if len(answerSheet.Answers) == 0 {
		return errors.WithCode(errCode.ErrValidation, "答案不能为空")
	}
	return nil
}
