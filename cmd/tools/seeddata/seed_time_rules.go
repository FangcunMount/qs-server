package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	seedRelationPrimaryOffset      = 2 * time.Hour
	seedRelationAttendingOffset    = 4 * time.Hour
	seedRelationCollaboratorOffset = 6 * time.Hour
	seedEntryResolveMinDelay       = 24 * time.Hour
	seedEntryResolveOffset         = 30 * time.Minute
	seedEntryIntakeOffset          = 10 * time.Minute
	seedEntryAttendingOffset       = 1 * time.Minute
	seedEntryAssessmentOffset      = 20 * time.Minute
	seedAssessmentInterpretOffset  = 30 * time.Second
	seedClinicianCreatedLead       = 7 * 24 * time.Hour
	seedStaffCreatedLead           = 24 * time.Hour
	seedEntryFlowPageSize          = 100
	seedEntryFlowDefaultMaxIntakes = 5
	seedByEntryDefaultMaxCount     = 5
	seedAssessmentPollInterval     = 500 * time.Millisecond
	seedAssessmentPollTimeout      = 20 * time.Second
)

func deriveRelationBoundAt(testeeCreatedAt time.Time, relationType string) (time.Time, error) {
	base := testeeCreatedAt.Round(0)
	if base.IsZero() {
		return time.Time{}, fmt.Errorf("testee created_at is zero")
	}

	switch strings.ToLower(strings.TrimSpace(relationType)) {
	case "primary":
		return base.Add(seedRelationPrimaryOffset), nil
	case "attending", "assigned":
		return base.Add(seedRelationAttendingOffset), nil
	case "collaborator":
		return base.Add(seedRelationCollaboratorOffset), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported relation type %q", relationType)
	}
}

func deriveEntryResolveAt(entryCreatedAt, testeeCreatedAt time.Time) time.Time {
	entryAnchor := entryCreatedAt.Round(0).Add(seedEntryResolveOffset)
	testeeAnchor := testeeCreatedAt.Round(0).Add(seedEntryResolveMinDelay)
	if entryAnchor.Before(testeeAnchor) {
		return testeeAnchor
	}
	return entryAnchor
}

func deriveEntryIntakeAt(resolveAt time.Time) time.Time {
	return resolveAt.Round(0).Add(seedEntryIntakeOffset)
}

func deriveEntryAccessRelationAt(intakeAt time.Time) time.Time {
	return intakeAt.Round(0).Add(seedEntryAttendingOffset)
}

func deriveEntryAssessmentSubmitAt(intakeAt time.Time) time.Time {
	return intakeAt.Round(0).Add(seedEntryAssessmentOffset)
}

func deriveAssessmentInterpretAt(submittedAt time.Time) time.Time {
	return submittedAt.Round(0).Add(seedAssessmentInterpretOffset)
}

func deriveClinicianCreatedAt(firstBoundAt time.Time) time.Time {
	if firstBoundAt.IsZero() {
		return time.Time{}
	}
	return firstBoundAt.Round(0).Add(-seedClinicianCreatedLead)
}

func deriveStaffCreatedAt(clinicianCreatedAt time.Time) time.Time {
	if clinicianCreatedAt.IsZero() {
		return time.Time{}
	}
	return clinicianCreatedAt.Round(0).Add(-seedStaffCreatedLead)
}

func normalizeMaxIntakesPerEntry(value int) int {
	if value <= 0 {
		return seedEntryFlowDefaultMaxIntakes
	}
	return value
}

func normalizeMaxAssessmentsPerEntry(value int) int {
	if value <= 0 {
		return seedByEntryDefaultMaxCount
	}
	return value
}

func flexibleIDSet(values []FlexibleID) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value.IsZero() {
			continue
		}
		result[value.String()] = struct{}{}
	}
	return result
}

func stringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result[value] = struct{}{}
	}
	return result
}

func sortClinicianRelationsByTesteeCreatedAt(items []*ClinicianRelationResponse) {
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		if left == nil || left.Testee == nil {
			return false
		}
		if right == nil || right.Testee == nil {
			return true
		}
		if !left.Testee.CreatedAt.Equal(right.Testee.CreatedAt) {
			return left.Testee.CreatedAt.Before(right.Testee.CreatedAt)
		}
		return parseID(left.Testee.ID) < parseID(right.Testee.ID)
	})
}

func isAssessmentEntryRelation(item *RelationResponse, entryID string) bool {
	if item == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(item.SourceType), "assessment_entry") {
		return false
	}
	if item.SourceID == nil {
		return false
	}
	return strings.TrimSpace(*item.SourceID) == strings.TrimSpace(entryID)
}

func isAccessGrantRelationType(relationType string) bool {
	switch strings.ToLower(strings.TrimSpace(relationType)) {
	case "primary", "attending", "assigned", "collaborator":
		return true
	default:
		return false
	}
}
