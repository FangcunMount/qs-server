package ws

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	reportEventsActiveConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "report_events_active_connections",
		Help: "Current active report-events WebSocket connections.",
	})
	reportEventsPushTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "report_events_push_total",
		Help: "Total report-events WebSocket status pushes.",
	})
	reportEventsSubscribeDeniedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "report_events_subscribe_denied_total",
		Help: "Total denied report-events WebSocket subscribe attempts.",
	}, []string{"reason"})
)

func incSubscribeDenied(reason string) {
	reportEventsSubscribeDeniedTotal.WithLabelValues(reason).Inc()
}

func incPush() {
	reportEventsPushTotal.Inc()
}

type connectionManager struct {
	mu           sync.Mutex
	maxTotal     int
	maxPerTestee int
	total        int
	perTestee    map[string]int
}

func newConnectionManager(maxTotal, maxPerTestee int) *connectionManager {
	if maxTotal <= 0 {
		maxTotal = 2000
	}
	if maxPerTestee <= 0 {
		maxPerTestee = 2
	}
	return &connectionManager{
		maxTotal:     maxTotal,
		maxPerTestee: maxPerTestee,
		perTestee:    make(map[string]int),
	}
}

func (m *connectionManager) TryAcquire(testeeID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.total >= m.maxTotal {
		incSubscribeDenied("max_connections")
		return false
	}
	if m.perTestee[testeeID] >= m.maxPerTestee {
		incSubscribeDenied("max_per_testee")
		return false
	}
	m.total++
	m.perTestee[testeeID]++
	reportEventsActiveConnections.Set(float64(m.total))
	return true
}

func (m *connectionManager) Release(testeeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.total > 0 {
		m.total--
	}
	if count, ok := m.perTestee[testeeID]; ok {
		count--
		if count <= 0 {
			delete(m.perTestee, testeeID)
		} else {
			m.perTestee[testeeID] = count
		}
	}
	reportEventsActiveConnections.Set(float64(m.total))
}
