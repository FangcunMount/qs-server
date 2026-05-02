package assessment

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type assessmentListCache interface {
	Get(ctx context.Context, userID uint64, page, pageSize int, status, scaleCode, riskLevel, dateFrom, dateTo string, dest interface{}) error
	Set(ctx context.Context, userID uint64, page, pageSize int, status, scaleCode, riskLevel, dateFrom, dateTo string, value interface{})
	Invalidate(ctx context.Context, userID uint64) error
}

// submissionService 测评提交服务实现
// 行为者：答题者 (Testee)
type submissionService struct {
	repo        assessment.Repository
	reader      evaluationreadmodel.AssessmentReader
	creator     assessment.AssessmentCreator
	txRunner    apptransaction.Runner
	eventStager EventStager
	listCache   assessmentListCache
}

// NewSubmissionService 创建测评提交服务
func NewSubmissionService(
	repo assessment.Repository,
	creator assessment.AssessmentCreator,
	_ event.EventPublisher,
) AssessmentSubmissionService {
	return &submissionService{
		repo:    repo,
		creator: creator,
	}
}

func NewSubmissionServiceWithReadModel(
	repo assessment.Repository,
	reader evaluationreadmodel.AssessmentReader,
	creator assessment.AssessmentCreator,
	_ event.EventPublisher,
) AssessmentSubmissionService {
	return &submissionService{
		repo:    repo,
		reader:  reader,
		creator: creator,
	}
}

// NewSubmissionServiceWithListCache 创建带“我的测评列表”缓存的测评提交服务
func NewSubmissionServiceWithListCache(
	repo assessment.Repository,
	creator assessment.AssessmentCreator,
	_ event.EventPublisher,
	listCache assessmentListCache,
) AssessmentSubmissionService {
	return &submissionService{
		repo:      repo,
		creator:   creator,
		listCache: listCache,
	}
}

func NewSubmissionServiceWithTransactionalOutbox(
	repo assessment.Repository,
	creator assessment.AssessmentCreator,
	txRunner apptransaction.Runner,
	eventStager EventStager,
	listCache assessmentListCache,
) AssessmentSubmissionService {
	return &submissionService{
		repo:        repo,
		creator:     creator,
		txRunner:    txRunner,
		eventStager: eventStager,
		listCache:   listCache,
	}
}

func NewSubmissionServiceWithTransactionalOutboxAndReadModel(
	repo assessment.Repository,
	reader evaluationreadmodel.AssessmentReader,
	creator assessment.AssessmentCreator,
	txRunner apptransaction.Runner,
	eventStager EventStager,
	listCache assessmentListCache,
) AssessmentSubmissionService {
	return &submissionService{
		repo:        repo,
		reader:      reader,
		creator:     creator,
		txRunner:    txRunner,
		eventStager: eventStager,
		listCache:   listCache,
	}
}

func (s *submissionService) Create(ctx context.Context, dto CreateAssessmentDTO) (*AssessmentResult, error) {
	return assessmentCreatorWorkflow{service: s}.Create(ctx, dto)
}

type assessmentCreatorWorkflow struct {
	service *submissionService
}

// Create 创建测评
func (w assessmentCreatorWorkflow) Create(ctx context.Context, dto CreateAssessmentDTO) (*AssessmentResult, error) {
	s := w.service
	l := logger.L(ctx)
	startTime := time.Now()

	l.Infow("开始创建测评",
		"action", "create_assessment",
		"resource", "assessment",
		"testee_id", dto.TesteeID,
		"questionnaire_code", dto.QuestionnaireCode,
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
	// 问卷编码验证
	if dto.QuestionnaireCode == "" {
		l.Warnw("问卷编码为空",
			"action", "create_assessment",
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "问卷编码不能为空")
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
	req, err := s.buildCreateRequest(dto)
	if err != nil {
		l.Warnw("来源类型无效",
			"origin_type", dto.OriginType,
			"result", "invalid_params",
			"error", err.Error(),
		)
		return nil, err
	}

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
	occurredAt := time.Now()
	additionalEvents := []event.DomainEvent{
		domainStatistics.NewFootprintAssessmentCreatedEvent(req.OrgID, dto.TesteeID, dto.AnswerSheetID, a.ID().Uint64(), occurredAt),
	}
	if err := saveAssessmentAndStageEvents(ctx, s.repo, s.txRunner, s.eventStager, a, additionalEvents); err != nil {
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

	myAssessmentListCacheHelper{cache: s.listCache}.Invalidate(ctx, dto.TesteeID)

	result, convErr := toAssessmentResult(a)
	if convErr != nil {
		return nil, convErr
	}

	return result, nil
}

func (s *submissionService) Submit(ctx context.Context, assessmentID uint64) (*AssessmentResult, error) {
	return assessmentSubmitWorkflow{service: s}.Submit(ctx, assessmentID)
}

type assessmentSubmitWorkflow struct {
	service *submissionService
}

// Submit 提交测评
func (w assessmentSubmitWorkflow) Submit(ctx context.Context, assessmentID uint64) (*AssessmentResult, error) {
	s := w.service
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
	if err := saveAssessmentAndStageEvents(ctx, s.repo, s.txRunner, s.eventStager, a, nil); err != nil {
		l.Errorw("保存测评失败",
			"assessment_id", assessmentID,
			"action", "submit_assessment",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存测评失败")
	}

	duration := time.Since(startTime)
	l.Infow("提交测评成功",
		"action", "submit_assessment",
		"result", "success",
		"assessment_id", assessmentID,
		"duration_ms", duration.Milliseconds(),
	)

	myAssessmentListCacheHelper{cache: s.listCache}.Invalidate(ctx, a.TesteeID().Uint64())

	result, convErr := toAssessmentResult(a)
	if convErr != nil {
		return nil, convErr
	}

	return result, nil
}

// GetMyAssessment 获取我的测评详情
func (s *submissionService) GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentResult, error) {
	return assessmentGetter{service: s}.GetMyAssessment(ctx, testeeID, assessmentID)
}

// GetMyAssessmentByAnswerSheetID 通过答卷ID获取测评详情
func (s *submissionService) GetMyAssessmentByAnswerSheetID(ctx context.Context, answerSheetID uint64) (*AssessmentResult, error) {
	return assessmentGetter{service: s}.GetMyAssessmentByAnswerSheetID(ctx, answerSheetID)
}

// ListMyAssessments 查询我的测评列表
func (s *submissionService) ListMyAssessments(ctx context.Context, dto ListMyAssessmentsDTO) (*AssessmentListResult, error) {
	return myAssessmentListWorkflow{service: s}.List(ctx, dto)
}

// buildCreateRequest 构造创建请求
func (s *submissionService) buildCreateRequest(dto CreateAssessmentDTO) (assessment.CreateAssessmentRequest, error) {
	orgID, err := safeconv.Uint64ToInt64(dto.OrgID)
	if err != nil {
		return assessment.CreateAssessmentRequest{}, errors.WithCode(errorCode.ErrInvalidArgument, "机构ID超出 int64 范围")
	}

	req := assessment.CreateAssessmentRequest{
		OrgID:    orgID,
		TesteeID: meta.FromUint64(dto.TesteeID),
		QuestionnaireRef: assessment.NewQuestionnaireRefByCode(
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
	case "", "adhoc":
		req.Origin = assessment.NewAdhocOrigin()
	case "plan":
		if dto.OriginID != nil {
			req.Origin = assessment.NewPlanOrigin(*dto.OriginID)
		}
	default:
		return assessment.CreateAssessmentRequest{}, errors.WithCode(errorCode.ErrInvalidArgument, "不支持的来源类型: %s", dto.OriginType)
	}

	return req, nil
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
