package retirement

import (
	"errors"
	"slices"
	"testing"
)

func TestHistoricalCoordinatorMissingAdaptersCannotBecomeHistoricalClassOrCAS(t *testing.T) {
	f := coordinatorFixture(t, 8, false)
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.NextPage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if e := c.QualifyAIPage(t.Context(), p, nil); !errors.Is(e, ErrCoordinatorPage) {
		t.Fatal("event page accepted in AI adapter")
	}
	if e := c.QualifyPage(t.Context(), p, &SQLBusinessOwnerBatch{}, nil); e == nil {
		t.Fatal("unprepared SQL batch acquired source coverage")
	}
	if e := c.QualifyPage(t.Context(), p, nil, &MongoHistoricalOwnerBatch{}); e == nil {
		t.Fatal("unprepared Mongo batch acquired source coverage")
	}
	if err = c.QualifyPage(t.Context(), p, nil, nil); err != nil {
		t.Fatal(err)
	}
	coordinatorDrain(t, c)
	rows, err := c.CandidateRange(0, 512)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range rows {
		if v.LocalClassification != "blocked" || v.LocalQualified || len(v.BlockingReasons) == 0 || !slices.Contains(v.RequiredAdapters, "production_source_origin_authentication") || !slices.Contains(v.RequiredAdapters, "all_four_targets_committed_cas_and_independent_readback") {
			t.Fatal("missing closure interpreted as retired proof")
		}
		if v.Source.Database == "mongodb" && !slices.Contains(v.RequiredAdapters, "mongo_original_two_types_to_sql_held_deadletter_and_inbox_adapter") {
			t.Fatal("Mongo two-type SQL responsibilities silently treated complete")
		}
		if v.EventType == "ai.command.start" && !slices.Contains(v.RequiredAdapters, "qs_ai_all_original_runs_model_calls_jobs_leases_and_messages_closure") {
			t.Fatal("delivered incorrectly became AI acceptance")
		}
		if v.BusinessBindingSHA256 != "" {
			t.Fatal("missing actual binding was fabricated")
		}
	}
	r := c.Receipt()
	if r.CASComplete || r.BusinessClosureVerified || r.DropReady {
		t.Fatal("candidate acquired write or DROP permission")
	}
}
func TestHistoricalCoordinatorNoInferenceOfSourceRun(t *testing.T) {
	f := coordinatorFixture(t, 0, false)
	c, e := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if e != nil {
		t.Fatal(e)
	}
	coordinatorDrain(t, c)
	values, _ := c.CandidateRange(0, 128)
	for _, v := range values {
		if v.EventType == "evaluation.requested" || v.EventType == "evaluation.failed" {
			if v.OriginalRunID != "" || v.OriginalAttempt != nil {
				t.Fatal("current run substituted for missing original Run")
			}
		}
	}
}
func TestHistoricalCoordinatorClassifyingLocalGapsDoesNotCreateFinalEvidence(t *testing.T) {
	rows := []HistoricalCandidate{{LocalQualified: true}, {LocalQualified: true, HistoricalGaps: []string{"missing_original_run_identity"}}, {LocalQualified: true, BlockingReasons: []string{"runtime_attempt_ambiguous"}}}
	wants := []string{"candidate_local_verified_requires_joint_closure", "candidate_historical_gap_requires_joint_closure", "blocked"}
	for i := range rows {
		coordinatorClassify(&rows[i])
		if rows[i].LocalClassification != wants[i] {
			t.Fatal("pure historical gap or hard ambiguity misclassified")
		}
	}
	if rows[2].LocalQualified {
		t.Fatal("ambiguity retained local qualification")
	}
}
