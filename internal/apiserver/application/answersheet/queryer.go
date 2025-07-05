package answersheet

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet/port"
	qnPort "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
	errCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// Queryer 答卷查询器
type Queryer struct {
	aRepoMongo port.AnswerSheetRepositoryMongo
	qRepoMongo qnPort.QuestionnaireRepositoryMongo
	mapper     *AnswerMapper
}

// NewQueryer 创建答卷查询器
func NewQueryer(
	aRepoMongo port.AnswerSheetRepositoryMongo,
	qRepoMongo qnPort.QuestionnaireRepositoryMongo,
) *Queryer {
	return &Queryer{
		aRepoMongo: aRepoMongo,
		qRepoMongo: qRepoMongo,
		mapper:     NewAnswerMapper(),
	}
}

// GetAnswerSheetByID 根据ID获取答卷详情
func (q *Queryer) GetAnswerSheetByID(ctx context.Context, id uint64) (*dto.AnswerSheetDetailDTO, error) {
	// 1. 获取答卷领域对象
	aDomain, err := q.aRepoMongo.FindByID(ctx, id)
	if err != nil {
		return nil, errors.WrapC(err, errCode.ErrAnswerSheetNotFound, "答卷不存在")
	}

	// 2. 获取问卷信息
	qDomain, err := q.qRepoMongo.FindByCode(ctx, aDomain.GetQuestionnaireCode())
	if err != nil {
		return nil, errors.WrapC(err, errCode.ErrQuestionnaireNotFound, "问卷不存在")
	}

	// 3. 转换为 DTO
	answerSheetDTO := &dto.AnswerSheetDTO{
		ID:                   aDomain.GetID(),
		QuestionnaireCode:    aDomain.GetQuestionnaireCode(),
		QuestionnaireVersion: aDomain.GetQuestionnaireVersion(),
		Title:                aDomain.GetTitle(),
		Score:                aDomain.GetScore(),
		WriterID:             aDomain.GetWriter().GetUserID().Value(),
		TesteeID:             aDomain.GetTestee().GetUserID().Value(),
		Answers:              q.mapper.ToDTOs(aDomain.GetAnswers()),
	}

	// 4. 构建详情 DTO
	return &dto.AnswerSheetDetailDTO{
		AnswerSheet: *answerSheetDTO,
		WriterName:  aDomain.GetWriter().GetName(),
		TesteeName:  aDomain.GetTestee().GetName(),
		Questionnaire: dto.QuestionnaireDTO{
			Code:        qDomain.GetCode().Value(),
			Version:     qDomain.GetVersion().Value(),
			Title:       qDomain.GetTitle(),
			Description: qDomain.GetDescription(),
		},
		CreatedAt: aDomain.GetCreatedAt().Format("2006-01-02 15:04:05"),
		UpdatedAt: aDomain.GetUpdatedAt().Format("2006-01-02 15:04:05"),
	}, nil
}

// GetAnswerSheetList 获取答卷列表
func (q *Queryer) GetAnswerSheetList(ctx context.Context, filter dto.AnswerSheetDTO, page, pageSize int) ([]dto.AnswerSheetDTO, int64, error) {
	// 1. 构建查询条件
	conditions := make(map[string]interface{})
	if filter.QuestionnaireCode != "" {
		conditions["questionnaire_code"] = filter.QuestionnaireCode
	}
	if filter.QuestionnaireVersion != "" {
		conditions["questionnaire_version"] = filter.QuestionnaireVersion
	}
	if filter.WriterID != 0 {
		conditions["writer.id"] = filter.WriterID
	}
	if filter.TesteeID != 0 {
		conditions["testee.id"] = filter.TesteeID
	}

	// 2. 获取总数
	total, err := q.aRepoMongo.CountWithConditions(ctx, conditions)
	if err != nil {
		return nil, 0, errors.WrapC(err, errCode.ErrDatabase, "统计答卷数量失败")
	}

	// 3. 如果没有数据，直接返回
	if total == 0 {
		return []dto.AnswerSheetDTO{}, 0, nil
	}

	// 4. 获取答卷列表
	var answerSheets []dto.AnswerSheetDTO

	// 4.1 根据不同条件使用不同的查询方法
	if filter.WriterID != 0 {
		domains, err := q.aRepoMongo.FindListByWriter(ctx, filter.WriterID, page, pageSize)
		if err != nil {
			return nil, 0, errors.WrapC(err, errCode.ErrDatabase, "查询答卷列表失败")
		}
		answerSheets = q.convertDomainsToAnswerSheetDTOs(domains)
	} else if filter.TesteeID != 0 {
		domains, err := q.aRepoMongo.FindListByTestee(ctx, filter.TesteeID, page, pageSize)
		if err != nil {
			return nil, 0, errors.WrapC(err, errCode.ErrDatabase, "查询答卷列表失败")
		}
		answerSheets = q.convertDomainsToAnswerSheetDTOs(domains)
	} else {
		// TODO: 实现通用的条件查询
		return []dto.AnswerSheetDTO{}, total, nil
	}

	return answerSheets, total, nil
}

// convertDomainsToAnswerSheetDTOs 将领域对象列表转换为 DTO 列表
func (q *Queryer) convertDomainsToAnswerSheetDTOs(domains []*answersheet.AnswerSheet) []dto.AnswerSheetDTO {
	dtos := make([]dto.AnswerSheetDTO, len(domains))
	for i, domain := range domains {
		dtos[i] = dto.AnswerSheetDTO{
			ID:                   domain.GetID(),
			QuestionnaireCode:    domain.GetQuestionnaireCode(),
			QuestionnaireVersion: domain.GetQuestionnaireVersion(),
			Title:                domain.GetTitle(),
			Score:                domain.GetScore(),
			WriterID:             domain.GetWriter().GetUserID().Value(),
			TesteeID:             domain.GetTestee().GetUserID().Value(),
			Answers:              q.mapper.ToDTOs(domain.GetAnswers()),
		}
	}
	return dtos
}
