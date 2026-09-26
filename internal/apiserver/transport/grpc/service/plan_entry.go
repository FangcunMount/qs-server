package service

import (
	"context"

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
	return &pb.ResolveTaskEntryResponse{
		TaskId:    entry.TaskID,
		TesteeId:  entry.TesteeID,
		ScaleCode: entry.ScaleCode,
		ExpiresAt: entry.ExpireAt.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}
