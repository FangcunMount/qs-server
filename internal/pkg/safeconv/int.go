package safeconv

import (
	"fmt"
	"math"
	"strconv"
)

func Uint64ToInt64(value uint64) (int64, error) {
	if value > math.MaxInt64 {
		return 0, fmt.Errorf("%d exceeds int64", value)
	}
	return int64(value), nil
}

func Int64ToUint64(value int64) (uint64, error) {
	if value < 0 {
		return 0, fmt.Errorf("%d is negative", value)
	}
	return uint64(value), nil
}

func IntToInt32(value int) (int32, error) {
	if value < math.MinInt32 || value > math.MaxInt32 {
		return 0, fmt.Errorf("%d exceeds int32", value)
	}
	return int32(value), nil
}

func Int64ToInt32(value int64) (int32, error) {
	if value < math.MinInt32 || value > math.MaxInt32 {
		return 0, fmt.Errorf("%d exceeds int32", value)
	}
	return int32(value), nil
}

func Int32ToInt8(value int32) (int8, error) {
	if value < math.MinInt8 || value > math.MaxInt8 {
		return 0, fmt.Errorf("%d exceeds int8", value)
	}
	return int8(value), nil
}

func Int64ToInt(value int64) (int, error) {
	if strconv.IntSize == 32 && (value < math.MinInt32 || value > math.MaxInt32) {
		return 0, fmt.Errorf("%d exceeds int", value)
	}
	return int(value), nil
}
