package interpretation

import (
	"context"
	"fmt"
	"strconv"
	"time"

	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
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
	_, err := s.repairPlans.InsertOne(ctx, catalogRepairPlanPO(plan))
	if err != nil {
		return fmt.Errorf("save catalog repair plan: %w", err)
	}
	return nil
}

func (s *CatalogReconcileStore) FindRepairPlan(ctx context.Context, dryRunID string) (CatalogRepairPlan, error) {
	var po catalogRepairPlanPO
	if err := s.repairPlans.FindOne(ctx, bson.M{"dry_run_id": dryRunID}, &po); err != nil {
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

func (s *CatalogReconcileStore) repairAssociation(ctx context.Context, plan CatalogRepairPlan) (string, error) {
	var entry ReportCatalogPO
	if err := s.catalog.FindOne(ctx, bson.M{
		"assessment_id": plan.Item.AssessmentID,
	}, &entry); err != nil {
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
		return "rejected", fmt.Errorf("catalog source organization is unproven")
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
	res, err := s.catalog.UpdateOne(ctx,
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
	_, err = s.catalog.ReplaceOne(ctx, filter, entry, options.Replace().SetUpsert(plan.Item.Kind == CatalogDriftMissing))
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
	err := s.reports.FindOne(ctx,
		bson.M{"assessment_id": assessmentID, "deleted_at": nil},
		&artifact,
		options.FindOne().SetSort(bson.D{{Key: "generated_at", Value: -1}, {Key: "domain_id", Value: -1}}),
	)
	if err == nil {
		return ReportCatalogPO{
			AssessmentID: artifact.AssessmentID, OrgID: artifact.OrgID, TesteeID: artifact.TesteeID,
			OutcomeID: artifact.OutcomeID, GenerationID: artifact.GenerationID,
			SourceKind: ReportCatalogSourceArtifact, SourceID: artifact.DomainID.Uint64(),
			ModelCode: artifact.ScaleCode, RiskLevel: artifact.RiskLevel,
			SortAt: artifact.GeneratedAt, SortReportID: artifact.DomainID.Uint64(), UpdatedAt: time.Now().UTC(),
		}, nil
	}
	return ReportCatalogPO{}, err
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
	Missing             int64 `bson:"missing"`
	Dangling            int64 `bson:"dangling"`
	AssociationMismatch int64 `bson:"association_mismatch"`
	WrongWinner         int64 `bson:"wrong_winner"`
}

func (c CatalogDriftCounts) Total() int64 {
	return c.Missing + c.Dangling + c.AssociationMismatch + c.WrongWinner
}

// CatalogReconcileStore performs read-only catalog drift detection against Mongo.
type CatalogReconcileStore struct {
	db          *mongo.Database
	repairPlans base.BaseRepository
	catalog     base.BaseRepository
	reports     base.BaseRepository
}

func NewCatalogReconcileStore(db *mongo.Database, opts ...base.BaseRepositoryOptions) (*CatalogReconcileStore, error) {
	store := &CatalogReconcileStore{db: db}
	if db != nil {
		store.repairPlans = base.NewBaseRepository(db, catalogRepairPlanCollection, opts...)
		store.catalog = base.NewBaseRepository(db, (ReportCatalogPO{}).CollectionName(), opts...)
		store.reports = base.NewBaseRepository(db, (InterpretReportPO{}).CollectionName(), opts...)
		if _, err := db.Collection(catalogRepairPlanCollection).Indexes().CreateMany(context.Background(), []mongo.IndexModel{
			{Keys: bson.D{{Key: "dry_run_id", Value: 1}}, Options: options.Index().SetName("uk_catalog_repair_dry_run").SetUnique(true)},
			{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetName("ttl_catalog_repair_plan").SetExpireAfterSeconds(0)},
		}); err != nil {
			return nil, fmt.Errorf("ensure catalog repair plan indexes: %w", err)
		}
	}
	return store, nil
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
	if sourceKind != ReportCatalogSourceArtifact {
		return nil, fmt.Errorf("unknown report catalog source %q", sourceKind)
	}
	cur, err := s.reports.Find(
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

func (s *CatalogReconcileStore) listCatalogBased(
	ctx context.Context,
	filter CatalogReconcileFilter,
	after uint64,
	limit int,
) (CatalogDriftPage, error) {
	query := catalogMatchStage(filter)
	if filter.AssessmentID == nil && after != 0 {
		query["assessment_id"] = bson.M{"$gt": after}
	}
	cur, err := s.db.Collection((ReportCatalogPO{}).CollectionName()).Find(ctx, query,
		options.Find().SetSort(bson.D{{Key: "assessment_id", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return CatalogDriftPage{}, err
	}
	entries := make([]ReportCatalogPO, 0, limit)
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
		return CatalogDriftPage{Items: []CatalogDriftItem{}}, nil
	}
	lastScanned := entries[len(entries)-1].AssessmentID
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
	items := make([]CatalogDriftItem, 0, len(entries))
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
		if matched {
			items = append(items, CatalogDriftItem{
				CatalogID: strconv.FormatUint(entry.AssessmentID, 10), ReportID: strconv.FormatUint(entry.SourceID, 10),
				AssessmentID: entry.AssessmentID, Source: entry.SourceKind, Kind: filter.Kind, Fields: fields,
				ObservedState: fmt.Sprintf("source=%s/%d", entry.SourceKind, entry.SourceID), Version: strconv.FormatInt(entry.UpdatedAt.UnixNano(), 10),
			})
		}
	}
	return catalogDriftPage(items, lastScanned, filter.AssessmentID != nil), nil
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
	if filter.OrgID != nil {
		artifactMatch["org_id"] = *filter.OrgID
	}
	if filter.AssessmentID != nil {
		artifactMatch["assessment_id"] = *filter.AssessmentID
	} else if after != 0 {
		artifactMatch["assessment_id"] = bson.M{"$gt": after}
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: artifactMatch}},
		{{Key: "$sort", Value: bson.D{{Key: "assessment_id", Value: 1}, {Key: "generated_at", Value: -1}, {Key: "domain_id", Value: -1}}}},
		{{Key: "$limit", Value: limit}},
		{{Key: "$project", Value: bson.M{
			"assessment_id": 1, "report_id": "$domain_id", "source": bson.M{"$literal": ReportCatalogSourceArtifact},
			"priority": bson.M{"$literal": 2}, "sort_at": "$generated_at",
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "assessment_id", Value: 1}, {Key: "priority", Value: -1}, {Key: "sort_at", Value: -1}, {Key: "report_id", Value: -1}}}},
		{{Key: "$group", Value: bson.M{
			"_id": "$assessment_id", "report_id": bson.M{"$first": "$report_id"}, "source": bson.M{"$first": "$source"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		{{Key: "$limit", Value: limit}},
		{{Key: "$lookup", Value: bson.M{
			"from": (ReportCatalogPO{}).CollectionName(), "localField": "_id", "foreignField": "assessment_id", "as": "catalog",
		}}},
	}
	cur, err := s.db.Collection((InterpretReportPO{}).CollectionName()).Aggregate(ctx, pipeline)
	if err != nil {
		return CatalogDriftPage{}, err
	}
	defer func() { _ = cur.Close(ctx) }()
	items := make([]CatalogDriftItem, 0, limit)
	var last uint64
	rows := 0
	for cur.Next(ctx) {
		var row struct {
			AssessmentID uint64     `bson:"_id"`
			ReportID     uint64     `bson:"report_id"`
			Source       string     `bson:"source"`
			Catalog      []bson.Raw `bson:"catalog"`
		}
		if err := cur.Decode(&row); err != nil {
			return CatalogDriftPage{}, err
		}
		rows++
		last = row.AssessmentID
		if len(row.Catalog) > 0 {
			continue
		}
		items = append(items, CatalogDriftItem{
			CatalogID: strconv.FormatUint(row.AssessmentID, 10), ReportID: strconv.FormatUint(row.ReportID, 10),
			AssessmentID: row.AssessmentID, Source: row.Source, Kind: CatalogDriftMissing,
			ObservedState: "catalog=missing", Version: "missing",
		})
	}
	if err := cur.Err(); err != nil {
		return CatalogDriftPage{}, err
	}
	if rows == 0 {
		return CatalogDriftPage{Items: []CatalogDriftItem{}}, nil
	}
	return catalogDriftPage(items, last, filter.AssessmentID != nil), nil
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

// HasAssociationMismatch reports whether catalog and source disagree under IR-R002 rules.
func HasAssociationMismatch(catalog ReportCatalogPO, source CatalogSourceAssociation) bool {
	return len(MismatchedAssociationFields(catalog, source)) > 0
}
