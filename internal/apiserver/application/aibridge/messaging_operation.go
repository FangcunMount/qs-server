package aibridge

import "encoding/json"

// OperationScope is supplied by the authenticated host, never trusted from a
// status-query body. ResourceID retains the original request/session/run locator.
type OperationScope struct {
	OrganizationID string
	SubjectID      string
	ResourceID     string
}

// MessagingOperation separates durable submission, transport and AI decision.
// A published message is still submitted until an authenticated receipt commits.
type MessagingOperation struct {
	OperationID     string          `json:"operation_id"`
	CommandID       string          `json:"command_id"`
	Status          string          `json:"status"`
	TransportStatus string          `json:"transport_status"`
	Decision        string          `json:"decision,omitempty"`
	Code            string          `json:"code,omitempty"`
	ResourceID      string          `json:"resource_id"`
	Receipt         json.RawMessage `json:"receipt,omitempty"`
}
