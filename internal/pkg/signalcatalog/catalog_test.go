package signalcatalog

import (
	"os"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

type manifest struct {
	Signals map[string]manifestSignal `yaml:"signals"`
}

type manifestSignal struct {
	Delivery  string `yaml:"delivery"`
	Transport string `yaml:"transport"`
}

func TestSignalsManifestAndCodeConstantsStayInSync(t *testing.T) {
	raw, err := os.ReadFile("../../../configs/signals.yaml")
	if err != nil {
		t.Fatalf("read signals.yaml: %v", err)
	}

	var cfg manifest
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse signals.yaml: %v", err)
	}

	for _, signalName := range SignalNames() {
		signal, ok := cfg.Signals[signalName]
		if !ok {
			t.Fatalf("code signal %q missing from signals.yaml", signalName)
		}
		if signal.Delivery != "ephemeral_signal" || signal.Transport != "redis_pubsub" {
			t.Fatalf("signal %q = delivery %q transport %q, want ephemeral_signal/redis_pubsub", signalName, signal.Delivery, signal.Transport)
		}
	}
	for signalName := range cfg.Signals {
		if !slices.Contains(SignalNames(), signalName) {
			t.Fatalf("signals.yaml signal %q missing from code constants", signalName)
		}
	}
}
