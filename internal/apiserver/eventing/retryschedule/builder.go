package retryschedule

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventmessage"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

// Builder is pure. The host calls it before entering its original root. The
// authorization value must be the stable command, without generated clocks,
// messages, credentials, mutable results or external-call effects.
type Builder struct{ Resolver catalog.TopicResolver }

func (b Builder) Freeze(a Authority, evt event.DomainEvent, d *retrygovernance.Decision, authorization any) (Frozen, error) {
	if b.Resolver == nil || evt == nil {
		return Frozen{}, ErrInvalid
	}
	digest, err := DecisionDigest(d)
	if err != nil {
		return Frozen{}, err
	}
	a.DecisionHash = digest
	if a.Origin != retrygovernance.AttemptOriginAutomatic {
		if authorization == nil {
			return Frozen{}, ErrInvalid
		}
		raw, err := json.Marshal(authorization)
		if err != nil || len(raw) == 0 || len(raw) > 1<<20 || string(raw) == "null" {
			return Frozen{}, ErrInvalid
		}
		digest := sha256.Sum256(raw)
		a.AuthorizationSHA256 = hex.EncodeToString(digest[:])
		a.Authorization = bytes.Clone(raw)
	}
	prepared, err := eventmessage.PrepareIntents([]event.DomainEvent{evt}, b.Resolver, "api-server")
	if err != nil {
		return Frozen{}, err
	}
	f, err := New(a, prepared[0].Message.Input(), *d.NextAttemptAt)
	if err != nil {
		return Frozen{}, err
	}
	if err := f.MatchDecision(d, *d.NextAttemptAt); err != nil {
		return Frozen{}, err
	}
	return f, nil
}
