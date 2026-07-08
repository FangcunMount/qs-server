package characterization_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strconv"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	assessmentapp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcodec"
	workerhandlers "github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type charCrossModuleConfig struct {
	v1SplitPhaseConfig

	Assessment *assessment.Assessment
}

type charAnswerSheetCrossModuleConfig struct {
	AnswerSheetID uint64
	Async         bool
}

type charScaleBinding struct {
	scaleCode    string
	scaleVersion string
}

type charCrossModuleHarness struct {
	repo               *charAssessmentRepo
	assessment         *assessment.Assessment
	submitSvc          assessmentapp.AssessmentSubmissionService
	executeSvc         evaluationexecute.Service
	reportSaver        *charSplitPhaseReportSaver
	submitStaged       *[]event.DomainEvent
	evaluateStaged     []event.DomainEvent
	answersheetHandler workerhandlers.HandlerFunc
	submittedHandler   workerhandlers.HandlerFunc
	evaluatedHandler   workerhandlers.HandlerFunc
	scaleBinding       charScaleBinding
}

func buildCharCrossModuleHarness(t *testing.T, cfg charCrossModuleConfig) *charCrossModuleHarness {
	t.Helper()
	return buildCharCrossModuleHarnessCore(t, cfg.Assessment, cfg.Async, cfg.v1SplitPhaseConfig)
}

func buildCharAnswerSheetCrossModuleHarness(t *testing.T, cfg charAnswerSheetCrossModuleConfig) *charCrossModuleHarness {
	t.Helper()
	splitCfg := v1SplitPhaseConfig{
		Input: scaleInputSnapshot(),
		ReportBuilder: interpretationreporting.NewFactorScoringReportBuilder(
			domainreport.NewDefaultInterpretReportBuilder(nil),
		),
		Async: cfg.Async,
	}
	return buildCharCrossModuleHarnessCore(t, nil, cfg.Async, splitCfg)
}

func buildCharCrossModuleHarnessCore(
	t *testing.T,
	initialAssessment *assessment.Assessment,
	async bool,
	splitCfg v1SplitPhaseConfig,
) *charCrossModuleHarness {
	t.Helper()

	repo := &charAssessmentRepo{assessment: initialAssessment}
	submitStaged := &[]event.DomainEvent{}
	submitStager := &charEventCaptureStager{events: submitStaged}
	h := &charCrossModuleHarness{
		repo:         repo,
		assessment:   initialAssessment,
		reportSaver:  &charSplitPhaseReportSaver{},
		submitStaged: submitStaged,
		scaleBinding: charScaleBinding{scaleCode: "S-001", scaleVersion: "1.0.0"},
	}

	splitCfg.Assessment = initialAssessment
	if async {
		splitCfg.Async = true
		splitCfg.SnapshotStore = outcomescoring.NewMemorySnapshotStore()
		splitCfg.StageEvaluated = func(_ context.Context, events ...event.DomainEvent) error {
			h.evaluateStaged = append(h.evaluateStaged, events...)
			return nil
		}
	}

	executeSvc, reportSaver := buildV1SplitPhaseExecuteService(t, splitCfg, repo)
	h.executeSvc = executeSvc
	h.reportSaver = reportSaver

	h.submitSvc = assessmentapp.NewSubmissionService(
		repo,
		nil,
		assessment.NewSimpleAssessmentCreator(),
		&charTxRunner{},
		submitStager,
		nil,
	)

	bridge := &charBridgeInternalClient{
		execute:      executeSvc,
		repo:         repo,
		submitSvc:    h.submitSvc,
		scaleBinding: h.scaleBinding,
	}
	deps := &workerhandlers.Dependencies{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		InternalClient: bridge,
	}
	registry := workerhandlers.NewRegistry()
	h.answersheetHandler, _ = registry.Create("answersheet_submitted_handler", deps)
	h.submittedHandler, _ = registry.Create("assessment_submitted_handler", deps)
	h.evaluatedHandler, _ = registry.Create("assessment_evaluated_handler", deps)
	return h
}

func (h *charCrossModuleHarness) syncAssessmentFromRepo() {
	if h.repo != nil {
		h.assessment = h.repo.assessment
	}
}

func (h *charCrossModuleHarness) submitAssessment(t *testing.T, ctx context.Context) {
	t.Helper()
	if _, err := h.submitSvc.Submit(ctx, h.assessment.ID().Uint64()); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	h.syncAssessmentFromRepo()
	if !h.assessment.Status().IsSubmitted() {
		t.Fatalf("assessment status after submit = %s, want submitted", h.assessment.Status())
	}
}

func (h *charCrossModuleHarness) runAnswerSheetSubmittedWorker(t *testing.T, ctx context.Context, answerSheetID uint64) {
	t.Helper()
	payload := buildAnswerSheetSubmittedPayload(t, answerSheetID)
	if err := h.answersheetHandler(ctx, eventcatalog.AnswerSheetSubmitted, payload); err != nil {
		t.Fatalf("answersheet_submitted_handler: %v", err)
	}
	h.syncAssessmentFromRepo()
	if h.assessment == nil {
		t.Fatal("expected assessment to be created from answersheet")
	}
	if !h.assessment.Status().IsSubmitted() {
		t.Fatalf("assessment status after answersheet worker = %s, want submitted", h.assessment.Status())
	}
}

func (h *charCrossModuleHarness) runSubmittedWorker(t *testing.T, ctx context.Context) {
	t.Helper()
	payload := encodeFirstStagedEvent(t, *h.submitStaged, eventcatalog.AssessmentSubmitted)
	if err := h.submittedHandler(ctx, eventcatalog.AssessmentSubmitted, payload); err != nil {
		t.Fatalf("assessment_submitted_handler: %v", err)
	}
	h.syncAssessmentFromRepo()
}

func (h *charCrossModuleHarness) runEvaluatedWorker(t *testing.T, ctx context.Context) {
	t.Helper()
	payload := encodeFirstStagedEvent(t, h.evaluateStaged, eventcatalog.AssessmentEvaluated)
	if err := h.evaluatedHandler(ctx, eventcatalog.AssessmentEvaluated, payload); err != nil {
		t.Fatalf("assessment_evaluated_handler: %v", err)
	}
	h.syncAssessmentFromRepo()
}

type charEventCaptureStager struct {
	events *[]event.DomainEvent
}

func (s *charEventCaptureStager) Stage(_ context.Context, events ...event.DomainEvent) error {
	*s.events = append(*s.events, events...)
	return nil
}

func hasStagedEvent(staged []event.DomainEvent, eventType string) bool {
	for _, evt := range staged {
		if evt.EventType() == eventType {
			return true
		}
	}
	return false
}

func encodeFirstStagedEvent(t *testing.T, staged []event.DomainEvent, eventType string) []byte {
	t.Helper()
	for _, evt := range staged {
		if evt.EventType() == eventType {
			payload, err := eventcodec.EncodeDomainEvent(evt)
			if err != nil {
				t.Fatalf("EncodeDomainEvent(%s): %v", eventType, err)
			}
			return payload
		}
	}
	t.Fatalf("staged event %q not found in %#v", eventType, staged)
	return nil
}

func buildAnswerSheetSubmittedPayload(t *testing.T, answerSheetID uint64) []byte {
	t.Helper()

	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	answerSheetIDStr := strconv.FormatUint(answerSheetID, 10)
	payload, err := json.Marshal(map[string]any{
		"id":            "evt-answersheet-cross-module",
		"eventType":     eventcatalog.AnswerSheetSubmitted,
		"occurredAt":    now,
		"aggregateType": "AnswerSheet",
		"aggregateID":   answerSheetIDStr,
		"data": map[string]any{
			"answersheet_id":        answerSheetIDStr,
			"questionnaire_code":    "Q-001",
			"questionnaire_version": "1.0.0",
			"testee_id":             uint64(8001),
			"org_id":                int64(1),
			"filler_id":             uint64(8001),
			"filler_type":           "testee",
			"task_id":               "",
			"submitted_at":          now,
		},
	})
	if err != nil {
		t.Fatalf("marshal answersheet payload: %v", err)
	}
	return payload
}

type charBridgeInternalClient struct {
	execute      evaluationexecute.Service
	repo         *charAssessmentRepo
	submitSvc    assessmentapp.AssessmentSubmissionService
	scaleBinding charScaleBinding
}

var _ workerhandlers.InternalClient = (*charBridgeInternalClient)(nil)

func (b *charBridgeInternalClient) EvaluateAssessment(ctx context.Context, assessmentID uint64) (*pb.EvaluateAssessmentResponse, error) {
	if err := b.execute.Evaluate(ctx, assessmentID); err != nil {
		return &pb.EvaluateAssessmentResponse{
			Success: false,
			Status:  "failed",
			Message: err.Error(),
		}, nil
	}
	return b.evaluateResponse(ctx, assessmentID), nil
}

func (b *charBridgeInternalClient) GenerateReportFromAssessment(ctx context.Context, assessmentID uint64) (*pb.GenerateReportFromAssessmentResponse, error) {
	if err := b.execute.GenerateReport(ctx, assessmentID); err != nil {
		return &pb.GenerateReportFromAssessmentResponse{
			Success: false,
			Status:  "failed",
			Message: err.Error(),
		}, nil
	}
	return &pb.GenerateReportFromAssessmentResponse{
		Success: true,
		Status:  "interpreted",
		Message: "报告生成完成",
	}, nil
}

func (b *charBridgeInternalClient) evaluateResponse(ctx context.Context, assessmentID uint64) *pb.EvaluateAssessmentResponse {
	a, _ := b.repo.FindByID(ctx, assessment.NewID(assessmentID))
	status := "interpreted"
	if a != nil && a.Status().IsEvaluated() {
		status = "evaluated"
	}
	resp := &pb.EvaluateAssessmentResponse{
		Success: true,
		Status:  status,
		Message: "评估完成",
	}
	if a != nil {
		resp.Outcome = charEvaluateOutcomeSummary(a)
	}
	return resp
}

func charEvaluateOutcomeSummary(a *assessment.Assessment) *pb.OutcomeSummary {
	if a == nil {
		return nil
	}
	outcome := &pb.OutcomeSummary{}
	if score := a.TotalScore(); score != nil {
		outcome.PrimaryScore = &pb.ScoreValue{
			Kind:  domainreport.ScoreKindRawTotal,
			Value: *score,
		}
	}
	if risk := a.RiskLevel(); risk != nil && *risk != "" {
		if lv := domainreport.LevelFromRisk(domainreport.RiskLevel(*risk)); lv != nil {
			outcome.Level = &pb.ResultLevel{
				Code:     lv.Code,
				Label:    lv.Label,
				Severity: lv.Severity,
			}
		}
	}
	return outcome
}

func (b *charBridgeInternalClient) CreateAssessmentFromAnswerSheet(
	ctx context.Context,
	req *pb.CreateAssessmentFromAnswerSheetRequest,
) (*pb.CreateAssessmentFromAnswerSheetResponse, error) {
	if req == nil || req.AnswersheetId == 0 {
		return &pb.CreateAssessmentFromAnswerSheetResponse{
			Success: false,
			Message: "invalid create assessment request",
		}, nil
	}
	scaleCode := b.scaleBinding.scaleCode
	scaleVersion := b.scaleBinding.scaleVersion
	modelKind := "scale"
	dto := assessmentapp.CreateAssessmentDTO{
		OrgID:                req.OrgId,
		TesteeID:             req.TesteeId,
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
		AnswerSheetID:        req.AnswersheetId,
		MedicalScaleCode:     &scaleCode,
		ModelKind:            &modelKind,
		ModelCode:            &scaleCode,
		ModelVersion:         &scaleVersion,
		OriginType:           "adhoc",
	}
	if req.OriginType != "" {
		dto.OriginType = req.OriginType
	}

	result, err := b.submitSvc.Create(ctx, dto)
	if err != nil {
		return &pb.CreateAssessmentFromAnswerSheetResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	autoSubmitted := false
	if dto.MedicalScaleCode != nil || dto.ModelCode != nil {
		if _, err := b.submitSvc.Submit(ctx, result.ID); err != nil {
			return &pb.CreateAssessmentFromAnswerSheetResponse{
				Success: false,
				Message: err.Error(),
			}, nil
		}
		autoSubmitted = true
	}

	return &pb.CreateAssessmentFromAnswerSheetResponse{
		Success:       true,
		AssessmentId:  result.ID,
		Created:       true,
		AutoSubmitted: autoSubmitted,
		Message:       "ok",
	}, nil
}

func (b *charBridgeInternalClient) CalculateAnswerSheetScore(
	_ context.Context,
	req *pb.CalculateAnswerSheetScoreRequest,
) (*pb.CalculateAnswerSheetScoreResponse, error) {
	if req == nil || req.AnswersheetId == 0 {
		return &pb.CalculateAnswerSheetScoreResponse{
			Success: false,
			Message: "answersheet_id is required",
		}, nil
	}
	return &pb.CalculateAnswerSheetScoreResponse{
		Success:    true,
		Message:    "ok",
		TotalScore: 5,
	}, nil
}

func (b *charBridgeInternalClient) SyncAssessmentAttention(
	_ context.Context,
	_ *pb.SyncAssessmentAttentionRequest,
) (*pb.SyncAssessmentAttentionResponse, error) {
	return &pb.SyncAssessmentAttentionResponse{}, nil
}

func (b *charBridgeInternalClient) GenerateQuestionnaireQRCode(
	_ context.Context,
	_, _ string,
) (*pb.GenerateQuestionnaireQRCodeResponse, error) {
	return &pb.GenerateQuestionnaireQRCodeResponse{}, nil
}

func (b *charBridgeInternalClient) GenerateScaleQRCode(
	_ context.Context,
	_ string,
) (*pb.GenerateScaleQRCodeResponse, error) {
	return &pb.GenerateScaleQRCodeResponse{}, nil
}

func (b *charBridgeInternalClient) HandleQuestionnairePublishedPostActions(
	_ context.Context,
	_, _ string,
) (*pb.GenerateQuestionnaireQRCodeResponse, error) {
	return &pb.GenerateQuestionnaireQRCodeResponse{}, nil
}

func (b *charBridgeInternalClient) HandleScalePublishedPostActions(
	_ context.Context,
	_ string,
) (*pb.GenerateScaleQRCodeResponse, error) {
	return &pb.GenerateScaleQRCodeResponse{}, nil
}

func (b *charBridgeInternalClient) ProjectBehaviorEvent(
	_ context.Context,
	_ *pb.ProjectBehaviorEventRequest,
) (*pb.ProjectBehaviorEventResponse, error) {
	return &pb.ProjectBehaviorEventResponse{}, nil
}

func (b *charBridgeInternalClient) SendTaskOpenedMiniProgramNotification(
	_ context.Context,
	_ int64,
	_ string,
	_ uint64,
	_ string,
	_ time.Time,
) (*pb.SendTaskOpenedMiniProgramNotificationResponse, error) {
	return &pb.SendTaskOpenedMiniProgramNotificationResponse{}, nil
}

func scaleCrossModuleConfig(t *testing.T, a *assessment.Assessment, async bool) charCrossModuleConfig {
	t.Helper()
	return charCrossModuleConfig{
		Assessment: a,
		v1SplitPhaseConfig: v1SplitPhaseConfig{
			Assessment: a,
			Input:      scaleInputSnapshot(),
			ReportBuilder: interpretationreporting.NewFactorScoringReportBuilder(
				domainreport.NewDefaultInterpretReportBuilder(nil),
			),
			Async: async,
		},
	}
}
