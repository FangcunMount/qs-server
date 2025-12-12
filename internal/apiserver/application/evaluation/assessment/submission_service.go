package assessment

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// submissionService 测评提交服务实现
// 行为者：答题者 (Testee)
type submissionService struct {
	repo           assessment.Repository
	creator        assessment.AssessmentCreator
	eventPublisher event.EventPublisher
}

// NewSubmissionService 创建测评提交服务
func NewSubmissionService(
	repo assessment.Repository,
	creator assessment.AssessmentCreator,
	eventPublisher event.EventPublisher,
) AssessmentSubmissionService {
	return &submissionService{
		repo:           repo,
		creator:        creator,
		eventPublisher: eventPublisher,
	}
}

// Create 创建测评
func (s *submissionService) Create(ctx context.Context, dto CreateAssessmentDTO) (*AssessmentResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Infow("开始创建测评",
		"action", "create_assessment",
		"resource", "assessment",
		"testee_id", dto.TesteeID,
		"questionnaire_id", dto.QuestionnaireID,
		"answersheet_id", dto.AnswerSheetID,
	)

	// 1. 验证必要参数
	if dto.TesteeID == 0 {
		l.Warnw("受试者ID为空",
			"action", "create_assessment",
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "受试者ID不能为空")
	}
	if dto.QuestionnaireID == 0 {
		l.Warnw("问卷ID为空",
			"action", "create_assessment",
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "问卷ID不能为空")
	}
	if dto.AnswerSheetID == 0 {
		l.Warnw("答卷ID为空",
			"action", "create_assessment",
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "答卷ID不能为空")
	}

	// 2. 构造创建请求
	l.Debugw("构造创建请求",
		"testee_id", dto.TesteeID,
	)
	req := s.buildCreateRequest(dto)

	// 3. 调用领域服务创建测评
	l.Debugw("调用领域服务创建测评",
		"action", "create",
	)
	a, err := s.creator.Create(ctx, req)
	if err != nil {
		l.Errorw("创建测评失败",
			"testee_id", dto.TesteeID,
			"action", "create_assessment",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrAssessmentCreateFailed, "创建测评失败")
	}

	// 4. 持久化
	l.Debugw("持久化测评",
		"assessment_id", a.ID().Uint64(),
		"action", "save",
	)
	if err := s.repo.Save(ctx, a); err != nil {
		l.Errorw("保存测评失败",
			"assessment_id", a.ID().Uint64(),
			"action", "create_assessment",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存测评失败")
	}

	duration := time.Since(startTime)
	l.Infow("创建测评成功",
		"action", "create_assessment",
		"result", "success",
		"assessment_id", a.ID().Uint64(),
		"testee_id", dto.TesteeID,
		"duration_ms", duration.Milliseconds(),
	)

	return toAssessmentResult(a), nil
}

// Submit 提交测评
func (s *submissionService) Submit(ctx context.Context, assessmentID uint64) (*AssessmentResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Infow("开始提交测评",
		"action", "submit_assessment",
		"resource", "assessment",
		"assessment_id", assessmentID,
	)

	// 1. 查询测评
	l.Debugw("加载测评数据",
		"assessment_id", assessmentID,
		"action", "read",
	)
	id := meta.FromUint64(assessmentID)
	a, err := s.repo.FindByID(ctx, id)
	if err != nil {
		l.Errorw("加载测评失败",
			"assessment_id", assessmentID,
			"action", "submit_assessment",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrAssessmentNotFound, "测评不存在")
	}

	// 2. 提交测评
	l.Debugw("执行测评提交逻辑",
		"assessment_id", assessmentID,
		"current_status", a.Status().String(),
	)
	if err := a.Submit(); err != nil {
		l.Errorw("提交测评失败",
			"assessment_id", assessmentID,
			"action", "submit_assessment",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrAssessmentSubmitFailed, "提交测评失败")
	}

	// 3. 持久化
	l.Debugw("持久化提交结果",
		"assessment_id", assessmentID,
		"new_status", a.Status().String(),
	)
	if err := s.repo.Save(ctx, a); err != nil {
		l.Errorw("保存测评失败",
			"assessment_id", assessmentID,
			"action", "submit_assessment",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存测评失败")
	}

	// 4. 发布领域事件
	// 说明：领域事件已在 Submit() 内部添加到聚合根，这里统一发布
	s.publishEvents(ctx, a, l)

	duration := time.Since(startTime)
	l.Infow("提交测评成功",
		"action", "submit_assessment",
		"result", "success",
		"assessment_id", assessmentID,
		"duration_ms", duration.Milliseconds(),
	)

	return toAssessmentResult(a), nil
}

// GetMyAssessment 获取我的测评详情
func (s *submissionService) GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("获取我的测评详情",
		"action", "get_my_assessment",
		"testee_id", testeeID,
		"assessment_id", assessmentID,
	)

	// 1. 查询测评
	l.Debugw("从数据库查询测评",
		"assessment_id", assessmentID,
		"action", "read",
	)
	id := meta.FromUint64(assessmentID)
	a, err := s.repo.FindByID(ctx, id)
	if err != nil {
		l.Errorw("查询测评失败",
			"assessment_id", assessmentID,
			"action", "get_my_assessment",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrAssessmentNotFound, "测评不存在")
	}

	// 2. 验证归属
	l.Debugw("验证测评归属",
		"testee_id", testeeID,
		"assessment_testee_id", a.TesteeID().Uint64(),
	)
	if a.TesteeID().Uint64() != testeeID {
		l.Warnw("无权访问测评",
			"action", "get_my_assessment",
			"testee_id", testeeID,
			"assessment_testee_id", a.TesteeID().Uint64(),
			"result", "permission_denied",
		)
		return nil, errors.WithCode(errorCode.ErrForbidden, "无权访问此测评")
	}

	duration := time.Since(startTime)
	l.Debugw("获取我的测评成功",
		"assessment_id", assessmentID,
		"status", a.Status().String(),
		"duration_ms", duration.Milliseconds(),
	)

	return toAssessmentResult(a), nil
}

// ListMyAssessments 查询我的测评列表
func (s *submissionService) ListMyAssessments(ctx context.Context, dto ListMyAssessmentsDTO) (*AssessmentListResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("查询我的测评列表",
		"action", "list_my_assessments",
		"testee_id", dto.TesteeID,
		"page", dto.Page,
		"page_size", dto.PageSize,
	)

	// 1. 验证参数
	if dto.TesteeID == 0 {
		l.Warnw("受试者ID为空",
			"action", "list_my_assessments",
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "受试者ID不能为空")
	}

	// 2. 设置默认分页
	l.Debugw("处理分页参数",
		"page", dto.Page,
		"page_size", dto.PageSize,
	)
	page, pageSize := normalizePagination(dto.Page, dto.PageSize)

	// 3. 构造查询参数
	testeeID := testee.NewID(dto.TesteeID)
	pagination := assessment.NewPagination(page, pageSize)

	// 4. 查询（暂不支持状态筛选，后续可扩展 Repository 接口）
	l.Debugw("开始查询测评列表",
		"testee_id", dto.TesteeID,
		"page", page,
		"page_size", pageSize,
	)
	list, total, err := s.repo.FindByTesteeID(ctx, testeeID, pagination)
	if err != nil {
		l.Errorw("查询测评列表失败",
			"testee_id", dto.TesteeID,
			"action", "list_my_assessments",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询测评列表失败")
	}

	// 5. 转换结果
	items := make([]*AssessmentResult, len(list))
	for i, a := range list {
		items[i] = toAssessmentResult(a)
	}

	totalInt := int(total)
	duration := time.Since(startTime)
	l.Debugw("查询我的测评列表成功",
		"action", "list_my_assessments",
		"result", "success",
		"testee_id", dto.TesteeID,
		"total_count", totalInt,
		"page_count", len(list),
		"duration_ms", duration.Milliseconds(),
	)

	return &AssessmentListResult{
		Items:      items,
		Total:      totalInt,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: (totalInt + pageSize - 1) / pageSize,
	}, nil
}

// buildCreateRequest 构造创建请求
func (s *submissionService) buildCreateRequest(dto CreateAssessmentDTO) assessment.CreateAssessmentRequest {
	req := assessment.CreateAssessmentRequest{
		OrgID:    int64(dto.OrgID),
		TesteeID: meta.FromUint64(dto.TesteeID),
		QuestionnaireRef: assessment.NewQuestionnaireRef(
			meta.FromUint64(dto.QuestionnaireID),
			meta.NewCode(dto.QuestionnaireCode),
			dto.QuestionnaireVersion,
		),
		AnswerSheetRef: assessment.NewAnswerSheetRef(
			meta.FromUint64(dto.AnswerSheetID),
		),
	}

	// 设置量表引用（可选）
	if dto.MedicalScaleID != nil {
		scaleCode := ""
		if dto.MedicalScaleCode != nil {
			scaleCode = *dto.MedicalScaleCode
		}
		scaleName := ""
		if dto.MedicalScaleName != nil {
			scaleName = *dto.MedicalScaleName
		}
		scaleRef := assessment.NewMedicalScaleRef(
			meta.FromUint64(*dto.MedicalScaleID),
			meta.NewCode(scaleCode),
			scaleName,
		)
		req.MedicalScaleRef = &scaleRef
	}

	// 设置来源
	switch dto.OriginType {
	case "plan":
		if dto.OriginID != nil {
			req.Origin = assessment.NewPlanOrigin(*dto.OriginID)
		}
	case "screening":
		if dto.OriginID != nil {
			req.Origin = assessment.NewScreeningOrigin(*dto.OriginID)
		}
	default:
		req.Origin = assessment.NewAdhocOrigin()
	}

	return req
}

// normalizePagination 规范化分页参数
func normalizePagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

// publishEvents 发布聚合根收集的领域事件
func (s *submissionService) publishEvents(ctx context.Context, a *assessment.Assessment, l *logger.RequestLogger) {
	if s.eventPublisher == nil {
		l.Warnw("事件发布器未配置，跳过事件发布",
			"action", "publish_event",
			"resource", "assessment",
			"assessment_id", a.ID().Uint64(),
		)
		return
	}

	events := a.Events()
	for _, evt := range events {
		if err := s.eventPublisher.Publish(ctx, evt); err != nil {
			l.Errorw("发布领域事件失败",
				"action", "publish_event",
				"resource", "assessment",
				"assessment_id", a.ID().Uint64(),
				"event_type", evt.EventType(),
				"error", err.Error(),
			)
		} else {
			l.Infow("发布领域事件成功",
				"action", "publish_event",
				"resource", "assessment",
				"assessment_id", a.ID().Uint64(),
				"event_type", evt.EventType(),
			)
		}
	}

	// 清空已发布的事件
	a.ClearEvents()
}
