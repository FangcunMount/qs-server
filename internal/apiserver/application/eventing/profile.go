package eventing

import (
	"context"

	"github.com/FangcunMount/qs-server/pkg/event"
)

type EventStager interface {
	Stage(ctx context.Context, events ...event.DomainEvent) error
}

type ProfileBinding struct {
	Stager     EventStager
	PostCommit PostCommitDispatcher
}
