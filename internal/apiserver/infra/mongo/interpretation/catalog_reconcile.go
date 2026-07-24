package interpretation

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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
	AssessmentID *uint64
	Kind         string
	SortAtAfter  *time.Time
	SortAtBefore *time.Time
}

type CatalogDriftItem struct {
	CatalogID     string   `bson:"catalog_id"`
	ReportID      string   `bson:"report_id"`
	AssessmentID  uint64   `bson:"assessment_id"`
	Source        string   `bson:"source"`
	Kind          string   `bson:"kind"`
	Fields        []string `bson:"fields,omitempty"`
	ObservedState string   `bson:"observed_state"`
	Version       string   `bson:"version"`
}

type CatalogDriftPage struct {
	Items      []CatalogDriftItem
	NextCursor string
}

type CatalogRepairPlan struct {
	DryRunID  string
	OrgID     int64
	Item      CatalogDriftItem
	CreatedAt time.Time
	ExpiresAt time.Time
}

type CatalogOutcomeAssociation struct {
	OutcomeID    uint64
	OrgID        int64
	AssessmentID uint64
	TesteeID     uint64
}

type catalogRepairPlanPO struct {
	DryRunID  string           `bson:"dry_run_id"`
	OrgID     int64            `bson:"org_id"`
	Item      CatalogDriftItem `bson:"item"`
	CreatedAt time.Time        `bson:"created_at"`
	ExpiresAt time.Time        `bson:"expires_at"`
}

const catalogRepairPlanCollection = "interpretation_catalog_repair_plans"

func (s *CatalogReconcileStore) ListDrifts(ctx context.Context, filter CatalogReconcileFilter, cursor string, limit int) (CatalogDriftPage, error) {
	if s == nil || s.db == nil {
		return CatalogDriftPage{}, fmt.Errorf("catalog reconcile store is not configured")
	}
	if limit <= 0 || limit > catalogReconcileBatchSize {
		limit = catalogReconcileBatchSize
	}
	after, err := parseCatalogCursor(cursor)
	if err != nil {
		return CatalogDriftPage{}, err
	}
	switch filter.Kind {
	case CatalogDriftMissing:
		return s.listMissing(ctx, filter, after, limit)
	case CatalogDriftDangling, CatalogDriftAssociationMismatch, CatalogDriftWrongWinner:
		return s.listCatalogBased(ctx, filter, after, limit)
	default:
		return CatalogDriftPage{}, fmt.Errorf("unknown catalog drift kind %q", filter.Kind)
	}
}

func (s *CatalogReconcileStore) SaveRepairPlan(ctx context.Context, plan CatalogRepairPlan) error {
	if s == nil || s.db == nil || plan.DryRunID == "" || plan.ExpiresAt.IsZero() {
		return fmt.Errorf("catalog repair plan is invalid")
	}
	_, err := s.db.Collection(catalogRepairPlanCollection).InsertOne(ctx, catalogRepairPlanPO(plan))
	if err != nil {
		return fmt.Errorf("save catalog repair plan: %w", err)
	}
	return nil
}

func (s *CatalogReconcileStore) FindRepairPlan(ctx context.Context, dryRunID string) (CatalogRepairPlan, error) {
	var po catalogRepairPlanPO
	if err := s.db.Collection(catalogRepairPlanCollection).FindOne(ctx, bson.M{"dry_run_id": dryRunID}).Decode(&po); err != nil {
		return CatalogRepairPlan{}, fmt.Errorf("find catalog repair plan: %w", err)
	}
	return CatalogRepairPlan(po), nil
}

func (s *CatalogReconcileStore) ApplyRepair(ctx context.Context, plan CatalogRepairPlan) (string, error) {
	switch plan.Item.Kind {
	case CatalogDriftDangling:
		return "rejected", fmt.Errorf("dangling catalog source requires manual source recovery")
	case CatalogDriftAssociationMismatch:
		return s.repairAssociation(ctx, plan)
	case CatalogDriftMissing, CatalogDriftWrongWinner:
		return s.repairWinner(ctx, plan)
	default:
		return "rejected", fmt.Errorf("unsupported catalog repair kind %q", plan.Item.Kind)
	}
}

// RecoverArchiveAssociation fills only legacy association metadata using the
// committed Evaluation outcome as authority. The immutable report body,
// conclusion and risk fields are never part of the update.
func (s *CatalogReconcileStore) RecoverArchiveAssociation(
	ctx context.Context,
	assessmentID uint64,
	authority CatalogOutcomeAssociation,
) (string, error) {
	if s == nil || s.db == nil || assessmentID == 0 || authority.AssessmentID != assessmentID ||
		authority.OrgID == 0 || authority.OutcomeID == 0 || authority.TesteeID == 0 {
		return "rejected", fmt.Errorf("archive association authority is invalid")
	}
	collection := s.db.Collection((ArchivedReportPO{}).CollectionName())
	var archive ArchivedReportPO
	if err := collection.FindOne(ctx, bson.M{
		"domain_id": assessmentID, "deleted_at": nil,
	}).Decode(&archive); err != nil {
		return "rejected", fmt.Errorf("load archive repair target: %w", err)
	}
	if archive.TesteeID != authority.TesteeID {
		return "rejected", fmt.Errorf("archive testee association conflicts with committed outcome")
	}
	if archive.OutcomeID != 0 && archive.OutcomeID != authority.OutcomeID {
		return "rejected", fmt.Errorf("archive outcome association conflicts with committed outcome")
	}
	if archive.OrgID != nil {
		if *archive.OrgID == authority.OrgID {
			return "already_repaired", nil
		}
		return "rejected", fmt.Errorf("archive organization association conflicts with committed outcome")
	}
	set := bson.M{"org_id": authority.OrgID, "updated_at": time.Now().UTC()}
	if archive.OutcomeID == 0 {
		set["outcome_id"] = authority.OutcomeID
	}
	result, err := collection.UpdateOne(ctx, bson.M{
		"_id": archive.ID, "domain_id": assessmentID, "deleted_at": nil,
		"updated_at": archive.UpdatedAt, "testee_id": archive.TesteeID,
		"outcome_id": archive.OutcomeID,
		"$or":        []bson.M{{"org_id": bson.M{"$exists": false}}, {"org_id": nil}},
	}, bson.M{"$set": set})
	if err != nil {
		return "conflict", fmt.Errorf("recover archive association: %w", err)
	}
	if result.ModifiedCount == 0 {
		return "conflict", fmt.Errorf("archive association CAS conflict")
	}
	return "repaired", nil
}

func (s *CatalogReconcileStore) repairAssociation(ctx context.Context, plan CatalogRepairPlan) (string, error) {
	var entry ReportCatalogPO
	if err := s.db.Collection((ReportCatalogPO{}).CollectionName()).FindOne(ctx, bson.M{
		"assessment_id": plan.Item.AssessmentID,
	}).Decode(&entry); err != nil {
		return "conflict", err
	}
	if strconv.FormatInt(entry.UpdatedAt.UnixNano(), 10) != plan.Item.Version ||
		entry.SourceKind != plan.Item.Source {
		return "conflict", fmt.Errorf("catalog repair version changed")
	}
	sources, err := s.loadCatalogSourceAssociations(ctx, entry.SourceKind, []ReportCatalogPO{entry})
	if err != nil {
		return "rejected", err
	}
	source, ok := sources[entry.SourceID]
	if !ok {
		return "rejected", fmt.Errorf("catalog source is dangling")
	}
	if !source.HasOrgID {
		return "rejected", fmt.Errorf("archive org_id must be recovered from committed outcome before catalog repair")
	}
	set := bson.M{
		"org_id": source.OrgID, "testee_id": source.TesteeID, "updated_at": time.Now().UTC(),
	}
	if source.HasOutcomeID {
		set["outcome_id"] = source.OutcomeID
	}
	if source.HasGenerationID {
		set["generation_id"] = source.GenerationID
	}
	res, err := s.db.Collection((ReportCatalogPO{}).CollectionName()).UpdateOne(ctx,
		bson.M{"assessment_id": entry.AssessmentID, "source_kind": entry.SourceKind, "source_id": entry.SourceID, "updated_at": entry.UpdatedAt},
		bson.M{"$set": set},
	)
	if err != nil {
		return "conflict", err
	}
	if res.ModifiedCount == 0 {
		return "conflict", fmt.Errorf("catalog repair CAS conflict")
	}
	return "repaired", nil
}

func (s *CatalogReconcileStore) repairWinner(ctx context.Context, plan CatalogRepairPlan) (string, error) {
	entry, err := s.latestCatalogCandidate(ctx, plan.Item.AssessmentID)
	if err != nil {
		return "rejected", err
	}
	if entry.OrgID != plan.OrgID {
		return "rejected", fmt.Errorf("catalog repair candidate organization mismatch")
	}
	filter := bson.M{"assessment_id": entry.AssessmentID}
	if plan.Item.Kind == CatalogDriftMissing {
		filter = bson.M{"assessment_id": entry.AssessmentID, "source_kind": bson.M{"$exists": false}}
	}
	_, err = s.db.Collection((ReportCatalogPO{}).CollectionName()).ReplaceOne(ctx, filter, entry, options.Replace().SetUpsert(plan.Item.Kind == CatalogDriftMissing))
	if mongo.IsDuplicateKeyError(err) {
		return "conflict", fmt.Errorf("catalog repair CAS conflict")
	}
	if err != nil {
		return "conflict", err
	}
	return "repaired", nil
}

func (s *CatalogReconcileStore) latestCatalogCandidate(ctx context.Context, assessmentID uint64) (ReportCatalogPO, error) {
	var artifact InterpretReportPO
	err := s.db.Collection((InterpretReportPO{}).CollectionName()).FindOne(ctx,
		bson.M{"assessment_id": assessmentID, "deleted_at": nil},
		options.FindOne().SetSort(bson.D{{Key: "generated_at", Value: -1}, {Key: "domain_id", Value: -1}}),
	).Decode(&artifact)
	if err == nil {
		return ReportCatalogPO{
			AssessmentID: artifact.AssessmentID, OrgID: artifact.OrgID, TesteeID: artifact.TesteeID,
			OutcomeID: artifact.OutcomeID, GenerationID: artifact.GenerationID,
			SourceKind: ReportCatalogSourceArtifact, SourceID: artifact.DomainID.Uint64(),
			ModelCode: artifact.ScaleCode, RiskLevel: artifact.RiskLevel,
			SortAt: artifact.GeneratedAt, SortReportID: artifact.DomainID.Uint64(), UpdatedAt: time.Now().UTC(),
		}, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return ReportCatalogPO{}, err
	}
	var archive ArchivedReportPO
	if err := s.db.Collection((ArchivedReportPO{}).CollectionName()).FindOne(ctx,
		bson.M{"domain_id": assessmentID, "deleted_at": nil},
	).Decode(&archive); err != nil {
		return ReportCatalogPO{}, err
	}
	if archive.OrgID == nil {
		return ReportCatalogPO{}, fmt.Errorf("archive org_id is unproven")
	}
	return ReportCatalogPO{
		AssessmentID: assessmentID, OrgID: *archive.OrgID, TesteeID: archive.TesteeID,
		OutcomeID:  archive.OutcomeID,
		SourceKind: ReportCatalogSourceArchive, SourceID: archive.DomainID.Uint64(),
		ModelCode: archive.ScaleCode, RiskLevel: archive.RiskLevel,
		SortAt: archive.CreatedAt, SortReportID: archive.DomainID.Uint64(), UpdatedAt: time.Now().UTC(),
	}, nil
}

func parseCatalogCursor(cursor string) (uint64, error) {
	if cursor == "" {
		return 0, nil
	}
	value, err := strconv.ParseUint(cursor, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid catalog drift cursor")
	}
	return value, nil
}

func catalogDriftPage(items []CatalogDriftItem, lastScanned uint64, exhausted bool) CatalogDriftPage {
	page := CatalogDriftPage{Items: items}
	if !exhausted && lastScanned != 0 {
		page.NextCursor = strconv.FormatUint(lastScanned, 10)
	}
	return page
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

func NewCatalogReconcileStore(db *mongo.Database) (*CatalogReconcileStore, error) {
	store := &CatalogReconcileStore{db: db}
	if db != nil {
		if _, err := db.Collection(catalogRepairPlanCollection).Indexes().CreateMany(context.Background(), []mongo.IndexModel{
			{Keys: bson.D{{Key: "dry_run_id", Value: 1}}, Options: options.Index().SetName("uk_catalog_repair_dry_run").SetUnique(true)},
			{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetName("ttl_catalog_repair_plan").SetExpireAfterSeconds(0)},
		}); err != nil {
			return nil, fmt.Errorf("ensure catalog repair plan indexes: %w", err)
		}
	}
	return store, nil
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
			options.Find().SetProjection(bson.M{
				"domain_id": 1, "assessment_id": 1, "org_id": 1, "testee_id": 1,
				"outcome_id": 1, "generation_id": 1,
			}),
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
				OutcomeID: po.OutcomeID, HasOutcomeID: po.OutcomeID != 0,
				GenerationID: po.GenerationID, HasGenerationID: po.GenerationID != 0,
			}
		}
		return sources, cur.Err()
	case ReportCatalogSourceArchive:
		cur, err := s.db.Collection((ArchivedReportPO{}).CollectionName()).Find(
			ctx,
			bson.M{"domain_id": bson.M{"$in": ids}, "deleted_at": nil},
			options.Find().SetProjection(bson.M{"domain_id": 1, "org_id": 1, "testee_id": 1, "outcome_id": 1}),
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
			source := CatalogSourceAssociation{
				AssessmentID: po.DomainID.Uint64(), TesteeID: po.TesteeID,
				OutcomeID: po.OutcomeID, HasOutcomeID: po.OutcomeID != 0,
			}
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

func (s *CatalogReconcileStore) listCatalogBased(
	ctx context.Context,
	filter CatalogReconcileFilter,
	after uint64,
	limit int,
) (CatalogDriftPage, error) {
	items := make([]CatalogDriftItem, 0, limit)
	lastScanned := after
	for len(items) < limit {
		query := catalogMatchStage(filter)
		if filter.AssessmentID == nil && lastScanned != 0 {
			query["assessment_id"] = bson.M{"$gt": lastScanned}
		}
		cur, err := s.db.Collection((ReportCatalogPO{}).CollectionName()).Find(ctx, query,
			options.Find().SetSort(bson.D{{Key: "assessment_id", Value: 1}}).SetLimit(catalogReconcileBatchSize))
		if err != nil {
			return CatalogDriftPage{}, err
		}
		entries := make([]ReportCatalogPO, 0, catalogReconcileBatchSize)
		for cur.Next(ctx) {
			var entry ReportCatalogPO
			if err := cur.Decode(&entry); err != nil {
				_ = cur.Close(ctx)
				return CatalogDriftPage{}, err
			}
			entries = append(entries, entry)
		}
		if err := cur.Err(); err != nil {
			_ = cur.Close(ctx)
			return CatalogDriftPage{}, err
		}
		_ = cur.Close(ctx)
		if len(entries) == 0 {
			return catalogDriftPage(items, lastScanned, true), nil
		}
		lastScanned = entries[len(entries)-1].AssessmentID

		byKind := map[string][]ReportCatalogPO{}
		for _, entry := range entries {
			byKind[entry.SourceKind] = append(byKind[entry.SourceKind], entry)
		}
		sources := map[string]map[uint64]CatalogSourceAssociation{}
		for kind, group := range byKind {
			loaded, err := s.loadCatalogSourceAssociations(ctx, kind, group)
			if err != nil {
				return CatalogDriftPage{}, err
			}
			sources[kind] = loaded
		}
		var latest map[uint64]uint64
		if filter.Kind == CatalogDriftWrongWinner {
			latest, err = s.latestArtifactIDs(ctx, entries)
			if err != nil {
				return CatalogDriftPage{}, err
			}
		}
		for _, entry := range entries {
			source, found := sources[entry.SourceKind][entry.SourceID]
			fields := []string(nil)
			matched := false
			switch filter.Kind {
			case CatalogDriftDangling:
				matched = !found
			case CatalogDriftAssociationMismatch:
				if found {
					fields = MismatchedAssociationFields(entry, source)
					matched = len(fields) > 0
				}
			case CatalogDriftWrongWinner:
				winner := latest[entry.AssessmentID]
				matched = winner != 0 && (entry.SourceKind != ReportCatalogSourceArtifact || entry.SourceID != winner)
			}
			if !matched {
				continue
			}
			items = append(items, CatalogDriftItem{
				CatalogID:    strconv.FormatUint(entry.AssessmentID, 10),
				ReportID:     strconv.FormatUint(entry.SourceID, 10),
				AssessmentID: entry.AssessmentID, Source: entry.SourceKind, Kind: filter.Kind,
				Fields:        fields,
				ObservedState: fmt.Sprintf("source=%s/%d", entry.SourceKind, entry.SourceID),
				Version:       strconv.FormatInt(entry.UpdatedAt.UnixNano(), 10),
			})
			if len(items) == limit {
				return catalogDriftPage(items, lastScanned, false), nil
			}
		}
		if len(entries) < catalogReconcileBatchSize || filter.AssessmentID != nil {
			return catalogDriftPage(items, lastScanned, true), nil
		}
	}
	return catalogDriftPage(items, lastScanned, false), nil
}

func (s *CatalogReconcileStore) latestArtifactIDs(ctx context.Context, entries []ReportCatalogPO) (map[uint64]uint64, error) {
	assessmentIDs := make([]uint64, 0, len(entries))
	for _, entry := range entries {
		assessmentIDs = append(assessmentIDs, entry.AssessmentID)
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"assessment_id": bson.M{"$in": assessmentIDs}, "deleted_at": nil}}},
		{{Key: "$sort", Value: bson.D{{Key: "assessment_id", Value: 1}, {Key: "generated_at", Value: -1}, {Key: "domain_id", Value: -1}}}},
		{{Key: "$group", Value: bson.M{"_id": "$assessment_id", "report_id": bson.M{"$first": "$domain_id"}}}},
	}
	cur, err := s.db.Collection((InterpretReportPO{}).CollectionName()).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()
	out := make(map[uint64]uint64, len(entries))
	for cur.Next(ctx) {
		var row struct {
			AssessmentID uint64 `bson:"_id"`
			ReportID     uint64 `bson:"report_id"`
		}
		if err := cur.Decode(&row); err != nil {
			return nil, err
		}
		out[row.AssessmentID] = row.ReportID
	}
	return out, cur.Err()
}

func (s *CatalogReconcileStore) listMissing(
	ctx context.Context,
	filter CatalogReconcileFilter,
	after uint64,
	limit int,
) (CatalogDriftPage, error) {
	artifactMatch := bson.M{"deleted_at": nil}
	archiveMatch := bson.M{"deleted_at": nil, "org_id": bson.M{"$ne": nil}}
	if filter.OrgID != nil {
		artifactMatch["org_id"] = *filter.OrgID
		archiveMatch["org_id"] = *filter.OrgID
	}
	if filter.AssessmentID != nil {
		artifactMatch["assessment_id"] = *filter.AssessmentID
		archiveMatch["domain_id"] = *filter.AssessmentID
	} else if after != 0 {
		artifactMatch["assessment_id"] = bson.M{"$gt": after}
		archiveMatch["domain_id"] = bson.M{"$gt": after}
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: artifactMatch}},
		{{Key: "$project", Value: bson.M{
			"assessment_id": 1, "report_id": "$domain_id", "source": bson.M{"$literal": ReportCatalogSourceArtifact},
			"priority": bson.M{"$literal": 2}, "sort_at": "$generated_at",
		}}},
		{{Key: "$unionWith", Value: bson.M{"coll": (ArchivedReportPO{}).CollectionName(), "pipeline": mongo.Pipeline{
			{{Key: "$match", Value: archiveMatch}},
			{{Key: "$project", Value: bson.M{
				"assessment_id": "$domain_id", "report_id": "$domain_id", "source": bson.M{"$literal": ReportCatalogSourceArchive},
				"priority": bson.M{"$literal": 1}, "sort_at": "$created_at",
			}}},
		}}}},
		{{Key: "$sort", Value: bson.D{{Key: "assessment_id", Value: 1}, {Key: "priority", Value: -1}, {Key: "sort_at", Value: -1}, {Key: "report_id", Value: -1}}}},
		{{Key: "$group", Value: bson.M{
			"_id": "$assessment_id", "report_id": bson.M{"$first": "$report_id"}, "source": bson.M{"$first": "$source"},
		}}},
		{{Key: "$lookup", Value: bson.M{
			"from": (ReportCatalogPO{}).CollectionName(), "localField": "_id", "foreignField": "assessment_id", "as": "catalog",
		}}},
		{{Key: "$match", Value: bson.M{"catalog": bson.M{"$size": 0}}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		{{Key: "$limit", Value: limit}},
	}
	cur, err := s.db.Collection((InterpretReportPO{}).CollectionName()).Aggregate(ctx, pipeline, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return CatalogDriftPage{}, err
	}
	defer func() { _ = cur.Close(ctx) }()
	items := make([]CatalogDriftItem, 0, limit)
	var last uint64
	for cur.Next(ctx) {
		var row struct {
			AssessmentID uint64 `bson:"_id"`
			ReportID     uint64 `bson:"report_id"`
			Source       string `bson:"source"`
		}
		if err := cur.Decode(&row); err != nil {
			return CatalogDriftPage{}, err
		}
		last = row.AssessmentID
		items = append(items, CatalogDriftItem{
			CatalogID: strconv.FormatUint(row.AssessmentID, 10), ReportID: strconv.FormatUint(row.ReportID, 10),
			AssessmentID: row.AssessmentID, Source: row.Source, Kind: CatalogDriftMissing,
			ObservedState: "catalog=missing", Version: "missing",
		})
	}
	if err := cur.Err(); err != nil {
		return CatalogDriftPage{}, err
	}
	return catalogDriftPage(items, last, len(items) < limit), nil
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
	if filter.AssessmentID != nil {
		match["assessment_id"] = *filter.AssessmentID
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
