//go:build integration && m3_host_runtime

package container

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"testing"
	"time"
)

type nativeProtocolStatusError struct{}

func (nativeProtocolStatusError) Error() string  { return "private original error material" }
func (nativeProtocolStatusError) GetCode() int32 { return 40000 }

func TestOwnedRuntimeErrorExportsOnlyFixedClassification(t *testing.T) {
	for _, tc := range []struct {
		err  error
		kind string
		code int
	}{
		{nil, "none", 0},
		{context.DeadlineExceeded, "deadline", 0},
		{errors.New("initial settings deadline exceeded"), "deadline", 0},
		{status.Error(codes.DeadlineExceeded, "private original error material"), "deadline", 4},
		{fmt.Errorf("private wrapper: %w", nativeProtocolStatusError{}), "other", 40000},
		{errors.New("private original error material"), "other", 0},
	} {
		result := ownerNativeError(tc.err)
		require.Equal(t, map[string]any{"kind": tc.kind, "code": tc.code}, result)
		raw, err := json.Marshal(result)
		require.NoError(t, err)
		require.NotContains(t, string(raw), "private")
	}
}

func TestOwnedRouteProbeUsesPinnedSignatureWithoutExportingCredentials(t *testing.T) {
	c := rocketmq.Config{AccessKey: "fixture-access", SecretKey: "fixture-secret"}
	ctx := ownedRouteSignature(t.Context(), c, "fixture-client", time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC))
	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok)
	require.Equal(t, []string{"20261023T000000Z"}, md.Get("x-mq-date-time"))
	require.Equal(t, []string{"MQv2-HMAC-SHA1 Credential=fixture-access//Rocketmq, SignedHeaders=x-mq-date-time, Signature=cb25685356100cebcd93d284785562e94747fce0"}, md.Get("authorization"))
	require.Equal(t, []string{"v2"}, md.Get("x-mq-protocol"))
	require.Equal(t, []string{"5.1.4"}, md.Get("x-mq-client-version"))
	require.Equal(t, []string{"fixture-client"}, md.Get("x-mq-client-id"))
	require.Len(t, md.Get("x-mq-request-id"), 1)
	require.NotContains(t, fmt.Sprint(md), "fixture-secret")
}
