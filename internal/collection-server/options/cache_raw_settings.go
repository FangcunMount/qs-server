package options

import genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"

func (o *Options) ValidateRawSettings(settings map[string]any) error {
	leaf := genericoptions.FieldSchema(nil)
	catalog := genericoptions.FieldSchema{
		"enabled": leaf, "ttl_seconds": leaf, "ttl_jitter_ratio": leaf,
		"max_entries": leaf, "singleflight": leaf, "signal_evict_enabled": leaf,
	}
	return genericoptions.ValidateRawSection(settings, "cache", genericoptions.FieldSchema{
		"capabilities": {
			"catalog":       {"questionnaire": catalog, "typology": catalog},
			"report_status": {"ttl_seconds": leaf},
		},
	})
}
