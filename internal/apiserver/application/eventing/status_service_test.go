package eventing

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
)

type fakeStatusReader struct {
	snapshot outboxport.StatusSnapshot
	err      error
}

func (r fakeStatusReader) OutboxStatusSnapshot(context.Context, time.Time) (outboxport.StatusSnapshot, error) {
	return r.snapshot, r.err
}

func TestStatusServiceReturnsCatalogAndOutboxSnapshots(t *testing.T) {
	cfg, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("load events config: %v", err)
	}
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	service := NewStatusService(StatusServiceOptions{
		Catalog: eventcatalog.NewCatalog(cfg),
		Outboxes: []NamedOutboxStatusReader{
			{
				Name: "mysql",
				Reader: fakeStatusReader{snapshot: outboxport.StatusSnapshot{
					Store:       "mysql",
					GeneratedAt: now,
					Buckets:     []outboxport.StatusBucket{{Status: "pending", Count: 2}},
				}},
			},
		},
		Now: func() time.Time { return now },
	})

	snapshot, err := service.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if snapshot.Catalog.EventCount == 0 || snapshot.Catalog.TopicCount == 0 {
		t.Fatalf("catalog summary = %#v, want non-empty", snapshot.Catalog)
	}
	if snapshot.Catalog.BestEffortCount == 0 || snapshot.Catalog.DurableOutboxCount == 0 {
		t.Fatalf("catalog delivery summary = %#v, want both delivery classes", snapshot.Catalog)
	}
	if len(snapshot.Outboxes) != 1 || snapshot.Outboxes[0].Degraded {
		t.Fatalf("outboxes = %#v, want one healthy outbox", snapshot.Outboxes)
	}
	if snapshot.Outboxes[0].Buckets[0].Count != 2 {
		t.Fatalf("bucket count = %d, want 2", snapshot.Outboxes[0].Buckets[0].Count)
	}
}

func TestStatusServiceMarksSingleOutboxDegraded(t *testing.T) {
	service := NewStatusService(StatusServiceOptions{
		Outboxes: []NamedOutboxStatusReader{
			{Name: "mysql", Reader: fakeStatusReader{err: errors.New("db unavailable")}},
		},
	})

	snapshot, err := service.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if len(snapshot.Outboxes) != 1 || !snapshot.Outboxes[0].Degraded {
		t.Fatalf("outboxes = %#v, want degraded outbox", snapshot.Outboxes)
	}
	if snapshot.Outboxes[0].Error == "" {
		t.Fatalf("degraded outbox should include error")
	}
}

func TestStatusServiceExportsCompleteEffectiveContract(t *testing.T) {
	cfg, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := eventcatalog.NewEffectiveRegistry(eventcatalog.NewCatalog(cfg), eventcatalog.DefaultSpecs())
	if err != nil {
		t.Fatal(err)
	}
	service := NewStatusService(StatusServiceOptions{
		Catalog:  eventcatalog.NewCatalog(cfg),
		Registry: registry,
		RuntimeSnapshot: func() RuntimeStatusSnapshot {
			return RuntimeStatusSnapshot{
				Profiles: map[eventcatalog.OutboxProfile]ProfileRuntimeStatus{
					eventcatalog.OutboxProfileMongoDomain: {
						Running: true, RelayEnabled: true, ReconcilerEnabled: true, ImmediateEnabled: true,
					},
					eventcatalog.OutboxProfileAssessmentMySQL: {
						Running: false, RelayEnabled: true, ReconcilerEnabled: true, ImmediateEnabled: true,
					},
				},
				Consumers: map[string]ConsumerRuntimeStatus{
					"modelcatalog.hot_rank_projection": {
						Topic: "qs.evaluation.lifecycle", Enabled: true, Healthy: false, LastError: "redis unavailable",
					},
				},
			}
		},
	})

	snapshot, err := service.GetStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := map[string]EventSummary{
		eventcatalog.QuestionnaireChanged: {
			Type: eventcatalog.QuestionnaireChanged, Owner: "survey/questionnaire", Delivery: eventcatalog.DeliveryClassBestEffort,
			Handler: "questionnaire_changed_handler", Idempotency: "published-lifecycle-post-action", Settlement: eventcatalog.SettlementHandlerErrorNack,
		},
		eventcatalog.AssessmentModelChanged: {
			Type: eventcatalog.AssessmentModelChanged, Owner: "modelcatalog", Delivery: eventcatalog.DeliveryClassBestEffort,
			Handler: "assessment_model_changed_handler", Idempotency: "published-model-post-action", Settlement: eventcatalog.SettlementHandlerErrorNack,
		},
		eventcatalog.AnswerSheetSubmitted: {
			Type: eventcatalog.AnswerSheetSubmitted, Owner: "survey/answersheet", Delivery: eventcatalog.DeliveryClassDurableOutbox,
			Profile: eventcatalog.OutboxProfileMongoDomain, Immediate: true, Priority: eventcatalog.PriorityP0,
			Handler: "answersheet_submitted_handler", Idempotency: "answersheet-id-lease-and-ensure-assessment", Settlement: eventcatalog.SettlementHandlerErrorNack,
		},
		eventcatalog.EvaluationRequested: {
			Type: eventcatalog.EvaluationRequested, Owner: "evaluation", Delivery: eventcatalog.DeliveryClassDurableOutbox,
			Profile: eventcatalog.OutboxProfileAssessmentMySQL, Immediate: true, Priority: eventcatalog.PriorityP0,
			Handler: "evaluation_requested_handler", Idempotency: "evaluation-run-state-claim", Settlement: eventcatalog.SettlementHandlerErrorNack,
		},
		eventcatalog.EvaluationOutcomeCommitted: {
			Type: eventcatalog.EvaluationOutcomeCommitted, Owner: "evaluation", Delivery: eventcatalog.DeliveryClassDurableOutbox,
			Profile: eventcatalog.OutboxProfileAssessmentMySQL, Immediate: true, Priority: eventcatalog.PriorityP1,
			Handler: "evaluation_outcome_committed_handler", Idempotency: "report-business-key-run-claim-cas", Settlement: eventcatalog.SettlementHandlerErrorNack,
		},
		eventcatalog.EvaluationFailed: {
			Type: eventcatalog.EvaluationFailed, Owner: "evaluation", Delivery: eventcatalog.DeliveryClassDurableOutbox,
			Profile: eventcatalog.OutboxProfileAssessmentMySQL, Priority: eventcatalog.PriorityP1,
			Handler: "evaluation_failed_handler", Idempotency: "report-status-overwrite", Settlement: eventcatalog.SettlementHandlerErrorNack,
		},
		eventcatalog.InterpretationReportGenerated: {
			Type: eventcatalog.InterpretationReportGenerated, Owner: "interpretation/report", Delivery: eventcatalog.DeliveryClassDurableOutbox,
			Profile: eventcatalog.OutboxProfileMongoDomain, Priority: eventcatalog.PriorityP1,
			Handler: "interpretation_report_generated_handler", Idempotency: "repeatable-attention-projection", Settlement: eventcatalog.SettlementHandlerErrorNack,
		},
		eventcatalog.InterpretationReportFailed: {
			Type: eventcatalog.InterpretationReportFailed, Owner: "interpretation/report", Delivery: eventcatalog.DeliveryClassDurableOutbox,
			Profile: eventcatalog.OutboxProfileMongoDomain, Priority: eventcatalog.PriorityP1,
			Handler: "interpretation_report_failed_handler", Idempotency: "terminal-failure-fact", Settlement: eventcatalog.SettlementHandlerErrorNack,
		},
		eventcatalog.TaskOpened: {
			Type: eventcatalog.TaskOpened, Owner: "plan", Delivery: eventcatalog.DeliveryClassBestEffort,
			Handler: "task_opened_handler", Idempotency: "notification-event-metadata", Settlement: eventcatalog.SettlementHandlerErrorNack,
		},
		eventcatalog.TaskCompleted: {
			Type: eventcatalog.TaskCompleted, Owner: "plan", Delivery: eventcatalog.DeliveryClassBestEffort,
			Handler: "task_completed_handler", Idempotency: "notification-event-metadata", Settlement: eventcatalog.SettlementHandlerErrorNack,
		},
		eventcatalog.TaskExpired: {
			Type: eventcatalog.TaskExpired, Owner: "plan", Delivery: eventcatalog.DeliveryClassBestEffort,
			Handler: "task_expired_handler", Idempotency: "notification-event-metadata", Settlement: eventcatalog.SettlementHandlerErrorNack,
		},
		eventcatalog.TaskCanceled: {
			Type: eventcatalog.TaskCanceled, Owner: "plan", Delivery: eventcatalog.DeliveryClassBestEffort,
			Handler: "task_canceled_handler", Idempotency: "notification-event-metadata", Settlement: eventcatalog.SettlementHandlerErrorNack,
		},
	}
	if len(snapshot.Events) != len(wantEvents) {
		t.Fatalf("events = %d, want %d", len(snapshot.Events), len(wantEvents))
	}
	for _, event := range snapshot.Events {
		if want, ok := wantEvents[event.Type]; !ok || event != want {
			t.Fatalf("event status %q = %#v, want %#v", event.Type, event, want)
		}
	}

	if len(snapshot.Profiles) != 2 {
		t.Fatalf("profiles = %#v, want two profiles", snapshot.Profiles)
	}
	profiles := make(map[eventcatalog.OutboxProfile]ProfileSummary, len(snapshot.Profiles))
	for _, profile := range snapshot.Profiles {
		profiles[profile.Name] = profile
	}
	mongo := profiles[eventcatalog.OutboxProfileMongoDomain]
	if mongo.EventCount != 3 || !slices.Equal(mongo.ImmediateEventTypes, []string{eventcatalog.AnswerSheetSubmitted}) || !mongo.Running || !mongo.RelayEnabled || !mongo.ReconcilerEnabled || !mongo.ImmediateEnabled {
		t.Fatalf("mongo profile = %#v", mongo)
	}
	mysql := profiles[eventcatalog.OutboxProfileAssessmentMySQL]
	if mysql.EventCount != 3 || !slices.Equal(mysql.ImmediateEventTypes, []string{eventcatalog.EvaluationOutcomeCommitted, eventcatalog.EvaluationRequested}) || mysql.Running || !mysql.RelayEnabled || !mysql.ReconcilerEnabled || !mysql.ImmediateEnabled {
		t.Fatalf("assessment profile = %#v", mysql)
	}

	if len(snapshot.Consumers) != 1 {
		t.Fatalf("consumers = %#v", snapshot.Consumers)
	}
	consumer := snapshot.Consumers[0]
	if consumer.ID != "modelcatalog.hot_rank_projection" || consumer.EventType != eventcatalog.AnswerSheetSubmitted || consumer.Runtime != "apiserver" ||
		consumer.Topic != "qs.evaluation.lifecycle" || consumer.Channel != "qs-apiserver-modelcatalog-hot-rank-v1" || !consumer.Enabled || consumer.Healthy ||
		consumer.LastError != "redis unavailable" || consumer.Settlement != eventcatalog.SettlementHandlerErrorNack {
		t.Fatalf("hot-rank consumer = %#v", consumer)
	}
}
