package statisticsv2

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainstats "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	domainv2 "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics/v2"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type QueryFilter struct {
	Preset, From, To string
}

type DateRange struct {
	Preset string    `json:"preset"`
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
}

type Freshness struct {
	AsOfDate   string    `json:"as_of_date"`
	SnapshotAt time.Time `json:"snapshot_at"`
	IsStale    bool      `json:"is_stale"`
}

type OverviewMetrics struct {
	TesteeCount                      int64 `json:"testee_count"`
	ClinicianCount                   int64 `json:"clinician_count"`
	ActiveClinicianCount             int64 `json:"active_clinician_count"`
	EntryCount                       int64 `json:"entry_count"`
	ActiveEntryCount                 int64 `json:"active_entry_count"`
	ActiveEnrollmentCount            int64 `json:"active_enrollment_count"`
	AnswerSheetSubmissionCount       int64 `json:"answersheet_submission_count"`
	AssessmentCount                  int64 `json:"assessment_count"`
	ReportCount                      int64 `json:"report_count"`
	ContentCount                     int64 `json:"content_count"`
	EntryOpenedCount                 int64 `json:"entry_opened_count"`
	IntakeConfirmedCount             int64 `json:"intake_confirmed_count"`
	TesteeCreatedCount               int64 `json:"testee_created_count"`
	CareRelationshipEstablishedCount int64 `json:"care_relationship_established_count"`
	CareRelationshipTransferredCount int64 `json:"care_relationship_transferred_count"`
	WindowAnswerSheetSubmittedCount  int64 `json:"window_answersheet_submitted_count"`
	WindowAssessmentCreatedCount     int64 `json:"window_assessment_created_count"`
	WindowOutcomeCommittedCount      int64 `json:"window_outcome_committed_count"`
	WindowAssessmentFailedCount      int64 `json:"window_assessment_failed_count"`
	WindowReportGeneratedCount       int64 `json:"window_report_generated_count"`
	WindowReportFailedCount          int64 `json:"window_report_failed_count"`
	TaskCreatedCount                 int64 `json:"task_created_count"`
	TaskOpenedCount                  int64 `json:"task_opened_count"`
	TaskCompletedCount               int64 `json:"task_completed_count"`
	TaskExpiredCount                 int64 `json:"task_expired_count"`
	TaskCanceledCount                int64 `json:"task_canceled_count"`
	PlannedTaskCount                 int64 `json:"planned_task_count"`
	DueTaskCount                     int64 `json:"due_task_count"`
	CompletedOnTimeCount             int64 `json:"completed_on_time_count"`
	CompletedOverdueCount            int64 `json:"completed_overdue_count"`
	UncompletedOverdueCount          int64 `json:"uncompleted_overdue_count"`
}

type Overview struct {
	OrgID                int64                                   `json:"org_id"`
	TimeRange            DateRange                               `json:"time_range"`
	Freshness            Freshness                               `json:"freshness"`
	Metrics              OverviewMetrics                         `json:"metrics"`
	OrganizationOverview domainstats.OrganizationOverview        `json:"organization_overview"`
	AccessFunnel         domainstats.AccessFunnelStatistics      `json:"access_funnel"`
	AssessmentService    domainstats.AssessmentServiceStatistics `json:"assessment_service"`
	DimensionAnalysis    domainstats.DimensionAnalysisSummary    `json:"dimension_analysis"`
	Plan                 domainstats.PlanDomainStatistics        `json:"plan"`
}

type OverviewTrends struct {
	Access          domainstats.AccessFunnelTrend
	Assessment      domainstats.AssessmentServiceTrend
	PlanActivity    domainstats.PlanTaskActivityTrend
	PlanFulfillment domainstats.PlanTaskFulfillmentTrend
	EnrolledTestees int64
}

type ClinicianItem struct {
	ID                               uint64  `json:"id"`
	OperatorID                       *uint64 `json:"operator_id,omitempty"`
	Name                             string  `json:"name"`
	Department                       string  `json:"department,omitempty"`
	Title                            string  `json:"title,omitempty"`
	ClinicianType                    string  `json:"clinician_type"`
	IsActive                         bool    `json:"is_active"`
	EntryOpenedCount                 int64   `json:"entry_opened_count"`
	IntakeConfirmedCount             int64   `json:"intake_confirmed_count"`
	CareRelationshipEstablishedCount int64   `json:"care_relationship_established_count"`
	AssessmentCreatedCount           int64   `json:"assessment_created_count"`
	OutcomeCommittedCount            int64   `json:"outcome_committed_count"`
	ReportGeneratedCount             int64   `json:"report_generated_count"`
	PrimaryTesteeCount               int64   `json:"primary_testee_count"`
	AttendingTesteeCount             int64   `json:"attending_testee_count"`
	CollaboratorTesteeCount          int64   `json:"collaborator_testee_count"`
	TotalAccessibleTestees           int64   `json:"total_accessible_testees"`
	ActiveEntryCount                 int64   `json:"active_entry_count"`
}

type EntryItem struct {
	ID                     uint64     `json:"id"`
	ClinicianID            uint64     `json:"clinician_id"`
	ClinicianName          string     `json:"clinician_name,omitempty"`
	Token                  string     `json:"token"`
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

type Page[T any] struct {
	Items      []T       `json:"items"`
	Total      int64     `json:"total"`
	Page       int       `json:"page"`
	PageSize   int       `json:"page_size"`
	TotalPages int       `json:"total_pages"`
	TimeRange  DateRange `json:"time_range"`
	Freshness  Freshness `json:"freshness"`
}

type TesteeSummary struct {
	TotalAccessibleTestees  int64     `json:"total_accessible_testees"`
	PrimaryTesteeCount      int64     `json:"primary_testee_count"`
	AttendingTesteeCount    int64     `json:"attending_testee_count"`
	CollaboratorTesteeCount int64     `json:"collaborator_testee_count"`
	KeyFocusTesteeCount     int64     `json:"key_focus_testee_count"`
	AssessedInWindowCount   int64     `json:"assessed_in_window_count"`
	TimeRange               DateRange `json:"time_range"`
	Freshness               Freshness `json:"freshness"`
}

type ContentRef struct {
	Kind string `json:"kind"`
	Code string `json:"code"`
}

type ContentItem struct {
	Kind             string  `json:"kind"`
	Code             string  `json:"code"`
	TotalSubmissions int64   `json:"total_submissions"`
	HasCompletion    bool    `json:"has_completion"`
	TotalCompletions int64   `json:"total_completions,omitempty"`
	CompletionRate   float64 `json:"completion_rate,omitempty"`
}

type ContentBatch struct {
	Items     []ContentItem `json:"items"`
	Freshness Freshness     `json:"freshness"`
}

type Snapshot struct {
	AsOfDate, SnapshotAt time.Time
}

type ReadStore interface {
	LatestSuccessfulSnapshot(context.Context, int64) (*Snapshot, error)
	SnapshotForDate(context.Context, int64, time.Time) (*Snapshot, error)
	Overview(context.Context, int64, time.Time, time.Time) (OverviewMetrics, error)
	OverviewTrends(context.Context, int64, time.Time, time.Time) (OverviewTrends, error)
	ListClinicians(context.Context, int64, *uint64, *int64, time.Time, time.Time, int, int) ([]ClinicianItem, int64, error)
	ListEntries(context.Context, int64, *uint64, *uint64, *bool, time.Time, time.Time, int, int) ([]EntryItem, int64, error)
	CurrentClinicianID(context.Context, int64, int64) (uint64, error)
	CurrentClinicianTesteeSummary(context.Context, int64, uint64, time.Time, time.Time) (TesteeSummary, error)
	ContentBatch(context.Context, int64, []ContentRef) ([]ContentItem, error)
}

type ReadService struct {
	store ReadStore
	cache ReadCache
	now   func() time.Time
}

type ReadCache interface {
	Get(context.Context, int64, string, any) (hit bool, stale bool)
	Set(context.Context, int64, string, any)
}

func NewReadService(store ReadStore, caches ...ReadCache) *ReadService {
	service := &ReadService{store: store, now: time.Now}
	if len(caches) > 0 {
		service.cache = caches[0]
	}
	return service
}

func (s *ReadService) cacheGet(ctx context.Context, orgID int64, key string, out any) (bool, bool) {
	if s.cache == nil {
		return false, false
	}
	return s.cache.Get(ctx, orgID, key, out)
}

func (s *ReadService) cacheSet(ctx context.Context, orgID int64, key string, value any) {
	if s.cache != nil {
		s.cache.Set(ctx, orgID, key, value)
	}
}

func cacheKey(kind string, values ...any) string {
	payload, _ := json.Marshal(values)
	return kind + ":" + string(payload)
}

func (s *ReadService) resolve(ctx context.Context, orgID int64, filter QueryFilter) (DateRange, Freshness, error) {
	if s == nil || s.store == nil {
		return DateRange{}, Freshness{}, fmt.Errorf("statistics v2 read service is unavailable")
	}
	snapshot, err := s.store.LatestSuccessfulSnapshot(ctx, orgID)
	if err != nil {
		return DateRange{}, Freshness{}, err
	}
	if snapshot == nil {
		return DateRange{}, Freshness{}, errors.WithCode(code.ErrStatisticsNotReady, "statistics_not_ready")
	}
	asOf := domainv2.BusinessDate(snapshot.AsOfDate)
	preset := strings.TrimSpace(filter.Preset)
	if preset == "" {
		preset = "7d"
	}
	rangeValue := DateRange{Preset: preset}
	switch preset {
	case "latest_complete_day":
		rangeValue.From, rangeValue.To = asOf, asOf
	case "7d":
		rangeValue.From, rangeValue.To = asOf.AddDate(0, 0, -6), asOf
	case "30d":
		rangeValue.From, rangeValue.To = asOf.AddDate(0, 0, -29), asOf
	case "custom":
		var parseErr error
		rangeValue.From, parseErr = time.ParseInLocation("2006-01-02", strings.TrimSpace(filter.From), domainv2.Shanghai)
		if parseErr != nil {
			return DateRange{}, Freshness{}, errors.WithCode(code.ErrInvalidArgument, "invalid from date")
		}
		rangeValue.To, parseErr = time.ParseInLocation("2006-01-02", strings.TrimSpace(filter.To), domainv2.Shanghai)
		if parseErr != nil {
			return DateRange{}, Freshness{}, errors.WithCode(code.ErrInvalidArgument, "invalid to date")
		}
	default:
		return DateRange{}, Freshness{}, errors.WithCode(code.ErrInvalidArgument, "unsupported statistics preset: %s", preset)
	}
	if rangeValue.To.After(asOf) || rangeValue.From.After(rangeValue.To) || rangeValue.To.Sub(rangeValue.From) > 365*24*time.Hour {
		return DateRange{}, Freshness{}, errors.WithCode(code.ErrInvalidArgument, "statistics date range is invalid or exceeds 366 days")
	}
	previousDay := domainv2.BusinessDate(s.now()).AddDate(0, 0, -1)
	observeFreshness(orgID, asOf, previousDay)
	freshness := Freshness{AsOfDate: asOf.Format("2006-01-02"), SnapshotAt: snapshot.SnapshotAt, IsStale: asOf.Before(previousDay)}
	return rangeValue, freshness, nil
}

func queryBounds(value DateRange) (time.Time, time.Time) {
	return value.From, value.To.AddDate(0, 0, 1)
}

func (s *ReadService) Overview(ctx context.Context, orgID int64, filter QueryFilter) (*Overview, error) {
	r, freshness, err := s.resolve(ctx, orgID, filter)
	if err != nil {
		return nil, err
	}
	return s.overviewResolved(ctx, orgID, r, freshness)
}

func (s *ReadService) overviewResolved(ctx context.Context, orgID int64, r DateRange, freshness Freshness) (*Overview, error) {
	from, to := queryBounds(r)
	key := cacheKey("overview", r.Preset, r.From, r.To)
	cached := &Overview{}
	if hit, stale := s.cacheGet(ctx, orgID, key, cached); hit {
		if stale {
			cached.Freshness.IsStale = true
		}
		return cached, nil
	}
	metrics, err := s.store.Overview(ctx, orgID, from, to)
	if err != nil {
		return nil, err
	}
	trends, err := s.store.OverviewTrends(ctx, orgID, from, to)
	if err != nil {
		return nil, err
	}
	completedTasks := metrics.CompletedOnTimeCount + metrics.CompletedOverdueCount
	overdueTasks := metrics.CompletedOverdueCount + metrics.UncompletedOverdueCount
	completionRate, onTimeRate := float64(0), float64(0)
	if metrics.DueTaskCount > 0 {
		completionRate = float64(completedTasks) * 100 / float64(metrics.DueTaskCount)
		onTimeRate = float64(metrics.CompletedOnTimeCount) * 100 / float64(metrics.DueTaskCount)
	}
	value := &Overview{
		OrgID: orgID, TimeRange: r, Freshness: freshness, Metrics: metrics,
		OrganizationOverview: domainstats.OrganizationOverview{
			TesteeCount: metrics.TesteeCount, ClinicianCount: metrics.ClinicianCount, ActiveEntryCount: metrics.ActiveEntryCount,
			AssessmentCount: metrics.AssessmentCount, ReportCount: metrics.ReportCount, ContentCount: metrics.ContentCount,
			AnswerSheetSubmissionCount: metrics.AnswerSheetSubmissionCount,
		},
		AccessFunnel: domainstats.AccessFunnelStatistics{
			Window: domainstats.AccessFunnelWindow{EntryOpenedCount: metrics.EntryOpenedCount, IntakeConfirmedCount: metrics.IntakeConfirmedCount, TesteeCreatedCount: metrics.TesteeCreatedCount, CareRelationshipEstablishedCount: metrics.CareRelationshipEstablishedCount},
			Trend:  trends.Access,
		},
		AssessmentService: domainstats.AssessmentServiceStatistics{
			Window: domainstats.AssessmentServiceWindow{AnswerSheetSubmittedCount: metrics.WindowAnswerSheetSubmittedCount, AssessmentCreatedCount: metrics.WindowAssessmentCreatedCount, ReportGeneratedCount: metrics.WindowReportGeneratedCount, AssessmentFailedCount: metrics.WindowAssessmentFailedCount},
			Trend:  trends.Assessment,
		},
		DimensionAnalysis: domainstats.DimensionAnalysisSummary{ClinicianCount: metrics.ClinicianCount, EntryCount: metrics.EntryCount, ContentCount: metrics.ContentCount},
		Plan: domainstats.PlanDomainStatistics{
			Activity:    domainstats.PlanTaskActivityStatistics{Window: domainstats.PlanTaskActivityWindow{TaskCreatedCount: metrics.TaskCreatedCount, TaskOpenedCount: metrics.TaskOpenedCount, TaskCompletedCount: metrics.TaskCompletedCount, TaskExpiredCount: metrics.TaskExpiredCount, EnrolledTestees: trends.EnrolledTestees, ActiveTestees: metrics.ActiveEnrollmentCount}, Trend: trends.PlanActivity},
			Fulfillment: domainstats.PlanTaskFulfillmentStatistics{Window: domainstats.PlanTaskFulfillmentWindow{PlannedTaskCount: metrics.PlannedTaskCount, DueTaskCount: metrics.DueTaskCount, CompletedTaskCount: completedTasks, OnTimeCompletedCount: metrics.CompletedOnTimeCount, OverdueTaskCount: overdueTasks, CompletionRate: completionRate, OnTimeCompletionRate: onTimeRate}, Trend: trends.PlanFulfillment},
		},
	}
	s.cacheSet(ctx, orgID, key, value)
	return value, nil
}

// Warm builds the common complete-day windows after the generation switch.
// It intentionally resolves the just-committed snapshot directly because the
// SyncRun is still data_committed until prewarming finishes.
func (s *ReadService) Warm(ctx context.Context, orgID int64, asOfDate time.Time) error {
	snapshot, err := s.store.SnapshotForDate(ctx, orgID, domainv2.BusinessDate(asOfDate))
	if err != nil {
		return err
	}
	if snapshot == nil {
		return fmt.Errorf("statistics v2 snapshot is unavailable for cache warmup")
	}
	asOf := domainv2.BusinessDate(snapshot.AsOfDate)
	freshness := Freshness{AsOfDate: asOf.Format("2006-01-02"), SnapshotAt: snapshot.SnapshotAt, IsStale: false}
	ranges := []DateRange{
		{Preset: "latest_complete_day", From: asOf, To: asOf},
		{Preset: "7d", From: asOf.AddDate(0, 0, -6), To: asOf},
		{Preset: "30d", From: asOf.AddDate(0, 0, -29), To: asOf},
	}
	for _, value := range ranges {
		if _, err := s.overviewResolved(ctx, orgID, value, freshness); err != nil {
			return err
		}
	}
	return nil
}

func normalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

func (s *ReadService) Clinicians(ctx context.Context, orgID int64, clinicianID *uint64, operatorUserID *int64, filter QueryFilter, page, size int) (*Page[ClinicianItem], error) {
	r, freshness, err := s.resolve(ctx, orgID, filter)
	if err != nil {
		return nil, err
	}
	page, size = normalizePage(page, size)
	from, to := queryBounds(r)
	key := cacheKey("clinicians", clinicianID, operatorUserID, r.Preset, r.From, r.To, page, size)
	cached := &Page[ClinicianItem]{}
	if hit, stale := s.cacheGet(ctx, orgID, key, cached); hit {
		if stale {
			cached.Freshness.IsStale = true
		}
		return cached, nil
	}
	items, total, err := s.store.ListClinicians(ctx, orgID, clinicianID, operatorUserID, from, to, page, size)
	if err != nil {
		return nil, err
	}
	value := &Page[ClinicianItem]{Items: items, Total: total, Page: page, PageSize: size, TotalPages: int((total + int64(size) - 1) / int64(size)), TimeRange: r, Freshness: freshness}
	s.cacheSet(ctx, orgID, key, value)
	return value, nil
}

func (s *ReadService) Entries(ctx context.Context, orgID int64, entryID, clinicianID *uint64, active *bool, filter QueryFilter, page, size int) (*Page[EntryItem], error) {
	r, freshness, err := s.resolve(ctx, orgID, filter)
	if err != nil {
		return nil, err
	}
	page, size = normalizePage(page, size)
	from, to := queryBounds(r)
	key := cacheKey("entries", entryID, clinicianID, active, r.Preset, r.From, r.To, page, size)
	cached := &Page[EntryItem]{}
	if hit, stale := s.cacheGet(ctx, orgID, key, cached); hit {
		if stale {
			cached.Freshness.IsStale = true
		}
		return cached, nil
	}
	items, total, err := s.store.ListEntries(ctx, orgID, entryID, clinicianID, active, from, to, page, size)
	if err != nil {
		return nil, err
	}
	value := &Page[EntryItem]{Items: items, Total: total, Page: page, PageSize: size, TotalPages: int((total + int64(size) - 1) / int64(size)), TimeRange: r, Freshness: freshness}
	s.cacheSet(ctx, orgID, key, value)
	return value, nil
}

func (s *ReadService) CurrentClinicianID(ctx context.Context, orgID, userID int64) (uint64, error) {
	return s.store.CurrentClinicianID(ctx, orgID, userID)
}

func (s *ReadService) CurrentClinicianTesteeSummary(ctx context.Context, orgID, userID int64, filter QueryFilter) (*TesteeSummary, error) {
	r, freshness, err := s.resolve(ctx, orgID, filter)
	if err != nil {
		return nil, err
	}
	clinicianID, err := s.store.CurrentClinicianID(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	from, to := queryBounds(r)
	key := cacheKey("clinician_testees", clinicianID, r.Preset, r.From, r.To)
	cached := &TesteeSummary{}
	if hit, stale := s.cacheGet(ctx, orgID, key, cached); hit {
		if stale {
			cached.Freshness.IsStale = true
		}
		return cached, nil
	}
	value, err := s.store.CurrentClinicianTesteeSummary(ctx, orgID, clinicianID, from, to)
	if err != nil {
		return nil, err
	}
	value.TimeRange, value.Freshness = r, freshness
	s.cacheSet(ctx, orgID, key, &value)
	return &value, nil
}

func (s *ReadService) Contents(ctx context.Context, orgID int64, refs []ContentRef) (*ContentBatch, error) {
	_, freshness, err := s.resolve(ctx, orgID, QueryFilter{Preset: "latest_complete_day"})
	if err != nil {
		return nil, err
	}
	key := cacheKey("contents", refs)
	cached := &ContentBatch{}
	if hit, stale := s.cacheGet(ctx, orgID, key, cached); hit {
		if stale {
			cached.Freshness.IsStale = true
		}
		return cached, nil
	}
	items, err := s.store.ContentBatch(ctx, orgID, refs)
	if err != nil {
		return nil, err
	}
	value := &ContentBatch{Items: items, Freshness: freshness}
	s.cacheSet(ctx, orgID, key, value)
	return value, nil
}
