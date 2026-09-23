//go:build reliable_messaging_m4

package standardoutbox

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
)

func TestPostCommitWakeSignalsCommittedEventsWithoutBlocking(t *testing.T) {
	wake := NewPostCommitWake()
	wake.AfterCommit(context.Background(), nil, time.Now())
	select {
	case <-wake.Wake():
		t.Fatal("empty transaction signaled relay")
	default:
	}
	evt := event.Event[map[string]any]{BaseEvent: event.BaseEvent{ID: "event-1"}}
	for range 100 {
		wake.AfterCommit(context.Background(), []event.DomainEvent{evt}, time.Now())
	}
	select {
	case <-wake.Wake():
	default:
		t.Fatal("committed event did not signal relay")
	}
	select {
	case <-wake.Wake():
		t.Fatal("wake signals did not coalesce")
	default:
	}
}
