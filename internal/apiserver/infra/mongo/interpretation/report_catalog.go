package interpretation

import (
	"context"
	"fmt"
	"time"

	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ReportCatalogProjector struct {
	base.BaseRepository
	mapper *LifecycleMapper
}

func NewReportCatalogProjector(db *mongo.Database, opts ...base.BaseRepositoryOptions) (*ReportCatalogProjector, error) {
	p := &ReportCatalogProjector{BaseRepository: base.NewBaseRepository(db, (ReportCatalogPO{}).CollectionName(), opts...), mapper: NewLifecycleMapper()}
	if _, err := p.Collection().Indexes().CreateMany(context.Background(), reportCatalogIndexModels()); err != nil {
		return nil, fmt.Errorf("create report catalog indexes: %w", err)
	}
	return p, nil
}

// ReportCatalogIndexModels returns the canonical report_query_catalog indexes.
// Mongo migration 000015 is the deployment source of truth; runtime CreateMany
// reuses this list as a defensive reconcile only.
func ReportCatalogIndexModels() []mongo.IndexModel {
	return reportCatalogIndexModels()
}

func reportCatalogIndexModels() []mongo.IndexModel {
	return []mongo.IndexModel{
		{Keys: bson.D{{Key: "assessment_id", Value: 1}}, Options: options.Index().SetName("uk_report_catalog_assessment").SetUnique(true)},
		{Keys: bson.D{{Key: "org_id", Value: 1}, {Key: "sort_at", Value: -1}, {Key: "assessment_id", Value: -1}}, Options: options.Index().SetName("idx_report_catalog_org_sort")},
		{Keys: bson.D{{Key: "testee_id", Value: 1}, {Key: "sort_at", Value: -1}, {Key: "assessment_id", Value: -1}}, Options: options.Index().SetName("idx_report_catalog_testee_sort")},
		{Keys: bson.D{{Key: "org_id", Value: 1}, {Key: "model_code", Value: 1}, {Key: "sort_at", Value: -1}}, Options: options.Index().SetName("idx_report_catalog_org_model_sort")},
		{Keys: bson.D{{Key: "org_id", Value: 1}, {Key: "risk_level", Value: 1}, {Key: "sort_at", Value: -1}}, Options: options.Index().SetName("idx_report_catalog_org_risk_sort")},
		{Keys: bson.D{{Key: "testee_id", Value: 1}, {Key: "model_code", Value: 1}, {Key: "sort_at", Value: -1}}, Options: options.Index().SetName("idx_report_catalog_testee_model_sort")},
		{Keys: bson.D{{Key: "testee_id", Value: 1}, {Key: "risk_level", Value: 1}, {Key: "sort_at", Value: -1}}, Options: options.Index().SetName("idx_report_catalog_testee_risk_sort")},
	}
}

// RequiredReportCatalogIndexNames lists canonical indexes that must exist after
// Mongo migration 000015. Runtime CreateMany remains a defensive reconcile only.
func RequiredReportCatalogIndexNames() []string {
	models := reportCatalogIndexModels()
	names := make([]string, 0, len(models))
	for _, model := range models {
		if model.Options != nil && model.Options.Name != nil {
			names = append(names, *model.Options.Name)
		}
	}
	return names
}

// VerifyReportCatalogIndexes fails closed when required catalog indexes are missing.
func VerifyReportCatalogIndexes(ctx context.Context, db *mongo.Database) error {
	if db == nil {
		return fmt.Errorf("mongo database is required")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	present, err := listReportCatalogIndexNames(ctx, db.Collection((ReportCatalogPO{}).CollectionName()))
	if err != nil {
		return fmt.Errorf("list report_query_catalog indexes: %w", err)
	}
	for _, name := range RequiredReportCatalogIndexNames() {
		if !present[name] {
			return fmt.Errorf("required report catalog index report_query_catalog.%s is missing; run Mongo migration 000015", name)
		}
	}
	return nil
}

func listReportCatalogIndexNames(ctx context.Context, collection *mongo.Collection) (map[string]bool, error) {
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	out := make(map[string]bool)
	for cursor.Next(ctx) {
		var item struct {
			Name string `bson:"name"`
		}
		if err := cursor.Decode(&item); err != nil {
			return nil, err
		}
		out[item.Name] = true
	}
	return out, cursor.Err()
}

func (p *ReportCatalogProjector) ProjectCurrent(ctx context.Context, report *domainreport.InterpretReport) error {
	if p == nil || report == nil {
		return fmt.Errorf("report catalog projector and report are required")
	}
	po := p.mapper.ReportToPO(report)
	entry := ReportCatalogPO{AssessmentID: po.AssessmentID, OrgID: po.OrgID, TesteeID: po.TesteeID, SourceKind: ReportCatalogSourceArtifact, SourceID: po.DomainID.Uint64(), ModelCode: po.ScaleCode, RiskLevel: po.RiskLevel, SortAt: po.GeneratedAt, SortReportID: po.DomainID.Uint64(), UpdatedAt: po.GeneratedAt}
	filter := bson.M{"assessment_id": po.AssessmentID, "$or": bson.A{
		bson.M{"source_kind": ReportCatalogSourceArchive},
		bson.M{"sort_at": bson.M{"$lt": po.GeneratedAt}},
		bson.M{"sort_at": po.GeneratedAt, "sort_report_id": bson.M{"$lt": po.DomainID.Uint64()}},
	}}
	_, err := p.Collection().UpdateOne(ctx, filter, bson.M{"$set": entry}, options.Update().SetUpsert(true))
	if mongo.IsDuplicateKeyError(err) { // a newer artifact already won
		return nil
	}
	if err != nil {
		return fmt.Errorf("project current report catalog entry: %w", err)
	}
	return nil
}
