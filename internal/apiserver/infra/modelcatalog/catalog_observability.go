package modelcatalog

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// CatalogStore identifies which published-model read adapter emitted observability.
type CatalogStore string

const (
	CatalogStoreDualStore         CatalogStore = "dual_store"
	CatalogStorePublishedTypology CatalogStore = "published_typology"
	CatalogStoreLayeredStatic     CatalogStore = "layered_static"
)

var legacyFallbackHitsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "qs_modelcatalog_legacy_fallback_hits_total",
		Help: "Legacy published-model catalog fallback reads before v2-only retirement.",
	},
	[]string{"store", "operation"},
)

type fallbackRecorder func(store CatalogStore, operation string)

var recordFallbackHit fallbackRecorder = func(store CatalogStore, operation string) {
	legacyFallbackHitsTotal.WithLabelValues(string(store), operation).Inc()
}

func RecordLegacyFallback(ctx context.Context, store CatalogStore, operation, detail string) {
	recordLegacyFallback(ctx, store, operation, detail)
}

func recordLegacyFallback(ctx context.Context, store CatalogStore, operation, detail string) {
	recordFallbackHit(store, operation)
	logger.L(ctx).Warnw("model catalog legacy fallback read",
		"store", store,
		"operation", operation,
		"detail", detail,
	)
}

func SetFallbackRecorderForTest(recorder fallbackRecorder) func() {
	previous := recordFallbackHit
	if recorder == nil {
		recordFallbackHit = func(CatalogStore, string) {}
	} else {
		recordFallbackHit = recorder
	}
	return func() {
		recordFallbackHit = previous
	}
}
