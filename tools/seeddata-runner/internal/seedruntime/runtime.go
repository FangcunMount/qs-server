package seedruntime

import (
	"context"
	"fmt"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/seedapi"
	"github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/seedconfig"
	"github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/seediauth"
)

type Dependencies struct {
	Logger           log.Logger
	Config           *seedconfig.Config
	APIClient        *seedapi.APIClient
	CollectionClient *seedapi.APIClient
}

func NewLogger(verbose bool) log.Logger {
	seedLogger := log.New(newLogOptions(verbose, false))
	log.Init(newLogOptions(verbose, true))
	return seedLogger
}

func LoadDependencies(ctx context.Context, cfg *seedconfig.Config, logger log.Logger) (*Dependencies, error) {
	if cfg == nil {
		return nil, fmt.Errorf("seeddata config is nil")
	}

	token, err := resolveAPIToken(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}
	apiBaseURL := strings.TrimSpace(cfg.API.BaseURL)
	if apiBaseURL == "" {
		return nil, fmt.Errorf("api.baseUrl is required")
	}

	apiClient := newConfiguredAPIClient(apiBaseURL, token, cfg, logger)
	logger.Infow("Initialized API client", "base_url", apiBaseURL)

	collectionURL := firstNonEmpty(cfg.API.CollectionBaseURL, apiBaseURL)
	collectionClient := newConfiguredAPIClient(collectionURL, token, cfg, logger)
	logger.Infow("Initialized collection client", "base_url", collectionURL)

	configureIAMTokenRefresh(cfg, logger, token, apiClient, collectionClient)

	return &Dependencies{
		Logger:           logger,
		Config:           cfg,
		APIClient:        apiClient,
		CollectionClient: collectionClient,
	}, nil
}

func NewSignalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
}

func SleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func ParseID(raw string) uint64 {
	value, _ := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	return value
}

func NullableString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func NormalizePlanWorkers(workers, taskCount int) int {
	if workers <= 0 {
		workers = 1
	}
	if taskCount > 0 && workers > taskCount {
		return taskCount
	}
	return workers
}

func ParseRelativeDuration(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return 0, fmt.Errorf("duration is empty")
	}

	if strings.HasSuffix(raw, "d") {
		days, err := strconv.ParseFloat(strings.TrimSuffix(raw, "d"), 64)
		if err != nil {
			return 0, err
		}
		return time.Duration(days * float64(24*time.Hour)), nil
	}
	if strings.HasSuffix(raw, "w") {
		weeks, err := strconv.ParseFloat(strings.TrimSuffix(raw, "w"), 64)
		if err != nil {
			return 0, err
		}
		return time.Duration(weeks * float64(7*24*time.Hour)), nil
	}
	return time.ParseDuration(raw)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func resolveAPIToken(ctx context.Context, cfg *seedconfig.Config, logger log.Logger) (string, error) {
	token := strings.TrimSpace(cfg.API.Token)
	if token != "" {
		return token, nil
	}
	if cfg.IAM == (seedconfig.IAMConfig{}) {
		return "", fmt.Errorf("api.token is required when iam config is not set")
	}

	logger.Infow("Fetching API token from IAM", "login_url", cfg.IAM.LoginURL, "username", cfg.IAM.Username)
	token, err := seediauth.FetchTokenFromIAM(ctx, seediauth.Config{
		BaseURL:  cfg.IAM.BaseURL,
		LoginURL: cfg.IAM.LoginURL,
		Username: cfg.IAM.Username,
		Password: cfg.IAM.Password,
		TenantID: cfg.IAM.TenantID,
	}, logger)
	if err != nil {
		return "", fmt.Errorf("fetch token from iam: %w", err)
	}
	return token, nil
}

func newConfiguredAPIClient(baseURL, token string, cfg *seedconfig.Config, logger log.Logger) *seedapi.APIClient {
	client := seedapi.NewAPIClient(baseURL, token, logger)
	client.SetRetryConfig(cfg.API.Retry)
	return client
}

func configureIAMTokenRefresh(cfg *seedconfig.Config, logger log.Logger, token string, clients ...*seedapi.APIClient) {
	if cfg == nil || cfg.IAM == (seedconfig.IAMConfig{}) {
		return
	}

	refresher := func(ctx context.Context) (string, error) {
		return seediauth.FetchTokenFromIAM(ctx, seediauth.Config{
			BaseURL:  cfg.IAM.BaseURL,
			LoginURL: cfg.IAM.LoginURL,
			Username: cfg.IAM.Username,
			Password: cfg.IAM.Password,
			TenantID: cfg.IAM.TenantID,
		}, logger)
	}

	tokenProvider := seedapi.NewTokenProvider(token, refresher)
	for _, client := range clients {
		client.SetTokenProvider(tokenProvider)
		client.SetTokenRefresher(refresher)
	}
}

func newLogOptions(verbose bool, quiet bool) *log.Options {
	opts := log.NewOptions()
	switch {
	case verbose:
		opts.Level = "debug"
	case quiet:
		opts.Level = "warn"
	default:
		opts.Level = "info"
	}
	return opts
}
