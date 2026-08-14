package modelcatalogcache

import (
	"time"

	cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	localcache "github.com/FangcunMount/qs-server/internal/pkg/cache/local"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	publishedModelExactL1TTL        = 15 * time.Minute
	publishedModelExactL1MaxEntries = 512
	publishedModelExactL1Bucket     = "exact_by_ref"
)

var (
	publishedModelL1Requests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "qs_apiserver_l1_cache_requests_total",
		Help: "Total qs-apiserver L1 lookups by capability, bucket and result.",
	}, []string{"capability", "bucket", "result"})
	publishedModelL1Entries = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "qs_apiserver_l1_cache_entries",
		Help: "Current qs-apiserver L1 entries by capability and bucket.",
	}, []string{"capability", "bucket"})
	publishedModelL1MaxEntries = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "qs_apiserver_l1_cache_max_entries",
		Help: "Configured qs-apiserver L1 maximum entries by capability and bucket.",
	}, []string{"capability", "bucket"})
	publishedModelL1Evictions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "qs_apiserver_l1_cache_evictions_total",
		Help: "Total qs-apiserver L1 removals by capability, bucket and reason.",
	}, []string{"capability", "bucket", "reason"})
)

func newPublishedModelExactL1(policies sharedcache.PolicyProvider) *localcache.Cache[*port.PublishedModel] {
	policy := publishedModelPolicy(policies)
	ttl := policy.TTLOr(publishedModelExactL1TTL)
	capability := string(cachepolicy.CapabilityModelCatalogPublished)
	publishedModelL1Entries.WithLabelValues(capability, publishedModelExactL1Bucket).Set(0)
	publishedModelL1MaxEntries.WithLabelValues(capability, publishedModelExactL1Bucket).Set(publishedModelExactL1MaxEntries)
	return localcache.New(localcache.Options{
		TTL: ttl,
		TTLProvider: func() time.Duration {
			return publishedModelPolicy(policies).TTLOr(publishedModelExactL1TTL)
		},
		MaxEntries: publishedModelExactL1MaxEntries, TTLJitterRatio: policy.JitterRatio,
		OnHit: func() {
			publishedModelL1Requests.WithLabelValues(capability, publishedModelExactL1Bucket, "hit").Inc()
		},
		OnMiss: func() {
			publishedModelL1Requests.WithLabelValues(capability, publishedModelExactL1Bucket, "miss").Inc()
		},
		OnEntries: func(entries int) {
			publishedModelL1Entries.WithLabelValues(capability, publishedModelExactL1Bucket).Set(float64(entries))
		},
		OnEviction: func(reason localcache.EvictionReason) {
			publishedModelL1Evictions.WithLabelValues(capability, publishedModelExactL1Bucket, string(reason)).Inc()
		},
	}, func(model *port.PublishedModel) *port.PublishedModel {
		// PublishedModel is an immutable runtime snapshot by port contract. Sharing
		// the pointer avoids turning an L1 hit back into a deep-copy/JSON hot path.
		return model
	})
}

func publishedModelPolicy(policies sharedcache.PolicyProvider) sharedcache.Policy {
	if policies != nil {
		if effective, ok := policies.Resolve(cachepolicy.CapabilityModelCatalogPublished); ok {
			return effective.Policy
		}
	}
	return sharedcache.Policy{}
}
