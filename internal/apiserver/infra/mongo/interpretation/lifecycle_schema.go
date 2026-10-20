package interpretation

import (
	"context"
	"fmt"

	template "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/reporttemplate"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"go.mongodb.org/mongo-driver/mongo"
)

// The transaction path borrows previously installed collections. These
// constructors never reconcile indexes or create resources at runtime.
func NewGenerationRepositoryWithoutSchema(db *mongo.Database, opts ...base.BaseRepositoryOptions) *GenerationRepository {
	return &GenerationRepository{BaseRepository: base.NewBaseRepository(db, (ReportGenerationPO{}).CollectionName(), opts...), mapper: NewLifecycleMapper()}
}

func NewRunRepositoryWithoutSchema(db *mongo.Database, opts ...base.BaseRepositoryOptions) *RunRepository {
	return &RunRepository{BaseRepository: base.NewBaseRepository(db, (InterpretationRunPO{}).CollectionName(), opts...), mapper: NewLifecycleMapper()}
}

func NewReportRepositoryWithoutSchema(db *mongo.Database, opts ...base.BaseRepositoryOptions) *ReportRepository {
	return &ReportRepository{BaseRepository: base.NewBaseRepository(db, (InterpretReportPO{}).CollectionName(), opts...), mapper: NewLifecycleMapper()}
}

func NewReportCatalogProjectorWithoutSchema(db *mongo.Database, opts ...base.BaseRepositoryOptions) *ReportCatalogProjector {
	return &ReportCatalogProjector{BaseRepository: base.NewBaseRepository(db, (ReportCatalogPO{}).CollectionName(), opts...), mapper: NewLifecycleMapper()}
}

func NewCatalogReconcileStoreWithoutSchema(db *mongo.Database, opts ...base.BaseRepositoryOptions) *CatalogReconcileStore {
	store := &CatalogReconcileStore{db: db}
	if db != nil {
		store.repairPlans = base.NewBaseRepository(db, catalogRepairPlanCollection, opts...)
		store.catalog = base.NewBaseRepository(db, (ReportCatalogPO{}).CollectionName(), opts...)
		store.reports = base.NewBaseRepository(db, (InterpretReportPO{}).CollectionName(), opts...)
	}
	return store
}

// NewReportTemplateRepositoryWithoutSchema also omits template bootstrap.
// A missing template remains missing until the host explicitly installs it.
func NewReportTemplateRepositoryWithoutSchema(db *mongo.Database, manifests template.ManifestCatalog, opts ...base.BaseRepositoryOptions) (*ReportTemplateRepository, error) {
	if db == nil || manifests == nil {
		return nil, fmt.Errorf("original report database and template manifest catalog are required")
	}
	return &ReportTemplateRepository{BaseRepository: base.NewBaseRepository(db, reportTemplateCollection, opts...), manifests: manifests}, nil
}

// InstallReportModuleSchema installs module-owned indexes and initial template
// releases explicitly. Startup and the inert constructors never call it.
func InstallReportModuleSchema(ctx context.Context, db *mongo.Database, manifests template.ManifestCatalog) error {
	repo, err := NewReportTemplateRepositoryWithoutSchema(db, manifests)
	if err != nil {
		return err
	}
	if err := InstallLifecycleSchema(ctx, db); err != nil {
		return err
	}
	if _, err := db.Collection(catalogRepairPlanCollection).Indexes().CreateMany(ctx, catalogRepairIndexModels()); err != nil {
		return fmt.Errorf("install catalog repair indexes: %w", err)
	}
	if _, err := repo.Collection().Indexes().CreateMany(ctx, reportTemplateIndexModels()); err != nil {
		return fmt.Errorf("install report template indexes: %w", err)
	}
	return repo.ensureBootstrap(ctx, manifests)
}

// InstallLifecycleSchema is an explicit operator/test installation step. It is
// never called by a constructor or Start. SDK protocol schema is separate.
func InstallLifecycleSchema(ctx context.Context, db *mongo.Database) error {
	if db == nil {
		return fmt.Errorf("original report database is required")
	}
	for _, resource := range []struct {
		name    string
		indexes []mongo.IndexModel
	}{
		{(ReportGenerationPO{}).CollectionName(), generationIndexModels()},
		{(InterpretationRunPO{}).CollectionName(), runIndexModels()},
		{(InterpretReportPO{}).CollectionName(), reportIndexModels()},
		{(ReportCatalogPO{}).CollectionName(), reportCatalogIndexModels()},
	} {
		if _, err := db.Collection(resource.name).Indexes().CreateMany(ctx, resource.indexes); err != nil {
			return fmt.Errorf("install report lifecycle indexes %s: %w", resource.name, err)
		}
	}
	return nil
}
