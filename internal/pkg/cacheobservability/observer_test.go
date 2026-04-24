package cacheobservability_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachehotset"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
)

var _ cachequery.FamilyObserver = (*cacheobservability.ComponentObserver)(nil)
var _ cachehotset.FamilyObserver = (*cacheobservability.ComponentObserver)(nil)

func TestComponentObserverReportsConfiguredComponent(t *testing.T) {
	observer := cacheobservability.NewComponentObserver("component-a")

	if got := observer.Component(); got != "component-a" {
		t.Fatalf("Component() = %q, want component-a", got)
	}
}
