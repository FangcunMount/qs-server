package compatibilityretirementbackup

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
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
	Version                int              `json:"version"`
	Approval               Approval         `json:"approval"`
	Files                  []string         `json:"files"`
	ContainsOriginalBodies bool             `json:"contains_original_bodies"`
	PurgeAfterAcceptance   bool             `json:"purge_after_acceptance"`
	ResumeAllowed          bool             `json:"resume_allowed"`
	StorageApproval        *StorageApproval `json:"storage_approval,omitempty"`
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
	if exactJSON(raw, &m) != nil || (m.Version != 1 && m.Version != 2) || !approvalValid(m.Approval) || m.OrderedMongoSchemaHash != m.Approval.OrderedMongoSchemaSHA256 || jsonSHA(m.Mongo) != m.OrderedMongoSchemaHash || sha(m.InventoryRaw) != m.Approval.InventorySHA256 || m.SQLMetadataHash != m.Approval.SQLMetadataSHA256 || m.MongoMetadataHash != m.Approval.MongoMetadataSHA256 {
		return nil, ErrPrivate
	}
	// Older version 1 framing did not carry the recovery projection. Only B
	// recovery requires it; a present malformed value is never accepted.
	if m.SQLRecoveryNonTargetHash != "" && !hashPattern.MatchString(m.SQLRecoveryNonTargetHash) {
		return nil, ErrPrivate
	}
	if m.Version == 1 {
		original, e := validateInventory(m.InventoryRaw, m.Approval)
		if e != nil || m.StorageApproval != nil || !reflect.DeepEqual(original, m.Inventory) {
			return nil, ErrApproval
		}
	} else if e = validateStorageManifest(m); e != nil {
		return nil, e
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
	if m.Version == 2 {
		if _, e = storageFiles(ctx, a); e != nil {
			return nil, e
		}
	}
	if e = a.verifyAssets(ctx); e != nil {
		return nil, e
	}
	return a, nil
}
func (a *Archive) verifyAssets(ctx context.Context) error {
	if a == nil || privateDirectory(a.dir) != nil {
		return ErrPrivate
	}
	if a.data.Version == 2 {
		if _, e := storageFiles(ctx, a); e != nil {
			return e
		}
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
		r, e := a.sourceReader(ctx, io.TeeReader(f, h), i, columns)
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
	if e != nil || ce != nil || exactJSON(raw, &reg) != nil || reg.Version != 1 || reg.StorageApproval != nil || !reflect.DeepEqual(reg.Approval, approval) || !reflect.DeepEqual(reg.Files, registrationNames()) || reg.ResumeAllowed || !reg.ContainsOriginalBodies || !reg.PurgeAfterAcceptance {
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

func storageNames() []string {
	return append(append([]string(nil), registrationNames()...), "mysql-metadata.private.json", "mongodb-metadata.private.json")
}
func storageApprovalValid(a StorageApproval) bool {
	if !sourcePattern.MatchString(a.OriginalSourceSHA) || !sourcePattern.MatchString(a.ToolSourceSHA) || !runPattern.MatchString(a.OperationID) || !runPattern.MatchString(a.RunID) || len(a.Bindings) != 2 {
		return false
	}
	for _, value := range []string{a.RequestHash, a.TargetHash, a.SQLMetadataSHA256, a.MongoMetadataSHA256, a.OrderedMongoSchemaSHA256} {
		if !hashPattern.MatchString(value) {
			return false
		}
	}
	fixed := [4][3]string{{"mysql", targetNames[0], "base_table"}, {"mysql", targetNames[1], "base_table"}, {"mysql", targetNames[2], "base_table"}, {"mongodb", targetNames[3], "collection"}}
	if a.TargetHash != jsonSHA(fixed) {
		return false
	}
	for i, target := range a.Targets {
		if target.Database != fixed[i][0] || target.Name != fixed[i][1] || target.Kind != fixed[i][2] || !hashPattern.MatchString(target.IdentityHash) || !hashPattern.MatchString(target.SchemaHash) {
			return false
		}
		b := target.Boundary
		if b.Database != target.Database || b.Name != target.Name || b.Kind != target.Kind || b.IdentityHash != target.IdentityHash || b.SchemaHash != target.SchemaHash || !b.Present || (b.Empty && b.UpperToken != "") || (!b.Empty && b.UpperToken == "") || (i == 0 && b.PKType != "uint64") || (i > 0 && i < 3 && b.PKType != "ascii_string") {
			return false
		}
	}
	for database, binding := range a.Bindings {
		if database != "mysql" && database != "mongodb" {
			return false
		}
		if !hashPattern.MatchString(binding.IdentityHash) || !hashPattern.MatchString(binding.AnchorHash) || !hashPattern.MatchString(binding.CatalogHash) || !hashPattern.MatchString(binding.NonTargetHash) || binding.Version == 0 || binding.Dirty || !binding.HeadMatch || !binding.IdentityMatch || !binding.MetadataComplete || binding.ErrorCategory != "none" {
			return false
		}
		if database == "mysql" && (binding.AnchorHash != binding.IdentityHash || binding.GenerationHash != "") {
			return false
		}
		if database == "mongodb" && (!hashPattern.MatchString(binding.GenerationHash) || (binding.NamespaceAnchor != nil && (binding.NamespaceAnchor.Validate() != nil || binding.NamespaceAnchor.Hash != binding.AnchorHash))) {
			return false
		}
	}
	return true
}

// storageMetadata observes only structure/identity, both before and after the
// one content capture. No caller-provided count/hash can reach this function.
func storageMetadata(ctx context.Context, borrowed BorrowedSources, approved StorageApproval, sqlBytes, mongoBytes []byte) (manifest, error) {
	var m manifest
	var sm sqlMetadata
	var mm mongoMetadata
	if exactJSON(sqlBytes, &sm) != nil || exactJSON(mongoBytes, &mm) != nil || jsonSHA(sm.Schema) != approved.Bindings["mysql"].CatalogHash || jsonSHA(mm.Schema) != approved.Bindings["mongodb"].CatalogHash || !strings.HasPrefix(mm.Version, "7.") {
		return m, ErrStructure
	}
	if _, e := sqlState(ctx, borrowed.SQL, approved.Bindings["mysql"]); e != nil {
		return m, e
	}
	grants, e := readSQL(ctx, borrowed.SQL, "SHOW GRANTS FOR CURRENT_USER")
	if e != nil || jsonSHA(grants) != sm.GrantsHash {
		return m, ErrIdentity
	}
	defs, e := readSQLCatalog(ctx, borrowed.SQL)
	if e != nil || jsonSHA(defs) != jsonSHA(sm.Schema) {
		return m, ErrStructure
	}
	namespace, e := readSQL(ctx, borrowed.SQL, "SELECT DATABASE()")
	if e != nil || len(namespace) != 1 || len(namespace[0]) != 1 || cell(namespace[0], 0) == "" {
		return m, ErrIdentity
	}
	m.SQLRecoveryNonTargetHash, e = targetSQLNonTarget(defs, cell(namespace[0], 0))
	if e != nil {
		return m, e
	}
	if triggers, ok := defs["triggers"].([][]*string); ok {
		for _, row := range triggers {
			for _, name := range targetNames[:3] {
				if cell(row, 1) == name {
					return m, ErrStructure
				}
			}
		}
	}
	for i, target := range approved.Targets[:3] {
		m.SQL[i], e = sqlStructure(ctx, borrowed.SQL, i)
		if e != nil {
			return m, e
		}
		normalized := increment.ReplaceAllString(m.SQL[i].DDL, "")
		if jsonSHA([][]*string{{&targetNames[i], &normalized}}) != target.SchemaHash || parts("mysql-object-v1", target.Name, target.SchemaHash) != target.IdentityHash {
			return m, ErrStructure
		}
	}
	collections, defsMongo, e := mongoCatalog(ctx, borrowed.Mongo)
	if e != nil || jsonSHA(defsMongo) != jsonSHA(mm.Schema) {
		return m, ErrStructure
	}
	if _, e = mongoState(ctx, borrowed.Mongo, approved.Bindings["mongodb"], collections); e != nil {
		return m, e
	}
	ordered, e := ReadOrderedMongoSchema(ctx, borrowed.Mongo)
	if e != nil || ordered.digest != approved.OrderedMongoSchemaSHA256 {
		return m, ErrStructure
	}
	if e = verifyMongoObject(ordered.data, SourceSnapshot{SchemaHash: approved.Targets[3].SchemaHash, IdentityHash: approved.Targets[3].IdentityHash}); e != nil {
		return m, e
	}
	var hello bson.Raw
	if borrowed.Mongo.Client().Database("admin").RunCommand(mongoMetadataContext(ctx), bson.D{{Key: "hello", Value: 1}}).Decode(&hello) != nil {
		return m, ErrIdentity
	}
	process, ok := hello.Lookup("topologyVersion", "processId").ObjectIDOK()
	if !ok || process.IsZero() {
		return m, ErrIdentity
	}
	m.Mongo, m.SourceMongoNamespace, m.SourceMongoProcessID = ordered.data, borrowed.Mongo.Name(), process.Hex()
	m.SQLMetadataHash, m.MongoMetadataHash, m.OrderedMongoSchemaHash = sha(sqlBytes), sha(mongoBytes), ordered.digest
	return m, nil
}

// CaptureStorage reads each original target once into its original binary/BSON
// framing. It approves no history, writer fence, restore or DROP. The host owns
// both database handles and the SQL read transaction throughout the capture.
func CaptureStorage(ctx context.Context, borrowed BorrowedSources, approved StorageApproval, in StorageInputs, dir string) (*Archive, error) {
	if ctx == nil || ctx.Err() != nil || borrowed.SQL == nil || borrowed.Mongo == nil || !storageApprovalValid(approved) || privateDirectory(dir) != nil {
		return nil, ErrApproval
	}
	entries, e := os.ReadDir(dir)
	if e != nil || len(entries) != 0 {
		return nil, ErrPrivate
	}
	// Detach all metadata expectations from mutable caller maps/pointers.
	approvalRaw, e := json.Marshal(approved)
	var detached StorageApproval
	if e != nil || exactJSON(approvalRaw, &detached) != nil {
		return nil, ErrApproval
	}
	approved = detached
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Second)
	defer cancel()
	sqlBytes, e := readPrivate(in.SQLMetadata, metadataBudget)
	if e != nil || sha(sqlBytes) != approved.SQLMetadataSHA256 {
		return nil, ErrApproval
	}
	mongoBytes, e := readPrivate(in.MongoMetadata, metadataBudget)
	if e != nil || sha(mongoBytes) != approved.MongoMetadataSHA256 {
		return nil, ErrApproval
	}
	m, e := storageMetadata(ctx, borrowed, approved, sqlBytes, mongoBytes)
	if e != nil {
		return nil, e
	}
	m.Version, m.StorageApproval = 2, &approved
	m.Approval = approved.archiveApproval()
	reg := registration{Version: 2, Approval: m.Approval, StorageApproval: &approved, Files: storageNames(), ContainsOriginalBodies: true, PurgeAfterAcceptance: true}
	raw, e := json.Marshal(reg)
	if e != nil || writePrivate(filepath.Join(dir, "assets.private.json"), raw) != nil {
		return nil, ErrPrivate
	}
	for i, data := range [][]byte{sqlBytes, mongoBytes} {
		if writePrivate(filepath.Join(dir, storageNames()[5+i]), data) != nil {
			return nil, ErrPrivate
		}
	}
	r := inventory{Format: 1, Kind: "storage_backup_v1", SourceSHA: approved.OriginalSourceSHA, OperationID: approved.OperationID, RunID: approved.RunID, RequestHash: approved.RequestHash, TargetHash: approved.TargetHash, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Bindings: approved.Bindings, Protocol: "mysql_cast_binary_columns_pk_order_v2+mongodb_server_bson_pk_order_v2", Semantics: "single_full_storage_capture_eof_metadata_before_after_no_writer_fence", ErrorCategory: "none", Diagnostic: true, BoundaryHash: jsonSHA(approved.Targets)}
	for i, target := range approved.Targets {
		s := SourceSnapshot{Database: target.Database, Name: target.Name, Kind: target.Kind, Present: true, SchemaHash: target.SchemaHash, IdentityHash: target.IdentityHash, SourceFile: sourceNames[i], ErrorCategory: "none", Passes: 1}
		s.Boundary = target.Boundary
		if i < 3 {
			s, m.Assets[i], e = captureStorageSQL(ctx, borrowed.SQL, dir, i, m.SQL[i], s)
		} else {
			s, m.Assets[i], e = captureStorageMongo(ctx, borrowed.Mongo, dir, s)
		}
		if e != nil {
			return nil, e
		}
		r.Targets = append(r.Targets, s)
	}
	end, e := storageMetadata(ctx, borrowed, approved, sqlBytes, mongoBytes)
	if e != nil {
		return nil, e
	}
	if !reflect.DeepEqual(m.SQL, end.SQL) || !reflect.DeepEqual(m.Mongo, end.Mongo) || m.SourceMongoProcessID != end.SourceMongoProcessID || m.SourceMongoNamespace != end.SourceMongoNamespace || m.SQLRecoveryNonTargetHash != end.SQLRecoveryNonTargetHash || ctx.Err() != nil {
		return nil, ErrStructure
	}
	r.Complete = true
	m.Inventory, m.InventoryRaw = r, nil
	m.InventoryRaw, e = json.Marshal(r)
	if e != nil {
		return nil, ErrPrivate
	}
	m.Approval.InventorySHA256 = sha(m.InventoryRaw)
	if e = validateStorageManifest(m); e != nil {
		return nil, e
	}
	raw, e = json.Marshal(m)
	if e != nil || writePrivate(filepath.Join(dir, "archive.private.json"), raw) != nil || ctx.Err() != nil {
		return nil, ErrPrivate
	}
	return &Archive{dir: dir, digest: sha(raw), data: m}, nil
}

func validateStorageManifest(m manifest) error {
	if m.Version != 2 || m.StorageApproval == nil || !storageApprovalValid(*m.StorageApproval) || !approvalValid(m.Approval) {
		return ErrApproval
	}
	a, r := *m.StorageApproval, m.Inventory
	want := a.archiveApproval()
	want.InventorySHA256 = sha(m.InventoryRaw)
	var raw inventory
	if m.Approval != want || exactJSON(m.InventoryRaw, &raw) != nil || !reflect.DeepEqual(raw, r) || r.Format != 1 || r.Kind != "storage_backup_v1" || r.SourceSHA != a.OriginalSourceSHA || r.OperationID != a.OperationID || r.RunID != a.RunID || r.RequestHash != a.RequestHash || r.TargetHash != a.TargetHash || r.BoundaryHash != jsonSHA(a.Targets) || !reflect.DeepEqual(r.Bindings, a.Bindings) || !r.Complete || r.DropReady || !r.Diagnostic || r.ErrorCategory != "none" || r.Protocol != "mysql_cast_binary_columns_pk_order_v2+mongodb_server_bson_pk_order_v2" || r.Semantics != "single_full_storage_capture_eof_metadata_before_after_no_writer_fence" || len(r.Targets) != 4 {
		return ErrApproval
	}
	for i, s := range r.Targets {
		t := a.Targets[i]
		b := s.Boundary
		if s.Database != t.Database || s.Name != t.Name || s.Kind != t.Kind || s.IdentityHash != t.IdentityHash || s.SchemaHash != t.SchemaHash || !reflect.DeepEqual(b, t.Boundary) || s.SourceFile != sourceNames[i] || s.Passes != 1 || s.Pages != 1 || !s.Present || !s.Complete || s.NextCycle || s.ErrorCategory != "none" || !hashPattern.MatchString(s.DataHash) || s.Records > 1000000 || s.Bytes > 2<<30 || b.Empty != (s.Records == 0) || (b.Empty && s.Bytes != 0) {
			return ErrSource
		}
	}
	return nil
}

func storageFiles(ctx context.Context, a *Archive) ([2]Asset, error) {
	var assets [2]Asset
	if ctx == nil || ctx.Err() != nil || a == nil || validateStorageManifest(a.data) != nil || privateDirectory(a.dir) != nil {
		return assets, ErrApproval
	}
	entries, e := os.ReadDir(a.dir)
	allowed := map[string]bool{"assets.private.json": true}
	for _, name := range storageNames() {
		allowed[name] = true
	}
	if e != nil || len(entries) != len(allowed) {
		return assets, ErrPrivate
	}
	for _, entry := range entries {
		if !allowed[entry.Name()] {
			return assets, ErrPrivate
		}
	}
	f, e := openPrivateFile(filepath.Join(a.dir, "archive.private.json"))
	if e != nil {
		return assets, e
	}
	raw, e := readPrivate(f, metadataBudget)
	ce := f.Close()
	if e != nil || ce != nil || sha(raw) != a.digest {
		return assets, ErrPrivate
	}
	f, e = openPrivateFile(filepath.Join(a.dir, "assets.private.json"))
	if e != nil {
		return assets, e
	}
	raw, e = readPrivate(f, metadataBudget)
	ce = f.Close()
	var reg registration
	approval := a.data.Approval
	approval.InventorySHA256 = ""
	if e != nil || ce != nil || exactJSON(raw, &reg) != nil || reg.Version != 2 || reg.Approval != approval || reg.StorageApproval == nil || !reflect.DeepEqual(*reg.StorageApproval, *a.data.StorageApproval) || !reflect.DeepEqual(reg.Files, storageNames()) || reg.ResumeAllowed || !reg.ContainsOriginalBodies || !reg.PurgeAfterAcceptance {
		return assets, ErrPrivate
	}
	for i, expected := range []string{approval.SQLMetadataSHA256, approval.MongoMetadataSHA256} {
		name := storageNames()[5+i]
		f, e = openPrivateFile(filepath.Join(a.dir, name))
		if e != nil {
			return assets, e
		}
		raw, e = readPrivate(f, metadataBudget)
		ce = f.Close()
		if e != nil || ce != nil || sha(raw) != expected {
			return assets, ErrPrivate
		}
		assets[i] = Asset{Filename: name, SHA256: expected, Bytes: int64(len(raw))}
	}
	return assets, nil
}

func (a *Archive) StorageBaseline(ctx context.Context) (StorageBaseline, error) {
	var out StorageBaseline
	metadata, e := storageFiles(ctx, a)
	if e != nil {
		return out, e
	}
	if e = a.verifyAssets(ctx); e != nil {
		return out, e
	}
	// Detached report projections never alias the opaque archive's private data.
	raw, e := json.Marshal(StorageBaseline{Kind: "storage_backup_v1", ArchiveSHA256: a.digest, Approval: a.data.Approval, StorageApproval: *a.data.StorageApproval, Targets: a.data.Inventory.Targets, MetadataAssets: metadata, SourceAssets: a.data.Assets})
	if e != nil || exactJSON(raw, &out) != nil {
		return StorageBaseline{}, ErrPrivate
	}
	return out, nil
}

// PurgeStorageRegistered addresses only this new producer's eight exact files.
// It also works for registered partial captures; the host supplies retention
// authority, and an extra member always blocks cleanup.
func PurgeStorageRegistered(dir string, approved StorageApproval) error {
	if !storageApprovalValid(approved) || privateDirectory(dir) != nil {
		return ErrPrivate
	}
	f, e := openPrivateFile(filepath.Join(dir, "assets.private.json"))
	if e != nil {
		return e
	}
	raw, e := readPrivate(f, metadataBudget)
	ce := f.Close()
	var reg registration
	if e != nil || ce != nil || exactJSON(raw, &reg) != nil || reg.Version != 2 || reg.Approval != approved.archiveApproval() || reg.StorageApproval == nil || !reflect.DeepEqual(*reg.StorageApproval, approved) || !reflect.DeepEqual(reg.Files, storageNames()) || reg.ResumeAllowed || !reg.ContainsOriginalBodies || !reg.PurgeAfterAcceptance {
		return ErrPrivate
	}
	allowed := map[string]bool{"assets.private.json": true}
	for _, name := range reg.Files {
		allowed[name] = true
	}
	entries, e := os.ReadDir(dir)
	if e != nil {
		return ErrPrivate
	}
	for _, entry := range entries {
		if !allowed[entry.Name()] {
			return ErrPrivate
		}
		f, e = openPrivateFile(filepath.Join(dir, entry.Name()))
		if e != nil {
			return e
		}
		if f.Close() != nil {
			return ErrPrivate
		}
	}
	for _, entry := range entries {
		if entry.Name() != "assets.private.json" && (os.Remove(filepath.Join(dir, entry.Name())) != nil || syncDirectory(dir) != nil) {
			return ErrPrivate
		}
	}
	if os.Remove(filepath.Join(dir, "assets.private.json")) != nil {
		return ErrPrivate
	}
	return syncDirectory(dir)
}
