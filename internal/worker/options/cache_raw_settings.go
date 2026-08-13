package options

import (
	"fmt"

	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
)

func (o *Options) ValidateRawSettings(settings map[string]any) error {
	if hasNestedSetting(settings, "worker", "max-retries") || hasNestedSetting(settings, "worker", "max_retries") {
		return fmt.Errorf("worker.max-retries has been removed; use messaging.delivery.max-attempts")
	}
	o.deliveryConfigured = hasNestedSetting(settings, "messaging", "delivery")
	if err := genericoptions.ValidateRawSection(settings, "cache", genericoptions.FieldSchema{}); err != nil {
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
