package clinician

import (
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

func toClinicianResult(item *domainClinician.Clinician) *ClinicianResult {
	if item == nil {
		return nil
	}

	return &ClinicianResult{
		ID:                   item.ID().Uint64(),
		OrgID:                item.OrgID(),
		OperatorID:           item.OperatorID(),
		Name:                 item.Name(),
		Department:           item.Department(),
		Title:                item.Title(),
		ClinicianType:        string(item.ClinicianType()),
		EmployeeCode:         item.EmployeeCode(),
		IsActive:             item.IsActive(),
		AssignedTesteeCount:  0,
		AssessmentEntryCount: 0,
	}
}

func toRelationResult(item *domainRelation.ClinicianTesteeRelation) *RelationResult {
	if item == nil {
		return nil
	}

	return &RelationResult{
		ID:           item.ID().Uint64(),
		OrgID:        item.OrgID(),
		ClinicianID:  item.ClinicianID().Uint64(),
		TesteeID:     item.TesteeID().Uint64(),
		RelationType: string(item.RelationType()),
		SourceType:   string(item.SourceType()),
		SourceID:     item.SourceID(),
		IsActive:     item.IsActive(),
		BoundAt:      item.BoundAt(),
		UnboundAt:    item.UnboundAt(),
	}
}

func toAssignedTesteeResult(item *domainTestee.Testee) *AssignedTesteeResult {
	if item == nil {
		return nil
	}

	return &AssignedTesteeResult{
		ID:         item.ID().Uint64(),
		OrgID:      item.OrgID(),
		ProfileID:  item.ProfileID(),
		Name:       item.Name(),
		Gender:     int8(item.Gender()),
		Birthday:   item.Birthday(),
		Age:        item.GetAge(),
		Tags:       item.TagsAsStrings(),
		Source:     item.Source(),
		IsKeyFocus: item.IsKeyFocus(),
	}
}
