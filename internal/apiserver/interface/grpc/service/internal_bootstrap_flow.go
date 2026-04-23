package service

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	operatorApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

type operatorBootstrapFlow struct {
	service *InternalService
}

func newOperatorBootstrapFlow(service *InternalService) operatorBootstrapFlow {
	return operatorBootstrapFlow{service: service}
}

func (flow operatorBootstrapFlow) BootstrapOperator(
	ctx context.Context,
	req *pb.BootstrapOperatorRequest,
) (*pb.BootstrapOperatorResponse, error) {
	s := flow.service
	l := logger.L(ctx)
	var orgID, userID int64
	if req != nil {
		orgID = req.OrgId
		userID = req.UserId
	}
	l.Infow("gRPC: 收到 operator bootstrap 请求",
		"action", "bootstrap_operator",
		"org_id", orgID,
		"user_id", userID,
	)

	if err := validateBootstrapOperatorRequest(s, req); err != nil {
		return nil, err
	}

	created, finalResult, err := s.runBootstrapOperator(ctx, req)
	if err != nil {
		return nil, err
	}

	l.Infow("operator bootstrap 完成",
		"action", "bootstrap_operator",
		"org_id", req.OrgId,
		"user_id", req.UserId,
		"operator_id", finalResult.ID,
		"created", created,
		"roles", finalResult.Roles,
	)

	return buildBootstrapOperatorResponse(finalResult, created), nil
}

func validateBootstrapOperatorRequest(s *InternalService, req *pb.BootstrapOperatorRequest) error {
	switch {
	case s.operatorLifecycleService == nil || s.operatorQueryService == nil:
		return status.Error(codes.FailedPrecondition, "operator services not configured")
	case req == nil:
		return status.Error(codes.InvalidArgument, "request 不能为空")
	case req.OrgId <= 0:
		return status.Error(codes.InvalidArgument, "org_id 不能为空")
	case req.UserId <= 0:
		return status.Error(codes.InvalidArgument, "user_id 不能为空")
	case req.Name == "":
		return status.Error(codes.InvalidArgument, "name 不能为空")
	default:
		return nil
	}
}

func (s *InternalService) bootstrapOperatorCreated(ctx context.Context, orgID, userID int64) (bool, error) {
	if _, err := s.operatorQueryService.GetByUser(ctx, orgID, userID); err != nil {
		if errors.IsCode(err, errorCode.ErrUserNotFound) {
			return true, nil
		}
		return false, status.Errorf(codes.Internal, "query existing operator failed: %v", err)
	}
	return false, nil
}

func (s *InternalService) runBootstrapOperator(
	ctx context.Context,
	req *pb.BootstrapOperatorRequest,
) (bool, *operatorApp.OperatorResult, error) {
	created, err := s.bootstrapOperatorCreated(ctx, req.OrgId, req.UserId)
	if err != nil {
		return false, nil, err
	}

	result, err := s.operatorLifecycleService.EnsureByUser(ctx, req.OrgId, req.UserId, req.Name)
	if err != nil {
		return false, nil, status.Errorf(codes.Internal, "ensure operator failed: %v", err)
	}

	if err := s.syncBootstrapOperatorProfile(ctx, req, result.ID); err != nil {
		return false, nil, err
	}
	if err := s.syncBootstrapOperatorActivation(ctx, req, result.ID); err != nil {
		return false, nil, err
	}
	if err := s.syncBootstrapOperatorRoles(ctx, req, result.ID); err != nil {
		return false, nil, err
	}

	finalResult, err := s.operatorQueryService.GetByUser(ctx, req.OrgId, req.UserId)
	if err != nil {
		return false, nil, status.Errorf(codes.Internal, "query operator after bootstrap failed: %v", err)
	}
	return created, finalResult, nil
}

func (s *InternalService) syncBootstrapOperatorProfile(
	ctx context.Context,
	req *pb.BootstrapOperatorRequest,
	operatorID uint64,
) error {
	if req.Name == "" && req.Email == "" && req.Phone == "" {
		return nil
	}
	if err := s.operatorLifecycleService.UpdateFromExternalSource(ctx, operatorID, req.Name, req.Email, req.Phone); err != nil {
		return status.Errorf(codes.Internal, "sync operator profile failed: %v", err)
	}
	return nil
}

func (s *InternalService) syncBootstrapOperatorActivation(
	ctx context.Context,
	req *pb.BootstrapOperatorRequest,
	operatorID uint64,
) error {
	if s.operatorAuthService == nil {
		return nil
	}

	var err error
	if req.IsActive {
		err = s.operatorAuthService.Activate(ctx, operatorID)
		if err != nil {
			return status.Errorf(codes.Internal, "activate operator failed: %v", err)
		}
		return nil
	}

	err = s.operatorAuthService.Deactivate(ctx, operatorID)
	if err != nil {
		return status.Errorf(codes.Internal, "deactivate operator failed: %v", err)
	}
	return nil
}

func (s *InternalService) syncBootstrapOperatorRoles(
	ctx context.Context,
	req *pb.BootstrapOperatorRequest,
	operatorID uint64,
) error {
	if !req.IsActive || s.operatorRoleSyncer == nil {
		return nil
	}
	if err := s.operatorRoleSyncer.SyncRoles(ctx, req.OrgId, operatorID); err != nil {
		return status.Errorf(codes.Internal, "%v", err)
	}
	return nil
}

func bootstrapOperatorMessage(created bool) string {
	if created {
		return "operator bootstrapped"
	}
	return "operator already exists"
}

func buildBootstrapOperatorResponse(result *operatorApp.OperatorResult, created bool) *pb.BootstrapOperatorResponse {
	if result == nil {
		return &pb.BootstrapOperatorResponse{
			Created: created,
			Message: bootstrapOperatorMessage(created),
		}
	}
	return &pb.BootstrapOperatorResponse{
		OperatorId: result.ID,
		Created:    created,
		Message:    bootstrapOperatorMessage(created),
		Roles:      append([]string(nil), result.Roles...),
	}
}
