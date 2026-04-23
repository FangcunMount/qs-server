package assembler

import (
	"testing"

	"go.mongodb.org/mongo-driver/mongo"
)

func TestNormalizeSurveyModuleDepsRequiresMongoDB(t *testing.T) {
	t.Parallel()

	if _, err := normalizeSurveyModuleDeps(SurveyModuleDeps{}); err == nil {
		t.Fatal("normalizeSurveyModuleDeps() error = nil, want missing Mongo error")
	}
}

func TestNormalizeSurveyModuleDepsDefaultsEventPublisher(t *testing.T) {
	t.Parallel()

	deps, err := normalizeSurveyModuleDeps(SurveyModuleDeps{MongoDB: &mongo.Database{}})
	if err != nil {
		t.Fatalf("normalizeSurveyModuleDeps() error = %v", err)
	}
	if deps.EventPublisher == nil {
		t.Fatal("EventPublisher = nil, want Nop publisher")
	}
}
