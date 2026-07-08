package modelcatalog

import (
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
)

func TestLegacyRegisterNamesRemainStable(t *testing.T) {
	t.Parallel()

	desc := Describe()
	want := []string{
		string(modules.PackageModelCatalog),
		"scale",
		"typologymodel",
	}
	if !reflect.DeepEqual(desc.RegisterNames, want) {
		t.Fatalf("RegisterNames = %v, want %v", desc.RegisterNames, want)
	}
	if !reflect.DeepEqual(desc.LegacyRegisterNames, []string{"scale", "typologymodel"}) {
		t.Fatalf("LegacyRegisterNames = %v", desc.LegacyRegisterNames)
	}
}
