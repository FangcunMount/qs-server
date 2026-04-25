package serviceauth

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
	"github.com/FangcunMount/qs-server/internal/pkg/securityprojection"
)

// TokenProvider is the narrow token source needed by gRPC bearer credentials.
type TokenProvider interface {
	GetToken(ctx context.Context) (string, error)
}

// BearerRequestMetadata builds the current PerRPC metadata contract.
func BearerRequestMetadata(ctx context.Context, provider TokenProvider) (map[string]string, error) {
	if provider == nil {
		return nil, fmt.Errorf("service auth token provider is nil")
	}
	token, err := provider.GetToken(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]string{"authorization": "Bearer " + token}, nil
}

// RequireTransportSecurity returns the current compatibility contract.
func RequireTransportSecurity() bool {
	return false
}

// ServiceIdentity projects service auth config into the Security Control Plane model.
func ServiceIdentity(serviceID string, audience []string) securityplane.ServiceIdentity {
	return securityprojection.ServiceIdentityFromInput(securityprojection.ServiceIdentityInput{
		ServiceID:      serviceID,
		Source:         securityplane.ServiceIdentitySourceServiceAuth,
		TargetAudience: audience,
	})
}
