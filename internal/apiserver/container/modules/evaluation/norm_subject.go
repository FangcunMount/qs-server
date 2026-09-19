package evaluation

import (
	"context"
	"fmt"

	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// normTesteeReader is the internal Actor fact port used after an assessment has
// been loaded for execution. It must not depend on an operator authorization snapshot.
type normTesteeReader interface {
	GetTestee(context.Context, uint64) (*actorreadmodel.TesteeRow, error)
}

// NewNormSubjectReader adapts internal Actor facts for trusted scoring. Public
// testee queries retain their own authorization; this adapter is not a transport entry.
func NewNormSubjectReader(query normTesteeReader) evaluationinput.NormSubjectReader {
	if query == nil {
		return nil
	}
	return testeeNormSubjectReader{query: query}
}

type testeeNormSubjectReader struct {
	query normTesteeReader
}

func (r testeeNormSubjectReader) ReadNormSubjectFacts(ctx context.Context, testeeID uint64) (*evaluationinput.NormSubjectFacts, error) {
	if testeeID == 0 {
		return &evaluationinput.NormSubjectFacts{}, nil
	}
	result, err := r.query.GetTestee(ctx, testeeID)
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
