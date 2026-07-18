package messaging

import (
	"encoding/json"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestRabbitAttemptMetadata(t *testing.T) {
	for _, test := range []struct {
		headers amqp.Table
		want    int
	}{{nil, 1}, {amqp.Table{rabbitDeliveryAttemptHeader: int32(8)}, 8}, {amqp.Table{rabbitDeliveryAttemptHeader: "3"}, 3}} {
		if got := rabbitAttempt(test.headers); got != test.want {
			t.Fatalf("rabbitAttempt(%#v)=%d want=%d", test.headers, got, test.want)
		}
	}
}

func TestDeadLetterRecordRetainsGovernanceIdentity(t *testing.T) {
	payload, err := json.Marshal(map[string]any{"id": "event-42", "data": map[string]any{"org_id": 18}})
	if err != nil {
		t.Fatal(err)
	}
	record := deadLetterRecord("nsq", "assessment-lifecycle", "qs-worker", 8, "message-7", payload, "exhausted")
	if record.EventID != "event-42" || record.OrgID == nil || *record.OrgID != 18 || record.DeliveryAttempts != 8 {
		t.Fatalf("dead-letter identity = %#v", record)
	}
}

func TestRabbitRetryDelayIsBoundedAndDeterministic(t *testing.T) {
	for attempt := 1; attempt <= 8; attempt++ {
		first := transportRetryDelay(attempt, "event-1")
		second := transportRetryDelay(attempt, "event-1")
		if first != second {
			t.Fatalf("attempt %d jitter is not deterministic: %s != %s", attempt, first, second)
		}
		if first < 24*time.Second || first > 6*time.Minute {
			t.Fatalf("attempt %d delay %s outside 20%% jitter bounds", attempt, first)
		}
	}
}
