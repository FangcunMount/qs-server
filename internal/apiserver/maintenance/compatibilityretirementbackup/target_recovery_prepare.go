package compatibilityretirementbackup

import (
	"context"
	"encoding/hex"
	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"regexp"
	"strconv"
	"strings"
)

func PrepareTargetRecovery(ctx context.Context, a *Archive, b TargetRecoveryBorrowed, r TargetRecoveryRequest, dir string, w *fence.MaintenanceWindow) (*TargetRecoveryPlan, error) {
	if ctx == nil || ctx.Err() != nil || a == nil || b.SQL == nil || b.Mongo == nil || mongo.SessionFromContext(ctx) != nil {
		return nil, ErrRecoveryBinding
	}
	if !hashPattern.MatchString(r.ManifestSHA256) || !hashPattern.MatchString(r.SQLNonTargetSHA256) || !hashPattern.MatchString(r.MongoNonTargetSHA256) || !runPattern.MatchString(r.ActualRunID) || r.SourceSHA != a.data.Approval.SourceSHA || r.OperationID != a.data.Approval.OperationID || r.OriginalRunID != a.data.Approval.RunID || r.ArchiveSHA256 != a.digest || !sourcePattern.MatchString(r.SourceSHA) || !runPattern.MatchString(r.OperationID) {
		return nil, ErrRecoveryBinding
	}
	if !targetSupportedHead(r.SQLHead, a.data.Inventory.Bindings["mysql"].Version) || !targetSupportedHead(r.MongoHead, a.data.Inventory.Bindings["mongodb"].Version) {
		return nil, ErrRecoveryHead
	}
	if e := a.verifyAssets(ctx); e != nil {
		return nil, e
	}
	for _, structure := range a.data.SQL {
		if targetRecoveryForeignKeys(structure.DDL) != nil {
			return nil, ErrStructure
		}
	}
	p := &TargetRecoveryPlan{archive: a, borrowed: b, request: r, window: w}
	p.self = p
	if w != nil {
		receipt, e := w.Diagnostic(ctx)
		want := fence.WindowBinding{TargetSHA256: fence.MaintenanceWindowTargetSHA256(), SourceSHA: r.SourceSHA, OperationID: r.OperationID, ManifestSHA256: r.ManifestSHA256, OriginalRunID: r.OriginalRunID}
		if e != nil || receipt.Binding != want || !receipt.BudgetOnly || !receipt.DirectoryLeaseHeld {
			return nil, ErrRecoveryBinding
		}
		p.budget = w.RecoveryContext
	}
	if e := p.checkBases(ctx, false); e != nil {
		return nil, e
	}
	for i := 0; i < 4; i++ {
		present, e := p.checkTarget(ctx, i)
		if e != nil {
			return nil, e
		}
		state := "missing_unproven"
		if present {
			state = "existing_exact"
		}
		database := "mysql"
		if i == 3 {
			database = "mongodb"
		}
		p.observations[i] = TargetRecoveryObservation{database, targetNames[i], state, a.data.Inventory.Targets[i].Records}
	}
	j, e := newTargetJournal(dir, r)
	if e != nil {
		return nil, e
	}
	p.journal = j
	return p, nil
}
func targetSupportedHead(actual, original uint64) bool {
	return original > 0 && actual == original
}
func (p *TargetRecoveryPlan) checkBases(ctx context.Context, held bool) error {
	if p == nil || p.self != p || p.blocked || ctx == nil || ctx.Err() != nil || p.borrowed.SQL == nil || p.borrowed.Mongo == nil || mongo.SessionFromContext(ctx) != nil {
		return ErrRecoveryBinding
	}
	sb := p.archive.data.Inventory.Bindings["mysql"]
	sb.Version = p.request.SQLHead
	if p.bMigration != nil {
		if p.bMigration.proof == nil || p.journal == nil || !p.journal.has("b-migration-intent", p.bMigration.intentSHA) || !p.journal.has("b-migration-result", p.bMigration.resultSHA) {
			return ErrRecoveryJournal
		}
		if e := p.bMigration.proof.VerifyAfter(ctx, p.borrowed.SQL, p.borrowed.Mongo); e != nil {
			return ErrRecoveryHead
		}
		sb = p.bMigration.sqlBinding
	}
	if _, e := sqlState(ctx, p.borrowed.SQL, sb); e != nil {
		return ErrRecoveryHead
	}
	settings, e := readSQL(ctx, p.borrowed.SQL, "SELECT @@session.foreign_key_checks,@@session.autocommit,CONNECTION_ID(),DATABASE()")
	if e != nil || len(settings) != 1 || len(settings[0]) != 4 || cell(settings[0], 0) != "1" || cell(settings[0], 1) != "1" || cell(settings[0], 2) == "" || cell(settings[0], 3) == "" {
		return ErrRecoveryState
	}
	if held && (cell(settings[0], 2) != p.sqlConnection || cell(settings[0], 3) != p.sqlNamespace) {
		return ErrIdentity
	}
	p.sqlConnection = cell(settings[0], 2)
	p.sqlNamespace = cell(settings[0], 3)
	if e = p.checkSQLNoTransaction(ctx); e != nil {
		return e
	}
	defs, e := readSQLCatalog(ctx, p.borrowed.SQL)
	if e != nil {
		return e
	}
	non, e := targetSQLNonTarget(defs, p.sqlNamespace)
	if e != nil {
		return e
	}
	if non != p.request.SQLNonTargetSHA256 || (held && non != p.sqlBaseline) {
		return ErrStructure
	}
	p.sqlBaseline = non
	cols, mdefs, e := mongoCatalog(ctx, p.borrowed.Mongo)
	if e != nil {
		return e
	}
	mb := p.archive.data.Inventory.Bindings["mongodb"]
	mb.Version = p.request.MongoHead
	if p.bMigration != nil {
		mb = p.bMigration.mongoBinding
	}
	if _, e = mongoState(ctx, p.borrowed.Mongo, mb, cols); e != nil {
		return ErrRecoveryHead
	}
	mnon := targetMongoNonTarget(mdefs)
	expectedMongoNonTarget := p.request.MongoNonTargetSHA256
	if p.bMigration != nil {
		stable, e := targetBStableMongoNonTarget(mdefs)
		if e != nil || stable != p.bMigration.stableMongoSchema {
			return ErrStructure
		}
		expectedMongoNonTarget = p.bMigration.afterMongoSchema
	}
	if mnon != expectedMongoNonTarget || (held && mnon != p.mongoBaseline) {
		return ErrStructure
	}
	p.mongoBaseline = mnon
	var hello bson.Raw
	if p.borrowed.Mongo.Client().Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello) != nil {
		return ErrIdentity
	}
	process, ok := hello.Lookup("topologyVersion", "processId").ObjectIDOK()
	if !ok || process.IsZero() {
		return ErrIdentity
	}
	if held && p.mongoProcess != process.Hex() {
		return ErrIdentity
	}
	p.mongoProcess = process.Hex()
	if p.journal != nil {
		return p.journal.validate()
	}
	return nil
}

// Exclude only owned target rows, never non-target constraints or triggers.
func targetSQLNonTarget(defs map[string]any, namespace string) (string, error) {
	non := map[string]any{}
	for key, value := range defs {
		if strings.HasPrefix(key, "table:") && targetSQLName(strings.TrimPrefix(key, "table:")) {
			continue
		}
		if key == "constraints" || key == "inbound_constraints" {
			rows, ok := value.([][]*string)
			if !ok {
				return "", ErrStructure
			}
			filtered := [][]*string{}
			for _, row := range rows {
				if key == "inbound_constraints" {
					if len(row) < 7 || cell(row, 0) != namespace || !targetSQLName(cell(row, 1)) {
						return "", ErrStructure
					}
					continue
				}
				if len(row) < 4 {
					return "", ErrStructure
				}
				if targetSQLName(cell(row, 0)) {
					continue
				}
				filtered = append(filtered, row)
			}
			non[key] = filtered
			continue
		}
		if key == "triggers" {
			rows, ok := value.([][]*string)
			if !ok {
				return "", ErrStructure
			}
			for _, row := range rows {
				if targetSQLName(cell(row, 1)) {
					return "", ErrStructure
				}
			}
		}
		non[key] = value
	}
	return jsonSHA(non), nil
}
func targetSQLName(name string) bool {
	for _, n := range targetNames[:3] {
		if name == n {
			return true
		}
	}
	return false
}
func targetMongoNonTarget(defs map[string]any) string {
	out := map[string]any{}
	for key, value := range defs {
		if key != "collection:domain_event_outbox" && key != "indexes:domain_event_outbox" {
			out[key] = value
		}
	}
	return jsonSHA(out)
}
func (p *TargetRecoveryPlan) checkTarget(ctx context.Context, i int) (bool, error) {
	if i < 0 || i >= 4 {
		return false, ErrRecoveryBinding
	}
	if i < 3 {
		rows, e := readSQL(ctx, p.borrowed.SQL, "SELECT TABLE_TYPE FROM information_schema.tables WHERE table_schema=DATABASE() AND BINARY table_name=BINARY ?", targetNames[i])
		if e != nil {
			return false, e
		}
		if len(rows) == 0 {
			return false, nil
		}
		if len(rows) != 1 || cell(rows[0], 0) != "BASE TABLE" {
			return false, ErrRecoveryState
		}
		structure, e := sqlStructure(ctx, p.borrowed.SQL, i)
		if e != nil || !sqlStructuresEqual(structure, p.archive.data.SQL[i]) {
			return false, ErrRecoveryState
		}
		if e = verifySQLContent(ctx, p.borrowed.SQL, structure, p.archive.data.Inventory.Targets[i], i); e != nil {
			return false, e
		}
		return true, nil
	}
	cols, _, e := mongoCatalog(ctx, p.borrowed.Mongo)
	if e != nil {
		return false, e
	}
	col, ok := cols[targetNames[3]]
	if !ok {
		return false, nil
	}
	kind, ok := col.Lookup("type").StringValueOK()
	if !ok || kind != "collection" {
		return false, ErrRecoveryState
	}
	structure, e := ReadOrderedMongoSchema(ctx, p.borrowed.Mongo)
	if e != nil || !schemaEqual(structure.data, p.archive.data.Mongo) {
		return false, ErrRecoveryState
	}
	typ, id, ok := bson.Raw(structure.data.Collection).Lookup("info", "uuid").BinaryOK()
	if !ok || typ != 4 || len(id) != 16 {
		return false, ErrIdentity
	}
	observed := hex.EncodeToString(id)
	if p.mongoTargetUUID == "" {
		originalKind, originalID, valid := bson.Raw(p.archive.data.Mongo.Collection).Lookup("info", "uuid").BinaryOK()
		if !valid || originalKind != 4 || len(originalID) != 16 {
			return false, ErrIdentity
		}
		p.mongoTargetUUID = hex.EncodeToString(originalID)
	}
	if p.observations[3].State != "dropped_native" && p.mongoTargetUUID != observed {
		return false, ErrIdentity
	}
	p.mongoTargetUUID = observed
	if e = verifyMongoContent(ctx, p.borrowed.Mongo, p.archive.data.Inventory.Targets[3]); e != nil {
		return false, e
	}
	return true, nil
}
func (p *TargetRecoveryPlan) validDrop(i int) bool {
	if i < 0 || i >= 4 {
		return false
	}
	d := p.drop[i]
	if d == nil || d.self != d || d.plan != p || d.index != i || !d.nativeAbsent || p.journal == nil {
		return false
	}
	name := strconv.Itoa(i)
	return p.journal.has(name+"-drop-intent", d.intentHash) && p.journal.has(name+"-drop-result", d.resultHash)
}
func RecoverTargets(ctx context.Context, p *TargetRecoveryPlan) (*TargetRecoveryVerification, error) {
	if p == nil || p.self != p {
		return nil, ErrRecoveryBinding
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, o := range p.observations {
		if o.State != "existing_exact" && o.State != "restored_exact" && !p.validDrop(i) {
			return nil, ErrRecoveryAuthority
		}
	}
	if p.budget == nil {
		return nil, ErrBudget
	}
	q, c, e := p.budget(ctx)
	if e != nil || q == nil || c == nil {
		return nil, ErrBudget
	}
	defer c()
	if _, ok := q.Deadline(); !ok || q.Err() != nil {
		return nil, ErrBudget
	}
	if e = p.checkBases(q, true); e != nil {
		p.blocked = true
		return nil, e
	}
	if e = p.archive.verifyAssets(q); e != nil {
		p.blocked = true
		return nil, e
	}
	for i := 0; i < 4; i++ {
		present, e := p.checkTarget(q, i)
		if e != nil {
			p.blocked = true
			return nil, e
		}
		if present {
			if p.observations[i].State != "existing_exact" && p.observations[i].State != "restored_exact" {
				p.blocked = true
				return nil, ErrRecoveryState
			}
			continue
		}
		if !p.validDrop(i) || p.observations[i].State == "restored_exact" {
			p.blocked = true
			return nil, ErrRecoveryAuthority
		}
		if i < 3 {
			e = p.recoverSQL(q, i)
		} else {
			e = p.recoverMongo(q)
		}
		if e != nil {
			p.blocked = true
			return nil, e
		}
	}
	if e = p.checkBases(q, true); e != nil {
		p.blocked = true
		return nil, e
	}
	for i := 0; i < 4; i++ {
		present, e := p.checkTarget(q, i)
		if e != nil || !present {
			p.blocked = true
			return nil, ErrContent
		}
	}
	v := &TargetRecoveryVerification{summary: p.summaryLocked()}
	v.self = v
	return v, nil
}

// Visibility is checked, never enabled here. Session defaults cannot prove the
// absence of a caller-owned START TRANSACTION on an autocommit connection.
func (p *TargetRecoveryPlan) checkSQLNoTransaction(ctx context.Context) error {
	instrument, e := readSQL(ctx, p.borrowed.SQL, "SELECT ENABLED FROM performance_schema.setup_instruments WHERE NAME='transaction'")
	consumer, ce := readSQL(ctx, p.borrowed.SQL, "SELECT ENABLED FROM performance_schema.setup_consumers WHERE NAME='events_transactions_current'")
	thread, te := readSQL(ctx, p.borrowed.SQL, "SELECT THREAD_ID FROM performance_schema.threads WHERE PROCESSLIST_ID=CONNECTION_ID()")
	if e != nil || ce != nil || te != nil || len(instrument) != 1 || len(consumer) != 1 || len(thread) != 1 || cell(instrument[0], 0) != "YES" || cell(consumer[0], 0) != "YES" || cell(thread[0], 0) == "" {
		return ErrRecoveryTransaction
	}
	rows, e := readSQL(ctx, p.borrowed.SQL, "SELECT STATE,END_EVENT_ID FROM performance_schema.events_transactions_current WHERE THREAD_ID=?", cell(thread[0], 0))
	if e != nil || len(rows) > 1 {
		return ErrRecoveryTransaction
	}
	if len(rows) == 1 && (len(rows[0]) != 2 || (cell(rows[0], 0) != "COMMITTED" && cell(rows[0], 0) != "ROLLED BACK") || rows[0][1] == nil) {
		return ErrRecoveryTransaction
	}
	active, e := readSQL(ctx, p.borrowed.SQL, "SELECT trx_mysql_thread_id FROM information_schema.innodb_trx WHERE trx_mysql_thread_id=CONNECTION_ID()")
	if e != nil || len(active) != 0 {
		return ErrRecoveryTransaction
	}
	return nil
}

var recoveryFK = regexp.MustCompile("REFERENCES `([^`]+)` (\\([^)]*\\))")

func targetRecoveryForeignKeys(ddl string) error {
	matches := recoveryFK.FindAllStringSubmatch(ddl, -1)
	if strings.Count(ddl, "REFERENCES") != len(matches) {
		return ErrStructure
	}
	for _, m := range matches {
		if len(m) != 3 || targetSQLName(m[1]) {
			return ErrStructure
		}
	}
	return nil
}
