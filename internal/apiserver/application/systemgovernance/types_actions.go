package systemgovernance

import "time"

// ActionDescriptor describes a governance command exposed to operators.
type ActionDescriptor struct {
	ID                   string                 `json:"id"`
	Domain               Domain                 `json:"domain"`
	Label                string                 `json:"label"`
	RiskLevel            string                 `json:"risk_level"`
	Enabled              bool                   `json:"enabled"`
	Planned              bool                   `json:"planned"`
	RequiresConfirmation bool                   `json:"requires_confirmation"`
	InputSchema          map[string]interface{} `json:"input_schema,omitempty"`
}

// ActionsView lists governance commands.
type ActionsView struct {
	GeneratedAt time.Time          `json:"generated_at"`
	Actions     []ActionDescriptor `json:"actions"`
}

// ActionRunRequest is the body for POST /actions/:action_id/runs.
type ActionRunRequest struct {
	Confirm bool                   `json:"confirm"`
	Input   map[string]interface{} `json:"input,omitempty"`
}

// ActionRunResult is the outcome of an executed governance command.
type ActionRunResult struct {
	ActionID   string                 `json:"action_id"`
	StartedAt  time.Time              `json:"started_at"`
	FinishedAt time.Time              `json:"finished_at"`
	Status     string                 `json:"status"`
	Message    string                 `json:"message,omitempty"`
	Result     map[string]interface{} `json:"result,omitempty"`
}
