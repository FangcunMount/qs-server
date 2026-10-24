package survey

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestSurveyRuntimeInfraDoesNotOwnModelCatalogRepository(t *testing.T) {
	content, err := os.ReadFile("survey_runtime_infra.go")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(content), "Model"+"Repository") {
		t.Fatal("SurveyRuntimeInfra must not own a draft assessment-model repository")
	}
}

func TestSurveyRuntimeBorrowsInstalledSchemaWithoutDatabaseIO(t *testing.T) {
	client, err := mongo.NewClient() //nolint:staticcheck // Disconnected fixture; no monitoring or network.
	require.NoError(t, err)
	db := client.Database("original_primary")
	infra, err := EnsureSurveyRuntimeInfra(SurveyRuntimeInfraDeps{MongoDB: db, BorrowInstalledSchema: true})
	require.NoError(t, err)
	require.Same(t, db, infra.AnswerSheetRepo.DB())
	cached, err := EnsureSurveyRuntimeInfraCached(infra, SurveyRuntimeInfraDeps{MongoDB: db, BorrowInstalledSchema: true})
	require.NoError(t, err)
	require.Same(t, infra, cached)
	// The unchanged legacy path still attempts its explicit constructor indexes.
	_, err = EnsureSurveyRuntimeInfra(SurveyRuntimeInfraDeps{MongoDB: db})
	require.Error(t, err)
}
