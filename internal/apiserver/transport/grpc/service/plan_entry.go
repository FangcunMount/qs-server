package service

import (
	"context"
	"time"

	baseerrors "github.com/FangcunMount/component-base/pkg/errors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	planapp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PlanEntryService exposes the current task entry only over the internal
// authenticated gRPC plane. Collection must still authorize the IAM User
// against the returned Testee before presenting it to the caller.
type PlanEntryService struct {
	pb.UnimplementedPlanEntryServiceServer
	resolver planapp.TaskEntryResolver
}

func NewPlanEntryService(resolver planapp.TaskEntryResolver) *PlanEntryService {
	return &PlanEntryService{resolver: resolver}
}

func (s *PlanEntryService) RegisterService(server *grpc.Server) {
	pb.RegisterPlanEntryServiceServer(server, s)
}

func (s *PlanEntryService) ResolveTaskEntry(ctx context.Context, req *pb.ResolveTaskEntryRequest) (*pb.ResolveTaskEntryResponse, error) {
	if s == nil || s.resolver == nil || req == nil {
		return nil, status.Error(codes.Unavailable, "task entry resolver unavailable")
	}
	entry, err := s.resolver.ResolveTaskEntry(ctx, req.GetTaskId(), req.GetToken())
	if err != nil {
		if baseerrors.IsCode(err, code.ErrPageNotFound) {
			return nil, status.Error(codes.NotFound, "task entry not found")
		}
		return nil, status.Error(codes.Unavailable, "task entry lookup unavailable")
	}
	return taskEntryResponse(entry), nil
}

func taskEntryResponse(entry *planapp.ResolvedTaskEntry) *pb.ResolveTaskEntryResponse {
	format := func(at time.Time) string {
		if at.IsZero() {
			return ""
		}
		return at.Format(time.RFC3339)
	}
	return &pb.ResolveTaskEntryResponse{TaskId: entry.TaskID, TesteeId: entry.TesteeID, ScaleCode: entry.ScaleCode, ExpiresAt: format(entry.ExpireAt), PlanId: entry.PlanID, Title: entry.Title, OpenAt: format(entry.OpenAt), DueAt: format(entry.DueAt), TaskStatus: entry.Status, CanStart: entry.CanStart, QuestionnaireCode: entry.QuestionnaireCode, QuestionnaireVersion: entry.QuestionnaireVersion, ModelVersion: entry.ModelVersion}
}

func (s *PlanEntryService) ListParticipantTasks(ctx context.Context, req *pb.ListParticipantTasksRequest) (*pb.ListParticipantTasksResponse, error) {
	reader, ok := s.resolver.(interface {
		ListParticipantTasks(context.Context, string) ([]*planapp.ResolvedTaskEntry, error)
	})
	if !ok || req == nil {
		return nil, status.Error(codes.Unavailable, "participant task lookup unavailable")
	}
	entries, err := reader.ListParticipantTasks(ctx, req.GetTesteeId())
	if err != nil {
		return nil, status.Error(codes.Unavailable, "participant task lookup unavailable")
	}
	result := &pb.ListParticipantTasksResponse{}
	for _, entry := range entries {
		result.Items = append(result.Items, taskEntryResponse(entry))
	}
	return result, nil
}
