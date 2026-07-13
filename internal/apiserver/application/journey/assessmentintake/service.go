// Package assessmentintake coordinates the submitted-answer-sheet journey
// across Survey, Model Catalog, Plan, Evaluation and report-status projection.
package assessmentintake

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	evaluationintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	planapp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	answersheetapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

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
}

// Result 评估入库结果
type Result struct {
	AssessmentID  uint64 // 评估ID
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
	}

	// 创建评估入库命令
	dto := evaluationintake.CreateCommand{OrgID: command.OrgID, TesteeID: command.TesteeID, QuestionnaireCode: command.QuestionnaireCode, QuestionnaireVersion: command.QuestionnaireVersion, AnswerSheetID: command.AnswerSheetID, OriginType: command.OriginType}
	if dto.OriginType == "" {
		dto.OriginType = "adhoc"
	}
	if command.OriginID != "" {
		dto.OriginID = &command.OriginID
	}

	// 应用绑定
	bound, err := s.applyBinding(ctx, command, &dto)
	if err != nil {
		return nil, err
	}

	// 匹配计划
	matched := s.matchPlan(ctx, command, dto.ModelCode)
	if matched != nil {
		dto.OriginType = "plan"
		dto.OriginID = &matched.PlanID
	}

	// 查找已存在的评估
	if existing, findErr := s.intake.FindByAnswerSheetID(ctx, command.AnswerSheetID); findErr == nil && existing != nil {
		s.completePlanBestEffort(ctx, command.OrgID, matched, existing.ID)
		return &Result{AssessmentID: existing.ID}, nil
	}

	// 创建评估
	created, err := s.intake.CreateForAnswerSheet(ctx, dto)
	if err != nil {
		if errors.IsCode(err, code.ErrAssessmentDuplicate) {
			if existing, findErr := s.intake.FindByAnswerSheetID(ctx, command.AnswerSheetID); findErr == nil && existing != nil {
				s.completePlanBestEffort(ctx, command.OrgID, matched, existing.ID)
				return &Result{AssessmentID: existing.ID}, nil
			}
		}
		return nil, err
	}

	// 创建结果
	result := &Result{AssessmentID: created.ID, Created: true}
	if bound {
		// 自动提交评估
		if _, submitErr := s.intake.SubmitForEvaluation(ctx, created.ID); submitErr == nil {
			result.AutoSubmitted = true
		}
	}

	// 完成计划最佳实践
	s.completePlanBestEffort(ctx, command.OrgID, matched, created.ID)

	// 设置报告状态
	if s.reportStatus != nil {
		s.reportStatus.SetQueued(ctx, reportstatus.AssessmentKey(created.ID), reportstatus.AssessmentKey(command.AnswerSheetID))
	}
	return result, nil
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
