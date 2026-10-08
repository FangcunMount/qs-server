package retirement

import (
	"bytes"
	"context"
	"reflect"
	"sort"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retirementevidence"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const ErrMongoBatchCAS SourceError = "mongo_historical_batch_cas_rejected"

// An attachment supplies a compact conclusion, never closure authorization.
// Its event is rebound to one real complete four-copy source index below.
type MongoHistoricalBatchAttachment struct {
	Source *VerifiedSourceEvent
	Entry  evidence.HistoricalReferenceEntryV1
}
type mongoCASGroup struct {
	collection, slot string
	id               uint64
	pk               bson.RawValue
	set              *evidence.HistoricalReferenceSetV1
	entries          []evidence.HistoricalReferenceEntryV1
}
type MongoHistoricalBatchCASPlan struct {
	db                                                           *mongo.Database
	config                                                       MongoOwnerConfig
	oldTxn                                                       mongoCycleTxn
	metadataHash, identity, originalSQLIdentity, originalSQLRows string
	selection                                                    mongoBatchSelection
	limits                                                       MongoHistoricalOwnerBatchLimits
	hints                                                        map[string]string
	originalSQLConnection                                        any
	expires                                                      time.Time
	before                                                       map[string][]bson.Raw
	groups                                                       []mongoCASGroup
	attachments                                                  []mongoCASAttachmentFacts
}
type mongoCASAttachmentFacts struct {
	collection, slot string
	id               uint64
	entry            evidence.HistoricalReferenceEntryV1
	content          evidence.Digest
}
type MongoHistoricalBatchCASStatement struct {
	plan        *MongoHistoricalBatchCASPlan
	transaction mongoCycleTxn
	expected    map[string][]bson.Raw
	changed     map[string]bool
}
type MongoHistoricalBatchCASLocation struct {
	Collection, Slot                                       string
	ID                                                     uint64
	EventID, EventType                                     string
	Source                                                 evidence.HistoricalSourceReferenceV1
	ContentDigest                                          evidence.Digest
	BusinessBindingSHA256                                  string
	FenceWritePerformed, AlreadyMatched, ReferenceAppended bool
}
type MongoHistoricalBatchCASReport struct {
	Protocol, DatabaseIdentitySHA256, ExpectedBusinessRowsSHA256, OriginalSQLBusinessRowsSHA256 string
	Locations                                                                                   []MongoHistoricalBatchCASLocation
	StatementApplied, HostCommitRequired, IndependentReadbackRequired, JointSQLRecheckRequired  bool
	SourceCopyFactsBound, ExternalOriginAuthenticationRequired                                  bool
	HostCommitVerified, BusinessClosureVerified, DropReady                                      bool
	RangeAbsenceRecheckRequired                                                                 bool
}

func (*MongoHistoricalBatchCASPlan) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*MongoHistoricalBatchCASPlan) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*MongoHistoricalBatchCASPlan) String() string {
	return "private Mongo historical CAS plan; joint closure unproven"
}
func (p *MongoHistoricalBatchCASPlan) GoString() string { return p.String() }
func (*MongoHistoricalBatchCASStatement) MarshalJSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*MongoHistoricalBatchCASStatement) MarshalBSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*MongoHistoricalBatchCASStatement) String() string {
	return "private Mongo historical CAS statement; host commit unproven"
}
func (s *MongoHistoricalBatchCASStatement) GoString() string { return s.String() }

func mongoCASCloneData(data map[string][]bson.Raw) map[string][]bson.Raw {
	out := map[string][]bson.Raw{}
	for name, rows := range data {
		if rows == nil {
			out[name] = nil
			continue
		}
		out[name] = make([]bson.Raw, len(rows))
		for i, raw := range rows {
			out[name][i] = append(bson.Raw(nil), raw...)
		}
	}
	return out
}
func mongoCASCloneSelection(v mongoBatchSelection) mongoBatchSelection {
	copyIDs := func(m map[uint64]bool) map[uint64]bool {
		r := map[uint64]bool{}
		for k, v := range m {
			r[k] = v
		}
		return r
	}
	return mongoBatchSelection{copyIDs(v.sheets), copyIDs(v.outcomes), copyIDs(v.generations), copyIDs(v.artifacts), copyIDs(v.runs)}
}
func mongoCASKey(name string, id uint64) string { return name + ":" + strconv.FormatUint(id, 10) }
func mongoCASRow(data map[string][]bson.Raw, name string, id uint64) (bson.Raw, error) {
	var found bson.Raw
	for _, raw := range data[name] {
		n, ok := mongoExactInteger(raw.Lookup("domain_id"))
		if !ok || n <= 0 {
			return nil, ErrMongoBatchCAS
		}
		if uint64(n) == id {
			if found != nil {
				return nil, ErrMongoBatchCAS
			}
			found = raw
		}
	}
	if found == nil {
		return nil, ErrMongoBatchCAS
	}
	return found, nil
}
func mongoCASSet(raw bson.Raw, slot string) (*evidence.HistoricalReferenceSetV1, error) {
	if raw.Lookup(slot).Type == 0 {
		return nil, nil
	}
	return retirementevidence.DecodeHistoricalSetDocument(raw, slot)
}

// RawValue retains all BSON types, nested field order, missing and physical
// null. This replaces only the selected maintenance field, never a whole PO.
func mongoCASReplace(raw bson.Raw, slot string, value any) (bson.Raw, error) {
	elements, e := raw.Elements()
	if e != nil {
		return nil, e
	}
	doc := bson.D{}
	found := false
	for _, element := range elements {
		if element.Key() == slot {
			if found {
				return nil, ErrMongoBatchCAS
			}
			found = true
			doc = append(doc, bson.E{Key: slot, Value: value})
		} else {
			doc = append(doc, bson.E{Key: element.Key(), Value: element.Value()})
		}
	}
	if !found {
		doc = append(doc, bson.E{Key: slot, Value: value})
	}
	encoded, e := bson.Marshal(doc)
	if e != nil || len(encoded) > MaxSourceRowBytes {
		return nil, ErrMongoBatchBounds
	}
	return encoded, nil
}
func mongoCASSort(data map[string][]bson.Raw) {
	for name := range data {
		sort.Slice(data[name], func(i, j int) bool { return bytes.Compare(data[name][i], data[name][j]) < 0 })
	}
}
func mongoCASHash(data map[string][]bson.Raw, metadata string) string {
	parts := []string{"mongo-batch-cas-raw/v1", metadata}
	for _, name := range mongoBatchBusinessCollections {
		parts = append(parts, name)
		for _, raw := range data[name] {
			parts = append(parts, string(raw))
		}
	}
	return mongoOwnerHashParts(parts...)
}

func mongoCASHistoryIndex(metadata mongoCycleMetadata, name, slot string) bool {
	definition, ok := metadata.definitions[name]
	if !ok {
		return false
	}
	key := slot + ".entries.event_id"
	for _, raw := range definition.indexes {
		f, e := exactBSONFields(raw)
		if e != nil {
			return false
		}
		keyDocument, keyOK := f["key"].DocumentOK()
		if !keyOK {
			return false
		}
		keys, e := exactBSONFields(keyDocument)
		direction, valid := mongoExactInteger(keys[key])
		unique, uniqueOK := f["unique"].BooleanOK()
		if e != nil || len(keys) != 1 || !valid || direction != 1 || !uniqueOK || !unique {
			continue
		}
		if collation, has := f["collation"]; has && (collation.Type != bson.TypeEmbeddedDocument || collation.Document().Lookup("locale").Type != bson.TypeString || collation.Document().Lookup("locale").StringValue() != "simple") {
			continue
		}
		partialDocument, partialOK := f["partialFilterExpression"].DocumentOK()
		if !partialOK {
			continue
		}
		partial, e := exactBSONFields(partialDocument)
		if e != nil || len(partial) != 1 || partial[key].Type != bson.TypeEmbeddedDocument {
			continue
		}
		condition, e := exactBSONFields(partial[key].Document())
		if e == nil && len(condition) == 1 && condition["$type"].Type == bson.TypeString && condition["$type"].StringValue() == "string" {
			return true
		}
	}
	return false
}

func PrepareMongoHistoricalBatchCAS(ctx context.Context, b *MongoHistoricalOwnerBatch, copies *VerifiedSourceCopies, attachments []MongoHistoricalBatchAttachment) (*MongoHistoricalBatchCASPlan, error) {
	if b == nil || b.global == nil || copies == nil || !copies.complete || len(attachments) == 0 || len(attachments) > 512 {
		return nil, ErrMongoBatchCAS
	}
	if err := b.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	p := &MongoHistoricalBatchCASPlan{db: b.global.db, config: b.global.config, oldTxn: b.global.txn, metadataHash: b.global.metadata.hash, identity: b.global.metadata.identity, originalSQLIdentity: b.sql.Report().DatabaseIdentitySHA256, originalSQLRows: b.sql.Report().BusinessRowsSHA256, selection: mongoCASCloneSelection(b.selection), limits: b.limits, before: mongoCASCloneData(b.data)}
	byGroup := map[string]int{}
	byEvent, bySource := map[string]mongoCASAttachmentFacts{}, map[string]mongoCASAttachmentFacts{}
	seenAttachments := map[string]mongoCASAttachmentFacts{}
	p.originalSQLConnection = b.sqlConnection
	p.expires = b.started.Add(b.limits.MaxDuration)
	p.hints = map[string]string{}
	for _, pair := range mongoCASSelections(p.selection) {
		name, err := b.index(pair.name, pair.field, pair.field == "domain_id")
		if err != nil {
			return nil, err
		}
		p.hints[pair.name+":"+pair.field] = name
	}
	register := func(a mongoCASAttachmentFacts) error {
		key := a.entry.Source.Database + ":" + a.entry.Source.Object + ":" + a.entry.Source.PrimaryKeySHA256
		if old, ok := byEvent[a.entry.EventID]; ok && (old.collection != a.collection || old.id != a.id || !reflect.DeepEqual(old.entry, a.entry)) {
			return evidence.ErrHistoricalReferenceConflict
		}
		if old, ok := bySource[key]; ok && (old.collection != a.collection || old.id != a.id || !reflect.DeepEqual(old.entry, a.entry)) {
			return evidence.ErrHistoricalReferenceConflict
		}
		byEvent[a.entry.EventID], bySource[key] = a, a
		return nil
	}
	// Existing source references in every selected business document are
	// immutable constraints, even when that document has no new attachment.
	for _, location := range []struct{ name, slot, eventType string }{{"answersheets", "legacy_submission_evidence", "answersheet.submitted"}, {"report_generations", "historical_generated_evidence", "interpretation.report.generated"}} {
		for _, raw := range p.before[location.name] {
			set, e := mongoCASSet(raw, location.slot)
			if e != nil {
				return nil, e
			}
			if set == nil {
				continue
			}
			id, ok := mongoExactInteger(raw.Lookup("domain_id"))
			if !ok || id <= 0 {
				return nil, ErrMongoBatchCAS
			}
			for _, prior := range set.Entries {
				if prior.EventType != location.eventType {
					return nil, ErrMongoBatchCAS
				}
				if e = register(mongoCASAttachmentFacts{collection: location.name, slot: location.slot, id: uint64(id), entry: prior.Clone()}); e != nil {
					return nil, e
				}
			}
		}
	}
	for _, input := range attachments {
		facts, err := input.Source.Facts()
		if err != nil {
			return nil, err
		}
		handle, err := copies.BindEvent(facts)
		if err != nil {
			return nil, err
		}
		source, _, err := b.source(handle)
		if err != nil {
			return nil, err
		}
		q, err := b.ResolveSource(ctx, handle)
		if err != nil {
			return nil, err
		}
		local := q.Local()
		entry := input.Entry.Clone()
		if err = entry.Validate(); err != nil {
			return nil, err
		}
		if entry.EventID != source.EventID || entry.EventType != source.EventType || entry.Source != source.Source || entry.Source.Digest.Kind != MongoRowDigestKind || entry.Proof.BusinessBindingSHA256 != local.BusinessBindingSHA256 || !reflect.DeepEqual(entry.Run, local.OriginalRun) {
			return nil, ErrMongoBatchCAS
		}
		name, slot, id := "answersheets", "legacy_submission_evidence", local.AnswerSheetID
		if source.EventType == "interpretation.report.generated" {
			name, slot, id = "report_generations", "historical_generated_evidence", local.GenerationID
		} else if source.EventType != "answersheet.submitted" {
			return nil, ErrSourceEventType
		}
		if !mongoCASHistoryIndex(b.global.metadata, name, slot) {
			return nil, ErrMongoCycleSchema
		}
		raw, err := mongoCASRow(p.before, name, id)
		if err != nil {
			return nil, err
		}
		if raw.Lookup("_id").Type != bson.TypeObjectID {
			return nil, ErrMongoBatchCAS
		}
		a := mongoCASAttachmentFacts{collection: name, slot: slot, id: id, entry: entry, content: source.ContentDigest}
		if old, ok := seenAttachments[entry.EventID]; ok {
			if !reflect.DeepEqual(old, a) {
				return nil, evidence.ErrHistoricalReferenceConflict
			}
			continue
		}
		if err = register(a); err != nil {
			return nil, err
		}
		seenAttachments[entry.EventID] = a
		key := mongoCASKey(name, id)
		position, exists := byGroup[key]
		if !exists {
			set, err := mongoCASSet(raw, slot)
			if err != nil {
				return nil, err
			}
			if set != nil {
				for _, prior := range set.Entries {
					if prior.EventType != entry.EventType || prior.Proof.BusinessBindingSHA256 != local.BusinessBindingSHA256 || !reflect.DeepEqual(prior.Run, local.OriginalRun) {
						return nil, ErrMongoBatchCAS
					}
					if err = register(mongoCASAttachmentFacts{collection: name, slot: slot, id: id, entry: prior}); err != nil {
						return nil, err
					}
				}
			}
			p.groups = append(p.groups, mongoCASGroup{collection: name, slot: slot, id: id, pk: raw.Lookup("_id"), set: set})
			position = len(p.groups) - 1
			byGroup[key] = position
		}
		group := &p.groups[position]
		group.entries = append(group.entries, entry)
		next, err := group.set.Append(entry)
		if err != nil {
			return nil, err
		}
		encoded, err := bson.Marshal(next)
		if err != nil || len(encoded) > evidence.HistoricalReferenceMaxBytes {
			return nil, ErrMongoBatchBounds
		}
		group.set = next
		p.attachments = append(p.attachments, a)
	}
	if err := b.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

type mongoCASSelectionQuery struct {
	name, field string
	ids         map[uint64]bool
}

func mongoCASSelections(selection mongoBatchSelection) []mongoCASSelectionQuery {
	return []mongoCASSelectionQuery{{"answersheets", "domain_id", selection.sheets}, {"report_generations", "outcome_id", selection.outcomes}, {"report_generations", "domain_id", selection.generations}, {"interpret_report_artifacts", "outcome_id", selection.outcomes}, {"interpret_report_artifacts", "generation_id", selection.generations}, {"interpret_report_artifacts", "domain_id", selection.artifacts}, {"interpretation_runs", "generation_id", selection.generations}, {"interpretation_runs", "domain_id", selection.runs}}
}

func (p *MongoHistoricalBatchCASPlan) capture(ctx context.Context) (map[string][]bson.Raw, error) {
	data := map[string][]bson.Raw{}
	var rowCount int
	var size uint64
	pairs := mongoCASSelections(p.selection)
	seen := map[string]map[string]bson.Raw{}
	for _, name := range mongoBatchBusinessCollections {
		data[name] = nil
		seen[name] = map[string]bson.Raw{}
	}
	for _, pair := range pairs {
		if len(pair.ids) == 0 {
			continue
		}
		ids := bson.A{}
		for _, id := range mongoBatchIDs(pair.ids) {
			if id == 0 || id > 1<<63-1 {
				return nil, ErrMongoBatchCAS
			}
			ids = append(ids, int64(id))
		}
		remaining := p.limits.MaxRows - rowCount
		if remaining <= 0 {
			return nil, ErrMongoBatchBounds
		}
		cursor, err := p.db.Collection(pair.name).Find(ctx, bson.D{{Key: pair.field, Value: bson.D{{Key: "$in", Value: ids}}}}, options.Find().SetHint(p.hints[pair.name+":"+pair.field]).SetCollation(&options.Collation{Locale: "simple"}).SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(remaining+1)).SetBatchSize(128))
		if err != nil {
			return nil, ErrMongoOwnerRead
		}
		var raws []bson.Raw
		if err = cursor.All(ctx, &raws); err != nil {
			return nil, ErrMongoOwnerRead
		}
		for _, raw := range raws {
			rowCount++
			size += uint64(len(raw))
			if rowCount > p.limits.MaxRows || size > p.limits.MaxBytes || len(raw) > MaxSourceRowBytes {
				return nil, ErrMongoBatchBounds
			}
			if mongoUniqueBSON(raw, 0) != nil || mongoPOShape(raw, mongoBatchPOType(pair.name)) != nil {
				return nil, ErrMongoCycleSchema
			}
			if raw.Lookup("_id").Type != bson.TypeObjectID {
				return nil, ErrMongoBatchCAS
			}
			pk := string(raw.Lookup("_id").Value)
			if previous, ok := seen[pair.name][pk]; ok {
				if !bytes.Equal(previous, raw) {
					return nil, ErrMongoBatchConflict
				}
				continue
			}
			cloned := append(bson.Raw(nil), raw...)
			seen[pair.name][pk] = cloned
			data[pair.name] = append(data[pair.name], cloned)
		}
	}
	mongoCASSort(data)
	return data, nil
}
func (p *MongoHistoricalBatchCASPlan) expectedData() (map[string][]bson.Raw, error) {
	data := mongoCASCloneData(p.before)
	for _, group := range p.groups {
		for i, raw := range data[group.collection] {
			if raw.Lookup("_id").Equal(group.pk) {
				next, err := mongoCASReplace(raw, group.slot, group.set.Clone())
				if err != nil {
					return nil, err
				}
				data[group.collection][i] = next
			}
		}
	}
	mongoCASSort(data)
	return data, nil
}
func mongoCASFilter(raw bson.Raw) bson.D {
	return bson.D{{Key: "_id", Value: raw.Lookup("_id")}, {Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$$ROOT", bson.D{{Key: "$literal", Value: raw}}}}}}}
}

func (p *MongoHistoricalBatchCASPlan) Apply(ctx context.Context) (*MongoHistoricalBatchCASStatement, error) {
	if p == nil || p.db == nil || len(p.groups) == 0 || ctx == nil || time.Now().After(p.expires) {
		return nil, ErrMongoBatchCAS
	}
	transaction, err := mongoCycleTransaction(ctx, p.db)
	if err != nil {
		return nil, err
	}
	if transaction.number == p.oldTxn.number && bytes.Equal(transaction.session, p.oldTxn.session) {
		return nil, ErrMongoCycleTransaction
	}
	metadata, err := observeMongoCycleMetadata(ctx, p.db, p.config)
	if err != nil {
		return nil, err
	}
	if metadata.hash != p.metadataHash {
		return nil, ErrMongoBatchConflict
	}
	current, err := p.capture(ctx)
	if err != nil {
		return nil, err
	}
	expected, err := p.expectedData()
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(current, p.before) && !reflect.DeepEqual(current, expected) {
		return nil, ErrMongoBatchConflict
	}
	changed := map[string]bool{}
	fencedData := mongoCASCloneData(current)
	fence := mongoOwnerHashParts("mongo-batch-cas-fence/v1", p.identity, mongoCASHash(expected, p.metadataHash))
	const fenceField = "_retirement_batch_cas_fence"
	// Force an actual write for every selected dependency, including already
	// matched proofs. Snapshot reads alone do not lock concurrently changed Run
	// or Artifact facts; server MVCC then rejects a stale writer. This does not
	// lock absent ranges. The independent fresh scan and writer fence remain
	// mandatory, including newly inserted descendants.
	for _, name := range mongoBatchBusinessCollections {
		for i, raw := range current[name] {
			if raw.Lookup(fenceField).Type != 0 {
				return nil, ErrMongoBatchConflict
			}
			next, e := mongoCASReplace(raw, fenceField, fence)
			if e != nil {
				return nil, e
			}
			result, e := p.db.Collection(name).UpdateOne(ctx, mongoCASFilter(raw), bson.D{{Key: "$set", Value: bson.D{{Key: fenceField, Value: fence}}}}, options.Update().SetCollation(&options.Collation{Locale: "simple"}))
			if e != nil {
				return nil, e
			}
			if result.MatchedCount != 1 || result.ModifiedCount != 1 {
				return nil, ErrMongoBatchConflict
			}
			fencedData[name][i] = next
		}
	}
	for _, group := range p.groups {
		raw, e := mongoCASRow(current, group.collection, group.id)
		if e != nil {
			return nil, e
		}
		next, e := mongoCASRow(expected, group.collection, group.id)
		if e != nil {
			return nil, e
		}
		changed[mongoCASKey(group.collection, group.id)] = !bytes.Equal(raw, next)
		for i, fenced := range fencedData[group.collection] {
			if !fenced.Lookup("_id").Equal(group.pk) {
				continue
			}
			result, e := p.db.Collection(group.collection).UpdateOne(ctx, mongoCASFilter(fenced), bson.D{{Key: "$set", Value: bson.D{{Key: group.slot, Value: group.set.Clone()}}}}, options.Update().SetCollation(&options.Collation{Locale: "simple"}))
			if e != nil {
				return nil, e
			}
			if result.MatchedCount != 1 {
				return nil, ErrMongoBatchConflict
			}
			fencedData[group.collection][i], e = mongoCASReplace(fenced, group.slot, group.set.Clone())
			if e != nil {
				return nil, e
			}
		}
	}
	for _, name := range mongoBatchBusinessCollections {
		for _, fenced := range fencedData[name] {
			result, e := p.db.Collection(name).UpdateOne(ctx, mongoCASFilter(fenced), bson.D{{Key: "$unset", Value: bson.D{{Key: fenceField, Value: ""}}}}, options.Update().SetCollation(&options.Collation{Locale: "simple"}))
			if e != nil {
				return nil, e
			}
			if result.MatchedCount != 1 || result.ModifiedCount != 1 {
				return nil, ErrMongoBatchConflict
			}
		}
	}

	after, err := p.capture(ctx)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(after, expected) {
		return nil, ErrMongoBatchConflict
	}
	metadata, err = observeMongoCycleMetadata(ctx, p.db, p.config)
	if err != nil || metadata.hash != p.metadataHash {
		return nil, ErrMongoBatchConflict
	}
	actual, err := mongoCycleTransaction(ctx, p.db)
	if err != nil || actual.number != transaction.number || !bytes.Equal(actual.session, transaction.session) {
		return nil, ErrMongoCycleTransaction
	}
	if time.Now().After(p.expires) {
		return nil, ErrMongoBatchBounds
	}
	return &MongoHistoricalBatchCASStatement{plan: p, transaction: transaction, expected: after, changed: changed}, nil
}

func (s *MongoHistoricalBatchCASStatement) Report() MongoHistoricalBatchCASReport {
	r := MongoHistoricalBatchCASReport{Protocol: "mongo-batch-cas-statement/v1", HostCommitRequired: true, IndependentReadbackRequired: true, JointSQLRecheckRequired: true, ExternalOriginAuthenticationRequired: true, RangeAbsenceRecheckRequired: true}
	if s == nil || s.plan == nil {
		return r
	}
	r.StatementApplied = true
	r.SourceCopyFactsBound = true
	r.DatabaseIdentitySHA256 = s.plan.identity
	r.ExpectedBusinessRowsSHA256 = mongoCASHash(s.expected, s.plan.metadataHash)
	r.OriginalSQLBusinessRowsSHA256 = s.plan.originalSQLRows
	for _, a := range s.plan.attachments {
		existed := false
		raw, _ := mongoCASRow(s.plan.before, a.collection, a.id)
		set, _ := mongoCASSet(raw, a.slot)
		if set != nil {
			for _, entry := range set.Entries {
				if reflect.DeepEqual(entry, a.entry) {
					existed = true
					break
				}
			}
		}
		changed := s.changed[mongoCASKey(a.collection, a.id)]
		r.Locations = append(r.Locations, MongoHistoricalBatchCASLocation{Collection: a.collection, Slot: a.slot, ID: a.id, EventID: a.entry.EventID, EventType: a.entry.EventType, Source: a.entry.Source, ContentDigest: a.content, BusinessBindingSHA256: a.entry.Proof.BusinessBindingSHA256, FenceWritePerformed: true, AlreadyMatched: existed || !changed, ReferenceAppended: changed && !existed})
	}
	return r
}

// This reads the expected actual BSON in a genuinely new snapshot. It does not
// assert a successful host Commit response or jointly recheck SQL after its own
// legitimate evidence CAS; the SQL bridge's expected baseline must do that.
func (s *MongoHistoricalBatchCASStatement) VerifyPersisted(ctx context.Context, fresh *MongoHistoricalOwnerBatch) error {
	if s == nil || s.plan == nil || ctx == nil || time.Now().After(s.plan.expires) || fresh == nil || fresh.global == nil || fresh.sql == nil || fresh.sqlConnection == nil || fresh.sqlConnection == s.plan.originalSQLConnection || fresh.global.db.Name() != s.plan.db.Name() || fresh.global.metadata.hash != s.plan.metadataHash || fresh.sql.Report().DatabaseIdentitySHA256 != s.plan.originalSQLIdentity || !reflect.DeepEqual(fresh.selection, s.plan.selection) {
		return ErrMongoBatchCAS
	}
	actual, err := mongoCycleTransaction(ctx, fresh.global.db)
	if err != nil {
		return err
	}
	if actual.number == s.transaction.number && bytes.Equal(actual.session, s.transaction.session) || actual.number == s.plan.oldTxn.number && bytes.Equal(actual.session, s.plan.oldTxn.session) {
		return ErrMongoCycleTransaction
	}
	if err = fresh.ValidateBorrowedSnapshot(ctx); err != nil {
		return err
	}
	if !reflect.DeepEqual(fresh.data, s.expected) {
		return ErrMongoBatchConflict
	}
	metadata, err := observeMongoCycleMetadata(ctx, fresh.global.db, fresh.global.config)
	if err != nil {
		return err
	}
	if metadata.hash != s.plan.metadataHash {
		return ErrMongoBatchConflict
	}
	return fresh.ValidateBorrowedSnapshot(ctx)
}
