// Package mongoconsistency implements the read-only Mongo side of the
// production consistency audit. This package intentionally contains no Mongo
// insert, update, replace, delete, or bulk-write operation.
package mongoconsistency

import (
	"context"
	"fmt"
	"strconv"
	"time"

	appaudit "github.com/FangcunMount/qs-server/internal/apiserver/application/mongoconsistency"
	domaininterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	modeldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	answersheetdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	modelmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	modelport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Scanner struct {
	db      *mongo.Database
	limiter backpressure.Acquirer
}

func NewScanner(db *mongo.Database, limiter backpressure.Acquirer) *Scanner {
	return &Scanner{db: db, limiter: limiter}
}

func (s *Scanner) UpperBound(ctx context.Context, phase appaudit.Phase, maxTime time.Duration) (uint64, error) {
	if s == nil || s.db == nil || maxTime <= 0 {
		return 0, fmt.Errorf("mongo consistency scanner is not configured")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return 0, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, maxTime)
	defer cancel()
	if phase == appaudit.PhaseOutboxAnswerSheet {
		return s.outboxUpperBound(ctx, maxTime)
	}
	collection, filter, err := s.anchor(phase)
	if err != nil {
		return 0, err
	}
	var last struct {
		DomainID uint64 `bson:"domain_id"`
	}
	err = collection.FindOne(ctx, filter, options.FindOne().SetSort(bson.D{{Key: "domain_id", Value: -1}}).SetProjection(bson.M{"domain_id": 1})).Decode(&last)
	if err == mongo.ErrNoDocuments {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return last.DomainID, nil
}

func (s *Scanner) ScanBatch(ctx context.Context, request appaudit.BatchRequest) (appaudit.BatchResult, error) {
	if s == nil || s.db == nil || request.Limit <= 0 || request.MaxTime <= 0 {
		return appaudit.BatchResult{}, fmt.Errorf("mongo consistency scan request is invalid")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return appaudit.BatchResult{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, request.MaxTime)
	defer cancel()
	switch request.Phase {
	case appaudit.PhaseAnswerSheetOutbox:
		return s.scanAnswerSheetOutbox(ctx, request)
	case appaudit.PhaseOutboxAnswerSheet:
		return s.scanOutboxAnswerSheet(ctx, request)
	case appaudit.PhaseGenerationRun:
		return s.scanGenerationRun(ctx, request)
	case appaudit.PhaseGeneratedTerminal:
		return s.scanGeneratedTerminal(ctx, request)
	case appaudit.PhaseRetryOutbox:
		return s.scanRetryOutbox(ctx, request)
	case appaudit.PhaseModelRelease:
		return s.scanModelRelease(ctx, request)
	case appaudit.PhasePublishedModelRuntime:
		return s.scanPublishedModelRuntime(ctx, request)
	default:
		return appaudit.BatchResult{}, fmt.Errorf("unsupported mongo consistency phase %q", request.Phase)
	}
}

func (s *Scanner) acquire(ctx context.Context) (context.Context, func(), error) {
	if s.limiter == nil {
		return ctx, func() {}, nil
	}
	return s.limiter.Acquire(ctx)
}

func (s *Scanner) anchor(phase appaudit.Phase) (*mongo.Collection, bson.M, error) {
	switch phase {
	case appaudit.PhaseAnswerSheetOutbox:
		return s.db.Collection("answersheets"), bson.M{"durable_acceptance.schema_version": 1, "deleted_at": nil}, nil
	case appaudit.PhaseGenerationRun:
		return s.db.Collection("report_generations"), bson.M{"transaction_schema_version": 1, "deleted_at": nil}, nil
	case appaudit.PhaseGeneratedTerminal:
		return s.db.Collection("report_generations"), bson.M{"transaction_schema_version": 1, "status": "generated", "deleted_at": nil}, nil
	case appaudit.PhaseRetryOutbox:
		return s.db.Collection("interpretation_runs"), bson.M{"retry_event_id": bson.M{"$type": "string", "$ne": ""}, "deleted_at": nil}, nil
	case appaudit.PhaseModelRelease:
		return s.db.Collection("assessment_models"), bson.M{"record_role": "head", "status": "published", "deleted_at": nil}, nil
	case appaudit.PhasePublishedModelRuntime:
		// Published snapshots historically have no positive domain_id. Page by
		// the published head and resolve its active snapshot by stable model code.
		return s.db.Collection("assessment_models"), bson.M{"record_role": "head", "status": "published", "deleted_at": nil}, nil
	default:
		return nil, nil, fmt.Errorf("phase %q has no numeric anchor", phase)
	}
}

func boundedFilter(base bson.M, request appaudit.BatchRequest) bson.M {
	filter := bson.M{}
	for key, value := range base {
		filter[key] = value
	}
	filter["domain_id"] = bson.M{"$gt": request.AfterID, "$lte": request.UpperBound}
	return filter
}

func findOptions(request appaudit.BatchRequest, projection bson.M) *options.FindOptions {
	opts := options.Find().SetSort(bson.D{{Key: "domain_id", Value: 1}}).SetLimit(int64(request.Limit)).SetMaxTime(request.MaxTime)
	if projection != nil {
		opts.SetProjection(projection)
	}
	return opts
}

func batchDone(scanned int, next uint64, request appaudit.BatchRequest) bool {
	return scanned < request.Limit || next >= request.UpperBound
}

type markedAnswerSheet struct {
	DomainID          uint64 `bson:"domain_id"`
	DurableAcceptance struct {
		EventID string `bson:"event_id"`
	} `bson:"durable_acceptance"`
}

func (s *Scanner) scanAnswerSheetOutbox(ctx context.Context, request appaudit.BatchRequest) (appaudit.BatchResult, error) {
	filter := boundedFilter(bson.M{"durable_acceptance.schema_version": 1, "deleted_at": nil}, request)
	var sheets []markedAnswerSheet
	if err := findAll(ctx, s.db.Collection("answersheets"), filter, findOptions(request, bson.M{"domain_id": 1, "durable_acceptance.event_id": 1}), &sheets); err != nil {
		return appaudit.BatchResult{}, err
	}
	result := appaudit.BatchResult{Scanned: len(sheets)}
	if len(sheets) == 0 {
		result.Exhausted = true
		return result, nil
	}
	eventIDs := make([]string, 0, len(sheets))
	for _, sheet := range sheets {
		eventIDs = append(eventIDs, sheet.DurableAcceptance.EventID)
		result.NextID = sheet.DomainID
	}
	var events []struct {
		EventID       string `bson:"event_id"`
		EventType     string `bson:"event_type"`
		AggregateType string `bson:"aggregate_type"`
		AggregateID   string `bson:"aggregate_id"`
	}
	if err := findAll(ctx, s.db.Collection("domain_event_outbox"), bson.M{"event_id": bson.M{"$in": eventIDs}}, options.Find().SetProjection(bson.M{"event_id": 1, "event_type": 1, "aggregate_type": 1, "aggregate_id": 1}).SetMaxTime(request.MaxTime), &events); err != nil {
		return appaudit.BatchResult{}, err
	}
	byEvent := make(map[string]struct {
		eventType, aggregateType, aggregateID string
	}, len(events))
	for _, item := range events {
		byEvent[item.EventID] = struct{ eventType, aggregateType, aggregateID string }{item.EventType, item.AggregateType, item.AggregateID}
	}
	for _, sheet := range sheets {
		event, ok := byEvent[sheet.DurableAcceptance.EventID]
		if !ok || event.eventType != answersheetdomain.EventTypeSubmitted || event.aggregateType != answersheetdomain.AggregateType || event.aggregateID != strconv.FormatUint(sheet.DomainID, 10) {
			result.Findings = append(result.Findings, finding(appaudit.DriftAnswerSheetMissingOutbox, sheet.DomainID))
		}
	}
	result.Exhausted = batchDone(result.Scanned, result.NextID, request)
	return result, nil
}

type submittedOutbox struct {
	EventID     string `bson:"event_id"`
	AggregateID string `bson:"aggregate_id"`
}

const outboxConsistencyAuditIndex = "idx_outbox_consistency_audit"

func outboxBaseFilter() bson.M {
	return bson.M{
		"event_type":     answersheetdomain.EventTypeSubmitted,
		"aggregate_type": answersheetdomain.AggregateType,
	}
}

func parseOutboxAggregateID(value string) (uint64, error) {
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("invalid AnswerSheet outbox aggregate_id %q", value)
	}
	return id, nil
}

func (s *Scanner) outboxUpperBound(ctx context.Context, maxTime time.Duration) (uint64, error) {
	var row submittedOutbox
	err := s.db.Collection("domain_event_outbox").FindOne(
		ctx,
		outboxBaseFilter(),
		options.FindOne().
			SetSort(bson.D{{Key: "aggregate_id", Value: -1}}).
			SetProjection(bson.M{"aggregate_id": 1}).
			SetHint(outboxConsistencyAuditIndex).
			SetMaxTime(maxTime),
	).Decode(&row)
	if err == mongo.ErrNoDocuments {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return parseOutboxAggregateID(row.AggregateID)
}

func (s *Scanner) scanOutboxAnswerSheet(ctx context.Context, request appaudit.BatchRequest) (appaudit.BatchResult, error) {
	var rows []submittedOutbox
	filter := outboxBaseFilter()
	filter["aggregate_id"] = bson.M{
		"$gt":  strconv.FormatUint(request.AfterID, 10),
		"$lte": strconv.FormatUint(request.UpperBound, 10),
	}
	if err := findAll(
		ctx,
		s.db.Collection("domain_event_outbox"),
		filter,
		options.Find().
			SetSort(bson.D{{Key: "aggregate_id", Value: 1}}).
			SetLimit(int64(request.Limit)).
			SetProjection(bson.M{"event_id": 1, "aggregate_id": 1}).
			SetHint(outboxConsistencyAuditIndex).
			SetMaxTime(request.MaxTime),
		&rows,
	); err != nil {
		return appaudit.BatchResult{}, err
	}
	result := appaudit.BatchResult{Scanned: len(rows)}
	if len(rows) == 0 {
		result.Exhausted = true
		return result, nil
	}
	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		auditID, err := parseOutboxAggregateID(row.AggregateID)
		if err != nil {
			return appaudit.BatchResult{}, err
		}
		ids = append(ids, auditID)
		result.NextID = auditID
	}
	var sheets []struct {
		DomainID uint64 `bson:"domain_id"`
	}
	if err := findAll(ctx, s.db.Collection("answersheets"), bson.M{"domain_id": bson.M{"$in": ids}, "deleted_at": nil}, options.Find().SetProjection(bson.M{"domain_id": 1}).SetMaxTime(request.MaxTime), &sheets); err != nil {
		return appaudit.BatchResult{}, err
	}
	found := make(map[uint64]struct{}, len(sheets))
	for _, sheet := range sheets {
		found[sheet.DomainID] = struct{}{}
	}
	for index := range rows {
		if _, ok := found[ids[index]]; !ok {
			result.Findings = append(result.Findings, finding(appaudit.DriftOutboxMissingAnswerSheet, ids[index]))
		}
	}
	// aggregate_id is stored and indexed as a decimal string. Cursor equality,
	// rather than numeric >=, preserves correct exhaustion when lexicographic
	// order crosses digit widths (for example 1, 10, 100, 2).
	result.Exhausted = result.Scanned < request.Limit || result.NextID == request.UpperBound
	return result, nil
}

type generationDoc struct {
	DomainID  uint64 `bson:"domain_id"`
	Status    string `bson:"status"`
	LatestRun uint64 `bson:"latest_run_id"`
	ReportID  uint64 `bson:"report_id"`
}

type runDoc struct {
	DomainID      uint64     `bson:"domain_id"`
	GenerationID  uint64     `bson:"generation_id"`
	Status        string     `bson:"status"`
	RetryEventID  string     `bson:"retry_event_id"`
	NextAttemptAt *time.Time `bson:"next_attempt_at"`
}

func (s *Scanner) generationBatch(ctx context.Context, request appaudit.BatchRequest, status string) ([]generationDoc, error) {
	base := bson.M{"transaction_schema_version": 1, "deleted_at": nil}
	if status != "" {
		base["status"] = status
	}
	var rows []generationDoc
	err := findAll(ctx, s.db.Collection("report_generations"), boundedFilter(base, request), findOptions(request, bson.M{"domain_id": 1, "status": 1, "latest_run_id": 1, "report_id": 1}), &rows)
	return rows, err
}

func (s *Scanner) scanGenerationRun(ctx context.Context, request appaudit.BatchRequest) (appaudit.BatchResult, error) {
	rows, err := s.generationBatch(ctx, request, "")
	if err != nil {
		return appaudit.BatchResult{}, err
	}
	result := appaudit.BatchResult{Scanned: len(rows)}
	if len(rows) == 0 {
		result.Exhausted = true
		return result, nil
	}
	runIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		result.NextID = row.DomainID
		if row.LatestRun != 0 {
			runIDs = append(runIDs, row.LatestRun)
		}
	}
	var runs []runDoc
	if len(runIDs) > 0 {
		if err := findAll(ctx, s.db.Collection("interpretation_runs"), bson.M{"domain_id": bson.M{"$in": runIDs}, "deleted_at": nil}, options.Find().SetProjection(bson.M{"domain_id": 1, "generation_id": 1, "status": 1}).SetMaxTime(request.MaxTime), &runs); err != nil {
			return appaudit.BatchResult{}, err
		}
	}
	byID := make(map[uint64]runDoc, len(runs))
	for _, run := range runs {
		byID[run.DomainID] = run
	}
	for _, generation := range rows {
		if generation.Status == "pending" && generation.LatestRun == 0 {
			continue
		}
		run, ok := byID[generation.LatestRun]
		if !ok {
			result.Findings = append(result.Findings, finding(appaudit.DriftGenerationMissingRun, generation.DomainID))
			continue
		}
		wantStatus := map[string]string{"generating": "running", "generated": "succeeded", "failed": "failed"}[generation.Status]
		if run.GenerationID != generation.DomainID || wantStatus == "" || run.Status != wantStatus {
			result.Findings = append(result.Findings, finding(appaudit.DriftGenerationRunStateMismatch, generation.DomainID))
		}
	}
	result.Exhausted = batchDone(result.Scanned, result.NextID, request)
	return result, nil
}

func (s *Scanner) scanGeneratedTerminal(ctx context.Context, request appaudit.BatchRequest) (appaudit.BatchResult, error) {
	rows, err := s.generationBatch(ctx, request, "generated")
	if err != nil {
		return appaudit.BatchResult{}, err
	}
	result := appaudit.BatchResult{Scanned: len(rows)}
	if len(rows) == 0 {
		result.Exhausted = true
		return result, nil
	}
	reportIDs := make([]uint64, 0, len(rows))
	generationIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		result.NextID = row.DomainID
		reportIDs = append(reportIDs, row.ReportID)
		generationIDs = append(generationIDs, strconv.FormatUint(row.DomainID, 10))
	}
	var artifacts []struct {
		DomainID     uint64 `bson:"domain_id"`
		GenerationID uint64 `bson:"generation_id"`
	}
	if err := findAll(ctx, s.db.Collection("interpret_report_artifacts"), bson.M{"domain_id": bson.M{"$in": reportIDs}, "deleted_at": nil}, options.Find().SetProjection(bson.M{"domain_id": 1, "generation_id": 1}).SetMaxTime(request.MaxTime), &artifacts); err != nil {
		return appaudit.BatchResult{}, err
	}
	artifactByID := make(map[uint64]uint64, len(artifacts))
	for _, artifact := range artifacts {
		artifactByID[artifact.DomainID] = artifact.GenerationID
	}
	var events []struct {
		AggregateID string `bson:"aggregate_id"`
	}
	if err := findAll(ctx, s.db.Collection("domain_event_outbox"), bson.M{"aggregate_type": domaininterpretation.AggregateType, "aggregate_id": bson.M{"$in": generationIDs}, "event_type": domaininterpretation.EventTypeReportGenerated}, options.Find().SetProjection(bson.M{"aggregate_id": 1}).SetMaxTime(request.MaxTime), &events); err != nil {
		return appaudit.BatchResult{}, err
	}
	eventByGeneration := make(map[string]struct{}, len(events))
	for _, event := range events {
		eventByGeneration[event.AggregateID] = struct{}{}
	}
	for _, generation := range rows {
		if owner, ok := artifactByID[generation.ReportID]; !ok || owner != generation.DomainID {
			result.Findings = append(result.Findings, finding(appaudit.DriftGeneratedMissingArtifact, generation.DomainID))
		}
		if _, ok := eventByGeneration[strconv.FormatUint(generation.DomainID, 10)]; !ok {
			result.Findings = append(result.Findings, finding(appaudit.DriftGeneratedMissingTerminalOutbox, generation.DomainID))
		}
	}
	result.Exhausted = batchDone(result.Scanned, result.NextID, request)
	return result, nil
}

func (s *Scanner) scanRetryOutbox(ctx context.Context, request appaudit.BatchRequest) (appaudit.BatchResult, error) {
	base := bson.M{"retry_event_id": bson.M{"$type": "string", "$ne": ""}, "deleted_at": nil}
	var rows []runDoc
	if err := findAll(ctx, s.db.Collection("interpretation_runs"), boundedFilter(base, request), findOptions(request, bson.M{"domain_id": 1, "generation_id": 1, "retry_event_id": 1, "next_attempt_at": 1}), &rows); err != nil {
		return appaudit.BatchResult{}, err
	}
	result := appaudit.BatchResult{Scanned: len(rows)}
	if len(rows) == 0 {
		result.Exhausted = true
		return result, nil
	}
	eventIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		result.NextID = row.DomainID
		eventIDs = append(eventIDs, row.RetryEventID)
	}
	var events []struct {
		EventID       string    `bson:"event_id"`
		AggregateType string    `bson:"aggregate_type"`
		AggregateID   string    `bson:"aggregate_id"`
		NextAttemptAt time.Time `bson:"next_attempt_at"`
	}
	if err := findAll(ctx, s.db.Collection("domain_event_outbox"), bson.M{"event_id": bson.M{"$in": eventIDs}, "event_type": domaininterpretation.EventTypeRetryRequested}, options.Find().SetProjection(bson.M{"event_id": 1, "aggregate_type": 1, "aggregate_id": 1, "next_attempt_at": 1}).SetMaxTime(request.MaxTime), &events); err != nil {
		return appaudit.BatchResult{}, err
	}
	found := make(map[string]struct {
		aggregateType string
		aggregateID   string
		nextAttemptAt time.Time
	}, len(events))
	for _, event := range events {
		found[event.EventID] = struct {
			aggregateType string
			aggregateID   string
			nextAttemptAt time.Time
		}{event.AggregateType, event.AggregateID, event.NextAttemptAt}
	}
	for _, row := range rows {
		event, ok := found[row.RetryEventID]
		dueMatches := row.NextAttemptAt != nil && !event.nextAttemptAt.IsZero() && row.NextAttemptAt.Equal(event.nextAttemptAt)
		if !ok || event.aggregateType != domaininterpretation.AggregateType || event.aggregateID != strconv.FormatUint(row.GenerationID, 10) || !dueMatches {
			result.Findings = append(result.Findings, finding(appaudit.DriftRetryMissingScheduledOutbox, row.DomainID))
		}
	}
	result.Exhausted = batchDone(result.Scanned, result.NextID, request)
	return result, nil
}

func (s *Scanner) scanModelRelease(ctx context.Context, request appaudit.BatchRequest) (appaudit.BatchResult, error) {
	base := bson.M{"record_role": "head", "status": "published", "deleted_at": nil}
	var heads []modelmongo.PublishedAssessmentModelPO
	if err := findAll(ctx, s.db.Collection("assessment_models"), boundedFilter(base, request), findOptions(request, bson.M{"domain_id": 1, "kind": 1, "algorithm": 1, "code": 1, "status": 1, "questionnaire_code": 1, "questionnaire_version": 1}), &heads); err != nil {
		return appaudit.BatchResult{}, err
	}
	result := appaudit.BatchResult{Scanned: len(heads)}
	if len(heads) == 0 {
		result.Exhausted = true
		return result, nil
	}
	codes := make([]string, 0, len(heads))
	for _, head := range heads {
		result.NextID = head.DomainID.Uint64()
		codes = append(codes, head.Code)
	}
	var snapshots []modelmongo.PublishedAssessmentModelPO
	if err := findAll(ctx, s.db.Collection("assessment_models"), bson.M{"record_role": "published_snapshot", "release_status": "active", "status": "published", "code": bson.M{"$in": codes}, "deleted_at": nil}, options.Find().SetMaxTime(request.MaxTime), &snapshots); err != nil {
		return appaudit.BatchResult{}, err
	}
	snapshotsByCode := make(map[string][]modelmongo.PublishedAssessmentModelPO, len(snapshots))
	questionnaireKeys := make([]bson.M, 0, len(snapshots))
	for _, snapshot := range snapshots {
		snapshotsByCode[snapshot.Code] = append(snapshotsByCode[snapshot.Code], snapshot)
		questionnaireKeys = append(questionnaireKeys, bson.M{"code": snapshot.QuestionnaireCode, "version": snapshot.QuestionnaireVersion})
	}
	var questionnaireSnapshots []struct {
		Code    string `bson:"code"`
		Version string `bson:"version"`
	}
	if len(questionnaireKeys) > 0 {
		if err := findAll(ctx, s.db.Collection("questionnaires"), bson.M{"record_role": "published_snapshot", "release_status": "active", "deleted_at": nil, "$or": questionnaireKeys}, options.Find().SetProjection(bson.M{"code": 1, "version": 1}).SetMaxTime(request.MaxTime), &questionnaireSnapshots); err != nil {
			return appaudit.BatchResult{}, err
		}
	}
	questionnaires := make(map[string]struct{}, len(questionnaireSnapshots))
	for _, questionnaire := range questionnaireSnapshots {
		questionnaires[questionnaire.Code+"@"+questionnaire.Version] = struct{}{}
	}
	for _, head := range heads {
		matches := snapshotsByCode[head.Code]
		if len(matches) != 1 {
			result.Findings = append(result.Findings, finding(appaudit.DriftModelActiveSnapshotMismatch, head.DomainID.Uint64()))
			continue
		}
		snapshot := matches[0]
		if head.Kind != snapshot.Kind || head.Algorithm != snapshot.Algorithm || head.QuestionnaireCode != snapshot.QuestionnaireCode || head.QuestionnaireVersion != snapshot.QuestionnaireVersion {
			result.Findings = append(result.Findings, finding(appaudit.DriftModelBindingMismatch, head.DomainID.Uint64()))
		}
		if _, ok := questionnaires[snapshot.QuestionnaireCode+"@"+snapshot.QuestionnaireVersion]; !ok {
			result.Findings = append(result.Findings, finding(appaudit.DriftModelQuestionnaireMissing, head.DomainID.Uint64()))
		}
	}
	result.Exhausted = batchDone(result.Scanned, result.NextID, request)
	return result, nil
}

func (s *Scanner) scanPublishedModelRuntime(ctx context.Context, request appaudit.BatchRequest) (appaudit.BatchResult, error) {
	base := bson.M{"record_role": "head", "status": "published", "deleted_at": nil}
	var heads []modelmongo.PublishedAssessmentModelPO
	if err := findAll(ctx, s.db.Collection("assessment_models"), boundedFilter(base, request), findOptions(request, bson.M{"domain_id": 1, "code": 1}), &heads); err != nil {
		return appaudit.BatchResult{}, err
	}
	result := appaudit.BatchResult{Scanned: len(heads)}
	if len(heads) == 0 {
		result.Exhausted = true
		return result, nil
	}
	codes := make([]string, 0, len(heads))
	sampleByCode := make(map[string]uint64, len(heads))
	for _, head := range heads {
		sampleID := head.DomainID.Uint64()
		result.NextID = sampleID
		codes = append(codes, head.Code)
		sampleByCode[head.Code] = sampleID
	}
	var snapshots []modelmongo.PublishedAssessmentModelPO
	if err := findAll(ctx, s.db.Collection("assessment_models"), bson.M{
		"record_role": "published_snapshot", "release_status": "active", "status": "published",
		"code": bson.M{"$in": codes}, "deleted_at": nil,
	}, options.Find().SetMaxTime(request.MaxTime), &snapshots); err != nil {
		return appaudit.BatchResult{}, err
	}
	normVersions := make(map[string]struct{})
	for _, snapshot := range snapshots {
		if snapshot.DefinitionV2 != nil {
			for _, ref := range snapshot.DefinitionV2.Calibration.NormRefs {
				if ref.NormTableVersion != "" {
					normVersions[ref.NormTableVersion] = struct{}{}
				}
			}
		}
	}
	versions := make([]string, 0, len(normVersions))
	for version := range normVersions {
		versions = append(versions, version)
	}
	var norms []struct {
		TableVersion string `bson:"table_version"`
	}
	if len(versions) > 0 {
		if err := findAll(ctx, s.db.Collection("assessment_norms"), bson.M{"table_version": bson.M{"$in": versions}, "deleted_at": nil}, options.Find().SetProjection(bson.M{"table_version": 1}).SetMaxTime(request.MaxTime), &norms); err != nil {
			return appaudit.BatchResult{}, err
		}
	}
	foundNorms := make(map[string]struct{}, len(norms))
	for _, norm := range norms {
		foundNorms[norm.TableVersion] = struct{}{}
	}
	mapper := modelmongo.NewMapper()
	for _, po := range snapshots {
		sampleID := sampleByCode[po.Code]
		model := mapper.ToPublished(&po)
		invalid := model == nil || model.SchemaVersion != modeldomain.SchemaVersionV2 || model.DefinitionV2 == nil || model.DecisionKind == ""
		if !invalid {
			invalid = len(modeldefinition.Validate(*model.DefinitionV2)) > 0
			_, familyOK := modeldomain.AlgorithmFamilyFromDecisionKind(model.DecisionKind)
			_, identityOK := modeldomain.AlgorithmFamilyFromIdentity(model.Kind, model.SubKind, model.Algorithm)
			invalid = invalid || !familyOK || !identityOK
		}
		if invalid {
			result.Findings = append(result.Findings, finding(appaudit.DriftModelRuntimeInvalid, sampleID))
		}
		if model != nil && model.DefinitionV2 != nil {
			hash, err := modeldefinition.CanonicalContentHash(model.DefinitionV2)
			stored := modelport.DefinitionHashFromSource(model.Source)
			if err != nil || stored == "" || stored != hash {
				result.Findings = append(result.Findings, finding(appaudit.DriftModelDefinitionHashMismatch, sampleID))
			}
			for _, ref := range model.DefinitionV2.Calibration.NormRefs {
				if _, ok := foundNorms[ref.NormTableVersion]; !ok {
					result.Findings = append(result.Findings, finding(appaudit.DriftModelNormMissing, sampleID))
					break
				}
			}
		}
	}
	result.Exhausted = batchDone(result.Scanned, result.NextID, request)
	return result, nil
}

func findAll(ctx context.Context, collection *mongo.Collection, filter bson.M, opts *options.FindOptions, output interface{}) error {
	cur, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return err
	}
	defer func() { _ = cur.Close(ctx) }()
	return cur.All(ctx, output)
}

func finding(kind string, id uint64) appaudit.Finding {
	return appaudit.Finding{Kind: kind, Severity: appaudit.DriftSeverities[kind], SampleID: strconv.FormatUint(id, 10)}
}
