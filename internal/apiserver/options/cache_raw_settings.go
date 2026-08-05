package options

import (
	"fmt"

	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
)

func (o *Options) ValidateRawSettings(settings map[string]any) error {
	if hasNestedSetting(settings, "iam", "jwks", "fetch-strategies") || hasNestedSetting(settings, "iam", "jwks", "fetch_strategies") {
		return fmt.Errorf("iam.jwks.fetch-strategies has been removed; configure iam.jwks.url and iam.jwks.grpc-endpoint")
	}
	leaf := genericoptions.FieldSchema(nil)
	policy := genericoptions.FieldSchema{"enabled": leaf, "ttl": leaf, "negative_ttl": leaf, "ttl_jitter_ratio": leaf, "compress": leaf, "singleflight": leaf, "negative": leaf}
	family := genericoptions.FieldSchema{"negative_ttl": leaf, "ttl_jitter_ratio": leaf, "compress": leaf, "singleflight": leaf, "negative": leaf}
	return genericoptions.ValidateRawSection(settings, "cache", genericoptions.FieldSchema{
		"defaults": {"compress_payload": leaf, "ttl_jitter_ratio": leaf, "static": family, "object": family, "query": family},
		"capabilities": {
			"survey": {"questionnaire": policy}, "modelcatalog": {"published_model": policy},
			"evaluation": {"assessment_detail": policy, "assessment_list": policy},
			"actor":      {"testee": policy}, "plan": {"detail": policy}, "statistics": {"query": policy},
			"report_status": {"ttl_seconds": leaf},
		},
		"governance": {
			"statistics_warmup": {"enable": leaf, "warm_on_startup": leaf, "org_ids": leaf, "overview_presets": leaf},
			"warmup":            {"enable": leaf, "startup": {"static": leaf, "query": leaf}, "hotset": {"enable": leaf, "top_n": leaf, "max_items_per_kind": leaf}},
		},
	})
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
