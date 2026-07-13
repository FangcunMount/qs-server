package eventcatalog

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func loadDefaultRegistry(t *testing.T) *EffectiveRegistry {
	t.Helper()
	cfg, err := Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("Load events.yaml: %v", err)
	}
	registry, err := NewEffectiveRegistry(NewCatalog(cfg), DefaultSpecs())
	if err != nil {
		t.Fatalf("NewEffectiveRegistry: %v", err)
	}
	return registry
}

func TestEffectiveRegistryAndContractMatrixStayInSync(t *testing.T) {
	registry := loadDefaultRegistry(t)
	matrix, err := os.ReadFile("../../../../docs/03-基础设施/event/09-事件契约矩阵.md")
	if err != nil {
		t.Fatalf("read event matrix: %v", err)
	}
	rows := make(map[string][]string)
	for _, line := range strings.Split(string(matrix), "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) < 9 {
			continue
		}
		eventType := strings.Trim(strings.TrimSpace(cells[1]), "`")
		if _, ok := registry.Lookup(eventType); ok {
			rows[eventType] = cells
		}
	}
	if len(rows) != len(registry.Snapshot()) {
		t.Fatalf("matrix event rows = %d, effective events = %d", len(rows), len(registry.Snapshot()))
	}
	for _, evt := range registry.Snapshot() {
		row := rows[evt.Type]
		joined := strings.Join(row, "|")
		for _, want := range []string{string(evt.Delivery), evt.PrimaryHandler, yesNo(evt.Immediate)} {
			if !strings.Contains(joined, want) {
				t.Fatalf("matrix row for %q does not contain %q: %s", evt.Type, want, joined)
			}
		}
		wantPriority := "无"
		if evt.Priority != PriorityNone {
			wantPriority = strings.ToUpper(string(evt.Priority))
		}
		if got := strings.TrimSpace(row[6]); got != wantPriority {
			t.Fatalf("matrix priority for %q = %q, want %q", evt.Type, got, wantPriority)
		}
		if strings.TrimSpace(row[8]) == "" || strings.TrimSpace(row[9]) == "" {
			t.Fatalf("matrix row for %q must declare idempotency and settlement: %s", evt.Type, joined)
		}
		switch evt.OutboxProfile {
		case OutboxProfileMongoDomain:
			if !strings.Contains(joined, "Mongo `domain_event_outbox`") {
				t.Fatalf("matrix row for %q has wrong Mongo store: %s", evt.Type, joined)
			}
		case OutboxProfileAssessmentMySQL:
			if !strings.Contains(joined, "MySQL `domain_event_outbox`") {
				t.Fatalf("matrix row for %q has wrong MySQL store: %s", evt.Type, joined)
			}
		}
	}
}

func yesNo(value bool) string {
	if value {
		return "是"
	}
	return "否"
}

func TestEffectiveRegistryCoversWireCatalog(t *testing.T) {
	registry := loadDefaultRegistry(t)
	if len(registry.Snapshot()) != len(EventTypes()) {
		t.Fatalf("effective events = %d, code events = %d", len(registry.Snapshot()), len(EventTypes()))
	}
	for _, eventType := range EventTypes() {
		if _, ok := registry.Lookup(eventType); !ok {
			t.Fatalf("event %q missing from effective registry", eventType)
		}
	}
}

func TestEffectiveRegistryDerivesProfilePolicy(t *testing.T) {
	registry := loadDefaultRegistry(t)

	if got := registry.ImmediateTypes(OutboxProfileMongoDomain); !slices.Equal(got, []string{AnswerSheetSubmitted}) {
		t.Fatalf("mongo immediate types = %#v", got)
	}
	if got := registry.ImmediateTypes(OutboxProfileAssessmentMySQL); !slices.Equal(got, []string{EvaluationOutcomeCommitted, EvaluationRequested}) && !slices.Equal(got, []string{EvaluationRequested, EvaluationOutcomeCommitted}) {
		t.Fatalf("mysql immediate types = %#v", got)
	}
	if got := registry.PriorityBucket(AnswerSheetSubmitted); got != string(PriorityP0) {
		t.Fatalf("answersheet bucket = %q", got)
	}
	if got := registry.PriorityBucket(TaskOpened); got != string(PriorityP2) {
		t.Fatalf("best-effort fallback bucket = %q", got)
	}

	tiers := registry.PriorityTiers(OutboxProfileMongoDomain)
	if len(tiers) != 3 || !slices.Contains(tiers[0], AnswerSheetSubmitted) || tiers[2] != nil {
		t.Fatalf("mongo priority tiers = %#v", tiers)
	}
}

func TestEffectiveRegistryDeclaresHotRankSecondaryConsumer(t *testing.T) {
	consumers := loadDefaultRegistry(t).Consumers(AnswerSheetSubmitted)
	if len(consumers) != 1 {
		t.Fatalf("consumers = %#v", consumers)
	}
	if consumers[0].ID != "modelcatalog.hot_rank_projection" || consumers[0].Channel != "qs-apiserver-modelcatalog-hot-rank-v1" {
		t.Fatalf("consumer = %#v", consumers[0])
	}
}

func TestEffectiveRegistryRejectsDeliveryPolicyMismatch(t *testing.T) {
	cfg, err := Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	specs := DefaultSpecs()
	for i := range specs {
		if specs[i].Type == TaskOpened {
			specs[i].Immediate = true
			break
		}
	}
	if _, err := NewEffectiveRegistry(NewCatalog(cfg), specs); err == nil {
		t.Fatal("best-effort immediate policy must be rejected")
	}
}
