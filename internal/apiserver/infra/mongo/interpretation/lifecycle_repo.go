package interpretation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type GenerationRepository struct {
	base.BaseRepository
	mapper *LifecycleMapper
}

func NewGenerationRepository(db *mongo.Database, opts ...base.BaseRepositoryOptions) (*GenerationRepository, error) {
	repo := &GenerationRepository{BaseRepository: base.NewBaseRepository(db, (ReportGenerationPO{}).CollectionName(), opts...), mapper: NewLifecycleMapper()}
	if _, err := repo.Collection().Indexes().CreateMany(context.Background(), generationIndexModels()); err != nil {
		return nil, fmt.Errorf("create report generation indexes: %w", err)
	}
	return repo, nil
}

func generationIndexModels() []mongo.IndexModel {
	return []mongo.IndexModel{
		{Keys: bson.D{{Key: "domain_id", Value: 1}}, Options: options.Index().SetName("uk_generation_domain_id").SetUnique(true)},
		{Keys: bson.D{{Key: "outcome_id", Value: 1}, {Key: "report_type", Value: 1}, {Key: "template_version", Value: 1}}, Options: options.Index().SetName("uk_generation_key").SetUnique(true)},
		{Keys: bson.D{{Key: "outcome_id", Value: 1}, {Key: "status", Value: 1}, {Key: "updated_at", Value: -1}}, Options: options.Index().SetName("idx_generation_outcome_status_updated")},
	}
}

var _ generation.Repository = (*GenerationRepository)(nil)

func (r *GenerationRepository) Create(ctx context.Context, domain *generation.ReportGeneration) error {
	po := r.mapper.GenerationToPO(domain)
	if po == nil {
		return fmt.Errorf("report generation is required")
	}
	if _, err := r.InsertOne(ctx, po); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("create report generation: %w", generation.ErrAlreadyExists)
		}
		return fmt.Errorf("create report generation: %w", err)
	}
	return nil
}

func (r *GenerationRepository) FindByID(ctx context.Context, id generation.ID) (*generation.ReportGeneration, error) {
	var po ReportGenerationPO
	if err := r.FindOne(ctx, bson.M{"domain_id": id.Uint64()}, &po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, generation.ErrNotFound
		}
		return nil, fmt.Errorf("find report generation by id: %w", err)
	}
	return r.mapper.GenerationToDomain(&po)
}

func (r *GenerationRepository) FindByKey(ctx context.Context, key generation.Key) (*generation.ReportGeneration, error) {
	var po ReportGenerationPO
	if err := r.FindOne(ctx, bson.M{"outcome_id": key.OutcomeID.Uint64(), "report_type": key.ReportType.String(), "template_version": key.TemplateVersion.String()}, &po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, generation.ErrNotFound
		}
		return nil, fmt.Errorf("find report generation by key: %w", err)
	}
	return r.mapper.GenerationToDomain(&po)
}

func (r *GenerationRepository) ListByOutcomeID(ctx context.Context, outcomeID generation.ID) ([]*generation.ReportGeneration, error) {
	cursor, err := r.Find(ctx, bson.M{"outcome_id": outcomeID.Uint64()}, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}))
	if err != nil {
		return nil, fmt.Errorf("list report generations by outcome id: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()
	items := make([]*generation.ReportGeneration, 0)
	for cursor.Next(ctx) {
		var po ReportGenerationPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		item, err := r.mapper.GenerationToDomain(&po)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, cursor.Err()
}

func (r *GenerationRepository) Save(ctx context.Context, domain *generation.ReportGeneration, expectedVersion uint64) error {
	if domain == nil || expectedVersion == 0 || domain.Version() <= expectedVersion {
		return generation.ErrVersionConflict
	}
	po := r.mapper.GenerationToPO(domain)
	update := bson.M{"$set": bson.M{
		"status":        po.Status,
		"latest_run_id": po.LatestRunID,
		"report_id":     po.ReportID,
		"version":       po.Version,
		"updated_at":    po.UpdatedAt,
	}}
	result, err := r.UpdateOne(ctx, bson.M{"domain_id": domain.ID().Uint64(), "version": expectedVersion}, update)
	if err != nil {
		return fmt.Errorf("save report generation: %w", err)
	}
	if result.MatchedCount != 1 {
		return generation.ErrVersionConflict
	}
	return nil
}

type RunRepository struct {
	base.BaseRepository
	mapper *LifecycleMapper
}

func NewRunRepository(db *mongo.Database, opts ...base.BaseRepositoryOptions) (*RunRepository, error) {
	repo := &RunRepository{BaseRepository: base.NewBaseRepository(db, (InterpretationRunPO{}).CollectionName(), opts...), mapper: NewLifecycleMapper()}
	if _, err := repo.Collection().Indexes().CreateMany(context.Background(), runIndexModels()); err != nil {
		return nil, fmt.Errorf("create interpretation run indexes: %w", err)
	}
	return repo, nil
}

func runIndexModels() []mongo.IndexModel {
	return []mongo.IndexModel{
		{Keys: bson.D{{Key: "domain_id", Value: 1}}, Options: options.Index().SetName("uk_interpretation_run_domain_id").SetUnique(true)},
		{Keys: bson.D{{Key: "generation_id", Value: 1}, {Key: "attempt", Value: 1}}, Options: options.Index().SetName("uk_interpretation_run_generation_attempt").SetUnique(true)},
		{Keys: bson.D{{Key: "generation_id", Value: 1}, {Key: "attempt", Value: -1}}, Options: options.Index().SetName("idx_interpretation_run_generation_attempt_desc")},
	}
}

var _ interpretationrun.Repository = (*RunRepository)(nil)

func (r *RunRepository) Create(ctx context.Context, domain *interpretationrun.InterpretationRun) error {
	po := r.mapper.RunToPO(domain)
	if po == nil {
		return fmt.Errorf("interpretation run is required")
	}
	now := time.Now()
	po.CreatedAt = now
	po.UpdatedAt = now
	if _, err := r.InsertOne(ctx, po); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("create interpretation run: %w", interpretationrun.ErrAlreadyExists)
		}
		return fmt.Errorf("create interpretation run: %w", err)
	}
	return nil
}

func (r *RunRepository) FindByID(ctx context.Context, id interpretationrun.ID) (*interpretationrun.InterpretationRun, error) {
	var po InterpretationRunPO
	if err := r.FindOne(ctx, bson.M{"domain_id": id.Uint64()}, &po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, interpretationrun.ErrNotFound
		}
		return nil, fmt.Errorf("find interpretation run by id: %w", err)
	}
	return r.mapper.RunToDomain(&po)
}

func (r *RunRepository) FindLatestByGenerationID(ctx context.Context, generationID interpretationrun.ID) (*interpretationrun.InterpretationRun, error) {
	cursor, err := r.Find(ctx, bson.M{"generation_id": generationID.Uint64()}, options.Find().SetSort(bson.D{{Key: "attempt", Value: -1}}).SetLimit(1))
	if err != nil {
		return nil, fmt.Errorf("find latest interpretation run: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()
	if !cursor.Next(ctx) {
		if err := cursor.Err(); err != nil {
			return nil, fmt.Errorf("find latest interpretation run: %w", err)
		}
		return nil, interpretationrun.ErrNotFound
	}
	var po InterpretationRunPO
	if err := cursor.Decode(&po); err != nil {
		return nil, fmt.Errorf("decode latest interpretation run: %w", err)
	}
	return r.mapper.RunToDomain(&po)
}

func (r *RunRepository) ListByGenerationID(ctx context.Context, generationID interpretationrun.ID) ([]*interpretationrun.InterpretationRun, error) {
	cursor, err := r.Find(ctx, bson.M{"generation_id": generationID.Uint64()}, options.Find().SetSort(bson.D{{Key: "attempt", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list interpretation runs: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()
	runs := make([]*interpretationrun.InterpretationRun, 0)
	for cursor.Next(ctx) {
		var po InterpretationRunPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		domain, err := r.mapper.RunToDomain(&po)
		if err != nil {
			return nil, err
		}
		runs = append(runs, domain)
	}
	return runs, cursor.Err()
}

func (r *RunRepository) Save(ctx context.Context, domain *interpretationrun.InterpretationRun) error {
	po := r.mapper.RunToPO(domain)
	if po == nil {
		return fmt.Errorf("interpretation run is required")
	}
	update := bson.M{"$set": bson.M{"status": po.Status, "failure": po.Failure, "trace_id": po.TraceID, "started_at": po.StartedAt, "lease_expires_at": po.LeaseExpiresAt, "finished_at": po.FinishedAt, "updated_at": time.Now()}}
	result, err := r.UpdateOne(ctx, bson.M{"domain_id": domain.ID().Uint64()}, update)
	if err != nil {
		return fmt.Errorf("save interpretation run: %w", err)
	}
	if result.MatchedCount != 1 {
		return interpretationrun.ErrNotFound
	}
	return nil
}

type ReportRepository struct {
	base.BaseRepository
	mapper *LifecycleMapper
}

func NewReportRepository(db *mongo.Database, opts ...base.BaseRepositoryOptions) (*ReportRepository, error) {
	repo := &ReportRepository{BaseRepository: base.NewBaseRepository(db, (InterpretReportPO{}).CollectionName(), opts...), mapper: NewLifecycleMapper()}
	if _, err := repo.Collection().Indexes().CreateMany(context.Background(), reportIndexModels()); err != nil {
		return nil, fmt.Errorf("create interpretation report indexes: %w", err)
	}
	return repo, nil
}

func reportIndexModels() []mongo.IndexModel {
	return []mongo.IndexModel{
		{Keys: bson.D{{Key: "domain_id", Value: 1}}, Options: options.Index().SetName("uk_artifact_domain_id").SetUnique(true)},
		{Keys: bson.D{{Key: "generation_id", Value: 1}}, Options: options.Index().SetName("uk_artifact_generation_id").SetUnique(true)},
		{Keys: bson.D{{Key: "outcome_id", Value: 1}, {Key: "report_type", Value: 1}, {Key: "template_version", Value: 1}}, Options: options.Index().SetName("idx_artifact_outcome_type_version")},
		{Keys: bson.D{{Key: "assessment_id", Value: 1}, {Key: "generated_at", Value: -1}}, Options: options.Index().SetName("idx_artifact_assessment_generated")},
		{Keys: bson.D{{Key: "testee_id", Value: 1}, {Key: "generated_at", Value: -1}}, Options: options.Index().SetName("idx_artifact_testee_generated")},
	}
}

var _ domainreport.ReportRepository = (*ReportRepository)(nil)

func (r *ReportRepository) Insert(ctx context.Context, domain *domainreport.InterpretReport) error {
	po := r.mapper.ReportToPO(domain)
	if po == nil {
		return fmt.Errorf("interpretation report is required")
	}
	if _, err := r.InsertOne(ctx, po); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("insert interpretation report: %w", domainreport.ErrInterpretReportAlreadyExists)
		}
		return fmt.Errorf("insert interpretation report: %w", err)
	}
	return nil
}

func (r *ReportRepository) FindByID(ctx context.Context, id meta.ID) (*domainreport.InterpretReport, error) {
	var po InterpretReportPO
	if err := r.FindOne(ctx, bson.M{"domain_id": id.Uint64()}, &po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domainreport.ErrInterpretReportNotFound
		}
		return nil, fmt.Errorf("find interpretation report by id: %w", err)
	}
	return r.mapper.ReportToDomain(&po)
}

func (r *ReportRepository) FindByGenerationID(ctx context.Context, generationID meta.ID) (*domainreport.InterpretReport, error) {
	var po InterpretReportPO
	if err := r.FindOne(ctx, bson.M{"generation_id": generationID.Uint64()}, &po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domainreport.ErrInterpretReportNotFound
		}
		return nil, fmt.Errorf("find interpretation report by generation id: %w", err)
	}
	return r.mapper.ReportToDomain(&po)
}

func (r *ReportRepository) ListByAssessmentID(ctx context.Context, assessmentID meta.ID) ([]*domainreport.InterpretReport, error) {
	cursor, err := r.Find(ctx, bson.M{"assessment_id": assessmentID.Uint64()}, options.Find().SetSort(bson.D{{Key: "generated_at", Value: -1}}))
	if err != nil {
		return nil, fmt.Errorf("list interpretation reports by assessment id: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()
	items := make([]*domainreport.InterpretReport, 0)
	for cursor.Next(ctx) {
		var po InterpretReportPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		item, err := r.mapper.ReportToDomain(&po)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, cursor.Err()
}
