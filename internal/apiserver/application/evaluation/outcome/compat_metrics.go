package outcome

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Offline condition (ops / EV-R012):
// when qs_evaluation_score_catalog_fallback_total stays flat (rate≈0) for a
// sustained window (e.g. 14d) across environments, remove the current-catalog
// metadata fallback in ScoreFactReader.
var scoreCatalogFallbackTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "qs",
	Subsystem: "evaluation",
	Name:      "score_catalog_fallback_total",
	Help:      "ScoreFactReader hits that fill factor name/max from current Scale catalog (EV-R012).",
})

func observeScoreCatalogFallback() {
	scoreCatalogFallbackTotal.Inc()
}
