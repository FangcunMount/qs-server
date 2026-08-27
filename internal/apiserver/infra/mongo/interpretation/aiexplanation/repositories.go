package aiexplanation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	appsubjectexport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/subjectexport"
	domainartifact "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/artifact"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

type GenerationRepository struct {
	base.BaseRepository
	mapper    *Mapper
	retention RetentionPolicy
}

func NewGenerationRepository(db *mongo.Database, retention RetentionPolicy, opts ...base.BaseRepositoryOptions) (*GenerationRepository, error) {
	if err := retention.Validate(); err != nil {
		return nil, err
	}
	repository := &GenerationRepository{BaseRepository: base.NewBaseRepository(db, (GenerationPO{}).CollectionName(), opts...), mapper: NewMapper(), retention: retention}
	_, err := repository.Collection().Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "domain_id", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_generation_domain").SetUnique(true)},
		{Keys: bson.D{
			{Key: "source_report_id", Value: 1}, {Key: "audience", Value: 1}, {Key: "profile.id", Value: 1}, {Key: "profile.version", Value: 1},
			{Key: "profile.fingerprint", Value: 1}, {Key: "input_fingerprint", Value: 1}, {Key: "execution_spec_fingerprint", Value: 1},
		}, Options: options.Index().SetName("uk_ai_explanation_generation_semantic_key").SetUnique(true)},
		{Keys: bson.D{{Key: "association.assessment_id", Value: 1}, {Key: "created_at", Value: -1}}, Options: options.Index().SetName("idx_ai_explanation_generation_assessment_created")},
		ttlIndex(),
	})
	if err != nil {
		return nil, fmt.Errorf("create AI explanation generation indexes: %w", err)
	}
	if retention.Enabled() {
		if err := rejectMissingTerminalExpiration(context.Background(), repository.Collection(), bson.M{"status": bson.M{"$in": bson.A{string(domaingeneration.StatusGenerated), string(domaingeneration.StatusFailed)}}}); err != nil {
			return nil, fmt.Errorf("verify AI explanation generation retention boundary: %w", err)
		}
	}
	return repository, nil
}

var _ domaingeneration.Repository = (*GenerationRepository)(nil)

func (r *GenerationRepository) Create(ctx context.Context, value *domaingeneration.AIExplanationGeneration) error {
	po, err := r.mapper.GenerationToPO(value)
	if err != nil {
		return err
	}
	if _, err := r.InsertOne(ctx, po); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("create AI explanation generation: %w", domaingeneration.ErrAlreadyExists)
		}
		return fmt.Errorf("create AI explanation generation: %w", err)
	}
	return nil
}

func (r *GenerationRepository) FindByID(ctx context.Context, id meta.ID) (*domaingeneration.AIExplanationGeneration, error) {
	var po GenerationPO
	if err := r.FindOne(ctx, bson.M{"domain_id": id}, &po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domaingeneration.ErrNotFound
		}
		return nil, fmt.Errorf("find AI explanation generation: %w", err)
	}
	return r.mapper.GenerationToDomain(&po)
}

func (r *GenerationRepository) FindByKey(ctx context.Context, key domaingeneration.Key) (*domaingeneration.AIExplanationGeneration, error) {
	var po GenerationPO
	filter := bson.M{
		"source_report_id": key.SourceReportID.Uint64(), "audience": string(key.Audience), "profile.id": key.Profile.ID,
		"profile.version": key.Profile.Version, "profile.fingerprint": key.Profile.Fingerprint.String(),
		"input_fingerprint": key.InputFingerprint.String(), "execution_spec_fingerprint": key.ExecutionSpecFingerprint.String(),
	}
	if err := r.FindOne(ctx, filter, &po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domaingeneration.ErrNotFound
		}
		return nil, fmt.Errorf("find AI explanation generation by key: %w", err)
	}
	return r.mapper.GenerationToDomain(&po)
}

func (r *GenerationRepository) Save(ctx context.Context, value *domaingeneration.AIExplanationGeneration, expectedVersion uint64) error {
	po, err := r.mapper.GenerationToPO(value)
	if err != nil {
		return err
	}
	if expectedVersion == 0 || po.Version <= expectedVersion {
		return domaingeneration.ErrConflict
	}
	setFields := bson.M{
		"status": po.Status, "latest_run_id": po.LatestRunID, "artifact_id": po.ArtifactID, "version": po.Version, "updated_at": po.UpdatedAt,
	}
	update := bson.M{"$set": setFields}
	if value.Status() == domaingeneration.StatusGenerated || value.Status() == domaingeneration.StatusFailed {
		expiresAt, expiryErr := expiresAfter(value.UpdatedAt(), r.retention.ParticipantRecordRetention)
		if expiryErr != nil {
			return expiryErr
		}
		setFields["expires_at"] = expiresAt
		setFields["retention_policy_version"] = strings.TrimSpace(r.retention.Version)
	} else {
		update["$unset"] = bson.M{"expires_at": "", "retention_policy_version": ""}
	}
	result, err := r.UpdateOne(ctx, bson.M{"domain_id": po.DomainID, "version": expectedVersion}, update)
	if err != nil {
		return fmt.Errorf("save AI explanation generation: %w", err)
	}
	if result.MatchedCount != 1 {
		return domaingeneration.ErrConflict
	}
	return nil
}

type RunRepository struct {
	base.BaseRepository
	mapper    *Mapper
	retention RetentionPolicy
}

func NewRunRepository(db *mongo.Database, retention RetentionPolicy, opts ...base.BaseRepositoryOptions) (*RunRepository, error) {
	if err := retention.Validate(); err != nil {
		return nil, err
	}
	repository := &RunRepository{BaseRepository: base.NewBaseRepository(db, (RunPO{}).CollectionName(), opts...), mapper: NewMapper(), retention: retention}
	_, err := repository.Collection().Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "domain_id", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_run_domain").SetUnique(true)},
		{Keys: bson.D{{Key: "generation_id", Value: 1}, {Key: "attempt", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_run_attempt").SetUnique(true)},
		{Keys: bson.D{{Key: "generation_id", Value: 1}, {Key: "attempt", Value: -1}}, Options: options.Index().SetName("idx_ai_explanation_run_latest")},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "lease_expires_at", Value: 1}, {Key: "domain_id", Value: 1}}, Options: options.Index().SetName("idx_ai_explanation_run_expired_lease")},
		{Keys: bson.D{{Key: "retry_authorization.request_id", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_run_retry_request").SetUnique(true).SetSparse(true)},
		{Keys: bson.D{{Key: "retry_authorization.event_id", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_run_retry_event").SetUnique(true).SetSparse(true)},
		ttlIndex(),
	})
	if err != nil {
		return nil, fmt.Errorf("create AI explanation run indexes: %w", err)
	}
	if retention.Enabled() {
		if err := rejectMissingTerminalExpiration(context.Background(), repository.Collection(), bson.M{"status": bson.M{"$in": bson.A{string(domainrun.StatusSucceeded), string(domainrun.StatusFailed)}}}); err != nil {
			return nil, fmt.Errorf("verify AI explanation run retention boundary: %w", err)
		}
	}
	return repository, nil
}

var _ domainrun.Repository = (*RunRepository)(nil)
var _ domainrun.ExpiredLeaseReader = (*RunRepository)(nil)
var _ domainrun.LeaseReclaimer = (*RunRepository)(nil)
var _ domainrun.RetryAuthorizer = (*RunRepository)(nil)
var _ domainrun.RecoveryWakeupScheduler = (*RunRepository)(nil)

func (r *RunRepository) Create(ctx context.Context, value *domainrun.AIExplanationRun) error {
	po, err := r.mapper.RunToPO(value)
	if err != nil {
		return err
	}
	now := time.Now()
	po.CreatedAt, po.UpdatedAt = now, now
	if _, err := r.InsertOne(ctx, po); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("create AI explanation run: %w", domainrun.ErrAlreadyExists)
		}
		return fmt.Errorf("create AI explanation run: %w", err)
	}
	return nil
}

func (r *RunRepository) FindByID(ctx context.Context, id meta.ID) (*domainrun.AIExplanationRun, error) {
	var po RunPO
	if err := r.FindOne(ctx, bson.M{"domain_id": id}, &po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domainrun.ErrNotFound
		}
		return nil, fmt.Errorf("find AI explanation run: %w", err)
	}
	return r.mapper.RunToDomain(&po)
}

func (r *RunRepository) FindLatestByGenerationID(ctx context.Context, generationID meta.ID) (*domainrun.AIExplanationRun, error) {
	var po RunPO
	if err := r.FindOne(ctx, bson.M{"generation_id": generationID.Uint64()}, &po, options.FindOne().SetSort(bson.D{{Key: "attempt", Value: -1}})); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domainrun.ErrNotFound
		}
		return nil, fmt.Errorf("find latest AI explanation run: %w", err)
	}
	return r.mapper.RunToDomain(&po)
}

func (r *RunRepository) Save(ctx context.Context, value *domainrun.AIExplanationRun) error {
	po, err := r.mapper.RunToPO(value)
	if err != nil {
		return err
	}
	filter := bson.M{"domain_id": po.DomainID}
	if value.Status() == domainrun.StatusRunning && value.InvocationPhase() == domainrun.InvocationPhaseDispatching {
		filter["status"] = string(domainrun.StatusRunning)
		filter["invocation_phase"] = string(domainrun.InvocationPhasePrepared)
	} else if value.Status() == domainrun.StatusSucceeded || value.Status() == domainrun.StatusFailed {
		filter["status"] = string(domainrun.StatusRunning)
	}
	setFields := bson.M{
		"status": po.Status, "failure": po.Failure, "trace_id": po.TraceID, "started_at": po.StartedAt,
		"lease_expires_at": po.LeaseExpiresAt, "finished_at": po.FinishedAt, "origin": po.Origin,
		"invocation_id": po.InvocationID, "invocation_phase": po.InvocationPhase, "dispatch_started_at": po.DispatchStartedAt,
		"receipt": po.Receipt, "claim_history": po.ClaimHistory, "recovery_count": po.RecoveryCount,
		"last_reclaimed_at": po.LastReclaimedAt, "recovery_wakeup": po.RecoveryWakeup, "updated_at": time.Now(),
	}
	if value.Status() == domainrun.StatusSucceeded || value.Status() == domainrun.StatusFailed {
		finishedAt := value.FinishedAt()
		if finishedAt == nil {
			return fmt.Errorf("terminal AI explanation Run has no finished time")
		}
		expiresAt, expiryErr := expiresAfter(*finishedAt, r.retention.ParticipantRecordRetention)
		if expiryErr != nil {
			return expiryErr
		}
		setFields["expires_at"] = expiresAt
		setFields["retention_policy_version"] = strings.TrimSpace(r.retention.Version)
	}
	result, err := r.UpdateOne(ctx, filter, bson.M{"$set": setFields})
	if err != nil {
		return fmt.Errorf("save AI explanation run: %w", err)
	}
	if result.MatchedCount != 1 {
		return domainrun.ErrConflict
	}
	return nil
}

func (r *RunRepository) ScheduleRecoveryWakeup(ctx context.Context, id meta.ID, wakeup domainrun.RecoveryWakeup) (*domainrun.AIExplanationRun, bool, error) {
	if r == nil || id.IsZero() {
		return nil, false, fmt.Errorf("schedule AI explanation lease recovery: repository and Run are required")
	}
	if err := wakeup.Validate(); err != nil {
		return nil, false, err
	}
	baseFilter := bson.M{
		"domain_id": id.Uint64(), "status": string(domainrun.StatusRunning),
		"lease_expires_at": wakeup.ExpectedLeaseExpiresAt,
		"invocation_phase": string(wakeup.InvocationPhase),
	}
	var current RunPO
	if err := r.FindOne(ctx, baseFilter, &current); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, false, domainrun.ErrRecoveryNotAllowed
		}
		return nil, false, fmt.Errorf("load AI explanation Run for lease recovery: %w", err)
	}
	currentDomain, err := r.mapper.RunToDomain(&current)
	if err != nil {
		return nil, false, err
	}
	created, err := currentDomain.ScheduleRecoveryWakeup(wakeup)
	if err != nil || !created {
		return currentDomain, false, err
	}
	updated, err := r.mapper.RunToPO(currentDomain)
	if err != nil {
		return nil, false, err
	}
	filter := baseFilter
	filter["recovery_wakeup"] = bson.M{"$exists": false}
	result, err := r.UpdateOne(ctx, filter, bson.M{"$set": bson.M{
		"recovery_wakeup": updated.RecoveryWakeup, "updated_at": wakeup.RequestedAt.UTC(),
	}})
	if err != nil {
		return nil, false, fmt.Errorf("schedule AI explanation lease recovery: %w", err)
	}
	if result.ModifiedCount == 1 {
		return currentDomain, true, nil
	}
	var winner RunPO
	if err := r.FindOne(ctx, bson.M{"domain_id": id.Uint64()}, &winner); err != nil {
		return nil, false, fmt.Errorf("load concurrent AI explanation lease recovery: %w", err)
	}
	winnerDomain, err := r.mapper.RunToDomain(&winner)
	if err != nil {
		return nil, false, err
	}
	if existing := winnerDomain.RecoveryWakeup(); existing != nil && existing.Same(wakeup) {
		return winnerDomain, false, nil
	}
	return nil, false, domainrun.ErrConflict
}

func (r *RunRepository) AuthorizeRetry(ctx context.Context, generationID meta.ID, authorization domainrun.RetryAuthorization) (*domainrun.AIExplanationRun, bool, error) {
	if r == nil || generationID.IsZero() {
		return nil, false, fmt.Errorf("authorize AI explanation retry: repository and Generation are required")
	}
	if err := authorization.Validate(); err != nil {
		return nil, false, err
	}
	filter := bson.M{
		"generation_id": generationID.Uint64(), "attempt": authorization.ExpectedAttempt,
		"status": string(domainrun.StatusFailed), "retry_authorization": bson.M{"$exists": false},
	}
	var current RunPO
	if err := r.FindOne(ctx, bson.M{
		"generation_id": generationID.Uint64(), "attempt": authorization.ExpectedAttempt,
		"status": string(domainrun.StatusFailed),
	}, &current); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, false, domainrun.ErrNotFound
		}
		return nil, false, fmt.Errorf("load failed AI explanation Run for retry authorization: %w", err)
	}
	currentDomain, err := r.mapper.RunToDomain(&current)
	if err != nil {
		return nil, false, err
	}
	if existing := currentDomain.RetryAuthorization(); existing != nil {
		if existing.SameAction(authorization) {
			return currentDomain, false, nil
		}
		return nil, false, domainrun.ErrConflict
	}
	if err := currentDomain.AuthorizeManualRetry(authorization); err != nil {
		return nil, false, err
	}
	updated, err := r.mapper.RunToPO(currentDomain)
	if err != nil {
		return nil, false, err
	}
	result, err := r.UpdateOne(ctx, filter, bson.M{
		"$set": bson.M{"retry_authorization": updated.RetryAuthorization, "updated_at": authorization.AuthorizedAt.UTC()},
	})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, false, domainrun.ErrConflict
		}
		return nil, false, fmt.Errorf("authorize AI explanation retry: %w", err)
	}
	if result.ModifiedCount == 1 {
		return currentDomain, true, nil
	}
	var winner RunPO
	if err := r.FindOne(ctx, bson.M{
		"generation_id": generationID.Uint64(), "attempt": authorization.ExpectedAttempt,
		"status": string(domainrun.StatusFailed),
	}, &winner); err != nil {
		return nil, false, fmt.Errorf("load concurrent AI explanation retry authorization: %w", err)
	}
	winnerDomain, err := r.mapper.RunToDomain(&winner)
	if err != nil {
		return nil, false, err
	}
	if existing := winnerDomain.RetryAuthorization(); existing != nil && existing.SameAction(authorization) {
		return winnerDomain, false, nil
	}
	return nil, false, domainrun.ErrConflict
}

func (r *RunRepository) ListExpiredLeases(ctx context.Context, now time.Time, limit int) ([]domainrun.ExpiredLease, error) {
	if limit <= 0 {
		return nil, nil
	}
	cursor, err := r.Find(ctx, bson.M{"status": string(domainrun.StatusRunning), "lease_expires_at": bson.M{"$lte": now}}, options.Find().SetSort(bson.D{{Key: "lease_expires_at", Value: 1}, {Key: "domain_id", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	result := make([]domainrun.ExpiredLease, 0, limit)
	for cursor.Next(ctx) {
		var po RunPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		if po.LeaseExpiresAt == nil {
			return nil, fmt.Errorf("running AI explanation run %s has no lease", po.DomainID)
		}
		result = append(result, domainrun.ExpiredLease{RunID: po.DomainID, GenerationID: meta.FromUint64(po.GenerationID), LeaseExpiredAt: *po.LeaseExpiresAt, InvocationPhase: domainrun.InvocationPhase(po.InvocationPhase)})
	}
	return result, cursor.Err()
}

func (r *RunRepository) ReclaimExpiredLease(
	ctx context.Context,
	id meta.ID,
	at time.Time,
	traceID string,
	leaseUntil time.Time,
	allowIdempotentRedispatch bool,
) (*domainrun.AIExplanationRun, bool, error) {
	if id.IsZero() || at.IsZero() || !leaseUntil.After(at) {
		return nil, false, fmt.Errorf("invalid AI explanation lease reclaim")
	}
	filter := bson.M{
		"domain_id": id.Uint64(), "status": string(domainrun.StatusRunning),
		"lease_expires_at": bson.M{"$lte": at},
	}
	var current RunPO
	if err := r.FindOne(ctx, filter, &current); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("load expired AI explanation lease: %w", err)
	}
	runRecord, err := r.mapper.RunToDomain(&current)
	if err != nil {
		return nil, false, err
	}
	if err := runRecord.ReclaimExpiredLease(at, traceID, leaseUntil, allowIdempotentRedispatch); err != nil {
		return nil, false, err
	}
	updated, err := r.mapper.RunToPO(runRecord)
	if err != nil {
		return nil, false, err
	}
	filter["invocation_phase"] = current.InvocationPhase
	var claimed RunPO
	err = r.FindOneAndUpdate(ctx, filter, bson.M{"$set": bson.M{
		"trace_id": updated.TraceID, "lease_expires_at": updated.LeaseExpiresAt,
		"claim_history": updated.ClaimHistory, "recovery_count": updated.RecoveryCount,
		"last_reclaimed_at": updated.LastReclaimedAt, "updated_at": at,
	}, "$unset": bson.M{"recovery_wakeup": ""}}, &claimed, options.FindOneAndUpdate().SetReturnDocument(options.After))
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("reclaim expired AI explanation lease: %w", err)
	}
	reclaimed, err := r.mapper.RunToDomain(&claimed)
	return reclaimed, err == nil, err
}

type ArtifactRepository struct {
	base.BaseRepository
	mapper    *Mapper
	retention RetentionPolicy
}

func NewArtifactRepository(db *mongo.Database, retention RetentionPolicy, opts ...base.BaseRepositoryOptions) (*ArtifactRepository, error) {
	if err := retention.Validate(); err != nil {
		return nil, err
	}
	repository := &ArtifactRepository{BaseRepository: base.NewBaseRepository(db, (ArtifactPO{}).CollectionName(), opts...), mapper: NewMapper(), retention: retention}
	_, err := repository.Collection().Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "domain_id", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_artifact_domain").SetUnique(true)},
		{Keys: bson.D{{Key: "generation_id", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_artifact_generation").SetUnique(true)},
		{Keys: bson.D{{Key: "source.report_id", Value: 1}, {Key: "audience", Value: 1}, {Key: "generated_at", Value: -1}}, Options: options.Index().SetName("idx_ai_explanation_artifact_source_audience")},
		{Keys: bson.D{{Key: "source.association.org_id", Value: 1}, {Key: "source.association.testee_id", Value: 1}, {Key: "audience", Value: 1}, {Key: "generated_at", Value: -1}, {Key: "domain_id", Value: -1}}, Options: options.Index().SetName("idx_ai_explanation_artifact_subject_export")},
		ttlIndex(),
	})
	if err != nil {
		return nil, fmt.Errorf("create AI explanation artifact indexes: %w", err)
	}
	if retention.Enabled() {
		if err := rejectMissingTerminalExpiration(context.Background(), repository.Collection(), bson.M{"domain_id": bson.M{"$exists": true}}); err != nil {
			return nil, fmt.Errorf("verify AI explanation artifact retention boundary: %w", err)
		}
	}
	return repository, nil
}

var _ domainartifact.Repository = (*ArtifactRepository)(nil)
var _ appsubjectexport.Reader = (*ArtifactRepository)(nil)

func (r *ArtifactRepository) Insert(ctx context.Context, value *domainartifact.AIExplanationArtifact) error {
	po, err := r.mapper.ArtifactToPO(value)
	if err != nil {
		return err
	}
	expiresAt, err := expiresAfter(value.GeneratedAt(), r.retention.ParticipantRecordRetention)
	if err != nil {
		return err
	}
	po.ExpiresAt = expiresAt
	po.RetentionPolicyVersion = strings.TrimSpace(r.retention.Version)
	if _, err := r.InsertOne(ctx, po); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("insert AI explanation artifact: %w", domainartifact.ErrAlreadyExists)
		}
		return fmt.Errorf("insert AI explanation artifact: %w", err)
	}
	return nil
}

func (r *ArtifactRepository) FindByID(ctx context.Context, id meta.ID) (*domainartifact.AIExplanationArtifact, error) {
	return r.findOne(ctx, bson.M{"domain_id": id})
}

func (r *ArtifactRepository) FindByGenerationID(ctx context.Context, generationID meta.ID) (*domainartifact.AIExplanationArtifact, error) {
	return r.findOne(ctx, bson.M{"generation_id": generationID.Uint64()})
}

func (r *ArtifactRepository) FindBySourceReportAndAudience(ctx context.Context, reportID meta.ID, audience policy.Audience) (*domainartifact.AIExplanationArtifact, error) {
	return r.findOne(ctx, bson.M{"source.report_id": reportID.Uint64(), "audience": string(audience)})
}

func (r *ArtifactRepository) ListParticipantArtifacts(ctx context.Context, query appsubjectexport.ReadQuery) ([]*domainartifact.AIExplanationArtifact, error) {
	filter, findOptions, err := participantSubjectExportQuery(query)
	if err != nil {
		return nil, err
	}
	cursor, err := r.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("find participant AI explanation artifacts for export: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()
	values := make([]*domainartifact.AIExplanationArtifact, 0, query.Limit)
	for cursor.Next(ctx) {
		var po ArtifactPO
		if err := cursor.Decode(&po); err != nil {
			return nil, fmt.Errorf("decode participant AI explanation artifact for export: %w", err)
		}
		value, err := r.mapper.ArtifactToDomain(&po)
		if err != nil {
			return nil, fmt.Errorf("map participant AI explanation artifact for export: %w", err)
		}
		values = append(values, value)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("scan participant AI explanation artifacts for export: %w", err)
	}
	return values, nil
}

func participantSubjectExportQuery(query appsubjectexport.ReadQuery) (bson.M, *options.FindOptions, error) {
	if err := query.Subject.Validate(); err != nil || query.SnapshotAt.IsZero() || query.Limit < 1 || query.Limit > appsubjectexport.MaxPageSize+1 {
		return nil, nil, fmt.Errorf("participant AI explanation export query is invalid")
	}
	if query.AfterGeneratedAt.IsZero() != query.AfterArtifactID.IsZero() || (!query.AfterGeneratedAt.IsZero() && query.AfterGeneratedAt.After(query.SnapshotAt)) {
		return nil, nil, fmt.Errorf("participant AI explanation export cursor is invalid")
	}
	filter := bson.M{
		"source.association.org_id":    query.Subject.OrgID,
		"source.association.testee_id": query.Subject.TesteeID.Uint64(),
		"audience":                     string(policy.AudienceParticipant),
		"generated_at":                 bson.M{"$lte": query.SnapshotAt.UTC()},
	}
	if !query.AfterGeneratedAt.IsZero() {
		filter["$or"] = bson.A{
			bson.M{"generated_at": bson.M{"$lt": query.AfterGeneratedAt.UTC()}},
			bson.M{"generated_at": query.AfterGeneratedAt.UTC(), "domain_id": bson.M{"$lt": query.AfterArtifactID}},
		}
	}
	findOptions := options.Find().SetSort(bson.D{{Key: "generated_at", Value: -1}, {Key: "domain_id", Value: -1}}).SetLimit(int64(query.Limit))
	return filter, findOptions, nil
}

func (r *ArtifactRepository) findOne(ctx context.Context, filter bson.M) (*domainartifact.AIExplanationArtifact, error) {
	var po ArtifactPO
	if err := r.FindOne(ctx, filter, &po, options.FindOne().SetSort(bson.D{{Key: "generated_at", Value: -1}})); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domainartifact.ErrNotFound
		}
		return nil, fmt.Errorf("find AI explanation artifact: %w", err)
	}
	return r.mapper.ArtifactToDomain(&po)
}

type ProfileRepository struct {
	base.BaseRepository
	mapper *Mapper
}

func NewProfileRepository(db *mongo.Database, opts ...base.BaseRepositoryOptions) (*ProfileRepository, error) {
	repository := &ProfileRepository{BaseRepository: base.NewBaseRepository(db, (ProfilePO{}).CollectionName(), opts...), mapper: NewMapper()}
	_, err := repository.Collection().Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "domain_id", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_profile_domain").SetUnique(true)},
		{Keys: bson.D{{Key: "definition.profile_id", Value: 1}, {Key: "definition.version", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_profile_release").SetUnique(true)},
		{Keys: bson.D{{Key: "selector_slot_key", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_profile_published_selector_slot").SetUnique(true).SetPartialFilterExpression(bson.M{"status": string(domainprofile.StatusPublished)})},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "definition.selector.audience", Value: 1}, {Key: "definition.selector.model_kind", Value: 1}, {Key: "definition.selector.decision_kind", Value: 1}}, Options: options.Index().SetName("idx_ai_explanation_profile_selector")},
		{Keys: bson.D{{Key: "created_at", Value: -1}, {Key: "domain_id", Value: -1}}, Options: options.Index().SetName("idx_ai_explanation_profile_created")},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "created_at", Value: -1}, {Key: "domain_id", Value: -1}}, Options: options.Index().SetName("idx_ai_explanation_profile_status_created")},
	})
	if err != nil {
		return nil, fmt.Errorf("create AI explanation Profile indexes: %w", err)
	}
	return repository, nil
}

var _ domainprofile.Repository = (*ProfileRepository)(nil)

func (r *ProfileRepository) Save(ctx context.Context, value *domainprofile.AIExplanationProfile) error {
	po, err := r.mapper.ProfileToPO(value)
	if err != nil {
		return err
	}
	if value.Status() == domainprofile.StatusDraft {
		if _, err := r.InsertOne(ctx, po); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return domainprofile.ErrAlreadyExists
			}
			return err
		}
		return nil
	}
	previous := domainprofile.StatusDraft
	if value.Status() == domainprofile.StatusDisabled {
		previous = domainprofile.StatusPublished
	}
	result, err := r.UpdateOne(ctx, bson.M{"domain_id": po.DomainID, "fingerprint": po.Fingerprint, "status": string(previous)}, bson.M{"$set": bson.M{
		"status": po.Status, "published_at": po.PublishedAt, "published_by": po.PublishedBy, "published_reason": po.PublishedReason,
		"published_evidence_run_id": po.PublishedEvidenceRunID,
		"disabled_at":               po.DisabledAt, "disabled_by": po.DisabledBy, "disabled_reason": po.DisabledReason, "updated_at": po.UpdatedAt,
	}})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) && value.Status() == domainprofile.StatusPublished {
			return domainprofile.ErrAmbiguousSelector
		}
		return fmt.Errorf("save AI explanation Profile: %w", err)
	}
	if result.MatchedCount != 1 {
		return domainprofile.ErrConflict
	}
	return nil
}

func (r *ProfileRepository) FindByKey(ctx context.Context, profileID, version string) (*domainprofile.AIExplanationProfile, error) {
	var po ProfilePO
	if err := r.FindOne(ctx, bson.M{"definition.profile_id": profileID, "definition.version": version}, &po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domainprofile.ErrNotFound
		}
		return nil, fmt.Errorf("find AI explanation Profile: %w", err)
	}
	return r.mapper.ProfileToDomain(&po)
}

func (r *ProfileRepository) ListPublishedByBaseSelector(ctx context.Context, audience policy.Audience, kind modelcatalog.Kind, decision modelcatalog.DecisionKind) ([]*domainprofile.AIExplanationProfile, error) {
	cursor, err := r.Find(ctx, bson.M{
		"status": string(domainprofile.StatusPublished), "definition.selector.audience": string(audience),
		"definition.selector.model_kind": string(kind), "definition.selector.decision_kind": string(decision),
	}, options.Find().SetSort(bson.D{{Key: "definition.selector.model_code", Value: -1}, {Key: "definition.selector.model_version", Value: -1}, {Key: "definition.profile_id", Value: 1}, {Key: "definition.version", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	result := make([]*domainprofile.AIExplanationProfile, 0)
	for cursor.Next(ctx) {
		var po ProfilePO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		profileRecord, err := r.mapper.ProfileToDomain(&po)
		if err != nil {
			return nil, err
		}
		result = append(result, profileRecord)
	}
	return result, cursor.Err()
}

type PromptEvaluationRepository struct {
	base.BaseRepository
	mapper    *Mapper
	retention RetentionPolicy
}

func NewPromptEvaluationRepository(db *mongo.Database, retention RetentionPolicy, opts ...base.BaseRepositoryOptions) (*PromptEvaluationRepository, error) {
	if err := retention.Validate(); err != nil {
		return nil, err
	}
	repository := &PromptEvaluationRepository{
		BaseRepository: base.NewBaseRepository(db, (PromptEvaluationRunPO{}).CollectionName(), opts...), mapper: NewMapper(), retention: retention,
	}
	_, err := repository.Collection().Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "domain_id", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_prompt_evaluation_domain").SetUnique(true)},
		{Keys: bson.D{
			{Key: "release.profile.id", Value: 1}, {Key: "release.profile.version", Value: 1},
			{Key: "release.profile.fingerprint", Value: 1}, {Key: "status", Value: 1}, {Key: "finalized_at", Value: -1},
		}, Options: options.Index().SetName("idx_ai_explanation_prompt_evaluation_profile_status")},
		{Keys: bson.D{
			{Key: "release.suite.fingerprint", Value: 1},
			{Key: "release.prompt.fingerprint", Value: 1},
			{Key: "release.semantic_evaluator.prompt.fingerprint", Value: 1},
			{Key: "release.semantic_evaluator.output_schema.fingerprint", Value: 1},
			{Key: "release.semantic_evaluator.provider.fingerprint", Value: 1},
			{Key: "created_at", Value: -1},
		}, Options: options.Index().SetName("idx_ai_explanation_prompt_evaluation_release")},
		{Keys: bson.D{{Key: "active_release_key", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_prompt_evaluation_active_release").SetUnique(true).SetPartialFilterExpression(bson.M{"active_release_key": bson.M{"$type": "string"}})},
		{Keys: bson.D{{Key: "active_execution_org_key", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_prompt_evaluation_active_org_execution").SetUnique(true).SetPartialFilterExpression(bson.M{"active_execution_org_key": bson.M{"$type": "string"}})},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "execution.phase", Value: 1}, {Key: "execution.lease_expires_at", Value: 1}, {Key: "domain_id", Value: 1}}, Options: options.Index().SetName("idx_ai_explanation_prompt_evaluation_expired_lease").SetPartialFilterExpression(bson.M{"status": string(domainevaluation.StatusCollecting), "execution.phase": string(domainevaluation.AttemptExecutionPrepared), "execution.lease_expires_at": bson.M{"$type": "date"}})},
		{Keys: bson.D{{Key: "requested_org_id", Value: 1}, {Key: "created_at", Value: -1}, {Key: "domain_id", Value: -1}}, Options: options.Index().SetName("idx_ai_explanation_prompt_evaluation_org_created")},
		{Keys: bson.D{{Key: "requested_org_id", Value: 1}, {Key: "status", Value: 1}, {Key: "created_at", Value: -1}, {Key: "domain_id", Value: -1}}, Options: options.Index().SetName("idx_ai_explanation_prompt_evaluation_org_status_created")},
		ttlIndex(),
	})
	if err != nil {
		return nil, fmt.Errorf("create AI explanation Prompt evaluation indexes: %w", err)
	}
	if retention.Enabled() {
		if err := rejectMissingTerminalExpiration(context.Background(), repository.Collection(), bson.M{"status": bson.M{"$in": bson.A{string(domainevaluation.StatusApproved), string(domainevaluation.StatusRejected), string(domainevaluation.StatusCanceled)}}}); err != nil {
			return nil, fmt.Errorf("verify AI explanation Prompt evaluation retention boundary: %w", err)
		}
	}
	return repository, nil
}

var (
	_ domainevaluation.Repository               = (*PromptEvaluationRepository)(nil)
	_ domainevaluation.ExpiredPreparationReader = (*PromptEvaluationRepository)(nil)
)

func (r *PromptEvaluationRepository) Create(ctx context.Context, value *domainevaluation.PromptEvaluationRun) error {
	po, err := r.mapper.PromptEvaluationRunToPO(value)
	if err != nil {
		return err
	}
	if _, err := r.InsertOne(ctx, po); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			if isActiveOrgExecutionDuplicate(err) {
				return fmt.Errorf("create AI explanation Prompt evaluation run: %w", domainevaluation.ErrOrgConcurrencyExceeded)
			}
			return fmt.Errorf("create AI explanation Prompt evaluation run: %w", domainevaluation.ErrAlreadyExists)
		}
		return fmt.Errorf("create AI explanation Prompt evaluation run: %w", err)
	}
	return nil
}

func isActiveOrgExecutionDuplicate(err error) bool {
	return err != nil && strings.Contains(err.Error(), "uk_ai_explanation_prompt_evaluation_active_org_execution")
}

func (r *PromptEvaluationRepository) Save(ctx context.Context, value *domainevaluation.PromptEvaluationRun, expectedVersion int64) error {
	po, err := r.mapper.PromptEvaluationRunToPO(value)
	if err != nil {
		return err
	}
	if expectedVersion < 1 || po.Version <= expectedVersion {
		return domainevaluation.ErrConflict
	}
	setFields := bson.M{
		"status": po.Status, "version": po.Version, "attempts": po.Attempts, "reviews": po.Reviews, "recoveries": po.Recoveries,
		"execution": po.Execution,
		"closed_at": po.ClosedAt, "finalized_at": po.FinalizedAt, "finalized_by": po.FinalizedBy,
		"final_reason": po.FinalReason, "gate": po.Gate, "canceled_at": po.CanceledAt,
		"canceled_by": po.CanceledBy, "cancel_reason": po.CancelReason, "updated_at": po.UpdatedAt,
	}
	update := bson.M{"$set": setFields}
	if value.Status().IsTerminal() {
		terminalAt := value.FinalizedAt()
		if value.Status() == domainevaluation.StatusCanceled {
			terminalAt = value.CanceledAt()
		}
		if terminalAt == nil {
			return fmt.Errorf("terminal AI explanation Prompt evaluation has no terminal time")
		}
		expiresAt, expiryErr := expiresAfter(*terminalAt, r.retention.PromptEvaluationRetention)
		if expiryErr != nil {
			return expiryErr
		}
		setFields["expires_at"] = expiresAt
		setFields["retention_policy_version"] = strings.TrimSpace(r.retention.Version)
	}
	if po.ActiveReleaseKey != "" {
		setFields["active_release_key"] = po.ActiveReleaseKey
	} else {
		update["$unset"] = bson.M{"active_release_key": ""}
	}
	if po.ActiveExecutionOrgKey != "" {
		setFields["active_execution_org_key"] = po.ActiveExecutionOrgKey
	} else {
		unset, _ := update["$unset"].(bson.M)
		if unset == nil {
			unset = bson.M{}
		}
		unset["active_execution_org_key"] = ""
		update["$unset"] = unset
	}
	result, err := r.UpdateOne(ctx, bson.M{"domain_id": po.DomainID, "version": expectedVersion}, update)
	if err != nil {
		return fmt.Errorf("save AI explanation Prompt evaluation run: %w", err)
	}
	if result.MatchedCount != 1 {
		return domainevaluation.ErrConflict
	}
	return nil
}

func (r *PromptEvaluationRepository) FindByID(ctx context.Context, id meta.ID) (*domainevaluation.PromptEvaluationRun, error) {
	var po PromptEvaluationRunPO
	if err := r.FindOne(ctx, bson.M{"domain_id": id}, &po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domainevaluation.ErrNotFound
		}
		return nil, fmt.Errorf("find AI explanation Prompt evaluation run: %w", err)
	}
	return r.mapper.PromptEvaluationRunToDomain(&po)
}

func (r *PromptEvaluationRepository) ListExpiredPreparations(ctx context.Context, at time.Time, limit int) ([]domainevaluation.ExpiredPreparation, error) {
	if r == nil || at.IsZero() || limit < 1 {
		return nil, fmt.Errorf("list expired AI explanation Prompt evaluation preparations: invalid query")
	}
	cursor, err := r.Find(ctx, expiredPreparedEvaluationFilter(at), options.Find().
		SetProjection(bson.M{"domain_id": 1, "execution.invocation_id": 1, "execution.lease_expires_at": 1}).
		SetSort(bson.D{{Key: "execution.lease_expires_at", Value: 1}, {Key: "domain_id", Value: 1}}).
		SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("list expired AI explanation Prompt evaluation preparations: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	type expiredPreparationPO struct {
		DomainID  meta.ID `bson:"domain_id"`
		Execution struct {
			InvocationID   string    `bson:"invocation_id"`
			LeaseExpiresAt time.Time `bson:"lease_expires_at"`
		} `bson:"execution"`
	}
	result := make([]domainevaluation.ExpiredPreparation, 0)
	for cursor.Next(ctx) {
		var value expiredPreparationPO
		if err := cursor.Decode(&value); err != nil {
			return nil, fmt.Errorf("decode expired AI explanation Prompt evaluation preparation: %w", err)
		}
		if value.DomainID.IsZero() || value.Execution.InvocationID == "" || value.Execution.LeaseExpiresAt.IsZero() {
			return nil, fmt.Errorf("expired AI explanation Prompt evaluation preparation identity is invalid")
		}
		result = append(result, domainevaluation.ExpiredPreparation{
			RunID: value.DomainID, InvocationID: value.Execution.InvocationID, LeaseExpiresAt: value.Execution.LeaseExpiresAt,
		})
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("scan expired AI explanation Prompt evaluation preparations: %w", err)
	}
	return result, nil
}

func expiredPreparedEvaluationFilter(at time.Time) bson.M {
	return bson.M{
		"status":                     string(domainevaluation.StatusCollecting),
		"execution.phase":            string(domainevaluation.AttemptExecutionPrepared),
		"execution.lease_expires_at": bson.M{"$lte": at},
	}
}

type PromptEvaluationBudgetRepository struct {
	base.BaseRepository
	retention RetentionPolicy
}

func NewPromptEvaluationBudgetRepository(db *mongo.Database, retention RetentionPolicy, opts ...base.BaseRepositoryOptions) (*PromptEvaluationBudgetRepository, error) {
	if err := retention.Validate(); err != nil {
		return nil, err
	}
	repository := &PromptEvaluationBudgetRepository{
		BaseRepository: base.NewBaseRepository(db, (PromptEvaluationDailyBudgetPO{}).CollectionName(), opts...), retention: retention,
	}
	_, err := repository.Collection().Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "org_id", Value: 1}, {Key: "budget_day", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_prompt_evaluation_budget_org_day").SetUnique(true)},
		{Keys: bson.D{{Key: "reservations.run_id", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_prompt_evaluation_budget_run").SetUnique(true).SetSparse(true)},
		ttlIndex(),
	})
	if err != nil {
		return nil, fmt.Errorf("create AI explanation Prompt evaluation budget indexes: %w", err)
	}
	if retention.Enabled() {
		if err := rejectMissingTerminalExpiration(context.Background(), repository.Collection(), bson.M{"domain_id": bson.M{"$exists": true}}); err != nil {
			return nil, fmt.Errorf("verify AI explanation Prompt evaluation budget retention boundary: %w", err)
		}
	}
	return repository, nil
}

var (
	_ domainevaluation.CapacityRepository = (*PromptEvaluationBudgetRepository)(nil)
	_ domainevaluation.CapacityReader     = (*PromptEvaluationBudgetRepository)(nil)
)

func (r *PromptEvaluationBudgetRepository) EnsureDailyBucket(ctx context.Context, orgID int64, budgetDay, at time.Time) error {
	if r == nil || orgID <= 0 || budgetDay.IsZero() || !budgetDay.Equal(domainevaluation.UTCBudgetDay(budgetDay)) || at.IsZero() {
		return fmt.Errorf("ensure AI explanation Prompt evaluation daily budget: invalid bucket")
	}
	expiresAt, err := capacityLedgerExpiresAt(budgetDay, r.retention.CapacityLedgerRetention)
	if err != nil {
		return err
	}
	filter := bson.M{"org_id": orgID, "budget_day": budgetDay}
	update := bson.M{"$setOnInsert": bson.M{
		"domain_id": meta.New(), "org_id": orgID, "budget_day": budgetDay,
		"reserved_provider_invocations": 0, "reservations": []PromptEvaluationBudgetReservationPO{},
		"created_at": at.UTC(), "updated_at": at.UTC(), "expires_at": expiresAt,
		"retention_policy_version": strings.TrimSpace(r.retention.Version),
	}}
	if _, err := r.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true)); err != nil && !mongo.IsDuplicateKeyError(err) {
		return fmt.Errorf("ensure AI explanation Prompt evaluation daily budget: %w", err)
	}
	return nil
}

func (r *PromptEvaluationBudgetRepository) ReserveDailyProviderInvocations(ctx context.Context, reservation domainevaluation.DailyCapacityReservation) error {
	if r == nil {
		return fmt.Errorf("reserve AI explanation Prompt evaluation daily budget: repository is required")
	}
	if err := reservation.Validate(); err != nil {
		return err
	}
	filter := dailyBudgetReservationFilter(reservation)
	update := dailyBudgetReservationUpdate(reservation)
	result, err := r.UpdateOne(ctx, filter, update)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("reserve AI explanation Prompt evaluation daily budget: %w", domainevaluation.ErrConflict)
		}
		return fmt.Errorf("reserve AI explanation Prompt evaluation daily budget: %w", err)
	}
	if result.MatchedCount != 1 {
		var existing struct {
			DomainID meta.ID `bson:"domain_id"`
		}
		err := r.FindOne(ctx, bson.M{"reservations.run_id": reservation.RunID}, &existing, options.FindOne().SetProjection(bson.M{"domain_id": 1}))
		if err == nil {
			return domainevaluation.ErrConflict
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return fmt.Errorf("classify AI explanation Prompt evaluation daily budget rejection: %w", err)
		}
		return domainevaluation.ErrDailyBudgetExceeded
	}
	return nil
}

func (r *PromptEvaluationBudgetRepository) FindDailyCapacityUsage(ctx context.Context, orgID int64, budgetDay time.Time) (domainevaluation.DailyCapacityUsage, bool, error) {
	if r == nil || orgID <= 0 || budgetDay.Location() != time.UTC || !budgetDay.Equal(domainevaluation.UTCBudgetDay(budgetDay)) {
		return domainevaluation.DailyCapacityUsage{}, false, fmt.Errorf("find AI explanation Prompt evaluation daily capacity: invalid query")
	}
	var po PromptEvaluationDailyBudgetPO
	if err := r.FindOne(ctx, bson.M{"org_id": orgID, "budget_day": budgetDay}, &po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return domainevaluation.DailyCapacityUsage{}, false, nil
		}
		return domainevaluation.DailyCapacityUsage{}, false, fmt.Errorf("find AI explanation Prompt evaluation daily capacity: %w", err)
	}
	usage, err := dailyCapacityUsageFromPO(&po)
	if err != nil {
		return domainevaluation.DailyCapacityUsage{}, false, err
	}
	if usage.OrgID != orgID || !usage.BudgetDay.Equal(budgetDay) {
		return domainevaluation.DailyCapacityUsage{}, false, fmt.Errorf("AI explanation Prompt evaluation daily capacity identity is inconsistent")
	}
	return usage, true, nil
}

func dailyCapacityUsageFromPO(po *PromptEvaluationDailyBudgetPO) (domainevaluation.DailyCapacityUsage, error) {
	if po == nil {
		return domainevaluation.DailyCapacityUsage{}, fmt.Errorf("AI explanation Prompt evaluation daily capacity document is required")
	}
	usage := domainevaluation.DailyCapacityUsage{
		OrgID: po.OrgID, BudgetDay: po.BudgetDay, ReservedProviderInvocations: po.ReservedProviderInvocations,
		Reservations: make([]domainevaluation.DailyCapacityUsageReservation, 0, len(po.Reservations)),
	}
	for _, reservation := range po.Reservations {
		usage.Reservations = append(usage.Reservations, domainevaluation.DailyCapacityUsageReservation{
			RunID: reservation.RunID, RequestedBy: strings.TrimSpace(reservation.RequestedBy),
			ProviderInvocations: reservation.ProviderInvocations, ReservedAt: reservation.ReservedAt,
		})
	}
	if err := usage.Validate(); err != nil {
		return domainevaluation.DailyCapacityUsage{}, err
	}
	return usage, nil
}

func dailyBudgetReservationFilter(reservation domainevaluation.DailyCapacityReservation) bson.M {
	return bson.M{
		"org_id":                        reservation.OrgID,
		"budget_day":                    reservation.BudgetDay,
		"reserved_provider_invocations": bson.M{"$lte": reservation.DailyLimit - reservation.ProviderInvocations},
		"reservations.run_id":           bson.M{"$ne": reservation.RunID},
	}
}

func dailyBudgetReservationUpdate(reservation domainevaluation.DailyCapacityReservation) bson.M {
	return bson.M{
		"$inc": bson.M{"reserved_provider_invocations": reservation.ProviderInvocations},
		"$push": bson.M{"reservations": PromptEvaluationBudgetReservationPO{
			RunID: reservation.RunID, RequestedBy: strings.TrimSpace(reservation.RequestedBy),
			ProviderInvocations: reservation.ProviderInvocations, ReservedAt: reservation.ReservedAt.UTC(),
		}},
		"$set": bson.M{"updated_at": reservation.ReservedAt.UTC()},
	}
}

// ParticipantBudgetRepository owns exact per-attempt cost reservations.
// One organization/day document keeps the organization, user and Assessment
// predicates in a single atomic update and bounds ledger growth by the org cap.
type ParticipantBudgetRepository struct {
	base.BaseRepository
	retention RetentionPolicy
}

func NewParticipantBudgetRepository(db *mongo.Database, retention RetentionPolicy, opts ...base.BaseRepositoryOptions) (*ParticipantBudgetRepository, error) {
	if err := retention.Validate(); err != nil {
		return nil, err
	}
	repository := &ParticipantBudgetRepository{
		BaseRepository: base.NewBaseRepository(db, (ParticipantDailyBudgetPO{}).CollectionName(), opts...), retention: retention,
	}
	_, err := repository.Collection().Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "org_id", Value: 1}, {Key: "budget_day", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_participant_budget_org_day").SetUnique(true)},
		{Keys: bson.D{{Key: "reservations.reservation_id", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_participant_budget_reservation").SetUnique(true).SetSparse(true)},
		ttlIndex(),
	})
	if err != nil {
		return nil, fmt.Errorf("create AI explanation participant budget indexes: %w", err)
	}
	if retention.Enabled() {
		if err := rejectMissingTerminalExpiration(context.Background(), repository.Collection(), bson.M{"domain_id": bson.M{"$exists": true}}); err != nil {
			return nil, fmt.Errorf("verify AI explanation participant budget retention boundary: %w", err)
		}
	}
	return repository, nil
}

var (
	_ domaingeneration.ParticipantCapacityRepository = (*ParticipantBudgetRepository)(nil)
	_ domaingeneration.ParticipantCapacityReader     = (*ParticipantBudgetRepository)(nil)
)

func (r *ParticipantBudgetRepository) EnsureParticipantDailyBucket(ctx context.Context, orgID int64, budgetDay, at time.Time) error {
	if r == nil || orgID <= 0 || budgetDay.IsZero() || !budgetDay.Equal(domaingeneration.ParticipantUTCBudgetDay(budgetDay)) || at.IsZero() {
		return fmt.Errorf("ensure AI explanation participant daily budget: invalid bucket")
	}
	expiresAt, err := capacityLedgerExpiresAt(budgetDay, r.retention.CapacityLedgerRetention)
	if err != nil {
		return err
	}
	filter := bson.M{"org_id": orgID, "budget_day": budgetDay}
	update := bson.M{"$setOnInsert": bson.M{
		"domain_id": meta.New(), "org_id": orgID, "budget_day": budgetDay,
		"reserved_provider_invocations": 0, "redacted_provider_invocations": 0, "reservations": []ParticipantBudgetReservationPO{},
		"created_at": at.UTC(), "updated_at": at.UTC(), "expires_at": expiresAt,
		"retention_policy_version": strings.TrimSpace(r.retention.Version),
	}}
	if _, err := r.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true)); err != nil && !mongo.IsDuplicateKeyError(err) {
		return fmt.Errorf("ensure AI explanation participant daily budget: %w", err)
	}
	return nil
}

func (r *ParticipantBudgetRepository) ReserveParticipantDailyProviderInvocations(ctx context.Context, reservation domaingeneration.ParticipantDailyCapacityReservation) error {
	if r == nil {
		return fmt.Errorf("reserve AI explanation participant daily budget: repository is required")
	}
	if err := reservation.Validate(); err != nil {
		return err
	}
	reservation.UserID = strings.TrimSpace(reservation.UserID)
	result, err := r.UpdateOne(ctx, participantDailyBudgetReservationFilter(reservation), participantDailyBudgetReservationUpdate(reservation))
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("reserve AI explanation participant daily budget: %w", domaingeneration.ErrConflict)
		}
		return fmt.Errorf("reserve AI explanation participant daily budget: %w", err)
	}
	if result.MatchedCount == 1 {
		return nil
	}
	return r.classifyParticipantDailyBudgetRejection(ctx, reservation)
}

func (r *ParticipantBudgetRepository) FindParticipantDailyCapacityUsage(ctx context.Context, orgID int64, budgetDay time.Time) (domaingeneration.ParticipantDailyCapacityUsage, bool, error) {
	if r == nil || orgID <= 0 || budgetDay.Location() != time.UTC || !budgetDay.Equal(domaingeneration.ParticipantUTCBudgetDay(budgetDay)) {
		return domaingeneration.ParticipantDailyCapacityUsage{}, false, fmt.Errorf("find AI explanation participant daily capacity: invalid query")
	}
	var po ParticipantDailyBudgetPO
	if err := r.FindOne(ctx, bson.M{"org_id": orgID, "budget_day": budgetDay}, &po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return domaingeneration.ParticipantDailyCapacityUsage{}, false, nil
		}
		return domaingeneration.ParticipantDailyCapacityUsage{}, false, fmt.Errorf("find AI explanation participant daily capacity: %w", err)
	}
	usage, err := participantDailyCapacityUsageFromPO(&po)
	if err != nil {
		return domaingeneration.ParticipantDailyCapacityUsage{}, false, err
	}
	if usage.OrgID != orgID || !usage.BudgetDay.Equal(budgetDay) {
		return domaingeneration.ParticipantDailyCapacityUsage{}, false, fmt.Errorf("AI explanation participant daily capacity identity is inconsistent")
	}
	return usage, true, nil
}

func participantDailyCapacityUsageFromPO(po *ParticipantDailyBudgetPO) (domaingeneration.ParticipantDailyCapacityUsage, error) {
	if po == nil {
		return domaingeneration.ParticipantDailyCapacityUsage{}, fmt.Errorf("AI explanation participant daily capacity document is required")
	}
	usage := domaingeneration.ParticipantDailyCapacityUsage{
		OrgID: po.OrgID, BudgetDay: po.BudgetDay, ReservedProviderInvocations: po.ReservedProviderInvocations,
		RedactedProviderInvocations: po.RedactedProviderInvocations,
		Reservations:                make([]domaingeneration.ParticipantDailyCapacityUsageReservation, 0, len(po.Reservations)),
	}
	for _, reservation := range po.Reservations {
		usage.Reservations = append(usage.Reservations, domaingeneration.ParticipantDailyCapacityUsageReservation{
			ReservationID: strings.TrimSpace(reservation.ReservationID), GenerationID: reservation.GenerationID,
			Attempt: reservation.Attempt, Origin: retrygovernance.AttemptOrigin(reservation.Origin),
			UserID: strings.TrimSpace(reservation.UserID), AssessmentID: reservation.AssessmentID,
			ProviderInvocations: reservation.ProviderInvocations, ReservedAt: reservation.ReservedAt,
		})
	}
	if err := usage.Validate(); err != nil {
		return domaingeneration.ParticipantDailyCapacityUsage{}, err
	}
	return usage, nil
}

func (r *ParticipantBudgetRepository) classifyParticipantDailyBudgetRejection(ctx context.Context, reservation domaingeneration.ParticipantDailyCapacityReservation) error {
	var existing struct {
		DomainID meta.ID `bson:"domain_id"`
	}
	if err := r.FindOne(ctx, bson.M{"reservations.reservation_id": strings.TrimSpace(reservation.ReservationID)}, &existing, options.FindOne().SetProjection(bson.M{"domain_id": 1})); err == nil {
		return domaingeneration.ErrConflict
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("classify AI explanation participant duplicate reservation: %w", err)
	}

	var bucket ParticipantDailyBudgetPO
	if err := r.FindOne(ctx, bson.M{"org_id": reservation.OrgID, "budget_day": reservation.BudgetDay}, &bucket); err != nil {
		return fmt.Errorf("classify AI explanation participant daily budget rejection: %w", err)
	}
	if bucket.ReservedProviderInvocations+reservation.ProviderInvocations > reservation.Policy.DailyProviderInvocationBudgetPerOrg {
		return domaingeneration.ErrOrgDailyBudgetExceeded
	}
	userInvocations, assessmentInvocations := 0, 0
	for _, current := range bucket.Reservations {
		if current.UserID == reservation.UserID {
			userInvocations += current.ProviderInvocations
		}
		if current.AssessmentID == reservation.AssessmentID {
			assessmentInvocations += current.ProviderInvocations
		}
	}
	if userInvocations+reservation.ProviderInvocations > reservation.Policy.DailyProviderInvocationBudgetPerUser {
		return domaingeneration.ErrUserDailyBudgetExceeded
	}
	if assessmentInvocations+reservation.ProviderInvocations > reservation.Policy.DailyProviderInvocationBudgetPerAssessment {
		return domaingeneration.ErrAssessmentDailyBudgetExceeded
	}
	return domaingeneration.ErrConflict
}

func participantDailyBudgetReservationFilter(reservation domaingeneration.ParticipantDailyCapacityReservation) bson.M {
	userID := strings.TrimSpace(reservation.UserID)
	return bson.M{
		"org_id":                        reservation.OrgID,
		"budget_day":                    reservation.BudgetDay,
		"reserved_provider_invocations": bson.M{"$lte": reservation.Policy.DailyProviderInvocationBudgetPerOrg - reservation.ProviderInvocations},
		"reservations.reservation_id":   bson.M{"$ne": strings.TrimSpace(reservation.ReservationID)},
		"$expr": bson.M{"$and": bson.A{
			participantReservationDimensionWithinLimit("user_id", userID, reservation.Policy.DailyProviderInvocationBudgetPerUser-reservation.ProviderInvocations),
			participantReservationDimensionWithinLimit("assessment_id", reservation.AssessmentID, reservation.Policy.DailyProviderInvocationBudgetPerAssessment-reservation.ProviderInvocations),
		}},
	}
}

func participantReservationDimensionWithinLimit(field string, value any, maximumExisting int) bson.M {
	return bson.M{"$lte": bson.A{
		bson.M{"$size": bson.M{"$filter": bson.M{
			"input": "$reservations", "as": "reservation",
			"cond": bson.M{"$eq": bson.A{"$$reservation." + field, value}},
		}}},
		maximumExisting,
	}}
}

func participantDailyBudgetReservationUpdate(reservation domaingeneration.ParticipantDailyCapacityReservation) bson.M {
	return bson.M{
		"$inc": bson.M{"reserved_provider_invocations": reservation.ProviderInvocations},
		"$push": bson.M{"reservations": ParticipantBudgetReservationPO{
			ReservationID: strings.TrimSpace(reservation.ReservationID), GenerationID: reservation.GenerationID,
			Attempt: reservation.Attempt, Origin: string(reservation.Origin),
			UserID: strings.TrimSpace(reservation.UserID), AssessmentID: reservation.AssessmentID,
			ProviderInvocations: reservation.ProviderInvocations, ReservedAt: reservation.ReservedAt.UTC(),
		}},
		"$set": bson.M{"updated_at": reservation.ReservedAt.UTC()},
	}
}

// ParticipantActiveCapacityRepository owns the exact distributed Provider
// execution slots for participant traffic. One bounded document per
// organization makes the org/user/Assessment predicates a single atomic write.
type ParticipantActiveCapacityRepository struct {
	base.BaseRepository
}

func NewParticipantActiveCapacityRepository(db *mongo.Database, opts ...base.BaseRepositoryOptions) (*ParticipantActiveCapacityRepository, error) {
	repository := &ParticipantActiveCapacityRepository{
		BaseRepository: base.NewBaseRepository(db, (ParticipantActiveCapacityPO{}).CollectionName(), opts...),
	}
	_, err := repository.Collection().Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "org_id", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_participant_active_capacity_org").SetUnique(true)},
		{Keys: bson.D{{Key: "reservations.generation_id", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_participant_active_generation").SetUnique(true).SetSparse(true)},
		{Keys: bson.D{{Key: "reservations.run_id", Value: 1}}, Options: options.Index().SetName("uk_ai_explanation_participant_active_run").SetUnique(true).SetSparse(true)},
	})
	if err != nil {
		return nil, fmt.Errorf("create AI explanation participant active capacity indexes: %w", err)
	}
	return repository, nil
}

var (
	_ domaingeneration.ParticipantActiveCapacityRepository = (*ParticipantActiveCapacityRepository)(nil)
	_ domaingeneration.ParticipantActiveCapacityReader     = (*ParticipantActiveCapacityRepository)(nil)
)

func (r *ParticipantActiveCapacityRepository) EnsureParticipantActiveBucket(ctx context.Context, orgID int64, at time.Time) error {
	if r == nil || orgID <= 0 || at.IsZero() {
		return fmt.Errorf("ensure AI explanation participant active capacity: invalid bucket")
	}
	update := bson.M{"$setOnInsert": bson.M{
		"domain_id": meta.New(), "org_id": orgID, "active_executions": 0,
		"reservations": []ParticipantActiveCapacityReservationPO{}, "created_at": at.UTC(), "updated_at": at.UTC(),
	}}
	if _, err := r.UpdateOne(ctx, bson.M{"org_id": orgID}, update, options.Update().SetUpsert(true)); err != nil && !mongo.IsDuplicateKeyError(err) {
		return fmt.Errorf("ensure AI explanation participant active capacity: %w", err)
	}
	return nil
}

func (r *ParticipantActiveCapacityRepository) AcquireParticipantActiveSlot(ctx context.Context, slot domaingeneration.ParticipantActiveSlot) error {
	if r == nil {
		return fmt.Errorf("acquire AI explanation participant active slot: repository is required")
	}
	if err := slot.Validate(); err != nil {
		return err
	}
	slot.UserID = strings.TrimSpace(slot.UserID)
	result, err := r.UpdateOne(ctx, participantActiveSlotFilter(slot), participantActiveSlotAcquireUpdate(slot))
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("acquire AI explanation participant active slot: %w", domaingeneration.ErrConflict)
		}
		return fmt.Errorf("acquire AI explanation participant active slot: %w", err)
	}
	if result.MatchedCount == 1 {
		return nil
	}
	return r.classifyParticipantActiveCapacityRejection(ctx, slot)
}

func (r *ParticipantActiveCapacityRepository) ReleaseParticipantActiveSlot(ctx context.Context, release domaingeneration.ParticipantActiveSlotRelease) error {
	if r == nil {
		return fmt.Errorf("release AI explanation participant active slot: repository is required")
	}
	if err := release.Validate(); err != nil {
		return err
	}
	release.UserID = strings.TrimSpace(release.UserID)
	match := bson.M{
		"generation_id": release.GenerationID, "run_id": release.RunID, "user_id": release.UserID,
		"assessment_id": release.AssessmentID,
	}
	filter := bson.M{"org_id": release.OrgID, "active_executions": bson.M{"$gte": 1}, "reservations": bson.M{"$elemMatch": match}}
	update := bson.M{
		"$inc":  bson.M{"active_executions": -1},
		"$pull": bson.M{"reservations": match},
		"$set":  bson.M{"updated_at": release.ReleasedAt.UTC()},
	}
	result, err := r.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("release AI explanation participant active slot: %w", err)
	}
	if result.MatchedCount != 1 {
		return fmt.Errorf("release AI explanation participant active slot: %w", domaingeneration.ErrConflict)
	}
	return nil
}

func (r *ParticipantActiveCapacityRepository) FindParticipantActiveCapacityUsage(ctx context.Context, orgID int64) (domaingeneration.ParticipantActiveCapacityUsage, bool, error) {
	if r == nil || orgID <= 0 {
		return domaingeneration.ParticipantActiveCapacityUsage{}, false, fmt.Errorf("find AI explanation participant active capacity: invalid query")
	}
	var po ParticipantActiveCapacityPO
	if err := r.FindOne(ctx, bson.M{"org_id": orgID}, &po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return domaingeneration.ParticipantActiveCapacityUsage{}, false, nil
		}
		return domaingeneration.ParticipantActiveCapacityUsage{}, false, fmt.Errorf("find AI explanation participant active capacity: %w", err)
	}
	usage, err := participantActiveCapacityUsageFromPO(&po)
	if err != nil {
		return domaingeneration.ParticipantActiveCapacityUsage{}, false, err
	}
	if usage.OrgID != orgID {
		return domaingeneration.ParticipantActiveCapacityUsage{}, false, fmt.Errorf("AI explanation participant active capacity identity is inconsistent")
	}
	return usage, true, nil
}

func participantActiveCapacityUsageFromPO(po *ParticipantActiveCapacityPO) (domaingeneration.ParticipantActiveCapacityUsage, error) {
	if po == nil {
		return domaingeneration.ParticipantActiveCapacityUsage{}, fmt.Errorf("AI explanation participant active capacity document is required")
	}
	usage := domaingeneration.ParticipantActiveCapacityUsage{
		OrgID: po.OrgID, ActiveExecutions: po.ActiveExecutions,
		Reservations: make([]domaingeneration.ParticipantActiveCapacityUsageReservation, 0, len(po.Reservations)),
	}
	for _, reservation := range po.Reservations {
		usage.Reservations = append(usage.Reservations, domaingeneration.ParticipantActiveCapacityUsageReservation{
			GenerationID: reservation.GenerationID, RunID: reservation.RunID, UserID: strings.TrimSpace(reservation.UserID),
			AssessmentID: reservation.AssessmentID, AcquiredAt: reservation.AcquiredAt,
		})
	}
	if err := usage.Validate(); err != nil {
		return domaingeneration.ParticipantActiveCapacityUsage{}, err
	}
	return usage, nil
}

func (r *ParticipantActiveCapacityRepository) classifyParticipantActiveCapacityRejection(ctx context.Context, slot domaingeneration.ParticipantActiveSlot) error {
	var existing struct {
		DomainID meta.ID `bson:"domain_id"`
	}
	if err := r.FindOne(ctx, bson.M{"$or": bson.A{
		bson.M{"reservations.generation_id": slot.GenerationID}, bson.M{"reservations.run_id": slot.RunID},
	}}, &existing, options.FindOne().SetProjection(bson.M{"domain_id": 1})); err == nil {
		return domaingeneration.ErrConflict
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("classify AI explanation participant duplicate active slot: %w", err)
	}

	var bucket ParticipantActiveCapacityPO
	if err := r.FindOne(ctx, bson.M{"org_id": slot.OrgID}, &bucket); err != nil {
		return fmt.Errorf("classify AI explanation participant active capacity rejection: %w", err)
	}
	if bucket.ActiveExecutions >= slot.Policy.MaxActiveProviderExecutionsPerOrg {
		return domaingeneration.ErrOrgActiveCapacityExceeded
	}
	userExecutions, assessmentExecutions := 0, 0
	for _, current := range bucket.Reservations {
		if current.UserID == slot.UserID {
			userExecutions++
		}
		if current.AssessmentID == slot.AssessmentID {
			assessmentExecutions++
		}
	}
	if userExecutions >= slot.Policy.MaxActiveProviderExecutionsPerUser {
		return domaingeneration.ErrUserActiveCapacityExceeded
	}
	if assessmentExecutions >= slot.Policy.MaxActiveProviderExecutionsPerAssessment {
		return domaingeneration.ErrAssessmentActiveCapacityExceeded
	}
	return domaingeneration.ErrConflict
}

func participantActiveSlotFilter(slot domaingeneration.ParticipantActiveSlot) bson.M {
	userID := strings.TrimSpace(slot.UserID)
	return bson.M{
		"org_id":                     slot.OrgID,
		"active_executions":          bson.M{"$lte": slot.Policy.MaxActiveProviderExecutionsPerOrg - 1},
		"reservations.generation_id": bson.M{"$ne": slot.GenerationID},
		"reservations.run_id":        bson.M{"$ne": slot.RunID},
		"$expr": bson.M{"$and": bson.A{
			participantReservationDimensionWithinLimit("user_id", userID, slot.Policy.MaxActiveProviderExecutionsPerUser-1),
			participantReservationDimensionWithinLimit("assessment_id", slot.AssessmentID, slot.Policy.MaxActiveProviderExecutionsPerAssessment-1),
		}},
	}
}

func participantActiveSlotAcquireUpdate(slot domaingeneration.ParticipantActiveSlot) bson.M {
	return bson.M{
		"$inc": bson.M{"active_executions": 1},
		"$push": bson.M{"reservations": ParticipantActiveCapacityReservationPO{
			GenerationID: slot.GenerationID, RunID: slot.RunID, UserID: strings.TrimSpace(slot.UserID),
			AssessmentID: slot.AssessmentID, AcquiredAt: slot.AcquiredAt.UTC(),
		}},
		"$set": bson.M{"updated_at": slot.AcquiredAt.UTC()},
	}
}
