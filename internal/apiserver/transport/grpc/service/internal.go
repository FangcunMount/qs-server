package service

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	operatorApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine"
	notificationApp "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	answerSheetApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	domainoperator "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	domaintestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	planDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

// InternalService 内部 gRPC 服务 - 供 Worker 调用
// 用于事件处理后的业务逻辑调用。
type InternalService struct {
	pb.UnimplementedInternalServiceServer
	answerSheetScoringService answerSheetApp.AnswerSheetScoringService
	submissionService         assessmentApp.AssessmentSubmissionService
	managementService         assessmentApp.AssessmentManagementService
	engineService             engine.Service
	scaleRepo                 scale.Repository
	testeeTaggingService      testeeApp.TesteeTaggingService
	planTaskRepo              planDomain.AssessmentTaskRepository
	planCommandService        planApp.PlanCommandService
	operatorLifecycleService  operatorApp.OperatorLifecycleService
	operatorAuthService       operatorApp.OperatorAuthorizationService
	operatorQueryService      operatorApp.OperatorQueryService
	operatorRoleSyncer        operatorBootstrapRoleSyncer
	behaviorProjectorService  statisticsApp.BehaviorProjectorService
	warmupCoordinator         cachegov.Coordinator
	// 小程序码生成服务（可选）
	qrCodeService qrcodeApp.QRCodeService
	// 小程序 task 消息服务（可选）
	miniProgramTaskNotificationService notificationApp.MiniProgramTaskNotificationService
}

type assessmentScaleContext struct {
	medicalScaleID   *uint64
	medicalScaleCode *string
	medicalScaleName *string
}

type operatorBootstrapRoleSyncer interface {
	SyncRoles(ctx context.Context, orgID int64, operatorID uint64) error
}

type authzSnapshotOperatorRoleSyncer struct {
	operatorRepo  domainoperator.Repository
	authzSnapshot *iaminfra.AuthzSnapshotLoader
}

func (s authzSnapshotOperatorRoleSyncer) SyncRoles(ctx context.Context, orgID int64, operatorID uint64) error {
	if s.operatorRepo == nil || s.authzSnapshot == nil {
		return nil
	}

	op, err := s.operatorRepo.FindByID(ctx, domainoperator.ID(meta.FromUint64(operatorID)))
	if err != nil {
		return fmt.Errorf("load operator aggregate failed: %w", err)
	}
	if _, err := iaminfra.SyncAndPersistOperatorRolesFromSnapshot(ctx, s.authzSnapshot, s.operatorRepo, orgID, op); err != nil {
		return fmt.Errorf("sync operator roles from snapshot failed: %w", err)
	}
	return nil
}

// NewInternalService 创建内部 gRPC 服务
func NewInternalService(
	answerSheetScoringService answerSheetApp.AnswerSheetScoringService,
	submissionService assessmentApp.AssessmentSubmissionService,
	managementService assessmentApp.AssessmentManagementService,
	engineService engine.Service,
	scaleRepo scale.Repository,
	testeeTaggingService testeeApp.TesteeTaggingService,
	planTaskRepo planDomain.AssessmentTaskRepository,
	planCommandService planApp.PlanCommandService,
	operatorLifecycleService operatorApp.OperatorLifecycleService,
	operatorAuthService operatorApp.OperatorAuthorizationService,
	operatorQueryService operatorApp.OperatorQueryService,
	operatorRepo domainoperator.Repository,
	authzSnapshot *iaminfra.AuthzSnapshotLoader,
	behaviorProjectorService statisticsApp.BehaviorProjectorService,
	warmupCoordinator cachegov.Coordinator,
	qrCodeService interface{}, // qrcodeApp.QRCodeService，可能为 nil
	miniProgramTaskNotificationService notificationApp.MiniProgramTaskNotificationService,
) *InternalService {
	var qrService qrcodeApp.QRCodeService
	if q, ok := qrCodeService.(qrcodeApp.QRCodeService); ok {
		qrService = q
	}
	var roleSyncer operatorBootstrapRoleSyncer
	if operatorRepo != nil || authzSnapshot != nil {
		roleSyncer = authzSnapshotOperatorRoleSyncer{
			operatorRepo:  operatorRepo,
			authzSnapshot: authzSnapshot,
		}
	}

	return &InternalService{
		answerSheetScoringService:          answerSheetScoringService,
		submissionService:                  submissionService,
		managementService:                  managementService,
		engineService:                      engineService,
		scaleRepo:                          scaleRepo,
		testeeTaggingService:               testeeTaggingService,
		planTaskRepo:                       planTaskRepo,
		planCommandService:                 planCommandService,
		operatorLifecycleService:           operatorLifecycleService,
		operatorAuthService:                operatorAuthService,
		operatorQueryService:               operatorQueryService,
		operatorRoleSyncer:                 roleSyncer,
		behaviorProjectorService:           behaviorProjectorService,
		warmupCoordinator:                  warmupCoordinator,
		qrCodeService:                      qrService,
		miniProgramTaskNotificationService: miniProgramTaskNotificationService,
	}
}

// RegisterService 注册 gRPC 服务
func (s *InternalService) RegisterService(server *grpc.Server) {
	pb.RegisterInternalServiceServer(server, s)
}

func (s *InternalService) ProjectBehaviorEvent(
	ctx context.Context,
	req *pb.ProjectBehaviorEventRequest,
) (*pb.ProjectBehaviorEventResponse, error) {
	return newBehaviorProjectionFlow(s).ProjectBehaviorEvent(ctx, req)
}

// CalculateAnswerSheetScore 计算答卷分数
// 场景：worker 处理 answersheet.submitted 事件后调用
func (s *InternalService) CalculateAnswerSheetScore(
	ctx context.Context,
	req *pb.CalculateAnswerSheetScoreRequest,
) (*pb.CalculateAnswerSheetScoreResponse, error) {
	return newAssessmentFlow(s).CalculateAnswerSheetScore(ctx, req)
}

// CreateAssessmentFromAnswerSheet 从答卷创建测评
// 场景：worker 处理 answersheet.submitted 事件后调用（在计分之后）
func (s *InternalService) CreateAssessmentFromAnswerSheet(
	ctx context.Context,
	req *pb.CreateAssessmentFromAnswerSheetRequest,
) (*pb.CreateAssessmentFromAnswerSheetResponse, error) {
	return newAssessmentFlow(s).CreateAssessmentFromAnswerSheet(ctx, req)
}

func validateCreateAssessmentFromAnswerSheetRequest(req *pb.CreateAssessmentFromAnswerSheetRequest) error {
	switch {
	case req == nil:
		return status.Error(codes.InvalidArgument, "request 不能为空")
	case req.AnswersheetId == 0:
		return status.Error(codes.InvalidArgument, "answersheet_id 不能为空")
	case req.QuestionnaireCode == "":
		return status.Error(codes.InvalidArgument, "questionnaire_code 不能为空")
	case req.QuestionnaireVersion == "":
		return status.Error(codes.InvalidArgument, "questionnaire_version 不能为空")
	case req.TesteeId == 0:
		return status.Error(codes.InvalidArgument, "testee_id 不能为空")
	case req.FillerId == 0:
		return status.Error(codes.InvalidArgument, "filler_id 不能为空")
	default:
		return nil
	}
}

func (s *InternalService) resolveAssessmentScaleContext(ctx context.Context, questionnaireCode string) assessmentScaleContext {
	l := logger.L(ctx)
	if s.scaleRepo == nil || questionnaireCode == "" {
		return assessmentScaleContext{}
	}

	medicalScale, err := s.scaleRepo.FindByQuestionnaireCode(ctx, questionnaireCode)
	if err != nil || medicalScale == nil {
		l.Infow("问卷未关联量表，将创建纯问卷模式的测评",
			"questionnaire_code", questionnaireCode,
		)
		return assessmentScaleContext{}
	}

	scaleID := medicalScale.GetID().Uint64()
	scaleCode := medicalScale.GetCode().Value()
	scaleName := medicalScale.GetTitle()
	l.Infow("找到关联量表",
		"scale_id", scaleID,
		"scale_code", scaleCode,
		"scale_name", scaleName,
	)

	return assessmentScaleContext{
		medicalScaleID:   &scaleID,
		medicalScaleCode: &scaleCode,
		medicalScaleName: &scaleName,
	}
}

func buildCreateAssessmentDTO(
	req *pb.CreateAssessmentFromAnswerSheetRequest,
	scaleCtx assessmentScaleContext,
) assessmentApp.CreateAssessmentDTO {
	dto := assessmentApp.CreateAssessmentDTO{
		OrgID:                req.OrgId,
		TesteeID:             req.TesteeId,
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
		AnswerSheetID:        req.AnswersheetId,
		MedicalScaleID:       scaleCtx.medicalScaleID,
		MedicalScaleCode:     scaleCtx.medicalScaleCode,
		MedicalScaleName:     scaleCtx.medicalScaleName,
		OriginType:           req.OriginType,
	}
	if dto.OriginType == "" {
		dto.OriginType = "adhoc"
	}
	if req.OriginId != "" {
		dto.OriginID = &req.OriginId
	}
	return dto
}

func (s *InternalService) applyMatchedTaskOrigin(
	ctx context.Context,
	l *logger.RequestLogger,
	req *pb.CreateAssessmentFromAnswerSheetRequest,
	medicalScaleCode *string,
	dto *assessmentApp.CreateAssessmentDTO,
) *planDomain.AssessmentTask {
	var matchedTask *planDomain.AssessmentTask
	switch {
	case req.TaskId != "":
		matchedTask = s.resolvePlanTaskByID(ctx, req, medicalScaleCode)
	case medicalScaleCode != nil:
		matchedTask = s.resolveOpenedPlanTask(ctx, req.OrgId, req.TesteeId, *medicalScaleCode)
	}
	if matchedTask == nil {
		return nil
	}

	planID := matchedTask.GetPlanID().String()
	dto.OriginType = "plan"
	dto.OriginID = &planID
	l.Infow("识别到计划任务上下文",
		"task_id", matchedTask.GetID().String(),
		"plan_id", planID,
		"testee_id", req.TesteeId,
	)
	return matchedTask
}

func (s *InternalService) loadExistingAssessmentResponse(
	ctx context.Context,
	l *logger.RequestLogger,
	answerSheetID uint64,
	orgID uint64,
	matchedTask *planDomain.AssessmentTask,
) (*pb.CreateAssessmentFromAnswerSheetResponse, bool) {
	existing, err := s.submissionService.GetMyAssessmentByAnswerSheetID(ctx, answerSheetID)
	if err != nil || existing == nil {
		return nil, false
	}

	l.Infow("检测到答卷已创建测评，直接返回",
		"answersheet_id", answerSheetID,
		"assessment_id", existing.ID,
	)
	s.completeMatchedTask(ctx, l, orgID, matchedTask, existing.ID)
	return existingAssessmentResponse(existing.ID), true
}

func (s *InternalService) createAssessmentFromAnswerSheet(
	ctx context.Context,
	l *logger.RequestLogger,
	req *pb.CreateAssessmentFromAnswerSheetRequest,
	dto assessmentApp.CreateAssessmentDTO,
	matchedTask *planDomain.AssessmentTask,
	shouldAutoSubmit bool,
) (*pb.CreateAssessmentFromAnswerSheetResponse, error) {
	result, err := s.submissionService.Create(ctx, dto)
	if err != nil {
		if errors.IsCode(err, errorCode.ErrAssessmentDuplicate) {
			if response, ok := s.loadExistingAssessmentResponse(ctx, l, req.AnswersheetId, req.OrgId, matchedTask); ok {
				l.Infow("测评已存在，返回已有结果",
					"answersheet_id", req.AnswersheetId,
				)
				return response, nil
			}
		}

		l.Errorw("创建测评失败",
			"action", "create_assessment_from_answersheet",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, status.Errorf(codes.Internal, "创建测评失败: %v", err)
	}

	l.Infow("创建测评成功",
		"action", "create_assessment_from_answersheet",
		"assessment_id", result.ID,
		"result", "success",
	)

	autoSubmitted := false
	if shouldAutoSubmit {
		autoSubmitted = s.autoSubmitAssessment(ctx, l, result.ID)
	}

	s.completeMatchedTask(ctx, l, req.OrgId, matchedTask, result.ID)
	return createdAssessmentResponse(result.ID, autoSubmitted), nil
}

func (s *InternalService) autoSubmitAssessment(ctx context.Context, l *logger.RequestLogger, assessmentID uint64) bool {
	if _, err := s.submissionService.Submit(ctx, assessmentID); err != nil {
		l.Warnw("自动提交测评失败",
			"assessment_id", assessmentID,
			"error", err.Error(),
		)
		return false
	}

	l.Infow("自动提交测评成功",
		"assessment_id", assessmentID,
	)
	return true
}

func existingAssessmentResponse(assessmentID uint64) *pb.CreateAssessmentFromAnswerSheetResponse {
	return &pb.CreateAssessmentFromAnswerSheetResponse{
		AssessmentId:  assessmentID,
		Created:       false,
		AutoSubmitted: false,
		Message:       "测评已存在",
	}
}

func createdAssessmentResponse(assessmentID uint64, autoSubmitted bool) *pb.CreateAssessmentFromAnswerSheetResponse {
	return &pb.CreateAssessmentFromAnswerSheetResponse{
		AssessmentId:  assessmentID,
		Created:       true,
		AutoSubmitted: autoSubmitted,
		Message:       "测评创建成功",
	}
}

func (s *InternalService) resolvePlanTaskByID(
	ctx context.Context,
	req *pb.CreateAssessmentFromAnswerSheetRequest,
	medicalScaleCode *string,
) *planDomain.AssessmentTask {
	if s.planTaskRepo == nil || req.TaskId == "" {
		return nil
	}

	taskID, err := planDomain.ParseAssessmentTaskID(req.TaskId)
	if err != nil {
		logger.L(ctx).Warnw("计划任务ID格式非法，跳过显式任务识别",
			"task_id", req.TaskId,
			"error", err.Error(),
		)
		return nil
	}

	task, err := s.planTaskRepo.FindByID(ctx, taskID)
	if err != nil || task == nil {
		logger.L(ctx).Warnw("查询计划任务失败，跳过显式任务识别",
			"task_id", req.TaskId,
			"error", err,
		)
		return nil
	}

	requestOrgID, convErr := safeconv.Uint64ToInt64(req.OrgId)
	if convErr != nil {
		logger.L(ctx).Warnw("请求机构ID超出 int64 范围，跳过显式任务识别",
			"org_id", req.OrgId,
			"error", convErr.Error(),
		)
		return nil
	}
	if task.GetOrgID() != requestOrgID {
		logger.L(ctx).Warnw("计划任务机构不匹配，跳过显式任务识别",
			"task_id", req.TaskId,
			"request_org_id", req.OrgId,
			"task_org_id", task.GetOrgID(),
		)
		return nil
	}
	if task.GetTesteeID().Uint64() != req.TesteeId {
		logger.L(ctx).Warnw("计划任务受试者不匹配，跳过显式任务识别",
			"task_id", req.TaskId,
			"request_testee_id", req.TesteeId,
			"task_testee_id", task.GetTesteeID().Uint64(),
		)
		return nil
	}
	if !task.IsOpened() {
		logger.L(ctx).Warnw("计划任务未处于 opened 状态，跳过显式任务识别",
			"task_id", req.TaskId,
			"task_status", task.GetStatus().String(),
		)
		return nil
	}
	if medicalScaleCode == nil {
		logger.L(ctx).Warnw("计划任务已传入，但问卷未关联量表，无法建立计划测评关系",
			"task_id", req.TaskId,
			"questionnaire_code", req.QuestionnaireCode,
		)
		return nil
	}
	if task.GetScaleCode() != *medicalScaleCode {
		logger.L(ctx).Warnw("计划任务量表不匹配，跳过显式任务识别",
			"task_id", req.TaskId,
			"task_scale_code", task.GetScaleCode(),
			"request_scale_code", *medicalScaleCode,
		)
		return nil
	}

	return task
}

func (s *InternalService) resolveOpenedPlanTask(
	ctx context.Context,
	orgID uint64,
	testeeID uint64,
	scaleCode string,
) *planDomain.AssessmentTask {
	if s.planTaskRepo == nil || scaleCode == "" || testeeID == 0 {
		return nil
	}

	tasks, err := s.planTaskRepo.FindByTesteeID(ctx, domaintestee.ID(meta.FromUint64(testeeID)))
	if err != nil {
		logger.L(ctx).Warnw("查询受试者计划任务失败",
			"testee_id", testeeID,
			"scale_code", scaleCode,
			"error", err.Error(),
		)
		return nil
	}

	var matched *planDomain.AssessmentTask
	targetOrgID, convErr := safeconv.Uint64ToInt64(orgID)
	if convErr != nil {
		logger.L(ctx).Warnw("机构ID超出 int64 范围，跳过自动 plan 识别",
			"org_id", orgID,
			"error", convErr.Error(),
		)
		return nil
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if task.GetOrgID() != targetOrgID || task.GetScaleCode() != scaleCode || !task.IsOpened() {
			continue
		}
		if matched != nil {
			logger.L(ctx).Warnw("存在多个候选 opened task，跳过自动 plan 识别",
				"testee_id", testeeID,
				"org_id", orgID,
				"scale_code", scaleCode,
				"first_task_id", matched.GetID().String(),
				"second_task_id", task.GetID().String(),
			)
			return nil
		}
		matched = task
	}

	return matched
}

func (s *InternalService) completeMatchedTask(
	ctx context.Context,
	l *logger.RequestLogger,
	orgID uint64,
	task *planDomain.AssessmentTask,
	assessmentID uint64,
) {
	if task == nil || s.planCommandService == nil || assessmentID == 0 {
		return
	}
	if task.IsCompleted() {
		return
	}
	targetOrgID, err := safeconv.Uint64ToInt64(orgID)
	if err != nil {
		l.Warnw("机构ID超出 int64 范围，跳过计划任务完成回写",
			"org_id", orgID,
			"assessment_id", assessmentID,
			"error", err.Error(),
		)
		return
	}

	if _, err := s.planCommandService.CompleteTask(
		ctx,
		targetOrgID,
		task.GetID().String(),
		meta.FromUint64(assessmentID).String(),
	); err != nil {
		l.Warnw("回写计划任务完成状态失败",
			"task_id", task.GetID().String(),
			"assessment_id", assessmentID,
			"error", err.Error(),
		)
		return
	}

	l.Infow("已回写计划任务完成状态",
		"task_id", task.GetID().String(),
		"plan_id", task.GetPlanID().String(),
		"assessment_id", assessmentID,
	)
}

// EvaluateAssessment 执行测评评估
// 场景：worker 处理 assessment.submitted 事件后调用
func (s *InternalService) EvaluateAssessment(
	ctx context.Context,
	req *pb.EvaluateAssessmentRequest,
) (*pb.EvaluateAssessmentResponse, error) {
	return newAssessmentFlow(s).EvaluateAssessment(ctx, req)
}

// TagTestee 给受试者打标签
// 场景：worker 处理 report.generated 事件后调用
// 职责：协议转换，将 gRPC 请求转换为应用服务调用
// 业务逻辑：由 TesteeTaggingService 处理
func (s *InternalService) TagTestee(
	ctx context.Context,
	req *pb.TagTesteeRequest,
) (*pb.TagTesteeResponse, error) {
	return newAssessmentFlow(s).TagTestee(ctx, req)
}

// ==================== 小程序码生成操作 ====================

// GenerateQuestionnaireQRCode 生成问卷小程序码
// 场景：worker 处理 questionnaire.changed(published) 事件后调用
func (s *InternalService) GenerateQuestionnaireQRCode(
	ctx context.Context,
	req *pb.GenerateQuestionnaireQRCodeRequest,
) (*pb.GenerateQuestionnaireQRCodeResponse, error) {
	return newNotificationFlow(s).GenerateQuestionnaireQRCode(ctx, req)
}

func (s *InternalService) HandleQuestionnairePublishedPostActions(
	ctx context.Context,
	req *pb.GenerateQuestionnaireQRCodeRequest,
) (*pb.GenerateQuestionnaireQRCodeResponse, error) {
	return newNotificationFlow(s).HandleQuestionnairePublishedPostActions(ctx, req)
}

func (s *InternalService) generateQuestionnaireQRCode(
	ctx context.Context,
	req *pb.GenerateQuestionnaireQRCodeRequest,
) (*pb.GenerateQuestionnaireQRCodeResponse, error) {
	l := logger.L(ctx)

	l.Infow("gRPC: 收到生成问卷小程序码请求",
		"action", "generate_questionnaire_qrcode",
		"code", req.Code,
		"version", req.Version,
	)

	// 验证参数
	if req.Code == "" {
		return nil, status.Error(codes.InvalidArgument, "code 不能为空")
	}
	if req.Version == "" {
		return nil, status.Error(codes.InvalidArgument, "version 不能为空")
	}

	// 检查小程序码生成服务是否配置
	if s.qrCodeService == nil {
		l.Warnw("小程序码生成服务未配置",
			"action", "generate_questionnaire_qrcode",
		)
		return &pb.GenerateQuestionnaireQRCodeResponse{
			Success: false,
			Message: "小程序码生成功能未配置",
		}, nil
	}

	// 调用应用层服务生成小程序码
	qrCodeURL, err := s.qrCodeService.GenerateQuestionnaireQRCode(ctx, req.Code, req.Version)
	if err != nil {
		l.Errorw("生成问卷小程序码失败",
			"action", "generate_questionnaire_qrcode",
			"code", req.Code,
			"version", req.Version,
			"error", err.Error(),
		)
		return &pb.GenerateQuestionnaireQRCodeResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	l.Infow("问卷小程序码生成成功",
		"action", "generate_questionnaire_qrcode",
		"code", req.Code,
		"version", req.Version,
		"qrcode_url", qrCodeURL,
	)

	return &pb.GenerateQuestionnaireQRCodeResponse{
		Success:   true,
		QrcodeUrl: qrCodeURL,
		Message:   "小程序码生成成功",
	}, nil
}

// GenerateScaleQRCode 生成量表小程序码
// 场景：worker 处理 scale.changed(published) 事件后调用
func (s *InternalService) GenerateScaleQRCode(
	ctx context.Context,
	req *pb.GenerateScaleQRCodeRequest,
) (*pb.GenerateScaleQRCodeResponse, error) {
	return newNotificationFlow(s).GenerateScaleQRCode(ctx, req)
}

func (s *InternalService) HandleScalePublishedPostActions(
	ctx context.Context,
	req *pb.GenerateScaleQRCodeRequest,
) (*pb.GenerateScaleQRCodeResponse, error) {
	return newNotificationFlow(s).HandleScalePublishedPostActions(ctx, req)
}

func (s *InternalService) generateScaleQRCode(
	ctx context.Context,
	req *pb.GenerateScaleQRCodeRequest,
) (*pb.GenerateScaleQRCodeResponse, error) {
	l := logger.L(ctx)

	l.Infow("gRPC: 收到生成量表小程序码请求",
		"action", "generate_scale_qrcode",
		"code", req.Code,
	)

	// 验证参数
	if req.Code == "" {
		return nil, status.Error(codes.InvalidArgument, "code 不能为空")
	}

	// 检查小程序码生成服务是否配置
	if s.qrCodeService == nil {
		l.Warnw("小程序码生成服务未配置",
			"action", "generate_scale_qrcode",
		)
		return &pb.GenerateScaleQRCodeResponse{
			Success: false,
			Message: "小程序码生成功能未配置",
		}, nil
	}

	// 调用应用层服务生成小程序码
	qrCodeURL, err := s.qrCodeService.GenerateScaleQRCode(ctx, req.Code)
	if err != nil {
		l.Errorw("生成量表小程序码失败",
			"action", "generate_scale_qrcode",
			"code", req.Code,
			"error", err.Error(),
		)
		return &pb.GenerateScaleQRCodeResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	l.Infow("量表小程序码生成成功",
		"action", "generate_scale_qrcode",
		"code", req.Code,
		"qrcode_url", qrCodeURL,
	)

	return &pb.GenerateScaleQRCodeResponse{
		Success:   true,
		QrcodeUrl: qrCodeURL,
		Message:   "小程序码生成成功",
	}, nil
}

// SendTaskOpenedMiniProgramNotification 发送 task.opened 小程序订阅消息。
func (s *InternalService) SendTaskOpenedMiniProgramNotification(
	ctx context.Context,
	req *pb.SendTaskOpenedMiniProgramNotificationRequest,
) (*pb.SendTaskOpenedMiniProgramNotificationResponse, error) {
	return newNotificationFlow(s).SendTaskOpenedMiniProgramNotification(ctx, req)
}

// BootstrapOperator 自举首个操作者。
func (s *InternalService) BootstrapOperator(
	ctx context.Context,
	req *pb.BootstrapOperatorRequest,
) (*pb.BootstrapOperatorResponse, error) {
	return newOperatorBootstrapFlow(s).BootstrapOperator(ctx, req)
}
