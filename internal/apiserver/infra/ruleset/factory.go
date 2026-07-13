package ruleset

import (
	"context"

	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// NewDefaultStaticCatalog builds embedded typology fixtures for tests and
// one-off tooling. Production composition always reads published models.
func NewDefaultStaticCatalog() (port.Catalog, error) {
	ruleSets, err := DefaultEmbeddedRuleSets(context.Background())
	if err != nil {
		return nil, err
	}
	return NewStaticCompositeCatalog(ruleSets), nil
}
