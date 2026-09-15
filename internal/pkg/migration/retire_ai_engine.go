package migration

import "context"

// This migration ships only after the separate, restore-verified M5 data cleanup.
const aiEngineRetirementVersion = 35

var aiEngineRetirementCollections = []string{
	"ai_explanation_generations",
	"ai_explanation_runs",
	"ai_explanation_artifacts",
	"ai_explanation_profiles",
	"ai_explanation_prompt_evaluations",
	"ai_explanation_prompt_evaluation_rechecks",
	"ai_explanation_prompt_evaluation_daily_budgets",
	"ai_explanation_participant_daily_budgets",
	"ai_explanation_participant_active_capacity",
}

func (d *MongoDriver) prepareAIEngineRetirement(ctx context.Context, database string, versionBefore uint) error {
	if versionBefore >= aiEngineRetirementVersion {
		return nil
	}
	// Check all objects before creating any empty namespace. Live data must be
	// backed up and removed by the maintenance tool, never by normal startup.
	if err := d.verifyEmptyMongoCollections(ctx, database, aiEngineRetirementCollections, "AI engine"); err != nil {
		return err
	}
	if versionBefore >= 33 {
		// Historical migrations create the namespaces on a fresh database. A
		// cleaned production database needs empty namespaces for Mongo's drop command.
		return d.ensureMongoCollections(ctx, database, aiEngineRetirementCollections)
	}
	return nil
}
