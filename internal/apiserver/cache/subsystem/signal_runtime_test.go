package cachebootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestSignalOptionsRedisOptions(t *testing.T) {
	defaults := (SignalOptions{}).redisOptions()
	if defaults.Prefix != "qs:signal" || defaults.BufferSize != 100 || defaults.Channel != "" {
		t.Fatalf("default Redis options = %+v", defaults)
	}

	overrides := (SignalOptions{Prefix: "custom", Channel: "cache-events", BufferSize: 7}).redisOptions()
	if overrides.Prefix != "custom" || overrides.Channel != "cache-events" || overrides.BufferSize != 7 {
		t.Fatalf("overridden Redis options = %+v", overrides)
	}
}

func TestDisabledSignalRuntimeDoesNotCreateSignallers(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	runtime := newSignalRuntime(client, SignalOptions{Enabled: false}, "apiserver")
	if runtime == nil {
		t.Fatal("disabled signal runtime = nil, want no-op runtime")
	}
	if runtime.questionnaire != nil || runtime.scale != nil || runtime.typology != nil {
		t.Fatal("disabled signal runtime created Redis signallers")
	}

	// The business-facing port remains best-effort and safe when disabled.
	runtime.NotifyScaleCacheChanged(context.Background(), "scale-1", "published")
}

func TestSignalRuntimePublishesConfiguredChannelBestEffort(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	subscriber := client.Subscribe(ctx, "cache-events")
	t.Cleanup(func() { _ = subscriber.Close() })
	if _, err := subscriber.Receive(ctx); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	runtime := newSignalRuntime(client, SignalOptions{Enabled: true, Channel: "cache-events"}, "apiserver")
	runtime.NotifyQuestionnaireCacheChanged(ctx, "q-1", "v2", "published")

	select {
	case message := <-subscriber.Channel():
		if message == nil || message.Channel != "cache-events" {
			t.Fatalf("published message = %#v", message)
		}
	case <-ctx.Done():
		t.Fatal("cache signal was not published")
	}

	// A transport failure is intentionally not returned to the caller.
	_ = client.Close()
	runtime.NotifyQuestionnaireCacheChanged(context.Background(), "q-2", "v3", "published")
}
