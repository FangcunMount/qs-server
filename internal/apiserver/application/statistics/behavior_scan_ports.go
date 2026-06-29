package statistics

import (
	"context"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

// BehaviorJourneyScanService projects behavior journey statistics from fact tables.
type BehaviorJourneyScanService interface {
	ScanDue(ctx context.Context, input BehaviorJourneyScanInput) (BehaviorJourneyScanResult, error)
}

// BehaviorJourneyScanInput controls one scan invocation.
type BehaviorJourneyScanInput struct {
	OrgIDs       []int64
	Sources      []string
	BatchSize    int
	Lookback     time.Duration
	Now          time.Time
	DryRun       bool
	WindowRecalc bool
}

// BehaviorJourneyScanResult summarizes one scan invocation.
type BehaviorJourneyScanResult struct {
	SourceResults []BehaviorJourneyScanSourceResult
	RecalcResults []BehaviorJourneyScanRecalcResult
}

// BehaviorJourneyScanRecalcResult summarizes journey daily window recalculation for one org.
type BehaviorJourneyScanRecalcResult struct {
	OrgID     int64
	StartDate time.Time
	EndDate   time.Time
	Error     string
}

// BehaviorJourneyScanSourceResult summarizes one source/org scan batch.
type BehaviorJourneyScanSourceResult struct {
	SourceName  string
	OrgID       int64
	Scanned     int
	Projected   int
	Skipped     int
	Failed      int
	WatermarkID uint64
	Error       string
}

// BehaviorJourneyScanRepository loads scan facts and persists watermarks/projections.
type BehaviorJourneyScanRepository interface {
	BehaviorJourneyRepository
	LoadScanWatermark(ctx context.Context, orgID int64, sourceName string) (*domainStatistics.ScanWatermark, error)
	SaveScanWatermark(ctx context.Context, watermark *domainStatistics.ScanWatermark) error
	ListReportGeneratedFacts(ctx context.Context, orgID int64, sinceID uint64, sinceTime time.Time, limit int) ([]domainStatistics.ReportGeneratedFact, error)
	ListEntryResolveFacts(ctx context.Context, orgID int64, sinceID uint64, sinceTime time.Time, limit int) ([]domainStatistics.EntryResolveFact, error)
	ListEntryIntakeFacts(ctx context.Context, orgID int64, sinceID uint64, sinceTime time.Time, limit int) ([]domainStatistics.EntryIntakeFact, error)
	RebuildJourneyDailyWindow(ctx context.Context, orgID int64, startDate, endDate time.Time) error
}

// AnswerSheetScanSource lists submitted answer sheets from Mongo.
type AnswerSheetScanSource interface {
	ListSubmittedAnswerSheetFacts(ctx context.Context, orgID int64, sinceID uint64, sinceTime time.Time, limit int) ([]domainStatistics.AnswerSheetSubmittedFact, error)
}
