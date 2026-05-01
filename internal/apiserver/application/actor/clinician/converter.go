package clinician

import (
	"time"

	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
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

func toClinicianResultFromRow(row *actorreadmodel.ClinicianRow) *ClinicianResult {
	if row == nil {
		return nil
	}
	return &ClinicianResult{
		ID:            row.ID,
		OrgID:         row.OrgID,
		OperatorID:    row.OperatorID,
		Name:          row.Name,
		Department:    row.Department,
		Title:         row.Title,
		ClinicianType: row.ClinicianType,
		EmployeeCode:  row.EmployeeCode,
		IsActive:      row.IsActive,
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

func toAssignedTesteeResultFromRow(row *actorreadmodel.TesteeRow) *AssignedTesteeResult {
	if row == nil {
		return nil
	}
	return &AssignedTesteeResult{
		ID:         row.ID,
		OrgID:      row.OrgID,
		ProfileID:  row.ProfileID,
		Name:       row.Name,
		Gender:     row.Gender,
		Birthday:   row.Birthday,
		Age:        ageFromBirthday(row.Birthday),
		Tags:       append([]string(nil), row.Tags...),
		Source:     row.Source,
		IsKeyFocus: row.IsKeyFocus,
	}
}

func toRelationResultFromRow(row *actorreadmodel.RelationRow) *RelationResult {
	if row == nil {
		return nil
	}
	return &RelationResult{
		ID:           row.ID,
		OrgID:        row.OrgID,
		ClinicianID:  row.ClinicianID,
		TesteeID:     row.TesteeID,
		RelationType: row.RelationType,
		SourceType:   row.SourceType,
		SourceID:     row.SourceID,
		IsActive:     row.IsActive,
		BoundAt:      row.BoundAt,
		UnboundAt:    row.UnboundAt,
	}
}

func ageFromBirthday(birthday *time.Time) int {
	if birthday == nil {
		return 0
	}
	now := time.Now()
	age := now.Year() - birthday.Year()
	if now.YearDay() < birthday.YearDay() {
		age--
	}
	return age
}

func relationTypesToStrings(items []domainRelation.RelationType) []string {
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, string(item))
	}
	return result
}
