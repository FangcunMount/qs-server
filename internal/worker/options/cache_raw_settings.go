package options

import genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"

func (o *Options) ValidateRawSettings(settings map[string]any) error {
	leaf := genericoptions.FieldSchema(nil)
	return genericoptions.ValidateRawSection(settings, "cache", genericoptions.FieldSchema{
		"capabilities": {"report_status": {"ttl_seconds": leaf}},
	})
}
