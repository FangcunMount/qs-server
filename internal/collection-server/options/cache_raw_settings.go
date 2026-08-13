package options

import (
	"fmt"

	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
)

func (o *Options) ValidateRawSettings(settings map[string]any) error {
	if hasNestedSetting(settings, "concurrency", "max-concurrency") || hasNestedSetting(settings, "concurrency", "max_concurrency") {
		return fmt.Errorf("concurrency.max-concurrency has been removed; use concurrency.max-query-concurrency")
	}
	if hasNestedSetting(settings, "iam", "jwks", "fetch-strategies") || hasNestedSetting(settings, "iam", "jwks", "fetch_strategies") {
		return fmt.Errorf("iam.jwks.fetch-strategies has been removed; configure iam.jwks.url and iam.jwks.grpc-endpoint")
	}
	leaf := genericoptions.FieldSchema(nil)
	if err := genericoptions.ValidateRawSection(settings, "cache", genericoptions.FieldSchema{"policy_file": leaf}); err != nil {
		return err
	}
	return genericoptions.ValidateRawSection(settings, "runtime_state", genericoptions.RuntimeStateRawSchema())
}

func hasNestedSetting(settings map[string]any, path ...string) bool {
	var current any = settings
	for _, key := range path {
		values, ok := current.(map[string]any)
		if !ok {
			return false
		}
		current, ok = values[key]
		if !ok {
			return false
		}
	}
	return true
}
