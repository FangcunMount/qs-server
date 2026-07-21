package interpretation

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// IR-R002 read-path observability. Association mismatch is always fail-closed;
// archive org absence is transitional and must not relax assessment/testee checks.
var (
	catalogAssociationMismatchTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs",
		Subsystem: "interpretation",
		Name:      "report_catalog_association_mismatch_total",
		Help:      "Catalog entries whose loaded source disagrees on assessment/org/testee (IR-R002).",
	}, []string{"source_kind"})

	catalogArchiveOrgUnprovenTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "qs",
		Subsystem: "interpretation",
		Name:      "report_catalog_archive_org_unproven_total",
		Help:      "Archive sources loaded without org_id; org not compared, assessment/testee still enforced (IR-R002).",
	})
)

func observeCatalogAssociationMismatch(sourceKind string) {
	if sourceKind == "" {
		sourceKind = "unknown"
	}
	catalogAssociationMismatchTotal.WithLabelValues(sourceKind).Inc()
}

func observeCatalogArchiveOrgUnproven() {
	catalogArchiveOrgUnprovenTotal.Inc()
}
