package evaluation

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

func mustUint64FromMetaID(field string, value meta.ID) uint64 {
	converted, err := safeconv.MetaIDToUint64(value)
	if err != nil {
		panic(fmt.Errorf("%s: %w", field, err))
	}
	return converted
}

func mustMetaIDFromUint64(field string, value uint64) meta.ID {
	id, err := safeconv.Uint64ToMetaID(value)
	if err != nil {
		panic(fmt.Errorf("%s: %w", field, err))
	}
	return id
}

func mustAssessmentIDFromUint64(field string, value uint64) assessment.ID {
	return assessment.ID(mustMetaIDFromUint64(field, value))
}

func mustTesteeIDFromUint64(field string, value uint64) testee.ID {
	return testee.ID(mustMetaIDFromUint64(field, value))
}
