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

func (r *readModel) ListTesteesByIDs(ctx context.Context, orgID int64, ids []uint64) ([]actorreadmodel.TesteeRow, error) {
	if len(ids) == 0 {
		return []actorreadmodel.TesteeRow{}, nil
	}
	testeesByID, err := r.loadTesteeRowsByIDInOrg(ctx, orgID, ids)
	if err != nil {
		return nil, err
	}
	rows := make([]actorreadmodel.TesteeRow, 0, len(ids))
	seen := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if row, ok := testeesByID[id]; ok {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func (r *readModel) applyTesteeFilter(query *gorm.DB, filter actorreadmodel.TesteeFilter) *gorm.DB {
	query = query.Where("org_id = ? AND deleted_at IS NULL", filter.OrgID)
	if filter.RestrictToStoreScope {
		if filter.AllAssignedStores {
			query = query.Where("store_id IS NOT NULL AND store_id > 0")
		} else {
			query = query.Where("store_id IN ?", filter.AllowedStoreIDs)
		}
	}
	if filter.StoreID != nil {
		query = query.Where("store_id = ?", *filter.StoreID)
	}
	if filter.UnassignedStore {
		query = query.Where("store_id IS NULL")
	}
	if filter.RestrictToAccessScope {
		query = query.Where("id IN ?", filter.AccessibleTesteeIDs)
	}
	if filter.ProfileID != nil {
		query = query.Where("profile_id = ?", *filter.ProfileID)
	}
	if filter.Name != "" {
		query = query.Where("name LIKE ?", "%"+filter.Name+"%")
	}
	if filter.KeyFocus != nil {
		query = query.Where("is_key_focus = ?", *filter.KeyFocus)
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
	query := r.WithContext(ctx).Where("deleted_at IS NULL")
	if filter.OrgID > 0 {
		query = query.Where("org_id = ?", filter.OrgID)
	}
	if filter.UserID > 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.ActiveOnly {
		query = query.Where("is_active = ?", true)
	}
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
	rows := []actorreadmodel.ClinicianRow{row}
	if err := r.enrichClinicianStores(ctx, rows); err != nil {
		return nil, err
	}
	return &rows[0], nil
}

func (r *readModel) clinicianQuery(ctx context.Context, f actorreadmodel.ClinicianFilter) *gorm.DB {
	q := r.WithContext(ctx).Model(&ClinicianPO{}).Where("org_id=? AND deleted_at IS NULL", f.OrgID)
	if f.StoreID != nil {
		q = q.Where("store_id=?", *f.StoreID)
	}
	if f.Unconfigured {
		q = q.Where("store_id IS NULL")
	}
	return q
}
func (r *readModel) ListClinicians(ctx context.Context, f actorreadmodel.ClinicianFilter) ([]actorreadmodel.ClinicianRow, error) {
	var pos []*ClinicianPO
	if err := r.clinicianQuery(ctx, f).Order("id ASC").Offset(f.Offset).Limit(f.Limit).Find(&pos).Error; err != nil {
		return nil, err
	}
	rows := clinicianRowsFromPOs(pos)
	if err := r.enrichClinicianStores(ctx, rows); err != nil {
		return nil, err
	}
	return rows, nil
}
func (r *readModel) CountClinicians(ctx context.Context, f actorreadmodel.ClinicianFilter) (int64, error) {
	var n int64
	err := r.clinicianQuery(ctx, f).Count(&n).Error
	return n, err
}

// Batch load store display data; store IDs remain the association authority.
func (r *readModel) enrichClinicianStores(ctx context.Context, rows []actorreadmodel.ClinicianRow) error {
	ids := make([]uint64, 0, len(rows))
	for _, v := range rows {
		if v.StoreID != nil {
			ids = append(ids, *v.StoreID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var stores []struct {
		ID         uint64
		OrgID      int64
		Code, Name string
	}
	if err := r.WithContext(ctx).Table("actor_stores").Where("id IN ?", ids).Find(&stores).Error; err != nil {
		return err
	}
	for i := range rows {
		if rows[i].StoreID == nil {
			continue
		}
		for _, s := range stores {
			if s.ID == *rows[i].StoreID && s.OrgID == rows[i].OrgID {
				rows[i].StoreCode = s.Code
				rows[i].StoreName = s.Name
				break
			}
		}
	}
	return nil
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
	testeesByID, err := r.loadRelationTesteeRows(ctx, filter, testeeIDs)
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

func (r *readModel) ListActiveTesteeRelationsByTesteeIDs(ctx context.Context, orgID int64, testeeIDs []uint64, relationTypes []string) ([]actorreadmodel.TesteeRelationRow, error) {
	if len(testeeIDs) == 0 {
		return []actorreadmodel.TesteeRelationRow{}, nil
	}
	var relationRows []*ClinicianRelationPO
	if err := buildActiveTesteeRelationsQuery(r.WithContext(ctx), orgID, testeeIDs, relationTypes).
		Find(&relationRows).Error; err != nil {
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

func buildActiveTesteeRelationsQuery(db *gorm.DB, orgID int64, testeeIDs []uint64, relationTypes []string) *gorm.DB {
	query := db.
		Where("org_id = ? AND testee_id IN ? AND is_active = ? AND deleted_at IS NULL", orgID, uniqueUint64(testeeIDs), true)
	if len(relationTypes) > 0 {
		query = query.Where("relation_type IN ?", relationTypes)
	}
	return query.
		Order("testee_id ASC").
		Order("CASE relation_type WHEN 'primary' THEN 0 WHEN 'attending' THEN 1 WHEN 'collaborator' THEN 2 WHEN 'assigned' THEN 3 ELSE 4 END ASC").
		Order("bound_at DESC, id DESC")
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
	testeesByID, err := r.loadRelationTesteeRows(ctx, filter, testeeIDs)
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
	if len(filter.TesteeIDs) > 0 {
		query = query.Where("testee_id IN ?", filter.TesteeIDs)
	}
	if filter.ActiveOnly {
		query = query.Where("is_active = ?", true)
	}
	if len(filter.RelationTypes) > 0 {
		query = query.Where("relation_type IN ?", filter.RelationTypes)
	}

	if filter.RestrictToStoreScope {
		currentTestees := r.WithContext(ctx).Table("testee").Select("1").
			Where("testee.id = clinician_relation.testee_id AND testee.org_id = ? AND testee.deleted_at IS NULL", filter.OrgID)
		if filter.AllAssignedStores {
			currentTestees = currentTestees.Where("testee.store_id IS NOT NULL AND testee.store_id > 0")
		} else {
			currentTestees = currentTestees.Where("testee.store_id IN ?", filter.AllowedStoreIDs)
		}
		query = query.Where("EXISTS (?)", currentTestees)
	}

	countQuery := query.Session(&gorm.Session{})
	var total int64
	if filter.ActiveOnly || countActiveOnly {
		if err := countQuery.Model(&ClinicianRelationPO{}).Count(&total).Error; err != nil {
			return nil, 0, err
		}
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = -1 // -1 means no limit in GORM; 0 would generate LIMIT 0 and return nothing
	}
	err := query.Order("bound_at DESC, id DESC").Offset(filter.Offset).Limit(limit).Find(&pos).Error
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

// Recheck ownership during hydration. If a scoped page changed between the
// relation query and this query, reject the page instead of returning partial
// data with a stale total or an out-of-scope Testee.
func (r *readModel) loadRelationTesteeRows(ctx context.Context, filter actorreadmodel.RelationFilter, ids []uint64) (map[uint64]actorreadmodel.TesteeRow, error) {
	result := map[uint64]actorreadmodel.TesteeRow{}
	if len(ids) == 0 {
		return result, nil
	}
	query := r.applyTesteeFilter(r.WithContext(ctx), actorreadmodel.TesteeFilter{OrgID: filter.OrgID, RestrictToStoreScope: filter.RestrictToStoreScope, AllowedStoreIDs: filter.AllowedStoreIDs, AllAssignedStores: filter.AllAssignedStores}).Where("id IN ?", uniqueUint64(ids))
	var pos []*TesteePO
	if err := query.Find(&pos).Error; err != nil {
		return nil, err
	}
	for _, po := range pos {
		row := testeeRowFromPO(po)
		result[row.ID] = row
	}
	if filter.RestrictToStoreScope && len(result) != len(uniqueUint64(ids)) {
		return nil, errors.WithCode(code.ErrConflict, "受试者归属已变化，请刷新列表")
	}
	return result, nil
}

func (r *readModel) loadTesteeRowsByIDInOrg(ctx context.Context, orgID int64, ids []uint64) (map[uint64]actorreadmodel.TesteeRow, error) {
	if len(ids) == 0 {
		return map[uint64]actorreadmodel.TesteeRow{}, nil
	}
	query := r.WithContext(ctx).Where("id IN ? AND deleted_at IS NULL", uniqueUint64(ids))
	if orgID > 0 {
		query = query.Where("org_id = ?", orgID)
	}
	var pos []*TesteePO
	if err := query.Find(&pos).Error; err != nil {
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
		StoreID: po.StoreID, StoreVersion: po.StoreVersion,
		ID:         uint64(po.ID),
		OrgID:      po.OrgID,
		ProfileID:  po.ProfileID,
		Name:       po.Name,
		Gender:     po.Gender,
		Birthday:   po.Birthday,
		CreatedAt:  po.CreatedAt,
		UpdatedAt:  po.UpdatedAt,
		Source:     po.Source,
		IsKeyFocus: po.IsKeyFocus,
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
		Version:                po.Version,
		ID:                     uint64(po.ID),
		OrgID:                  po.OrgID,
		UserID:                 po.UserID,
		Roles:                  append([]string(nil), po.Roles...),
		EffectiveRoles:         append([]string(nil), po.EffectiveRoles...),
		AuthzPolicyVersion:     po.AuthzPolicyVersion,
		AuthzProjectionPending: po.AuthzProjectionPending,
		Name:                   po.Name,
		Email:                  po.Email,
		Phone:                  po.Phone,
		IsActive:               po.IsActive,
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
		StoreID: po.StoreID, Version: po.Version,
		ID:            uint64(po.ID),
		OrgID:         po.OrgID,
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
			InvalidatedAt: po.InvalidatedAt, InvalidationReason: po.InvalidationReason,
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

func (r *readModel) ListTesteeIDsInStores(ctx context.Context, orgID int64, stores []uint64, allAssigned bool) ([]uint64, error) {
	if orgID <= 0 {
		return nil, errors.WithCode(code.ErrPermissionDenied, "company scope required")
	}
	ids := []uint64{}
	if !allAssigned && len(stores) == 0 {
		return ids, nil
	}
	filter := actorreadmodel.TesteeFilter{OrgID: orgID, RestrictToStoreScope: true, AllowedStoreIDs: stores, AllAssignedStores: allAssigned}
	err := r.applyTesteeFilter(r.WithContext(ctx).Model(&TesteePO{}), filter).Order("id ASC").Pluck("id", &ids).Error
	return ids, err
}
