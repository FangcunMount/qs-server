package plan

import (
	"fmt"
	"strings"
	"testing"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"gorm.io/gorm"
)

func TestPlanModuleIdentifiesMissingRequiredDependency(t *testing.T) {
	for name, deps := range map[string]Deps{
		"MySQL database":          {},
		"published model catalog": {MySQLDB: &gorm.DB{}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := normalizeDeps(deps)
			if !errors.IsCode(err, code.ErrModuleInitializationFailed) || !strings.Contains(fmt.Sprintf("%-v", err), name+" is required") {
				t.Fatalf("missing module dependency must be identified: %-v", err)
			}
		})
	}
}
