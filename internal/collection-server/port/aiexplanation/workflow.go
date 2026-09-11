package aiexplanation

import (
	"context"
	"encoding/json"
)

type WorkflowAccepted struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
}
type WorkflowClient interface {
	RequestWorkflow(context.Context, uint64, uint64, uint64, string) (*WorkflowAccepted, error)
}

// WorkflowResult exposes content and source provenance, not the internal execution envelope.
type WorkflowResult struct {
	RequestID     string          `json:"request_id"`
	Status        string          `json:"status"`
	Version       int64           `json:"version"`
	Content       json.RawMessage `json:"content,omitempty" swaggertype:"object"`
	ArtifactID    string          `json:"artifact_id,omitempty"`
	ReportID      string          `json:"report_id,omitempty"`
	SourceVersion string          `json:"source_version,omitempty"`
}
type WorkflowReader interface {
	GetWorkflow(context.Context, uint64, uint64, string) (*WorkflowResult, error)
}
