package answersheet

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
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
	l := logger.L(ctx)
	startTime := time.Now()

	l.Infow("开始提交答卷",
		"action", "submit",
		"resource", "answersheet",
		"questionnaire_code", dto.QuestionnaireCode,
		"questionnaire_ver", dto.QuestionnaireVer,
		"filler_id", dto.FillerID,
		"answer_count", len(dto.Answers),
	)

	// 1. 验证输入参数
	if dto.QuestionnaireCode == "" {
		l.Warnw("答卷提交失败：问卷编码为空",
			"action", "submit",
			"resource", "answersheet",
			"result", "failed",
			"reason", "empty_questionnaire_code",
		)
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "问卷编码不能为空")
	}
	if dto.QuestionnaireVer <= 0 {
		l.Warnw("答卷提交失败：问卷版本无效",
			"action", "submit",
			"resource", "answersheet",
			"result", "failed",
			"questionnaire_ver", dto.QuestionnaireVer,
		)
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "问卷版本不能为空")
	}
	if dto.FillerID == 0 {
		l.Warnw("答卷提交失败：填写人ID为空",
			"action", "submit",
			"resource", "answersheet",
			"result", "failed",
		)
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "填写人ID不能为空")
	}
	if len(dto.Answers) == 0 {
		l.Warnw("答卷提交失败：答案列表为空",
			"action", "submit",
			"resource", "answersheet",
			"result", "failed",
			"questionnaire_code", dto.QuestionnaireCode,
		)
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "答案列表不能为空")
	}

	l.Debugw("输入参数验证通过",
		"questionnaire_code", dto.QuestionnaireCode,
		"filler_id", dto.FillerID,
	)

	// 2. 构建填写人引用
	fillerRef := actor.NewFillerRef(int64(dto.FillerID), actor.FillerTypeSelf)

	// 3. 获取问卷信息（用于验证）
	l.Debugw("开始获取问卷信息",
		"questionnaire_code", dto.QuestionnaireCode,
		"action", "read",
		"resource", "questionnaire",
	)

	qnr, err := s.questionnaireRepo.FindByCode(ctx, dto.QuestionnaireCode)
	if err != nil {
		l.Errorw("获取问卷信息失败",
			"questionnaire_code", dto.QuestionnaireCode,
			"action", "read",
			"resource", "questionnaire",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "问卷不存在")
	}

	l.Debugw("问卷信息获取成功",
		"questionnaire_code", dto.QuestionnaireCode,
		"questionnaire_title", qnr.GetTitle(),
		"question_count", len(qnr.GetQuestions()),
		"result", "success",
	)

	// 验证问卷版本是否匹配
	qnrVer, _ := strconv.Atoi(qnr.GetVersion().Value())
	if qnrVer != dto.QuestionnaireVer {
		l.Warnw("问卷版本不匹配",
			"questionnaire_code", dto.QuestionnaireCode,
			"expected_version", dto.QuestionnaireVer,
			"actual_version", qnrVer,
			"result", "failed",
		)
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid,
			"%s", fmt.Sprintf("问卷版本不匹配，期望: %d, 实际: %d", dto.QuestionnaireVer, qnrVer))
	}

	// 验证问卷是否已发布（只能对已发布的问卷提交答卷）
	if !qnr.IsPublished() {
		l.Warnw("问卷未发布，无法提交答卷",
			"questionnaire_code", dto.QuestionnaireCode,
			"status", qnr.GetStatus().String(),
			"result", "failed",
		)
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "只能对已发布的问卷提交答卷")
	}

	// 构建问题编码到问题对象的映射（用于后续验证）
	questionMap := make(map[string]questionnaire.Question)
	for _, q := range qnr.GetQuestions() {
		questionMap[q.GetCode().Value()] = q
	}

	l.Debugw("问卷验证通过",
		"questionnaire_code", dto.QuestionnaireCode,
		"version", qnrVer,
		"question_count", len(questionMap),
	)

	// 4. 构建问卷引用
	questionnaireRef := answersheet.NewQuestionnaireRef(
		dto.QuestionnaireCode,
		strconv.Itoa(dto.QuestionnaireVer),
		qnr.GetTitle(), // 使用查询到的问卷标题
	)

	// 5. 转换答案列表并验证
	l.Infow("开始验证答案",
		"answer_count", len(dto.Answers),
		"action", "validate",
		"resource", "answer",
	)

	answers := make([]answersheet.Answer, 0, len(dto.Answers))
	validatedCount := 0
	for i, answerDTO := range dto.Answers {
		// 5.1 检查问题是否存在于问卷中
		question, exists := questionMap[answerDTO.QuestionCode]
		if !exists {
			l.Warnw("问题不存在于问卷中",
				"question_code", answerDTO.QuestionCode,
				"answer_index", i,
				"result", "failed",
			)
			return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid,
				"%s", fmt.Sprintf("问题 %s 不存在于问卷中", answerDTO.QuestionCode))
		}

		// 5.2 使用工厂方法创建答案值对象
		answerValue, err := answersheet.CreateAnswerValueFromRaw(
			questionnaire.QuestionType(answerDTO.QuestionType),
			answerDTO.Value,
		)
		if err != nil {
			l.Warnw("创建答案值失败",
				"question_code", answerDTO.QuestionCode,
				"question_type", answerDTO.QuestionType,
				"error", err.Error(),
				"result", "failed",
			)
			return nil, errors.WrapC(err, errorCode.ErrAnswerSheetInvalid,
				"%s", fmt.Sprintf("创建答案值失败 [%s]", answerDTO.QuestionCode))
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
			l.Warnw("答案验证失败",
				"question_code", answerDTO.QuestionCode,
				"validation_errors", fmt.Sprintf("%v", errMessages),
				"result", "failed",
			)
			return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid,
				"%s", fmt.Sprintf("问题 %s 答案验证失败: %v", answerDTO.QuestionCode, errMessages))
		}

		// 5.4 创建答案对象
		answer, err := answersheet.NewAnswer(
			meta.NewCode(answerDTO.QuestionCode),
			questionnaire.QuestionType(answerDTO.QuestionType),
			answerValue,
			0, // 初始分数为0，后续由评分系统计算
		)
		if err != nil {
			l.Errorw("创建答案对象失败",
				"question_code", answerDTO.QuestionCode,
				"error", err.Error(),
				"result", "failed",
			)
			return nil, errors.WrapC(err, errorCode.ErrAnswerSheetInvalid,
				"%s", fmt.Sprintf("创建答案失败 [%s]", answerDTO.QuestionCode))
		}

		answers = append(answers, answer)
		validatedCount++
	}

	l.Infow("答案验证完成",
		"validated_count", validatedCount,
		"total_count", len(dto.Answers),
		"result", "success",
	)

	// 6. 创建答卷领域对象
	l.Debugw("开始创建答卷领域对象",
		"questionnaire_code", dto.QuestionnaireCode,
		"filler_id", dto.FillerID,
		"answer_count", len(answers),
	)

	sheet, err := answersheet.NewAnswerSheet(
		questionnaireRef,
		fillerRef,
		answers,
		time.Now(), // 填写时间
	)
	if err != nil {
		l.Errorw("创建答卷领域对象失败",
			"questionnaire_code", dto.QuestionnaireCode,
			"error", err.Error(),
			"result", "failed",
		)
		return nil, errors.WrapC(err, errorCode.ErrAnswerSheetInvalid, "创建答卷失败")
	}

	// 7. 持久化
	l.Infow("开始保存答卷",
		"action", "create",
		"resource", "answersheet",
		"questionnaire_code", dto.QuestionnaireCode,
	)

	if err := s.repo.Create(ctx, sheet); err != nil {
		l.Errorw("保存答卷失败",
			"action", "create",
			"resource", "answersheet",
			"questionnaire_code", dto.QuestionnaireCode,
			"error", err.Error(),
			"result", "failed",
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存答卷失败")
	}

	duration := time.Since(startTime)
	l.Infow("答卷提交成功",
		"action", "submit",
		"resource", "answersheet",
		"result", "success",
		"answersheet_id", sheet.ID().Uint64(),
		"questionnaire_code", dto.QuestionnaireCode,
		"filler_id", dto.FillerID,
		"answer_count", len(answers),
		"duration_ms", duration.Milliseconds(),
	)

	return toAnswerSheetResult(sheet), nil
}

// GetMyAnswerSheet 获取我的答卷
func (s *submissionService) GetMyAnswerSheet(ctx context.Context, fillerID uint64, answerSheetID uint64) (*AnswerSheetResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("查询我的答卷",
		"action", "get_my_answersheet",
		"filler_id", fillerID,
		"answersheet_id", answerSheetID,
	)

	// 1. 验证输入参数
	if fillerID == 0 {
		l.Warnw("填写人 ID 为空",
			"action", "get_my_answersheet",
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "填写人ID不能为空")
	}
	if answerSheetID == 0 {
		l.Warnw("答卷 ID 为空", "action", "get_my_answersheet", "result", "invalid_params")
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "答卷ID不能为空")
	}

	// 2. 获取答卷
	l.Debugw("加载答卷数据", "answersheet_id", answerSheetID)
	sheet, err := s.repo.FindByID(ctx, meta.ID(answerSheetID))
	if err != nil {
		l.Errorw("加载答卷失败",
			"action", "get_my_answersheet",
			"answersheet_id", answerSheetID,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrAnswerSheetNotFound, "获取答卷失败")
	}

	// 3. 验证是否是本人的答卷
	l.Debugw("验证答卷权限", "filler_id", fillerID, "answersheet_filler_id", sheet.Filler().UserID())
	fillerRef := actor.NewFillerRef(int64(fillerID), actor.FillerTypeSelf)
	if !sheet.IsFilledBy(fillerRef) {
		l.Warnw("无权查看答卷",
			"action", "get_my_answersheet",
			"filler_id", fillerID,
			"answersheet_filler_id", sheet.Filler().UserID(),
			"result", "permission_denied",
		)
		return nil, errors.WithCode(errorCode.ErrPermissionDenied, "无权查看此答卷")
	}

	duration := time.Since(startTime)
	l.Debugw("查询我的答卷成功",
		"action", "get_my_answersheet",
		"result", "success",
		"answersheet_id", answerSheetID,
		"duration_ms", duration.Milliseconds(),
	)
	return toAnswerSheetResult(sheet), nil
}

// ListMyAnswerSheets 查询我的答卷列表
func (s *submissionService) ListMyAnswerSheets(ctx context.Context, dto ListMyAnswerSheetsDTO) (*AnswerSheetSummaryListResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("查询我的答卷列表",
		"action", "list_my_answersheets",
		"filler_id", dto.FillerID,
		"page", dto.Page,
		"page_size", dto.PageSize,
	)

	// 1. 验证输入参数
	if dto.FillerID == 0 {
		l.Warnw("填写人 ID 为空",
			"action", "list_my_answersheets",
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "填写人ID不能为空")
	}
	if dto.Page <= 0 {
		l.Warnw("页码有效性检查失败",
			"action", "list_my_answersheets",
			"page", dto.Page,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "页码必须大于0")
	}
	if dto.PageSize <= 0 {
		l.Warnw("每页数量有效性检查失败",
			"action", "list_my_answersheets",
			"page_size", dto.PageSize,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "每页数量必须大于0")
	}
	if dto.PageSize > 100 {
		l.Warnw("每页数量超限",
			"action", "list_my_answersheets",
			"page_size", dto.PageSize,
			"max_size", 100,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "每页数量不能超过100")
	}

	// 2. 查询答卷摘要列表（使用 FillerID）
	l.Debugw("开始查询答卷列表",
		"filler_id", dto.FillerID,
		"page", dto.Page,
		"page_size", dto.PageSize,
	)
	sheets, err := s.repo.FindSummaryListByFiller(ctx, dto.FillerID, dto.Page, dto.PageSize)
	if err != nil {
		l.Errorw("查询答卷列表失败",
			"action", "list_my_answersheets",
			"filler_id", dto.FillerID,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询答卷列表失败")
	}

	// 3. 获取总数
	l.Debugw("查询答卷总数",
		"filler_id", dto.FillerID,
	)
	total, err := s.repo.CountWithConditions(ctx, map[string]interface{}{
		"filler_id": dto.FillerID,
	})
	if err != nil {
		l.Errorw("获取答卷总数失败",
			"action", "list_my_answersheets",
			"filler_id", dto.FillerID,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取答卷总数失败")
	}

	duration := time.Since(startTime)
	l.Debugw("查询我的答卷列表成功",
		"action", "list_my_answersheets",
		"result", "success",
		"filler_id", dto.FillerID,
		"total_count", total,
		"page_count", len(sheets),
		"duration_ms", duration.Milliseconds(),
	)

	return toSummaryListResult(sheets, total), nil
}
