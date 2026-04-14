package main

import "testing"

func TestHasAnyActiveAccessRelation_IgnoresCreator(t *testing.T) {
	items := []*TesteeClinicianRelationResponse{
		{
			Relation: &RelationResponse{
				RelationType: "creator",
				IsActive:     true,
			},
		},
	}

	if hasAnyActiveAccessRelation(items) {
		t.Fatalf("expected creator relation to be ignored")
	}
}

func TestHasAnyActiveAccessRelation_CountsAccessGrantRelation(t *testing.T) {
	items := []*TesteeClinicianRelationResponse{
		{
			Relation: &RelationResponse{
				RelationType: "primary",
				IsActive:     true,
			},
		},
	}

	if !hasAnyActiveAccessRelation(items) {
		t.Fatalf("expected primary relation to count as active access relation")
	}
}

func TestBuildTesteeAssignmentJobs_RoundRobinPreservesPosition(t *testing.T) {
	cfg := TesteeAssignmentConfig{Strategy: "round_robin"}
	targets := []clinicianAssignmentTarget{
		{ID: "c1"},
		{ID: "c2"},
	}
	testees := []*ApiserverTesteeResponse{
		{ID: "t1"},
		{ID: "t2"},
		{ID: "t3"},
	}

	jobs := buildTesteeAssignmentJobs(cfg, targets, testees)
	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}
	if jobs[0].Target.ID != "c1" || jobs[1].Target.ID != "c2" || jobs[2].Target.ID != "c1" {
		t.Fatalf("unexpected round-robin targets: %+v", jobs)
	}
}

func TestNormalizeAssignmentWorkers(t *testing.T) {
	if got := normalizeAssignmentWorkers(0, 0); got != defaultAssignmentWorkers {
		t.Fatalf("expected default workers %d, got %d", defaultAssignmentWorkers, got)
	}
	if got := normalizeAssignmentWorkers(8, 3); got != 3 {
		t.Fatalf("expected workers capped by jobs, got %d", got)
	}
	if got := normalizeAssignmentWorkers(2, 10); got != 2 {
		t.Fatalf("expected explicit workers kept, got %d", got)
	}
}
