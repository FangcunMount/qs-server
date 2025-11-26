package answersheet

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/validation"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// submissionService 答卷提交服务实现
// 行为者：答题者
type submissionService struct {
	repo              answersheet.Repository
	questionnaireRepo questionnaire.Repository
	validator         validation.Validator
}

// NewSubmissionService 创建答卷提交服务
func NewSubmissionService(
	repo answersheet.Repository,
	questionnaireRepo questionnaire.Repository,
	validator validation.Validator,
) AnswerSheetSubmissionService {
	return &submissionService{
		repo:              repo,
		questionnaireRepo: questionnaireRepo,
		validator:         validator,
	}
}

// Submit 提交答卷
func (s *submissionService) Submit(ctx context.Context, dto SubmitAnswerSheetDTO) (*AnswerSheetResult, error) {
	// 1. 验证输入参数
	if dto.QuestionnaireCode == "" {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "问卷编码不能为空")
	}
	if dto.QuestionnaireVer <= 0 {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "问卷版本不能为空")
	}
	if dto.FillerID == 0 {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "填写人ID不能为空")
	}
	if len(dto.Answers) == 0 {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "答案列表不能为空")
	}

	// 2. 构建填写人引用
	fillerRef := actor.NewFillerRef(int64(dto.FillerID), actor.FillerTypeSelf)

	// 3. 获取问卷信息（用于验证）
	qnr, err := s.questionnaireRepo.FindByCode(ctx, dto.QuestionnaireCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "问卷不存在")
	}

	// 验证问卷版本是否匹配
	qnrVer, _ := strconv.Atoi(qnr.GetVersion().Value())
	if qnrVer != dto.QuestionnaireVer {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid,
			fmt.Sprintf("问卷版本不匹配，期望: %d, 实际: %d", dto.QuestionnaireVer, qnrVer))
	}

	// 验证问卷是否已发布（只能对已发布的问卷提交答卷）
	if !qnr.IsPublished() {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "只能对已发布的问卷提交答卷")
	}

	// 构建问题编码到问题对象的映射（用于后续验证）
	questionMap := make(map[string]questionnaire.Question)
	for _, q := range qnr.GetQuestions() {
		questionMap[q.GetCode().Value()] = q
	}

	// 4. 构建问卷引用
	questionnaireRef := answersheet.NewQuestionnaireRef(
		dto.QuestionnaireCode,
		strconv.Itoa(dto.QuestionnaireVer),
		qnr.GetTitle(), // 使用查询到的问卷标题
	)

	// 5. 转换答案列表并验证
	answers := make([]answersheet.Answer, 0, len(dto.Answers))
	for _, answerDTO := range dto.Answers {
		// 5.1 检查问题是否存在于问卷中
		question, exists := questionMap[answerDTO.QuestionCode]
		if !exists {
			return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid,
				fmt.Sprintf("问题 %s 不存在于问卷中", answerDTO.QuestionCode))
		}

		// 5.2 使用工厂方法创建答案值对象
		answerValue, err := answersheet.CreateAnswerValueFromRaw(
			questionnaire.QuestionType(answerDTO.QuestionType),
			answerDTO.Value,
		)
		if err != nil {
			return nil, errors.WrapC(err, errorCode.ErrAnswerSheetInvalid,
				fmt.Sprintf("创建答案值失败 [%s]", answerDTO.QuestionCode))
		}

		// 5.3 验证答案值是否符合问题的校验规则
		validatableValue := answersheet.NewAnswerValueAdapter(answerValue)
		validationResult := s.validator.ValidateValue(validatableValue, question.GetValidationRules())
		if !validationResult.IsValid() {
			// 收集所有错误信息
			errMessages := make([]string, 0, len(validationResult.GetErrors()))
			for _, validationErr := range validationResult.GetErrors() {
				errMessages = append(errMessages, validationErr.GetMessage())
			}
			return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid,
				fmt.Sprintf("问题 %s 答案验证失败: %v", answerDTO.QuestionCode, errMessages))
		}

		// 5.4 创建答案对象
		answer, err := answersheet.NewAnswer(
			meta.NewCode(answerDTO.QuestionCode),
			questionnaire.QuestionType(answerDTO.QuestionType),
			answerValue,
			0, // 初始分数为0，后续由评分系统计算
		)
		if err != nil {
			return nil, errors.WrapC(err, errorCode.ErrAnswerSheetInvalid,
				fmt.Sprintf("创建答案失败 [%s]", answerDTO.QuestionCode))
		}

		answers = append(answers, answer)
	}

	// 6. 创建答卷领域对象
	sheet, err := answersheet.NewAnswerSheet(
		questionnaireRef,
		fillerRef,
		answers,
		time.Now(), // 填写时间
	)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAnswerSheetInvalid, "创建答卷失败")
	}

	// 7. 持久化
	if err := s.repo.Create(ctx, sheet); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存答卷失败")
	}

	return toAnswerSheetResult(sheet), nil
}

// GetMyAnswerSheet 获取我的答卷
func (s *submissionService) GetMyAnswerSheet(ctx context.Context, fillerID uint64, answerSheetID uint64) (*AnswerSheetResult, error) {
	// 1. 验证输入参数
	if fillerID == 0 {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "填写人ID不能为空")
	}
	if answerSheetID == 0 {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "答卷ID不能为空")
	}

	// 2. 获取答卷
	sheet, err := s.repo.FindByID(ctx, meta.ID(answerSheetID))
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAnswerSheetNotFound, "获取答卷失败")
	}

	// 3. 验证是否是本人的答卷
	fillerRef := actor.NewFillerRef(int64(fillerID), actor.FillerTypeSelf)
	if !sheet.IsFilledBy(fillerRef) {
		return nil, errors.WithCode(errorCode.ErrPermissionDenied, "无权查看此答卷")
	}

	return toAnswerSheetResult(sheet), nil
}

// ListMyAnswerSheets 查询我的答卷列表
func (s *submissionService) ListMyAnswerSheets(ctx context.Context, dto ListMyAnswerSheetsDTO) (*AnswerSheetListResult, error) {
	// 1. 验证输入参数
	if dto.FillerID == 0 {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "填写人ID不能为空")
	}
	if dto.Page <= 0 {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "页码必须大于0")
	}
	if dto.PageSize <= 0 {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "每页数量必须大于0")
	}
	if dto.PageSize > 100 {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "每页数量不能超过100")
	}

	// 2. 查询答卷列表（使用 FillerID）
	sheets, err := s.repo.FindListByFiller(ctx, dto.FillerID, dto.Page, dto.PageSize)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询答卷列表失败")
	}

	// 3. 获取总数
	total, err := s.repo.CountWithConditions(ctx, map[string]interface{}{
		"filler_id": dto.FillerID,
	})
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取答卷总数失败")
	}

	return toAnswerSheetListResult(sheets, total), nil
}
