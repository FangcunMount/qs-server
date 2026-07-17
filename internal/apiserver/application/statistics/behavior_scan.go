package statistics

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

type behaviorJourneyScanner struct {
	uow          transactionRunner
	journeyRepo  BehaviorJourneyRepository
	scanRepo     BehaviorJourneyScanStateRepository
	rebuilder    JourneyProjectionRebuilder
	answerSheets AnswerSheetScanSource
	reports      ReportScanSource
	lifecycler   episodeLifecycler
}

type transactionRunner interface {
	WithinTransaction(ctx context.Context, fn func(context.Context) error) error
}

type scanBatchResult struct {
	scanned      int
	projected    int
	lastSeenID   uint64
	lastSeenTime *time.Time
}

// NewBehaviorJourneyScanService 创建background scan 投影器。
func NewBehaviorJourneyScanService(
	runner transactionRunner,
	journeyRepo BehaviorJourneyRepository,
	scanRepo BehaviorJourneyScanStateRepository,
	rebuilder JourneyProjectionRebuilder,
	answerSheets AnswerSheetScanSource,
	reports ReportScanSource,
) BehaviorJourneyScanService {
	if runner == nil || journeyRepo == nil || scanRepo == nil || rebuilder == nil {
		return nil
	}
	journey := journeyWriter{repo: journeyRepo}
	return &behaviorJourneyScanner{
		uow:          runner,
		journeyRepo:  journeyRepo,
		scanRepo:     scanRepo,
		rebuilder:    rebuilder,
		answerSheets: answerSheets,
		reports:      reports,
		lifecycler:   episodeLifecycler{repo: journeyRepo, journey: journey},
	}
}

func (s *behaviorJourneyScanner) ScanDue(ctx context.Context, input BehaviorJourneyScanInput) (BehaviorJourneyScanResult, error) {
	startedAt := time.Now()
	defer observeBehaviorScanDuration(startedAt)
	result := BehaviorJourneyScanResult{}
	if s == nil {
		return result, nil
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	batchSize := input.BatchSize
	if batchSize <= 0 {
		batchSize = 1000
	}
	lookback := input.Lookback
	if lookback <= 0 {
		lookback = 2 * time.Hour
	}
	sources := input.Sources
	if len(sources) == 0 {
		sources = []string{
			domainStatistics.ScanSourceEntryResolve,
			domainStatistics.ScanSourceEntryIntake,
			domainStatistics.ScanSourceAnswerSheet,
			domainStatistics.ScanSourceAssessment,
			domainStatistics.ScanSourceReport,
		}
	}

	for _, orgID := range input.OrgIDs {
		for _, source := range sources {
			sourceResult := s.scanSource(ctx, orgID, source, batchSize, lookback, now, input.DryRun, input.WindowRecalc)
			result.SourceResults = append(result.SourceResults, sourceResult)
			observeBehaviorScanSource(sourceResult)
			if sourceResult.Error != "" {
				logger.L(ctx).Warnw("behavior journey scan source partially failed",
					"org_id", orgID,
					"source", source,
					"scanned", sourceResult.Scanned,
					"projected", sourceResult.Projected,
					"skipped", sourceResult.Skipped,
					"failed", sourceResult.Failed,
					"error", sourceResult.Error,
				)
			}
		}
		if input.WindowRecalc && !input.DryRun {
			recalcResult := s.recalcJourneyDailyWindow(ctx, orgID, lookback, now)
			result.RecalcResults = append(result.RecalcResults, recalcResult)
		}
	}
	return result, nil
}

func (s *behaviorJourneyScanner) lifecyclerForScan(skipStatisticsMutations bool) episodeLifecycler {
	lc := s.lifecycler
	lc.skipStatisticsMutations = skipStatisticsMutations
	return lc
}

func (s *behaviorJourneyScanner) recalcJourneyDailyWindow(ctx context.Context, orgID int64, lookback time.Duration, now time.Time) BehaviorJourneyScanRecalcResult {
	startedAt := time.Now()
	startDate, endDate := journeyRecalcWindow(now, lookback)
	result := BehaviorJourneyScanRecalcResult{
		OrgID:     orgID,
		StartDate: startDate,
		EndDate:   endDate,
	}
	if !startDate.Before(endDate) {
		return result
	}
	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		return s.rebuilder.RebuildJourneyDailyWindow(txCtx, orgID, startDate, endDate)
	})
	observeBehaviorProjectionRebuild(startedAt, err)
	if err != nil {
		result.Error = err.Error()
	}
	return result
}

func journeyRecalcWindow(now time.Time, lookback time.Duration) (time.Time, time.Time) {
	loc := now.Location()
	windowStart := now.Add(-lookback)
	startDate := time.Date(windowStart.Year(), windowStart.Month(), windowStart.Day(), 0, 0, 0, 0, loc)
	endDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).Add(24 * time.Hour)
	return startDate, endDate
}

func (s *behaviorJourneyScanner) scanSource(
	ctx context.Context,
	orgID int64,
	source string,
	batchSize int,
	lookback time.Duration,
	now time.Time,
	dryRun bool,
	windowRecalc bool,
) BehaviorJourneyScanSourceResult {
	result := BehaviorJourneyScanSourceResult{SourceName: source, OrgID: orgID}
	watermark, err := s.scanRepo.LoadScanWatermark(ctx, orgID, source)
	if err != nil {
		result.Error = err.Error()
		result.Failed = 1
		return result
	}
	if watermark == nil {
		start := now.Add(-lookback)
		watermark = &domainStatistics.ScanWatermark{
			SourceName:      source,
			OrgID:           orgID,
			LastSeenTime:    &start,
			ScanWindowStart: &start,
			Status:          domainStatistics.ScanWatermarkStatusIdle,
		}
	}
	sinceTime := now.Add(-lookback)
	if watermark.LastSeenTime != nil && watermark.LastSeenTime.After(sinceTime) {
		sinceTime = *watermark.LastSeenTime
	}
	windowEnd := now
	watermark.Status = domainStatistics.ScanWatermarkStatusRunning
	watermark.ScanWindowStart = &sinceTime
	watermark.ScanWindowEnd = &windowEnd
	watermark.LastError = ""
	if !dryRun {
		if err := s.scanRepo.SaveScanWatermark(ctx, watermark); err != nil {
			result.Error = err.Error()
			result.Failed = 1
			return result
		}
	}

	var batch scanBatchResult
	switch source {
	case domainStatistics.ScanSourceEntryResolve:
		batch, err = s.scanEntryResolve(ctx, orgID, watermark.LastSeenID, sinceTime, batchSize, dryRun, windowRecalc)
	case domainStatistics.ScanSourceEntryIntake:
		batch, err = s.scanEntryIntake(ctx, orgID, watermark.LastSeenID, sinceTime, batchSize, dryRun, windowRecalc)
	case domainStatistics.ScanSourceAnswerSheet:
		batch, err = s.scanAnswerSheets(ctx, orgID, watermark.LastSeenID, sinceTime, batchSize, dryRun, windowRecalc)
	case domainStatistics.ScanSourceAssessment:
		batch, err = s.scanAssessments(ctx, orgID, watermark.LastSeenID, sinceTime, batchSize, dryRun, windowRecalc)
	case domainStatistics.ScanSourceReport:
		batch, err = s.scanReports(ctx, orgID, watermark.LastSeenID, sinceTime, batchSize, dryRun, windowRecalc)
	default:
		err = nil
	}
	result.Scanned = batch.scanned
	result.Projected = batch.projected
	if err != nil {
		watermark.Status = domainStatistics.ScanWatermarkStatusFailed
		watermark.LastError = err.Error()
		result.Error = err.Error()
		result.Failed = batch.scanned - batch.projected
		if !dryRun {
			_ = s.scanRepo.SaveScanWatermark(ctx, watermark)
		}
		return result
	}
	if batch.scanned > 0 && batch.lastSeenTime != nil {
		watermark.LastSeenID = batch.lastSeenID
		watermark.LastSeenTime = batch.lastSeenTime
	}
	watermark.Status = domainStatistics.ScanWatermarkStatusIdle
	if !dryRun {
		if err := s.scanRepo.SaveScanWatermark(ctx, watermark); err != nil {
			result.Error = err.Error()
			result.Failed = 1
			return result
		}
	}
	result.Skipped = batch.scanned - batch.projected
	return result
}

func scanBatchFromFacts[T any](
	facts []T,
	dryRun bool,
	lastSeenID func(T) uint64,
	lastSeenTime func(T) time.Time,
	project func(T) error,
) (scanBatchResult, error) {
	result := scanBatchResult{scanned: len(facts)}
	for _, fact := range facts {
		if dryRun {
			result.projected++
			continue
		}
		if err := project(fact); err != nil {
			return result, err
		}
		result.projected++
	}
	if len(facts) > 0 {
		last := facts[len(facts)-1]
		occurredAt := lastSeenTime(last)
		result.lastSeenID = lastSeenID(last)
		result.lastSeenTime = &occurredAt
	}
	return result, nil
}

func (s *behaviorJourneyScanner) scanEntryResolve(
	ctx context.Context,
	orgID int64,
	sinceID uint64,
	sinceTime time.Time,
	limit int,
	dryRun bool,
	windowRecalc bool,
) (scanBatchResult, error) {
	facts, err := s.scanRepo.ListEntryResolveFacts(ctx, orgID, sinceID, sinceTime, limit)
	if err != nil {
		return scanBatchResult{}, err
	}
	lc := s.lifecyclerForScan(windowRecalc)
	return scanBatchFromFacts(facts, dryRun,
		func(f domainStatistics.EntryResolveFact) uint64 { return f.LogID },
		func(f domainStatistics.EntryResolveFact) time.Time { return f.OccurredAt },
		func(f domainStatistics.EntryResolveFact) error {
			return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
				return s.projectEntryResolve(txCtx, lc, f)
			})
		},
	)
}

func (s *behaviorJourneyScanner) scanEntryIntake(
	ctx context.Context,
	orgID int64,
	sinceID uint64,
	sinceTime time.Time,
	limit int,
	dryRun bool,
	windowRecalc bool,
) (scanBatchResult, error) {
	facts, err := s.scanRepo.ListEntryIntakeFacts(ctx, orgID, sinceID, sinceTime, limit)
	if err != nil {
		return scanBatchResult{}, err
	}
	lc := s.lifecyclerForScan(windowRecalc)
	return scanBatchFromFacts(facts, dryRun,
		func(f domainStatistics.EntryIntakeFact) uint64 { return f.LogID },
		func(f domainStatistics.EntryIntakeFact) time.Time { return f.OccurredAt },
		func(f domainStatistics.EntryIntakeFact) error {
			return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
				return s.projectEntryIntake(txCtx, lc, f)
			})
		},
	)
}

func (s *behaviorJourneyScanner) scanAnswerSheets(
	ctx context.Context,
	orgID int64,
	sinceID uint64,
	sinceTime time.Time,
	limit int,
	dryRun bool,
	windowRecalc bool,
) (scanBatchResult, error) {
	if s.answerSheets == nil {
		return scanBatchResult{}, nil
	}
	facts, err := s.answerSheets.ListSubmittedAnswerSheetFacts(ctx, orgID, sinceID, sinceTime, limit)
	if err != nil {
		return scanBatchResult{}, err
	}
	lc := s.lifecyclerForScan(windowRecalc)
	return scanBatchFromFacts(facts, dryRun,
		func(f domainStatistics.AnswerSheetSubmittedFact) uint64 { return f.AnswerSheetID },
		func(f domainStatistics.AnswerSheetSubmittedFact) time.Time { return f.OccurredAt },
		func(f domainStatistics.AnswerSheetSubmittedFact) error {
			return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
				return s.projectAnswerSheetSubmitted(txCtx, lc, f)
			})
		},
	)
}

func (s *behaviorJourneyScanner) scanAssessments(
	ctx context.Context,
	orgID int64,
	sinceID uint64,
	sinceTime time.Time,
	limit int,
	dryRun bool,
	windowRecalc bool,
) (scanBatchResult, error) {
	facts, err := s.scanRepo.ListAssessmentCreatedFacts(ctx, orgID, sinceID, sinceTime, limit)
	if err != nil {
		return scanBatchResult{}, err
	}
	lc := s.lifecyclerForScan(windowRecalc)
	return scanBatchFromFacts(facts, dryRun,
		func(f domainStatistics.AssessmentCreatedFact) uint64 { return f.AssessmentID },
		func(f domainStatistics.AssessmentCreatedFact) time.Time { return f.OccurredAt },
		func(f domainStatistics.AssessmentCreatedFact) error {
			return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
				return s.projectAssessmentCreated(txCtx, lc, f)
			})
		},
	)
}

func (s *behaviorJourneyScanner) scanReports(
	ctx context.Context,
	orgID int64,
	sinceID uint64,
	sinceTime time.Time,
	limit int,
	dryRun bool,
	windowRecalc bool,
) (scanBatchResult, error) {
	if s.reports == nil {
		return scanBatchResult{}, nil
	}
	facts, err := s.reports.ListReportGeneratedFacts(ctx, orgID, sinceID, sinceTime, limit)
	if err != nil {
		return scanBatchResult{}, err
	}
	lc := s.lifecyclerForScan(windowRecalc)
	return scanBatchFromFacts(facts, dryRun,
		func(f domainStatistics.ReportGeneratedFact) uint64 { return f.AssessmentID },
		func(f domainStatistics.ReportGeneratedFact) time.Time { return f.OccurredAt },
		func(f domainStatistics.ReportGeneratedFact) error {
			return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
				return s.projectReportGenerated(txCtx, lc, f)
			})
		},
	)
}

func (s *behaviorJourneyScanner) projectEntryResolve(ctx context.Context, lc episodeLifecycler, fact domainStatistics.EntryResolveFact) error {
	input := BehaviorProjectEventInput{
		EventID:     domainStatistics.ScanBehaviorFootprintID(domainStatistics.BehaviorEventEntryOpened, fact.LogID),
		EventType:   string(domainStatistics.BehaviorEventEntryOpened),
		OrgID:       fact.OrgID,
		ClinicianID: fact.ClinicianID,
		EntryID:     fact.EntryID,
		OccurredAt:  fact.OccurredAt,
	}
	return lc.applyEntryOpened(ctx, input)
}

func (s *behaviorJourneyScanner) projectEntryIntake(ctx context.Context, lc episodeLifecycler, fact domainStatistics.EntryIntakeFact) error {
	base := BehaviorProjectEventInput{
		OrgID:       fact.OrgID,
		ClinicianID: fact.ClinicianID,
		EntryID:     fact.EntryID,
		TesteeID:    fact.TesteeID,
		OccurredAt:  fact.OccurredAt,
	}
	intakeInput := base
	intakeInput.EventID = domainStatistics.ScanBehaviorFootprintID(domainStatistics.BehaviorEventIntakeConfirmed, fact.LogID)
	intakeInput.EventType = string(domainStatistics.BehaviorEventIntakeConfirmed)
	if err := lc.applyIntakeConfirmed(ctx, intakeInput); err != nil {
		return err
	}
	if fact.TesteeCreated {
		testeeInput := base
		testeeInput.EventID = domainStatistics.ScanBehaviorFootprintID(domainStatistics.BehaviorEventTesteeProfileCreated, fact.LogID)
		testeeInput.EventType = string(domainStatistics.BehaviorEventTesteeProfileCreated)
		if err := lc.applyTesteeProfileCreated(ctx, testeeInput); err != nil {
			return err
		}
	}
	if fact.AssignmentCreated {
		relationshipInput := base
		relationshipInput.EventID = domainStatistics.ScanBehaviorFootprintID(domainStatistics.BehaviorEventCareRelationshipEstablished, fact.LogID)
		relationshipInput.EventType = string(domainStatistics.BehaviorEventCareRelationshipEstablished)
		if err := lc.applyCareRelationshipEstablished(ctx, relationshipInput); err != nil {
			return err
		}
	}
	return nil
}

func (s *behaviorJourneyScanner) projectAnswerSheetSubmitted(ctx context.Context, lc episodeLifecycler, fact domainStatistics.AnswerSheetSubmittedFact) error {
	input := BehaviorProjectEventInput{
		EventID:       domainStatistics.ScanBehaviorFootprintID(domainStatistics.BehaviorEventAnswerSheetSubmitted, fact.AnswerSheetID),
		EventType:     string(domainStatistics.BehaviorEventAnswerSheetSubmitted),
		OrgID:         fact.OrgID,
		TesteeID:      fact.TesteeID,
		AnswerSheetID: fact.AnswerSheetID,
		OccurredAt:    fact.OccurredAt,
	}
	return lc.applyAnswerSheetSubmitted(ctx, input)
}

func (s *behaviorJourneyScanner) projectAssessmentCreated(ctx context.Context, lc episodeLifecycler, fact domainStatistics.AssessmentCreatedFact) error {
	input := BehaviorProjectEventInput{
		EventID:       domainStatistics.ScanBehaviorFootprintID(domainStatistics.BehaviorEventAssessmentCreated, fact.AssessmentID),
		EventType:     string(domainStatistics.BehaviorEventAssessmentCreated),
		OrgID:         fact.OrgID,
		TesteeID:      fact.TesteeID,
		AnswerSheetID: fact.AnswerSheetID,
		AssessmentID:  fact.AssessmentID,
		OccurredAt:    fact.OccurredAt,
	}
	_, err := lc.applyAssessmentCreated(ctx, input)
	return err
}

func (s *behaviorJourneyScanner) projectReportGenerated(ctx context.Context, lc episodeLifecycler, fact domainStatistics.ReportGeneratedFact) error {
	input := BehaviorProjectEventInput{
		EventID:      domainStatistics.ScanBehaviorFootprintID(domainStatistics.BehaviorEventReportGenerated, fact.ReportID),
		EventType:    string(domainStatistics.BehaviorEventReportGenerated),
		OrgID:        fact.OrgID,
		TesteeID:     fact.TesteeID,
		AssessmentID: fact.AssessmentID,
		ReportID:     fact.ReportID,
		OccurredAt:   fact.OccurredAt,
	}
	return s.projectReportGeneratedFromScan(ctx, lc, input)
}

func (s *behaviorJourneyScanner) projectReportGeneratedFromScan(ctx context.Context, lc episodeLifecycler, input BehaviorProjectEventInput) error {
	if err := lc.journey.appendBehaviorFootprint(ctx, input, domainStatistics.BehaviorEventReportGenerated, "report", input.ReportID, "assessment", input.AssessmentID); err != nil {
		return err
	}
	episode, err := s.journeyRepo.FindEpisodeByAssessmentID(ctx, input.OrgID, input.AssessmentID)
	if err != nil {
		return err
	}
	if episode == nil {
		return nil
	}
	if episode.ReportID != nil && *episode.ReportID == input.ReportID && episode.ReportGeneratedAt != nil {
		return nil
	}
	episode.ReportID = uint64Ptr(input.ReportID)
	episode.ReportGeneratedAt = timePtr(input.OccurredAt)
	episode.Status = domainStatistics.EpisodeStatusCompleted
	if err := s.journeyRepo.SaveEpisode(ctx, episode); err != nil {
		return err
	}
	if lc.skipStatisticsMutations {
		return nil
	}
	return s.journeyRepo.ApplyStatisticsJourneyMutation(ctx, domainStatistics.StatisticsJourneyMutation{
		OrgID:                 input.OrgID,
		ClinicianID:           valueOrZero(episode.ClinicianID),
		EntryID:               valueOrZero(episode.EntryID),
		StatDate:              input.OccurredAt,
		ReportGeneratedCount:  1,
		EpisodeCompletedCount: 1,
	})
}
