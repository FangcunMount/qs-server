package service

import (
	"context"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"google.golang.org/grpc/metadata"
)

const (
	retryEventIDMetadata         = "x-retry-event-id"
	retryExpectedAttemptMetadata = "x-retry-expected-attempt"
	retryOriginMetadata          = "x-retry-origin"
	retryActionRequestMetadata   = "x-retry-action-request-id"
	retryModeMetadata            = "x-retry-mode"
)

func withRetryAuthorization(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	expected, err := strconv.Atoi(firstMetadataValue(md, retryExpectedAttemptMetadata))
	if err != nil || expected < 1 {
		return ctx
	}
	origin := retrygovernance.AttemptOrigin(firstMetadataValue(md, retryOriginMetadata))
	if !origin.IsValid() {
		return ctx
	}
	return retrygovernance.WithAuthorization(ctx, retrygovernance.Authorization{
		EventID: firstMetadataValue(md, retryEventIDMetadata), ExpectedAttempt: expected, Origin: origin,
		ActionRequestID: firstMetadataValue(md, retryActionRequestMetadata), Mode: firstMetadataValue(md, retryModeMetadata),
	})
}

func firstMetadataValue(md metadata.MD, key string) string {
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
