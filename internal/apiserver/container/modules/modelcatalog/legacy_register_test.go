package modelcatalog

import (
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
)

func TestRegisterNamesExposeAggregateOnly(t *testing.T) {
	t.Parallel()

	desc := Describe()
	want := []string{string(modules.PackageModelCatalog)}
	if !reflect.DeepEqual(desc.RegisterNames, want) {
		t.Fatalf("RegisterNames = %v, want %v", desc.RegisterNames, want)
	}
}
