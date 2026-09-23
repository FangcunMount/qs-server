//go:build reliable_messaging_m4

package standardoutbox

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
)

// PostCommitWake accelerates the SDK's next scan after a host transaction
// commits. Signals may coalesce or be lost; the durable row and periodic scan
// remain the recovery path. It never publishes or claims a message itself.
type PostCommitWake struct {
	wake chan struct{}
}

func NewPostCommitWake() *PostCommitWake {
	return &PostCommitWake{wake: make(chan struct{}, 1)}
}

func (p *PostCommitWake) Wake() <-chan struct{} { return p.wake }

func (p *PostCommitWake) AfterCommit(_ context.Context, events []event.DomainEvent, _ time.Time) {
	for _, evt := range events {
		if evt == nil {
			continue
		}
		select {
		case p.wake <- struct{}{}:
		default:
		}
		return
	}
}
