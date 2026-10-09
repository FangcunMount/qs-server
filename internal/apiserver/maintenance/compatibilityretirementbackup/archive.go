package compatibilityretirementbackup

import (
	"context"
	"database/sql"
	"encoding/json"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"time"
)

type BorrowedSources struct {
	SQL   *sql.Tx
	Mongo *mongo.Database
}
type sqlMetadata struct {
	Identity    [][]*string     `json:"identity"`
	GrantsHash  string          `json:"grants_hash"`
	Permissions map[string]bool `json:"permission_facts"`
	Schema      map[string]any  `json:"schema"`
}
type mongoMetadata struct {
	Hello          any             `json:"hello"`
	PrivilegesHash string          `json:"effective_privileges_hash"`
	Permissions    map[string]bool `json:"permission_facts"`
	Schema         map[string]any  `json:"schema"`
	Version        string          `json:"version"`
}
type registration struct {
	Version                int      `json:"version"`
	Approval               Approval `json:"approval"`
	Files                  []string `json:"files"`
	ContainsOriginalBodies bool     `json:"contains_original_bodies"`
	PurgeAfterAcceptance   bool     `json:"purge_after_acceptance"`
	ResumeAllowed          bool     `json:"resume_allowed"`
}

func privateDirectory(dir string) error {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return ErrPrivate
	}
	for p := dir; ; p = filepath.Dir(p) {
		st, e := os.Lstat(p)
		if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return ErrPrivate
		}
		if p == dir && st.Mode().Perm() != 0700 {
			return ErrPrivate
		}
		if st.Mode().Perm()&0022 != 0 && st.Mode()&os.ModeSticky == 0 {
			return ErrPrivate
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	return nil
}
func newPrivateFile(path string) (*os.File, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return nil, ErrPrivate
	}
	return f, nil
}
func openPrivateFile(path string) (*os.File, error) {
	// A FIFO must not block before the descriptor's regular-file check. The
	// flag does not weaken O_NOFOLLOW or change reads of accepted regular files.
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return nil, ErrPrivate
	}
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Mode().Perm() != 0600 {
		_ = f.Close()
		return nil, ErrPrivate
	}
	return f, nil
}
func writePrivate(path string, b []byte) error {
	f, e := newPrivateFile(path)
	if e != nil {
		return e
	}
	_, writeErr := f.Write(b)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return ErrPrivate
	}
	// A synced file alone does not make its new directory entry durable. The
	// registration must survive a crash before any original body is copied.
	return syncDirectory(filepath.Dir(path))
}
func registrationNames() []string {
	return []string{"mysql-domain_event_outbox.source.ndjson", "mysql-ai_bridge_commands.source.ndjson", "mysql-ai_messaging_legacy_commands.source.ndjson", "mongodb-domain_event_outbox.source.bsonframes", "archive.private.json"}
}
func approvalValid(a Approval) bool {
	for _, v := range []string{a.InventorySHA256, a.SQLMetadataSHA256, a.MongoMetadataSHA256, a.OrderedMongoSchemaSHA256, a.RequestHash} {
		if !hashPattern.MatchString(v) {
			return false
		}
	}
	return sourcePattern.MatchString(a.SourceSHA) && runPattern.MatchString(a.OperationID) && runPattern.MatchString(a.RunID)
}
func validateInventory(raw []byte, approved Approval) (inventory, error) {
	var r inventory
	if exactJSON(raw, &r) != nil || r.Format != 2 || r.Kind != "readonly_compatibility_inventory" || r.SourceSHA != approved.SourceSHA || r.OperationID != approved.OperationID || r.RunID != approved.RunID || r.RequestHash != approved.RequestHash || r.ErrorCategory != "none" || !r.Complete || r.DropReady || !r.Diagnostic || r.Protocol != "mysql_cast_binary_columns_pk_order_v2+mongodb_server_bson_pk_order_v2" || len(r.Targets) != 4 || len(r.Bindings) != 2 || !hashPattern.MatchString(r.BoundaryHash) {
		return r, ErrApproval
	}
	expectedTargets := [4][3]string{{"mysql", "domain_event_outbox", "base_table"}, {"mysql", "ai_bridge_commands", "base_table"}, {"mysql", "ai_messaging_legacy_commands", "base_table"}, {"mongodb", "domain_event_outbox", "collection"}}
	if r.TargetHash != jsonSHA(expectedTargets) {
		return r, ErrApproval
	}
	for database, b := range r.Bindings {
		if database != "mysql" && database != "mongodb" {
			return r, ErrApproval
		}
		if !hashPattern.MatchString(b.IdentityHash) || !hashPattern.MatchString(b.AnchorHash) || !hashPattern.MatchString(b.CatalogHash) || !hashPattern.MatchString(b.NonTargetHash) || b.Version == 0 || b.Dirty || !b.HeadMatch || !b.IdentityMatch || !b.MetadataComplete || b.ErrorCategory != "none" {
			return r, ErrApproval
		}
		if database == "mysql" && (b.AnchorHash != b.IdentityHash || b.GenerationHash != "") {
			return r, ErrApproval
		}
		if database == "mongodb" && !hashPattern.MatchString(b.GenerationHash) {
			return r, ErrApproval
		}
	}
	for i, s := range r.Targets {
		b := s.Boundary
		if s.Database != expectedTargets[i][0] || s.Name != expectedTargets[i][1] || s.Kind != expectedTargets[i][2] || s.SourceFile != sourceNames[i] || s.ErrorCategory != "none" || b.Database != s.Database || b.Name != s.Name || b.Kind != s.Kind || !s.Present || !s.Complete || !b.Present || s.NextCycle || s.Passes != 2 || !hashPattern.MatchString(s.SchemaHash) || !hashPattern.MatchString(s.IdentityHash) {
			return r, ErrApproval
		}
	}
	return r, nil
}

// Capture copies and authenticates actual full v2 sources to EOF, then checks
// every raw row against a complete borrowed database read. It does not create
// a transaction, close a pool, fence a writer or manufacture source approval.
func Capture(ctx context.Context, borrowed BorrowedSources, approved Approval, in Inputs, dir string) (archive *Archive, result error) {
	if ctx == nil || ctx.Err() != nil || borrowed.SQL == nil || borrowed.Mongo == nil || !approvalValid(approved) || privateDirectory(dir) != nil {
		return nil, ErrApproval
	}
	ctx, c := context.WithTimeout(ctx, 1500*time.Second)
	defer c()
	reportBytes, e := readPrivate(in.Inventory, metadataBudget)
	if e != nil || sha(reportBytes) != approved.InventorySHA256 {
		return nil, ErrApproval
	}
	r, e := validateInventory(reportBytes, approved)
	if e != nil {
		return nil, e
	}
	sqlBytes, e := readPrivate(in.SQLMetadata, metadataBudget)
	if e != nil || sha(sqlBytes) != approved.SQLMetadataSHA256 {
		return nil, ErrApproval
	}
	mongoBytes, e := readPrivate(in.MongoMetadata, metadataBudget)
	if e != nil || sha(mongoBytes) != approved.MongoMetadataSHA256 {
		return nil, ErrApproval
	}
	var sm sqlMetadata
	var mm mongoMetadata
	if exactJSON(sqlBytes, &sm) != nil || exactJSON(mongoBytes, &mm) != nil || jsonSHA(sm.Schema) != r.Bindings["mysql"].CatalogHash || jsonSHA(mm.Schema) != r.Bindings["mongodb"].CatalogHash || !strings.HasPrefix(mm.Version, "7.") {
		return nil, ErrStructure
	}
	if _, e = sqlState(ctx, borrowed.SQL, r.Bindings["mysql"]); e != nil {
		return nil, e
	}
	grants, e := readSQL(ctx, borrowed.SQL, "SHOW GRANTS FOR CURRENT_USER")
	if e != nil || jsonSHA(grants) != sm.GrantsHash {
		return nil, ErrIdentity
	}
	defs, e := readSQLCatalog(ctx, borrowed.SQL)
	if e != nil || jsonSHA(defs) != jsonSHA(sm.Schema) {
		return nil, ErrStructure
	}
	// Seal the target-exclusion recovery projection while reading the actual
	// approved source catalog. Do not derive it from caller-provided recovery
	// requests or compare it with inventory's target-owned-FK projection.
	sqlNamespace, e := readSQL(ctx, borrowed.SQL, "SELECT DATABASE()")
	if e != nil || len(sqlNamespace) != 1 || len(sqlNamespace[0]) != 1 || cell(sqlNamespace[0], 0) == "" {
		return nil, ErrIdentity
	}
	sqlRecoveryNonTarget, e := targetSQLNonTarget(defs, cell(sqlNamespace[0], 0))
	if e != nil {
		return nil, e
	}
	// SHOW CREATE TABLE excludes trigger bodies. The current four historical
	// layouts have none; a newly discovered target trigger is unsupported, not
	// silently omitted from a supposedly complete rollback asset.
	if triggers, ok := defs["triggers"].([][]*string); ok {
		for _, row := range triggers {
			for _, name := range targetNames[:3] {
				if cell(row, 1) == name {
					return nil, ErrStructure
				}
			}
		}
	}

	collections, mdefs, e := mongoCatalog(ctx, borrowed.Mongo)
	if e != nil || jsonSHA(mdefs) != jsonSHA(mm.Schema) {
		return nil, ErrStructure
	}
	if _, e = mongoState(ctx, borrowed.Mongo, r.Bindings["mongodb"], collections); e != nil {
		return nil, e
	}
	ordered, e := ReadOrderedMongoSchema(ctx, borrowed.Mongo)
	if e != nil || ordered.digest != approved.OrderedMongoSchemaSHA256 {
		return nil, ErrApproval
	}
	if e = verifyMongoObject(ordered.data, r.Targets[3]); e != nil {
		return nil, e
	}
	var sourceHello bson.Raw
	if borrowed.Mongo.Client().Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&sourceHello) != nil {
		return nil, ErrIdentity
	}
	processID, ok := sourceHello.Lookup("topologyVersion", "processId").ObjectIDOK()
	if !ok || processID.IsZero() {
		return nil, ErrIdentity
	}
	m := manifest{InventoryRaw: append([]byte(nil), reportBytes...), SourceMongoNamespace: borrowed.Mongo.Name(), SourceMongoProcessID: processID.Hex(), Version: 1, Approval: approved, Inventory: r, Mongo: ordered.data, SQLRecoveryNonTargetHash: sqlRecoveryNonTarget, SQLMetadataHash: sha(sqlBytes), MongoMetadataHash: sha(mongoBytes), OrderedMongoSchemaHash: ordered.digest}
	for i := 0; i < 3; i++ {
		s, e := sqlStructure(ctx, borrowed.SQL, i)
		if e != nil {
			return nil, e
		}
		normalized := increment.ReplaceAllString(s.DDL, "")
		row := [][]*string{{&targetNames[i], &normalized}}
		if jsonSHA(row) != r.Targets[i].SchemaHash || parts("mysql-object-v1", targetNames[i], r.Targets[i].SchemaHash) != r.Targets[i].IdentityHash {
			return nil, ErrStructure
		}
		m.SQL[i] = s
	}
	// The complete fixed file registry is durable before the first original byte
	// is copied. Failed/partial assets remain registered and never resume.
	reg := registration{Version: 1, Approval: approved, Files: registrationNames(), ContainsOriginalBodies: true, PurgeAfterAcceptance: true}
	regBytes, e := json.Marshal(reg)
	if e != nil || writePrivate(filepath.Join(dir, "assets.private.json"), regBytes) != nil {
		return nil, ErrPrivate
	}
	for i := 0; i < 4; i++ {
		columns := m.SQL[0].Columns
		if i < 3 {
			columns = m.SQL[i].Columns
		}
		asset, e := copySource(ctx, dir, i, in.Sources[i], r.Targets[i], columns)
		if e != nil {
			return nil, e
		}
		m.Assets[i] = asset
	}
	for i := 0; i < 3; i++ {
		if e = verifySQLContent(ctx, borrowed.SQL, m.SQL[i], r.Targets[i], i); e != nil {
			return nil, e
		}
	}
	if e = verifyMongoContent(ctx, borrowed.Mongo, r.Targets[3]); e != nil {
		return nil, e
	}
	// Schema, UUID, migration head, identity and content are re-read. Equal
	// reads are diagnostic consistency, not a production write fence.
	defsEnd, e := readSQLCatalog(ctx, borrowed.SQL)
	if e != nil || jsonSHA(defsEnd) != jsonSHA(defs) {
		return nil, ErrStructure
	}
	if _, e = sqlState(ctx, borrowed.SQL, r.Bindings["mysql"]); e != nil {
		return nil, e
	}
	grantsEnd, e := readSQL(ctx, borrowed.SQL, "SHOW GRANTS FOR CURRENT_USER")
	if e != nil || jsonSHA(grantsEnd) != sm.GrantsHash {
		return nil, ErrIdentity
	}
	for i := 0; i < 3; i++ {
		s, e := sqlStructure(ctx, borrowed.SQL, i)
		if e != nil || !reflect.DeepEqual(s, m.SQL[i]) {
			return nil, ErrStructure
		}
		if e = verifySQLContent(ctx, borrowed.SQL, m.SQL[i], r.Targets[i], i); e != nil {
			return nil, e
		}
	}
	orderedEnd, e := ReadOrderedMongoSchema(ctx, borrowed.Mongo)
	if e != nil || orderedEnd.digest != ordered.digest {
		return nil, ErrStructure
	}
	endCollections, endDefs, e := mongoCatalog(ctx, borrowed.Mongo)
	if e != nil || jsonSHA(endDefs) != jsonSHA(mdefs) {
		return nil, ErrStructure
	}
	if _, e = mongoState(ctx, borrowed.Mongo, r.Bindings["mongodb"], endCollections); e != nil {
		return nil, e
	}
	if e = verifyMongoContent(ctx, borrowed.Mongo, r.Targets[3]); e != nil {
		return nil, e
	}
	raw, e := json.Marshal(m)
	if e != nil || writePrivate(filepath.Join(dir, "archive.private.json"), raw) != nil {
		return nil, ErrPrivate
	}
	if e = syncDirectory(dir); e != nil {
		return nil, e
	}
	return &Archive{dir: dir, digest: sha(raw), data: m}, nil
}
func syncDirectory(dir string) error {
	f, e := os.Open(dir)
	if e != nil {
		return ErrPrivate
	}
	se, ce := f.Sync(), f.Close()
	if se != nil || ce != nil {
		return ErrPrivate
	}
	return nil
}

// OpenArchive is for a trusted compiled tool inside a restore container. The
// expected manifest digest must arrive through the independent host binding.
func OpenArchive(ctx context.Context, dir, expected string) (*Archive, error) {
	if ctx == nil || ctx.Err() != nil || !hashPattern.MatchString(expected) || privateDirectory(dir) != nil {
		return nil, ErrPrivate
	}
	f, e := openPrivateFile(filepath.Join(dir, "archive.private.json"))
	if e != nil {
		return nil, e
	}
	raw, e := readPrivate(f, metadataBudget)
	ce := f.Close()
	if e != nil || ce != nil || sha(raw) != expected {
		return nil, ErrPrivate
	}
	var m manifest
	if exactJSON(raw, &m) != nil || m.Version != 1 || !approvalValid(m.Approval) || m.OrderedMongoSchemaHash != m.Approval.OrderedMongoSchemaSHA256 || jsonSHA(m.Mongo) != m.OrderedMongoSchemaHash || sha(m.InventoryRaw) != m.Approval.InventorySHA256 || m.SQLMetadataHash != m.Approval.SQLMetadataSHA256 || m.MongoMetadataHash != m.Approval.MongoMetadataSHA256 {
		return nil, ErrPrivate
	}
	// Older version 1 framing did not carry the recovery projection. Only B
	// recovery requires it; a present malformed value is never accepted.
	if m.SQLRecoveryNonTargetHash != "" && !hashPattern.MatchString(m.SQLRecoveryNonTargetHash) {
		return nil, ErrPrivate
	}
	original, e := validateInventory(m.InventoryRaw, m.Approval)
	if e != nil || !reflect.DeepEqual(original, m.Inventory) {
		return nil, ErrApproval
	}
	for i, structure := range m.SQL {
		normalized := increment.ReplaceAllString(structure.DDL, "")
		row := [][]*string{{&targetNames[i], &normalized}}
		if jsonSHA(row) != m.Inventory.Targets[i].SchemaHash || !supportedColumns(i, structure.Columns) {
			return nil, ErrStructure
		}
	}
	if e = verifyMongoObject(m.Mongo, m.Inventory.Targets[3]); e != nil {
		return nil, e
	}
	a := &Archive{dir: dir, digest: expected, data: m}
	if e = a.verifyAssets(ctx); e != nil {
		return nil, e
	}
	return a, nil
}
func (a *Archive) verifyAssets(ctx context.Context) error {
	if a == nil || privateDirectory(a.dir) != nil {
		return ErrPrivate
	}
	for i, asset := range a.data.Assets {
		if asset.Filename != sourceNames[i] || !hashPattern.MatchString(asset.SHA256) || asset.Bytes < 0 {
			return ErrPrivate
		}
		f, e := openPrivateFile(filepath.Join(a.dir, asset.Filename))
		if e != nil {
			return e
		}
		h := newCountHash()
		columns := a.data.SQL[0].Columns
		if i < 3 {
			columns = a.data.SQL[i].Columns
		}
		r, e := newRawReader(ctx, io.TeeReader(f, h), i, a.data.Inventory.Targets[i], columns)
		if e == nil {
			for {
				_, e = r.next()
				if e != nil {
					break
				}
			}
		}
		ce := f.Close()
		if e != io.EOF || ce != nil || h.count != asset.Bytes || h.digest() != asset.SHA256 {
			return ErrSource
		}
	}
	return nil
}

// PurgeRegistered removes only this fixed backend's registered temporary files.
// The host decides when rollback retention is over; business tables are never
// addressed here. Any unrelated file, symlink or mismatched origin blocks it.
func PurgeRegistered(dir string, approval Approval) error {
	if privateDirectory(dir) != nil || !approvalValid(approval) {
		return ErrPrivate
	}
	f, e := openPrivateFile(filepath.Join(dir, "assets.private.json"))
	if e != nil {
		return e
	}
	raw, e := readPrivate(f, metadataBudget)
	ce := f.Close()
	var reg registration
	if e != nil || ce != nil || exactJSON(raw, &reg) != nil || reg.Version != 1 || !reflect.DeepEqual(reg.Approval, approval) || !reflect.DeepEqual(reg.Files, registrationNames()) || reg.ResumeAllowed || !reg.ContainsOriginalBodies || !reg.PurgeAfterAcceptance {
		return ErrPrivate
	}
	allowed := map[string]bool{"assets.private.json": true}
	for _, n := range reg.Files {
		allowed[n] = true
	}
	entries, e := os.ReadDir(dir)
	if e != nil {
		return ErrPrivate
	}
	for _, entry := range entries {
		if !allowed[entry.Name()] {
			return ErrPrivate
		}
		f, e := openPrivateFile(filepath.Join(dir, entry.Name()))
		if e != nil {
			return e
		}
		if f.Close() != nil {
			return ErrPrivate
		}
	}
	for _, entry := range entries {
		if entry.Name() == "assets.private.json" {
			continue
		}
		if os.Remove(filepath.Join(dir, entry.Name())) != nil || syncDirectory(dir) != nil {
			return ErrPrivate
		}
	}
	// Retain the origin-bound registry through every partial cleanup. Remove
	// it only after all raw assets are durably gone; there is no resume writer.
	if os.Remove(filepath.Join(dir, "assets.private.json")) != nil {
		return ErrPrivate
	}
	return syncDirectory(dir)
}
