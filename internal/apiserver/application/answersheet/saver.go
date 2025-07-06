package answersheet

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/mapper"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet/port"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	errCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
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
	writer := user.NewWriter(user.NewUserID(answerSheetDTO.WriterID), "")
	testee := user.NewTestee(user.NewUserID(answerSheetDTO.TesteeID), "")
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
func (s *Saver) SaveAnswerSheetScores(ctx context.Context, id uint64, totalScore uint16, answers []dto.AnswerDTO) (*dto.AnswerSheetDTO, error) {
	// 1. 获取现有答卷
	aDomain, err := s.aRepoMongo.FindByID(ctx, id)
	if err != nil {
		return nil, errors.WrapC(err, errCode.ErrAnswerSheetNotFound, "答卷不存在")
	}

	// 2. 更新分数
	aDomain = answersheet.NewAnswerSheet(
		aDomain.GetQuestionnaireCode(),
		aDomain.GetQuestionnaireVersion(),
		answersheet.WithID(aDomain.GetID()),
		answersheet.WithTitle(aDomain.GetTitle()),
		answersheet.WithScore(totalScore),
		answersheet.WithWriter(aDomain.GetWriter()),
		answersheet.WithTestee(aDomain.GetTestee()),
		answersheet.WithAnswers(s.mapper.ToBOs(answers)),
		answersheet.WithCreatedAt(aDomain.GetCreatedAt()),
	)

	// 3. 保存到 MongoDB
	if err := s.aRepoMongo.Update(ctx, aDomain); err != nil {
		return nil, errors.WrapC(err, errCode.ErrDatabase, "更新答卷分数失败")
	}

	// 4. 转换为 DTO 并返回
	return &dto.AnswerSheetDTO{
		ID:                   aDomain.GetID(),
		QuestionnaireCode:    aDomain.GetQuestionnaireCode(),
		QuestionnaireVersion: aDomain.GetQuestionnaireVersion(),
		Title:                aDomain.GetTitle(),
		Score:                aDomain.GetScore(),
		WriterID:             aDomain.GetWriter().GetUserID().Value(),
		TesteeID:             aDomain.GetTestee().GetUserID().Value(),
		Answers:              s.mapper.ToDTOs(aDomain.GetAnswers()),
	}, nil
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
