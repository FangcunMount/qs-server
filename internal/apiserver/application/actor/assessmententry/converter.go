package assessmententry

import (
	domainAssessmentEntry "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/assessmententry"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
)

func toAssessmentEntryResult(item *domainAssessmentEntry.AssessmentEntry) *AssessmentEntryResult {
	if item == nil {
		return nil
	}

	return &AssessmentEntryResult{
		ID:            item.ID().Uint64(),
		OrgID:         item.OrgID(),
		ClinicianID:   item.ClinicianID().Uint64(),
		Token:         item.Token(),
		TargetType:    string(item.TargetType()),
		TargetCode:    item.TargetCode(),
		TargetVersion: item.TargetVersion(),
		IsActive:      item.IsActive(),
		ExpiresAt:     item.ExpiresAt(),
	}
}

func toAssessmentEntryResultFromRow(row *actorreadmodel.AssessmentEntryRow) *AssessmentEntryResult {
	if row == nil {
		return nil
	}
	return &AssessmentEntryResult{
		ID:            row.ID,
		OrgID:         row.OrgID,
		ClinicianID:   row.ClinicianID,
		Token:         row.Token,
		TargetType:    row.TargetType,
		TargetCode:    row.TargetCode,
		TargetVersion: row.TargetVersion,
		IsActive:      row.IsActive,
		ExpiresAt:     row.ExpiresAt,
	}
}

func toClinicianSummaryResult(item *domainClinician.Clinician) *ClinicianSummaryResult {
	if item == nil {
		return nil
	}

	return &ClinicianSummaryResult{
		ID:            item.ID().Uint64(),
		OperatorID:    item.OperatorID(),
		Name:          item.Name(),
		Department:    item.Department(),
		Title:         item.Title(),
		ClinicianType: string(item.ClinicianType()),
	}
}

func toTesteeSummaryResult(item *domainTestee.Testee) *TesteeSummaryResult {
	if item == nil {
		return nil
	}

	return &TesteeSummaryResult{
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

func toRelationSummaryResult(item *domainRelation.ClinicianTesteeRelation) *RelationSummaryResult {
	if item == nil {
		return nil
	}

	return &RelationSummaryResult{
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
