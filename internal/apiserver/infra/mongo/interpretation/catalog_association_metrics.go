package interpretation

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// IR-R002 read-path observability. Association mismatch is always fail-closed,
// including archive sources without org_id.
var (
	catalogAssociationMismatchTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs",
		Subsystem: "interpretation",
		Name:      "report_catalog_association_mismatch_total",
		Help:      "Catalog entries whose loaded source disagrees on assessment/org/testee (IR-R002).",
	}, []string{"source_kind"})
)

func observeCatalogAssociationMismatch(sourceKind string) {
	if sourceKind == "" {
		sourceKind = "unknown"
	}
	catalogAssociationMismatchTotal.WithLabelValues(sourceKind).Inc()
}
