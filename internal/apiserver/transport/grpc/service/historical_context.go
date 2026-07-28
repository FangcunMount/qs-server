package service

import (
	"context"

	commonpb "github.com/FangcunMount/qs-server/api/grpc/gen/common"
	"github.com/FangcunMount/qs-server/internal/pkg/historicalseed"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func withHistoricalExecutionContext(ctx context.Context, in *commonpb.HistoricalExecutionContext, expectedOrgID uint64, verifier *historicalseed.Verifier) (context.Context, error) {
	if in == nil {
		return ctx, nil
	}
	historical, err := historicalseed.FromProto(in)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "historical_context 无效: %v", err)
	}
	if expectedOrgID != 0 {
		if err := historical.ValidateOrg(expectedOrgID); err != nil {
			return nil, status.Error(codes.PermissionDenied, "historical_context 机构不匹配")
		}
	}
	if err := verifier.VerifyForwarded(historical); err != nil {
		return nil, status.Errorf(codes.PermissionDenied, "historical_context 未授权: %v", err)
	}
	return historicalseed.WithContext(ctx, historical), nil
}
