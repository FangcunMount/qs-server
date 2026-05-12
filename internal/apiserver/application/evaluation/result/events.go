package result

import (
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// EventAssembler 事件装配器。
type EventAssembler interface {
	Kind() assessment.EvaluationModelKind
	BuildSuccessEvents(outcome Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent
}

type EventAssemblerRegistry interface {
	Resolve(kind assessment.EvaluationModelKind) EventAssembler
}

type mutableEventAssemblerRegistry struct {
	items map[assessment.EvaluationModelKind]EventAssembler
}

func NewEventAssemblerRegistry(assemblers ...EventAssembler) (*mutableEventAssemblerRegistry, error) {
	registry := &mutableEventAssemblerRegistry{items: make(map[assessment.EvaluationModelKind]EventAssembler)}
	for _, assembler := range assemblers {
		if err := registry.Register(assembler); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *mutableEventAssemblerRegistry) Register(assembler EventAssembler) error {
	if assembler == nil {
		return fmt.Errorf("evaluation event assembler is nil")
	}
	kind := assembler.Kind()
	if kind == "" {
		return fmt.Errorf("evaluation event assembler kind is empty")
	}
	if _, exists := r.items[kind]; exists {
		return fmt.Errorf("evaluation event assembler already registered for kind %s", kind)
	}
	r.items[kind] = assembler
	return nil
}

func (r *mutableEventAssemblerRegistry) Resolve(kind assessment.EvaluationModelKind) EventAssembler {
	if r == nil {
		return GenericEventAssembler{}
	}
	if assembler, ok := r.items[kind]; ok {
		return assembler
	}
	return GenericEventAssembler{}
}

type GenericEventAssembler struct{}

func (GenericEventAssembler) Kind() assessment.EvaluationModelKind {
	return assessment.EvaluationModelKindMBTI
}

func (GenericEventAssembler) BuildSuccessEvents(outcome Outcome, _ *domainReport.InterpretReport) []event.DomainEvent {
	if outcome.Assessment == nil || outcome.Result == nil {
		return nil
	}
	modelRef := outcome.Result.ModelRef
	if modelRef.IsEmpty() && outcome.Assessment.EvaluationModelRef() != nil {
		modelRef = *outcome.Assessment.EvaluationModelRef()
	}
	if modelRef.IsEmpty() {
		return nil
	}
	return []event.DomainEvent{
		assessment.NewAssessmentModelInterpretedEvent(
			outcome.Assessment.OrgID(),
			outcome.Assessment.ID(),
			outcome.Assessment.TesteeID(),
			modelRef,
			outcome.Result.TotalScore,
			outcome.Result.RiskLevel,
			time.Now(),
		),
	}
}

type ScaleEventAssembler struct{}

func (ScaleEventAssembler) Kind() assessment.EvaluationModelKind {
	return assessment.EvaluationModelKindScale
}

// BuildSuccessEvents 构建 Scale 成功事件，保留旧 report/footprint 兼容事件。
func (ScaleEventAssembler) BuildSuccessEvents(outcome Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent {
	if outcome.Assessment == nil || outcome.Result == nil || rpt == nil {
		return nil
	}
	now := time.Now()
	assessmentRef := outcome.Assessment.MedicalScaleRef()
	if assessmentRef == nil {
		return GenericEventAssembler{}.BuildSuccessEvents(outcome, rpt)
	}
	modelRef := outcome.Assessment.EvaluationModelRef()
	if modelRef == nil {
		ref := assessmentRef.ToEvaluationModelRef()
		modelRef = &ref
	}

	scaleVersion := modelRef.Version()
	if scaleVersion == "" && outcome.Input != nil && outcome.Input.Model != nil {
		scaleVersion = outcome.Input.Model.Version
	}
	if scaleVersion == "" && outcome.Input != nil && outcome.Input.MedicalScale != nil {
		scaleVersion = outcome.Input.MedicalScale.ScaleVersion
	}
	if scaleVersion == "" && !outcome.Assessment.QuestionnaireRef().IsEmpty() {
		scaleVersion = outcome.Assessment.QuestionnaireRef().Version()
	}

	scaleRef := assessment.NewMedicalScaleRefWithVersion(
		assessmentRef.ID(),
		assessmentRef.Code(),
		assessmentRef.Name(),
		scaleVersion,
	)

	assessmentID := outcome.Assessment.ID().Uint64()
	reportID := rpt.ID().Uint64()
	testeeID := outcome.Assessment.TesteeID().Uint64()

	return []event.DomainEvent{
		assessment.NewAssessmentInterpretedEvent(
			outcome.Assessment.OrgID(),
			outcome.Assessment.ID(),
			outcome.Assessment.TesteeID(),
			*modelRef,
			scaleRef,
			outcome.Result.TotalScore,
			outcome.Result.RiskLevel,
			now,
		),
		domainReport.NewReportGeneratedEvent(
			strconv.FormatUint(reportID, 10),
			strconv.FormatUint(assessmentID, 10),
			testeeID,
			rpt.ScaleCode(),
			scaleVersion,
			rpt.TotalScore(),
			string(rpt.RiskLevel()),
			now,
		),
		domainStatistics.NewFootprintReportGeneratedEvent(
			outcome.Assessment.OrgID(),
			testeeID,
			assessmentID,
			reportID,
			now,
		),
	}
}
