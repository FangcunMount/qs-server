package reportwait

import (
	"sync"

	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
)

type inMemoryWaitHub struct {
	mu      sync.RWMutex
	waiters map[string]map[uint64]chan reportstatus.ChangedSignal
	nextID  uint64
}

func NewInMemoryWaitHub() WaitHub {
	return &inMemoryWaitHub{waiters: make(map[string]map[uint64]chan reportstatus.ChangedSignal)}
}

func (h *inMemoryWaitHub) Register(assessmentID string) (<-chan reportstatus.ChangedSignal, func()) {
	ch := make(chan reportstatus.ChangedSignal, 1)
	h.mu.Lock()
	h.nextID++
	id := h.nextID
	if h.waiters[assessmentID] == nil {
		h.waiters[assessmentID] = make(map[uint64]chan reportstatus.ChangedSignal)
	}
	h.waiters[assessmentID][id] = ch
	total := h.activeWaitersLocked()
	h.mu.Unlock()
	reportstatus.IncWaitRegistered()
	reportstatus.SetActiveWaiters(total)

	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if m, ok := h.waiters[assessmentID]; ok {
			if c, exists := m[id]; exists {
				delete(m, id)
				close(c)
				reportstatus.IncWaitUnregistered()
			}
			if len(m) == 0 {
				delete(h.waiters, assessmentID)
			}
		}
		reportstatus.SetActiveWaiters(h.activeWaitersLocked())
	}
	return ch, cancel
}

func (h *inMemoryWaitHub) Notify(signal reportstatus.ChangedSignal) {
	h.mu.RLock()
	m := h.waiters[signal.AssessmentID]
	waiters := make([]chan reportstatus.ChangedSignal, 0, len(m))
	for _, ch := range m {
		waiters = append(waiters, ch)
	}
	h.mu.RUnlock()

	for _, ch := range waiters {
		select {
		case ch <- signal:
			reportstatus.IncWaitWakeup()
		default:
		}
	}
}

func (h *inMemoryWaitHub) ActiveWaiters() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.activeWaitersLocked()
}

func (h *inMemoryWaitHub) activeWaitersLocked() int {
	total := 0
	for _, m := range h.waiters {
		total += len(m)
	}
	return total
}
