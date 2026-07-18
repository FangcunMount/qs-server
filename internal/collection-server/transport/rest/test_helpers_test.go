package rest

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/container"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	locksubsystem "github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/subsystem"
)

func mustNewCollectionContainer(t *testing.T, opts *options.Options, ops *redisruntime.Handle, locks *locksubsystem.Subsystem, status *observability.FamilyStatusRegistry) *container.Container {
	t.Helper()
	c, err := container.NewContainer(opts, ops, locks, status)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
