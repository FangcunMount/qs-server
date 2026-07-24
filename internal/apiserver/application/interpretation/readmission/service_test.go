package readmission

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/admission"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type failureRepoStub struct{ item *admission.Failure }

func (s failureRepoStub) UpsertByFingerprint(context.Context, *admission.Failure) (bool, error) {
	return false, nil
}
func (s failureRepoStub) FindByFingerprint(context.Context, string) (*admission.Failure, error) {
	return s.item, nil
}
func (s failureRepoStub) FindByOutcomeID(context.Context, meta.ID, int) ([]*admission.Failure, error) {
	return nil, nil
}

type outcomeRepoStub struct{ item *evaluationfact.Record }

func (s outcomeRepoStub) FindByID(context.Context, meta.ID) (*evaluationfact.Record, error) {
	return s.item, nil
}
func (s outcomeRepoStub) FindByAssessmentID(context.Context, meta.ID) (*evaluationfact.Record, error) {
	return s.item, nil
}

type automationStub struct{ calls int }

func (s *automationStub) Generate(context.Context, automation.GenerateCommand) (*automation.Result, error) {
	s.calls++
	return &automation.Result{Status: automation.StatusGenerated, GenerationID: meta.FromUint64(20)}, nil
}

func TestReadmitReloadsCommittedOutcomeAndChecksExpectedVersion(t *testing.T) {
	t.Parallel()
	at := time.Now().UTC()
	failure, err := admission.NewFailure(admission.Input{
		ID: meta.FromUint64(1), OutcomeID: meta.FromUint64(2), OrgID: 3,
		AssessmentID: meta.FromUint64(4), TesteeID: 5, Kind: admission.KindOutcomeIncomplete,
		Code: "outcome_incomplete", SafeMessage: "invalid", OccurredAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	outcome := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: meta.FromUint64(2), OrgID: 3, AssessmentID: meta.FromUint64(4), TesteeID: 5,
		SchemaVersion: 2, Payload: []byte(`{}`), ReportInput: []byte(`{}`),
	})
	automationService := &automationStub{}
	service := NewService(failureRepoStub{item: failure}, outcomeRepoStub{item: outcome}, automationService)
	result, err := service.Readmit(context.Background(), Command{
		OrgID: 3, OperatorUserID: 6, FailureFingerprint: failure.Fingerprint(),
		ExpectedReason: failure.Kind(), ExpectedOutcomeVersion: outcome.VersionToken(),
		Reason: "configuration repaired", RequestID: "req-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.GenerationID != 20 || automationService.calls != 1 {
		t.Fatalf("result=%#v calls=%d", result, automationService.calls)
	}
}
