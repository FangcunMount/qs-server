package statistics

import (
	"context"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

type behaviorJourneyScanner struct {
	uow          transactionRunner
	repo         BehaviorJourneyScanRepository
	answerSheets AnswerSheetScanSource
	lifecycler   episodeLifecycler
}

type transactionRunner interface {
	WithinTransaction(ctx context.Context, fn func(context.Context) error) error
}

// NewBehaviorJourneyScanService creates the background scan projector.
func NewBehaviorJourneyScanService(
	runner transactionRunner,
	repo BehaviorJourneyScanRepository,
	answerSheets AnswerSheetScanSource,
) BehaviorJourneyScanService {
	if runner == nil || repo == nil {
		return nil
	}
	journey := journeyWriter{repo: repo}
	return &behaviorJourneyScanner{
		uow:          runner,
		repo:         repo,
		answerSheets: answerSheets,
		lifecycler:   episodeLifecycler{repo: repo, journey: journey},
	}
}

func (s *behaviorJourneyScanner) ScanDue(ctx context.Context, input BehaviorJourneyScanInput) (BehaviorJourneyScanResult, error) {
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
			domainStatistics.ScanSourceReport,
		}
	}
	windowRecalc := input.WindowRecalc
	s.lifecycler.skipStatisticsMutations = windowRecalc

	for _, orgID := range input.OrgIDs {
		for _, source := range sources {
			sourceResult := s.scanSource(ctx, orgID, source, batchSize, lookback, now, input.DryRun)
			result.SourceResults = append(result.SourceResults, sourceResult)
		}
		if windowRecalc && !input.DryRun {
			recalcResult := s.recalcJourneyDailyWindow(ctx, orgID, lookback, now)
			result.RecalcResults = append(result.RecalcResults, recalcResult)
		}
	}
	return result, nil
}

func (s *behaviorJourneyScanner) recalcJourneyDailyWindow(ctx context.Context, orgID int64, lookback time.Duration, now time.Time) BehaviorJourneyScanRecalcResult {
	startDate, endDate := journeyRecalcWindow(now, lookback)
	result := BehaviorJourneyScanRecalcResult{
		OrgID:     orgID,
		StartDate: startDate,
		EndDate:   endDate,
	}
	if !startDate.Before(endDate) {
		return result
	}
	if err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		return s.repo.RebuildJourneyDailyWindow(txCtx, orgID, startDate, endDate)
	}); err != nil {
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
) BehaviorJourneyScanSourceResult {
	result := BehaviorJourneyScanSourceResult{SourceName: source, OrgID: orgID}
	watermark, err := s.repo.LoadScanWatermark(ctx, orgID, source)
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
		if err := s.repo.SaveScanWatermark(ctx, watermark); err != nil {
			result.Error = err.Error()
			result.Failed = 1
			return result
		}
	}

	var projected int
	var scanned int
	switch source {
	case domainStatistics.ScanSourceEntryResolve:
		scanned, projected, err = s.scanEntryResolve(ctx, orgID, watermark.LastSeenID, sinceTime, batchSize, dryRun)
	case domainStatistics.ScanSourceEntryIntake:
		scanned, projected, err = s.scanEntryIntake(ctx, orgID, watermark.LastSeenID, sinceTime, batchSize, dryRun)
	case domainStatistics.ScanSourceAnswerSheet:
		scanned, projected, err = s.scanAnswerSheets(ctx, orgID, watermark.LastSeenID, sinceTime, batchSize, dryRun)
	case domainStatistics.ScanSourceReport:
		scanned, projected, err = s.scanReports(ctx, orgID, watermark.LastSeenID, sinceTime, batchSize, dryRun)
	default:
		err = nil
	}
	result.Scanned = scanned
	result.Projected = projected
	if err != nil {
		watermark.Status = domainStatistics.ScanWatermarkStatusFailed
		watermark.LastError = err.Error()
		result.Error = err.Error()
		result.Failed = scanned - projected
		if !dryRun {
			_ = s.repo.SaveScanWatermark(ctx, watermark)
		}
		return result
	}
	if scanned > 0 {
		switch source {
		case domainStatistics.ScanSourceEntryResolve:
			if facts, listErr := s.repo.ListEntryResolveFacts(ctx, orgID, watermark.LastSeenID, sinceTime, batchSize); listErr == nil && len(facts) > 0 {
				last := facts[len(facts)-1]
				watermark.LastSeenID = last.LogID
				occurredAt := last.OccurredAt
				watermark.LastSeenTime = &occurredAt
			}
		case domainStatistics.ScanSourceEntryIntake:
			if facts, listErr := s.repo.ListEntryIntakeFacts(ctx, orgID, watermark.LastSeenID, sinceTime, batchSize); listErr == nil && len(facts) > 0 {
				last := facts[len(facts)-1]
				watermark.LastSeenID = last.LogID
				occurredAt := last.OccurredAt
				watermark.LastSeenTime = &occurredAt
			}
		case domainStatistics.ScanSourceAnswerSheet:
			if facts, listErr := s.answerSheets.ListSubmittedAnswerSheetFacts(ctx, orgID, watermark.LastSeenID, sinceTime, batchSize); listErr == nil && len(facts) > 0 {
				last := facts[len(facts)-1]
				watermark.LastSeenID = last.AnswerSheetID
				occurredAt := last.OccurredAt
				watermark.LastSeenTime = &occurredAt
			}
		case domainStatistics.ScanSourceReport:
			if facts, listErr := s.repo.ListReportGeneratedFacts(ctx, orgID, watermark.LastSeenID, sinceTime, batchSize); listErr == nil && len(facts) > 0 {
				last := facts[len(facts)-1]
				watermark.LastSeenID = last.AssessmentID
				occurredAt := last.OccurredAt
				watermark.LastSeenTime = &occurredAt
			}
		}
	}
	watermark.Status = domainStatistics.ScanWatermarkStatusIdle
	if !dryRun {
		if err := s.repo.SaveScanWatermark(ctx, watermark); err != nil {
			result.Error = err.Error()
			result.Failed = 1
			return result
		}
	}
	result.Skipped = scanned - projected
	return result
}

func (s *behaviorJourneyScanner) scanEntryResolve(
	ctx context.Context,
	orgID int64,
	sinceID uint64,
	sinceTime time.Time,
	limit int,
	dryRun bool,
) (int, int, error) {
	facts, err := s.repo.ListEntryResolveFacts(ctx, orgID, sinceID, sinceTime, limit)
	if err != nil {
		return 0, 0, err
	}
	projected := 0
	for _, fact := range facts {
		if dryRun {
			projected++
			continue
		}
		if err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
			return s.projectEntryResolve(txCtx, fact)
		}); err != nil {
			return len(facts), projected, err
		}
		projected++
	}
	return len(facts), projected, nil
}

func (s *behaviorJourneyScanner) scanEntryIntake(
	ctx context.Context,
	orgID int64,
	sinceID uint64,
	sinceTime time.Time,
	limit int,
	dryRun bool,
) (int, int, error) {
	facts, err := s.repo.ListEntryIntakeFacts(ctx, orgID, sinceID, sinceTime, limit)
	if err != nil {
		return 0, 0, err
	}
	projected := 0
	for _, fact := range facts {
		if dryRun {
			projected++
			continue
		}
		if err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
			return s.projectEntryIntake(txCtx, fact)
		}); err != nil {
			return len(facts), projected, err
		}
		projected++
	}
	return len(facts), projected, nil
}

func (s *behaviorJourneyScanner) projectEntryResolve(ctx context.Context, fact domainStatistics.EntryResolveFact) error {
	input := BehaviorProjectEventInput{
		EventID:     domainStatistics.ScanBehaviorFootprintID(domainStatistics.BehaviorEventEntryOpened, fact.LogID),
		EventType:   eventcatalog.FootprintEntryOpened,
		OrgID:       fact.OrgID,
		ClinicianID: fact.ClinicianID,
		EntryID:     fact.EntryID,
		OccurredAt:  fact.OccurredAt,
	}
	return s.lifecycler.applyEntryOpened(ctx, input)
}

func (s *behaviorJourneyScanner) projectEntryIntake(ctx context.Context, fact domainStatistics.EntryIntakeFact) error {
	base := BehaviorProjectEventInput{
		OrgID:       fact.OrgID,
		ClinicianID: fact.ClinicianID,
		EntryID:     fact.EntryID,
		TesteeID:    fact.TesteeID,
		OccurredAt:  fact.OccurredAt,
	}
	intakeInput := base
	intakeInput.EventID = domainStatistics.ScanBehaviorFootprintID(domainStatistics.BehaviorEventIntakeConfirmed, fact.LogID)
	intakeInput.EventType = eventcatalog.FootprintIntakeConfirmed
	if err := s.lifecycler.applyIntakeConfirmed(ctx, intakeInput); err != nil {
		return err
	}
	if fact.TesteeCreated {
		testeeInput := base
		testeeInput.EventID = domainStatistics.ScanBehaviorFootprintID(domainStatistics.BehaviorEventTesteeProfileCreated, fact.LogID)
		testeeInput.EventType = eventcatalog.FootprintTesteeProfileCreated
		if err := s.lifecycler.applyTesteeProfileCreated(ctx, testeeInput); err != nil {
			return err
		}
	}
	if fact.AssignmentCreated {
		relationshipInput := base
		relationshipInput.EventID = domainStatistics.ScanBehaviorFootprintID(domainStatistics.BehaviorEventCareRelationshipEstablished, fact.LogID)
		relationshipInput.EventType = eventcatalog.FootprintCareRelationshipEstablished
		if err := s.lifecycler.applyCareRelationshipEstablished(ctx, relationshipInput); err != nil {
			return err
		}
	}
	return nil
}

func (s *behaviorJourneyScanner) scanAnswerSheets(
	ctx context.Context,
	orgID int64,
	sinceID uint64,
	sinceTime time.Time,
	limit int,
	dryRun bool,
) (int, int, error) {
	if s.answerSheets == nil {
		return 0, 0, nil
	}
	facts, err := s.answerSheets.ListSubmittedAnswerSheetFacts(ctx, orgID, sinceID, sinceTime, limit)
	if err != nil {
		return 0, 0, err
	}
	projected := 0
	for _, fact := range facts {
		if dryRun {
			projected++
			continue
		}
		if err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
			return s.projectAnswerSheetSubmitted(txCtx, fact)
		}); err != nil {
			return len(facts), projected, err
		}
		projected++
	}
	return len(facts), projected, nil
}

func (s *behaviorJourneyScanner) scanReports(
	ctx context.Context,
	orgID int64,
	sinceID uint64,
	sinceTime time.Time,
	limit int,
	dryRun bool,
) (int, int, error) {
	facts, err := s.repo.ListReportGeneratedFacts(ctx, orgID, sinceID, sinceTime, limit)
	if err != nil {
		return 0, 0, err
	}
	projected := 0
	for _, fact := range facts {
		if dryRun {
			projected++
			continue
		}
		if err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
			return s.projectReportGenerated(txCtx, fact)
		}); err != nil {
			return len(facts), projected, err
		}
		projected++
	}
	return len(facts), projected, nil
}

func (s *behaviorJourneyScanner) projectAnswerSheetSubmitted(ctx context.Context, fact domainStatistics.AnswerSheetSubmittedFact) error {
	input := BehaviorProjectEventInput{
		EventID:       domainStatistics.ScanBehaviorFootprintID(domainStatistics.BehaviorEventAnswerSheetSubmitted, fact.AnswerSheetID),
		EventType:     eventcatalog.FootprintAnswerSheetSubmitted,
		OrgID:         fact.OrgID,
		TesteeID:      fact.TesteeID,
		AnswerSheetID: fact.AnswerSheetID,
		OccurredAt:    fact.OccurredAt,
	}
	return s.lifecycler.applyAnswerSheetSubmitted(ctx, input)
}

func (s *behaviorJourneyScanner) projectReportGenerated(ctx context.Context, fact domainStatistics.ReportGeneratedFact) error {
	input := BehaviorProjectEventInput{
		EventID:      domainStatistics.ScanBehaviorFootprintID(domainStatistics.BehaviorEventReportGenerated, fact.ReportID),
		EventType:    eventcatalog.FootprintReportGenerated,
		OrgID:        fact.OrgID,
		TesteeID:     fact.TesteeID,
		AssessmentID: fact.AssessmentID,
		ReportID:     fact.ReportID,
		OccurredAt:   fact.OccurredAt,
	}
	return s.projectReportGeneratedFromScan(ctx, input)
}

func (s *behaviorJourneyScanner) projectReportGeneratedFromScan(ctx context.Context, input BehaviorProjectEventInput) error {
	if err := s.lifecycler.journey.appendBehaviorFootprint(ctx, input, domainStatistics.BehaviorEventReportGenerated, "report", input.ReportID, "assessment", input.AssessmentID); err != nil {
		return err
	}
	episode, err := s.repo.FindEpisodeByAssessmentID(ctx, input.OrgID, input.AssessmentID)
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
	if err := s.repo.SaveEpisode(ctx, episode); err != nil {
		return err
	}
	if s.lifecycler.skipStatisticsMutations {
		return nil
	}
	return s.repo.ApplyStatisticsJourneyMutation(ctx, domainStatistics.StatisticsJourneyMutation{
		OrgID:                 input.OrgID,
		ClinicianID:           valueOrZero(episode.ClinicianID),
		EntryID:               valueOrZero(episode.EntryID),
		StatDate:              input.OccurredAt,
		ReportGeneratedCount:  1,
		EpisodeCompletedCount: 1,
	})
}
