package interpretation

import (
	"reflect"
	"testing"

	evaluationreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"go.mongodb.org/mongo-driver/bson"
)

func TestCurrentReportReadQueryUsesExistingFilterSemantics(t *testing.T) {
	orgID := int64(3)
	testeeID := uint64(8)
	risk := "medium"
	got := buildCatalogQuery(evaluationreadmodel.ReportFilter{
		OrgID:        &orgID,
		TesteeID:     &testeeID,
		TesteeIDs:    []uint64{8, 9},
		HighRiskOnly: true,
		ModelCode:    "SDS",
		RiskLevel:    &risk,
	})
	want := bson.M{
		"org_id":     int64(3),
		"testee_id":  bson.M{"$in": []uint64{8, 9}},
		"risk_level": "medium",
		"model_code": "SDS",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("current report query = %#v, want %#v", got, want)
	}
}

func TestReportCatalogIndexesProtectScopeAndStableSort(t *testing.T) {
	indexes := reportCatalogIndexModels()
	if len(indexes) != 7 {
		t.Fatalf("catalog indexes = %d, want 7", len(indexes))
	}
}
