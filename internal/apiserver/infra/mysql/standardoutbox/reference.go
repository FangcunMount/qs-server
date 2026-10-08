package standardoutbox

import (
	"fmt"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

var _ outbox.ReferencePreparer = (*Stager)(nil)

func (s *Stager) PrepareReference(evt event.DomainEvent) (evidence.StandardReference, error) {
	if s == nil {
		return evidence.StandardReference{}, fmt.Errorf("standard stager is not configured")
	}
	return standardoutbox.PrepareReference(evt, s.resolver, s.source)
}
