package interpretation

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	CatalogDriftMissing              = "missing"
	CatalogDriftDangling             = "dangling"
	CatalogDriftAssociationMismatch  = "association_mismatch"
	CatalogDriftWrongWinner          = "wrong_winner"
)

// CatalogReconcileFilter scopes read-only catalog drift scans.
type CatalogReconcileFilter struct {
	OrgID        *int64
	SortAtAfter  *time.Time
	SortAtBefore *time.Time
}

// CatalogDriftCounts aggregates the four IR-R015 drift classes.
type CatalogDriftCounts struct {
	Missing             int64
	Dangling            int64
	AssociationMismatch int64
	WrongWinner         int64
}

func (c CatalogDriftCounts) Total() int64 {
	return c.Missing + c.Dangling + c.AssociationMismatch + c.WrongWinner
}

// CatalogReconcileStore performs read-only catalog drift detection against Mongo.
type CatalogReconcileStore struct {
	db *mongo.Database
}

func NewCatalogReconcileStore(db *mongo.Database) *CatalogReconcileStore {
	return &CatalogReconcileStore{db: db}
}

func (s *CatalogReconcileStore) CountDrifts(ctx context.Context, filter CatalogReconcileFilter) (CatalogDriftCounts, error) {
	if s == nil || s.db == nil {
		return CatalogDriftCounts{}, fmt.Errorf("catalog reconcile store is not configured")
	}
	var out CatalogDriftCounts
	var err error
	if out.Missing, err = s.countMissing(ctx, filter); err != nil {
		return CatalogDriftCounts{}, fmt.Errorf("count missing catalog entries: %w", err)
	}
	if out.Dangling, err = s.countDangling(ctx, filter); err != nil {
		return CatalogDriftCounts{}, fmt.Errorf("count dangling catalog sources: %w", err)
	}
	if out.AssociationMismatch, err = s.countAssociationMismatch(ctx, filter); err != nil {
		return CatalogDriftCounts{}, fmt.Errorf("count catalog association mismatch: %w", err)
	}
	if out.WrongWinner, err = s.countWrongWinner(ctx, filter); err != nil {
		return CatalogDriftCounts{}, fmt.Errorf("count wrong catalog winner: %w", err)
	}
	return out, nil
}

func (s *CatalogReconcileStore) countMissing(ctx context.Context, filter CatalogReconcileFilter) (int64, error) {
	artifactMatch := bson.M{"deleted_at": nil}
	if filter.OrgID != nil {
		artifactMatch["org_id"] = *filter.OrgID
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"deleted_at": nil}}},
		{{Key: "$project", Value: bson.M{"assessment_id": "$domain_id"}}},
		{{Key: "$unionWith", Value: bson.M{"coll": "interpret_report_artifacts", "pipeline": mongo.Pipeline{
			{{Key: "$match", Value: artifactMatch}},
			{{Key: "$project", Value: bson.M{"assessment_id": 1}}},
		}}}},
		{{Key: "$group", Value: bson.M{"_id": "$assessment_id"}}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "report_query_catalog",
			"localField":   "_id",
			"foreignField": "assessment_id",
			"as":           "catalog",
		}}},
		{{Key: "$match", Value: bson.M{"catalog": bson.M{"$size": 0}}}},
	}
	return aggregateCount(ctx, s.db.Collection("archived_reports"), pipeline)
}

func (s *CatalogReconcileStore) countDangling(ctx context.Context, filter CatalogReconcileFilter) (int64, error) {
	catalog := s.db.Collection((ReportCatalogPO{}).CollectionName())
	artifact, err := aggregateCount(ctx, catalog, danglingSourcePipeline(ReportCatalogSourceArtifact, (InterpretReportPO{}).CollectionName(), catalogMatchStage(filter)))
	if err != nil {
		return 0, err
	}
	archive, err := aggregateCount(ctx, catalog, danglingSourcePipeline(ReportCatalogSourceArchive, (ArchivedReportPO{}).CollectionName(), catalogMatchStage(filter)))
	if err != nil {
		return 0, err
	}
	return artifact + archive, nil
}

func (s *CatalogReconcileStore) countAssociationMismatch(ctx context.Context, filter CatalogReconcileFilter) (int64, error) {
	catalog := s.db.Collection((ReportCatalogPO{}).CollectionName())
	artifact, err := aggregateCount(ctx, catalog, associationMismatchPipeline(
		ReportCatalogSourceArtifact,
		(InterpretReportPO{}).CollectionName(),
		catalogMatchStage(filter),
		artifactAssociationMismatchExpr(),
	))
	if err != nil {
		return 0, err
	}
	archive, err := aggregateCount(ctx, catalog, associationMismatchPipeline(
		ReportCatalogSourceArchive,
		(ArchivedReportPO{}).CollectionName(),
		catalogMatchStage(filter),
		archiveAssociationMismatchExpr(),
	))
	if err != nil {
		return 0, err
	}
	return artifact + archive, nil
}

func (s *CatalogReconcileStore) countWrongWinner(ctx context.Context, filter CatalogReconcileFilter) (int64, error) {
	artifactMatch := bson.M{"deleted_at": nil}
	if filter.OrgID != nil {
		artifactMatch["org_id"] = *filter.OrgID
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: artifactMatch}},
		{{Key: "$sort", Value: bson.D{{Key: "assessment_id", Value: 1}, {Key: "generated_at", Value: -1}, {Key: "domain_id", Value: -1}}}},
		{{Key: "$group", Value: bson.M{"_id": "$assessment_id", "source_id": bson.M{"$first": "$domain_id"}}}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "report_query_catalog",
			"localField":   "_id",
			"foreignField": "assessment_id",
			"as":           "catalog",
		}}},
		{{Key: "$unwind", Value: bson.M{"path": "$catalog", "preserveNullAndEmptyArrays": true}}},
	}
	if stages := catalogMatchStage(filter); len(stages) > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: stages}})
	}
	pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.M{"$expr": bson.M{"$or": bson.A{
		bson.M{"$ne": bson.A{"$catalog.source_kind", ReportCatalogSourceArtifact}},
		bson.M{"$ne": bson.A{"$catalog.source_id", "$source_id"}},
	}}}}})
	return aggregateCount(ctx, s.db.Collection((InterpretReportPO{}).CollectionName()), pipeline)
}

func catalogMatchStage(filter CatalogReconcileFilter) bson.M {
	match := bson.M{}
	if filter.OrgID != nil {
		match["org_id"] = *filter.OrgID
	}
	if filter.SortAtAfter != nil || filter.SortAtBefore != nil {
		sortAt := bson.M{}
		if filter.SortAtAfter != nil {
			sortAt["$gte"] = *filter.SortAtAfter
		}
		if filter.SortAtBefore != nil {
			sortAt["$lte"] = *filter.SortAtBefore
		}
		match["sort_at"] = sortAt
	}
	return match
}

func danglingSourcePipeline(sourceKind, collection string, catalogMatch bson.M) mongo.Pipeline {
	pipeline := mongo.Pipeline{}
	if len(catalogMatch) > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: catalogMatch}})
	}
	pipeline = append(pipeline,
		bson.D{{Key: "$match", Value: bson.M{"source_kind": sourceKind}}},
		bson.D{{Key: "$lookup", Value: bson.M{
			"from": collection,
			"let":  bson.M{"source_id": "$source_id"},
			"pipeline": mongo.Pipeline{
				{{Key: "$match", Value: bson.M{"$expr": bson.M{"$and": bson.A{
					bson.M{"$eq": bson.A{"$domain_id", "$$source_id"}},
					bson.M{"$eq": bson.A{bson.M{"$ifNull": bson.A{"$deleted_at", nil}}, nil}},
				}}}}},
			},
			"as": "source",
		}}},
		bson.D{{Key: "$match", Value: bson.M{"source": bson.M{"$size": 0}}}},
	)
	return pipeline
}

func associationMismatchPipeline(sourceKind, collection string, catalogMatch, mismatchExpr bson.M) mongo.Pipeline {
	pipeline := mongo.Pipeline{}
	if len(catalogMatch) > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: catalogMatch}})
	}
	pipeline = append(pipeline,
		bson.D{{Key: "$match", Value: bson.M{"source_kind": sourceKind}}},
		bson.D{{Key: "$lookup", Value: bson.M{
			"from": collection,
			"let":  bson.M{"source_id": "$source_id"},
			"pipeline": mongo.Pipeline{
				{{Key: "$match", Value: bson.M{"$expr": bson.M{"$and": bson.A{
					bson.M{"$eq": bson.A{"$domain_id", "$$source_id"}},
					bson.M{"$eq": bson.A{bson.M{"$ifNull": bson.A{"$deleted_at", nil}}, nil}},
				}}}}},
			},
			"as": "source",
		}}},
		bson.D{{Key: "$unwind", Value: "$source"}},
		bson.D{{Key: "$match", Value: bson.M{"$expr": mismatchExpr}}},
	)
	return pipeline
}

func artifactAssociationMismatchExpr() bson.M {
	return bson.M{"$or": bson.A{
		bson.M{"$ne": bson.A{"$assessment_id", "$source.assessment_id"}},
		bson.M{"$ne": bson.A{"$testee_id", "$source.testee_id"}},
		bson.M{"$and": bson.A{
			bson.M{"$ne": bson.A{bson.M{"$ifNull": bson.A{"$source.org_id", nil}}, nil}},
			bson.M{"$ne": bson.A{"$org_id", "$source.org_id"}},
		}},
	}}
}

func archiveAssociationMismatchExpr() bson.M {
	return bson.M{"$or": bson.A{
		bson.M{"$ne": bson.A{"$assessment_id", "$source.domain_id"}},
		bson.M{"$ne": bson.A{"$testee_id", "$source.testee_id"}},
		bson.M{"$and": bson.A{
			bson.M{"$ne": bson.A{bson.M{"$ifNull": bson.A{"$source.org_id", nil}}, nil}},
			bson.M{"$ne": bson.A{"$org_id", "$source.org_id"}},
		}},
	}}
}

func aggregateCount(ctx context.Context, collection *mongo.Collection, pipeline mongo.Pipeline) (int64, error) {
	pipeline = append(pipeline, bson.D{{Key: "$count", Value: "count"}})
	cur, err := collection.Aggregate(ctx, pipeline, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return 0, err
	}
	defer func() { _ = cur.Close(ctx) }()
	if !cur.Next(ctx) {
		return 0, cur.Err()
	}
	var row struct {
		Count int64 `bson:"count"`
	}
	if err := cur.Decode(&row); err != nil {
		return 0, err
	}
	return row.Count, nil
}

// HasAssociationMismatch reports whether catalog and source disagree under IR-R002 rules.
func HasAssociationMismatch(catalog ReportCatalogPO, source CatalogSourceAssociation) bool {
	return len(MismatchedAssociationFields(catalog, source)) > 0
}
