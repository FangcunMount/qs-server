package actor

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/errors"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

type readModel struct {
	mysql.BaseRepository[*TesteePO]
}

func NewReadModel(db *gorm.DB, opts ...mysql.BaseRepositoryOptions) actorreadmodel.ReadModel {
	return &readModel{BaseRepository: mysql.NewBaseRepository[*TesteePO](db, opts...)}
}

func (r *readModel) GetTestee(ctx context.Context, id uint64) (*actorreadmodel.TesteeRow, error) {
	var po TesteePO
	err := r.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return nil, err
	}
	row := testeeRowFromPO(&po)
	return &row, nil
}

func (r *readModel) FindTesteeByProfile(ctx context.Context, orgID int64, profileID uint64) (*actorreadmodel.TesteeRow, error) {
	var po TesteePO
	err := r.WithContext(ctx).
		Where("org_id = ? AND profile_id = ? AND deleted_at IS NULL", orgID, profileID).
		First(&po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return nil, err
	}
	row := testeeRowFromPO(&po)
	return &row, nil
}

func (r *readModel) ListTestees(ctx context.Context, filter actorreadmodel.TesteeFilter) ([]actorreadmodel.TesteeRow, error) {
	if filter.RestrictToAccessScope && len(filter.AccessibleTesteeIDs) == 0 {
		return []actorreadmodel.TesteeRow{}, nil
	}
	var pos []*TesteePO
	err := r.applyTesteeFilter(r.WithContext(ctx), filter).
		Order("created_at DESC").
		Order("id DESC").
		Offset(filter.Offset).
		Limit(filter.Limit).
		Find(&pos).Error
	if err != nil {
		return nil, err
	}
	return testeeRowsFromPOs(pos), nil
}

func (r *readModel) CountTestees(ctx context.Context, filter actorreadmodel.TesteeFilter) (int64, error) {
	if filter.RestrictToAccessScope && len(filter.AccessibleTesteeIDs) == 0 {
		return 0, nil
	}
	var count int64
	err := r.applyTesteeFilter(r.WithContext(ctx).Model(&TesteePO{}), filter).Count(&count).Error
	return count, err
}

func (r *readModel) ListTesteesByProfileIDs(ctx context.Context, profileIDs []uint64, offset, limit int) ([]actorreadmodel.TesteeRow, error) {
	if len(profileIDs) == 0 {
		return []actorreadmodel.TesteeRow{}, nil
	}
	var pos []*TesteePO
	err := r.WithContext(ctx).
		Where("profile_id IN ? AND deleted_at IS NULL", profileIDs).
		Order("created_at DESC").
		Order("id DESC").
		Offset(offset).
		Limit(limit).
		Find(&pos).Error
	if err != nil {
		return nil, err
	}
	return testeeRowsFromPOs(pos), nil
}

func (r *readModel) CountTesteesByProfileIDs(ctx context.Context, profileIDs []uint64) (int64, error) {
	if len(profileIDs) == 0 {
		return 0, nil
	}
	var count int64
	err := r.WithContext(ctx).
		Model(&TesteePO{}).
		Where("profile_id IN ? AND deleted_at IS NULL", profileIDs).
		Count(&count).Error
	return count, err
}

func (r *readModel) applyTesteeFilter(query *gorm.DB, filter actorreadmodel.TesteeFilter) *gorm.DB {
	query = query.Where("org_id = ? AND deleted_at IS NULL", filter.OrgID)
	if filter.RestrictToAccessScope {
		query = query.Where("id IN ?", filter.AccessibleTesteeIDs)
	}
	if filter.Name != "" {
		query = query.Where("name LIKE ?", "%"+filter.Name+"%")
	}
	if filter.KeyFocus != nil {
		query = query.Where("is_key_focus = ?", *filter.KeyFocus)
	}
	for _, tag := range filter.Tags {
		query = query.Where("JSON_CONTAINS(tags, ?)", `"`+tag+`"`)
	}
	if filter.CreatedAtStart != nil {
		query = query.Where("created_at >= ?", *filter.CreatedAtStart)
	}
	if filter.CreatedAtEnd != nil {
		query = query.Where("created_at < ?", *filter.CreatedAtEnd)
	}
	return query
}

func (r *readModel) GetOperator(ctx context.Context, id uint64) (*actorreadmodel.OperatorRow, error) {
	var po OperatorPO
	err := r.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "operator not found")
		}
		return nil, err
	}
	row := operatorRowFromPO(&po)
	return &row, nil
}

func (r *readModel) FindOperatorByUser(ctx context.Context, orgID int64, userID int64) (*actorreadmodel.OperatorRow, error) {
	var po OperatorPO
	err := r.WithContext(ctx).
		Where("org_id = ? AND user_id = ? AND deleted_at IS NULL", orgID, userID).
		First(&po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "operator not found")
		}
		return nil, err
	}
	row := operatorRowFromPO(&po)
	return &row, nil
}

func (r *readModel) ListOperators(ctx context.Context, filter actorreadmodel.OperatorFilter) ([]actorreadmodel.OperatorRow, error) {
	var pos []*OperatorPO
	query := r.WithContext(ctx).Where("org_id = ? AND deleted_at IS NULL", filter.OrgID)
	if filter.Role != "" {
		query = query.Where("JSON_CONTAINS(roles, ?)", `"`+filter.Role+`"`)
	}
	err := query.Order("id DESC").Offset(filter.Offset).Limit(filter.Limit).Find(&pos).Error
	if err != nil {
		return nil, err
	}
	return operatorRowsFromPOs(pos), nil
}

func (r *readModel) CountOperators(ctx context.Context, orgID int64) (int64, error) {
	var count int64
	err := r.WithContext(ctx).
		Model(&OperatorPO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&count).Error
	return count, err
}

func (r *readModel) GetClinician(ctx context.Context, id uint64) (*actorreadmodel.ClinicianRow, error) {
	var po ClinicianPO
	err := r.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "clinician not found")
		}
		return nil, err
	}
	row := clinicianRowFromPO(&po)
	return &row, nil
}

func (r *readModel) FindClinicianByOperator(ctx context.Context, orgID int64, operatorID uint64) (*actorreadmodel.ClinicianRow, error) {
	var po ClinicianPO
	tx := r.WithContext(ctx).
		Where("org_id = ? AND operator_id = ? AND deleted_at IS NULL", orgID, operatorID).
		Limit(1).
		Find(&po)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		return nil, errors.WithCode(code.ErrUserNotFound, "clinician not found")
	}
	row := clinicianRowFromPO(&po)
	return &row, nil
}

func (r *readModel) ListClinicians(ctx context.Context, filter actorreadmodel.ClinicianFilter) ([]actorreadmodel.ClinicianRow, error) {
	var pos []*ClinicianPO
	err := r.WithContext(ctx).
		Where("org_id = ? AND deleted_at IS NULL", filter.OrgID).
		Order("id DESC").
		Offset(filter.Offset).
		Limit(filter.Limit).
		Find(&pos).Error
	if err != nil {
		return nil, err
	}
	return clinicianRowsFromPOs(pos), nil
}

func (r *readModel) CountClinicians(ctx context.Context, orgID int64) (int64, error) {
	var count int64
	err := r.WithContext(ctx).
		Model(&ClinicianPO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&count).Error
	return count, err
}

func (r *readModel) ListAssignedTestees(ctx context.Context, filter actorreadmodel.RelationFilter) ([]actorreadmodel.TesteeRow, int64, error) {
	relationRows, total, err := r.listRelationPOs(ctx, filter, true)
	if err != nil {
		return nil, 0, err
	}
	testeeIDs := make([]uint64, 0, len(relationRows))
	for _, relation := range relationRows {
		testeeIDs = append(testeeIDs, uint64(relation.TesteeID))
	}
	testeesByID, err := r.loadTesteeRowsByID(ctx, testeeIDs)
	if err != nil {
		return nil, 0, err
	}
	rows := make([]actorreadmodel.TesteeRow, 0, len(relationRows))
	for _, relation := range relationRows {
		if item, ok := testeesByID[uint64(relation.TesteeID)]; ok {
			rows = append(rows, item)
		}
	}
	return rows, total, nil
}

func (r *readModel) ListActiveTesteeIDsByClinician(ctx context.Context, orgID int64, clinicianID uint64, relationTypes []string) ([]uint64, error) {
	var rawIDs []uint64
	query := r.WithContext(ctx).
		Model(&ClinicianRelationPO{}).
		Where("org_id = ? AND clinician_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, clinicianID, true)
	if len(relationTypes) > 0 {
		query = query.Where("relation_type IN ?", relationTypes)
	}
	err := query.Order("bound_at DESC, id DESC").Pluck("testee_id", &rawIDs).Error
	return rawIDs, err
}

func (r *readModel) ListTesteeRelations(ctx context.Context, filter actorreadmodel.RelationFilter) ([]actorreadmodel.TesteeRelationRow, error) {
	relationRows, _, err := r.listRelationPOs(ctx, filter, false)
	if err != nil {
		return nil, err
	}
	clinicianIDs := make([]uint64, 0, len(relationRows))
	for _, relation := range relationRows {
		clinicianIDs = append(clinicianIDs, uint64(relation.ClinicianID))
	}
	cliniciansByID, err := r.loadClinicianRowsByID(ctx, clinicianIDs)
	if err != nil {
		return nil, err
	}
	rows := make([]actorreadmodel.TesteeRelationRow, 0, len(relationRows))
	for _, relation := range relationRows {
		clinicianRow, ok := cliniciansByID[uint64(relation.ClinicianID)]
		if !ok {
			continue
		}
		rows = append(rows, actorreadmodel.TesteeRelationRow{
			Relation:  relationRowFromPO(relation),
			Clinician: clinicianRow,
		})
	}
	return rows, nil
}

func (r *readModel) ListClinicianRelations(ctx context.Context, filter actorreadmodel.RelationFilter) ([]actorreadmodel.ClinicianRelationRow, int64, error) {
	relationRows, total, err := r.listRelationPOs(ctx, filter, false)
	if err != nil {
		return nil, 0, err
	}
	testeeIDs := make([]uint64, 0, len(relationRows))
	for _, relation := range relationRows {
		testeeIDs = append(testeeIDs, uint64(relation.TesteeID))
	}
	testeesByID, err := r.loadTesteeRowsByID(ctx, testeeIDs)
	if err != nil {
		return nil, 0, err
	}
	rows := make([]actorreadmodel.ClinicianRelationRow, 0, len(relationRows))
	for _, relation := range relationRows {
		testeeRow, ok := testeesByID[uint64(relation.TesteeID)]
		if !ok {
			continue
		}
		rows = append(rows, actorreadmodel.ClinicianRelationRow{
			Relation: relationRowFromPO(relation),
			Testee:   testeeRow,
		})
	}
	return rows, total, nil
}

func (r *readModel) HasActiveRelationForTestee(ctx context.Context, orgID int64, clinicianID, testeeID uint64, relationTypes []string) (bool, error) {
	var count int64
	query := r.WithContext(ctx).
		Model(&ClinicianRelationPO{}).
		Where(
			"org_id = ? AND clinician_id = ? AND testee_id = ? AND is_active = ? AND deleted_at IS NULL",
			orgID,
			clinicianID,
			testeeID,
			true,
		)
	if len(relationTypes) > 0 {
		query = query.Where("relation_type IN ?", relationTypes)
	}
	err := query.Count(&count).Error
	return count > 0, err
}

func (r *readModel) listRelationPOs(ctx context.Context, filter actorreadmodel.RelationFilter, countActiveOnly bool) ([]*ClinicianRelationPO, int64, error) {
	var pos []*ClinicianRelationPO
	query := r.WithContext(ctx).
		Where("org_id = ? AND deleted_at IS NULL", filter.OrgID)
	if filter.ClinicianID > 0 {
		query = query.Where("clinician_id = ?", filter.ClinicianID)
	}
	if filter.TesteeID > 0 {
		query = query.Where("testee_id = ?", filter.TesteeID)
	}
	if filter.ActiveOnly {
		query = query.Where("is_active = ?", true)
	}
	if len(filter.RelationTypes) > 0 {
		query = query.Where("relation_type IN ?", filter.RelationTypes)
	}

	countQuery := query.Session(&gorm.Session{})
	var total int64
	if filter.ActiveOnly || countActiveOnly {
		if err := countQuery.Model(&ClinicianRelationPO{}).Count(&total).Error; err != nil {
			return nil, 0, err
		}
	}

	err := query.Order("bound_at DESC, id DESC").Offset(filter.Offset).Limit(filter.Limit).Find(&pos).Error
	if err != nil {
		return nil, 0, err
	}
	if !filter.ActiveOnly && !countActiveOnly {
		total = int64(len(pos))
	}
	return pos, total, nil
}

func (r *readModel) ListAssessmentEntriesByClinician(ctx context.Context, filter actorreadmodel.AssessmentEntryFilter) ([]actorreadmodel.AssessmentEntryRow, error) {
	var pos []*AssessmentEntryPO
	err := r.WithContext(ctx).
		Where("org_id = ? AND clinician_id = ? AND deleted_at IS NULL", filter.OrgID, filter.ClinicianID).
		Order("id DESC").
		Offset(filter.Offset).
		Limit(filter.Limit).
		Find(&pos).Error
	if err != nil {
		return nil, err
	}
	return assessmentEntryRowsFromPOs(pos), nil
}

func (r *readModel) CountAssessmentEntriesByClinician(ctx context.Context, orgID int64, clinicianID uint64) (int64, error) {
	var count int64
	err := r.WithContext(ctx).
		Model(&AssessmentEntryPO{}).
		Where("org_id = ? AND clinician_id = ? AND deleted_at IS NULL", orgID, clinicianID).
		Count(&count).Error
	return count, err
}

func (r *readModel) GetAssessmentEntryTitle(ctx context.Context, id uint64) (string, error) {
	var po AssessmentEntryPO
	err := r.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errors.WithCode(code.ErrUserNotFound, "assessment entry not found")
		}
		return "", err
	}
	if po.TargetVersion != nil && *po.TargetVersion != "" {
		return fmt.Sprintf("%s:%s@%s", po.TargetType, po.TargetCode, *po.TargetVersion), nil
	}
	return fmt.Sprintf("%s:%s", po.TargetType, po.TargetCode), nil
}

func (r *readModel) loadTesteeRowsByID(ctx context.Context, ids []uint64) (map[uint64]actorreadmodel.TesteeRow, error) {
	if len(ids) == 0 {
		return map[uint64]actorreadmodel.TesteeRow{}, nil
	}
	var pos []*TesteePO
	err := r.WithContext(ctx).Where("id IN ? AND deleted_at IS NULL", uniqueUint64(ids)).Find(&pos).Error
	if err != nil {
		return nil, err
	}
	result := make(map[uint64]actorreadmodel.TesteeRow, len(pos))
	for _, po := range pos {
		row := testeeRowFromPO(po)
		result[row.ID] = row
	}
	return result, nil
}

func (r *readModel) loadClinicianRowsByID(ctx context.Context, ids []uint64) (map[uint64]actorreadmodel.ClinicianRow, error) {
	if len(ids) == 0 {
		return map[uint64]actorreadmodel.ClinicianRow{}, nil
	}
	var pos []*ClinicianPO
	err := r.WithContext(ctx).Where("id IN ? AND deleted_at IS NULL", uniqueUint64(ids)).Find(&pos).Error
	if err != nil {
		return nil, err
	}
	result := make(map[uint64]actorreadmodel.ClinicianRow, len(pos))
	for _, po := range pos {
		row := clinicianRowFromPO(po)
		result[row.ID] = row
	}
	return result, nil
}

func testeeRowFromPO(po *TesteePO) actorreadmodel.TesteeRow {
	row := actorreadmodel.TesteeRow{
		ID:               uint64(po.ID),
		OrgID:            po.OrgID,
		ProfileID:        po.ProfileID,
		Name:             po.Name,
		Gender:           po.Gender,
		Birthday:         po.Birthday,
		CreatedAt:        po.CreatedAt,
		UpdatedAt:        po.UpdatedAt,
		Tags:             append([]string(nil), po.Tags...),
		Source:           po.Source,
		IsKeyFocus:       po.IsKeyFocus,
		LastAssessmentAt: po.LastAssessmentAt,
		TotalAssessments: po.TotalAssessments,
	}
	if po.LastRiskLevel != nil {
		row.LastRiskLevel = *po.LastRiskLevel
	}
	return row
}

func testeeRowsFromPOs(pos []*TesteePO) []actorreadmodel.TesteeRow {
	rows := make([]actorreadmodel.TesteeRow, 0, len(pos))
	for _, po := range pos {
		rows = append(rows, testeeRowFromPO(po))
	}
	return rows
}

func operatorRowFromPO(po *OperatorPO) actorreadmodel.OperatorRow {
	return actorreadmodel.OperatorRow{
		ID:       uint64(po.ID),
		OrgID:    po.OrgID,
		UserID:   po.UserID,
		Roles:    append([]string(nil), po.Roles...),
		Name:     po.Name,
		Email:    po.Email,
		Phone:    po.Phone,
		IsActive: po.IsActive,
	}
}

func operatorRowsFromPOs(pos []*OperatorPO) []actorreadmodel.OperatorRow {
	rows := make([]actorreadmodel.OperatorRow, 0, len(pos))
	for _, po := range pos {
		rows = append(rows, operatorRowFromPO(po))
	}
	return rows
}

func clinicianRowFromPO(po *ClinicianPO) actorreadmodel.ClinicianRow {
	employeeCode := ""
	if po.EmployeeCode != nil {
		employeeCode = *po.EmployeeCode
	}
	return actorreadmodel.ClinicianRow{
		ID:            uint64(po.ID),
		OrgID:         po.OrgID,
		OperatorID:    po.OperatorID,
		Name:          po.Name,
		Department:    po.Department,
		Title:         po.Title,
		ClinicianType: po.ClinicianType,
		EmployeeCode:  employeeCode,
		IsActive:      po.IsActive,
	}
}

func clinicianRowsFromPOs(pos []*ClinicianPO) []actorreadmodel.ClinicianRow {
	rows := make([]actorreadmodel.ClinicianRow, 0, len(pos))
	for _, po := range pos {
		rows = append(rows, clinicianRowFromPO(po))
	}
	return rows
}

func relationRowFromPO(po *ClinicianRelationPO) actorreadmodel.RelationRow {
	return actorreadmodel.RelationRow{
		ID:           uint64(po.ID),
		OrgID:        po.OrgID,
		ClinicianID:  uint64(po.ClinicianID),
		TesteeID:     uint64(po.TesteeID),
		RelationType: po.RelationType,
		SourceType:   po.SourceType,
		SourceID:     po.SourceID,
		IsActive:     po.IsActive,
		BoundAt:      po.BoundAt,
		UnboundAt:    po.UnboundAt,
	}
}

func assessmentEntryRowsFromPOs(pos []*AssessmentEntryPO) []actorreadmodel.AssessmentEntryRow {
	rows := make([]actorreadmodel.AssessmentEntryRow, 0, len(pos))
	for _, po := range pos {
		targetVersion := ""
		if po.TargetVersion != nil {
			targetVersion = *po.TargetVersion
		}
		rows = append(rows, actorreadmodel.AssessmentEntryRow{
			ID:            uint64(po.ID),
			OrgID:         po.OrgID,
			ClinicianID:   uint64(po.ClinicianID),
			Token:         po.Token,
			TargetType:    po.TargetType,
			TargetCode:    po.TargetCode,
			TargetVersion: targetVersion,
			IsActive:      po.IsActive,
			ExpiresAt:     po.ExpiresAt,
		})
	}
	return rows
}

func uniqueUint64(items []uint64) []uint64 {
	if len(items) == 0 {
		return []uint64{}
	}
	seen := make(map[uint64]struct{}, len(items))
	result := make([]uint64, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
