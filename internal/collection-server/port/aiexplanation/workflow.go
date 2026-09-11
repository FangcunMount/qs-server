package aiexplanation

import "context"

type WorkflowAccepted struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
}
type WorkflowClient interface {
	RequestWorkflow(context.Context, uint64, uint64, uint64, string) (*WorkflowAccepted, error)
}
