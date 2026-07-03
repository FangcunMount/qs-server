package component

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

// ResilienceResult holds one component resilience payload with fetch metadata.
type ResilienceResult struct {
	Available bool                             `json:"available"`
	Reason    string                           `json:"reason,omitempty"`
	Snapshot  *resilienceplane.RuntimeSnapshot `json:"snapshot,omitempty"`
}

// CacheResult holds one component cache/redis payload with fetch metadata.
type CacheResult struct {
	Available bool                           `json:"available"`
	Reason    string                         `json:"reason,omitempty"`
	Snapshot  *observability.RuntimeSnapshot `json:"snapshot,omitempty"`
}

// Adapter fetches remote component governance snapshots.
type Adapter struct {
	components map[string]*options.GovernanceComponentOptions
	http       *http.Client
}

// NewAdapter builds a component governance adapter.
func NewAdapter(opts map[string]*options.GovernanceComponentOptions) *Adapter {
	if len(opts) == 0 {
		return &Adapter{components: map[string]*options.GovernanceComponentOptions{}}
	}
	cloned := make(map[string]*options.GovernanceComponentOptions, len(opts))
	for name, cfg := range opts {
		if cfg == nil {
			continue
		}
		copyCfg := *cfg
		cloned[name] = &copyCfg
	}
	return &Adapter{
		components: cloned,
		http: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

// FetchResilience loads resilience snapshots for configured components.
func (a *Adapter) FetchResilience(ctx context.Context) map[string]ResilienceResult {
	result := make(map[string]ResilienceResult)
	if a == nil {
		return result
	}
	for name, cfg := range a.components {
		if cfg == nil || strings.TrimSpace(cfg.ResilienceURL) == "" {
			result[name] = ResilienceResult{
				Available: false,
				Reason:    "resilience_url not configured",
			}
			continue
		}
		snapshot, err := a.fetchResilience(ctx, cfg.ResilienceURL)
		if err != nil {
			result[name] = ResilienceResult{
				Available: false,
				Reason:    err.Error(),
			}
			continue
		}
		result[name] = ResilienceResult{
			Available: true,
			Snapshot:  snapshot,
		}
	}
	return result
}

// FetchCache loads cache/redis snapshots when configured.
func (a *Adapter) FetchCache(ctx context.Context) map[string]CacheResult {
	result := make(map[string]CacheResult)
	if a == nil {
		return result
	}
	for name, cfg := range a.components {
		if cfg == nil || strings.TrimSpace(cfg.CacheURL) == "" {
			continue
		}
		snapshot, err := a.fetchCache(ctx, cfg.CacheURL)
		if err != nil {
			result[name] = CacheResult{
				Available: false,
				Reason:    err.Error(),
			}
			continue
		}
		result[name] = CacheResult{
			Available: true,
			Snapshot:  snapshot,
		}
	}
	return result
}

func (a *Adapter) fetchResilience(ctx context.Context, endpoint string) (*resilienceplane.RuntimeSnapshot, error) {
	body, err := a.getJSON(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	var direct resilienceplane.RuntimeSnapshot
	if err := json.Unmarshal(body, &direct); err == nil && direct.Component != "" {
		return &direct, nil
	}
	var wrapped struct {
		Data resilienceplane.RuntimeSnapshot `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, err
	}
	if wrapped.Data.Component == "" {
		return nil, fmt.Errorf("empty resilience snapshot from %s", endpoint)
	}
	return &wrapped.Data, nil
}

func (a *Adapter) fetchCache(ctx context.Context, endpoint string) (*observability.RuntimeSnapshot, error) {
	body, err := a.getJSON(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	var direct observability.RuntimeSnapshot
	if err := json.Unmarshal(body, &direct); err == nil && direct.Component != "" {
		return &direct, nil
	}
	var wrapped struct {
		Data observability.RuntimeSnapshot `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, err
	}
	if wrapped.Data.Component == "" {
		return nil, fmt.Errorf("empty cache snapshot from %s", endpoint)
	}
	return &wrapped.Data, nil
}

func (a *Adapter) getJSON(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("component governance fetch failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	return body, nil
}
