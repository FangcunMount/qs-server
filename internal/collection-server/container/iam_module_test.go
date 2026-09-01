package container

import (
	"context"
	"strings"
	"testing"
)

func TestValidateRequiredRuntimeFailsClosedWithoutIAM(t *testing.T) {
	var module IAMModule
	if err := module.ValidateRequiredRuntime(context.Background()); err == nil || !strings.Contains(err.Error(), "IAM integration is required") {
		t.Fatalf("ValidateRequiredRuntime() error = %v, want required IAM error", err)
	}
}
