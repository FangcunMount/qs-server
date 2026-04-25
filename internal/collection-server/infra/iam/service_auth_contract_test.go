package iam

import (
	"context"
	"testing"
)

func TestServiceAuthHelperContractWithoutSDKHelper(t *testing.T) {
	t.Parallel()

	helper := &ServiceAuthHelper{}
	if helper.RequireTransportSecurity() {
		t.Fatal("RequireTransportSecurity() = true, want false for current compatibility contract")
	}
	if _, err := helper.GetRequestMetadata(context.Background()); err == nil {
		t.Fatal("GetRequestMetadata() succeeded with nil SDK helper")
	}
	helper.Stop()
}
