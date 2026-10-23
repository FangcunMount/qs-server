//go:build integration && m3_host_runtime

package container

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
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
