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
	CatalogDriftMissing             = "missing"
	CatalogDriftDangling            = "dangling"
	CatalogDriftAssociationMismatch = "association_mismatch"
	CatalogDriftWrongWinner         = "wrong_winner"
	catalogReconcileBatchSize       = 500
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
	var total int64
	for _, sourceKind := range []string{ReportCatalogSourceArtifact, ReportCatalogSourceArchive} {
		count, err := s.countAssociationMismatchForSource(ctx, filter, sourceKind)
		if err != nil {
			return 0, err
		}
		total += count
	}
	return total, nil
}

func (s *CatalogReconcileStore) countAssociationMismatchForSource(
	ctx context.Context,
	filter CatalogReconcileFilter,
	sourceKind string,
) (int64, error) {
	var total int64
	var afterAssessmentID uint64
	catalog := s.db.Collection((ReportCatalogPO{}).CollectionName())
	for {
		query := catalogMatchStage(filter)
		query["source_kind"] = sourceKind
		if afterAssessmentID != 0 {
			query["assessment_id"] = bson.M{"$gt": afterAssessmentID}
		}
		cur, err := catalog.Find(ctx, query, options.Find().
			SetSort(bson.D{{Key: "assessment_id", Value: 1}}).
			SetLimit(catalogReconcileBatchSize))
		if err != nil {
			return 0, err
		}
		entries := make([]ReportCatalogPO, 0, catalogReconcileBatchSize)
		for cur.Next(ctx) {
			var entry ReportCatalogPO
			if err := cur.Decode(&entry); err != nil {
				_ = cur.Close(ctx)
				return 0, err
			}
			entries = append(entries, entry)
		}
		if err := cur.Err(); err != nil {
			_ = cur.Close(ctx)
			return 0, err
		}
		_ = cur.Close(ctx)
		if len(entries) == 0 {
			return total, nil
		}
		sources, err := s.loadCatalogSourceAssociations(ctx, sourceKind, entries)
		if err != nil {
			return 0, err
		}
		total += countAssociationMismatches(entries, sources)
		afterAssessmentID = entries[len(entries)-1].AssessmentID
		if len(entries) < catalogReconcileBatchSize {
			return total, nil
		}
	}
}

func (s *CatalogReconcileStore) loadCatalogSourceAssociations(
	ctx context.Context,
	sourceKind string,
	entries []ReportCatalogPO,
) (map[uint64]CatalogSourceAssociation, error) {
	ids := make([]uint64, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.SourceID)
	}
	sources := make(map[uint64]CatalogSourceAssociation, len(ids))
	switch sourceKind {
	case ReportCatalogSourceArtifact:
		cur, err := s.db.Collection((InterpretReportPO{}).CollectionName()).Find(
			ctx,
			bson.M{"domain_id": bson.M{"$in": ids}, "deleted_at": nil},
			options.Find().SetProjection(bson.M{"domain_id": 1, "assessment_id": 1, "org_id": 1, "testee_id": 1}),
		)
		if err != nil {
			return nil, err
		}
		defer func() { _ = cur.Close(ctx) }()
		for cur.Next(ctx) {
			var po InterpretReportPO
			if err := cur.Decode(&po); err != nil {
				return nil, err
			}
			sources[po.DomainID.Uint64()] = CatalogSourceAssociation{
				AssessmentID: po.AssessmentID, OrgID: po.OrgID, HasOrgID: true, TesteeID: po.TesteeID,
			}
		}
		return sources, cur.Err()
	case ReportCatalogSourceArchive:
		cur, err := s.db.Collection((ArchivedReportPO{}).CollectionName()).Find(
			ctx,
			bson.M{"domain_id": bson.M{"$in": ids}, "deleted_at": nil},
			options.Find().SetProjection(bson.M{"domain_id": 1, "org_id": 1, "testee_id": 1}),
		)
		if err != nil {
			return nil, err
		}
		defer func() { _ = cur.Close(ctx) }()
		for cur.Next(ctx) {
			var po ArchivedReportPO
			if err := cur.Decode(&po); err != nil {
				return nil, err
			}
			source := CatalogSourceAssociation{AssessmentID: po.DomainID.Uint64(), TesteeID: po.TesteeID}
			if po.OrgID != nil {
				source.OrgID = *po.OrgID
				source.HasOrgID = true
			}
			sources[po.DomainID.Uint64()] = source
		}
		return sources, cur.Err()
	default:
		return nil, fmt.Errorf("unknown report catalog source %q", sourceKind)
	}
}

func countAssociationMismatches(entries []ReportCatalogPO, sources map[uint64]CatalogSourceAssociation) int64 {
	var count int64
	for _, entry := range entries {
		source, ok := sources[entry.SourceID]
		if !ok {
			continue // Counted independently as dangling.
		}
		if HasAssociationMismatch(entry, source) {
			count++
		}
	}
	return count
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
