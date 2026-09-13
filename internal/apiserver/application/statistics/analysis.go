package statistics

import (
	"context"
	"fmt"
	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"time"
)

// Analysis DTOs are an explicit operational whitelist, independent of legacy detail DTOs.
type AnalysisStore struct {
	ID       uint64 `json:"id,string" swaggertype:"string"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	IsActive bool   `json:"is_active"`
}
type AnalysisMetadata struct {
	Scope                  string          `json:"scope"`
	Stores                 []AnalysisStore `json:"stores"`
	From                   string          `json:"from"`
	ToExclusive            string          `json:"to_exclusive"`
	DataThrough            string          `json:"data_through"`
	PublishedAt            time.Time       `json:"published_at"`
	PublishedVersion       uint64          `json:"published_version,string" swaggertype:"string"`
	CurrentOwnershipReadAt time.Time       `json:"current_ownership_read_at"`
}
type AnalysisOverview struct {
	AnalysisMetadata
	OrganizationOverview domain.OrganizationOverview        `json:"organization_overview"`
	AccessFunnel         domain.AccessFunnelStatistics      `json:"access_funnel"`
	AssessmentService    domain.AssessmentServiceStatistics `json:"assessment_service"`
	Plan                 domain.PlanDomainStatistics        `json:"plan"`
}
type ClinicianSummary struct {
	ClinicianCount       int64 `json:"clinician_count"`
	ActiveClinicianCount int64 `json:"active_clinician_count"`
	CliniciansWithIntake int64 `json:"clinicians_with_intake"`
	IntakeConfirmedCount int64 `json:"intake_confirmed_count"`
	ReportGeneratedCount int64 `json:"report_generated_count"`
}
type AnalysisClinicianPage struct {
	AnalysisMetadata
	Items    []AnalysisClinician `json:"items"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
	Summary  ClinicianSummary    `json:"summary"`
}
type AnalysisEntryPage struct {
	AnalysisMetadata
	Items    []AnalysisEntry `json:"items"`
	Total    int64           `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}
type AnalysisClinician struct {
	ID                               uint64 `json:"id,string" swaggertype:"string"`
	Name                             string `json:"name"`
	Department                       string `json:"department,omitempty"`
	Title                            string `json:"title,omitempty"`
	ClinicianType                    string `json:"clinician_type"`
	IsActive                         bool   `json:"is_active"`
	EntryOpenedCount                 int64  `json:"entry_opened_count"`
	IntakeConfirmedCount             int64  `json:"intake_confirmed_count"`
	CareRelationshipEstablishedCount int64  `json:"care_relationship_established_count"`
	AssessmentCreatedCount           int64  `json:"assessment_created_count"`
	OutcomeCommittedCount            int64  `json:"outcome_committed_count"`
	ReportGeneratedCount             int64  `json:"report_generated_count"`
	PrimaryTesteeCount               int64  `json:"primary_testee_count"`
	AttendingTesteeCount             int64  `json:"attending_testee_count"`
	CollaboratorTesteeCount          int64  `json:"collaborator_testee_count"`
	TotalAccessibleTestees           int64  `json:"total_accessible_testees"`
	ActiveEntryCount                 int64  `json:"active_entry_count"`
}

func analysisClinician(v ClinicianItem) AnalysisClinician {
	//nolint:staticcheck // Explicit whitelist must remain independent of future legacy DTO fields.
	return AnalysisClinician{ID: v.ID, Name: v.Name, Department: v.Department, Title: v.Title, ClinicianType: v.ClinicianType, IsActive: v.IsActive, EntryOpenedCount: v.EntryOpenedCount, IntakeConfirmedCount: v.IntakeConfirmedCount, CareRelationshipEstablishedCount: v.CareRelationshipEstablishedCount, AssessmentCreatedCount: v.AssessmentCreatedCount, OutcomeCommittedCount: v.OutcomeCommittedCount, ReportGeneratedCount: v.ReportGeneratedCount, PrimaryTesteeCount: v.PrimaryTesteeCount, AttendingTesteeCount: v.AttendingTesteeCount, CollaboratorTesteeCount: v.CollaboratorTesteeCount, TotalAccessibleTestees: v.TotalAccessibleTestees, ActiveEntryCount: v.ActiveEntryCount}
}

type AnalysisEntry struct {
	ID                     uint64     `json:"id,string" swaggertype:"string"`
	ClinicianID            uint64     `json:"clinician_id,string" swaggertype:"string"`
	ClinicianName          string     `json:"clinician_name,omitempty"`
	TargetType             string     `json:"target_type"`
	TargetCode             string     `json:"target_code"`
	TargetVersion          string     `json:"target_version,omitempty"`
	IsActive               bool       `json:"is_active"`
	ExpiresAt              *time.Time `json:"expires_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	EntryOpenedCount       int64      `json:"entry_opened_count"`
	IntakeConfirmedCount   int64      `json:"intake_confirmed_count"`
	AssessmentCreatedCount int64      `json:"assessment_created_count"`
	OutcomeCommittedCount  int64      `json:"outcome_committed_count"`
	ReportGeneratedCount   int64      `json:"report_generated_count"`
}

func analysisEntry(v EntryItem) AnalysisEntry {
	return AnalysisEntry{ID: v.ID, ClinicianID: v.ClinicianID, ClinicianName: v.ClinicianName, TargetType: v.TargetType, TargetCode: v.TargetCode, TargetVersion: v.TargetVersion, IsActive: v.IsActive, ExpiresAt: v.ExpiresAt, CreatedAt: v.CreatedAt, EntryOpenedCount: v.EntryOpenedCount, IntakeConfirmedCount: v.IntakeConfirmedCount, AssessmentCreatedCount: v.AssessmentCreatedCount, OutcomeCommittedCount: v.OutcomeCommittedCount, ReportGeneratedCount: v.ReportGeneratedCount}
}

type ScopedAnalysisClinicianVisibility interface {
	ScopedAnalysisClinicianVisible(context.Context, int64, authz.StoreRange, uint64) (bool, error)
}

type ScopedClinicianAnalysisStore interface {
	ScopedClinicianAnalysis(context.Context, int64, authz.StoreRange, time.Time, time.Time, int, int) ([]ClinicianItem, int64, ClinicianSummary, error)
}

func (s *ReadService) prepareAnalysis(ctx context.Context, org int64, filter OperationsFilter) (*operationQuery, error) {
	if org <= 0 || actorctx.OperatorOrgID(ctx) != org {
		return nil, cberrors.WithCode(code.ErrPermissionDenied, "active company operator required")
	}
	q, err := s.prepareOperations(ctx, org, filter)
	if err != nil {
		return nil, err
	}
	// An all-stores grant is not authority to query another company's store ID.
	known := map[uint64]bool{}
	for _, v := range q.population {
		known[v.ID] = true
	}
	for _, id := range filter.StoreIDs {
		if !known[id] {
			return nil, cberrors.WithCode(code.ErrPermissionDenied, "requested store is not visible")
		}
	}
	return q, nil
}
func (s *ReadService) analysisMetadata(q *operationQuery) AnalysisMetadata {
	m := AnalysisMetadata{Scope: "stores", Stores: []AnalysisStore{}, From: q.requested.From.Format("2006-01-02"), ToExclusive: q.requested.To.Format("2006-01-02"), DataThrough: q.window.To.AddDate(0, 0, -1).Format("2006-01-02"), PublishedAt: q.published.SnapshotAt, PublishedVersion: q.published.VisibleRunID, CurrentOwnershipReadAt: s.now()}
	if q.stores.AllStores {
		m.Scope = "all_stores"
	}
	for _, v := range q.population {
		m.Stores = append(m.Stores, AnalysisStore{v.ID, v.Code, v.Name, v.IsActive})
	}
	return m
}
func (s *ReadService) checkAnalysisPublication(ctx context.Context, org int64, q *operationQuery) error {
	return s.validatePublishedResults(ctx, org, databaseReadPermit{visibleRunID: q.published.VisibleRunID, readable: true})
}
func (s *ReadService) AnalysisOverview(ctx context.Context, org int64, filter OperationsFilter) (*AnalysisOverview, error) {
	q, err := s.prepareAnalysis(ctx, org, filter)
	if err != nil {
		return nil, err
	}
	reader, ok := s.store.(ScopedOverviewStore)
	if !ok {
		return nil, fmt.Errorf("scoped overview reader unavailable")
	}
	data, err := analysisCached(ctx, s, org, q, "overview", nil, func() (ScopedOverviewData, error) {
		return reader.ScopedOverview(ctx, org, q.stores, q.window.From, q.window.To, domain.BusinessDate(q.published.AsOfDate).AddDate(0, 0, 1))
	})
	if err != nil {
		return nil, err
	}
	if err = s.checkAnalysisPublication(ctx, org, q); err != nil {
		return nil, err
	}
	v := buildOverview(org, DateRange{From: q.window.From, To: q.window.To.AddDate(0, 0, -1)}, Freshness{}, data.Metrics, data.Trends)
	return &AnalysisOverview{s.analysisMetadata(q), v.OrganizationOverview, v.AccessFunnel, v.AssessmentService, v.Plan}, nil
}
func (s *ReadService) AnalysisClinicians(ctx context.Context, org int64, filter OperationsFilter, page, size int) (*AnalysisClinicianPage, error) {
	q, err := s.prepareAnalysis(ctx, org, filter)
	if err != nil {
		return nil, err
	}
	page, size = normalizePage(page, size)
	reader, ok := s.store.(ScopedClinicianAnalysisStore)
	if !ok {
		return nil, fmt.Errorf("scoped clinician analysis reader unavailable")
	}
	cached, err := analysisCached(ctx, s, org, q, "clinicians", []int{page, size}, func() (AnalysisClinicianPage, error) {
		rows, total, summary, err := reader.ScopedClinicianAnalysis(ctx, org, q.stores, q.window.From, q.window.To, page, size)
		if err != nil {
			return AnalysisClinicianPage{}, err
		}
		items := make([]AnalysisClinician, 0, len(rows))
		for _, v := range rows {
			items = append(items, analysisClinician(v))
		}
		return AnalysisClinicianPage{Items: items, Total: total, Page: page, PageSize: size, Summary: summary}, nil
	})
	if err != nil {
		return nil, err
	}
	if err = s.checkAnalysisPublication(ctx, org, q); err != nil {
		return nil, err
	}
	cached.AnalysisMetadata = s.analysisMetadata(q)
	return &cached, nil
}
func (s *ReadService) AnalysisEntries(ctx context.Context, org int64, filter OperationsFilter, clinicianID *uint64, active *bool, page, size int) (*AnalysisEntryPage, error) {
	q, err := s.prepareAnalysis(ctx, org, filter)
	if err != nil {
		return nil, err
	}
	if clinicianID != nil {
		visibility, ok := s.store.(ScopedAnalysisClinicianVisibility)
		if !ok {
			return nil, fmt.Errorf("scoped clinician visibility unavailable")
		}
		visible, e := visibility.ScopedAnalysisClinicianVisible(ctx, org, q.stores, *clinicianID)
		if e != nil {
			return nil, e
		}
		if !visible {
			return nil, cberrors.WithCode(code.ErrPermissionDenied, "requested clinician is not visible")
		}
	}
	page, size = normalizePage(page, size)
	reader, ok := s.store.(ScopedEntryStore)
	if !ok {
		return nil, fmt.Errorf("scoped entry reader unavailable")
	}
	cached, err := analysisCached(ctx, s, org, q, "entries", []any{clinicianID, active, page, size}, func() (AnalysisEntryPage, error) {
		rows, total, err := reader.ScopedEntries(ctx, org, q.stores, nil, clinicianID, active, q.window.From, q.window.To, page, size)
		if err != nil {
			return AnalysisEntryPage{}, err
		}
		items := make([]AnalysisEntry, 0, len(rows))
		for _, v := range rows {
			items = append(items, analysisEntry(v))
		}
		return AnalysisEntryPage{Items: items, Total: total, Page: page, PageSize: size}, nil
	})
	if err != nil {
		return nil, err
	}
	if err = s.checkAnalysisPublication(ctx, org, q); err != nil {
		return nil, err
	}
	cached.AnalysisMetadata = s.analysisMetadata(q)
	return &cached, nil
}
