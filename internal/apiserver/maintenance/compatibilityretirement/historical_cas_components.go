package retirement

import (
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
type HistoricalCASComponents struct{ components []*HistoricalCASComponent }

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
	var find func(int) int
	find = func(i int) int {
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
		if ctx.Err() != nil || f == nil || f.self != f || f.index != index || f.binding != index.owner.binding || f.seal == "" || f.seal != f.digest() || f.sequence == 0 || len(f.sources) == 0 || len(f.owners) == 0 {
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
