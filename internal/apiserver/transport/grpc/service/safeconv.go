package service

import (
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func requestInt64FromUint64(field string, value uint64) (int64, error) {
	converted, err := safeconv.Uint64ToInt64(value)
	if err != nil {
		return 0, status.Errorf(codes.InvalidArgument, "%s 超出 int64 范围", field)
	}
	return converted, nil
}

func requestInt8FromInt32(field string, value int32) (int8, error) {
	converted, err := safeconv.Int32ToInt8(value)
	if err != nil {
		return 0, status.Errorf(codes.InvalidArgument, "%s 超出 int8 范围", field)
	}
	return converted, nil
}

func protoUint64FromInt64(field string, value int64) (uint64, error) {
	converted, err := safeconv.Int64ToUint64(value)
	if err != nil {
		return 0, status.Errorf(codes.Internal, "%s 超出 uint64 范围", field)
	}
	return converted, nil
}

func protoInt32FromInt(field string, value int) (int32, error) {
	converted, err := safeconv.IntToInt32(value)
	if err != nil {
		return 0, status.Errorf(codes.Internal, "%s 超出 int32 范围", field)
	}
	return converted, nil
}

func protoInt32Slice(field string, values []int) ([]int32, error) {
	if len(values) == 0 {
		return nil, nil
	}

	items := make([]int32, 0, len(values))
	for _, value := range values {
		converted, err := protoInt32FromInt(field, value)
		if err != nil {
			return nil, err
		}
		items = append(items, converted)
	}
	return items, nil
}
