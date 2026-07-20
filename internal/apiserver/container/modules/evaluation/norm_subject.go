package evaluation

import (
	"context"
	"fmt"

	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// NewNormSubjectReader adapts Actor TesteeQueryService into evaluation input demographics.
func NewNormSubjectReader(query testeeApp.TesteeQueryService) evaluationinput.NormSubjectReader {
	if query == nil {
		return nil
	}
	return testeeNormSubjectReader{query: query}
}

type testeeNormSubjectReader struct {
	query testeeApp.TesteeQueryService
}

func (r testeeNormSubjectReader) ReadNormSubjectFacts(ctx context.Context, testeeID uint64) (*evaluationinput.NormSubjectFacts, error) {
	if testeeID == 0 {
		return &evaluationinput.NormSubjectFacts{}, nil
	}
	result, err := r.query.GetByID(ctx, testeeID)
	if err != nil {
		return nil, fmt.Errorf("load testee demographics for norm subject: %w", err)
	}
	if result == nil {
		return &evaluationinput.NormSubjectFacts{}, nil
	}
	facts := &evaluationinput.NormSubjectFacts{Birthday: result.Birthday}
	switch domainTestee.Gender(result.Gender) {
	case domainTestee.GenderMale, domainTestee.GenderFemale:
		facts.Gender = domainTestee.Gender(result.Gender).String()
	}
	return facts, nil
}
