package answersheet

import (
	"fmt"

	"github.com/FangcunMount/component-base/pkg/errors"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

func answerSheetIDFromUint64(field string, value uint64) (meta.ID, error) {
	id, err := safeconv.Uint64ToMetaID(value)
	if err != nil {
		return 0, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "%s exceeds int64", field)
	}
	return id, nil
}

func fillerUserIDFromUint64(field string, value uint64) (int64, error) {
	userID, err := safeconv.Uint64ToInt64(value)
	if err != nil {
		return 0, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "%s exceeds int64", field)
	}
	return userID, nil
}

func mustUint64FromInt64(field string, value int64) uint64 {
	converted, err := safeconv.Int64ToUint64(value)
	if err != nil {
		panic(fmt.Errorf("%s: %w", field, err))
	}
	return converted
}
