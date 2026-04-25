package process

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FangcunMount/qs-server/internal/worker/options"
)

func TestWorkerEventConfigPathUsesOverrideOrDefault(t *testing.T) {
	if got := workerEventConfigPath(nil); got != defaultEventConfigPath {
		t.Fatalf("path = %q, want %q", got, defaultEventConfigPath)
	}
	if got := workerEventConfigPath(&options.WorkerOptions{}); got != defaultEventConfigPath {
		t.Fatalf("path = %q, want %q", got, defaultEventConfigPath)
	}
	if got := workerEventConfigPath(&options.WorkerOptions{EventConfigPath: "custom/events.yaml"}); got != "custom/events.yaml" {
		t.Fatalf("path = %q, want custom/events.yaml", got)
	}
}

func TestLoadWorkerEventCatalogLoadsConfiguredPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.yaml")
	if err := os.WriteFile(path, []byte(`
version: "1.0"
topics:
  sample:
    name: sample.topic
events:
  sample.created:
    topic: sample
    delivery: best_effort
    handler: sample_handler
`), 0o600); err != nil {
		t.Fatalf("write events config: %v", err)
	}

	catalog, err := loadWorkerEventCatalog(path)
	if err != nil {
		t.Fatalf("loadWorkerEventCatalog: %v", err)
	}
	topic, ok := catalog.GetTopicForEvent("sample.created")
	if !ok {
		t.Fatalf("sample.created topic not found")
	}
	if topic != "sample.topic" {
		t.Fatalf("topic = %q, want sample.topic", topic)
	}
}

func TestLoadWorkerEventCatalogReturnsErrorForInvalidPath(t *testing.T) {
	_, err := loadWorkerEventCatalog(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatalf("loadWorkerEventCatalog should fail for missing file")
	}
}
