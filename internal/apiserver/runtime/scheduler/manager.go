package scheduler

import "context"

// Runner is a background scheduler task that manages its own goroutine lifecycle.
type Runner interface {
	Name() string
	Start(ctx context.Context)
}

// Manager groups multiple scheduler runners behind one startup entry.
type Manager struct {
	runners []Runner
}

// NewManager builds a scheduler manager and drops nil runners.
func NewManager(runners ...Runner) *Manager {
	m := &Manager{}
	for _, runner := range runners {
		m.Add(runner)
	}
	return m
}

// Add registers one runner when it is available.
func (m *Manager) Add(runner Runner) {
	if m == nil || runner == nil {
		return
	}
	m.runners = append(m.runners, runner)
}

// Len returns the number of registered runners.
func (m *Manager) Len() int {
	if m == nil {
		return 0
	}
	return len(m.runners)
}

// Start starts all registered runners.
func (m *Manager) Start(ctx context.Context) {
	if m == nil {
		return
	}
	for _, runner := range m.runners {
		if runner == nil {
			continue
		}
		runner.Start(ctx)
	}
}
