package code

const (
	// ErrStatisticsNotReady - 503: Statistics V2 has no completed data batch.
	ErrStatisticsNotReady int = iota + 116001
)

func init() {
	register(ErrStatisticsNotReady, 503, "Statistics not ready")
}
