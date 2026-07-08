package option_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/option"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

func TestDefaultOptionsExposePresentationFields(t *testing.T) {
	t.Parallel()

	opts := option.DefaultOptions()
	if len(opts) == 0 {
		t.Fatal("expected catalog options")
	}
	found := false
	for _, item := range opts {
		if item.Kind == binding.KindTypology {
			found = true
			if item.APIKind != "typology" || item.DisplayName == "" {
				t.Fatalf("option = %#v", item)
			}
		}
	}
	if !found {
		t.Fatal("typology option missing")
	}
}
