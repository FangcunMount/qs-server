package process

import (
	"context"
	"errors"
	"fmt"
	"testing"

	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

func TestIncompleteShutdownRetainsOriginalDatabaseAndPublisherUntilContinuedDrain(t *testing.T) {
	for _, incomplete := range []error{
		&transaction.ShutdownIncomplete{ActiveSends: 1},
		&eventsubsystem.ShutdownIncomplete{PendingClosures: 1, Cause: context.DeadlineExceeded},
	} {
		t.Run(fmt.Sprintf("%T", incomplete), func(t *testing.T) {
			pending := fmt.Errorf("original container drain: %w", incomplete)
			publisher, db, cleanup := 0, 0, 0
			deps := processLifecycleDeps{
				container: containerLifecycleDeps{containerCleanup: func() error { cleanup++; return pending }},
				resource:  resourceLifecycleDeps{closePublisher: func() error { publisher++; return nil }, closeDatabase: func() error { db++; return nil }},
			}
			if err := runProcessLifecycleDeps(deps); !errors.Is(err, incomplete) {
				t.Fatal("lost real incomplete result", err)
			}
			if publisher != 0 || db != 0 {
				t.Fatal("dependencies closed while work remained", publisher, db)
			}
			pending = nil
			if err := runProcessLifecycleDeps(deps); err != nil {
				t.Fatal(err)
			}
			if publisher != 1 || db != 1 || cleanup != 2 {
				t.Fatal("continued drain did not release exactly once", publisher, db, cleanup)
			}
		})
	}
}
