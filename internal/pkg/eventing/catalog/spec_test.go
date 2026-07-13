package eventcatalog

import (
	"os"
	"slices"
	"strconv"
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
	matrixBytes, err := os.ReadFile("../../../../docs/03-基础设施/event/09-事件契约矩阵.md")
	if err != nil {
		t.Fatalf("read event matrix: %v", err)
	}
	matrix := string(matrixBytes)
	eventRows := parseMarkdownTable(t, matrix, []string{
		"Event type", "Owner", "Producer", "Delivery", "Profile", "Store", "Immediate", "Priority",
		"Handler", "Idempotency policy", "Settlement policy", "Handler failure behavior",
	})
	rows := indexMarkdownRows(t, eventRows, "Event type")
	if len(rows) != len(registry.Snapshot()) {
		t.Fatalf("matrix event rows = %d, effective events = %d", len(rows), len(registry.Snapshot()))
	}
	for _, evt := range registry.Snapshot() {
		row := rows[evt.Type]
		if row == nil {
			t.Fatalf("matrix is missing event %q", evt.Type)
		}
		want := map[string]string{
			"Owner":              evt.Owner,
			"Delivery":           string(evt.Delivery),
			"Profile":            string(evt.OutboxProfile),
			"Store":              matrixStoreToken(evt.OutboxProfile),
			"Immediate":          strconv.FormatBool(evt.Immediate),
			"Priority":           string(evt.Priority),
			"Handler":            evt.PrimaryHandler,
			"Idempotency policy": evt.IdempotencyPolicy,
			"Settlement policy":  string(evt.SettlementPolicy),
		}
		for field, expected := range want {
			if got := row[field]; got != expected {
				t.Fatalf("matrix %s for %q = %q, want %q", field, evt.Type, got, expected)
			}
		}
		if row["Producer"] == "" || row["Handler failure behavior"] == "" {
			t.Fatalf("matrix row for %q must include readable producer and failure behavior", evt.Type)
		}
	}

	consumerRows := parseMarkdownTable(t, matrix, []string{
		"Consumer ID", "Event", "Runtime", "Topic", "Channel", "Idempotency policy", "Settlement policy",
	})
	actualConsumers := indexMarkdownRows(t, consumerRows, "Consumer ID")
	wantConsumerCount := 0
	for _, evt := range registry.Snapshot() {
		for _, consumer := range evt.AdditionalConsumers {
			wantConsumerCount++
			row := actualConsumers[consumer.ID]
			if row == nil {
				t.Fatalf("additional consumer table is missing %q", consumer.ID)
			}
			want := map[string]string{
				"Event": evt.Type, "Runtime": consumer.Runtime, "Topic": evt.Topic,
				"Channel": consumer.Channel, "Idempotency policy": consumer.IdempotencyPolicy,
				"Settlement policy": string(consumer.SettlementPolicy),
			}
			for field, expected := range want {
				if got := row[field]; got != expected {
					t.Fatalf("additional consumer %s for %q = %q, want %q", field, consumer.ID, got, expected)
				}
			}
		}
	}
	if len(actualConsumers) != wantConsumerCount {
		t.Fatalf("additional consumer rows = %d, registry consumers = %d", len(actualConsumers), wantConsumerCount)
	}
}

func matrixStoreToken(profile OutboxProfile) string {
	switch profile {
	case OutboxProfileMongoDomain:
		return "Mongo domain_event_outbox"
	case OutboxProfileAssessmentMySQL:
		return "MySQL domain_event_outbox"
	default:
		return "none"
	}
}

func parseMarkdownTable(t *testing.T, document string, headers []string) []map[string]string {
	t.Helper()
	lines := strings.Split(document, "\n")
	for index, line := range lines {
		cells := markdownCells(line)
		if !slices.Equal(cells, headers) {
			continue
		}
		var rows []map[string]string
		for _, rowLine := range lines[index+2:] {
			rowCells := markdownCells(rowLine)
			if len(rowCells) == 0 {
				break
			}
			if len(rowCells) != len(headers) {
				t.Fatalf("matrix row has %d cells, want %d: %s", len(rowCells), len(headers), rowLine)
			}
			row := make(map[string]string, len(headers))
			for i, header := range headers {
				row[header] = rowCells[i]
			}
			rows = append(rows, row)
		}
		return rows
	}
	t.Fatalf("matrix table with headers %v not found", headers)
	return nil
}

func markdownCells(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return nil
	}
	parts := strings.Split(strings.Trim(line, "|"), "|")
	cells := make([]string, len(parts))
	for i, part := range parts {
		cells[i] = strings.Trim(strings.TrimSpace(part), "`")
	}
	return cells
}

func indexMarkdownRows(t *testing.T, rows []map[string]string, key string) map[string]map[string]string {
	t.Helper()
	indexed := make(map[string]map[string]string, len(rows))
	for _, row := range rows {
		value := row[key]
		if value == "" {
			t.Fatalf("matrix row has empty %s: %#v", key, row)
		}
		if _, exists := indexed[value]; exists {
			t.Fatalf("matrix has duplicate %s %q", key, value)
		}
		indexed[value] = row
	}
	return indexed
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
