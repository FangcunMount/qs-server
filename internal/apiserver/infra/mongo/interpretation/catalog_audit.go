package interpretation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	CatalogAuditCheckpointCollection = "interpretation_catalog_audit_checkpoints"
	CatalogAuditCheckpointID         = "report_catalog"
	CatalogAuditCheckpointSchema     = 1
	CatalogAuditPhaseMissing         = "missing_sources"
	CatalogAuditPhaseCatalog         = "catalog_entries"

	IndexCatalogAuditArtifact = "idx_interpret_report_audit_active_assessment_winner"
	IndexCatalogAuditArchive  = "idx_archived_report_audit_active_org_domain"
)

var (
	ErrCatalogAuditCheckpointMissing = errors.New("catalog audit checkpoint is missing")
	ErrCatalogAuditCheckpointCAS     = errors.New("catalog audit checkpoint CAS conflict")
)

type CatalogCompletedAuditSnapshot struct {
	CycleID     string
	CompletedAt time.Time
	Counts      CatalogDriftCounts
	OrgCounts   map[int64]CatalogDriftCounts
}

type CatalogAuditCheckpoint struct {
	SchemaVersion            int
	Revision                 int64
	CycleID                  string
	Phase                    string
	AfterAssessmentID        uint64
	SourceUpperAssessmentID  uint64
	CatalogUpperAssessmentID uint64
	WorkingCounts            CatalogDriftCounts
	WorkingOrgCounts         map[int64]CatalogDriftCounts
	LastCompleted            *CatalogCompletedAuditSnapshot
	NextCycleAt              time.Time
	UpdatedAt                time.Time
}

type CatalogAuditUpperBounds struct {
	SourceAssessmentID  uint64
	CatalogAssessmentID uint64
}

type CatalogAuditBatchRequest struct {
	Phase             string
	AfterAssessmentID uint64
	UpperAssessmentID uint64
	Limit             int
	MaxTime           time.Duration
}

type CatalogAuditBatchResult struct {
	NextAssessmentID uint64
	Scanned          int
	Exhausted        bool
	Counts           CatalogDriftCounts
	OrgCounts        map[int64]CatalogDriftCounts
}

type catalogAuditSnapshotPO struct {
	CycleID     string                        `bson:"cycle_id"`
	CompletedAt time.Time                     `bson:"completed_at"`
	Counts      CatalogDriftCounts            `bson:"counts"`
	OrgCounts   map[string]CatalogDriftCounts `bson:"org_counts"`
}

type catalogAuditCheckpointPO struct {
	ID                       string                        `bson:"_id"`
	SchemaVersion            int                           `bson:"schema_version"`
	Revision                 int64                         `bson:"revision"`
	CycleID                  string                        `bson:"cycle_id"`
	Phase                    string                        `bson:"phase"`
	AfterAssessmentID        uint64                        `bson:"after_assessment_id"`
	SourceUpperAssessmentID  uint64                        `bson:"source_upper_assessment_id"`
	CatalogUpperAssessmentID uint64                        `bson:"catalog_upper_assessment_id"`
	WorkingCounts            CatalogDriftCounts            `bson:"working_counts"`
	WorkingOrgCounts         map[string]CatalogDriftCounts `bson:"working_org_counts"`
	LastCompleted            *catalogAuditSnapshotPO       `bson:"last_completed,omitempty"`
	NextCycleAt              time.Time                     `bson:"next_cycle_at,omitempty"`
	UpdatedAt                time.Time                     `bson:"updated_at"`
}

func (s *CatalogReconcileStore) VerifyAuditIndexes(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("catalog audit store is not configured")
	}
	required := []struct {
		collection string
		index      string
	}{
		{(ReportCatalogPO{}).CollectionName(), "uk_report_catalog_assessment"},
		{(InterpretReportPO{}).CollectionName(), IndexCatalogAuditArtifact},
		{(ArchivedReportPO{}).CollectionName(), IndexCatalogAuditArchive},
	}
	for _, item := range required {
		present, err := listReportCatalogIndexNames(ctx, s.db.Collection(item.collection))
		if err != nil {
			return fmt.Errorf("list %s indexes: %w", item.collection, err)
		}
		if !present[item.index] {
			return fmt.Errorf("required audit index %s.%s is missing; run Mongo migration 000021", item.collection, item.index)
		}
	}
	return nil
}

func (s *CatalogReconcileStore) LoadAuditCheckpoint(ctx context.Context) (CatalogAuditCheckpoint, error) {
	if s == nil || s.db == nil {
		return CatalogAuditCheckpoint{}, fmt.Errorf("catalog audit store is not configured")
	}
	var po catalogAuditCheckpointPO
	if err := s.db.Collection(CatalogAuditCheckpointCollection).FindOne(ctx, bson.M{"_id": CatalogAuditCheckpointID}).Decode(&po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return CatalogAuditCheckpoint{}, ErrCatalogAuditCheckpointMissing
		}
		return CatalogAuditCheckpoint{}, err
	}
	if po.SchemaVersion != CatalogAuditCheckpointSchema {
		return CatalogAuditCheckpoint{}, fmt.Errorf("unsupported catalog audit checkpoint schema version %d", po.SchemaVersion)
	}
	return checkpointFromPO(po), nil
}

func (s *CatalogReconcileStore) SaveAuditCheckpoint(ctx context.Context, expectedRevision int64, checkpoint CatalogAuditCheckpoint) error {
	if s == nil || s.db == nil || checkpoint.Revision != expectedRevision+1 {
		return fmt.Errorf("catalog audit checkpoint revision is invalid")
	}
	po := checkpointToPO(checkpoint)
	collection := s.db.Collection(CatalogAuditCheckpointCollection)
	if expectedRevision == 0 {
		_, err := collection.InsertOne(ctx, po)
		if mongo.IsDuplicateKeyError(err) {
			return ErrCatalogAuditCheckpointCAS
		}
		return err
	}
	result, err := collection.ReplaceOne(ctx, bson.M{"_id": CatalogAuditCheckpointID, "revision": expectedRevision}, po)
	if err != nil {
		return err
	}
	if result.ModifiedCount != 1 {
		return ErrCatalogAuditCheckpointCAS
	}
	return nil
}

func (s *CatalogReconcileStore) LoadAuditUpperBounds(ctx context.Context, maxTime time.Duration) (CatalogAuditUpperBounds, error) {
	if s == nil || s.db == nil {
		return CatalogAuditUpperBounds{}, fmt.Errorf("catalog audit store is not configured")
	}
	artifact, err := maxUint64Field(ctx, s.db.Collection((InterpretReportPO{}).CollectionName()), "assessment_id", bson.M{"deleted_at": nil}, maxTime)
	if err != nil {
		return CatalogAuditUpperBounds{}, err
	}
	archive, err := maxUint64Field(ctx, s.db.Collection((ArchivedReportPO{}).CollectionName()), "domain_id", bson.M{"deleted_at": nil, "org_id": bson.M{"$ne": nil}}, maxTime)
	if err != nil {
		return CatalogAuditUpperBounds{}, err
	}
	catalog, err := maxUint64Field(ctx, s.db.Collection((ReportCatalogPO{}).CollectionName()), "assessment_id", bson.M{}, maxTime)
	if err != nil {
		return CatalogAuditUpperBounds{}, err
	}
	return CatalogAuditUpperBounds{SourceAssessmentID: max(artifact, archive), CatalogAssessmentID: catalog}, nil
}

func maxUint64Field(ctx context.Context, collection *mongo.Collection, field string, filter bson.M, maxTime time.Duration) (uint64, error) {
	var row bson.M
	err := collection.FindOne(ctx, filter, options.FindOne().
		SetSort(bson.D{{Key: field, Value: -1}}).
		SetProjection(bson.M{field: 1}).
		SetMaxTime(maxTime)).Decode(&row)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	value, ok := numericUint64(row[field])
	if !ok {
		return 0, fmt.Errorf("invalid %s upper bound", field)
	}
	return value, nil
}

func (s *CatalogReconcileStore) ScanAuditBatch(ctx context.Context, request CatalogAuditBatchRequest) (CatalogAuditBatchResult, error) {
	if request.Limit <= 0 || request.MaxTime <= 0 {
		return CatalogAuditBatchResult{}, fmt.Errorf("catalog audit batch request is invalid")
	}
	switch request.Phase {
	case CatalogAuditPhaseMissing:
		return s.scanMissingSources(ctx, request)
	case CatalogAuditPhaseCatalog:
		return s.scanCatalogEntries(ctx, request)
	default:
		return CatalogAuditBatchResult{}, fmt.Errorf("unknown catalog audit phase %q", request.Phase)
	}
}

type catalogSourceCandidate struct {
	AssessmentID uint64
	ReportID     uint64
	OrgID        int64
	Source       string
	SortAt       time.Time
}

func (s *CatalogReconcileStore) scanMissingSources(ctx context.Context, request CatalogAuditBatchRequest) (CatalogAuditBatchResult, error) {
	artifactRows, artifactFull, err := s.loadArtifactCandidates(ctx, request)
	if err != nil {
		return CatalogAuditBatchResult{}, err
	}
	archiveRows, archiveFull, err := s.loadArchiveCandidates(ctx, request)
	if err != nil {
		return CatalogAuditBatchResult{}, err
	}
	byAssessment := make(map[uint64]catalogSourceCandidate, len(artifactRows)+len(archiveRows))
	for _, candidate := range archiveRows {
		byAssessment[candidate.AssessmentID] = candidate
	}
	for _, candidate := range artifactRows {
		current, exists := byAssessment[candidate.AssessmentID]
		if !exists || current.Source != ReportCatalogSourceArtifact || candidate.SortAt.After(current.SortAt) ||
			(candidate.SortAt.Equal(current.SortAt) && candidate.ReportID > current.ReportID) {
			byAssessment[candidate.AssessmentID] = candidate
		}
	}
	ids := make([]uint64, 0, len(byAssessment))
	for assessmentID := range byAssessment {
		ids = append(ids, assessmentID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) > request.Limit {
		ids = ids[:request.Limit]
	}
	if err := s.promoteArchiveCandidatesWithArtifacts(ctx, ids, byAssessment, request.MaxTime); err != nil {
		return CatalogAuditBatchResult{}, err
	}
	catalogExists, err := s.catalogAssessmentSet(ctx, ids, request.MaxTime)
	if err != nil {
		return CatalogAuditBatchResult{}, err
	}
	result := CatalogAuditBatchResult{Scanned: len(ids), OrgCounts: make(map[int64]CatalogDriftCounts)}
	for _, assessmentID := range ids {
		if catalogExists[assessmentID] {
			continue
		}
		candidate := byAssessment[assessmentID]
		result.Counts.Missing++
		orgCounts := result.OrgCounts[candidate.OrgID]
		orgCounts.Missing++
		result.OrgCounts[candidate.OrgID] = orgCounts
	}
	if len(ids) > 0 {
		result.NextAssessmentID = ids[len(ids)-1]
	}
	result.Exhausted = len(ids) == 0 || (!artifactFull && !archiveFull && len(byAssessment) <= request.Limit)
	return result, nil
}

func (s *CatalogReconcileStore) loadArtifactCandidates(ctx context.Context, request CatalogAuditBatchRequest) ([]catalogSourceCandidate, bool, error) {
	filter := auditRange("assessment_id", request)
	filter["deleted_at"] = nil
	cursor, err := s.db.Collection((InterpretReportPO{}).CollectionName()).Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "assessment_id", Value: 1}, {Key: "generated_at", Value: -1}, {Key: "domain_id", Value: -1}}).
		SetLimit(int64(request.Limit)).SetMaxTime(request.MaxTime).
		SetProjection(bson.M{"domain_id": 1, "assessment_id": 1, "org_id": 1, "generated_at": 1}))
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	rows := make([]catalogSourceCandidate, 0, request.Limit)
	for cursor.Next(ctx) {
		var po InterpretReportPO
		if err := cursor.Decode(&po); err != nil {
			return nil, false, err
		}
		rows = append(rows, catalogSourceCandidate{AssessmentID: po.AssessmentID, ReportID: po.DomainID.Uint64(), OrgID: po.OrgID, Source: ReportCatalogSourceArtifact, SortAt: po.GeneratedAt})
	}
	return rows, len(rows) == request.Limit, cursor.Err()
}

func (s *CatalogReconcileStore) loadArchiveCandidates(ctx context.Context, request CatalogAuditBatchRequest) ([]catalogSourceCandidate, bool, error) {
	filter := auditRange("domain_id", request)
	filter["deleted_at"] = nil
	filter["org_id"] = bson.M{"$ne": nil}
	cursor, err := s.db.Collection((ArchivedReportPO{}).CollectionName()).Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "domain_id", Value: 1}}).SetLimit(int64(request.Limit)).SetMaxTime(request.MaxTime).
		SetProjection(bson.M{"domain_id": 1, "org_id": 1, "created_at": 1}))
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	rows := make([]catalogSourceCandidate, 0, request.Limit)
	for cursor.Next(ctx) {
		var po ArchivedReportPO
		if err := cursor.Decode(&po); err != nil {
			return nil, false, err
		}
		if po.OrgID != nil {
			rows = append(rows, catalogSourceCandidate{AssessmentID: po.DomainID.Uint64(), ReportID: po.DomainID.Uint64(), OrgID: *po.OrgID, Source: ReportCatalogSourceArchive, SortAt: po.CreatedAt})
		}
	}
	return rows, len(rows) == request.Limit, cursor.Err()
}

func (s *CatalogReconcileStore) promoteArchiveCandidatesWithArtifacts(ctx context.Context, ids []uint64, candidates map[uint64]catalogSourceCandidate, maxTime time.Duration) error {
	archiveIDs := make([]uint64, 0, len(ids))
	for _, assessmentID := range ids {
		if candidates[assessmentID].Source == ReportCatalogSourceArchive {
			archiveIDs = append(archiveIDs, assessmentID)
		}
	}
	if len(archiveIDs) == 0 {
		return nil
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"assessment_id": bson.M{"$in": archiveIDs}, "deleted_at": nil}}},
		{{Key: "$sort", Value: bson.D{{Key: "assessment_id", Value: 1}, {Key: "generated_at", Value: -1}, {Key: "domain_id", Value: -1}}}},
		{{Key: "$group", Value: bson.M{"_id": "$assessment_id", "report_id": bson.M{"$first": "$domain_id"}, "org_id": bson.M{"$first": "$org_id"}, "sort_at": bson.M{"$first": "$generated_at"}}}},
	}
	cursor, err := s.db.Collection((InterpretReportPO{}).CollectionName()).Aggregate(ctx, pipeline, options.Aggregate().SetMaxTime(maxTime))
	if err != nil {
		return err
	}
	defer func() { _ = cursor.Close(ctx) }()
	for cursor.Next(ctx) {
		var row struct {
			AssessmentID uint64    `bson:"_id"`
			ReportID     uint64    `bson:"report_id"`
			OrgID        int64     `bson:"org_id"`
			SortAt       time.Time `bson:"sort_at"`
		}
		if err := cursor.Decode(&row); err != nil {
			return err
		}
		candidates[row.AssessmentID] = catalogSourceCandidate{AssessmentID: row.AssessmentID, ReportID: row.ReportID, OrgID: row.OrgID, Source: ReportCatalogSourceArtifact, SortAt: row.SortAt}
	}
	return cursor.Err()
}

func (s *CatalogReconcileStore) catalogAssessmentSet(ctx context.Context, ids []uint64, maxTime time.Duration) (map[uint64]bool, error) {
	result := make(map[uint64]bool, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	cursor, err := s.db.Collection((ReportCatalogPO{}).CollectionName()).Find(ctx, bson.M{"assessment_id": bson.M{"$in": ids}}, options.Find().SetProjection(bson.M{"assessment_id": 1}).SetMaxTime(maxTime))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	for cursor.Next(ctx) {
		var row struct {
			AssessmentID uint64 `bson:"assessment_id"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		result[row.AssessmentID] = true
	}
	return result, cursor.Err()
}

func (s *CatalogReconcileStore) scanCatalogEntries(ctx context.Context, request CatalogAuditBatchRequest) (CatalogAuditBatchResult, error) {
	filter := auditRange("assessment_id", request)
	cursor, err := s.db.Collection((ReportCatalogPO{}).CollectionName()).Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "assessment_id", Value: 1}}).SetLimit(int64(request.Limit)).SetMaxTime(request.MaxTime))
	if err != nil {
		return CatalogAuditBatchResult{}, err
	}
	entries := make([]ReportCatalogPO, 0, request.Limit)
	for cursor.Next(ctx) {
		var entry ReportCatalogPO
		if err := cursor.Decode(&entry); err != nil {
			_ = cursor.Close(ctx)
			return CatalogAuditBatchResult{}, err
		}
		entries = append(entries, entry)
	}
	if err := cursor.Err(); err != nil {
		_ = cursor.Close(ctx)
		return CatalogAuditBatchResult{}, err
	}
	_ = cursor.Close(ctx)
	result := CatalogAuditBatchResult{Scanned: len(entries), Exhausted: len(entries) < request.Limit, OrgCounts: make(map[int64]CatalogDriftCounts)}
	if len(entries) == 0 {
		return result, nil
	}
	result.NextAssessmentID = entries[len(entries)-1].AssessmentID

	byKind := make(map[string][]ReportCatalogPO)
	for _, entry := range entries {
		byKind[entry.SourceKind] = append(byKind[entry.SourceKind], entry)
	}
	sources := make(map[string]map[uint64]CatalogSourceAssociation)
	for kind, group := range byKind {
		loaded, err := s.loadAuditSourceAssociations(ctx, kind, group, request.MaxTime)
		if err != nil {
			return CatalogAuditBatchResult{}, err
		}
		sources[kind] = loaded
	}
	latest, err := s.latestAuditArtifactIDs(ctx, entries, request.MaxTime)
	if err != nil {
		return CatalogAuditBatchResult{}, err
	}
	for _, entry := range entries {
		source, found := sources[entry.SourceKind][entry.SourceID]
		counts := classifyCatalogAuditEntry(entry, source, found, latest[entry.AssessmentID])
		result.Counts = addCatalogDriftCounts(result.Counts, counts)
		result.OrgCounts[entry.OrgID] = addCatalogDriftCounts(result.OrgCounts[entry.OrgID], counts)
	}
	return result, nil
}

func classifyCatalogAuditEntry(entry ReportCatalogPO, source CatalogSourceAssociation, found bool, latestArtifactID uint64) CatalogDriftCounts {
	counts := CatalogDriftCounts{}
	if !found {
		counts.Dangling++
	} else if HasAssociationMismatch(entry, source) {
		counts.AssociationMismatch++
	}
	if latestArtifactID != 0 && (entry.SourceKind != ReportCatalogSourceArtifact || entry.SourceID != latestArtifactID) {
		counts.WrongWinner++
	}
	return counts
}

func (s *CatalogReconcileStore) loadAuditSourceAssociations(ctx context.Context, sourceKind string, entries []ReportCatalogPO, maxTime time.Duration) (map[uint64]CatalogSourceAssociation, error) {
	ids := make([]uint64, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.SourceID)
	}
	sources := make(map[uint64]CatalogSourceAssociation, len(ids))
	collection := ""
	projection := bson.M{}
	switch sourceKind {
	case ReportCatalogSourceArtifact:
		collection = (InterpretReportPO{}).CollectionName()
		projection = bson.M{"domain_id": 1, "assessment_id": 1, "org_id": 1, "testee_id": 1, "outcome_id": 1, "generation_id": 1}
	case ReportCatalogSourceArchive:
		collection = (ArchivedReportPO{}).CollectionName()
		projection = bson.M{"domain_id": 1, "org_id": 1, "testee_id": 1, "outcome_id": 1}
	default:
		return sources, nil
	}
	cursor, err := s.db.Collection(collection).Find(ctx, bson.M{"domain_id": bson.M{"$in": ids}, "deleted_at": nil}, options.Find().SetProjection(projection).SetMaxTime(maxTime))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	for cursor.Next(ctx) {
		if sourceKind == ReportCatalogSourceArtifact {
			var po InterpretReportPO
			if err := cursor.Decode(&po); err != nil {
				return nil, err
			}
			sources[po.DomainID.Uint64()] = CatalogSourceAssociation{AssessmentID: po.AssessmentID, OrgID: po.OrgID, HasOrgID: true, TesteeID: po.TesteeID, OutcomeID: po.OutcomeID, HasOutcomeID: po.OutcomeID != 0, GenerationID: po.GenerationID, HasGenerationID: po.GenerationID != 0}
			continue
		}
		var po ArchivedReportPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		source := CatalogSourceAssociation{AssessmentID: po.DomainID.Uint64(), TesteeID: po.TesteeID, OutcomeID: po.OutcomeID, HasOutcomeID: po.OutcomeID != 0}
		if po.OrgID != nil {
			source.OrgID, source.HasOrgID = *po.OrgID, true
		}
		sources[po.DomainID.Uint64()] = source
	}
	return sources, cursor.Err()
}

func (s *CatalogReconcileStore) latestAuditArtifactIDs(ctx context.Context, entries []ReportCatalogPO, maxTime time.Duration) (map[uint64]uint64, error) {
	ids := make([]uint64, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.AssessmentID)
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"assessment_id": bson.M{"$in": ids}, "deleted_at": nil}}},
		{{Key: "$sort", Value: bson.D{{Key: "assessment_id", Value: 1}, {Key: "generated_at", Value: -1}, {Key: "domain_id", Value: -1}}}},
		{{Key: "$group", Value: bson.M{"_id": "$assessment_id", "report_id": bson.M{"$first": "$domain_id"}}}},
	}
	cursor, err := s.db.Collection((InterpretReportPO{}).CollectionName()).Aggregate(ctx, pipeline, options.Aggregate().SetMaxTime(maxTime))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	result := make(map[uint64]uint64, len(entries))
	for cursor.Next(ctx) {
		var row struct {
			AssessmentID uint64 `bson:"_id"`
			ReportID     uint64 `bson:"report_id"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		result[row.AssessmentID] = row.ReportID
	}
	return result, cursor.Err()
}

func auditRange(field string, request CatalogAuditBatchRequest) bson.M {
	rangeFilter := bson.M{"$gt": request.AfterAssessmentID, "$lte": request.UpperAssessmentID}
	return bson.M{field: rangeFilter}
}

func addCatalogDriftCounts(left, right CatalogDriftCounts) CatalogDriftCounts {
	return CatalogDriftCounts{Missing: left.Missing + right.Missing, Dangling: left.Dangling + right.Dangling, AssociationMismatch: left.AssociationMismatch + right.AssociationMismatch, WrongWinner: left.WrongWinner + right.WrongWinner}
}

func checkpointToPO(checkpoint CatalogAuditCheckpoint) catalogAuditCheckpointPO {
	po := catalogAuditCheckpointPO{
		ID: CatalogAuditCheckpointID, SchemaVersion: checkpoint.SchemaVersion, Revision: checkpoint.Revision,
		CycleID: checkpoint.CycleID, Phase: checkpoint.Phase, AfterAssessmentID: checkpoint.AfterAssessmentID,
		SourceUpperAssessmentID: checkpoint.SourceUpperAssessmentID, CatalogUpperAssessmentID: checkpoint.CatalogUpperAssessmentID,
		WorkingCounts: checkpoint.WorkingCounts, WorkingOrgCounts: orgCountsToPO(checkpoint.WorkingOrgCounts),
		NextCycleAt: checkpoint.NextCycleAt, UpdatedAt: checkpoint.UpdatedAt,
	}
	if checkpoint.LastCompleted != nil {
		po.LastCompleted = &catalogAuditSnapshotPO{CycleID: checkpoint.LastCompleted.CycleID, CompletedAt: checkpoint.LastCompleted.CompletedAt, Counts: checkpoint.LastCompleted.Counts, OrgCounts: orgCountsToPO(checkpoint.LastCompleted.OrgCounts)}
	}
	return po
}

func checkpointFromPO(po catalogAuditCheckpointPO) CatalogAuditCheckpoint {
	checkpoint := CatalogAuditCheckpoint{
		SchemaVersion: po.SchemaVersion, Revision: po.Revision, CycleID: po.CycleID, Phase: po.Phase,
		AfterAssessmentID: po.AfterAssessmentID, SourceUpperAssessmentID: po.SourceUpperAssessmentID,
		CatalogUpperAssessmentID: po.CatalogUpperAssessmentID, WorkingCounts: po.WorkingCounts,
		WorkingOrgCounts: orgCountsFromPO(po.WorkingOrgCounts), NextCycleAt: po.NextCycleAt, UpdatedAt: po.UpdatedAt,
	}
	if po.LastCompleted != nil {
		checkpoint.LastCompleted = &CatalogCompletedAuditSnapshot{CycleID: po.LastCompleted.CycleID, CompletedAt: po.LastCompleted.CompletedAt, Counts: po.LastCompleted.Counts, OrgCounts: orgCountsFromPO(po.LastCompleted.OrgCounts)}
	}
	return checkpoint
}

func orgCountsToPO(source map[int64]CatalogDriftCounts) map[string]CatalogDriftCounts {
	result := make(map[string]CatalogDriftCounts, len(source))
	for orgID, counts := range source {
		result[strconv.FormatInt(orgID, 10)] = counts
	}
	return result
}

func orgCountsFromPO(source map[string]CatalogDriftCounts) map[int64]CatalogDriftCounts {
	result := make(map[int64]CatalogDriftCounts, len(source))
	for rawOrgID, counts := range source {
		orgID, err := strconv.ParseInt(rawOrgID, 10, 64)
		if err == nil {
			result[orgID] = counts
		}
	}
	return result
}

func numericUint64(value any) (uint64, bool) {
	switch typed := value.(type) {
	case int32:
		return uint64(typed), typed >= 0
	case int64:
		return uint64(typed), typed >= 0
	case uint64:
		return typed, true
	default:
		return 0, false
	}
}
