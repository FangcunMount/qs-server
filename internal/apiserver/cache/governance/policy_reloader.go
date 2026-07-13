package cachegovernance

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	componenterrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	cachemodel "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	policyReloadTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "qs_cache_policy_reload_total",
		Help: "Total cache policy reload attempts.",
	}, []string{"component", "result"})
	policyReloadDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "qs_cache_policy_reload_duration_seconds",
		Help: "Cache policy reload duration in seconds.",
	}, []string{"component", "result"})
	policySnapshotVersion = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "qs_cache_policy_snapshot_version",
		Help: "Current cache policy registry snapshot version.",
	}, []string{"component"})
)

// PolicyCandidateLoader re-reads and validates process configuration, then
// returns a complete candidate registry. It must not mutate live Options.
type PolicyCandidateLoader func(context.Context) ([]sharedcache.EffectiveCapability, string, error)

type PolicyReloader struct {
	component string
	registry  *sharedcache.Registry
	loader    PolicyCandidateLoader

	mu     sync.Mutex
	status cachemodel.PolicyReloadStatus
}

func NewPolicyReloader(component string, registry *sharedcache.Registry, loader PolicyCandidateLoader) *PolicyReloader {
	r := &PolicyReloader{component: component, registry: registry, loader: loader}
	if registry != nil {
		policySnapshotVersion.WithLabelValues(component).Set(float64(registry.Version()))
	}
	return r
}

func (r *PolicyReloader) ReloadPolicy(ctx context.Context, orgID int64, request cachemodel.CachePolicyReloadRequest) (*cachemodel.CachePolicyReloadResult, error) {
	if r == nil || r.registry == nil || r.loader == nil {
		return nil, componenterrors.WithCode(code.ErrInternalServerError, "cache policy reload is unavailable")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	startedAt := time.Now()
	r.status.LastAttemptAt = startedAt
	resultLabel := "failure"
	defer func() {
		policyReloadTotal.WithLabelValues(r.component, resultLabel).Inc()
		policyReloadDuration.WithLabelValues(r.component, resultLabel).Observe(time.Since(startedAt).Seconds())
	}()

	if current := r.registry.Version(); current != request.ExpectedVersion {
		err := componenterrors.WithCode(code.ErrConflict, "cache policy snapshot version conflict: expected %d, current %d", request.ExpectedVersion, current)
		r.recordFailure(err)
		return nil, err
	}
	candidate, source, err := r.loader(ctx)
	if err != nil {
		r.recordFailure(err)
		return nil, err
	}
	changedCapabilities := changedCapabilityIDs(r.registry.All(), candidate)
	published, err := r.registry.Publish(request.ExpectedVersion, candidate, time.Now())
	if errors.Is(err, sharedcache.ErrRegistryVersionConflict) {
		err = componenterrors.WithCode(code.ErrConflict, "cache policy snapshot version conflict")
	}
	if err != nil {
		r.recordFailure(err)
		return nil, err
	}
	now := time.Now()
	r.status.LastSuccessAt = now
	r.status.LastError = ""
	resultLabel = "success"
	policySnapshotVersion.WithLabelValues(r.component).Set(float64(published.CurrentVersion))
	logger.L(ctx).Infow("Cache policy reload completed",
		"component", r.component, "org_id", orgID, "source", source,
		"previous_version", published.PreviousVersion, "current_version", published.CurrentVersion,
		"changed", published.Changed, "changed_capabilities", changedCapabilities,
	)
	return &cachemodel.CachePolicyReloadResult{
		PreviousVersion: published.PreviousVersion, CurrentVersion: published.CurrentVersion,
		Changed: published.Changed, Source: source, ChangedCapabilities: changedCapabilities,
	}, nil
}

func (r *PolicyReloader) ReloadStatus() cachemodel.PolicyReloadStatus {
	if r == nil {
		return cachemodel.PolicyReloadStatus{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

func (r *PolicyReloader) recordFailure(err error) {
	r.status.LastFailureAt = time.Now()
	if err != nil {
		r.status.LastError = err.Error()
	}
}

func changedCapabilityIDs(current, candidate []sharedcache.EffectiveCapability) []string {
	currentByID := make(map[sharedcache.Capability]sharedcache.EffectiveCapability, len(current))
	for _, item := range current {
		currentByID[item.Capability] = item
	}
	result := make([]string, 0)
	for _, item := range candidate {
		if previous, ok := currentByID[item.Capability]; !ok || !effectiveCapabilityEqual(previous, item) {
			result = append(result, string(item.Capability))
		}
	}
	sort.Strings(result)
	return result
}

func effectiveCapabilityEqual(left, right sharedcache.EffectiveCapability) bool {
	return left.Capability == right.Capability && left.Owner == right.Owner && left.Kind == right.Kind &&
		left.Layer == right.Layer && left.Family == right.Family && left.Enabled == right.Enabled &&
		left.Layers == right.Layers && left.Policy == right.Policy && left.Source == right.Source &&
		left.CatalogVersion == right.CatalogVersion && left.MetricLabel == right.MetricLabel
}
