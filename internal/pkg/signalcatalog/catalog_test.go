package signalcatalog

import (
	"os"
	"sort"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type manifest struct {
	Signals map[string]manifestSignal `yaml:"signals"`
}

type manifestSignal struct {
	Delivery  string `yaml:"delivery"`
	Transport string `yaml:"transport"`
	Publisher string `yaml:"publisher"`
	Subscribers []string `yaml:"subscribers"`
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

	wantTopology := map[string]struct {
		publishers  []string
		subscribers []string
	}{
		ReportStatusChanged:       {publishers: []string{"apiserver", "worker"}, subscribers: []string{"collection-server"}},
		QuestionnaireCacheChanged: {publishers: []string{"apiserver"}, subscribers: []string{"apiserver", "collection-server"}},
		ScaleCacheChanged:         {publishers: []string{"apiserver"}, subscribers: []string{"apiserver"}},
		TypologyModelCacheChanged: {publishers: []string{"apiserver"}, subscribers: []string{"apiserver", "collection-server"}},
	}
	for signalName, want := range wantTopology {
		signal := cfg.Signals[signalName]
		publishers := splitManifestList(signal.Publisher)
		subscribers := append([]string(nil), signal.Subscribers...)
		sort.Strings(subscribers)
		sort.Strings(want.publishers)
		sort.Strings(want.subscribers)
		if !slices.Equal(publishers, want.publishers) || !slices.Equal(subscribers, want.subscribers) {
			t.Fatalf("signal %q topology publishers=%v subscribers=%v, want publishers=%v subscribers=%v",
				signalName, publishers, subscribers, want.publishers, want.subscribers)
		}
	}
}

func splitManifestList(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			result = append(result, item)
		}
	}
	sort.Strings(result)
	return result
}
