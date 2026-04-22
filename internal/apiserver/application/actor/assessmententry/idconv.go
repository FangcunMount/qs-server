package assessmententry

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	domainAssessmentEntry "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/assessmententry"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

func assessmentEntryIDFromUint64(field string, value uint64) (domainAssessmentEntry.ID, error) {
	id, err := safeconv.Uint64ToMetaID(value)
	if err != nil {
		return 0, errors.WithCode(code.ErrInvalidArgument, "%s exceeds int64", field)
	}
	return domainAssessmentEntry.ID(id), nil
}

func clinicianIDFromUint64(field string, value uint64) (domainClinician.ID, error) {
	id, err := safeconv.Uint64ToMetaID(value)
	if err != nil {
		return 0, errors.WithCode(code.ErrInvalidArgument, "%s exceeds int64", field)
	}
	return domainClinician.ID(id), nil
}
