package interpretation

import (
	"context"
	"fmt"

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
