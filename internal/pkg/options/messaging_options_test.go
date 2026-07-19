package options

import (
	"strings"
	"testing"
)

func TestTransportDeliveryOptionsValidateHardCapWhenDisabled(t *testing.T) {
	options := &TransportDeliveryOptions{Enable: false, MaxAttempts: 9}
	errs := options.Validate("messaging.delivery")
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "between 1 and 8") {
		t.Fatalf("Validate() = %v, want hard-cap error", errs)
	}
}

func TestTransportDeliveryOptionsDisabledMeansOneAttempt(t *testing.T) {
	options := &TransportDeliveryOptions{Enable: false, MaxAttempts: 8}
	if got := options.EffectiveMaxAttempts(); got != 1 {
		t.Fatalf("EffectiveMaxAttempts() = %d, want 1", got)
	}
}

func TestIAMAuthzSyncValidatesDeliveryWhileDisabled(t *testing.T) {
	options := NewIAMAuthzSyncOptions()
	options.Enabled = false
	options.Delivery.MaxAttempts = 9
	if errs := options.Validate(); len(errs) != 1 || !strings.Contains(errs[0].Error(), "between 1 and 8") {
		t.Fatalf("Validate() = %v, want delivery hard-cap error", errs)
	}
}
