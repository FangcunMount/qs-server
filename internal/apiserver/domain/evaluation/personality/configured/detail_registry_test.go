package configured

import (
	"strings"
	"testing"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
)

func TestDetailAssemblerRegistryRejectsUnknownAdapter(t *testing.T) {
	_, err := DefaultDetailAssemblerRegistry().Assemble(DetailInput{
		Adapter: modeltypology.DetailAdapterKey("custom_unknown"),
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported detail adapter key") {
		t.Fatalf("Assemble error = %v, want unsupported detail adapter key", err)
	}
}
