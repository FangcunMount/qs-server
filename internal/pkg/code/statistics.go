package code

const (
	// ErrStatisticsNotReady - 503: Statistics V2 has no completed data batch.
	ErrStatisticsNotReady int = iota + 116001
	// ErrStatisticsOverloaded - 503: Statistics read capacity is temporarily exhausted.
	ErrStatisticsOverloaded
)

func init() {
	register(ErrStatisticsNotReady, 503, "Statistics not ready")
	register(ErrStatisticsOverloaded, 503, "Statistics temporarily overloaded")
}
