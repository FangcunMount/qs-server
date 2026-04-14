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
