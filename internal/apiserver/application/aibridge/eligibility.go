package aibridge

import "context"

// Eligibility is a read-only snapshot, not an admission or quota reservation.
type Eligibility struct {
	Status     string `json:"status"`
	ReasonCode string `json:"reason_code,omitempty"`
}

type EligibilityReader interface {
	CheckEligibility(context.Context, Actor, string, []string, []EvidenceItem) (*Eligibility, error)
}

func (e *Eligibility) Valid() bool {
	if e == nil {
		return false
	}
	if e.Status == "available" {
		return e.ReasonCode == ""
	}
	if e.Status != "unavailable" {
		return false
	}
	switch e.ReasonCode {
	case "unsupported_scene", "unsupported_model_version", "source_incomplete", "source_conflict", "publication_missing", "publication_paused", "asset_invalid":
		return true
	}
	return false
}
