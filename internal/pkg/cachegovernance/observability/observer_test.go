package observability_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachehotset"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
)

var _ cachequery.FamilyObserver = (*observability.ComponentObserver)(nil)
var _ cachehotset.FamilyObserver = (*observability.ComponentObserver)(nil)

func TestComponentObserverReportsConfiguredComponent(t *testing.T) {
	observer := observability.NewComponentObserver("component-a")

	if got := observer.Component(); got != "component-a" {
		t.Fatalf("Component() = %q, want component-a", got)
	}
}
