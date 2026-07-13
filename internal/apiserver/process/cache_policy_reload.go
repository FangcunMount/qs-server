package process

import (
	"context"
	"reflect"
	"strings"

	componenterrors "github.com/FangcunMount/component-base/pkg/errors"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance"
	cachebootstrap "github.com/FangcunMount/qs-server/internal/apiserver/cache/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/spf13/viper"
)

func (s *server) cachePolicyCandidateLoader(currentRegistry sharedcache.PolicyProvider) cachegov.PolicyCandidateLoader {
	return func(ctx context.Context) ([]sharedcache.EffectiveCapability, string, error) {
		if s == nil || s.config == nil || s.config.Options == nil || s.config.RawSettingsSource() == nil {
			return nil, "", componenterrors.WithCode(code.ErrInternalServerError, "startup cache configuration source unavailable")
		}
		raw, err := s.config.RawSettingsSource().Read(ctx)
		if err != nil {
			return nil, "", componenterrors.WithCode(code.ErrInternalServerError, "read cache configuration: %s", err.Error())
		}
		candidate := options.NewOptions()
		if err := candidate.ValidateRawSettings(raw.Values); err != nil {
			return nil, raw.Source, componenterrors.WithCode(code.ErrInvalidArgument, "invalid cache configuration: %s", err.Error())
		}
		decoder := viper.New()
		if err := decoder.MergeConfigMap(raw.Values); err != nil {
			return nil, raw.Source, componenterrors.WithCode(code.ErrInvalidArgument, "invalid cache configuration: %s", err.Error())
		}
		if err := decoder.Unmarshal(candidate); err != nil {
			return nil, raw.Source, componenterrors.WithCode(code.ErrInvalidArgument, "decode cache configuration: %s", err.Error())
		}
		if validationErrs := options.ValidateCacheOptions(candidate.Cache); len(validationErrs) > 0 {
			messages := make([]string, 0, len(validationErrs))
			for _, validationErr := range validationErrs {
				messages = append(messages, validationErr.Error())
			}
			return nil, raw.Source, componenterrors.WithCode(code.ErrInvalidArgument, "invalid cache configuration: %s", strings.Join(messages, "; "))
		}
		if !reflect.DeepEqual(candidate.Cache.Governance, s.config.Cache.Governance) {
			return nil, raw.Source, componenterrors.WithCode(code.ErrInvalidArgument, "cache.governance cannot be changed by cache.reload_policy")
		}
		if !reflect.DeepEqual(candidate.Cache.Capabilities.ReportStatus, s.config.Cache.Capabilities.ReportStatus) {
			return nil, raw.Source, componenterrors.WithCode(code.ErrInvalidArgument, "cache.capabilities.report_status cannot be changed by cache.reload_policy")
		}

		capabilities := cachebootstrap.BuildEffectiveCapabilities(buildContainerCacheOptions(candidate.Cache))
		for _, item := range capabilities {
			if item.Kind != sharedcache.KindCache {
				continue
			}
			current, ok := currentRegistry.Resolve(item.Capability)
			if !ok {
				return nil, raw.Source, componenterrors.WithCode(code.ErrInternalServerError, "current cache capability %s unavailable", item.Capability)
			}
			if current.Enabled != item.Enabled {
				return nil, raw.Source, componenterrors.WithCode(code.ErrInvalidArgument, "cache capability %s enabled cannot be changed by cache.reload_policy", item.Capability)
			}
			if current.Family != item.Family || current.Layer != item.Layer {
				return nil, raw.Source, componenterrors.WithCode(code.ErrInvalidArgument, "cache capability %s family/layer cannot be changed by cache.reload_policy", item.Capability)
			}
		}
		return capabilities, raw.Source, nil
	}
}
