package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func TestSeedTokenProviderProactivelyRefreshesNearExpiryOnce(t *testing.T) {
	expiringToken := mustMakeSeedTokenForTest(t, time.Now().Add(30*time.Second))
	freshToken := mustMakeSeedTokenForTest(t, time.Now().Add(15*time.Minute))

	var refreshCalls atomic.Int32
	provider := newSeedTokenProvider(expiringToken, func(ctx context.Context) (string, error) {
		refreshCalls.Add(1)
		return freshToken, nil
	})

	logger := log.L(context.Background())
	apiClient := NewAPIClient("https://qs.example.com", expiringToken, logger)
	collectionClient := NewAPIClient("https://collect.example.com", expiringToken, logger)
	apiClient.SetTokenProvider(provider)
	collectionClient.SetTokenProvider(provider)

	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 4; i++ {
		for _, client := range []*APIClient{apiClient, collectionClient} {
			wg.Add(1)
			go func(client *APIClient) {
				defer wg.Done()
				if err := client.ensureFreshToken(context.Background()); err != nil {
					errCh <- err
				}
			}(client)
		}
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("unexpected proactive refresh error: %v", err)
		}
	}

	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("expected one proactive refresh call, got %d", got)
	}
	if got := provider.Token(); got != freshToken {
		t.Fatalf("expected provider token to update, got %q", got)
	}
}

func TestSeedTokenProviderSkipsProactiveRefreshForFreshToken(t *testing.T) {
	freshToken := mustMakeSeedTokenForTest(t, time.Now().Add(15*time.Minute))

	var refreshCalls atomic.Int32
	provider := newSeedTokenProvider(freshToken, func(ctx context.Context) (string, error) {
		refreshCalls.Add(1)
		return mustMakeSeedTokenForTest(t, time.Now().Add(30*time.Minute)), nil
	})

	logger := log.L(context.Background())
	apiClient := NewAPIClient("https://qs.example.com", freshToken, logger)
	apiClient.SetTokenProvider(provider)

	if err := apiClient.ensureFreshToken(context.Background()); err != nil {
		t.Fatalf("unexpected proactive refresh error: %v", err)
	}
	if got := refreshCalls.Load(); got != 0 {
		t.Fatalf("expected no proactive refresh call, got %d", got)
	}
}

func TestSeedTokenProviderFailsWhenExpiredTokenCannotRefresh(t *testing.T) {
	expiredToken := mustMakeSeedTokenForTest(t, time.Now().Add(-time.Minute))

	provider := newSeedTokenProvider(expiredToken, func(ctx context.Context) (string, error) {
		return "", errors.New("iam unavailable")
	})

	logger := log.L(context.Background())
	apiClient := NewAPIClient("https://qs.example.com", expiredToken, logger)
	apiClient.SetTokenProvider(provider)

	err := apiClient.ensureFreshToken(context.Background())
	if err == nil {
		t.Fatal("expected proactive refresh error for expired token")
	}
	if got, want := err.Error(), "refresh api token before request"; got == "" || !containsString(got, want) {
		t.Fatalf("expected error to contain %q, got %q", want, got)
	}
}

func mustMakeSeedTokenForTest(t *testing.T, exp time.Time) string {
	t.Helper()

	headerBytes, err := json.Marshal(map[string]interface{}{
		"alg": "RS256",
		"typ": "JWT",
	})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	payloadBytes, err := json.Marshal(map[string]interface{}{
		"sub":        "110001",
		"user_id":    "110001",
		"account_id": "613486856213901870",
		"tenant_id":  "1",
		"exp":        exp.Unix(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	return base64.RawURLEncoding.EncodeToString(headerBytes) + "." +
		base64.RawURLEncoding.EncodeToString(payloadBytes) + ".sig"
}

func containsString(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && stringContains(haystack, needle))
}

func stringContains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
