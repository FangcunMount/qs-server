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

	jobs := buildTesteeAssignmentJobs(cfg, targets, testees, 0)
	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}
	if jobs[0].Target.ID != "c1" || jobs[1].Target.ID != "c2" || jobs[2].Target.ID != "c1" {
		t.Fatalf("unexpected round-robin targets: %+v", jobs)
	}
}

func TestBuildTesteeAssignmentJobs_RandomIsStable(t *testing.T) {
	cfg := TesteeAssignmentConfig{Key: "seed_all_pool", Strategy: "random"}
	targets := []clinicianAssignmentTarget{
		{ID: "c1"},
		{ID: "c2"},
		{ID: "c3"},
	}
	testees := []*ApiserverTesteeResponse{
		{ID: "t1"},
		{ID: "t2"},
		{ID: "t3"},
		{ID: "t4"},
	}

	jobsA := buildTesteeAssignmentJobs(cfg, targets, testees, 0)
	jobsB := buildTesteeAssignmentJobs(cfg, targets, testees, 0)
	if len(jobsA) != len(jobsB) {
		t.Fatalf("expected same number of jobs, got %d and %d", len(jobsA), len(jobsB))
	}
	for i := range jobsA {
		if jobsA[i].Target.ID != jobsB[i].Target.ID {
			t.Fatalf("expected stable random assignment at index %d, got %s and %s", i, jobsA[i].Target.ID, jobsB[i].Target.ID)
		}
	}
}

func TestBuildTesteeAssignmentJobs_RoundRobinPreservesGlobalOffset(t *testing.T) {
	cfg := TesteeAssignmentConfig{Strategy: "round_robin"}
	targets := []clinicianAssignmentTarget{
		{ID: "c1"},
		{ID: "c2"},
		{ID: "c3"},
	}
	testees := []*ApiserverTesteeResponse{
		{ID: "t4"},
		{ID: "t5"},
	}

	jobs := buildTesteeAssignmentJobs(cfg, targets, testees, 3)
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	if jobs[0].Target.ID != "c1" || jobs[1].Target.ID != "c2" {
		t.Fatalf("unexpected round-robin targets with global offset: %+v", jobs)
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
