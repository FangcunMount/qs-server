package interpretation

import (
	"context"
	"fmt"
)

// StartAIWorkflowRelay starts only the shared MQ runtime after dependencies are ready.
// Query-only hosts do not start a sender; MQ cannot fall back to retired gRPC writes.
func (m *Module) StartAIWorkflowRelay(ctx context.Context) error {
	if m != nil && m.aiMessagingRuntime != nil {
		return m.aiMessagingRuntime.Start(ctx)
	}
	if m != nil && m.aiWorkflowEnabled {
		return fmt.Errorf("AI runtime command intake requires the MQ runtime")
	}
	return nil
}
