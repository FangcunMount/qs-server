package statistics

import (
	"context"
	"testing"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

func TestEpisodeJourneyMutationsCountFormedAssessmentsWhenReportIsReady(t *testing.T) {
	t.Parallel()

	submittedAt := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	assessmentCreatedAt := time.Date(2026, 4, 1, 9, 1, 0, 0, time.UTC)
	reportGeneratedAt := time.Date(2026, 4, 1, 9, 2, 0, 0, time.UTC)
	episode := &domainStatistics.AssessmentEpisode{
		OrgID:               1,
		SubmittedAt:         submittedAt,
		AssessmentCreatedAt: &assessmentCreatedAt,
		ReportGeneratedAt:   &reportGeneratedAt,
		Status:              domainStatistics.EpisodeStatusCompleted,
	}

	got := episodeJourneyMutations(episode)
	if len(got) != 2 {
		t.Fatalf("mutation count = %d, want 2", len(got))
	}
	if got[0].StatDate != submittedAt || got[0].AnswerSheetSubmittedCount != 1 {
		t.Fatalf("submitted mutation = %+v, want submitted count at submittedAt", got[0])
	}
	if got[1].StatDate != reportGeneratedAt ||
		got[1].AssessmentCreatedCount != 1 ||
		got[1].ReportGeneratedCount != 1 ||
		got[1].EpisodeCompletedCount != 1 {
		t.Fatalf("report-ready mutation = %+v, want formed/report/completed counts at reportGeneratedAt", got[1])
	}
}

func TestEpisodeJourneyMutationsDoNotCountRawAssessmentCreationAsFormedAssessment(t *testing.T) {
	t.Parallel()

	submittedAt := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	assessmentCreatedAt := time.Date(2026, 4, 1, 9, 1, 0, 0, time.UTC)
	episode := &domainStatistics.AssessmentEpisode{
		OrgID:               1,
		SubmittedAt:         submittedAt,
		AssessmentCreatedAt: &assessmentCreatedAt,
		Status:              domainStatistics.EpisodeStatusActive,
	}

	got := episodeJourneyMutations(episode)
	if len(got) != 1 {
		t.Fatalf("mutation count = %d, want only submitted mutation", len(got))
	}
	if got[0].AssessmentCreatedCount != 0 || got[0].ReportGeneratedCount != 0 {
		t.Fatalf("raw assessment creation should not affect formed/report counts: %+v", got[0])
	}
}

type behaviorProjectorTxMarkerKey struct{}

type behaviorProjectorRunnerStub struct {
	txCount int
}

func (r *behaviorProjectorRunnerStub) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	r.txCount++
	return fn(context.WithValue(ctx, behaviorProjectorTxMarkerKey{}, "tx"))
}

type behaviorProjectorRepoStub struct {
	checkpointStatus string
	pendingRows      []*domainStatistics.AnalyticsPendingEvent
	episodeBySheet   *domainStatistics.AssessmentEpisode
	episodeByAssess  *domainStatistics.AssessmentEpisode
	latestFootprint  *domainStatistics.BehaviorFootprint

	footprints         []*domainStatistics.BehaviorFootprint
	savedEpisodes      []*domainStatistics.AssessmentEpisode
	mutations          []domainStatistics.StatisticsJourneyMutation
	clinicianMutations []domainStatistics.StatisticsJourneyMutation
	entryMutations     []domainStatistics.StatisticsJourneyMutation
	pendingPayload     string
	pendingReason      string
	rescheduled        []string
	deletedPending     []string
	markedStatuses     []string
	sawTxCtx           bool
}

func (r *behaviorProjectorRepoStub) markTx(ctx context.Context) {
	if ctx.Value(behaviorProjectorTxMarkerKey{}) == "tx" {
		r.sawTxCtx = true
	}
}

func (r *behaviorProjectorRepoStub) AppendBehaviorFootprint(ctx context.Context, footprint *domainStatistics.BehaviorFootprint) error {
	r.markTx(ctx)
	r.footprints = append(r.footprints, footprint)
	return nil
}

func (r *behaviorProjectorRepoStub) FindLatestFootprintByEvent(ctx context.Context, _ int64, _ uint64, _ domainStatistics.BehaviorEventName, _ time.Time, _ time.Duration) (*domainStatistics.BehaviorFootprint, error) {
	r.markTx(ctx)
	return r.latestFootprint, nil
}

func (r *behaviorProjectorRepoStub) SaveEpisode(ctx context.Context, episode *domainStatistics.AssessmentEpisode) error {
	r.markTx(ctx)
	r.savedEpisodes = append(r.savedEpisodes, episode)
	return nil
}

func (r *behaviorProjectorRepoStub) FindEpisodeByAnswerSheetID(ctx context.Context, _ int64, _ uint64) (*domainStatistics.AssessmentEpisode, error) {
	r.markTx(ctx)
	return r.episodeBySheet, nil
}

func (r *behaviorProjectorRepoStub) FindEpisodeByAssessmentID(ctx context.Context, _ int64, _ uint64) (*domainStatistics.AssessmentEpisode, error) {
	r.markTx(ctx)
	return r.episodeByAssess, nil
}

func (r *behaviorProjectorRepoStub) ApplyStatisticsJourneyMutation(ctx context.Context, mutation domainStatistics.StatisticsJourneyMutation) error {
	r.markTx(ctx)
	r.mutations = append(r.mutations, mutation)
	return nil
}

func (r *behaviorProjectorRepoStub) ApplyStatisticsJourneyClinicianMutation(ctx context.Context, mutation domainStatistics.StatisticsJourneyMutation) error {
	r.markTx(ctx)
	r.clinicianMutations = append(r.clinicianMutations, mutation)
	return nil
}

func (r *behaviorProjectorRepoStub) ApplyStatisticsJourneyEntryMutation(ctx context.Context, mutation domainStatistics.StatisticsJourneyMutation) error {
	r.markTx(ctx)
	r.entryMutations = append(r.entryMutations, mutation)
	return nil
}

func (r *behaviorProjectorRepoStub) ListEpisodesForAttribution(ctx context.Context, _ int64, _ uint64, _ time.Time, _ time.Duration) ([]*domainStatistics.AssessmentEpisode, error) {
	r.markTx(ctx)
	return nil, nil
}

func (r *behaviorProjectorRepoStub) TryBeginAnalyticsProjectorCheckpoint(ctx context.Context, _, _ string) (string, error) {
	r.markTx(ctx)
	return r.checkpointStatus, nil
}

func (r *behaviorProjectorRepoStub) MarkAnalyticsProjectorCheckpointStatus(ctx context.Context, eventID, status string) error {
	r.markTx(ctx)
	r.markedStatuses = append(r.markedStatuses, eventID+":"+status)
	return nil
}

func (r *behaviorProjectorRepoStub) UpsertAnalyticsPendingEvent(ctx context.Context, _ string, _ string, payload string, _ time.Time, lastError string) error {
	r.markTx(ctx)
	r.pendingPayload = payload
	r.pendingReason = lastError
	return nil
}

func (r *behaviorProjectorRepoStub) ListDueAnalyticsPendingEvents(_ context.Context, _ int, _ time.Time) ([]*domainStatistics.AnalyticsPendingEvent, error) {
	return r.pendingRows, nil
}

func (r *behaviorProjectorRepoStub) RescheduleAnalyticsPendingEvent(ctx context.Context, eventID, lastError string, _ time.Time) error {
	r.markTx(ctx)
	r.rescheduled = append(r.rescheduled, eventID+":"+lastError)
	return nil
}

func (r *behaviorProjectorRepoStub) DeleteAnalyticsPendingEvent(ctx context.Context, eventID string) error {
	r.markTx(ctx)
	r.deletedPending = append(r.deletedPending, eventID)
	return nil
}

func TestBehaviorProjectorAnswerSheetSubmittedCreatesEpisodeFootprintAndJourneyDailyInTransaction(t *testing.T) {
	occurredAt := time.Date(2026, 4, 28, 9, 0, 0, 0, time.UTC)
	repo := &behaviorProjectorRepoStub{}
	runner := &behaviorProjectorRunnerStub{}
	projector := NewAssessmentEpisodeProjectorWithTransactionRunner(runner, repo)

	result, err := projector.ProjectBehaviorEvent(t.Context(), BehaviorProjectEventInput{
		EventID:       "evt-answer",
		EventType:     domainStatistics.EventTypeFootprintAnswerSheetSubmitted,
		OrgID:         1,
		TesteeID:      2,
		AnswerSheetID: 3,
		OccurredAt:    occurredAt,
	})
	if err != nil {
		t.Fatalf("ProjectBehaviorEvent() error = %v", err)
	}
	if result.Status != BehaviorProjectEventStatusCompleted {
		t.Fatalf("status = %s, want completed", result.Status)
	}
	if runner.txCount != 1 || !repo.sawTxCtx {
		t.Fatalf("expected transaction context, txCount=%d sawTx=%v", runner.txCount, repo.sawTxCtx)
	}
	if len(repo.footprints) != 1 || repo.footprints[0].EventName != domainStatistics.BehaviorEventAnswerSheetSubmitted {
		t.Fatalf("footprints = %+v, want answer sheet submitted footprint", repo.footprints)
	}
	if len(repo.savedEpisodes) != 1 || repo.savedEpisodes[0].EpisodeID != 3 || repo.savedEpisodes[0].Status != domainStatistics.EpisodeStatusActive {
		t.Fatalf("saved episodes = %+v, want active episode for answersheet", repo.savedEpisodes)
	}
	if len(repo.mutations) != 1 || repo.mutations[0].AnswerSheetSubmittedCount != 1 {
		t.Fatalf("mutations = %+v, want submitted count", repo.mutations)
	}
	if len(repo.markedStatuses) != 1 || repo.markedStatuses[0] != "evt-answer:"+domainStatistics.AnalyticsProjectorCheckpointStatusCompleted {
		t.Fatalf("marked statuses = %+v, want completed checkpoint", repo.markedStatuses)
	}
}

func TestBehaviorProjectorAssessmentCreatedWithoutEpisodeQueuesPending(t *testing.T) {
	repo := &behaviorProjectorRepoStub{}
	runner := &behaviorProjectorRunnerStub{}
	projector := NewAssessmentEpisodeProjectorWithTransactionRunner(runner, repo)

	result, err := projector.ProjectBehaviorEvent(t.Context(), BehaviorProjectEventInput{
		EventID:       "evt-assessment",
		EventType:     domainStatistics.EventTypeFootprintAssessmentCreated,
		OrgID:         1,
		TesteeID:      2,
		AnswerSheetID: 3,
		AssessmentID:  4,
		OccurredAt:    time.Date(2026, 4, 28, 9, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ProjectBehaviorEvent() error = %v", err)
	}
	if result.Status != BehaviorProjectEventStatusPending {
		t.Fatalf("status = %s, want pending", result.Status)
	}
	if repo.pendingReason != "pending_attribution" || repo.pendingPayload == "" {
		t.Fatalf("pending reason/payload = %q/%q, want pending attribution payload", repo.pendingReason, repo.pendingPayload)
	}
	if len(repo.markedStatuses) != 1 || repo.markedStatuses[0] != "evt-assessment:"+domainStatistics.AnalyticsProjectorCheckpointStatusPending {
		t.Fatalf("marked statuses = %+v, want pending checkpoint", repo.markedStatuses)
	}
}

func TestBehaviorProjectorReconcileInvalidPayloadReschedules(t *testing.T) {
	repo := &behaviorProjectorRepoStub{
		pendingRows: []*domainStatistics.AnalyticsPendingEvent{{
			EventID:      "evt-bad",
			EventType:    domainStatistics.EventTypeFootprintAnswerSheetSubmitted,
			PayloadJSON:  "{bad-json",
			AttemptCount: 1,
		}},
	}
	projector := NewAssessmentEpisodeProjectorWithTransactionRunner(&behaviorProjectorRunnerStub{}, repo)

	processed, err := projector.ReconcilePendingBehaviorEvents(t.Context(), 10)
	if err != nil {
		t.Fatalf("ReconcilePendingBehaviorEvents() error = %v", err)
	}
	if processed != 0 {
		t.Fatalf("processed = %d, want 0 for invalid payload", processed)
	}
	if len(repo.rescheduled) != 1 || repo.rescheduled[0] == "evt-bad:" {
		t.Fatalf("rescheduled = %+v, want invalid payload reschedule", repo.rescheduled)
	}
}

func TestBehaviorProjectorReconcileCompletedDeletesPendingAndMarksCheckpoint(t *testing.T) {
	payload, err := marshalBehaviorProjectEventInput(BehaviorProjectEventInput{
		EventID:       "evt-pending",
		EventType:     domainStatistics.EventTypeFootprintAnswerSheetSubmitted,
		OrgID:         1,
		TesteeID:      2,
		AnswerSheetID: 3,
		OccurredAt:    time.Date(2026, 4, 28, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	repo := &behaviorProjectorRepoStub{
		pendingRows: []*domainStatistics.AnalyticsPendingEvent{{
			EventID:      "evt-pending",
			EventType:    domainStatistics.EventTypeFootprintAnswerSheetSubmitted,
			PayloadJSON:  payload,
			AttemptCount: 1,
		}},
	}
	projector := NewAssessmentEpisodeProjectorWithTransactionRunner(&behaviorProjectorRunnerStub{}, repo)

	processed, err := projector.ReconcilePendingBehaviorEvents(t.Context(), 10)
	if err != nil {
		t.Fatalf("ReconcilePendingBehaviorEvents() error = %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	if len(repo.deletedPending) != 1 || repo.deletedPending[0] != "evt-pending" {
		t.Fatalf("deleted pending = %+v, want evt-pending", repo.deletedPending)
	}
	if len(repo.markedStatuses) != 1 || repo.markedStatuses[0] != "evt-pending:"+domainStatistics.AnalyticsProjectorCheckpointStatusCompleted {
		t.Fatalf("marked statuses = %+v, want completed checkpoint", repo.markedStatuses)
	}
}
