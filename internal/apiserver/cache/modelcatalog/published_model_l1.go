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
	publishedModelL1FallbackTTL               = 15 * time.Minute
	publishedModelExactL1MaxEntries           = 512
	publishedModelCatalogListL1MaxEntries     = 256
	publishedModelByQuestionnaireL1MaxEntries = 512
	publishedModelExactL1Bucket               = "exact_by_ref"
	publishedModelCatalogListL1Bucket         = "catalog_list_versioned"
	publishedModelByQuestionnaireL1Bucket     = "by_questionnaire_versioned"
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
	return newPublishedModelL1(policies, publishedModelExactL1Bucket, publishedModelExactL1MaxEntries, func(model *port.PublishedModel) *port.PublishedModel {
		// PublishedModel is an immutable runtime snapshot by port contract. Sharing
		// the pointer avoids turning an L1 hit back into a deep-copy/JSON hot path.
		return model
	})
}

func newPublishedModelByQuestionnaireL1(policies sharedcache.PolicyProvider) *localcache.Cache[*port.PublishedModel] {
	return newPublishedModelL1(policies, publishedModelByQuestionnaireL1Bucket, publishedModelByQuestionnaireL1MaxEntries, func(model *port.PublishedModel) *port.PublishedModel {
		// The key contains the global catalog version, so a publish invalidation
		// makes every older entry unreachable across apiserver instances.
		return model
	})
}

func newPublishedModelCatalogListL1(policies sharedcache.PolicyProvider) *localcache.Cache[*publishedModelCatalogListPage] {
	return newPublishedModelL1(policies, publishedModelCatalogListL1Bucket, publishedModelCatalogListL1MaxEntries, func(page *publishedModelCatalogListPage) *publishedModelCatalogListPage {
		if page == nil {
			return nil
		}
		cloned := *page
		cloned.Models = append([]*port.PublishedModel(nil), page.Models...)
		return &cloned
	})
}

func newPublishedModelL1[T any](
	policies sharedcache.PolicyProvider,
	bucket string,
	maxEntries int,
	clone func(T) T,
) *localcache.Cache[T] {
	policy := publishedModelPolicy(policies)
	ttl := policy.TTLOr(publishedModelL1FallbackTTL)
	capability := string(cachepolicy.CapabilityModelCatalogPublished)
	publishedModelL1Entries.WithLabelValues(capability, bucket).Set(0)
	publishedModelL1MaxEntries.WithLabelValues(capability, bucket).Set(float64(maxEntries))
	return localcache.New(localcache.Options{
		TTL: ttl,
		TTLProvider: func() time.Duration {
			return publishedModelPolicy(policies).TTLOr(publishedModelL1FallbackTTL)
		},
		MaxEntries: maxEntries, TTLJitterRatio: policy.JitterRatio,
		OnHit: func() {
			publishedModelL1Requests.WithLabelValues(capability, bucket, "hit").Inc()
		},
		OnMiss: func() {
			publishedModelL1Requests.WithLabelValues(capability, bucket, "miss").Inc()
		},
		OnEntries: func(entries int) {
			publishedModelL1Entries.WithLabelValues(capability, bucket).Set(float64(entries))
		},
		OnEviction: func(reason localcache.EvictionReason) {
			publishedModelL1Evictions.WithLabelValues(capability, bucket, string(reason)).Inc()
		},
	}, clone)
}

func publishedModelPolicy(policies sharedcache.PolicyProvider) sharedcache.Policy {
	if policies != nil {
		if effective, ok := policies.Resolve(cachepolicy.CapabilityModelCatalogPublished); ok {
			return effective.Policy
		}
	}
	return sharedcache.Policy{}
}
