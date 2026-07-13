package reportstatus

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestNewSignalerAcceptsUniversalClient(t *testing.T) {
	mr := miniredis.RunT(t)
	var client redis.UniversalClient = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	signaler, err := NewSignaler(client, DefaultSignalingOptions())
	if err != nil {
		t.Fatalf("NewSignaler() error = %v", err)
	}
	if signaler == nil {
		t.Fatal("NewSignaler() = nil")
	}
}

func TestNewSignalerRejectsNilClient(t *testing.T) {
	if _, err := NewSignaler(nil, DefaultSignalingOptions()); err == nil {
		t.Fatal("NewSignaler(nil) error = nil")
	}
}
