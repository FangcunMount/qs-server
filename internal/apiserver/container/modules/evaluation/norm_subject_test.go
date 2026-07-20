package evaluation

import (
	"context"
	"testing"
	"time"

	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
)

type stubTesteeQuery struct {
	result *testeeApp.TesteeResult
	err    error
}

func (s stubTesteeQuery) GetByID(context.Context, uint64) (*testeeApp.TesteeResult, error) {
	return s.result, s.err
}

func (stubTesteeQuery) FindByProfile(context.Context, int64, uint64) (*testeeApp.TesteeResult, error) {
	return nil, nil
}
func (stubTesteeQuery) ListTestees(context.Context, testeeApp.ListTesteeDTO) (*testeeApp.TesteeListResult, error) {
	return nil, nil
}
func (stubTesteeQuery) ListKeyFocus(context.Context, int64, int, int) (*testeeApp.TesteeListResult, error) {
	return nil, nil
}
func (stubTesteeQuery) ListByProfileIDs(context.Context, []uint64, int, int) (*testeeApp.TesteeListResult, error) {
	return nil, nil
}

func TestNormSubjectReaderMapsGenderAndBirthday(t *testing.T) {
	birthday := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
	reader := NewNormSubjectReader(stubTesteeQuery{result: &testeeApp.TesteeResult{
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
	reader := NewNormSubjectReader(stubTesteeQuery{result: &testeeApp.TesteeResult{Gender: 0}})
	facts, err := reader.ReadNormSubjectFacts(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Gender != "" {
		t.Fatalf("gender = %q, want empty", facts.Gender)
	}
}
