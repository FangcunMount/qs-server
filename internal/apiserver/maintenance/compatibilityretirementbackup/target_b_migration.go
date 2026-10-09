package compatibilityretirementbackup

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"reflect"

	"github.com/FangcunMount/qs-server/internal/pkg/migration"
	buildversion "github.com/FangcunMount/qs-server/pkg/version"
	"go.mongodb.org/mongo-driver/mongo"
)

// Only native same-process transition or actual physical-journal reconciliation
// producers implement this package-private recovery observation interface.
type targetBMigrationVerifier interface {
	VerifyAfter(context.Context, *sql.Conn, *mongo.Database) error
}

type targetBMigrationTransition struct {
	proof                               targetBMigrationVerifier
	sqlBinding, mongoBinding            Binding
	stableMongoSchema, afterMongoSchema string
	intentSHA, resultSHA                string
	journalKind                         targetBJournalKind
}
type targetBMigrationIntent struct {
	Binding                 migration.CompatibilityPairRunBinding `json:"binding"`
	SQLIdentitySHA256       string                                `json:"mysql_identity_sha256"`
	MongoIdentitySHA256     string                                `json:"mongodb_identity_sha256"`
	MongoGenerationSHA256   string                                `json:"mongodb_generation_sha256"`
	MongoMigrationUUID      string                                `json:"mongodb_migration_uuid"`
	MongoStableSchemaSHA256 string                                `json:"mongodb_stable_schema_sha256"`
}
type targetBMigrationResult struct {
	IntentSHA256 string                                           `json:"intent_sha256"`
	State        string                                           `json:"state"`
	Attempt      migration.CompatibilityPairRunAttempt            `json:"attempt"`
	Observation  *migration.CompatibilityPairMigrationObservation `json:"observation"`
}

// Normalization allows exactly the driver-recreated schema_migrations UUID.
// All other namespace names/options/UUIDs and all indexes remain in the hash.
// It never mutates the original catalog or compares the archive to another DB.
func targetBStableMongoNonTarget(defs map[string]any) (string, error) {
	encoded, e := json.Marshal(defs)
	if e != nil || len(encoded) > metadataBudget {
		return "", ErrStructure
	}
	var copied map[string]any
	if json.Unmarshal(encoded, &copied) != nil {
		return "", ErrStructure
	}
	col, ok := copied["collection:schema_migrations"].(map[string]any)
	if !ok || col["name"] != "schema_migrations" || col["type"] != "collection" {
		return "", ErrStructure
	}
	info, ok := col["info"].(map[string]any)
	if !ok {
		return "", ErrStructure
	}
	uuid, ok := info["uuid"].(map[string]any)
	if !ok || len(uuid) != 1 {
		return "", ErrStructure
	}
	value, ok := uuid["$binary"].(map[string]any)
	if !ok || len(value) != 2 || value["subType"] != "04" {
		return "", ErrStructure
	}
	encodedUUID, ok := value["base64"].(string)
	if !ok {
		return "", ErrStructure
	}
	raw, e := base64.StdEncoding.Strict().DecodeString(encodedUUID)
	if e != nil || len(raw) != 16 {
		return "", ErrStructure
	}
	info["uuid"] = "exact-native-B-schema-migrations-generation/v1"
	return targetMongoNonTarget(copied), nil
}

// RunTargetBMigration is a fixed-purpose native kernel after this plan's four
// real DROP responses/readbacks. The existing host must already hold actual
// writer quiescence and approve the B build. These input strings/window are not
// an authorization capability. No serialized observation can rebind recovery.
// It borrows the existing dedicated SQL Conn and Mongo client; no pool opens,
// transaction starts, connection closes or recovery-window resets occur here.
func RunTargetBMigration(ctx context.Context, p *TargetRecoveryPlan, approvedBSourceSHA string) (*migration.CompatibilityPairMigrationProof, error) {
	if ctx == nil || ctx.Err() != nil || p == nil || p.self != p || !sourcePattern.MatchString(approvedBSourceSHA) || approvedBSourceSHA != buildversion.Get().GitCommit {
		return nil, ErrRecoveryBinding
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.archive == nil || p.journal == nil || p.window == nil || p.blocked || p.bMigration != nil || p.request.SQLHead != 99 || p.request.MongoHead != 38 || p.archive.data.Inventory.Bindings["mysql"].Version != 99 || p.archive.data.Inventory.Bindings["mongodb"].Version != 38 {
		return nil, ErrRecoveryHead
	}
	start, e := targetWindowMatches(ctx, p.window, p.request, "")
	if e != nil {
		return nil, e
	}
	q, c, e := p.window.ForwardContext(ctx)
	if e != nil || !targetBoundContextValid(q, c) {
		if c != nil {
			c()
		}
		return nil, ErrBudget
	}
	defer c()
	if p.archive.verifyAssets(q) != nil {
		return nil, ErrSource
	}
	if e = p.checkBases(q, true); e != nil {
		return nil, e
	}
	for i := 0; i < 4; i++ {
		if !p.validDrop(i) {
			return nil, ErrRecoveryAuthority
		}
		present, e := p.checkTarget(q, i)
		if e != nil || present {
			return nil, ErrRecoveryState
		}
	}
	beforeCols, beforeDefs, e := mongoCatalog(q, p.borrowed.Mongo)
	if e != nil {
		return nil, e
	}
	beforeUUID, e := targetActualMigrationUUID(beforeCols)
	if e != nil {
		return nil, e
	}
	stable, e := targetBStableMongoNonTarget(beforeDefs)
	if e != nil {
		return nil, e
	}
	sb := p.archive.data.Inventory.Bindings["mysql"]
	mb := p.archive.data.Inventory.Bindings["mongodb"]
	if mb.GenerationHash != parts("mongodb_migration_generation_v1", beforeUUID) {
		return nil, ErrIdentity
	}
	pair, e := migration.PreflightCompatibilityPairConnection(q, p.borrowed.SQL, p.borrowed.Mongo.Client(), migration.PairConfig{MySQLDatabase: p.sqlNamespace, MongoDatabase: p.borrowed.Mongo.Name(), ExpectedSourceSHA: approvedBSourceSHA})
	if e != nil {
		return nil, ErrRecoveryHead
	}
	binding := migration.CompatibilityPairRunBinding{ApprovedSourceSHA: approvedBSourceSHA, OriginalSourceSHA: p.request.SourceSHA, OperationID: p.request.OperationID, OriginalRunID: p.request.OriginalRunID, ActualRunID: p.request.ActualRunID, ManifestSHA256: p.request.ManifestSHA256, ArchiveSHA256: p.request.ArchiveSHA256, RequestSHA256: jsonSHA(p.request), WindowStartSHA256: start, ResourcesSHA256: migration.CompatibilityMigrationResourcesSHA256()}
	intent := targetBMigrationIntent{Binding: binding, SQLIdentitySHA256: sb.IdentityHash, MongoIdentitySHA256: mb.IdentityHash, MongoGenerationSHA256: mb.GenerationHash, MongoMigrationUUID: beforeUUID, MongoStableSchemaSHA256: stable}
	intentSHA, e := p.journal.write("b-migration-intent", intent)
	if e != nil {
		p.blocked = true
		return nil, ErrRecoveryUnknown
	}
	proof, attempt, runErr := pair.Run(q, binding, mb.NamespaceAnchor)
	result := targetBMigrationResult{IntentSHA256: intentSHA, State: "partial_or_unknown", Attempt: attempt}
	if runErr != nil || proof == nil || q.Err() != nil {
		_, _ = p.journal.write("b-migration-result", result)
		p.blocked = true
		return nil, ErrRecoveryUnknown
	}
	o := proof.Observation()
	if o.Binding != binding || o.SQLBefore != 99 || o.SQLAfter != 100 || o.MongoBefore != 38 || o.MongoAfter != 39 || !o.SQLChanged || !o.MongoChanged || o.SQLIdentitySHA256 != sb.IdentityHash || o.MongoBeforeIdentitySHA256 != mb.IdentityHash || o.MongoBeforeGenerationSHA256 != mb.GenerationHash || o.MongoBeforeMigrationUUID != beforeUUID || o.MongoBeforeGenerationSHA256 == o.MongoAfterGenerationSHA256 || proof.VerifyAfter(q, p.borrowed.SQL, p.borrowed.Mongo) != nil {
		_, _ = p.journal.write("b-migration-result", result)
		p.blocked = true
		return nil, ErrRecoveryUnknown
	}
	sqlDefs, e := readSQLCatalog(q, p.borrowed.SQL)
	if e != nil {
		_, _ = p.journal.write("b-migration-result", result)
		p.blocked = true
		return nil, ErrRecoveryUnknown
	}
	sqlNon, e := targetSQLNonTarget(sqlDefs, p.sqlNamespace)
	if e != nil || sqlNon != p.request.SQLNonTargetSHA256 {
		_, _ = p.journal.write("b-migration-result", result)
		p.blocked = true
		return nil, ErrRecoveryUnknown
	}
	cols, afterDefs, e := mongoCatalog(q, p.borrowed.Mongo)
	if e != nil {
		_, _ = p.journal.write("b-migration-result", result)
		p.blocked = true
		return nil, ErrRecoveryUnknown
	}
	afterStable, e := targetBStableMongoNonTarget(afterDefs)
	if e != nil || afterStable != stable {
		_, _ = p.journal.write("b-migration-result", result)
		p.blocked = true
		return nil, ErrRecoveryUnknown
	}
	sb.Version = 100
	sb.CatalogHash = jsonSHA(sqlDefs)
	sb.NonTargetHash = sqlNon
	mb.Version = 39
	mb.IdentityHash = o.MongoAfterIdentitySHA256
	mb.GenerationHash = o.MongoAfterGenerationSHA256
	mb.CatalogHash = jsonSHA(afterDefs)
	mb.NonTargetHash = targetMongoNonTarget(afterDefs)
	mb.NamespaceAnchor = mb.NamespaceAnchor.Clone()
	// Legacy profile's exact replica anchor and new profile's approved kept UUIDs
	// are both re-observed; permissions/errors never select the other profile.
	if _, e = sqlState(q, p.borrowed.SQL, sb); e != nil {
		_, _ = p.journal.write("b-migration-result", result)
		p.blocked = true
		return nil, ErrRecoveryUnknown
	}
	if _, e = mongoState(q, p.borrowed.Mongo, mb, cols); e != nil {
		_, _ = p.journal.write("b-migration-result", result)
		p.blocked = true
		return nil, ErrRecoveryUnknown
	}
	if _, e = targetWindowMatches(q, p.window, p.request, start); e != nil {
		_, _ = p.journal.write("b-migration-result", result)
		p.blocked = true
		return nil, ErrRecoveryUnknown
	}
	result.State = "completed"
	result.Observation = &o
	resultSHA, e := p.journal.write("b-migration-result", result)
	if e != nil {
		p.blocked = true
		return nil, ErrRecoveryUnknown
	}
	p.bMigration = &targetBMigrationTransition{proof: proof, sqlBinding: sb, mongoBinding: mb, stableMongoSchema: stable, afterMongoSchema: mb.NonTargetHash, intentSHA: intentSHA, resultSHA: resultSHA}
	p.mongoBaseline = mb.NonTargetHash
	if p.checkBases(q, true) != nil {
		p.blocked = true
		return nil, ErrRecoveryUnknown
	}
	return proof, nil
}

// Physical journal validation recognizes this exact batch's native response
// material only. It is deliberately not a constructor for process proof.
// Unknown/partial or a crash after intent remains blocked, never re-executed.
func targetValidateBMigrationJournal(s *targetJournalSnapshot) error {
	return targetValidateBMigrationJournalKind(s, targetBJournalComplete)
}
func targetValidateBMigrationJournalKind(s *targetJournalSnapshot, kind targetBJournalKind) error {
	if !targetBJournalKindValid(kind) {
		return ErrRecoveryBinding
	}
	if s == nil {
		return ErrRecoveryJournal
	}
	raw, hasIntent := s.files["target-recovery-b-migration-intent.json"]
	rr, hasResult := s.files["target-recovery-b-migration-result.json"]
	if !hasIntent && !hasResult {
		if kind != targetBJournalComplete {
			return ErrRecoveryHead
		}
		return nil
	}
	if !hasIntent {
		return ErrRecoveryJournal
	}
	var intent targetBMigrationIntent
	if exactJSON(raw, &intent) != nil || !targetBCanonical(raw, intent) {
		return ErrRecoveryJournal
	}
	b := intent.Binding
	r := s.request
	if !sourcePattern.MatchString(b.ApprovedSourceSHA) || b.OriginalSourceSHA != r.SourceSHA || b.OperationID != r.OperationID || b.OriginalRunID != r.OriginalRunID || b.ActualRunID != r.ActualRunID || b.RequestSHA256 != jsonSHA(r) || b.ArchiveSHA256 != r.ArchiveSHA256 || b.ManifestSHA256 != r.ManifestSHA256 || b.WindowStartSHA256 != s.windowStart || b.ResourcesSHA256 != migration.CompatibilityMigrationResourcesSHA256() || r.SQLHead != 99 || r.MongoHead != 38 || !hashPattern.MatchString(intent.SQLIdentitySHA256) || !hashPattern.MatchString(intent.MongoIdentitySHA256) || !hashPattern.MatchString(intent.MongoGenerationSHA256) || !hashPattern.MatchString(intent.MongoStableSchemaSHA256) || !targetBMigrationUUID(intent.MongoMigrationUUID) || intent.MongoGenerationSHA256 != parts("mongodb_migration_generation_v1", intent.MongoMigrationUUID) {
		return ErrRecoveryJournal
	}
	if !hasResult {
		return ErrRecoveryUnknown
	}
	var result targetBMigrationResult
	if exactJSON(rr, &result) != nil || !targetBCanonical(rr, result) || result.IntentSHA256 != sha(raw) {
		return ErrRecoveryJournal
	}
	if kind == targetBJournalSQLOnly {
		return targetValidateSQLOnlyAttempt(result)
	}
	if result.State == "partial_or_unknown" && result.Observation == nil {
		return ErrRecoveryUnknown
	}
	if result.State != "completed" || result.Observation == nil {
		return ErrRecoveryJournal
	}
	o := result.Observation
	a := result.Attempt
	if a.Completion != "completed" || !a.SQLAttempted || !a.MongoAttempted || a.SQLResponse != "success" || a.MongoResponse != "success" || a.SQLReturnedVersion != 100 || a.MongoReturnedVersion != 39 || !a.SQLChanged || !a.MongoChanged || o.Binding != b || o.SQLBefore != 99 || o.SQLAfter != 100 || o.MongoBefore != 38 || o.MongoAfter != 39 || !o.SQLChanged || !o.MongoChanged || o.SQLIdentitySHA256 != intent.SQLIdentitySHA256 || o.MongoBeforeIdentitySHA256 != intent.MongoIdentitySHA256 || o.MongoBeforeGenerationSHA256 != intent.MongoGenerationSHA256 || o.MongoBeforeMigrationUUID != intent.MongoMigrationUUID || !hashPattern.MatchString(o.MongoAfterIdentitySHA256) || !hashPattern.MatchString(o.MongoAfterGenerationSHA256) || !targetBMigrationUUID(o.MongoBeforeMigrationUUID) || !targetBMigrationUUID(o.MongoAfterMigrationUUID) || o.MongoBeforeMigrationUUID == o.MongoAfterMigrationUUID || o.MongoBeforeGenerationSHA256 != parts("mongodb_migration_generation_v1", o.MongoBeforeMigrationUUID) || o.MongoAfterGenerationSHA256 != parts("mongodb_migration_generation_v1", o.MongoAfterMigrationUUID) {
		return ErrRecoveryJournal
	}
	return nil
}
func targetBCanonical(raw []byte, v any) bool {
	encoded, e := json.Marshal(v)
	return e == nil && reflect.DeepEqual(raw, encoded)
}
func targetBMigrationUUID(s string) bool { return len(s) == 32 && hashPattern.MatchString(s+s) }

func targetBMigrationJournalPresent(s *targetJournalSnapshot) bool {
	if s == nil {
		return false
	}
	_, intent := s.files["target-recovery-b-migration-intent.json"]
	_, result := s.files["target-recovery-b-migration-result.json"]
	return intent || result
}
