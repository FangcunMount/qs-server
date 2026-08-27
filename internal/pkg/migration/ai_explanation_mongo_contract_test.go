package migration

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestAIExplanationMongoMigrationOwnsRuntimeCollectionsAndIndexes(t *testing.T) {
	up, err := os.ReadFile("migrations/mongodb/000025_add_ai_explanation_runtime.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []map[string]any
	if err := json.Unmarshal(up, &commands); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"ai_explanation_generations", "ai_explanation_runs", "ai_explanation_artifacts", "ai_explanation_profiles",
		"uk_ai_explanation_generation_semantic_key", "idx_ai_explanation_run_expired_lease",
		"uk_ai_explanation_artifact_generation", "uk_ai_explanation_profile_release", "idx_ai_explanation_profile_selector",
	} {
		if !containsJSONToken(up, required) {
			t.Fatalf("AI explanation Mongo migration does not contain %q", required)
		}
	}
	if containsJSONToken(up, "insert") || containsJSONToken(up, "published") {
		t.Fatal("schema migration must not publish an unvalidated AI explanation Profile")
	}

	down, err := os.ReadFile("migrations/mongodb/000025_add_ai_explanation_runtime.down.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(down, &commands); err != nil {
		t.Fatal(err)
	}
	for _, collection := range []string{"ai_explanation_profiles", "ai_explanation_artifacts", "ai_explanation_runs", "ai_explanation_generations"} {
		if !containsJSONToken(down, collection) {
			t.Fatalf("AI explanation Mongo down migration does not own %q", collection)
		}
	}
}

func TestAIExplanationPromptEvaluationMigrationOwnsImmutableEvidenceCollection(t *testing.T) {
	up, err := os.ReadFile("migrations/mongodb/000026_add_ai_explanation_prompt_evaluations.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []map[string]any
	if err := json.Unmarshal(up, &commands); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"ai_explanation_prompt_evaluations", "uk_ai_explanation_prompt_evaluation_domain",
		"idx_ai_explanation_prompt_evaluation_profile_status", "idx_ai_explanation_prompt_evaluation_release",
		"release.profile.fingerprint", "release.suite.fingerprint", "release.prompt.fingerprint",
		"release.semantic_evaluator.prompt.fingerprint", "release.semantic_evaluator.output_schema.fingerprint",
		"release.semantic_evaluator.provider.fingerprint",
		"uk_ai_explanation_profile_published_selector_slot", "selector_slot_key", "partialFilterExpression",
	} {
		if !containsJSONToken(up, required) {
			t.Fatalf("AI explanation Prompt evaluation migration does not contain %q", required)
		}
	}
	if containsJSONToken(up, "insert") || containsJSONToken(up, "approved") {
		t.Fatal("evaluation schema migration must not fabricate release evidence or publish a Profile")
	}

	down, err := os.ReadFile("migrations/mongodb/000026_add_ai_explanation_prompt_evaluations.down.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(down, &commands); err != nil {
		t.Fatal(err)
	}
	if !containsJSONToken(down, "ai_explanation_prompt_evaluations") {
		t.Fatal("AI explanation Prompt evaluation down migration does not own its collection")
	}
}

func TestAIExplanationPromptEvaluationMigrationFailsClosedOnPreexistingPublishedProfiles(t *testing.T) {
	raw, err := os.ReadFile("driver_mongo.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"aiExplanationEvaluationVersion",
		"= 26",
		"verifyNoPublishedAIExplanationProfiles",
		`bson.M{"status": "published"}`,
		"pre-existing published Profiles",
	} {
		if !strings.Contains(string(raw), required) {
			t.Fatalf("Mongo migration driver is missing AI explanation fail-closed precondition %q", required)
		}
	}
}

func TestAIExplanationPromptEvaluationExecutionMigrationOwnsUniquenessAndLeaseIndexes(t *testing.T) {
	up, err := os.ReadFile("migrations/mongodb/000027_add_ai_explanation_prompt_evaluation_execution.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []map[string]any
	if err := json.Unmarshal(up, &commands); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"active_release_key", "uk_ai_explanation_prompt_evaluation_active_release",
		"active_execution_org_key", "uk_ai_explanation_prompt_evaluation_active_org_execution",
		"execution.phase", "prepared", "execution.lease_expires_at", "idx_ai_explanation_prompt_evaluation_expired_lease",
		"ai_explanation_prompt_evaluation_daily_budgets", "uk_ai_explanation_prompt_evaluation_budget_org_day",
		"reservations.run_id", "uk_ai_explanation_prompt_evaluation_budget_run",
		"partialFilterExpression", "collecting",
	} {
		if !containsJSONToken(up, required) {
			t.Fatalf("AI explanation Prompt evaluation execution migration does not contain %q", required)
		}
	}
	down, err := os.ReadFile("migrations/mongodb/000027_add_ai_explanation_prompt_evaluation_execution.down.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(down, &commands); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"ai_explanation_prompt_evaluation_daily_budgets", "uk_ai_explanation_prompt_evaluation_active_release",
		"uk_ai_explanation_prompt_evaluation_active_org_execution", "idx_ai_explanation_prompt_evaluation_expired_lease",
	} {
		if !containsJSONToken(down, required) {
			t.Fatalf("AI explanation Prompt evaluation execution down migration does not own %q", required)
		}
	}
}

func TestAIExplanationPromptEvaluationExecutionMigrationFailsClosedOnActiveRuns(t *testing.T) {
	raw, err := os.ReadFile("driver_mongo.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"aiExplanationDurableEvaluationVersion = 27",
		"verifyNoActiveAIExplanationPromptEvaluations",
		"active evaluations",
	} {
		if !strings.Contains(string(raw), required) {
			t.Fatalf("Mongo migration driver is missing Prompt evaluation durability precondition %q", required)
		}
	}
}

func TestAIExplanationParticipantCapacityMigrationOwnsDailyLedger(t *testing.T) {
	up, err := os.ReadFile("migrations/mongodb/000028_add_ai_explanation_participant_capacity.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []map[string]any
	if err := json.Unmarshal(up, &commands); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"ai_explanation_participant_daily_budgets", "uk_ai_explanation_participant_budget_org_day",
		"reservations.generation_id", "uk_ai_explanation_participant_budget_generation", "org_id", "budget_day",
	} {
		if !containsJSONToken(up, required) {
			t.Fatalf("AI explanation participant capacity migration does not contain %q", required)
		}
	}

	down, err := os.ReadFile("migrations/mongodb/000028_add_ai_explanation_participant_capacity.down.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(down, &commands); err != nil {
		t.Fatal(err)
	}
	if !containsJSONToken(down, "ai_explanation_participant_daily_budgets") {
		t.Fatal("AI explanation participant capacity down migration does not own its collection")
	}
}

func TestAIExplanationParticipantActiveCapacityMigrationOwnsExactLedger(t *testing.T) {
	up, err := os.ReadFile("migrations/mongodb/000029_add_ai_explanation_participant_active_capacity.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []map[string]any
	if err := json.Unmarshal(up, &commands); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"ai_explanation_participant_active_capacity", "uk_ai_explanation_participant_active_capacity_org",
		"reservations.generation_id", "uk_ai_explanation_participant_active_generation",
		"reservations.run_id", "uk_ai_explanation_participant_active_run",
	} {
		if !containsJSONToken(up, required) {
			t.Fatalf("AI explanation participant active capacity migration does not contain %q", required)
		}
	}
	down, err := os.ReadFile("migrations/mongodb/000029_add_ai_explanation_participant_active_capacity.down.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(down, &commands); err != nil {
		t.Fatal(err)
	}
	if !containsJSONToken(down, "ai_explanation_participant_active_capacity") {
		t.Fatal("AI explanation participant active capacity down migration does not own its collection")
	}
}

func TestAIExplanationParticipantActiveCapacityMigrationFailsClosedOnGeneratingRuns(t *testing.T) {
	raw, err := os.ReadFile("driver_mongo.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"aiExplanationParticipantActiveCapacityVersion = 29",
		"verifyNoGeneratingAIExplanationParticipants",
		"generating participant explanations",
	} {
		if !strings.Contains(string(raw), required) {
			t.Fatalf("Mongo migration driver is missing participant active capacity precondition %q", required)
		}
	}
}

func TestAIExplanationParticipantRetryGovernanceMigrationOwnsAttemptIdentityAndAuthorizationIndexes(t *testing.T) {
	up, err := os.ReadFile("migrations/mongodb/000030_add_ai_explanation_participant_retry_governance.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []map[string]any
	if err := json.Unmarshal(up, &commands); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"uk_ai_explanation_participant_budget_generation", "reservations.reservation_id",
		"uk_ai_explanation_participant_budget_reservation", "retry_authorization.request_id",
		"uk_ai_explanation_run_retry_request", "retry_authorization.event_id", "uk_ai_explanation_run_retry_event",
	} {
		if !containsJSONToken(up, required) {
			t.Fatalf("AI explanation participant retry governance migration does not contain %q", required)
		}
	}
	down, err := os.ReadFile("migrations/mongodb/000030_add_ai_explanation_participant_retry_governance.down.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(down, &commands); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"uk_ai_explanation_run_retry_event", "uk_ai_explanation_run_retry_request", "uk_ai_explanation_participant_budget_reservation", "reservations.generation_id"} {
		if !containsJSONToken(down, required) {
			t.Fatalf("AI explanation participant retry governance down migration does not contain %q", required)
		}
	}
}

func TestAIExplanationParticipantRetryGovernanceMigrationFailsClosedOnExistingBudgets(t *testing.T) {
	raw, err := os.ReadFile("driver_mongo.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"aiExplanationParticipantRetryGovernanceVersion = 30",
		"verifyNoAIExplanationParticipantBudgetReservations",
		"non-empty daily budget ledgers",
	} {
		if !strings.Contains(string(raw), required) {
			t.Fatalf("Mongo migration driver is missing participant retry governance precondition %q", required)
		}
	}
}

func TestAIExplanationRetentionMigrationCreatesExplicitTTLIndexes(t *testing.T) {
	up, err := os.ReadFile("migrations/mongodb/000031_add_ai_explanation_retention_ttl.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []map[string]any
	if err := json.Unmarshal(up, &commands); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"ai_explanation_generations", "ai_explanation_runs", "ai_explanation_artifacts",
		"ai_explanation_prompt_evaluations", "ai_explanation_prompt_evaluation_daily_budgets",
		"ai_explanation_participant_daily_budgets", "expires_at", "ttl_ai_explanation_expires_at",
	} {
		if !containsJSONToken(up, required) {
			t.Fatalf("AI explanation retention migration does not contain %q", required)
		}
	}
	if len(commands) != 6 {
		t.Fatalf("AI explanation retention commands = %d, want 6", len(commands))
	}

	down, err := os.ReadFile("migrations/mongodb/000031_add_ai_explanation_retention_ttl.down.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(down, &commands); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 6 || !containsJSONToken(down, "ttl_ai_explanation_expires_at") {
		t.Fatalf("AI explanation retention down migration is incomplete: %s", down)
	}
}

func TestAIExplanationSubjectExportMigrationOwnsScopedKeysetIndex(t *testing.T) {
	up, err := os.ReadFile("migrations/mongodb/000032_add_ai_explanation_subject_export_index.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []map[string]any
	if err := json.Unmarshal(up, &commands); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"ai_explanation_artifacts", "source.association.org_id", "source.association.testee_id",
		"audience", "generated_at", "domain_id", "idx_ai_explanation_artifact_subject_export",
	} {
		if !containsJSONToken(up, required) {
			t.Fatalf("AI explanation subject export migration does not contain %q", required)
		}
	}
	if len(commands) != 1 {
		t.Fatalf("AI explanation subject export commands = %d, want 1", len(commands))
	}

	down, err := os.ReadFile("migrations/mongodb/000032_add_ai_explanation_subject_export_index.down.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(down, &commands); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || !containsJSONToken(down, "idx_ai_explanation_artifact_subject_export") {
		t.Fatalf("AI explanation subject export down migration is incomplete: %s", down)
	}
}

func containsJSONToken(raw []byte, token string) bool {
	quoted, _ := json.Marshal(token)
	for index := 0; index+len(quoted) <= len(raw); index++ {
		match := true
		for offset := range quoted {
			if raw[index+offset] != quoted[offset] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
