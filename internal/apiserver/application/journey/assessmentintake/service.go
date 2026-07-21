// Package assessmentintake coordinates the submitted-answer-sheet journey
// across Survey, Model Catalog, Plan, Evaluation and report-status projection.
package assessmentintake

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaluationintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	planapp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	answersheetapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

const assessmentStatusPending = "pending"

// Command 评估入库命令
type Command struct {
	OrgID                uint64 // 组织ID
	AnswerSheetID        uint64 // 答案卡ID
	QuestionnaireCode    string // 问卷编码
	QuestionnaireVersion string // 问卷版本
	TesteeID             uint64 // 被试ID
	FillerID             uint64 // 填表人ID
	TaskID               string // 任务ID
	OriginType           string // 来源类型
	OriginID             string // 来源ID
	// Admission is the submit-time frozen evaluation intent (EV-R001).
	// Nil means legacy event without admission; Journey falls back to live binding.
	Admission *Admission
}

// Admission freezes evaluation purpose and exact release for Journey Ensure.
type Admission struct {
	Purpose              string
	QuestionnaireCode    string
	QuestionnaireVersion string
	ModelKind            string
	ModelSubKind         string
	ModelAlgorithm       string
	ModelCode            string
	ModelVersion         string
	ModelTitle           string
}

func (a *Admission) RequiresAssessment() bool {
	return a != nil && a.Purpose == string(domainanswersheet.AdmissionPurposeAssessment)
}

func (a *Admission) IsIndependent() bool {
	return a != nil && a.Purpose == string(domainanswersheet.AdmissionPurposeIndependentQuestionnaire)
}

// Result 评估入库结果。
// AssessmentID 为 0 且 Created/AutoSubmitted 均为 false 时，表示独立问卷无需创建 Assessment。
type Result struct {
	AssessmentID  uint64 // 评估ID；独立问卷无绑定时为 0
	Created       bool   // 是否创建
	AutoSubmitted bool   // 是否自动提交
}

// Service 评估入库服务
type Service interface {
	// Ensure 确保评估入库
	Ensure(context.Context, Command) (*Result, error)
}

// service 评估入库服务实现
type service struct {
	scoring      answersheetapp.AnswerSheetScoringService
	binding      rulesetport.AssessmentBindingResolver
	plans        planapp.TaskAssessmentResolver
	planCommands planapp.PlanCommandService
	intake       evaluationintake.Service
	reportStatus *reportstatus.Reporter
}

// NewService 创建评估入库服务
func NewService(scoring answersheetapp.AnswerSheetScoringService, binding rulesetport.AssessmentBindingResolver, plans planapp.TaskAssessmentResolver, planCommands planapp.PlanCommandService, intake evaluationintake.Service, reportStatus *reportstatus.Reporter) Service {
	return &service{scoring: scoring, binding: binding, plans: plans, planCommands: planCommands, intake: intake, reportStatus: reportStatus}
}

// Ensure 确保评估入库
func (s *service) Ensure(ctx context.Context, command Command) (*Result, error) {
	l := logger.L(ctx)
	startedAt := time.Now()
	l.Infow("开始答卷测评入库",
		"action", "ensure_assessment",
		"answersheet_id", command.AnswerSheetID,
		"org_id", command.OrgID,
		"testee_id", command.TesteeID,
		"questionnaire_code", command.QuestionnaireCode,
		"questionnaire_version", command.QuestionnaireVersion,
		"task_id", command.TaskID,
	)
	// 验证命令参数
	if command.OrgID == 0 || command.AnswerSheetID == 0 || command.QuestionnaireCode == "" || command.QuestionnaireVersion == "" || command.TesteeID == 0 || command.FillerID == 0 {
		return nil, errors.WithCode(code.ErrInvalidArgument, "assessment intake command is incomplete")
	}
	if s.intake == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "evaluation intake is not configured")
	}
	if s.scoring != nil {
		if err := s.scoring.CalculateAndSave(ctx, command.AnswerSheetID); err != nil {
			return nil, err
		}
		l.Infow("答卷评分已持久化",
			"action", "ensure_assessment",
			"answersheet_id", command.AnswerSheetID,
			"testee_id", command.TesteeID,
		)
	}

	// 创建评估入库命令
	dto := evaluationintake.CreateCommand{OrgID: command.OrgID, TesteeID: command.TesteeID, QuestionnaireCode: command.QuestionnaireCode, QuestionnaireVersion: command.QuestionnaireVersion, AnswerSheetID: command.AnswerSheetID, OriginType: command.OriginType}
	if dto.OriginType == "" {
		dto.OriginType = "adhoc"
	}
	if command.OriginID != "" {
		dto.OriginID = &command.OriginID
	}

	// 应用绑定：优先消费提交时冻结的 Admission（EV-R001）。
	bound, admissionSource, err := s.applyAdmissionOrBinding(ctx, command, &dto)
	if err != nil {
		return nil, err
	}
	l.Infow("答卷测评绑定已解析",
		"action", "ensure_assessment",
		"answersheet_id", command.AnswerSheetID,
		"bound", bound,
		"admission_source", admissionSource,
		"model_kind", valueOrEmpty(dto.ModelKind),
		"model_code", valueOrEmpty(dto.ModelCode),
		"model_version", valueOrEmpty(dto.ModelVersion),
	)

	// 仅已绑定测评模型时匹配计划；独立问卷不得完成 Plan Task。
	var matched *planapp.TaskAssessmentContext
	if bound {
		matched = s.matchPlan(ctx, command, dto.ModelCode)
		if matched != nil {
			dto.OriginType = "plan"
			dto.OriginID = &matched.PlanID
		}
	}

	// 查找已存在的评估（含历史空壳 Assessment 的幂等复用）
	existing, findErr := s.intake.FindByAnswerSheetID(ctx, command.AnswerSheetID)
	switch {
	case findErr == nil && existing != nil:
		autoSubmitted, submitErr := s.submitPendingBoundAssessment(ctx, command, existing, bound)
		if submitErr != nil {
			return nil, submitErr
		}
		if bound {
			s.completePlanBestEffort(ctx, command.OrgID, matched, existing.ID)
		}
		l.Infow("答卷已有关联测评，复用已有测评",
			"action", "ensure_assessment",
			"answersheet_id", command.AnswerSheetID,
			"assessment_id", existing.ID,
			"assessment_status", existing.Status,
			"bound", bound,
			"auto_submitted", autoSubmitted,
			"find_result", "found",
			"result", "idempotent_hit",
		)
		return &Result{AssessmentID: existing.ID, AutoSubmitted: autoSubmitted}, nil
	case findErr != nil && !evalerrors.IsAssessmentNotFound(findErr):
		l.Errorw("答卷测评查询依赖失败，跳过创建",
			"action", "ensure_assessment",
			"answersheet_id", command.AnswerSheetID,
			"find_result", "dependency_error",
			"error", findErr.Error(),
		)
		return nil, findErr
	default:
		l.Infow("答卷尚无关联测评",
			"action", "ensure_assessment",
			"answersheet_id", command.AnswerSheetID,
			"find_result", "not_found",
			"bound", bound,
		)
	}

	// 独立 Questionnaire：保留 AnswerSheet 与基础题分后结束，不创建 Assessment。
	if !bound {
		l.Infow("答卷未绑定测评模型，入库链路在基础分后结束",
			"action", "ensure_assessment",
			"answersheet_id", command.AnswerSheetID,
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"result", "no_assessment_required",
		)
		return &Result{}, nil
	}

	// 创建评估
	created, err := s.intake.CreateForAnswerSheet(ctx, dto)
	if err != nil {
		if errors.IsCode(err, code.ErrAssessmentDuplicate) {
			existing, findErr := s.intake.FindByAnswerSheetID(ctx, command.AnswerSheetID)
			switch {
			case findErr == nil && existing != nil:
				autoSubmitted, submitErr := s.submitPendingBoundAssessment(ctx, command, existing, bound)
				if submitErr != nil {
					return nil, submitErr
				}
				s.completePlanBestEffort(ctx, command.OrgID, matched, existing.ID)
				l.Infow("测评创建冲突后复用已有测评",
					"action", "ensure_assessment",
					"answersheet_id", command.AnswerSheetID,
					"assessment_id", existing.ID,
					"find_result", "found",
					"result", "duplicate_hit",
				)
				return &Result{AssessmentID: existing.ID, AutoSubmitted: autoSubmitted}, nil
			case findErr != nil && !evalerrors.IsAssessmentNotFound(findErr):
				l.Errorw("测评创建冲突后再查依赖失败",
					"action", "ensure_assessment",
					"answersheet_id", command.AnswerSheetID,
					"find_result", "dependency_error",
					"error", findErr.Error(),
				)
				return nil, findErr
			}
		}
		return nil, err
	}

	// 创建结果
	result := &Result{AssessmentID: created.ID, Created: true}
	result.AutoSubmitted, err = s.submitPendingBoundAssessment(ctx, command, created, bound)
	if err != nil {
		return nil, err
	}

	// 完成计划最佳实践
	s.completePlanBestEffort(ctx, command.OrgID, matched, created.ID)

	// 设置报告状态
	if s.reportStatus != nil {
		s.reportStatus.SetQueued(ctx, reportstatus.AssessmentKey(created.ID), reportstatus.AssessmentKey(command.AnswerSheetID))
	}
	l.Infow("测评已创建并进入后续评估链路",
		"action", "ensure_assessment",
		"answersheet_id", command.AnswerSheetID,
		"assessment_id", created.ID,
		"bound", bound,
		"auto_submitted", result.AutoSubmitted,
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"result", "success",
	)
	return result, nil
}

// submitPendingBoundAssessment submits only a bound, pending assessment. This
// makes answersheet.submitted worker replays recover an assessment that was
// created before a prior automatic submission failed, while leaving submitted
// and terminal assessments idempotent.
func (s *service) submitPendingBoundAssessment(ctx context.Context, command Command, item *evaluationintake.Assessment, bound bool) (bool, error) {
	if !bound || item == nil || item.Status != assessmentStatusPending {
		return false, nil
	}

	if _, err := s.intake.SubmitForEvaluation(ctx, item.ID); err != nil {
		logger.L(ctx).Errorw("测评自动提交失败",
			"action", "ensure_assessment",
			"answersheet_id", command.AnswerSheetID,
			"assessment_id", item.ID,
			"assessment_status", item.Status,
			"error", err.Error(),
		)
		return false, evalerrors.AssessmentSubmitFailed(err, "自动提交测评失败")
	}

	logger.L(ctx).Infow("测评已自动提交，等待评估事件处理",
		"action", "ensure_assessment",
		"answersheet_id", command.AnswerSheetID,
		"assessment_id", item.ID,
		"assessment_status", item.Status,
		"result", "auto_submitted",
	)
	return true, nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// applyAdmissionOrBinding prefers submit-time Admission; legacy events fall back to live binding.
func (s *service) applyAdmissionOrBinding(ctx context.Context, command Command, dto *evaluationintake.CreateCommand) (bound bool, source string, err error) {
	if command.Admission != nil {
		if command.Admission.IsIndependent() {
			return false, "frozen_admission", nil
		}
		if command.Admission.RequiresAssessment() {
			if err := applyFrozenAdmission(command.Admission, dto); err != nil {
				return false, "frozen_admission", err
			}
			return true, "frozen_admission", nil
		}
		return false, "frozen_admission", errors.WithCode(code.ErrInvalidArgument, "admission purpose is invalid: %s", command.Admission.Purpose)
	}
	bound, err = s.applyBinding(ctx, command, dto)
	return bound, "legacy_binding", err
}

func applyFrozenAdmission(admission *Admission, dto *evaluationintake.CreateCommand) error {
	if admission == nil || !admission.RequiresAssessment() {
		return errors.WithCode(code.ErrInvalidArgument, "assessment admission is required")
	}
	if admission.ModelKind == "" || admission.ModelCode == "" || admission.ModelVersion == "" {
		return errors.WithCode(code.ErrInvalidArgument, "assessment admission model identity is incomplete")
	}
	kind := admission.ModelKind
	dto.ModelKind = &kind
	if admission.ModelSubKind != "" {
		v := admission.ModelSubKind
		dto.ModelSubKind = &v
	}
	if admission.ModelAlgorithm != "" {
		v := admission.ModelAlgorithm
		dto.ModelAlgorithm = &v
	}
	modelCode := admission.ModelCode
	modelVersion := admission.ModelVersion
	dto.ModelCode = &modelCode
	dto.ModelVersion = &modelVersion
	if admission.ModelTitle != "" {
		title := admission.ModelTitle
		dto.ModelTitle = &title
	}
	return nil
}

// applyBinding 应用绑定
func (s *service) applyBinding(ctx context.Context, command Command, dto *evaluationintake.CreateCommand) (bool, error) {
	if s.binding == nil {
		return false, nil
	}
	binding, ok, err := s.binding.ResolveAssessmentBinding(ctx, command.QuestionnaireCode, command.QuestionnaireVersion)
	if err != nil || !ok {
		return false, err
	}
	kind, subKind, algorithm, mapped := modelcatalog.LegacyKindMapping(binding.Ref.Kind)
	if !mapped {
		kind = binding.Ref.Kind
	}
	if binding.Ref.SubKind != "" {
		subKind = binding.Ref.SubKind
	}
	if binding.Ref.Algorithm != "" {
		algorithm = binding.Ref.Algorithm
	}
	k := kind.String()
	dto.ModelKind = &k
	if subKind != "" {
		v := subKind.String()
		dto.ModelSubKind = &v
	}
	if algorithm != "" {
		v := algorithm.String()
		dto.ModelAlgorithm = &v
	}
	dto.ModelCode = &binding.Ref.Code
	dto.ModelVersion = &binding.Ref.Version
	dto.ModelTitle = &binding.Ref.Title
	return true, nil
}

// matchPlan 匹配计划
func (s *service) matchPlan(ctx context.Context, command Command, modelCode *string) *planapp.TaskAssessmentContext {
	if s.plans == nil {
		return nil
	}
	if command.TaskID != "" {
		code := ""
		if modelCode != nil {
			code = *modelCode
		}
		return s.plans.ResolveTaskByIDForAssessment(ctx, planapp.TaskAssessmentResolveInput{TaskID: command.TaskID, OrgID: command.OrgID, TesteeID: command.TesteeID, ScaleCode: code, QuestionnaireCode: command.QuestionnaireCode})
	}
	if modelCode != nil {
		return s.plans.ResolveOpenedTaskForAssessment(ctx, planapp.OpenedTaskResolveInput{OrgID: command.OrgID, TesteeID: command.TesteeID, ScaleCode: *modelCode})
	}
	return nil
}

// completePlanBestEffort 完成计划最佳实践
func (s *service) completePlanBestEffort(ctx context.Context, orgID uint64, task *planapp.TaskAssessmentContext, assessmentID uint64) {
	if task == nil || task.Completed || s.planCommands == nil {
		return
	}
	orgScope, err := safeconv.Uint64ToInt64(orgID)
	if err != nil {
		return
	}
	_, _ = s.planCommands.CompleteTask(ctx, orgScope, task.TaskID, meta.FromUint64(assessmentID).String())
}
