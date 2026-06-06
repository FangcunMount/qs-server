package assessment

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcodec"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type footprintEventStager struct {
	events []event.DomainEvent
}

func (s *footprintEventStager) Stage(_ context.Context, events ...event.DomainEvent) error {
	s.events = append(s.events, events...)
	return nil
}

type idAssigningRepoStub struct{}

func (idAssigningRepoStub) Save(_ context.Context, a *domainAssessment.Assessment) error {
	if a == nil || !a.ID().IsZero() {
		return nil
	}
	a.AssignID(domainAssessment.NewID(622776822962598446))
	return nil
}

func (idAssigningRepoStub) FindByID(context.Context, domainAssessment.ID) (*domainAssessment.Assessment, error) {
	return nil, nil
}
func (idAssigningRepoStub) FindByAnswerSheetID(context.Context, domainAssessment.AnswerSheetRef) (*domainAssessment.Assessment, error) {
	return nil, nil
}
func (idAssigningRepoStub) Delete(context.Context, domainAssessment.ID) error { return nil }

type passthroughTxRunner struct{}

func (passthroughTxRunner) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestSaveAssessmentAndStageEventsBuildsAdditionalEventsAfterPersistedID(t *testing.T) {
	stager := &footprintEventStager{}
	a, err := domainAssessment.NewAssessment(
		1,
		testee.NewID(618855887087350318),
		domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("3adyDE"), "7.0.1"),
		domainAssessment.NewAnswerSheetRef(meta.FromUint64(622776820663595566)),
		domainAssessment.NewAdhocOrigin(),
	)
	if err != nil {
		t.Fatalf("NewAssessment() error = %v", err)
	}

	occurredAt := time.Date(2026, 6, 6, 16, 11, 53, 0, time.UTC)
	err = saveAssessmentAndStageEvents(
		context.Background(),
		idAssigningRepoStub{},
		passthroughTxRunner{},
		stager,
		a,
		func(saved *domainAssessment.Assessment) []event.DomainEvent {
			return []event.DomainEvent{
				domainStatistics.NewFootprintAssessmentCreatedEvent(
					1,
					618855887087350318,
					622776820663595566,
					saved.ID().Uint64(),
					occurredAt,
				),
			}
		},
	)
	if err != nil {
		t.Fatalf("saveAssessmentAndStageEvents() error = %v", err)
	}
	if len(stager.events) != 1 {
		t.Fatalf("staged events = %d, want 1", len(stager.events))
	}

	payload, err := eventcodec.EncodeDomainEvent(stager.events[0])
	if err != nil {
		t.Fatalf("EncodeDomainEvent() error = %v", err)
	}
	env, err := eventcodec.DecodeEnvelope(payload)
	if err != nil {
		t.Fatalf("DecodeEnvelope() error = %v", err)
	}
	var data domainStatistics.FootprintAssessmentCreatedData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("Unmarshal footprint data: %v", err)
	}
	if data.AssessmentID != 622776822962598446 {
		t.Fatalf("footprint assessment_id = %d, want persisted assessment id", data.AssessmentID)
	}
}

var _ domainAssessment.Repository = idAssigningRepoStub{}
var _ apptransaction.Runner = passthroughTxRunner{}
