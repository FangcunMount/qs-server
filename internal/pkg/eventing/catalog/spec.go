package eventcatalog

import (
	"fmt"
	"sort"
	"strings"
)

// OutboxProfile identifies one durable local-transaction outbox runtime.
type OutboxProfile string

const (
	OutboxProfileNone            OutboxProfile = ""
	OutboxProfileMongoDomain     OutboxProfile = "mongo_domain_events"
	OutboxProfileAssessmentMySQL OutboxProfile = "assessment_mysql_events"
)

// Priority controls claim order only; it never changes delivery guarantees.
type Priority string

const (
	PriorityNone Priority = ""
	PriorityP0   Priority = "p0"
	PriorityP1   Priority = "p1"
	PriorityP2   Priority = "p2"
)

var readyIndexBuckets = []string{string(PriorityP0), string(PriorityP1), string(PriorityP2)}

// SettlementPolicy describes how a transport settles a returned handler error.
type SettlementPolicy string

const SettlementHandlerErrorNack SettlementPolicy = "handler_error_nack"

// ConsumerSpec declares an additional consumer of an event. The primary
// worker handler remains part of configs/events.yaml.
type ConsumerSpec struct {
	ID                string
	Runtime           string
	Channel           string
	IdempotencyPolicy string
	SettlementPolicy  SettlementPolicy
}

// EventSpec contains qs-server engineering policy that is intentionally not a
// part of the shared wire catalog.
type EventSpec struct {
	Type                string
	Owner               string
	OutboxProfile       OutboxProfile
	Immediate           bool
	Priority            Priority
	IdempotencyPolicy   string
	SettlementPolicy    SettlementPolicy
	AdditionalConsumers []ConsumerSpec
}

// EffectiveEvent is the immutable merged view consumed by runtime code and
// governance endpoints.
type EffectiveEvent struct {
	Type                string
	Owner               string
	Topic               string
	Delivery            DeliveryClass
	PrimaryHandler      string
	OutboxProfile       OutboxProfile
	Immediate           bool
	Priority            Priority
	IdempotencyPolicy   string
	SettlementPolicy    SettlementPolicy
	AdditionalConsumers []ConsumerSpec
}

// EffectiveRegistry is the single read-only event policy view for a process.
type EffectiveRegistry struct {
	events map[string]EffectiveEvent
}

// DefaultSpecs returns the reviewed engineering policy for every wire event.
func DefaultSpecs() []EventSpec {
	return []EventSpec{
		bestEffortSpec(QuestionnaireChanged, "survey/questionnaire", "published-lifecycle-post-action"),
		bestEffortSpec(AssessmentModelChanged, "modelcatalog", "published-model-post-action"),
		{
			Type:              AnswerSheetSubmitted,
			Owner:             "survey/answersheet",
			OutboxProfile:     OutboxProfileMongoDomain,
			Immediate:         true,
			Priority:          PriorityP0,
			IdempotencyPolicy: "answersheet-id-lease-and-ensure-assessment",
			SettlementPolicy:  SettlementHandlerErrorNack,
			AdditionalConsumers: []ConsumerSpec{{
				ID:                "modelcatalog.hot_rank_projection",
				Runtime:           "apiserver",
				Channel:           "qs-apiserver-modelcatalog-hot-rank-v1",
				IdempotencyPolicy: "redis-processed-key-by-event-id",
				SettlementPolicy:  SettlementHandlerErrorNack,
			}},
		},
		durableSpec(EvaluationRequested, "evaluation", OutboxProfileAssessmentMySQL, true, PriorityP0, "evaluation-run-state-claim"),
		durableSpec(EvaluationOutcomeCommitted, "evaluation", OutboxProfileAssessmentMySQL, true, PriorityP1, "report-business-key-run-claim-cas"),
		durableSpec(EvaluationFailed, "evaluation", OutboxProfileAssessmentMySQL, false, PriorityP1, "report-status-overwrite"),
		durableSpec(InterpretationReportGenerated, "interpretation/report", OutboxProfileMongoDomain, false, PriorityP1, "repeatable-attention-projection"),
		durableSpec(InterpretationReportFailed, "interpretation/report", OutboxProfileMongoDomain, false, PriorityP1, "terminal-failure-fact"),
		bestEffortSpec(TaskOpened, "plan", "notification-event-metadata"),
		bestEffortSpec(TaskCompleted, "plan", "notification-event-metadata"),
		bestEffortSpec(TaskExpired, "plan", "notification-event-metadata"),
		bestEffortSpec(TaskCanceled, "plan", "notification-event-metadata"),
	}
}

func bestEffortSpec(eventType, owner, idempotency string) EventSpec {
	return EventSpec{Type: eventType, Owner: owner, IdempotencyPolicy: idempotency, SettlementPolicy: SettlementHandlerErrorNack}
}

func durableSpec(eventType, owner string, profile OutboxProfile, immediate bool, priority Priority, idempotency string) EventSpec {
	return EventSpec{
		Type: eventType, Owner: owner, OutboxProfile: profile, Immediate: immediate,
		Priority: priority, IdempotencyPolicy: idempotency, SettlementPolicy: SettlementHandlerErrorNack,
	}
}

// NewEffectiveRegistry validates and merges the wire catalog with engineering specs.
func NewEffectiveRegistry(catalog *Catalog, specs []EventSpec) (*EffectiveRegistry, error) {
	if catalog == nil || catalog.Config() == nil {
		return nil, fmt.Errorf("event catalog is not loaded")
	}
	byType := make(map[string]EventSpec, len(specs))
	consumerIDs := make(map[string]string)
	for _, spec := range specs {
		if err := validateSpec(spec); err != nil {
			return nil, err
		}
		if _, exists := byType[spec.Type]; exists {
			return nil, fmt.Errorf("duplicate event spec %q", spec.Type)
		}
		for _, consumer := range spec.AdditionalConsumers {
			if previous, exists := consumerIDs[consumer.ID]; exists {
				return nil, fmt.Errorf("consumer %q is declared by both %q and %q", consumer.ID, previous, spec.Type)
			}
			consumerIDs[consumer.ID] = spec.Type
		}
		byType[spec.Type] = copySpec(spec)
	}

	events := make(map[string]EffectiveEvent, len(catalog.Config().Events))
	for eventType, wire := range catalog.Config().Events {
		spec, ok := byType[eventType]
		if !ok {
			return nil, fmt.Errorf("event %q is missing EventSpec", eventType)
		}
		if err := validateSpecAgainstDelivery(spec, wire.Delivery); err != nil {
			return nil, err
		}
		if spec.Owner != wire.Domain {
			return nil, fmt.Errorf("event %q owner %q does not match catalog domain %q", eventType, spec.Owner, wire.Domain)
		}
		topic, ok := catalog.GetTopicForEvent(eventType)
		if !ok {
			return nil, fmt.Errorf("event %q has no resolved topic", eventType)
		}
		events[eventType] = EffectiveEvent{
			Type: eventType, Owner: spec.Owner, Topic: topic, Delivery: wire.Delivery,
			PrimaryHandler: wire.Handler, OutboxProfile: spec.OutboxProfile,
			Immediate: spec.Immediate, Priority: spec.Priority,
			IdempotencyPolicy: spec.IdempotencyPolicy, SettlementPolicy: spec.SettlementPolicy,
			AdditionalConsumers: append([]ConsumerSpec(nil), spec.AdditionalConsumers...),
		}
		delete(byType, eventType)
	}
	if len(byType) > 0 {
		extra := make([]string, 0, len(byType))
		for eventType := range byType {
			extra = append(extra, eventType)
		}
		sort.Strings(extra)
		return nil, fmt.Errorf("EventSpec types missing from catalog: %s", strings.Join(extra, ", "))
	}
	return &EffectiveRegistry{events: events}, nil
}

func validateSpec(spec EventSpec) error {
	if strings.TrimSpace(spec.Type) == "" || strings.TrimSpace(spec.Owner) == "" {
		return fmt.Errorf("event spec type and owner are required")
	}
	if strings.TrimSpace(spec.IdempotencyPolicy) == "" || spec.SettlementPolicy == "" {
		return fmt.Errorf("event %q must declare idempotency and settlement", spec.Type)
	}
	if spec.SettlementPolicy != SettlementHandlerErrorNack {
		return fmt.Errorf("event %q has unsupported settlement %q", spec.Type, spec.SettlementPolicy)
	}
	if spec.OutboxProfile != OutboxProfileNone && spec.OutboxProfile != OutboxProfileMongoDomain && spec.OutboxProfile != OutboxProfileAssessmentMySQL {
		return fmt.Errorf("event %q has unsupported outbox profile %q", spec.Type, spec.OutboxProfile)
	}
	if spec.Priority != PriorityNone && spec.Priority != PriorityP0 && spec.Priority != PriorityP1 && spec.Priority != PriorityP2 {
		return fmt.Errorf("event %q has unsupported priority %q", spec.Type, spec.Priority)
	}
	for _, consumer := range spec.AdditionalConsumers {
		if strings.TrimSpace(consumer.ID) == "" || strings.TrimSpace(consumer.Runtime) == "" || strings.TrimSpace(consumer.Channel) == "" {
			return fmt.Errorf("event %q has incomplete additional consumer", spec.Type)
		}
		if strings.TrimSpace(consumer.IdempotencyPolicy) == "" || consumer.SettlementPolicy == "" {
			return fmt.Errorf("consumer %q must declare idempotency and settlement", consumer.ID)
		}
		if consumer.SettlementPolicy != SettlementHandlerErrorNack {
			return fmt.Errorf("consumer %q has unsupported settlement %q", consumer.ID, consumer.SettlementPolicy)
		}
	}
	return nil
}

func validateSpecAgainstDelivery(spec EventSpec, delivery DeliveryClass) error {
	switch delivery {
	case DeliveryClassDurableOutbox:
		if spec.OutboxProfile == OutboxProfileNone || spec.Priority == PriorityNone {
			return fmt.Errorf("durable event %q must declare outbox profile and priority", spec.Type)
		}
	case DeliveryClassBestEffort:
		if spec.OutboxProfile != OutboxProfileNone || spec.Immediate || spec.Priority != PriorityNone {
			return fmt.Errorf("best-effort event %q cannot declare outbox profile, immediate or priority", spec.Type)
		}
	default:
		return fmt.Errorf("event %q has unsupported delivery %q", spec.Type, delivery)
	}
	return nil
}

func copySpec(spec EventSpec) EventSpec {
	spec.AdditionalConsumers = append([]ConsumerSpec(nil), spec.AdditionalConsumers...)
	return spec
}

// Lookup returns a defensive copy of one effective event.
func (r *EffectiveRegistry) Lookup(eventType string) (EffectiveEvent, bool) {
	if r == nil {
		return EffectiveEvent{}, false
	}
	evt, ok := r.events[eventType]
	evt.AdditionalConsumers = append([]ConsumerSpec(nil), evt.AdditionalConsumers...)
	return evt, ok
}

// Snapshot returns all effective events in stable event-type order.
func (r *EffectiveRegistry) Snapshot() []EffectiveEvent {
	if r == nil {
		return nil
	}
	result := make([]EffectiveEvent, 0, len(r.events))
	for eventType := range r.events {
		evt, _ := r.Lookup(eventType)
		result = append(result, evt)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Type < result[j].Type })
	return result
}

func (r *EffectiveRegistry) EventsByProfile(profile OutboxProfile) []EffectiveEvent {
	all := r.Snapshot()
	result := make([]EffectiveEvent, 0, len(all))
	for _, evt := range all {
		if evt.OutboxProfile == profile {
			result = append(result, evt)
		}
	}
	return result
}

func (r *EffectiveRegistry) ImmediateTypes(profile OutboxProfile) []string {
	var result []string
	for _, evt := range r.EventsByProfile(profile) {
		if evt.Immediate {
			result = append(result, evt.Type)
		}
	}
	return result
}

// PriorityTiers returns P0, P0+P1, then the fallback query for a profile.
func (r *EffectiveRegistry) PriorityTiers(profile OutboxProfile) [][]string {
	var p0, p1 []string
	for _, evt := range r.EventsByProfile(profile) {
		switch evt.Priority {
		case PriorityP0:
			p0 = append(p0, evt.Type)
		case PriorityP1:
			p1 = append(p1, evt.Type)
		}
	}
	sort.Strings(p0)
	sort.Strings(p1)
	combined := append(append([]string(nil), p0...), p1...)
	return [][]string{p0, combined, nil}
}

func (r *EffectiveRegistry) PriorityBucket(eventType string) string {
	if evt, ok := r.Lookup(eventType); ok && evt.Priority != PriorityNone {
		return string(evt.Priority)
	}
	return string(PriorityP2)
}

func (r *EffectiveRegistry) ReadyIndexBuckets() []string {
	return append([]string(nil), readyIndexBuckets...)
}

func (r *EffectiveRegistry) Consumers(eventType string) []ConsumerSpec {
	evt, ok := r.Lookup(eventType)
	if !ok {
		return nil
	}
	return append([]ConsumerSpec(nil), evt.AdditionalConsumers...)
}

// ValidatePrimaryHandlers verifies the worker binding registry without making
// eventcatalog depend on worker packages.
func (r *EffectiveRegistry) ValidatePrimaryHandlers(has func(string) bool) error {
	if has == nil {
		return fmt.Errorf("handler lookup is required")
	}
	for _, evt := range r.Snapshot() {
		if !has(evt.PrimaryHandler) {
			return fmt.Errorf("handler %q not registered for event %q", evt.PrimaryHandler, evt.Type)
		}
	}
	return nil
}
