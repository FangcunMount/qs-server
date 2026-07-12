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

type Command struct {
	OrgID                uint64
	AnswerSheetID        uint64
	QuestionnaireCode    string
	QuestionnaireVersion string
	TesteeID             uint64
	FillerID             uint64
	TaskID               string
	OriginType           string
	OriginID             string
}

type Result struct {
	AssessmentID  uint64
	Created       bool
	AutoSubmitted bool
}

type Service interface {
	Ensure(context.Context, Command) (*Result, error)
}

type service struct {
	scoring      answersheetapp.AnswerSheetScoringService
	binding      rulesetport.AssessmentBindingResolver
	plans        planapp.TaskAssessmentResolver
	planCommands planapp.PlanCommandService
	intake       evaluationintake.Service
	reportStatus *reportstatus.Reporter
}

func NewService(scoring answersheetapp.AnswerSheetScoringService, binding rulesetport.AssessmentBindingResolver, plans planapp.TaskAssessmentResolver, planCommands planapp.PlanCommandService, intake evaluationintake.Service, reportStatus *reportstatus.Reporter) Service {
	return &service{scoring: scoring, binding: binding, plans: plans, planCommands: planCommands, intake: intake, reportStatus: reportStatus}
}

func (s *service) Ensure(ctx context.Context, command Command) (*Result, error) {
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
	dto := evaluationintake.CreateCommand{OrgID: command.OrgID, TesteeID: command.TesteeID, QuestionnaireCode: command.QuestionnaireCode, QuestionnaireVersion: command.QuestionnaireVersion, AnswerSheetID: command.AnswerSheetID, OriginType: command.OriginType}
	if dto.OriginType == "" {
		dto.OriginType = "adhoc"
	}
	if command.OriginID != "" {
		dto.OriginID = &command.OriginID
	}
	bound, err := s.applyBinding(ctx, command, &dto)
	if err != nil {
		return nil, err
	}
	matched := s.matchPlan(ctx, command, dto.ModelCode)
	if matched != nil {
		dto.OriginType = "plan"
		dto.OriginID = &matched.PlanID
	}
	if existing, findErr := s.intake.FindByAnswerSheetID(ctx, command.AnswerSheetID); findErr == nil && existing != nil {
		s.completePlanBestEffort(ctx, command.OrgID, matched, existing.ID)
		return &Result{AssessmentID: existing.ID}, nil
	}
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
	result := &Result{AssessmentID: created.ID, Created: true}
	if bound {
		if _, submitErr := s.intake.SubmitForEvaluation(ctx, created.ID); submitErr == nil {
			result.AutoSubmitted = true
		}
	}
	s.completePlanBestEffort(ctx, command.OrgID, matched, created.ID)
	if s.reportStatus != nil {
		s.reportStatus.SetQueued(ctx, reportstatus.AssessmentKey(created.ID), reportstatus.AssessmentKey(command.AnswerSheetID))
	}
	return result, nil
}

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
