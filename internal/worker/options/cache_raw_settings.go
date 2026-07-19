package options

import genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"

func (o *Options) ValidateRawSettings(settings map[string]any) error {
	o.deliveryConfigured = hasNestedSetting(settings, "messaging", "delivery")
	leaf := genericoptions.FieldSchema(nil)
	return genericoptions.ValidateRawSection(settings, "cache", genericoptions.FieldSchema{
		"capabilities": {"report_status": {"ttl_seconds": leaf}},
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
