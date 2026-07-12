package grpcclient

import (
	"context"
	"fmt"
	"time"

	evalpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// InternalClient 内部服务客户端
// 用于 Worker 调用 APIServer 的内部接口
type InternalClient struct {
	manager    *Manager
	client     pb.InternalServiceClient
	intake     evalpb.AssessmentIntakeServiceClient
	evaluation evalpb.EvaluationWorkerServiceClient
	automation pb.InterpretationAutomationServiceClient
}

// NewInternalClient 创建内部服务客户端
func NewInternalClient(manager *Manager) *InternalClient {
	return &InternalClient{
		manager:    manager,
		client:     pb.NewInternalServiceClient(manager.Conn()),
		intake:     evalpb.NewAssessmentIntakeServiceClient(manager.Conn()),
		evaluation: evalpb.NewEvaluationWorkerServiceClient(manager.Conn()),
		automation: pb.NewInterpretationAutomationServiceClient(manager.Conn()),
	}
}

// CreateAssessmentFromAnswerSheet 从答卷创建测评
func (c *InternalClient) CreateAssessmentFromAnswerSheet(
	ctx context.Context,
	req *pb.CreateAssessmentFromAnswerSheetRequest,
) (*pb.CreateAssessmentFromAnswerSheetResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.intake.EnsureAssessment(ctx, &evalpb.EnsureAssessmentRequest{OrgId: req.OrgId, AnswerSheetId: req.AnswersheetId, QuestionnaireCode: req.QuestionnaireCode, QuestionnaireVersion: req.QuestionnaireVersion, TesteeId: req.TesteeId, FillerId: req.FillerId, TaskId: req.TaskId, OriginType: req.OriginType, OriginId: req.OriginId})
	if err != nil {
		return nil, fmt.Errorf("failed to create assessment from answersheet: %w", err)
	}

	return &pb.CreateAssessmentFromAnswerSheetResponse{Success: true, AssessmentId: resp.GetAssessmentId(), Created: resp.GetCreated(), AutoSubmitted: resp.GetAutoSubmitted(), Message: "assessment ensured"}, nil
}

// EvaluateAssessment 执行测评评估
func (c *InternalClient) EvaluateAssessment(
	ctx context.Context,
	assessmentID uint64,
) (*pb.EvaluateAssessmentResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.evaluation.ExecuteEvaluation(ctx, &evalpb.ExecuteEvaluationRequest{
		AssessmentId: assessmentID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate assessment: %w", err)
	}

	out := &pb.EvaluateAssessmentResponse{Success: resp.GetStatus() == "evaluated", Status: resp.GetStatus(), Message: resp.GetFailureMessage(), Retryable: resp.GetRetryable(), RunId: resp.GetRunId(), FailureKind: resp.GetFailureKind(), TraceId: resp.GetTraceId(), InputSnapshotRef: resp.GetInputSnapshotRef()}
	if resp.GetModel() != nil || resp.GetPrimaryScore() != nil || resp.GetLevel() != nil {
		out.Outcome = &pb.OutcomeSummary{}
		if v := resp.GetModel(); v != nil {
			out.Outcome.Model = &pb.ModelIdentity{Kind: v.GetKind(), SubKind: v.GetSubKind(), Algorithm: v.GetAlgorithm(), Code: v.GetCode(), Version: v.GetVersion(), Title: v.GetTitle()}
		}
		if v := resp.GetPrimaryScore(); v != nil {
			out.Outcome.PrimaryScore = &pb.ScoreValue{Kind: v.GetKind(), Value: v.GetValue(), Label: v.GetLabel(), Max: v.Max}
		}
		if v := resp.GetLevel(); v != nil {
			out.Outcome.Level = &pb.ResultLevel{Code: v.GetCode(), Label: v.GetLabel(), Severity: v.GetSeverity()}
		}
	}
	return out, nil
}

// GenerateReportFromOutcome 通过 Interpretation 用例消费持久化的评估结果并生成报告。
func (c *InternalClient) GenerateReportFromOutcome(
	ctx context.Context,
	outcomeID string,
) (*pb.GenerateReportFromAssessmentResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.automation.GenerateReportFromAssessment(ctx, &pb.GenerateReportFromAssessmentRequest{
		OutcomeId: outcomeID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate report from assessment: %w", err)
	}
	return resp, nil
}

// SyncAssessmentAttention 同步测评后置关注状态。
func (c *InternalClient) SyncAssessmentAttention(
	ctx context.Context,
	req *pb.SyncAssessmentAttentionRequest,
) (*pb.SyncAssessmentAttentionResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.SyncAssessmentAttention(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to sync assessment attention: %w", err)
	}

	return resp, nil
}

// GenerateQuestionnaireQRCode 生成问卷小程序码
func (c *InternalClient) GenerateQuestionnaireQRCode(
	ctx context.Context,
	code, version string,
) (*pb.GenerateQuestionnaireQRCodeResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.GenerateQuestionnaireQRCode(ctx, &pb.GenerateQuestionnaireQRCodeRequest{
		Code:    code,
		Version: version,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate questionnaire QR code: %w", err)
	}

	return resp, nil
}

func (c *InternalClient) HandleQuestionnairePublishedPostActions(
	ctx context.Context,
	code, version string,
) (*pb.GenerateQuestionnaireQRCodeResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.HandleQuestionnairePublishedPostActions(ctx, &pb.GenerateQuestionnaireQRCodeRequest{
		Code:    code,
		Version: version,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to handle questionnaire publish post-actions: %w", err)
	}
	return resp, nil
}

// GenerateScaleQRCode 生成量表小程序码
func (c *InternalClient) GenerateScaleQRCode(
	ctx context.Context,
	code string,
) (*pb.GenerateScaleQRCodeResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.GenerateScaleQRCode(ctx, &pb.GenerateScaleQRCodeRequest{
		Code: code,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate scale QR code: %w", err)
	}

	return resp, nil
}

func (c *InternalClient) HandleScalePublishedPostActions(
	ctx context.Context,
	code string,
) (*pb.GenerateScaleQRCodeResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.HandleScalePublishedPostActions(ctx, &pb.GenerateScaleQRCodeRequest{
		Code: code,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to handle scale publish post-actions: %w", err)
	}
	return resp, nil
}

func (c *InternalClient) ProjectBehaviorEvent(
	ctx context.Context,
	req *pb.ProjectBehaviorEventRequest,
) (*pb.ProjectBehaviorEventResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.ProjectBehaviorEvent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to project behavior event: %w", err)
	}

	return resp, nil
}

// SendTaskOpenedMiniProgramNotification 发送 task.opened 小程序订阅消息。
func (c *InternalClient) SendTaskOpenedMiniProgramNotification(
	ctx context.Context,
	orgID int64,
	taskID string,
	testeeID uint64,
	entryURL string,
	openAt time.Time,
) (*pb.SendTaskOpenedMiniProgramNotificationResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.SendTaskOpenedMiniProgramNotification(ctx, &pb.SendTaskOpenedMiniProgramNotificationRequest{
		OrgId:    orgID,
		TaskId:   taskID,
		TesteeId: testeeID,
		EntryUrl: entryURL,
		OpenAt:   timestamppb.New(openAt),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send task opened mini program notification: %w", err)
	}

	return resp, nil
}
