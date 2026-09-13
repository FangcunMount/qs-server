package interpretation

import (
	"context"
	"fmt"
)

// StartAIWorkflowRelay is called once by process bootstrap, after dependencies are ready.
// Cleanup cancels and joins it before closing the shared gRPC connection or database.
func (m *Module) StartAIWorkflowRelay(ctx context.Context) error {
	if m == nil || !m.aiWorkflowEnabled || m.aiRelayCancel != nil {
		return nil
	}
	if m.aiBridge == nil || m.aiBridge.Store == nil || m.aiBridge.Sender == nil {
		return fmt.Errorf("AI command relay dependencies are not configured")
	}
	ctx, m.aiRelayCancel = context.WithCancel(ctx)
	m.aiRelayDone = make(chan struct{})
	go func() {
		defer close(m.aiRelayDone)
		m.aiBridge.ServeRelay(ctx)
	}()
	return nil
}
