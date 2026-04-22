package clinician

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainOperator "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

func clinicianIDFromUint64(field string, value uint64) (domainClinician.ID, error) {
	id, err := safeconv.Uint64ToMetaID(value)
	if err != nil {
		return 0, errors.WithCode(code.ErrInvalidArgument, "%s exceeds int64", field)
	}
	return domainClinician.ID(id), nil
}

func operatorIDFromUint64(field string, value uint64) (domainOperator.ID, error) {
	id, err := safeconv.Uint64ToMetaID(value)
	if err != nil {
		return 0, errors.WithCode(code.ErrInvalidArgument, "%s exceeds int64", field)
	}
	return domainOperator.ID(id), nil
}

func relationIDFromUint64(field string, value uint64) (domainRelation.ID, error) {
	id, err := safeconv.Uint64ToMetaID(value)
	if err != nil {
		return 0, errors.WithCode(code.ErrInvalidArgument, "%s exceeds int64", field)
	}
	return domainRelation.ID(id), nil
}

func testeeIDFromUint64(field string, value uint64) (domainTestee.ID, error) {
	id, err := safeconv.Uint64ToMetaID(value)
	if err != nil {
		return 0, errors.WithCode(code.ErrInvalidArgument, "%s exceeds int64", field)
	}
	return domainTestee.ID(id), nil
}
