package operator

import (
	"context"
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"testing"

	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
)

// A role-only request must never clear scoped assignments or touch local state.
func TestReplaceRolesRejectsLegacyWritesBeforeDependencies(t *testing.T) {
	for _, roles := range [][]string{nil, {}, {"qs:assessment_operator"}} {
		service := NewAuthorizationService(nil, nil, nil, nil, nil)
		err := service.ReplaceRoles(context.Background(), 42, roles)
		if err == nil || !errors.IsCode(err, code.ErrValidation) {
			t.Fatalf("roles=%v error=%v, want validation rejection", roles, err)
		}
	}
}

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
