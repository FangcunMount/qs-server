package reportnotify

import (
	"sync"

	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
)

type inMemoryNotifier struct {
	mu      sync.RWMutex
	waiters map[string]map[uint64]chan StatusEvent
	nextID  uint64
}

// NewInMemoryNotifier 创建进程内报告状态通知器。
func NewInMemoryNotifier() Notifier {
	return &inMemoryNotifier{waiters: make(map[string]map[uint64]chan StatusEvent)}
}

func (n *inMemoryNotifier) Subscribe(assessmentID string) (<-chan StatusEvent, func()) {
	ch := make(chan StatusEvent, 1)
	n.mu.Lock()
	n.nextID++
	id := n.nextID
	if n.waiters[assessmentID] == nil {
		n.waiters[assessmentID] = make(map[uint64]chan StatusEvent)
	}
	n.waiters[assessmentID][id] = ch
	total := n.activeSubscriptionsLocked()
	n.mu.Unlock()
	reportstatus.IncWaitRegistered()
	reportstatus.SetActiveWaiters(total)

	cancel := func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		if m, ok := n.waiters[assessmentID]; ok {
			if c, exists := m[id]; exists {
				delete(m, id)
				close(c)
				reportstatus.IncWaitUnregistered()
			}
			if len(m) == 0 {
				delete(n.waiters, assessmentID)
			}
		}
		reportstatus.SetActiveWaiters(n.activeSubscriptionsLocked())
	}
	return ch, cancel
}

func (n *inMemoryNotifier) Notify(signal StatusEvent) {
	n.mu.RLock()
	m := n.waiters[signal.AssessmentID]
	waiters := make([]chan StatusEvent, 0, len(m))
	for _, ch := range m {
		waiters = append(waiters, ch)
	}
	n.mu.RUnlock()

	for _, ch := range waiters {
		select {
		case ch <- signal:
			reportstatus.IncWaitWakeup()
		default:
		}
	}
}

func (n *inMemoryNotifier) ActiveSubscriptions() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.activeSubscriptionsLocked()
}

func (n *inMemoryNotifier) activeSubscriptionsLocked() int {
	total := 0
	for _, m := range n.waiters {
		total += len(m)
	}
	return total
}
