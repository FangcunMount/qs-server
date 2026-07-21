// Package handlers provides worker event handler factories through an explicit
// Registry constructed at the process composition boundary.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/FangcunMount/component-base/pkg/eventcodec"
	evalpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/attentionprojection"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/worker/port"
)

var ErrAutomaticRetryPaused = eventruntime.ErrAutomaticRetryPaused

// HandlerFunc 处理器函数类型
type HandlerFunc func(ctx context.Context, eventType string, payload []byte) error

// InternalClient 抽象 Worker 侧已使用的内部 gRPC 能力，便于 handler 级测试替换。
type InternalClient interface {
	SyncAssessmentAttention(
		ctx context.Context,
		req *pb.SyncAssessmentAttentionRequest,
	) (*pb.SyncAssessmentAttentionResponse, error)
	GenerateQuestionnaireQRCode(
		ctx context.Context,
		code, version string,
	) (*pb.GenerateQuestionnaireQRCodeResponse, error)
	GenerateScaleQRCode(ctx context.Context, code string) (*pb.GenerateScaleQRCodeResponse, error)
	HandleQuestionnairePublishedPostActions(
		ctx context.Context,
		code, version string,
	) (*pb.GenerateQuestionnaireQRCodeResponse, error)
	HandleScalePublishedPostActions(ctx context.Context, code string) (*pb.GenerateScaleQRCodeResponse, error)
	SendTaskOpenedMiniProgramNotification(
		ctx context.Context,
		orgID int64,
		taskID string,
		testeeID uint64,
		entryURL string,
		openAt time.Time,
	) (*pb.SendTaskOpenedMiniProgramNotificationResponse, error)
}
type AssessmentIntakeClient interface {
	EnsureAssessment(context.Context, *evalpb.EnsureAssessmentRequest) (*evalpb.EnsureAssessmentResponse, error)
}
type EvaluationWorkerClient interface {
	ExecuteEvaluation(context.Context, uint64) (*evalpb.ExecuteEvaluationResponse, error)
}
type InterpretationAutomationClient interface {
	GenerateReportFromOutcome(context.Context, string) (*interpretationpb.GenerateReportFromAssessmentResponse, error)
}

// ReportStatusWriter projects report lifecycle states for client polling.
// Its Redis-backed implementation is supplied by the worker composition root.
type ReportStatusWriter interface {
	SetProcessing(ctx context.Context, assessmentID, answerSheetID, stage string)
	SetCompleted(ctx context.Context, assessmentID, answerSheetID, reportID string)
	SetFailed(ctx context.Context, assessmentID, answerSheetID, reason, message string)
	SetTemporarilyUnavailable(ctx context.Context, assessmentID, answerSheetID, reason, message string)
}

// Dependencies 处理器依赖
type Dependencies struct {
	Logger                         *slog.Logger
	AnswerSheetClient              *grpcclient.AnswerSheetClient
	InternalClient                 InternalClient
	AssessmentIntakeClient         AssessmentIntakeClient
	EvaluationWorkerClient         EvaluationWorkerClient
	InterpretationAutomationClient InterpretationAutomationClient
	LockManager                    locklease.Manager
	LockRunner                     locklease.Runner
	LockKeyBuilder                 *keyspace.Builder
	Notifier                       port.TaskNotifier
	ReportStatusReporter           ReportStatusWriter
	AttentionProjector             *attentionprojection.Projector
	DisableAutomaticRetry          bool
}

// HandlerFactory 处理器工厂函数
// 接收依赖，返回处理器函数
type HandlerFactory func(deps *Dependencies) HandlerFunc

// Registry is an explicit, immutable handler factory catalog.
type Registry struct {
	factories map[string]HandlerFactory
}

func newRegistryFromFactories(factories map[string]HandlerFactory) *Registry {
	copied := make(map[string]HandlerFactory, len(factories))
	for name, factory := range factories {
		if factory == nil {
			continue
		}
		copied[name] = factory
	}
	return &Registry{factories: copied}
}

// Names returns registered handler names in deterministic order.
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Has reports whether the registry contains a handler factory.
func (r *Registry) Has(name string) bool {
	if r == nil {
		return false
	}
	_, ok := r.factories[name]
	return ok
}

// Create instantiates one handler by name.
func (r *Registry) Create(name string, deps *Dependencies) (HandlerFunc, bool) {
	if r == nil {
		return nil, false
	}
	factory, ok := r.factories[name]
	if !ok {
		return nil, false
	}
	return factory(deps), true
}

// ==================== 事件消息解析 ====================

// EventEnvelope 事件信封结构。
type EventEnvelope = eventcodec.Envelope

// ParseEventEnvelope 解析事件信封
func ParseEventEnvelope(payload []byte) (*EventEnvelope, error) {
	return eventcodec.DecodeEnvelope(payload)
}

// ParseEventData 解析事件业务数据到指定类型
// 用法: var data MyPayload; ParseEventData(payload, &data)
func ParseEventData[T any](payload []byte, target *T) (*EventEnvelope, error) {
	env, err := ParseEventEnvelope(payload)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(env.Data, target); err != nil {
		return nil, fmt.Errorf("failed to parse event data: %w", err)
	}

	return env, nil
}
