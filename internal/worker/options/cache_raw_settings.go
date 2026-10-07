package options

import (
	"fmt"

	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
)

func (o *Options) ValidateRawSettings(settings map[string]any) error {
	if genericoptions.HasRawSetting(settings, "worker", "max-retries") || genericoptions.HasRawSetting(settings, "worker", "max_retries") {
		return fmt.Errorf("worker.max-retries has been removed; use messaging.delivery.max-attempts")
	}
	o.deliveryConfigured = genericoptions.HasRawSetting(settings, "messaging", "delivery")
	if err := genericoptions.ValidateRawSection(settings, "cache", genericoptions.FieldSchema{}); err != nil {
		return err
	}
	return genericoptions.ValidateRawSection(settings, "runtime_state", genericoptions.RuntimeStateRawSchema())
}
