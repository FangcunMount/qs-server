package statistics

import (
	"context"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

// BehaviorJourneyScanService 投影behavior journey statistics 从 fact tables。
type BehaviorJourneyScanService interface {
	ScanDue(ctx context.Context, input BehaviorJourneyScanInput) (BehaviorJourneyScanResult, error)
}

// BehaviorJourneyScanInput 控制一个scan invocation。
type BehaviorJourneyScanInput struct {
	OrgIDs       []int64
	Sources      []string
	BatchSize    int
	Lookback     time.Duration
	Now          time.Time
	DryRun       bool
	WindowRecalc bool
}

// BehaviorJourneyScanResult 汇总一个scan invocation。
type BehaviorJourneyScanResult struct {
	SourceResults []BehaviorJourneyScanSourceResult
	RecalcResults []BehaviorJourneyScanRecalcResult
}

// BehaviorJourneyScanRecalcResult 汇总journey daily 窗口 re计算 用于 一个org。
type BehaviorJourneyScanRecalcResult struct {
	OrgID     int64
	StartDate time.Time
	EndDate   time.Time
	Error     string
}

// BehaviorJourneyScanSourceResult 汇总一个来源/org scan batch。
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

// BehaviorJourneyScanStateRepository owns scan facts and watermarks.
type BehaviorJourneyScanStateRepository interface {
	LoadScanWatermark(ctx context.Context, orgID int64, sourceName string) (*domainStatistics.ScanWatermark, error)
	SaveScanWatermark(ctx context.Context, watermark *domainStatistics.ScanWatermark) error
	ListEntryResolveFacts(ctx context.Context, orgID int64, sinceID uint64, sinceTime time.Time, limit int) ([]domainStatistics.EntryResolveFact, error)
	ListEntryIntakeFacts(ctx context.Context, orgID int64, sinceID uint64, sinceTime time.Time, limit int) ([]domainStatistics.EntryIntakeFact, error)
	ListAssessmentCreatedFacts(ctx context.Context, orgID int64, sinceID uint64, sinceTime time.Time, limit int) ([]domainStatistics.AssessmentCreatedFact, error)
}

// JourneyProjectionRebuilder rebuilds a bounded journey projection window.
type JourneyProjectionRebuilder interface {
	RebuildJourneyDailyWindow(ctx context.Context, orgID int64, startDate, endDate time.Time) error
}

// ReportScanSource 列出generated reports 从 持久化 stores。
type ReportScanSource interface {
	ListReportGeneratedFacts(ctx context.Context, orgID int64, sinceID uint64, sinceTime time.Time, limit int) ([]domainStatistics.ReportGeneratedFact, error)
}

// AnswerSheetScanSource 列出submitted 答卷 从 Mongo。
type AnswerSheetScanSource interface {
	ListSubmittedAnswerSheetFacts(ctx context.Context, orgID int64, sinceID uint64, sinceTime time.Time, limit int) ([]domainStatistics.AnswerSheetSubmittedFact, error)
}
