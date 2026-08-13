package process

import (
	"context"
	"reflect"

	componenterrors "github.com/FangcunMount/component-base/pkg/errors"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance"
	cachebootstrap "github.com/FangcunMount/qs-server/internal/apiserver/cache/subsystem"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func (s *server) cachePolicyCandidateLoader(currentRegistry sharedcache.PolicyProvider) cachegov.PolicyCandidateLoader {
	return func(ctx context.Context) ([]sharedcache.EffectiveCapability, sharedcache.PolicySource, error) {
		if s == nil || s.config == nil || s.config.Options == nil || s.config.CachePolicySource() == nil {
			return nil, sharedcache.PolicySource{}, componenterrors.WithCode(code.ErrInternalServerError, "startup cache policy source unavailable")
		}
		candidate, source, err := s.config.CachePolicySource().Read(ctx)
		if err != nil {
			return nil, source, componenterrors.WithCode(code.ErrInvalidArgument, "invalid cache policy: %s", err.Error())
		}
		if !reflect.DeepEqual(candidate.Governance, s.config.Cache.Governance) {
			return nil, source, componenterrors.WithCode(code.ErrInvalidArgument, "cache.governance cannot be changed by cache.reload_policy")
		}

		capabilities := cachebootstrap.BuildEffectiveCapabilities(buildContainerCacheOptions(candidate, s.config.RuntimeState))
		for _, item := range capabilities {
			if item.Kind != sharedcache.KindCache {
				continue
			}
			current, ok := currentRegistry.Resolve(item.Capability)
			if !ok {
				return nil, source, componenterrors.WithCode(code.ErrInternalServerError, "current cache capability %s unavailable", item.Capability)
			}
			if current.Enabled != item.Enabled {
				return nil, source, componenterrors.WithCode(code.ErrInvalidArgument, "cache capability %s enabled cannot be changed by cache.reload_policy", item.Capability)
			}
			if current.Family != item.Family || current.Layer != item.Layer {
				return nil, source, componenterrors.WithCode(code.ErrInvalidArgument, "cache capability %s family/layer cannot be changed by cache.reload_policy", item.Capability)
			}
		}
		return capabilities, source, nil
	}
}
