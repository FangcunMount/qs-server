package evaluation

import (
	"context"
	"errors"
	"testing"
	"time"

	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
)

type stubTesteeReader struct {
	result *actorreadmodel.TesteeRow
	err    error
}

func (s stubTesteeReader) GetTestee(context.Context, uint64) (*actorreadmodel.TesteeRow, error) {
	return s.result, s.err
}

func TestNormSubjectReaderMapsGenderAndBirthday(t *testing.T) {
	birthday := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
	reader := NewNormSubjectReader(stubTesteeReader{result: &actorreadmodel.TesteeRow{
		Gender:   2,
		Birthday: &birthday,
	}})
	facts, err := reader.ReadNormSubjectFacts(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Gender != "female" || facts.Birthday == nil || !facts.Birthday.Equal(birthday) {
		t.Fatalf("facts = %#v", facts)
	}
}

func TestNormSubjectReaderTreatsUnknownGenderAsMissing(t *testing.T) {
	reader := NewNormSubjectReader(stubTesteeReader{result: &actorreadmodel.TesteeRow{Gender: 0}})
	facts, err := reader.ReadNormSubjectFacts(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Gender != "" {
		t.Fatalf("gender = %q, want empty", facts.Gender)
	}
}

type workerTesteeReader struct {
	actorreadmodel.TesteeReader
	row *actorreadmodel.TesteeRow
}

func (r workerTesteeReader) GetTestee(context.Context, uint64) (*actorreadmodel.TesteeRow, error) {
	return r.row, nil
}
func TestNormSubjectWorkerDoesNotRequireOperatorScope(t *testing.T) {
	birthday := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
	raw := workerTesteeReader{row: &actorreadmodel.TesteeRow{ID: 7, OrgID: 1, Gender: 2, Birthday: &birthday}}
	backend := testeeApp.NewQueryServiceWithAssessmentSummary(raw, nil)
	if _, err := backend.GetByID(context.Background(), 7); err == nil {
		t.Fatal("backend query must still require operator scope")
	}
	facts, err := NewNormSubjectReader(raw).ReadNormSubjectFacts(context.Background(), 7)
	if err != nil {
		t.Fatalf("trusted scoring task must read norm facts without an operator snapshot: %v", err)
	}
	if facts.Gender != "female" || facts.Birthday == nil || !facts.Birthday.Equal(birthday) {
		t.Fatalf("wrong norm facts: %#v", facts)
	}
}

func TestNormSubjectReaderPreservesDependencyFailure(t *testing.T) {
	cause := errors.New("actor store unavailable")
	_, err := NewNormSubjectReader(stubTesteeReader{err: cause}).ReadNormSubjectFacts(context.Background(), 7)
	if !errors.Is(err, cause) {
		t.Fatalf("must propagate dependency failure: %v", err)
	}
}
func TestNormSubjectReaderSkipsMissingIdentity(t *testing.T) {
	reader := NewNormSubjectReader(stubTesteeReader{err: errors.New("must not read")})
	facts, err := reader.ReadNormSubjectFacts(context.Background(), 0)
	if err != nil || facts == nil || facts.Birthday != nil || facts.Gender != "" {
		t.Fatalf("unexpected absent identity facts: %#v, %v", facts, err)
	}
}
