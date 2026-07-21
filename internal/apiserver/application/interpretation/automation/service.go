package automation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc/metadata"

	interpretationexecution "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	interpretationinput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/admission"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

type Actor struct {
	Source string
}

func TrustedServiceActor(source string) Actor { return Actor{Source: source} }

type GenerateCommand struct {
	Actor     Actor
	OutcomeID meta.ID
	TraceID   string
}

type Status string

const (
	StatusGenerated         Status = "generated"
	StatusProcessing        Status = "processing"
	StatusBlocked           Status = "blocked"
	StatusAdmissionRejected Status = "admission_rejected"
)

type Result struct {
	Status        Status
	GenerationID  meta.ID
	RunID         meta.ID
	ReportID      meta.ID
	AttemptOrigin retrygovernance.AttemptOrigin
	RetryDecision *retrygovernance.Decision
}

type Service interface {
	Generate(ctx context.Context, command GenerateCommand) (*Result, error)
}

type service struct {
	outcomes  evaluationfact.Repository
	executor  interpretationexecution.Executor
	admission admission.Repository
	now       func() time.Time
	newID     func() meta.ID
}

func NewService(outcomes evaluationfact.Repository, executor interpretationexecution.Executor, admissionRepo ...admission.Repository) (Service, error) {
	if outcomes == nil || executor == nil {
		return nil, fmt.Errorf("interpretation automation dependencies are required")
	}
	var repo admission.Repository
	if len(admissionRepo) > 0 {
		repo = admissionRepo[0]
	}
	return &service{
		outcomes:  outcomes,
		executor:  executor,
		admission: repo,
		now:       time.Now,
		newID:     meta.New,
	}, nil
}

func (s *service) Generate(ctx context.Context, command GenerateCommand) (*Result, error) {
	if command.Actor.Source == "" {
		return nil, fmt.Errorf("trusted automation actor is required")
	}
	if command.OutcomeID.IsZero() {
		return nil, fmt.Errorf("evaluation outcome id is required")
	}
	record, err := s.outcomes.FindByID(ctx, command.OutcomeID)
	if err != nil {
		return nil, s.rejectAdmission(ctx, command, nil, classifyOutcomeLookupError(err), err)
	}
	input, err := interpretationinput.FromOutcomeRecord(record)
	if err != nil {
		return nil, s.rejectAdmission(ctx, command, record, classifyInputError(err), err)
	}
	executed, err := interpretationexecution.ExecuteOutcome(ctx, s.executor, record, input, command.TraceID)
	if err != nil {
		return nil, err
	}
	result := &Result{}
	switch executed.Status {
	case interpretationexecution.ExecuteStatusProcessing:
		result.Status = StatusProcessing
	case interpretationexecution.ExecuteStatusBlocked:
		result.Status = StatusBlocked
	default:
		result.Status = StatusGenerated
	}
	if executed.Generation != nil {
		result.GenerationID = executed.Generation.ID()
	}
	if executed.Run != nil {
		result.RunID = executed.Run.ID()
		result.AttemptOrigin = executed.Run.Origin()
		result.RetryDecision = executed.Run.RetryDecision()
	}
	if executed.InterpretReport != nil {
		result.ReportID = executed.InterpretReport.ID()
		if result.RunID.IsZero() {
			result.RunID = executed.InterpretReport.InterpretationRunID()
		}
	}
	return result, nil
}

func (s *service) rejectAdmission(ctx context.Context, command GenerateCommand, record *evaluationfact.Record, kind admission.Kind, cause error) error {
	at := s.now()
	eventID := eventIDFromContext(ctx)
	code, message, retryable := admissionCodeMessage(kind, cause)
	input := admission.Input{
		ID: s.newID(), OutcomeID: command.OutcomeID, EventID: eventID, TraceID: command.TraceID,
		Kind: kind, Code: code, SafeMessage: message, Retryable: retryable, OccurredAt: at,
	}
	if record != nil {
		input.OrgID = record.OrgID()
		input.AssessmentID = record.AssessmentID()
		input.TesteeID = record.TesteeID()
		input.OutcomeID = record.ID()
	}
	failure, err := admission.NewFailure(input)
	if err != nil {
		return fmt.Errorf("build admission failure: %w", err)
	}
	if s.admission != nil {
		if _, err := s.admission.UpsertByFingerprint(ctx, failure); err != nil {
			return fmt.Errorf("persist admission failure: %w", err)
		}
	}
	return &admission.RejectedError{Failure: failure}
}

func classifyOutcomeLookupError(err error) admission.Kind {
	if err == nil {
		return admission.KindUnknown
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "not found") {
		return admission.KindOutcomeNotFound
	}
	return admission.KindUnknown
}

func classifyInputError(err error) admission.Kind {
	if errors.Is(err, modeltypology.ErrRuntimeSpecInvalid) {
		return admission.KindRuntimeSpecInvalid
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "unknown template"), strings.Contains(msg, "unknown_template"):
		return admission.KindRuntimeSpecInvalid
	case strings.Contains(msg, "report input"), strings.Contains(msg, "decode report"):
		return admission.KindReportInputDecode
	case strings.Contains(msg, "decode"), strings.Contains(msg, "payload"):
		return admission.KindPayloadDecode
	case strings.Contains(msg, "frozen"), strings.Contains(msg, "identity"):
		return admission.KindFrozenIdentity
	default:
		return admission.KindMapping
	}
}

func admissionCodeMessage(kind admission.Kind, cause error) (code, message string, retryable bool) {
	retryable = false
	switch kind {
	case admission.KindOutcomeNotFound:
		return "outcome_not_found", "评估结果不存在", false
	case admission.KindOutcomeUnauthorized:
		return "outcome_unauthorized", "评估结果未授权", false
	case admission.KindPayloadDecode:
		return "payload_decode", "评估结果载荷无法解码", false
	case admission.KindReportInputDecode:
		return "report_input_decode", "冻结报告输入无法解码", false
	case admission.KindFrozenIdentity:
		return "frozen_identity", "冻结模型身份不合法", false
	case admission.KindRuntimeSpecInvalid:
		return "runtime_spec_invalid", "报告路由配置非法", false
	case admission.KindMapping:
		return "mapping_failed", "评估结果无法映射为报告输入", false
	default:
		if cause != nil {
			return "admission_internal", "报告准入失败", true
		}
		return "admission_unknown", "报告准入失败", false
	}
}

func eventIDFromContext(ctx context.Context) string {
	values, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	items := values.Get("x-event-id")
	if len(items) == 0 || items[0] == "" {
		return ""
	}
	return items[0]
}

type Failure struct {
	GenerationID  meta.ID
	RunID         meta.ID
	Kind          run.FailureKind
	Code          string
	SafeMessage   string
	Retryable     bool
	AttemptOrigin retrygovernance.AttemptOrigin
	RetryDecision *retrygovernance.Decision
}

func FailureFrom(err error) (Failure, bool) {
	failed, ok := interpretationexecution.FailureFrom(err)
	if !ok {
		return Failure{}, false
	}
	return Failure{
		GenerationID:  failed.GenerationID,
		RunID:         failed.RunID,
		Kind:          failed.Failure.Kind,
		Code:          failed.Failure.Code,
		SafeMessage:   failed.Failure.SafeMessage,
		Retryable:     failed.Failure.Retryable,
		AttemptOrigin: failed.Origin,
		RetryDecision: failed.Decision,
	}, true
}
