package evaluation

import (
	"reflect"
	"strings"
	"testing"
)

func TestWireRequiresModelCatalogPublishedPort(t *testing.T) {
	_, err := Wire(WireInput{})
	if err == nil || !strings.Contains(err.Error(), "modelcatalog published model catalog is required") {
		t.Fatalf("Wire() error = %v, want missing modelcatalog port", err)
	}
}

func TestEvaluationAssemblyHasNoAssessmentListRedisDependencies(t *testing.T) {
	t.Parallel()

	for typeName, value := range map[string]any{
		"WireInput": WireInput{}, "BootstrapInput": BootstrapInput{}, "Deps": Deps{},
	} {
		typeOf := reflect.TypeOf(value)
		for _, field := range []string{"QueryRedisClient", "QueryCacheBuilder", "MetaRedisClient", "MetaCacheBuilder", "VersionStore"} {
			if _, exists := typeOf.FieldByName(field); exists {
				t.Fatalf("%s still exposes retired assessment-list dependency %s", typeName, field)
			}
		}
	}
}
