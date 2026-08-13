package app

import (
	"fmt"
	"testing"

	cliflag "github.com/FangcunMount/qs-server/pkg/flag"
	"github.com/spf13/viper"
)

type rawValidationOrderOptions struct {
	validated bool
}

func (o *rawValidationOrderOptions) Flags() (sets cliflag.NamedFlagSets) {
	sets.FlagSet("cache").Bool("cache.capabilities.demo.enabled", true, "test flag default")
	return sets
}

func (o *rawValidationOrderOptions) Validate() []error { return nil }

func (o *rawValidationOrderOptions) ValidateRawSettings(settings map[string]any) error {
	o.validated = true
	cache, _ := settings["cache"].(map[string]any)
	if _, exists := cache["capabilities"]; exists {
		return fmt.Errorf("bound flag defaults leaked into raw main settings")
	}
	return nil
}

func TestRunCommandValidatesMainSettingsBeforeBindingFlagDefaults(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("cache.policy_file", "cache/test.yaml")

	opts := &rawValidationOrderOptions{}
	application := NewApp("test", "raw-validation-order", WithOptions(opts))
	if err := application.runCommand(application.cmd, nil); err != nil {
		t.Fatalf("runCommand() error = %v", err)
	}
	if !opts.validated {
		t.Fatal("raw main settings were not validated")
	}
}
