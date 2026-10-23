package retryschedule

import (
	"math"

	"github.com/FangcunMount/reliable-messaging/transaction"
)

// LegacyInventory accounts for the original ledger only. It cannot prove that
// writers stopped, NSQ drained, consumers finished or a runtime never started.
type LegacyInventory struct {
	Binding                      transaction.Binding `json:"binding"`
	Counts                       map[string]uint64   `json:"original_outbox_counts"`
	Total                        uint64              `json:"total"`
	Active                       uint64              `json:"active"`
	VerifiedHandoffs             uint64              `json:"verified_handoffs"`
	UnverifiedHeld               uint64              `json:"unverified_held"`
	PublishedWithoutConfirmation uint64              `json:"published_without_confirmation"`
	ResponsibilitiesAccounted    bool                `json:"original_responsibilities_accounted"`
}

// Inventory validates complete state aggregates and precise, independently
// reviewed source/target pairs. Merely having a well-formed held marker never
// removes a responsibility from this inventory.
func Inventory(binding transaction.Binding, counts map[string]uint64, verified, unconfirmed uint64) (LegacyInventory, error) {
	if !binding.Valid() || (binding.Owner != "qs.mysql" && binding.Owner != "qs.mongo") {
		return LegacyInventory{}, ErrInvalid
	}
	result := LegacyInventory{Binding: binding, Counts: make(map[string]uint64, 6), VerifiedHandoffs: verified, PublishedWithoutConfirmation: unconfirmed}
	for state, count := range counts {
		switch state {
		case "pending", "retry_wait", "publishing", "quarantined", "held", "published":
		default:
			return LegacyInventory{}, ErrInvalid
		}
		if count > math.MaxInt64-result.Total {
			return LegacyInventory{}, ErrInvalid
		}
		result.Total += count
		result.Counts[state] = count
		if state != "published" && state != "held" {
			result.Active += count
		}
	}
	if verified > counts["held"] || unconfirmed > counts["published"] {
		return LegacyInventory{}, ErrInvalid
	}
	result.UnverifiedHeld = counts["held"] - verified
	result.ResponsibilitiesAccounted = result.Active == 0 && result.UnverifiedHeld == 0 && unconfirmed == 0
	return result, nil
}

func ValidateHandoffSelection(binding transaction.Binding, plans []Frozen) error {
	if !binding.Valid() || (binding.Owner != "qs.mysql" && binding.Owner != "qs.mongo") || len(plans) > 100 {
		return ErrInvalid
	}
	seen := make(map[[32]byte]bool, len(plans))
	for _, f := range plans {
		if !f.Valid() || f.Authority().Owner != binding.Owner || seen[f.Key()] {
			return ErrInvalid
		}
		seen[f.Key()] = true
	}
	return nil
}
