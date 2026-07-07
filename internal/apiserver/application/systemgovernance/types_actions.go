package systemgovernance

import "time"

// ActionDescriptor 描述governance 命令 exposed 到 operators。
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

// ActionsView 列出治理命令。
type ActionsView struct {
	GeneratedAt time.Time          `json:"generated_at"`
	Actions     []ActionDescriptor `json:"actions"`
}

// ActionRunRequest 是body 用于 POST /actions/:action_id/runs。
type ActionRunRequest struct {
	Confirm bool                   `json:"confirm"`
	Input   map[string]interface{} `json:"input,omitempty"`
}

// ActionRunResult 是结果 of executed governance 命令。
type ActionRunResult struct {
	ActionID   string                 `json:"action_id"`
	StartedAt  time.Time              `json:"started_at"`
	FinishedAt time.Time              `json:"finished_at"`
	Status     string                 `json:"status"`
	Message    string                 `json:"message,omitempty"`
	Result     map[string]interface{} `json:"result,omitempty"`
}
