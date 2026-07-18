package handlers

import (
	"context"
	"strconv"

	"google.golang.org/grpc/metadata"
)

func outgoingRetryAuthorization(ctx context.Context, eventID string, expectedAttempt int, origin, actionRequestID, mode string) context.Context {
	if expectedAttempt < 1 || origin == "" {
		return ctx
	}
	pairs := []string{
		"x-retry-event-id", eventID,
		"x-retry-expected-attempt", strconv.Itoa(expectedAttempt),
		"x-retry-origin", origin,
		"x-retry-mode", mode,
	}
	if actionRequestID != "" {
		pairs = append(pairs, "x-retry-action-request-id", actionRequestID)
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...)
}
