package options

import genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"

func (o *Options) ValidateRawSettings(settings map[string]any) error {
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
			"statistics_warmup":        {"enable": leaf, "warm_on_startup": leaf, "org_ids": leaf, "overview_presets": leaf, "questionnaire_codes": leaf, "plan_ids": leaf},
			"statistics_system":        {"service_singleflight": leaf, "disable_realtime_fallback": leaf, "stale_on_timeout": leaf, "load_timeout": leaf},
			"statistics_overview":      {"service_singleflight": leaf, "stale_on_timeout": leaf, "load_timeout": leaf},
			"statistics_questionnaire": {"service_singleflight": leaf, "stale_on_timeout": leaf, "load_timeout": leaf},
			"warmup":                   {"enable": leaf, "startup": {"static": leaf, "query": leaf}, "hotset": {"enable": leaf, "top_n": leaf, "max_items_per_kind": leaf}},
		},
	})
}
