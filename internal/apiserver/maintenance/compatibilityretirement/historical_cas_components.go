package retirement

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
)

const ErrHistoricalCASComponents SourceError = "historical_cas_component_inputs_rejected"

type historicalCASRowKey struct {
	store, name string
	id          uint64
}
type historicalCASRowInput struct {
	key   historicalCASRowKey
	sha   string
	bytes uint64
	write bool
}
type historicalCASOwnerKey struct {
	kind    string
	id, org uint64
}

// HistoricalCASComponentInput is an immutable footprint of physical inputs,
// not a qualified plan. Only the live owner/capture factory below produces it.
// Ending or expiring the original epoch never makes this input writable.
type HistoricalCASComponentInput struct {
	self                  *HistoricalCASComponentInput
	index                 *WholeSourceJointIndex
	inputPair             *HistoricalSourceInputPair
	binding               HistoricalCoordinatorBinding
	sequence              uint64
	partition, partitions uint32
	sources               []verifiedSourceKey
	owners                []historicalCASOwnerKey
	rows                  []historicalCASRowInput
	sqlRecipe             *sqlevaluation.SQLHistoricalComponentRecipe
	mongoRead             *mongoHistoricalComponentReadRecipe
	mongoCAS              *mongoHistoricalComponentCASRecipe
	seal                  string
}

func (*HistoricalCASComponentInput) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*HistoricalCASComponentInput) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*HistoricalCASComponentInput) UnmarshalJSON([]byte) error   { return ErrSourceSerialization }
func (*HistoricalCASComponentInput) UnmarshalBSON([]byte) error   { return ErrSourceSerialization }
func (*HistoricalCASComponentInput) String() string {
	return "private historical event dependency inputs; no CAS or AI authority"
}
func (f *HistoricalCASComponentInput) digest() string {
	if f == nil || f.index == nil {
		return ""
	}
	sqlSHA := ""
	if f.sqlRecipe != nil {
		var err error
		sqlSHA, err = f.sqlRecipe.InputSHA256()
		if err != nil {
			return ""
		}
	}
	parts := []string{"historical-cas-component-input/v2", f.binding.SourceSHA, f.binding.OperationID, f.index.indexSHA, strconv.FormatUint(f.sequence, 10), strconv.FormatUint(uint64(f.partition), 10), strconv.FormatUint(uint64(f.partitions), 10), sqlSHA, f.mongoRead.digest(), f.mongoCAS.digest()}
	if f.inputPair != nil {
		p := f.inputPair
		if p.self != p || p.first == nil || p.second == nil || p.first.recipe != f.index.input || p.second.recipe != f.index.input || f.mongoRead == nil || f.mongoRead.snapshotOriginalInput != p.second.mongo || f.mongoCAS != nil {
			return ""
		}
		parts = append(parts, "source-input-pair", p.first.epochHash, p.second.epochHash, p.second.resultHash, strconv.FormatUint(p.first.dev, 10), strconv.FormatUint(p.first.ino, 10), strconv.FormatUint(p.second.dev, 10), strconv.FormatUint(p.second.ino, 10))
	}
	for _, key := range f.sources {
		parts = append(parts, strconv.Itoa(int(key.object)), string(key.pk[:]))
	}
	for _, key := range f.owners {
		parts = append(parts, key.kind, strconv.FormatUint(key.id, 10), strconv.FormatUint(key.org, 10))
	}
	for _, row := range f.rows {
		parts = append(parts, row.key.store, row.key.name, strconv.FormatUint(row.key.id, 10), row.sha, strconv.FormatUint(row.bytes, 10), strconv.FormatBool(row.write))
	}
	return mongoOwnerHashParts(parts...)
}

// Freeze retains per-page inputs while the original physical reads are alive;
// it deliberately does not require the coordinator's later whole EOF seal.
// SQL and Mongo plans remain observations of intended targets, never imported
// DTOs or reused write qualifications. Fresh scoped qualification is separate.
func FreezeHistoricalCASComponentInput(ctx context.Context, joint *WholeSourceJointPage, sqlPlan *sqlevaluation.SQLHistoricalCASProvenance, unchangedSQL *sqlevaluation.SQLHistoricalCASReadBaseline, mongoPlan *MongoHistoricalBatchCASPlan) (*HistoricalCASComponentInput, error) {
	if ctx == nil || ctx.Err() != nil || joint == nil || joint.page == nil || joint.page.sequence == 0 || joint.index == nil || !joint.index.complete || joint.sql == nil || joint.mongo == nil || len(joint.current) == 0 || historicalCASComponentPageAlive(ctx, joint) != nil || joint.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrHistoricalCASComponents
	}
	sql, err := sqlevaluation.FreezeSQLHistoricalComponentRecipe(ctx, joint.sql.facts, joint.cross.page, sqlPlan, unchangedSQL)
	if err != nil {
		return nil, err
	}
	f := &HistoricalCASComponentInput{index: joint.index, binding: joint.owner.binding, sequence: joint.page.sequence, sqlRecipe: sql}
	f.mongoRead, err = freezeMongoHistoricalComponentReadRecipe(ctx, joint.mongo)
	if err != nil {
		return nil, err
	}
	if err = sql.RowDependencies(func(table string, id uint64, sha string, size uint64, write bool) error {
		f.rows = append(f.rows, historicalCASRowInput{historicalCASRowKey{"mysql", table, id}, sha, size, write})
		return nil
	}); err != nil {
		return nil, err
	}
	data := joint.mongo.data
	writes := map[historicalCASRowKey]bool{}
	if mongoPlan != nil {
		if mongoPlan.db != joint.mongo.global.db || mongoPlan.originalSQLConnection != joint.mongo.sqlConnection || mongoPlan.oldTxn.number != joint.mongo.global.txn.number || !reflect.DeepEqual(mongoPlan.oldTxn.session, joint.mongo.global.txn.session) || !reflect.DeepEqual(mongoPlan.before, data) || !time.Now().Before(mongoPlan.expires) {
			return nil, ErrHistoricalCASComponents
		}
		for _, group := range mongoPlan.groups {
			writes[historicalCASRowKey{"mongodb", group.collection, group.id}] = true
		}
	}
	f.mongoCAS, err = freezeMongoHistoricalComponentCASRecipe(ctx, joint, mongoPlan)
	if err != nil {
		return nil, err
	}
	seen := map[historicalCASRowKey]bool{}
	for _, name := range mongoBatchBusinessCollections {
		for _, raw := range data[name] {
			id, err := historicalSpoolMongoID(raw)
			key := historicalCASRowKey{"mongodb", name, id}
			if err != nil || id == 0 || seen[key] {
				return nil, ErrHistoricalCASComponents
			}
			seen[key] = true
			f.rows = append(f.rows, historicalCASRowInput{key, historicalSpoolSHA(raw), uint64(len(raw)), writes[key]})
		}
	}
	for key := range writes {
		if !seen[key] {
			return nil, ErrHistoricalCASComponents
		}
	}
	owners := map[historicalCASOwnerKey]bool{}
	for _, handle := range joint.current {
		facts, err := handle.Facts()
		if err != nil {
			return nil, err
		}
		entry, ok := joint.index.entries[facts.EventID]
		hash, err := privateFactsSHA(facts)
		if err != nil || !ok || entry.FactsSHA256 != hash {
			return nil, ErrHistoricalCASComponents
		}
		f.sources = append(f.sources, entry.Key)
	}
	for _, handle := range joint.sources {
		facts, err := handle.Facts()
		if err != nil {
			return nil, err
		}
		entry, ok := joint.index.entries[facts.EventID]
		if !ok || entry.OrgID == 0 {
			return nil, ErrHistoricalCASComponents
		}
		var owner *sqlevaluation.SQLHistoricalBatchOwnerFacts
		if facts.Submitted != nil {
			owner, err = joint.sql.OwnerByAnswerSheet(entry.AnswerSheetID)
		} else {
			owner, err = joint.sql.OwnerByAssessment(entry.AssessmentID)
		}
		if errors.Is(err, sqlevaluation.ErrSQLHistoricalOwnerAbsent) && facts.Submitted != nil {
			if entry.AnswerSheetID == 0 {
				return nil, ErrHistoricalCASComponents
			}
			owners[historicalCASOwnerKey{"sheet", entry.AnswerSheetID, entry.OrgID}] = true
			continue
		}
		if err != nil || owner == nil {
			return nil, ErrHistoricalCASComponents
		}
		actual := owner.Snapshot().Owner
		if actual.OrgID != entry.OrgID || actual.AssessmentID == 0 {
			return nil, ErrHistoricalCASComponents
		}
		owners[historicalCASOwnerKey{"assessment", actual.AssessmentID, actual.OrgID}] = true
		if actual.AnswerSheetID != 0 {
			owners[historicalCASOwnerKey{"sheet", actual.AnswerSheetID, actual.OrgID}] = true
		}
	}
	for owner := range owners {
		f.owners = append(f.owners, owner)
	}
	sort.Slice(f.owners, func(i, j int) bool {
		a, b := f.owners[i], f.owners[j]
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		if a.id != b.id {
			return a.id < b.id
		}
		return a.org < b.org
	})
	sort.Slice(f.rows, func(i, j int) bool {
		a, b := f.rows[i].key, f.rows[j].key
		if a.store != b.store {
			return a.store < b.store
		}
		if a.name != b.name {
			return a.name < b.name
		}
		return a.id < b.id
	})
	f.self, f.seal = f, f.digest()
	if f.seal == "" || historicalCASComponentPageAlive(ctx, joint) != nil || joint.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrHistoricalCASComponents
	}
	return f, nil
}

// A page only transports sources. These pure fragments retain each actual
// SQL/replay owner closure and its complete original Mongo negative ranges.
// The original joint is never copied, partly consumed or used as a child plan.
func FreezeHistoricalCASOwnerComponentInputs(ctx context.Context, joint *WholeSourceJointPage, sqlPlan *sqlevaluation.SQLHistoricalCASProvenance, unchangedSQL *sqlevaluation.SQLHistoricalCASReadBaseline, mongoPlan *MongoHistoricalBatchCASPlan, spool *sqlevaluation.SQLHistoricalCASSpool) ([]*HistoricalCASComponentInput, error) {
	if ctx == nil || ctx.Err() != nil || joint == nil || joint.sql == nil || joint.cross == nil || joint.mongo == nil || joint.index == nil || !joint.index.complete || len(joint.current) == 0 || historicalCASComponentPageAlive(ctx, joint) != nil || joint.ValidateBorrowedSnapshot(ctx) != nil || validateMongoHistoricalComponentOriginalPlan(ctx, joint, mongoPlan) != nil {
		return nil, ErrHistoricalCASComponents
	}
	byEvent := map[string]*VerifiedSourceEvent{}
	entries := map[string]wholeSourceJointEntry{}
	present, absent := map[string]uint64{}, map[string]uint64{}
	for _, handle := range joint.sources {
		facts, key, err := joint.mongo.source(handle)
		if err != nil || byEvent[facts.EventID] != nil {
			return nil, ErrHistoricalCASComponents
		}
		entry, ok := joint.index.entries[facts.EventID]
		actualFacts, err := handle.Facts()
		sha, hashErr := privateFactsSHA(actualFacts)
		if err != nil || hashErr != nil || !ok || entry.Key != key || entry.FactsSHA256 != sha || entry.OrgID != facts.OrgID || entry.OrgID == 0 {
			return nil, ErrHistoricalCASComponents
		}
		byEvent[facts.EventID], entries[facts.EventID] = handle, entry
		if owner := joint.mongo.sqlOwners[key]; owner != nil {
			actual := owner.Snapshot().Owner
			if actual.AssessmentID == 0 || actual.OrgID != entry.OrgID || entry.AssessmentID != 0 && entry.AssessmentID != actual.AssessmentID || entry.AnswerSheetID != 0 && entry.AnswerSheetID != actual.AnswerSheetID {
				return nil, ErrHistoricalCASComponents
			}
			present[facts.EventID] = actual.AssessmentID
		} else if joint.mongo.sqlAbsent[key] && actualFacts.Submitted != nil && entry.AnswerSheetID != 0 {
			absent[facts.EventID] = entry.AnswerSheetID
		} else {
			return nil, ErrHistoricalCASComponents
		}
	}
	var recipes []*sqlevaluation.SQLHistoricalComponentRecipe
	var err error
	if len(absent) == 0 {
		recipes, err = sqlevaluation.FreezeSQLHistoricalOwnerComponentRecipes(ctx, joint.sql.facts, joint.cross.page, sqlPlan, unchangedSQL, spool, present)
	} else {
		// The second map is independently checked by the SQL factory against
		// this original live page's actual empty AnswerSheet ranges.
		recipes, err = sqlevaluation.FreezeSQLHistoricalOwnerComponentRecipes(ctx, joint.sql.facts, joint.cross.page, sqlPlan, unchangedSQL, spool, present, absent)
	}
	if err != nil || len(recipes) == 0 || len(recipes) > 512 {
		return nil, ErrHistoricalCASComponents
	}
	current := map[string]bool{}
	for _, handle := range joint.current {
		facts, err := handle.Facts()
		if err != nil || current[facts.EventID] || byEvent[facts.EventID] == nil {
			return nil, ErrHistoricalCASComponents
		}
		current[facts.EventID] = true
	}
	parentRows := map[historicalCASRowKey][]byte{}
	for name, rows := range joint.mongo.data {
		for _, raw := range rows {
			id, err := historicalSpoolMongoID(raw)
			key := historicalCASRowKey{"mongodb", name, id}
			if err != nil || parentRows[key] != nil {
				return nil, ErrHistoricalCASComponents
			}
			parentRows[key] = raw
		}
	}
	seenEvents, seenCurrent, seenRows := map[string]bool{}, map[string]bool{}, map[historicalCASRowKey]bool{}
	coveredRanges := mongoCASCloneSelection(mongoBatchSelection{})
	var out []*HistoricalCASComponentInput
	for _, recipe := range recipes {
		if !recipe.OwnerPartitionResolved() {
			// A wider replay or unsupported ownership remains an explicit block.
			return nil, ErrHistoricalCASComponents
		}
		ids, err := recipe.SourceEventIDs()
		if err != nil || len(ids) == 0 {
			return nil, ErrHistoricalCASComponents
		}
		var sources []*VerifiedSourceEvent
		selected := map[string]bool{}
		owners := map[historicalCASOwnerKey]bool{}
		f := &HistoricalCASComponentInput{index: joint.index, binding: joint.owner.binding, sequence: joint.page.sequence, partition: uint32(len(out) + 1), partitions: uint32(len(recipes)), sqlRecipe: recipe}
		for _, eventID := range ids {
			entry, ok := entries[eventID]
			if !ok || seenEvents[eventID] || selected[eventID] {
				return nil, ErrHistoricalCASComponents
			}
			selected[eventID], seenEvents[eventID] = true, true
			sources = append(sources, byEvent[eventID])
			if owner := joint.mongo.sqlOwners[entry.Key]; owner != nil {
				actual := owner.Snapshot().Owner
				owners[historicalCASOwnerKey{"assessment", actual.AssessmentID, actual.OrgID}] = true
				if actual.AnswerSheetID != 0 {
					owners[historicalCASOwnerKey{"sheet", actual.AnswerSheetID, actual.OrgID}] = true
				}
			} else {
				owners[historicalCASOwnerKey{"sheet", entry.AnswerSheetID, entry.OrgID}] = true
			}
			if current[eventID] {
				f.sources = append(f.sources, entry.Key)
				seenCurrent[eventID] = true
			}
		}
		if len(f.sources) == 0 {
			return nil, ErrHistoricalCASComponents
		}
		// This reuses the actual original SQL batch and global Mongo epoch.
		// Selection derives from ALL related sources, never current targets.
		batch, err := PrepareMongoHistoricalOwnerBatch(ctx, joint.mongo.global, joint.sql.facts, sources, joint.mongo.limits)
		if err != nil {
			return nil, err
		}
		for i, query := range mongoCASSelections(batch.selection) {
			original := mongoCASSelections(joint.mongo.selection)[i]
			covered := mongoCASSelections(coveredRanges)[i]
			for id := range query.ids {
				if !original.ids[id] {
					return nil, ErrHistoricalCASComponents
				}
				covered.ids[id] = true
			}
		}
		f.mongoRead, err = freezeMongoHistoricalComponentReadRecipe(ctx, batch)
		if err != nil {
			return nil, err
		}
		f.mongoCAS, err = freezeMongoHistoricalOwnerComponentCASRecipe(ctx, joint, mongoPlan, batch, selected)
		if err != nil {
			return nil, err
		}
		writes := map[historicalCASRowKey]bool{}
		if f.mongoCAS != nil {
			for _, g := range f.mongoCAS.groups {
				writes[historicalCASRowKey{"mongodb", g.collection, g.id}] = true
			}
		}
		if err = recipe.RowDependencies(func(name string, id uint64, sha string, size uint64, write bool) error {
			f.rows = append(f.rows, historicalCASRowInput{historicalCASRowKey{"mysql", name, id}, sha, size, write})
			return nil
		}); err != nil {
			return nil, err
		}
		for name, rows := range batch.data {
			for _, raw := range rows {
				id, err := historicalSpoolMongoID(raw)
				key := historicalCASRowKey{"mongodb", name, id}
				if err != nil || !bytes.Equal(raw, parentRows[key]) {
					return nil, ErrMongoBatchConflict
				}
				seenRows[key] = true
				f.rows = append(f.rows, historicalCASRowInput{key, historicalSpoolSHA(raw), uint64(len(raw)), writes[key]})
			}
		}
		for key := range writes {
			if !seenRows[key] {
				return nil, ErrHistoricalCASComponents
			}
		}
		for owner := range owners {
			f.owners = append(f.owners, owner)
		}
		sort.Slice(f.owners, func(i, j int) bool {
			a, b := f.owners[i], f.owners[j]
			if a.kind != b.kind {
				return a.kind < b.kind
			}
			if a.id != b.id {
				return a.id < b.id
			}
			return a.org < b.org
		})
		sort.Slice(f.rows, func(i, j int) bool {
			a, b := f.rows[i].key, f.rows[j].key
			if a.store != b.store {
				return a.store < b.store
			}
			if a.name != b.name {
				return a.name < b.name
			}
			return a.id < b.id
		})
		f.self, f.seal = f, f.digest()
		if f.seal == "" {
			return nil, ErrHistoricalCASComponents
		}
		out = append(out, f)
	}
	if len(seenEvents) != len(joint.sources) || len(seenCurrent) != len(joint.current) || len(seenRows) != len(parentRows) || !reflect.DeepEqual(coveredRanges, joint.mongo.selection) || historicalCASComponentPageAlive(ctx, joint) != nil || joint.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrHistoricalCASComponents
	}
	return out, nil
}

func historicalCASComponentPageAlive(ctx context.Context, joint *WholeSourceJointPage) error {
	if joint == nil || joint.owner == nil || joint.page == nil || joint.index == nil {
		return ErrHistoricalCASComponents
	}
	c := joint.owner
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.alive(ctx) != nil || joint.page.owner != c || joint.index.owner != c || joint.index.auth != c.authenticated || c.now().Sub(joint.page.issued) > c.limits.PageTTL {
		return ErrHistoricalCASComponents
	}
	if !joint.page.consumed {
		return c.pageValid(joint.page)
	}
	if !joint.consumed || joint.page.sequence == 0 || joint.page.sequence > uint64(len(c.pages)) || c.pages[joint.page.sequence-1].CandidateSHA256 != joint.candidateSHA {
		return ErrHistoricalCASComponents
	}
	return nil
}

type HistoricalCASComponentLimits struct {
	MaxFrames, MaxSources, MaxOwners, MaxRows int
	MaxBytes                                  uint64
}

func DefaultHistoricalCASComponentLimits() HistoricalCASComponentLimits {
	return HistoricalCASComponentLimits{128, 512, 128, 32768, 64 << 20}
}
func (l HistoricalCASComponentLimits) valid() bool {
	return l.MaxFrames > 0 && l.MaxFrames <= 512 && l.MaxSources > 0 && l.MaxSources <= 512 && l.MaxOwners > 0 && l.MaxOwners <= 512 && l.MaxRows > 0 && l.MaxRows <= 131072 && l.MaxBytes > 0 && l.MaxBytes <= 256<<20
}

// Components contain only the original input instances in stable page order.
// The host must qualify/read/write/read back each complete component anew.
type HistoricalCASComponent struct {
	inputs []*HistoricalCASComponentInput
}
type HistoricalCASComponents struct {
	components []*HistoricalCASComponent
	inputPair  *HistoricalSourceInputPair
	inputIndex *WholeSourceJointIndex
	inputSeal  string
}

func (c *HistoricalCASComponent) Inputs() []*HistoricalCASComponentInput {
	if c == nil {
		return nil
	}
	return append([]*HistoricalCASComponentInput(nil), c.inputs...)
}
func (c *HistoricalCASComponents) Components() []*HistoricalCASComponent {
	if c == nil {
		return nil
	}
	return append([]*HistoricalCASComponent(nil), c.components...)
}
func (*HistoricalCASComponent) MarshalJSON() ([]byte, error)  { return nil, ErrSourceSerialization }
func (*HistoricalCASComponents) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*HistoricalCASComponent) String() string {
	return "private event input component; fresh SQL/Mongo/AI qualification required"
}

// Prepare checks the COMPLETE original event index before returning any batch.
// AI commands/negative operation selectors are outside this event footprint;
// this result cannot certify AI, full source cycles, commit, acceptance or DROP.
func PrepareHistoricalCASComponents(ctx context.Context, index *WholeSourceJointIndex, inputs []*HistoricalCASComponentInput, limits HistoricalCASComponentLimits) (*HistoricalCASComponents, error) {
	if ctx == nil || ctx.Err() != nil || index == nil || !index.complete || index.owner == nil || index.auth == nil || !index.auth.complete || len(inputs) == 0 || len(inputs) > 2_000_000 || !limits.valid() {
		return nil, ErrHistoricalCASComponents
	}
	return prepareHistoricalCASComponentGraph(ctx, index, inputs, limits, index.owner.binding, nil)
}

// Both callers keep their actual origin guards before sharing this pure graph.
// A source-input pair is never relabeled as an old coordinator qualification.
func prepareHistoricalCASComponentGraph(ctx context.Context, index *WholeSourceJointIndex, inputs []*HistoricalCASComponentInput, limits HistoricalCASComponentLimits, binding HistoricalCoordinatorBinding, pair *HistoricalSourceInputPair) (*HistoricalCASComponents, error) {
	seenSources := map[verifiedSourceKey]bool{}
	// Several genuine owners may share one source page. Its complete frozen
	// partition list must survive; page position alone must not join owners.
	type pageParts struct {
		count uint32
		seen  map[uint32]bool
	}
	seenSequences := map[uint64]*pageParts{}
	parent := make([]int, len(inputs))
	for i := range parent {
		parent[i] = i
	}
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	union := func(a, b int) {
		a, b = find(a), find(b)
		if a != b {
			if a > b {
				a, b = b, a
			}
			parent[b] = a
		}
	}
	readers := map[historicalCASRowKey][]int{}
	writers := map[historicalCASRowKey]int{}
	rowHashes := map[historicalCASRowKey]string{}
	ownerFrames := map[historicalCASOwnerKey]int{}
	ownerOrgs := map[historicalCASRowKey]uint64{}
	var totalBytes uint64
	var totalRows int
	for i, f := range inputs {
		if ctx.Err() != nil || f == nil || f.self != f || f.index != index || f.binding != binding || f.inputPair != pair || f.seal == "" || f.seal != f.digest() || f.sequence == 0 || len(f.sources) == 0 || len(f.owners) == 0 {
			return nil, ErrHistoricalCASComponents
		}
		if (f.partition == 0) != (f.partitions == 0) || f.partition > f.partitions || f.partitions > 512 {
			return nil, ErrHistoricalCASComponents
		}
		page := seenSequences[f.sequence]
		if page == nil {
			page = &pageParts{count: f.partitions, seen: map[uint32]bool{}}
			seenSequences[f.sequence] = page
		}
		if page.count != f.partitions || page.seen[f.partition] {
			return nil, ErrHistoricalCASComponents
		}
		page.seen[f.partition] = true
		for _, key := range f.sources {
			if seenSources[key] {
				return nil, ErrHistoricalCASComponents
			}
			seenSources[key] = true
		}
		for _, owner := range f.owners {
			if owner.id == 0 || owner.org == 0 || (owner.kind != "assessment" && owner.kind != "sheet") {
				return nil, ErrHistoricalCASComponents
			}
			identity := historicalCASRowKey{"owner", owner.kind, owner.id}
			if org, ok := ownerOrgs[identity]; ok && org != owner.org {
				return nil, ErrSourceOrganization
			}
			ownerOrgs[identity] = owner.org
			if old, ok := ownerFrames[owner]; ok {
				union(i, old)
			} else {
				ownerFrames[owner] = i
			}
		}
		local := map[historicalCASRowKey]bool{}
		for _, row := range f.rows {
			if !historicalCASFixedRow(row.key) || row.sha == "" || row.bytes == 0 || local[row.key] || row.bytes > (64<<30)-totalBytes {
				return nil, ErrHistoricalCASComponents
			}
			local[row.key] = true
			totalBytes += row.bytes
			totalRows++
			if totalRows > 4_000_000 {
				return nil, ErrHistoricalCASComponents
			}
			if old, ok := rowHashes[row.key]; ok && old != row.sha {
				return nil, ErrMongoBatchConflict
			}
			rowHashes[row.key] = row.sha
			readers[row.key] = append(readers[row.key], i)
			if row.write {
				if old, ok := writers[row.key]; ok {
					union(i, old)
				} else {
					writers[row.key] = i
				}
			}
		}
	}
	for _, page := range seenSequences {
		expected := int(page.count)
		if expected == 0 {
			expected = 1
		}
		if len(page.seen) != expected {
			return nil, ErrCoordinatorIncomplete
		}
	}
	if len(seenSources) != len(index.entries) {
		return nil, ErrCoordinatorIncomplete
	}
	for _, entry := range index.entries {
		if !seenSources[entry.Key] {
			return nil, ErrCoordinatorIncomplete
		}
	}
	// Read/read overlap does not join owners. A write joins every actual reader.
	for key, writer := range writers {
		for _, reader := range readers[key] {
			union(writer, reader)
		}
	}
	groups := map[int][]*HistoricalCASComponentInput{}
	for i, f := range inputs {
		root := find(i)
		groups[root] = append(groups[root], f)
	}
	result := &HistoricalCASComponents{}
	for _, frames := range groups {
		sort.Slice(frames, func(i, j int) bool { return historicalCASInputBefore(frames[i], frames[j]) })
		sources, owners := map[verifiedSourceKey]bool{}, map[historicalCASOwnerKey]bool{}
		var rows int
		var size uint64
		for _, f := range frames {
			for _, source := range f.sources {
				sources[source] = true
			}
			for _, owner := range f.owners {
				owners[owner] = true
			}
			rows += len(f.rows)
			for _, row := range f.rows {
				if row.bytes > limits.MaxBytes-size {
					return nil, ErrWholeSourceJointBounds
				}
				size += row.bytes
			}
		}
		if len(frames) > limits.MaxFrames || len(sources) > limits.MaxSources || len(owners) > limits.MaxOwners || rows > limits.MaxRows || size > limits.MaxBytes {
			return nil, ErrWholeSourceJointBounds
		}
		result.components = append(result.components, &HistoricalCASComponent{append([]*HistoricalCASComponentInput(nil), frames...)})
	}
	sort.Slice(result.components, func(i, j int) bool {
		return historicalCASInputBefore(result.components[i].inputs[0], result.components[j].inputs[0])
	})
	if ctx.Err() != nil {
		return nil, ErrHistoricalCASComponents
	}
	return result, nil
}

func historicalCASInputBefore(a, b *HistoricalCASComponentInput) bool {
	if a.sequence != b.sequence {
		return a.sequence < b.sequence
	}
	return a.partition < b.partition
}

func historicalCASFixedRow(key historicalCASRowKey) bool {
	if key.id == 0 {
		return false
	}
	if key.store == "mysql" {
		return key.name == "assessment" || key.name == "runtime_checkpoint" || key.name == "evaluation_outcome"
	}
	if key.store == "mongodb" {
		for _, name := range mongoBatchBusinessCollections {
			if key.name == name {
				return true
			}
		}
	}
	return false
}

// Plan captures only immutable inputs in the original SECOND RRRO/snapshot
// scopes after the actual two four-source captures matched. It neither builds
// an old joint/coordinator nor creates groups, qualifications or statements.
func PlanHistoricalSourceOwnerComponents(ctx context.Context, pair *HistoricalSourceInputPair, index *WholeSourceJointIndex, current *SQLResponsibilitySnapshot, mongoInput *MongoSnapshotInputEpoch, spool *sqlevaluation.SQLHistoricalCASSpool, limits HistoricalCASComponentLimits) (*HistoricalCASComponents, error) {
	if !limits.valid() || spool == nil || index.inputIndexIntact(ctx, pair, true) != nil || !pair.first.captureStopped || pair.second.captureStopped || pair.second.alive(ctx) != nil || current == nil || current.cycle != pair.second.sql || mongoInput == nil || mongoInput != pair.second.mongo || current.ValidateBorrowedSnapshot(ctx) != nil || mongoInput.ValidateBorrowedInputEpoch(ctx) != nil {
		return nil, ErrHistoricalCASComponents
	}
	ordered, err := index.inputEventOrder(ctx, pair, true)
	if err != nil {
		return nil, err
	}
	catalog, err := PrepareSQLCrossStoreResponsibilityCatalog(ctx, current, sqlevaluation.DefaultSQLCrossStoreLimits())
	if err != nil {
		return nil, err
	}
	pageSize := min(limits.MaxSources, index.limits.MaxRelatedSources)
	var inputs []*HistoricalCASComponentInput
	for start := 0; start < len(ordered); start += pageSize {
		page := ordered[start:min(start+pageSize, len(ordered))]
		fragments, e := planHistoricalSourceOwnerPage(ctx, pair, index, current, mongoInput, catalog, spool, page, uint64(start/pageSize+1))
		if e != nil {
			return nil, e
		}
		inputs = append(inputs, fragments...)
	}
	if _, err = index.inputEventOrder(ctx, pair, true); err != nil || pair.second.alive(ctx) != nil || current.ValidateBorrowedSnapshot(ctx) != nil || mongoInput.ValidateBorrowedInputEpoch(ctx) != nil {
		return nil, ErrHistoricalCASComponents
	}
	result, err := prepareHistoricalCASComponentGraph(ctx, index, inputs, limits, index.binding, pair)
	if err != nil {
		return nil, err
	}
	result.inputPair, result.inputIndex = pair, index
	result.inputSeal = result.sourceInputDigest()
	if result.inputSeal == "" {
		return nil, ErrHistoricalCASComponents
	}
	return result, nil
}

func planHistoricalSourceOwnerPage(ctx context.Context, pair *HistoricalSourceInputPair, index *WholeSourceJointIndex, current *SQLResponsibilitySnapshot, mongoInput *MongoSnapshotInputEpoch, catalog *SQLCrossStoreResponsibilityCatalog, spool *sqlevaluation.SQLHistoricalCASSpool, page []string, sequence uint64) ([]*HistoricalCASComponentInput, error) {
	if catalog == nil || catalog.current != current || catalog.index == nil || !catalog.Report().Complete || sequence == 0 {
		return nil, ErrHistoricalCASComponents
	}
	initialSources, err := index.InputEvents(ctx, pair, page)
	if err != nil {
		return nil, err
	}
	request, err := MongoHistoricalSQLBatchSelectors(initialSources)
	if err != nil {
		return nil, err
	}
	bl := sqlevaluation.DefaultSQLHistoricalOwnerBatchLimits()
	bl.MaxOwners = index.limits.MaxRelatedSources
	initial, err := PrepareSQLBusinessOwnerBatch(ctx, current, request, bl)
	if err != nil {
		return nil, err
	}
	// Exactly the old whole-source expansion: both actual assessment and sheet
	// associations select ALL original source frames, including a SQL-empty sheet.
	related := map[string]bool{}
	for _, source := range initialSources {
		facts, e := source.Facts()
		if e != nil {
			return nil, e
		}
		entry := index.entries[facts.EventID]
		related[facts.EventID] = true
		var actual *sqlevaluation.SQLHistoricalBatchOwnerFacts
		if facts.Submitted != nil {
			actual, e = initial.OwnerByAnswerSheet(entry.AnswerSheetID)
		} else {
			actual, e = initial.OwnerByAssessment(entry.AssessmentID)
		}
		if errors.Is(e, sqlevaluation.ErrSQLHistoricalOwnerAbsent) && facts.Submitted != nil {
			for _, id := range index.bySheet[entry.AnswerSheetID] {
				related[id] = true
			}
			continue
		}
		if e != nil {
			return nil, e
		}
		if actual == nil || actual.Snapshot().Owner.OrgID != facts.OrgID {
			return nil, ErrSourceOrganization
		}
		owner := actual.Snapshot().Owner
		for _, id := range index.byAssessment[owner.AssessmentID] {
			related[id] = true
		}
		for _, id := range index.bySheet[owner.AnswerSheetID] {
			related[id] = true
		}
	}
	if len(related) > index.limits.MaxRelatedSources {
		return nil, ErrWholeSourceJointBounds
	}
	ids := make([]string, 0, len(related))
	for id := range related {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	sources, err := index.InputEvents(ctx, pair, ids)
	if err != nil {
		return nil, err
	}
	request, err = MongoHistoricalSQLBatchSelectors(sources)
	if err != nil {
		return nil, err
	}
	batch, err := PrepareSQLBusinessOwnerBatch(ctx, current, request, bl)
	if err != nil {
		return nil, err
	}
	ml := DefaultMongoHistoricalOwnerBatchLimits()
	ml.MaxSources = index.limits.MaxRelatedSources
	parent, err := PrepareMongoSnapshotOwnerFootprint(ctx, mongoInput, batch.facts, sources, ml)
	if err != nil {
		return nil, err
	}
	selectors, present, absent, err := historicalSourceOwnerSelectors(index, parent)
	if err != nil {
		return nil, err
	}
	recipes, err := sqlevaluation.FreezeSQLHistoricalOwnerPlanningRecipes(ctx, batch.facts, catalog.index, selectors, spool, present, absent)
	if err != nil || len(recipes) == 0 || len(recipes) > 512 {
		return nil, ErrHistoricalCASComponents
	}
	byEvent := map[string]*VerifiedSourceEvent{}
	for _, source := range sources {
		facts, e := source.Facts()
		if e != nil || byEvent[facts.EventID] != nil {
			return nil, ErrHistoricalCASComponents
		}
		byEvent[facts.EventID] = source
	}
	currentSources := map[string]bool{}
	for _, id := range page {
		currentSources[id] = true
	}
	parentRows := map[historicalCASRowKey][]byte{}
	for name, rows := range parent.data {
		for _, raw := range rows {
			id, e := historicalSpoolMongoID(raw)
			key := historicalCASRowKey{"mongodb", name, id}
			if e != nil || parentRows[key] != nil {
				return nil, ErrHistoricalCASComponents
			}
			parentRows[key] = raw
		}
	}
	seenEvents, seenCurrent, seenRows := map[string]bool{}, map[string]bool{}, map[historicalCASRowKey]bool{}
	covered := mongoCASCloneSelection(mongoBatchSelection{})
	var out []*HistoricalCASComponentInput
	for _, recipe := range recipes {
		if !recipe.OwnerPartitionResolved() {
			return nil, ErrHistoricalCASComponents
		}
		events, e := recipe.SourceEventIDs()
		if e != nil || len(events) == 0 {
			return nil, ErrHistoricalCASComponents
		}
		f := &HistoricalCASComponentInput{index: index, inputPair: pair, binding: index.binding, sequence: sequence, partition: uint32(len(out) + 1), partitions: uint32(len(recipes)), sqlRecipe: recipe}
		var childSources []*VerifiedSourceEvent
		owners, writes := map[historicalCASOwnerKey]bool{}, map[historicalCASRowKey]bool{}
		for _, id := range events {
			source := byEvent[id]
			entry, ok := index.entries[id]
			if source == nil || !ok || seenEvents[id] {
				return nil, ErrHistoricalCASComponents
			}
			seenEvents[id] = true
			childSources = append(childSources, source)
			if owner, exists := parent.sqlOwners[entry.Key]; exists {
				owners[historicalCASOwnerKey{"assessment", owner.Owner.AssessmentID, owner.Owner.OrgID}] = true
				if owner.Owner.AnswerSheetID != 0 {
					owners[historicalCASOwnerKey{"sheet", owner.Owner.AnswerSheetID, owner.Owner.OrgID}] = true
				}
			} else if parent.sqlAbsent[entry.Key] {
				owners[historicalCASOwnerKey{"sheet", entry.AnswerSheetID, entry.OrgID}] = true
			} else {
				return nil, ErrHistoricalCASComponents
			}
			if currentSources[id] {
				f.sources = append(f.sources, entry.Key)
				seenCurrent[id] = true
			}
			// These are graph edges to genuine captured target rows only. The
			// SQL recipe still has groups=nil and no write execution authority.
			key, e := historicalSourcePotentialWrite(parent.sources[entry.Key])
			if e != nil {
				return nil, e
			}
			writes[key] = true
		}
		if len(f.sources) == 0 {
			return nil, ErrHistoricalCASComponents
		}
		child, e := PrepareMongoSnapshotOwnerFootprint(ctx, mongoInput, batch.facts, childSources, ml)
		if e != nil {
			return nil, e
		}
		for i, query := range mongoCASSelections(child.selection) {
			original, coveredRange := mongoCASSelections(parent.selection)[i], mongoCASSelections(covered)[i]
			for id := range query.ids {
				if !original.ids[id] {
					return nil, ErrHistoricalCASComponents
				}
				coveredRange.ids[id] = true
			}
		}
		f.mongoRead, e = freezeMongoSnapshotOwnerComponentReadRecipe(ctx, mongoInput, child)
		if e != nil {
			return nil, e
		}
		seenTargets := map[historicalCASRowKey]bool{}
		if e = recipe.RowDependencies(func(name string, id uint64, sha string, size uint64, write bool) error {
			if write { // A planning recipe must never contain old write groups.
				return ErrHistoricalCASComponents
			}
			key := historicalCASRowKey{"mysql", name, id}
			seenTargets[key] = true
			f.rows = append(f.rows, historicalCASRowInput{key, sha, size, writes[key]})
			return nil
		}); e != nil {
			return nil, e
		}
		for name, rows := range child.data {
			for _, raw := range rows {
				id, e := historicalSpoolMongoID(raw)
				key := historicalCASRowKey{"mongodb", name, id}
				if e != nil || !bytes.Equal(raw, parentRows[key]) {
					return nil, ErrMongoBatchConflict
				}
				seenRows[key], seenTargets[key] = true, true
				f.rows = append(f.rows, historicalCASRowInput{key, historicalSpoolSHA(raw), uint64(len(raw)), writes[key]})
			}
		}
		for target := range writes {
			if !seenTargets[target] {
				return nil, ErrHistoricalCASComponents
			}
		}
		for owner := range owners {
			f.owners = append(f.owners, owner)
		}
		sort.Slice(f.owners, func(i, j int) bool {
			a, b := f.owners[i], f.owners[j]
			if a.kind != b.kind {
				return a.kind < b.kind
			}
			if a.id != b.id {
				return a.id < b.id
			}
			return a.org < b.org
		})
		sort.Slice(f.rows, func(i, j int) bool {
			a, b := f.rows[i].key, f.rows[j].key
			if a.store != b.store {
				return a.store < b.store
			}
			if a.name != b.name {
				return a.name < b.name
			}
			return a.id < b.id
		})
		f.self, f.seal = f, f.digest()
		if f.seal == "" {
			return nil, ErrHistoricalCASComponents
		}
		out = append(out, f)
	}
	if len(seenEvents) != len(sources) || len(seenCurrent) != len(page) || len(seenRows) != len(parentRows) || !reflect.DeepEqual(covered, parent.selection) || parent.InputSHA256() == "" || index.inputIndexIntact(ctx, pair, true) != nil || pair.second.alive(ctx) != nil || batch.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrHistoricalCASComponents
	}
	return out, nil
}

// Source owner selectors include the actual complete generation/outcome graph
// read from the original snapshot, not merely IDs declared by current events.
func historicalSourceOwnerSelectors(index *WholeSourceJointIndex, footprint *MongoSnapshotOwnerFootprint) (sqlevaluation.SQLCrossStoreSelectors, map[string]uint64, map[string]uint64, error) {
	s := sqlevaluation.SQLCrossStoreSelectors{}
	present, absent := map[string]uint64{}, map[string]uint64{}
	assessments, orgs := map[uint64]bool{}, map[uint64]bool{}
	if index == nil || footprint == nil || footprint.InputSHA256() == "" {
		return s, nil, nil, ErrHistoricalCASComponents
	}
	for key, facts := range footprint.sources {
		entry, ok := index.entries[facts.EventID]
		if !ok || entry.Key != key || entry.FactsSHA256 != footprint.sourceFactsSHA[key] || entry.OrgID != facts.OrgID || facts.OrgID == 0 {
			return s, nil, nil, ErrHistoricalCASComponents
		}
		s.EventIDs = append(s.EventIDs, facts.EventID)
		orgs[facts.OrgID] = true
		if owner, exists := footprint.sqlOwners[key]; exists {
			if owner.Owner.AssessmentID == 0 || owner.Owner.OrgID != facts.OrgID || entry.AssessmentID != 0 && entry.AssessmentID != owner.Owner.AssessmentID || entry.AnswerSheetID != 0 && entry.AnswerSheetID != owner.Owner.AnswerSheetID {
				return s, nil, nil, ErrHistoricalCASComponents
			}
			present[facts.EventID] = owner.Owner.AssessmentID
			assessments[owner.Owner.AssessmentID] = true
		} else if footprint.sqlAbsent[key] && facts.Submitted != nil && entry.AnswerSheetID != 0 {
			absent[facts.EventID] = entry.AnswerSheetID
		} else {
			return s, nil, nil, ErrHistoricalCASComponents
		}
	}
	sort.Strings(s.EventIDs)
	s.AssessmentIDs, s.OrganizationIDs = mongoBatchIDs(assessments), mongoBatchIDs(orgs)
	for _, id := range mongoBatchIDs(footprint.selection.sheets) {
		s.MongoOwners = append(s.MongoOwners, sqlevaluation.SQLCrossStoreOwnerReference{Kind: "AnswerSheet", ID: strconv.FormatUint(id, 10)})
	}
	for _, id := range mongoBatchIDs(footprint.selection.generations) {
		s.MongoOwners = append(s.MongoOwners, sqlevaluation.SQLCrossStoreOwnerReference{Kind: "ReportGeneration", ID: strconv.FormatUint(id, 10)})
	}
	return s, present, absent, nil
}

func historicalSourcePotentialWrite(facts *DecodedSourceEvent) (historicalCASRowKey, error) {
	if facts == nil {
		return historicalCASRowKey{}, ErrHistoricalCASComponents
	}
	key := historicalCASRowKey{"mysql", "assessment", 0}
	value := facts.BusinessIDs["assessment_id"]
	switch facts.EventType {
	case "evaluation.requested", "evaluation.retry.requested", "evaluation.failed":
	case "evaluation.outcome.committed":
		key.name, value = "evaluation_outcome", facts.BusinessIDs["outcome_id"]
	case "answersheet.submitted":
		key.store, key.name, value = "mongodb", "answersheets", facts.BusinessIDs["answersheet_id"]
	case "interpretation.report.generated":
		key.store, key.name, value = "mongodb", "report_generations", facts.BusinessIDs["generation_id"]
	default:
		return historicalCASRowKey{}, ErrSourceEventType
	}
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil || id == 0 || strconv.FormatUint(id, 10) != value || key.store == "mongodb" && id > 1<<63-1 {
		return historicalCASRowKey{}, ErrHistoricalCASComponents
	}
	key.id = id
	return key, nil
}

func (c *HistoricalCASComponents) sourceInputDigest() string {
	if c == nil || c.inputPair == nil || c.inputPair.self != c.inputPair || c.inputIndex == nil {
		return ""
	}
	parts := []string{"historical-source-owner-components/v1", c.inputIndex.inputSeal}
	for _, component := range c.components {
		if component == nil || len(component.inputs) == 0 {
			return ""
		}
		for _, input := range component.inputs {
			if input == nil || input.self != input || input.index != c.inputIndex || input.inputPair != c.inputPair || input.seal == "" || input.seal != input.digest() {
				return ""
			}
			parts = append(parts, input.seal)
		}
	}
	return mongoOwnerHashParts(parts...)
}

// Ending capture/releasing auth never renews a permit. This checks the same
// immutable component/input instances plus the actual two source spools.
func (c *HistoricalCASComponents) ValidateInputSources(ctx context.Context, pair *HistoricalSourceInputPair) error {
	if c == nil || pair != c.inputPair || c.inputIndex == nil || c.inputSeal == "" || c.inputSeal != c.sourceInputDigest() || c.inputIndex.inputIndexIntact(ctx, pair, false) != nil || pair.ValidateFrozen(ctx) != nil {
		return ErrHistoricalCASComponents
	}
	if _, err := c.inputIndex.inputEventOrder(ctx, pair, false); err != nil {
		return err
	}
	return nil
}
