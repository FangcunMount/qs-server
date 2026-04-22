package safeconv

import (
	"fmt"
	"math"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func Uint64ToMetaID(value uint64) (meta.ID, error) {
	if value > math.MaxInt64 {
		return 0, fmt.Errorf("%d exceeds meta.ID", value)
	}
	return meta.ID(int64(value)), nil
}

func Int64ToMetaID(value int64) (meta.ID, error) {
	if value < 0 {
		return 0, fmt.Errorf("%d is negative", value)
	}
	return meta.ID(value), nil
}

func MetaIDToUint64(value meta.ID) (uint64, error) {
	if value.Int64() < 0 {
		return 0, fmt.Errorf("%d is negative", value.Int64())
	}
	return value.Uint64(), nil
}
