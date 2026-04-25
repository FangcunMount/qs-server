package service

import (
	"math"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRequestInt64FromUint64RejectsOverflow(t *testing.T) {
	_, err := requestInt64FromUint64("org_id", uint64(math.MaxInt64)+1)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %s", status.Code(err))
	}
}

func TestProtoInt32FromIntRejectsOverflow(t *testing.T) {
	_, err := protoInt32FromInt("total", math.MaxInt32+1)
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected Internal, got %s", status.Code(err))
	}
}
