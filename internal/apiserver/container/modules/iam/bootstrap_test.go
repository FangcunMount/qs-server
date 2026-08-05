package iam

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/options"
)

func TestConvertIAMOptionsMapsKeepalive(t *testing.T) {
	t.Parallel()

	opts := options.NewIAMOptions()
	converted := convertIAMOptions(opts)
	if converted.GRPC == nil {
		t.Fatal("GRPC = nil")
	}
	if converted.GRPC.KeepaliveTime != 5*time.Minute || converted.GRPC.KeepaliveTimeout != 20*time.Second || converted.GRPC.KeepalivePermitWithoutStream {
		t.Fatalf("converted keepalive = %#v", converted.GRPC)
	}
}
