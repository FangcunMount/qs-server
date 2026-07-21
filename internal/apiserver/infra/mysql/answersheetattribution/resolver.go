package answersheetattribution

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	attributionport "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetattribution"
	"gorm.io/gorm"
)

type Resolver struct {
	db  *gorm.DB
	now func() time.Time
}

func NewResolver(db *gorm.DB) *Resolver { return &Resolver{db: db, now: time.Now} }

func (r *Resolver) Resolve(ctx context.Context, request attributionport.ResolveRequest) (domainanswersheet.AttributionSnapshot, error) {
	if r == nil || r.db == nil {
		return domainanswersheet.AttributionSnapshot{}, fmt.Errorf("attribution database is unavailable")
	}
	if err := request.OriginRef.Validate(); err != nil {
		return domainanswersheet.AttributionSnapshot{}, err
	}
	switch request.OriginRef.Type {
	case domainanswersheet.OriginTypeSelfService:
		return domainanswersheet.NewAttributionSnapshot(request.OriginRef, "", "", "", "", "", r.now())
	case domainanswersheet.OriginTypeAssessmentEntry:
		return r.resolveEntry(ctx, request)
	case domainanswersheet.OriginTypePlanTask:
		return r.resolvePlanTask(ctx, request)
	case domainanswersheet.OriginTypeClinicianDirect:
		return r.resolveClinician(ctx, request)
	default:
		return domainanswersheet.AttributionSnapshot{}, fmt.Errorf("unsupported origin type %q", request.OriginRef.Type)
	}
}

type entryRow struct {
	ID            uint64
	OrgID         int64
	ClinicianID   uint64
	TargetType    string
	TargetCode    string
	TargetVersion string
	IsActive      bool
	ExpiresAt     *time.Time
	ClinicianOn   bool `gorm:"column:clinician_on"`
}

func (r *Resolver) resolveEntry(ctx context.Context, request attributionport.ResolveRequest) (domainanswersheet.AttributionSnapshot, error) {
	id, err := parseOriginID(request.OriginRef.ID)
	if err != nil {
		return domainanswersheet.AttributionSnapshot{}, err
	}
	var row entryRow
	err = r.db.WithContext(ctx).Raw(`
		SELECT e.id, e.org_id, e.clinician_id, e.target_type, e.target_code, e.target_version,
		       e.is_active, e.expires_at, c.is_active AS clinician_on
		FROM assessment_entry e
		JOIN clinician c ON c.id = e.clinician_id AND c.deleted_at IS NULL
		WHERE e.id = ? AND e.org_id = ? AND e.deleted_at IS NULL`, id, request.OrgID).Scan(&row).Error
	if err != nil {
		return domainanswersheet.AttributionSnapshot{}, err
	}
	if row.ID == 0 || !row.IsActive || !row.ClinicianOn || (row.ExpiresAt != nil && r.now().After(*row.ExpiresAt)) {
		return domainanswersheet.AttributionSnapshot{}, fmt.Errorf("assessment entry is unavailable")
	}
	if err := validateEntryContent(row, request); err != nil {
		return domainanswersheet.AttributionSnapshot{}, err
	}
	return domainanswersheet.NewAttributionSnapshot(
		request.OriginRef, strconv.FormatUint(row.ClinicianID, 10), strconv.FormatUint(row.ID, 10), "", "", "", r.now(),
	)
}

func validateEntryContent(row entryRow, request attributionport.ResolveRequest) error {
	wantCode, wantVersion := request.QuestionnaireCode, request.QuestionnaireVersion
	if row.TargetType == "scale" {
		if !request.Admission.RequiresAssessment() {
			return fmt.Errorf("scale entry cannot submit an independent questionnaire")
		}
		wantCode, wantVersion = request.Admission.ModelCode(), request.Admission.ModelVersion()
	}
	if row.TargetType != "scale" && row.TargetType != "questionnaire" {
		return fmt.Errorf("unsupported assessment entry target type %q", row.TargetType)
	}
	if strings.TrimSpace(row.TargetCode) != strings.TrimSpace(wantCode) {
		return fmt.Errorf("assessment entry target does not match submitted content")
	}
	if row.TargetVersion != "" && row.TargetVersion != wantVersion {
		return fmt.Errorf("assessment entry version does not match submitted content")
	}
	return nil
}

type planTaskRow struct {
	ID               uint64
	OrgID            int64
	TesteeID         uint64
	PlanID           uint64
	EnrollmentID     uint64
	ScaleCode        string
	TaskStatus       string
	EnrollmentStatus string
}

func (r *Resolver) resolvePlanTask(ctx context.Context, request attributionport.ResolveRequest) (domainanswersheet.AttributionSnapshot, error) {
	id, err := parseOriginID(request.OriginRef.ID)
	if err != nil {
		return domainanswersheet.AttributionSnapshot{}, err
	}
	var row planTaskRow
	err = r.db.WithContext(ctx).Raw(`
		SELECT t.id, t.org_id, t.testee_id, t.plan_id, t.enrollment_id, t.scale_code,
		       t.status AS task_status, e.status AS enrollment_status
		FROM assessment_task t
		JOIN plan_enrollment e ON e.id = t.enrollment_id AND e.deleted_at IS NULL
		WHERE t.id = ? AND t.org_id = ? AND t.testee_id = ? AND t.deleted_at IS NULL`, id, request.OrgID, request.TesteeID).Scan(&row).Error
	if err != nil {
		return domainanswersheet.AttributionSnapshot{}, err
	}
	if row.ID == 0 || row.TaskStatus != "opened" || row.EnrollmentStatus != "active" {
		return domainanswersheet.AttributionSnapshot{}, fmt.Errorf("plan task is unavailable")
	}
	if !request.Admission.RequiresAssessment() || row.ScaleCode != request.Admission.ModelCode() {
		return domainanswersheet.AttributionSnapshot{}, fmt.Errorf("plan task content does not match submitted assessment")
	}
	clinicianID, err := r.resolvePrimaryClinician(ctx, request.OrgID, request.TesteeID)
	if err != nil {
		return domainanswersheet.AttributionSnapshot{}, err
	}
	return domainanswersheet.NewAttributionSnapshot(
		request.OriginRef, clinicianID, "", strconv.FormatUint(row.PlanID, 10),
		strconv.FormatUint(row.EnrollmentID, 10), strconv.FormatUint(row.ID, 10), r.now(),
	)
}

func (r *Resolver) resolveClinician(ctx context.Context, request attributionport.ResolveRequest) (domainanswersheet.AttributionSnapshot, error) {
	id, err := parseOriginID(request.OriginRef.ID)
	if err != nil {
		return domainanswersheet.AttributionSnapshot{}, err
	}
	var count int64
	err = r.db.WithContext(ctx).Table("clinician").
		Where("id = ? AND org_id = ? AND is_active = 1 AND deleted_at IS NULL", id, request.OrgID).Count(&count).Error
	if err != nil {
		return domainanswersheet.AttributionSnapshot{}, err
	}
	if count != 1 {
		return domainanswersheet.AttributionSnapshot{}, fmt.Errorf("clinician is unavailable")
	}
	return domainanswersheet.NewAttributionSnapshot(request.OriginRef, strconv.FormatUint(id, 10), "", "", "", "", r.now())
}

func (r *Resolver) resolvePrimaryClinician(ctx context.Context, orgID, testeeID uint64) (string, error) {
	var row struct{ ClinicianID uint64 }
	err := r.db.WithContext(ctx).Raw(`
		SELECT cr.clinician_id
		FROM clinician_relation cr
		JOIN clinician c ON c.id = cr.clinician_id AND c.is_active = 1 AND c.deleted_at IS NULL
		WHERE cr.org_id = ? AND cr.testee_id = ? AND cr.is_active = 1 AND cr.deleted_at IS NULL
		ORDER BY CASE cr.relation_type WHEN 'primary' THEN 0 WHEN 'attending' THEN 1 ELSE 2 END, cr.bound_at DESC, cr.id DESC
		LIMIT 1`, orgID, testeeID).Scan(&row).Error
	if err != nil {
		return "", err
	}
	if row.ClinicianID == 0 {
		return "", nil
	}
	return strconv.FormatUint(row.ClinicianID, 10), nil
}

func parseOriginID(raw string) (uint64, error) {
	id, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("invalid origin id")
	}
	return id, nil
}
