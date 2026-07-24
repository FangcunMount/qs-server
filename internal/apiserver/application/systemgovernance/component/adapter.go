package component

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
)

const (
	defaultComponentTimeout = 3 * time.Second
	maxDNSAddresses         = 16
)

type HostResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

// ResilienceResult 保存一个组件 resilience 载荷 使用 fetch 元数据。
type ResilienceResult struct {
	Available               bool                                   `json:"available"`
	Reason                  string                                 `json:"reason,omitempty"`
	Snapshot                *resilience.RuntimeSnapshot            `json:"snapshot,omitempty"`
	Instances               map[string]*resilience.RuntimeSnapshot `json:"instances,omitempty"`
	DiscoveredInstanceCount int                                    `json:"discovered_instance_count,omitempty"`
	AvailableInstanceCount  int                                    `json:"available_instance_count,omitempty"`
	Partial                 bool                                   `json:"partial,omitempty"`
	TargetErrors            map[string]string                      `json:"target_errors,omitempty"`
}

// CacheResult 保存一个组件 缓存/redis 载荷 使用 fetch 元数据。
type CacheResult struct {
	Available               bool                                      `json:"available"`
	Reason                  string                                    `json:"reason,omitempty"`
	Snapshot                *observability.RuntimeSnapshot            `json:"snapshot,omitempty"`
	Instances               map[string]*observability.RuntimeSnapshot `json:"instances,omitempty"`
	DiscoveredInstanceCount int                                       `json:"discovered_instance_count,omitempty"`
	AvailableInstanceCount  int                                       `json:"available_instance_count,omitempty"`
	Partial                 bool                                      `json:"partial,omitempty"`
	TargetErrors            map[string]string                         `json:"target_errors,omitempty"`
}

// Adapter fetches remote 组件 governance 快照。
type Adapter struct {
	components map[string]*options.GovernanceComponentOptions
	http       *http.Client
	resolver   HostResolver
}

// NewAdapter 构建组件 governance adapter。
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
		http:       &http.Client{},
		resolver:   net.DefaultResolver,
	}
}

// FetchResilience 加载resilience 快照 用于 配置化 组件。
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
		if cfg.DiscoveryMode() == "dns" {
			result[name] = a.fetchDNSResilience(ctx, cfg)
			continue
		}
		snapshot, err := a.fetchResilience(ctx, requestTarget{URL: cfg.ResilienceURL}, componentTimeout(cfg))
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

// FetchCache 加载缓存/redis 快照 when 配置化。
func (a *Adapter) FetchCache(ctx context.Context) map[string]CacheResult {
	result := make(map[string]CacheResult)
	if a == nil {
		return result
	}
	for name, cfg := range a.components {
		if cfg == nil || strings.TrimSpace(cfg.CacheURL) == "" {
			continue
		}
		if cfg.DiscoveryMode() == "dns" {
			result[name] = a.fetchDNSCache(ctx, cfg)
			continue
		}
		snapshot, err := a.fetchCache(ctx, requestTarget{URL: cfg.CacheURL}, componentTimeout(cfg))
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

type requestTarget struct {
	Address    string
	URL        string
	HostHeader string
}

func (a *Adapter) fetchResilience(ctx context.Context, target requestTarget, timeout time.Duration) (*resilience.RuntimeSnapshot, error) {
	body, err := a.getJSON(ctx, target, timeout)
	if err != nil {
		return nil, err
	}
	var direct resilience.RuntimeSnapshot
	if err := json.Unmarshal(body, &direct); err == nil && direct.Component != "" {
		return &direct, nil
	}
	var wrapped struct {
		Data resilience.RuntimeSnapshot `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, err
	}
	if wrapped.Data.Component == "" {
		return nil, fmt.Errorf("empty resilience snapshot from %s", target.URL)
	}
	return &wrapped.Data, nil
}

func (a *Adapter) fetchCache(ctx context.Context, target requestTarget, timeout time.Duration) (*observability.RuntimeSnapshot, error) {
	body, err := a.getJSON(ctx, target, timeout)
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
		return nil, fmt.Errorf("empty cache snapshot from %s", target.URL)
	}
	return &wrapped.Data, nil
}

func (a *Adapter) getJSON(ctx context.Context, target requestTarget, timeout time.Duration) ([]byte, error) {
	if timeout <= 0 {
		timeout = defaultComponentTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL, nil)
	if err != nil {
		return nil, err
	}
	if target.HostHeader != "" {
		req.Host = target.HostHeader
	}
	client := a.http
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
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

func (a *Adapter) fetchDNSResilience(ctx context.Context, cfg *options.GovernanceComponentOptions) ResilienceResult {
	targets, err := a.resolveTargets(ctx, cfg.ResilienceURL)
	if err != nil {
		return ResilienceResult{Available: false, Reason: err.Error(), TargetErrors: map[string]string{"dns": err.Error()}}
	}
	responses := fetchTargets(ctx, targets, func(ctx context.Context, target requestTarget) (*resilience.RuntimeSnapshot, error) {
		return a.fetchResilience(ctx, target, componentTimeout(cfg))
	})
	instances := make(map[string]*resilience.RuntimeSnapshot)
	targetErrors := make(map[string]string)
	for _, item := range responses {
		if item.err != nil {
			targetErrors[item.target.Address] = item.err.Error()
			continue
		}
		instanceID := strings.TrimSpace(item.snapshot.InstanceID)
		if instanceID == "" {
			targetErrors[item.target.Address] = "empty instance_id"
			continue
		}
		if _, exists := instances[instanceID]; exists {
			targetErrors[item.target.Address] = "duplicate instance_id: " + instanceID
			continue
		}
		instances[instanceID] = item.snapshot
	}
	result := ResilienceResult{
		Available:               len(instances) > 0,
		Instances:               instances,
		DiscoveredInstanceCount: len(targets),
		AvailableInstanceCount:  len(instances),
		TargetErrors:            targetErrors,
	}
	result.Snapshot = representativeResilienceSnapshot(instances)
	result.Partial = len(targetErrors) > 0 || len(instances) < cfg.RequiredInstances()
	result.Reason = dnsResultReason(result.Available, result.Partial, len(instances), cfg.RequiredInstances())
	return result
}

func (a *Adapter) fetchDNSCache(ctx context.Context, cfg *options.GovernanceComponentOptions) CacheResult {
	targets, err := a.resolveTargets(ctx, cfg.CacheURL)
	if err != nil {
		return CacheResult{Available: false, Reason: err.Error(), TargetErrors: map[string]string{"dns": err.Error()}}
	}
	responses := fetchTargets(ctx, targets, func(ctx context.Context, target requestTarget) (*observability.RuntimeSnapshot, error) {
		return a.fetchCache(ctx, target, componentTimeout(cfg))
	})
	instances := make(map[string]*observability.RuntimeSnapshot)
	targetErrors := make(map[string]string)
	for _, item := range responses {
		if item.err != nil {
			targetErrors[item.target.Address] = item.err.Error()
			continue
		}
		instanceID := strings.TrimSpace(item.snapshot.InstanceID)
		if instanceID == "" {
			targetErrors[item.target.Address] = "empty instance_id"
			continue
		}
		if _, exists := instances[instanceID]; exists {
			targetErrors[item.target.Address] = "duplicate instance_id: " + instanceID
			continue
		}
		instances[instanceID] = item.snapshot
	}
	result := CacheResult{
		Available:               len(instances) > 0,
		Instances:               instances,
		DiscoveredInstanceCount: len(targets),
		AvailableInstanceCount:  len(instances),
		TargetErrors:            targetErrors,
	}
	result.Snapshot = representativeCacheSnapshot(instances)
	result.Partial = len(targetErrors) > 0 || len(instances) < cfg.RequiredInstances()
	result.Reason = dnsResultReason(result.Available, result.Partial, len(instances), cfg.RequiredInstances())
	return result
}

type targetResponse[T any] struct {
	target   requestTarget
	snapshot *T
	err      error
}

func fetchTargets[T any](
	ctx context.Context,
	targets []requestTarget,
	fetch func(context.Context, requestTarget) (*T, error),
) []targetResponse[T] {
	responses := make(chan targetResponse[T], len(targets))
	var group sync.WaitGroup
	for _, target := range targets {
		target := target
		group.Add(1)
		go func() {
			defer group.Done()
			snapshot, err := fetch(ctx, target)
			responses <- targetResponse[T]{target: target, snapshot: snapshot, err: err}
		}()
	}
	group.Wait()
	close(responses)
	result := make([]targetResponse[T], 0, len(targets))
	for response := range responses {
		result = append(result, response)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].target.Address < result[j].target.Address })
	return result
}

func (a *Adapter) resolveTargets(ctx context.Context, endpoint string) ([]requestTarget, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	hostname := parsed.Hostname()
	if parsed.Scheme != "http" || hostname == "" {
		return nil, fmt.Errorf("dns component governance URL must be absolute http: %s", endpoint)
	}
	resolver := a.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addresses, err := resolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", hostname, err)
	}
	unique := make(map[string]struct{})
	for _, address := range addresses {
		ip := address.IP.To4()
		if ip == nil {
			continue
		}
		unique[ip.String()] = struct{}{}
	}
	ips := make([]string, 0, len(unique))
	for ip := range unique {
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	if len(ips) == 0 {
		return nil, fmt.Errorf("resolve %s: no IPv4 addresses", hostname)
	}
	if len(ips) > maxDNSAddresses {
		ips = ips[:maxDNSAddresses]
	}
	port := parsed.Port()
	if port == "" {
		port = "80"
	}
	targets := make([]requestTarget, 0, len(ips))
	for _, ip := range ips {
		targetURL := *parsed
		targetURL.Host = net.JoinHostPort(ip, port)
		targets = append(targets, requestTarget{
			Address:    ip,
			URL:        targetURL.String(),
			HostHeader: parsed.Host,
		})
	}
	return targets, nil
}

func representativeResilienceSnapshot(instances map[string]*resilience.RuntimeSnapshot) *resilience.RuntimeSnapshot {
	for _, instanceID := range sortedKeys(instances) {
		return instances[instanceID]
	}
	return nil
}

func representativeCacheSnapshot(instances map[string]*observability.RuntimeSnapshot) *observability.RuntimeSnapshot {
	for _, instanceID := range sortedKeys(instances) {
		return instances[instanceID]
	}
	return nil
}

func sortedKeys[T any](items map[string]*T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func dnsResultReason(available, partial bool, actual, expected int) string {
	switch {
	case !available:
		return "all dns targets unavailable"
	case partial:
		return fmt.Sprintf("available %d/%d instances", actual, expected)
	default:
		return ""
	}
}

func componentTimeout(cfg *options.GovernanceComponentOptions) time.Duration {
	if cfg != nil && cfg.Timeout > 0 {
		return cfg.Timeout
	}
	return defaultComponentTimeout
}
