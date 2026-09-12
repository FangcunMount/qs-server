package access

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

type service struct {
	operatorReader actorreadmodel.OperatorReader
	testeeReader   actorreadmodel.TesteeReader
}

// NewTesteeAccessService checks backstage action scopes against current ownership.
func NewTesteeAccessService(operators actorreadmodel.OperatorReader, testees actorreadmodel.TesteeReader) TesteeAccessService {
	return &service{operatorReader: operators, testeeReader: testees}
}
func ensureAccessIDFromUint64(field string, value uint64) error {
	_, err := safeconv.Uint64ToMetaID(value)
	if err != nil {
		return errors.WithCode(code.ErrInvalidArgument, "%s exceeds int64", field)
	}
	return nil
}
