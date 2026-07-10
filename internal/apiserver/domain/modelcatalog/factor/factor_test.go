package factor_test

import (
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestFactorKeepsSlimIdentityShape(t *testing.T) {
	t.Parallel()

	if got := reflect.TypeOf(factor.Factor{}).NumField(); got != 3 {
		t.Fatalf("Factor field count = %d, want 3 identity fields", got)
	}
}
