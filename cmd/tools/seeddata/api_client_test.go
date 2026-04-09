package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/FangcunMount/component-base/pkg/log"
)

func TestSeedTokenProviderRefreshesOnceForConcurrentClients(t *testing.T) {
	var refreshCalls atomic.Int32
	provider := newSeedTokenProvider("expired-token", func(ctx context.Context) (string, error) {
		refreshCalls.Add(1)
		return "fresh-token", nil
	})

	logger := log.L(context.Background())
	apiClient := NewAPIClient("https://qs.example.com", "expired-token", logger)
	collectionClient := NewAPIClient("https://collect.example.com", "expired-token", logger)
	apiClient.SetTokenProvider(provider)
	collectionClient.SetTokenProvider(provider)

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for _, client := range []*APIClient{apiClient, collectionClient} {
		wg.Add(1)
		go func(client *APIClient) {
			defer wg.Done()
			if err := client.refreshToken(context.Background()); err != nil {
				errCh <- err
			}
		}(client)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("unexpected refresh error: %v", err)
		}
	}

	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("expected one shared refresh call, got %d", got)
	}
	if got := apiClient.getToken(); got != "fresh-token" {
		t.Fatalf("expected api client token to update, got %q", got)
	}
	if got := collectionClient.getToken(); got != "fresh-token" {
		t.Fatalf("expected collection client token to update, got %q", got)
	}
}
