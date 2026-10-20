package interpretation

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestTransactionLifecycleConstructorsBorrowDisconnectedDatabaseWithoutDDL(t *testing.T) {
	client, err := mongo.NewClient() //nolint:staticcheck // Inert disconnected fixture; Connect would start monitoring.
	require.NoError(t, err)
	db := client.Database("original_primary")
	generation := NewGenerationRepositoryWithoutSchema(db)
	run := NewRunRepositoryWithoutSchema(db)
	report := NewReportRepositoryWithoutSchema(db)
	catalog := NewReportCatalogProjectorWithoutSchema(db)
	require.Same(t, db, generation.DB())
	require.Same(t, db, run.DB())
	require.Same(t, db, report.DB())
	require.Same(t, db, catalog.DB())
	// Only the explicitly requested installer tries database work.
	require.Error(t, InstallLifecycleSchema(t.Context(), db))
}
