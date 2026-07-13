// Package signalcatalog owns stable Redis Pub/Sub signal names.
package signalcatalog

const (
	ReportStatusChanged       = "report_status_changed"
	QuestionnaireCacheChanged = "questionnaire_cache_changed"
	ScaleCacheChanged         = "scale_cache_changed"
	TypologyModelCacheChanged = "typology_model_cache_changed"
)

// SignalNames returns every signal name declared by code.
func SignalNames() []string {
	return []string{
		ReportStatusChanged,
		QuestionnaireCacheChanged,
		ScaleCacheChanged,
		TypologyModelCacheChanged,
	}
}
