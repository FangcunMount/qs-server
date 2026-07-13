package evaluation

import (
	"strings"
	"testing"
)

func TestWireRequiresModelCatalogPublishedPort(t *testing.T) {
	_, err := Wire(WireInput{})
	if err == nil || !strings.Contains(err.Error(), "modelcatalog published model catalog is required") {
		t.Fatalf("Wire() error = %v, want missing modelcatalog port", err)
	}
}
