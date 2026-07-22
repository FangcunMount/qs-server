package statistics

import (
	"reflect"
	"testing"

	statisticsDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

func TestProjectionRegistriesSeparateWindowRepairFromGlobalPublication(t *testing.T) {
	daily := NewDailyProjections(nil)
	global := NewGlobalProjections(nil)
	if got := projectionNames(daily); !reflect.DeepEqual(got, []string{"access_daily", "assessment_daily", "plan_activity_daily"}) {
		t.Fatalf("daily=%v", got)
	}
	if got := projectionNames(global); !reflect.DeepEqual(got, []string{"plan_fulfillment", "organization_snapshot"}) {
		t.Fatalf("global=%v", got)
	}
}

func projectionNames(items []statisticsDomain.Projection) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name())
	}
	return names
}
