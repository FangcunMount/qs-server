package operator

import (
	"context"

	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
)

type operatorAuthzGatewayFake struct {
	replaceCalls     int
	replacedRoles    []string
	committedVersion int64
	projection       iambridge.OperatorRoleProjection
}

func (*operatorAuthzGatewayFake) IsEnabled() bool { return true }
func (f *operatorAuthzGatewayFake) ReplaceManagedOperatorRoles(_ context.Context, _, _ int64, roles []string, _, _ string) (int64, error) {
	f.replaceCalls++
	f.replacedRoles = append([]string(nil), roles...)
	return f.committedVersion, nil
}
func (f *operatorAuthzGatewayFake) LoadOperatorRoleProjection(context.Context, int64, int64) (iambridge.OperatorRoleProjection, error) {
	return f.projection, nil
}
