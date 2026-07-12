package interpretation

import (
	"context"
	"fmt"

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
