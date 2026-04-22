package operator

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

func operatorIDFromUint64(field string, value uint64) (domain.ID, error) {
	id, err := safeconv.Uint64ToMetaID(value)
	if err != nil {
		return 0, errors.WithCode(code.ErrInvalidArgument, "%s exceeds int64", field)
	}
	return domain.ID(id), nil
}
