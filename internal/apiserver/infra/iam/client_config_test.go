package iam

import (
	"testing"
	"time"
)

func TestBuildSDKConfigMapsKeepaliveExplicitly(t *testing.T) {
	t.Parallel()

	config := buildSDKConfig(&IAMOptions{
		EnableTracing: true,
		GRPC: &GRPCOptions{
			Address:                      "iam:9090",
			Timeout:                      5 * time.Second,
			RetryMax:                     3,
			KeepaliveTime:                5 * time.Minute,
			KeepaliveTimeout:             20 * time.Second,
			KeepalivePermitWithoutStream: false,
		},
	})

	if config.Keepalive == nil {
		t.Fatal("Keepalive = nil")
	}
	if config.Keepalive.Time != 5*time.Minute || config.Keepalive.Timeout != 20*time.Second || config.Keepalive.PermitWithoutStream {
		t.Fatalf("Keepalive = %#v", config.Keepalive)
	}
}
