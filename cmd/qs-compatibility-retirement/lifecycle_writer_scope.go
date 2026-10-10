package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	dbcensus "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementdbcensus"
	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// This is expected input provenance, never a persisted isolation result. The
// native producer re-reads the complete current platform scope using fixed GETs.
type lifecycleWriterControl struct {
	WorkflowScopeSHA256 string                     `json:"workflow_scope_sha256"`
	DatabaseInput       *lifecycleFinalFileBinding `json:"database_input,omitempty"`
}

func (v *lifecycleWriterControl) valid() bool {
	return v != nil && hashRE.MatchString(v.WorkflowScopeSHA256) && (v.DatabaseInput == nil || filepath.IsAbs(v.DatabaseInput.Path) && filepath.Clean(v.DatabaseInput.Path) == v.DatabaseInput.Path && hashRE.MatchString(v.DatabaseInput.SHA256))
}

type lifecycleWriterObservation struct {
	token    []byte
	platform *fence.PlatformObservation
}

func (v *lifecycleWriterObservation) close() {
	if v == nil {
		return
	}
	for i := range v.token {
		v.token[i] = 0
	}
	v.token = nil
	v.platform = nil
}

func lifecyclePlatformToken() ([]byte, error) {
	// The fixed root caller received this one short-lived token in its private
	// credential stdin packet. Remove the inherited environment copy immediately;
	// the host owns the remaining bytes and zeroes them on Close. No file holds it.
	raw := os.Getenv("GITHUB_READ_TOKEN")
	if e := os.Unsetenv("GITHUB_READ_TOKEN"); e != nil {
		return nil, lifecycleError("lifecycle_platform_observation_unproven")
	}
	if len(raw) < 1 || len(raw) > 8192 {
		return nil, lifecycleError("lifecycle_platform_observation_unproven")
	}
	for _, c := range []byte(raw) {
		if c < 33 || c > 126 {
			return nil, lifecycleError("lifecycle_platform_observation_unproven")
		}
	}
	return []byte(raw), nil
}

func (h *lifecycleFixedHost) observeWholeWriterScopes(ctx context.Context, r lifecycleRequest) error {
	return h.observeWholeWriterScopesForOriginalD(ctx, r, nil)
}

// Only the same-process owned purge caller enters this phase. No request field
// chooses it, and it relaxes only the original D management liveness condition.
func (h *lifecycleFixedHost) observeWholeWriterScopesAfterDTerminal(ctx context.Context, r lifecycleRequest, terminal *lifecycleDTerminal) error {
	if terminal == nil {
		return lifecycleError("lifecycle_material_zero_unproven")
	}
	return h.observeWholeWriterScopesForOriginalD(ctx, r, terminal)
}

func (h *lifecycleFixedHost) checkOriginalDManagementPhase(ctx context.Context, r lifecycleRequest, terminal *lifecycleDTerminal) error {
	if h == nil || h.services == nil || h.services.child == nil {
		return lifecycleError("lifecycle_writer_scope_binding_rejected")
	}
	if terminal == nil {
		return h.services.child.requireLive()
	}
	return terminal.validate(ctx, h, r)
}

func (h *lifecycleFixedHost) observeWholeWriterScopesForOriginalD(ctx context.Context, r lifecycleRequest, terminal *lifecycleDTerminal) error {
	// A failed fresh read cannot reuse an older actor observation.
	if h != nil {
		h.writerRuntime = nil
	}
	if e := h.observePlatformQuarantineForOriginalD(ctx, r, terminal); e != nil {
		return e
	}
	if e := h.checkDatabaseWriterLease(ctx, r); e != nil {
		return e
	}
	// The approved four-target scope is closed only by real original actor
	// reads, not host account enumeration, broker history or a receipt flag.
	if e := h.observeKnownTargetServiceWriters(ctx, r, terminal); e != nil {
		return e
	}
	if e := h.checkDatabaseWriterLease(ctx, r); e != nil {
		return e // Actor readback cannot hide a new shared or foreign DB session.
	}
	return ctx.Err()
}

// All caller branches retain original native owners. API/Collection/Worker
// controlled runtime is accepted only after actual four-target absence, original
// frozen comparison and credential restoration. Unknown original writers fail.
func (h *lifecycleFixedHost) observeKnownTargetServiceWriters(ctx context.Context, r lifecycleRequest, terminal *lifecycleDTerminal) error {
	v := h.services
	if v == nil || v.local == nil || v.remote == nil || !v.stopAttempted || !v.remoteStopAttempted || !v.identity.matches(r) || h.aiStopped == nil || h.api == nil || h.api.self != h.api || h.api.unknown || h.api.rollbackCID != "" || h.dbWriters == nil {
		return lifecycleError("lifecycle_host_database_external_writer_isolation_unproven")
	}
	if h.api.bCID == "" {
		if h.dbWriters.restored || v.controlledAttempted || terminal != nil {
			return lifecycleError("lifecycle_host_database_external_writer_isolation_unproven")
		}
		if e := v.Check(ctx); e != nil {
			return e
		}
	} else {
		if !h.dbWriters.restored || h.verifyPreBDataComparison(ctx, r) != nil || h.acceptancePair == nil || h.api.observeAcceptance(ctx, r) != nil {
			return lifecycleError("lifecycle_host_database_external_writer_isolation_unproven")
		}
		if terminal != nil {
			// Original D's last live read was consumed by its native purge. It is
			// bound to this terminal sequence, not claimed as a new runtime GET.
			if h.finalRuntime.validate(h) != nil || terminal.native.ValidateOriginalRuntime(ctx, h.finalRuntime.remote) != nil {
				return lifecycleError("lifecycle_host_database_external_writer_isolation_unproven")
			}
			if _, e := v.local.ObserveRunningDependentsWithInlineAPI(ctx, h.api.bCID); e != nil {
				return e
			}
		} else if v.controlledAttempted {
			observed, e := v.observeControlled(ctx, h.api)
			if e != nil || observed.validate(h) != nil {
				return lifecycleError("lifecycle_host_database_external_writer_isolation_unproven")
			}
			h.writerRuntime = observed // Original latest read, not a serialized isolation flag.
		} else if e := v.CheckStoppedDependents(ctx, h.api.bCID); e != nil {
			return e
		}
	}
	return h.aiStopped.CheckWriterScope(ctx)
}

// This pre-stop observation deliberately has a narrower contract than the
// whole fence. The same fresh original-window platform GET is repeated by the
// whole-fence caller after StopAndDrain installs the actual database lease.
func (h *lifecycleFixedHost) CheckWriterPreconditions(ctx context.Context, r lifecycleRequest) error {
	if h == nil || h.services == nil || h.services.stopAttempted || h.services.remoteStopAttempted || h.services.local != nil || h.aiStopped != nil || h.dbWriters != nil {
		return lifecycleError("lifecycle_writer_precondition_phase_rejected")
	}
	return h.observePlatformQuarantineForOriginalD(ctx, r, nil)
}

func (h *lifecycleFixedHost) observePlatformQuarantineForOriginalD(ctx context.Context, r lifecycleRequest, terminal *lifecycleDTerminal) error {
	if h == nil || ctx == nil || ctx.Err() != nil || !r.WriterControl.valid() || r.DeploymentControl == nil || h.services == nil || h.services.window == nil || !h.services.managementReady || !h.services.identity.matches(r) || h.services.child == nil {
		return lifecycleError("lifecycle_writer_scope_binding_rejected")
	}
	// Ordinary phases require the same pre-established D process to be live.
	// Only completed native purge may verify that original process's terminal
	// result instead; neither phase creates a login or isolates other writers.
	if e := h.checkOriginalDManagementPhase(ctx, r, terminal); e != nil {
		return e
	}
	if e := validateLifecycleAPIInvocation(r); e != nil {
		return e
	}
	intentPath := filepath.Join(r.prepareRoot, "native-call.intent.private.json")
	before, e := readLifecycleAPIRecord(intentPath)
	if e != nil {
		return e
	}
	intent, e := decodeLifecycleAPIInvocationIntent(before)
	if e != nil {
		return e
	}
	if h.writers == nil {
		token, e := lifecyclePlatformToken()
		if e != nil {
			return e
		}
		h.writers = &lifecycleWriterObservation{token: token}
	}
	scopePath := filepath.Join(lifecycleServicesRoot(r.OperationID, "server-a"), "approved-workflow-scope.json")
	observed, e := fence.ObservePlatformQuarantine(ctx, h.services.window, scopePath, r.WriterControl.WorkflowScopeSHA256, r.ActualRunID, intent.DispatcherSourceSHA, h.writers.token)
	if e != nil {
		return lifecycleError("lifecycle_platform_observation_unproven")
	}
	after, e := readLifecycleAPIRecord(intentPath)
	if e != nil || !bytes.Equal(before, after) || validateLifecycleAPIInvocation(r) != nil || h.checkOriginalDManagementPhase(ctx, r, terminal) != nil || observed.ValidateOriginalWindow(ctx, h.services.window, lifecycleWindowBinding(r)) != nil {
		return lifecycleError("lifecycle_writer_scope_binding_rejected")
	}
	h.writers.platform = observed
	return nil // Only actual platform quarantine and original management binding.
}

// Expected ownership/baselines only. Actual credentials, principal enumeration,
// original stopped configurations and account changes stay in the native host.
type lifecycleDBPrincipalExpected struct {
	User           string   `json:"user"`
	HostOrDatabase string   `json:"host_or_database"`
	Owners         []string `json:"owners"`
}
type lifecycleDBWriterInput struct {
	FormatVersion     int                            `json:"format_version"`
	Kind              string                         `json:"kind"`
	ToolSourceSHA     string                         `json:"tool_source_sha"`
	OriginalSourceSHA string                         `json:"original_source_sha"`
	OperationID       string                         `json:"operation_id"`
	OriginalRunID     string                         `json:"original_run_id"`
	TargetHash        string                         `json:"target_hash"`
	ManifestSHA256    string                         `json:"manifest_sha256"`
	Census            lifecycleFinalFileBinding      `json:"census"`
	CensusSourceSHA   string                         `json:"census_source_sha"`
	CensusRunID       string                         `json:"census_run_id"`
	SQLObserver       lifecycleDBPrincipalExpected   `json:"mysql_observer"`
	MongoObserver     lifecycleDBPrincipalExpected   `json:"mongodb_observer"`
	SQLPrincipals     []lifecycleDBPrincipalExpected `json:"mysql_principals"`
	MongoPrincipals   []lifecycleDBPrincipalExpected `json:"mongodb_principals"`
	OriginalActors    []stop.DatabasePrincipal       `json:"original_actors"`
}
type lifecycleDBWriterLease struct {
	self                       *lifecycleDBWriterLease
	host                       *lifecycleFixedHost
	window                     *fence.MaintenanceWindow
	binding                    fence.WindowBinding
	actualRunID                string
	input                      lifecycleDBWriterInput
	inputRecord                *lifecycleFinalFileBinding
	baseline                   dbcensus.Catalog
	staticSHA                  string
	connectionID               uint64
	mongoConnectionID          int64
	sqlIdentity, mongoIdentity string
	sqlDatabase                string
	sqlAttempted               map[string]bool
	mongoAttempted             map[string]bool
	installed, restored        bool
}

func lifecycleDBPrincipalKey(user, host string) string { return user + "\x00" + host }
func lifecycleDBLiteral(v string) (string, error) {
	if v == "" || len(v) > 255 || strings.ContainsAny(v, "\\\x00\r\n") {
		return "", lifecycleError("lifecycle_database_principal_rejected")
	}
	return "'" + strings.ReplaceAll(v, "'", "''") + "'", nil
}
func lifecycleDBInputValid(v lifecycleDBWriterInput, r lifecycleRequest) bool {
	if v.FormatVersion != 1 || v.Kind != "qs_four_target_database_writer_expectations" || v.ToolSourceSHA != r.ToolSourceSHA || v.OriginalSourceSHA != r.OriginalSourceSHA || v.OperationID != r.OperationID || v.OriginalRunID != r.Recovery.OriginalRunID || v.TargetHash != digest(targets) || v.ManifestSHA256 != r.ManifestSHA256 || !shaRE.MatchString(v.CensusSourceSHA) || !runRE.MatchString(v.CensusRunID) || !hashRE.MatchString(v.Census.SHA256) || v.Census.Path != filepath.Join(lifecycleRootBatch(r.OperationID, v.CensusRunID), "db-writer-census.private.json") || len(v.OriginalActors) != 2 || len(v.SQLPrincipals) > 64 || len(v.MongoPrincipals) > 64 {
		return false
	}
	for _, observer := range []lifecycleDBPrincipalExpected{v.SQLObserver, v.MongoObserver} {
		if observer.User == "" || observer.HostOrDatabase == "" || len(observer.Owners) != 1 || observer.Owners[0] != "original_maintenance_run" {
			return false
		}
	}
	for _, list := range [][]lifecycleDBPrincipalExpected{v.SQLPrincipals, v.MongoPrincipals} {
		seen := map[string]bool{}
		for _, p := range list {
			k := lifecycleDBPrincipalKey(p.User, p.HostOrDatabase)
			if seen[k] || len(p.Owners) == 0 || len(p.Owners) > 3 {
				return false
			}
			seen[k] = true
			if _, e := lifecycleDBLiteral(p.User); e != nil {
				return false
			}
			if _, e := lifecycleDBLiteral(p.HostOrDatabase); e != nil {
				return false
			}
			owners := map[string]bool{}
			for _, owner := range p.Owners {
				if owners[owner] || owner != "qs-apiserver" && owner != "qs-worker" && owner != "qs-ai" && owner != "approved_maintenance" {
					return false
				}
				owners[owner] = true
			}
		}
	}
	return true
}

func lifecycleDBRequiredSections(c dbcensus.Catalog) error {
	for _, name := range []string{"mysql_accounts", "mysql_role_edges", "mysql_default_roles", "mysql_dynamic_grants", "mysql_proxy_grants", "mysql_observer_grants", "mysql_connections", "mysql_account_grants", "mongodb_observer_privileges", "mongodb_users", "mongodb_databases", "mongodb_stored_role_definitions", "mongodb_roles", "mongodb_authentication_parameters", "mongodb_connections_and_idle_operations", "mongodb_local_logical_sessions", "mongodb_persisted_logical_sessions"} {
		if !c.CompleteSection(name) {
			return lifecycleError("lifecycle_database_census_incomplete")
		}
		// Membership/grant rows must agree on two real reads; connection lifetimes
		// and session lastUse legitimately advance and are separately drained.
		if !strings.Contains(name, "connections") && !strings.Contains(name, "sessions") {
			for _, s := range c.Sections {
				if s.Name == name && !s.RecheckEqual {
					return lifecycleError("lifecycle_database_catalog_changed")
				}
			}
		}
	}
	return nil
}
func lifecycleDBStaticHash(c dbcensus.Catalog) string {
	accounts := [][]*string{}
	for _, row := range c.SQL["accounts"] {
		if len(row) != 4 {
			return ""
		}
		accounts = append(accounts, row[:3])
	}
	users := []bson.M{}
	for _, u := range c.Mongo["users"] {
		copy := bson.M{}
		for k, v := range u {
			if k != "roles" && k != "inheritedRoles" && k != "inheritedPrivileges" {
				copy[k] = v
			}
		}
		users = append(users, copy)
	}
	return lifecycleDBDigest(struct {
		Accounts [][]*string
		SQL      map[string][][]*string
		Mongo    map[string][]bson.M
		Users    []bson.M
	}{accounts, map[string][][]*string{"role_edges": c.SQL["role_edges"], "default_roles": c.SQL["default_roles"], "dynamic_grants": c.SQL["dynamic_grants"], "proxy_grants": c.SQL["proxy_grants"], "observer_grants": c.SQL["observer_grants"], "account_grants": c.SQL["account_grants"]}, map[string][]bson.M{"roles": c.Mongo["roles"], "stored_roles": c.Mongo["stored_role_definitions"], "databases": c.Mongo["databases"], "authentication_parameters": c.Mongo["authentication_parameters"], "observer_privileges": c.Mongo["observer_privileges"]}, users})
}
func lifecycleDBDigest(v any) string {
	raw, e := json.Marshal(v)
	if e != nil {
		return ""
	}
	var normalized any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if d.Decode(&normalized) != nil {
		return ""
	}
	return digest(normalized)
}

var lifecycleDBGrantScope = regexp.MustCompile(`(?i)^GRANT (.+) ON (.+) TO `)

func lifecycleSQLGrantMayWrite(g, db string) (bool, error) {
	if strings.HasPrefix(g, "REVOKE ") {
		return false, nil
	} // Conservative positive grants still include partially revoked scopes.
	m := lifecycleDBGrantScope.FindStringSubmatch(g)
	if m == nil {
		if strings.HasPrefix(g, "GRANT ") && strings.Contains(g, " TO ") {
			return false, nil
		}
		return false, lifecycleError("lifecycle_database_grant_schema_unproven")
	}
	scope := strings.ReplaceAll(m[2], "`", "")
	parts := strings.Split(scope, ".")
	if len(parts) != 2 {
		return false, lifecycleError("lifecycle_database_grant_schema_unproven")
	}
	mutates := false
	for _, priv := range strings.Split(strings.ToUpper(m[1]), ",") {
		priv = strings.TrimSpace(priv)
		if before, _, ok := strings.Cut(priv, " ("); ok {
			priv = before
		}
		switch priv {
		case "ALL PRIVILEGES", "INSERT", "UPDATE", "DELETE", "CREATE", "DROP", "ALTER", "INDEX", "TRIGGER", "CREATE VIEW", "CREATE USER", "CREATE ROLE", "ROLE_ADMIN", "SYSTEM_USER", "SUPER", "GRANT OPTION":
			mutates = true
		}
	}
	// EXECUTE may delegate to a definer even when the caller lacks direct DML.
	if strings.Contains(strings.ToUpper(m[1]), "EXECUTE") && (parts[0] == "*" || parts[0] == db) {
		return true, nil
	}
	if !mutates {
		return false, nil
	}
	// Unknown wildcard database patterns are not silently classified unrelated.
	if strings.ContainsAny(parts[0], "%_\\") && parts[0] != db {
		return false, lifecycleError("lifecycle_database_grant_scope_unproven")
	}
	if parts[0] == "mysql" {
		return true, nil
	} // Native authorization-table modification also controls target admission.
	if parts[0] != "*" && parts[0] != db {
		return false, nil
	}
	for _, t := range targets {
		if t[0] == "mysql" && (parts[1] == "*" || parts[1] == t[1]) {
			return true, nil
		}
	}
	return false, nil
}
func lifecycleDBArray(v any) ([]bson.M, error) {
	raw, e := bson.Marshal(bson.M{"items": v})
	if e != nil {
		return nil, e
	}
	var decoded struct {
		Items []bson.M `bson:"items"`
	}
	if bson.Unmarshal(raw, &decoded) != nil {
		return nil, lifecycleError("lifecycle_database_grant_schema_unproven")
	}
	return decoded.Items, nil
}
func lifecycleMongoMayWrite(u bson.M, db string) (bool, error) {
	ps, e := lifecycleDBArray(u["inheritedPrivileges"])
	if e != nil {
		return false, e
	}
	for _, p := range ps {
		raw, e := bson.Marshal(p)
		if e != nil {
			return false, e
		}
		var privilege struct {
			Resource struct {
				DB         string `bson:"db"`
				Collection string `bson:"collection"`
				Any        bool   `bson:"anyResource"`
				Cluster    bool   `bson:"cluster"`
			} `bson:"resource"`
			Actions []string `bson:"actions"`
		}
		if bson.Unmarshal(raw, &privilege) != nil {
			return false, lifecycleError("lifecycle_database_grant_schema_unproven")
		}
		for _, a := range privilege.Actions {
			switch a {
			case "grantRole", "revokeRole", "createUser", "updateUser", "createRole", "updateRole", "dropUser", "dropRole", "setParameter", "applyOps", "internal", "anyAction", "bypassWriteBlockingMode":
				return true, nil
			case "insert", "update", "remove", "dropCollection", "dropDatabase", "createCollection", "createIndex", "dropIndex", "collMod", "renameCollectionSameDB", "bypassDocumentValidation":
				if privilege.Resource.Any || !privilege.Resource.Cluster && (privilege.Resource.DB == "" || privilege.Resource.DB == db) && (privilege.Resource.Collection == "" || privilege.Resource.Collection == "domain_event_outbox") {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// This reference is a previously observed native census, not a caller-created
// expectation. Only the exact operation/run-owned census file is accepted.
func lifecycleDBInputCensusRun(path, operation string) (string, bool) {
	if !runRE.MatchString(operation) || filepath.Clean(path) != path || filepath.Base(path) != "db-writer-census.private.json" {
		return "", false
	}
	run := strings.TrimPrefix(filepath.Base(filepath.Dir(path)), operation+"-")
	return run, runRE.MatchString(run) && path == filepath.Join(lifecycleRootBatch(operation, run), "db-writer-census.private.json")
}
func lifecycleDBCensusProducerValid(c dbCensusPrivate, r lifecycleRequest, run string) bool {
	p := c.IdentityProducer
	return c.SourceSHA == r.OriginalSourceSHA && c.OperationID == r.OperationID && c.RunID == run && hashRE.MatchString(c.RequestSHA256) && !c.WriterScopeComplete && p.OperationID == r.OperationID && p.SourceSHA == r.OriginalSourceSHA && runRE.MatchString(p.RunID) && hashRE.MatchString(p.ReportSHA256) && hashRE.MatchString(p.RequestSHA256)
}

func (h *lifecycleFixedHost) observeDatabaseMaintenancePrincipals(ctx context.Context) (sqlObserver, mongoObserver lifecycleDBPrincipalExpected, database string, result error) {
	var authenticated, login string
	if h.owner.originalConn.QueryRowContext(ctx, "SELECT CURRENT_USER(),USER(),DATABASE()").Scan(&authenticated, &login, &database) != nil {
		result = lifecycleError("lifecycle_database_identity_or_authentication_rejected")
		return
	}
	index := strings.LastIndexByte(authenticated, '@')
	if index <= 0 || index == len(authenticated)-1 || !strings.HasPrefix(login, authenticated[:index]+"@") || database == "" {
		result = lifecycleError("lifecycle_database_identity_or_authentication_rejected")
		return
	}
	sqlObserver = lifecycleDBPrincipalExpected{User: authenticated[:index], HostOrDatabase: authenticated[index+1:], Owners: []string{"original_maintenance_run"}}
	var auth struct {
		AuthInfo struct {
			Users []struct {
				User string `bson:"user"`
				DB   string `bson:"db"`
			} `bson:"authenticatedUsers"`
		} `bson:"authInfo"`
	}
	if h.owner.originalMongo.Database("admin").RunCommand(ctx, bson.D{{Key: "connectionStatus", Value: 1}, {Key: "showPrivileges", Value: true}}).Decode(&auth) != nil || len(auth.AuthInfo.Users) != 1 {
		result = lifecycleError("lifecycle_database_identity_or_authentication_rejected")
		return
	}
	mongoObserver = lifecycleDBPrincipalExpected{User: auth.AuthInfo.Users[0].User, HostOrDatabase: auth.AuthInfo.Users[0].DB, Owners: []string{"original_maintenance_run"}}
	return
}

// Configured actors identify ownership; the actual inherited grant catalog
// decides whether a principal is in this four-target scope. No maintenance
// account is guessed, and subsequent coverage rejects every unowned writer.
func lifecycleProduceDBWriterInput(r lifecycleRequest, census lifecycleFinalFileBinding, original dbCensusPrivate, actual dbcensus.Catalog, actors []stop.DatabasePrincipal, aiUser string, sqlObserver, mongoObserver lifecycleDBPrincipalExpected, sqlDatabase, mongoDatabase string) (lifecycleDBWriterInput, error) {
	in := lifecycleDBWriterInput{FormatVersion: 1, Kind: "qs_four_target_database_writer_expectations", ToolSourceSHA: r.ToolSourceSHA, OriginalSourceSHA: r.OriginalSourceSHA, OperationID: r.OperationID, OriginalRunID: r.Recovery.OriginalRunID, TargetHash: digest(targets), ManifestSHA256: r.ManifestSHA256, Census: census, CensusSourceSHA: original.SourceSHA, CensusRunID: original.RunID, SQLObserver: sqlObserver, MongoObserver: mongoObserver, OriginalActors: actors}
	if len(actors) != 2 || aiUser == "" || sqlDatabase == "" || mongoDatabase == "" {
		return in, lifecycleError("lifecycle_database_original_config_changed")
	}
	writers, e := lifecycleDBSQLWriters(actual, sqlDatabase)
	if e != nil {
		return in, e
	}
	appendOwner := func(list *[]lifecycleDBPrincipalExpected, p lifecycleDBPrincipalExpected, owner string) {
		for i := range *list {
			if (*list)[i].User == p.User && (*list)[i].HostOrDatabase == p.HostOrDatabase {
				(*list)[i].Owners = append((*list)[i].Owners, owner)
				return
			}
		}
		p.Owners = []string{owner}
		*list = append(*list, p)
	}
	addSQL := func(user, owner string) error {
		var account []*string
		for _, row := range actual.SQL["accounts"] {
			if val(row, 0) == user {
				if len(row) != 4 || account != nil {
					return lifecycleError("lifecycle_database_ambiguous_session_owner")
				}
				account = row
			}
		}
		if account == nil {
			return lifecycleError("lifecycle_database_principal_rejected")
		}
		p := lifecycleDBPrincipalExpected{User: user, HostOrDatabase: val(account, 1)}
		if user == sqlObserver.User {
			return lifecycleError("lifecycle_database_shared_maintenance_account")
		}
		if writers[lifecycleDBPrincipalKey(p.User, p.HostOrDatabase)] {
			appendOwner(&in.SQLPrincipals, p, owner)
		}
		return nil
	}
	components := map[string]bool{}
	for _, a := range actors {
		if a.Component != "qs-apiserver" && a.Component != "qs-worker" || components[a.Component] || !hashRE.MatchString(a.ContainerID) || !hashRE.MatchString(a.EnvironmentSHA256) || a.SQLUser == "" || a.MongoUser == "" || a.SQLDatabase != sqlDatabase || a.MongoDatabase != mongoDatabase {
			return in, lifecycleError("lifecycle_database_original_config_changed")
		}
		components[a.Component] = true
		if e = addSQL(a.SQLUser, a.Component); e != nil {
			return in, e
		}
		var user bson.M
		for _, u := range actual.Mongo["users"] {
			if u["user"] == a.MongoUser && u["db"] == "admin" {
				if user != nil {
					return in, lifecycleError("lifecycle_database_ambiguous_session_owner")
				}
				user = u
			}
		}
		if user == nil {
			return in, lifecycleError("lifecycle_database_principal_rejected")
		}
		if a.MongoUser == mongoObserver.User {
			return in, lifecycleError("lifecycle_database_shared_maintenance_account")
		}
		yes, err := lifecycleMongoMayWrite(user, mongoDatabase)
		if err != nil {
			return in, err
		}
		if yes {
			appendOwner(&in.MongoPrincipals, lifecycleDBPrincipalExpected{User: a.MongoUser, HostOrDatabase: "admin"}, a.Component)
		}
	}
	if e = addSQL(aiUser, "qs-ai"); e != nil {
		return in, e
	}
	if !lifecycleDBInputValid(in, r) {
		return in, lifecycleError("lifecycle_database_writer_input_rejected")
	}
	return in, nil
}

func (h *lifecycleFixedHost) installDatabaseWriterLease(ctx context.Context, r lifecycleRequest) error {
	if h == nil || h.dbWriters != nil || h.owner == nil || h.owner.originalConn == nil || h.owner.originalMongo == nil || h.services == nil || h.services.window == nil || h.services.local == nil || h.services.remote == nil || h.aiStopped == nil || !r.WriterControl.valid() || r.WriterControl.DatabaseInput == nil {
		return lifecycleError("lifecycle_database_writer_input_missing")
	}
	if e := validateLifecycleAPIInvocation(r); e != nil {
		return e
	}
	intentRaw, e := readLifecycleAPIRecord(filepath.Join(r.prepareRoot, "native-call.intent.private.json"))
	if e != nil {
		return e
	}
	intent, e := decodeLifecycleAPIInvocationIntent(intentRaw)
	if e != nil {
		return e
	}
	f := *r.WriterControl.DatabaseInput
	var in lifecycleDBWriterInput
	var original dbCensusPrivate
	censusRun, nativeProducer := lifecycleDBInputCensusRun(f.Path, r.OperationID)
	if nativeProducer {
		if readLifecyclePrivateAs(f.Path, f.SHA256, &original, 64<<20, 0) != nil || !lifecycleDBCensusProducerValid(original, r, censusRun) {
			return lifecycleError("lifecycle_database_census_binding_rejected")
		}
	} else {
		if !lifecycleOwnedPath(filepath.Join("/opt/backups/qs-server/compatibility-retirement", r.OperationID), f.Path) || readLifecyclePrivateAs(f.Path, f.SHA256, &in, 256<<10, intent.SourceUID) != nil || !lifecycleDBInputValid(in, r) {
			return lifecycleError("lifecycle_database_writer_input_rejected")
		}
		if readLifecyclePrivateAs(in.Census.Path, in.Census.SHA256, &original, 64<<20, 0) != nil || original.SourceSHA != in.CensusSourceSHA || original.OperationID != r.OperationID || original.RunID != in.CensusRunID || original.WriterScopeComplete || original.IdentityProducer.OperationID != r.OperationID {
			return lifecycleError("lifecycle_database_census_binding_rejected")
		}
	}
	var manifest lifecycleFrozenManifest
	if readLifecyclePrivate(filepath.Join(r.prepareRoot, "manifest.json"), r.ManifestSHA256, &manifest) != nil || original.SQLIdentitySHA256 != manifest.DatabaseBindings["mysql"].IdentityHash || original.MongoIdentitySHA256 != manifest.DatabaseBindings["mongodb"].IdentityHash {
		return lifecycleError("lifecycle_database_identity_rejected")
	}
	local, e := h.services.local.ObserveDatabasePrincipals(ctx)
	if e != nil {
		return e
	}
	d, e := h.services.remote.Do(ctx, "observe_db_principals")
	if e != nil {
		return e
	}
	actors := append(local, d.DatabasePrincipals...)
	if !nativeProducer && digest(actors) != digest(in.OriginalActors) {
		return lifecycleError("lifecycle_database_original_config_changed")
	}
	aiUser, _, e := h.aiStopped.DatabasePrincipal(ctx)
	if e != nil {
		return e
	}
	actual, observeErr := dbcensus.ObserveConnection(ctx, h.owner.originalConn, h.owner.originalMongo)
	// The old diagnostic deliberately lacks startup-auth coverage. This leaf
	// proves its narrow native auth/topology itself; every required section must
	// still be actual, complete and stable, even when Observe returns incomplete.
	if observeErr != nil && !errors.Is(observeErr, dbcensus.ErrIncomplete) {
		return observeErr
	}
	if e = lifecycleDBRequiredSections(actual); e != nil {
		return e
	}
	baseline := dbcensus.Catalog{SQL: original.SQL, Mongo: original.Mongo, Sections: original.Sections, Gaps: original.Gaps}
	if e = lifecycleDBRequiredSections(baseline); e != nil {
		return e
	}
	if lifecycleDBStaticHash(actual) == "" || lifecycleDBStaticHash(actual) != lifecycleDBStaticHash(baseline) || digest(actual.SQL["accounts"]) != digest(baseline.SQL["accounts"]) || lifecycleDBDigest(actual.Mongo["users"]) != lifecycleDBDigest(baseline.Mongo["users"]) {
		return lifecycleError("lifecycle_database_catalog_changed")
	}
	if nativeProducer {
		sqlObserver, mongoObserver, database, err := h.observeDatabaseMaintenancePrincipals(ctx)
		if err != nil {
			return err
		}
		in, e = lifecycleProduceDBWriterInput(r, f, original, actual, actors, aiUser, sqlObserver, mongoObserver, database, h.owner.originalDB.Name())
		if e != nil {
			return e
		}
	}
	// Current run binding comes from the original protected native invocation,
	// not from a future-run prediction in an independently approved expectation.
	v := &lifecycleDBWriterLease{host: h, window: h.services.window, binding: lifecycleWindowBinding(r), actualRunID: r.ActualRunID, input: in, baseline: actual, staticSHA: lifecycleDBStaticHash(actual), sqlIdentity: original.SQLIdentitySHA256, mongoIdentity: original.MongoIdentitySHA256, sqlAttempted: map[string]bool{}, mongoAttempted: map[string]bool{}}
	v.self = v
	if e = v.validateOriginalPrincipals(ctx, actors, aiUser); e != nil {
		return e
	}
	if e = v.nativeIdentityAndAuthentication(ctx); e != nil {
		return e
	}
	if e = v.validateWriterCoverage(actual, false); e != nil {
		return e
	}
	if e = v.checkMongoTransactions(ctx); e != nil {
		return e
	}
	if e = v.checkSQLPreparedAndDelegated(ctx); e != nil {
		return e
	}
	h.dbWriters = v // Original native owner retained BEFORE the first mutating command.
	if nativeProducer {
		encoded, err := json.Marshal(in)
		if err != nil || len(encoded) > 256<<10 {
			return lifecycleError("lifecycle_database_writer_record_rejected")
		}
		// Exact exclusive native output, owned by this invocation. The original
		// caller's input/hash is never rewritten and this is never an installed token.
		v.inputRecord = &lifecycleFinalFileBinding{Path: filepath.Join(r.prepareRoot, "database-writer-expectations.private.json"), SHA256: digestRaw(append(encoded, '\n'))}
		if err = writeJSON(v.inputRecord.Path, in); err != nil || readLifecyclePrivate(v.inputRecord.Path, v.inputRecord.SHA256, new(lifecycleDBWriterInput)) != nil {
			return lifecycleError("lifecycle_database_writer_record_write_failed")
		}
	}
	for _, p := range in.SQLPrincipals {
		row := v.sqlAccount(actual, p)
		if row == nil {
			return lifecycleError("lifecycle_database_principal_rejected")
		}
		if val(row, 3) == "Y" {
			continue
		}
		v.sqlAttempted[lifecycleDBPrincipalKey(p.User, p.HostOrDatabase)] = true
		if e = v.setSQLLock(ctx, p, true); e != nil {
			return e
		}
	}
	if e = v.drainSQL(ctx); e != nil {
		return e
	}
	for _, p := range in.MongoPrincipals {
		u := v.mongoUser(actual, p)
		if u == nil {
			return lifecycleError("lifecycle_database_principal_rejected")
		}
		roles, e := lifecycleDBArray(u["roles"])
		if e != nil {
			return e
		}
		if len(roles) == 0 {
			continue
		}
		v.mongoAttempted[lifecycleDBPrincipalKey(p.User, p.HostOrDatabase)] = true
		if e = v.setMongoRoles(ctx, p, roles, false); e != nil {
			return e
		}
	}
	if e = v.drainMongo(ctx); e != nil {
		return e
	}
	if e = v.check(ctx, false); e != nil {
		return e
	}
	v.installed = true
	return nil
}

func (v *lifecycleDBWriterLease) sqlAccount(c dbcensus.Catalog, p lifecycleDBPrincipalExpected) []*string {
	for _, row := range c.SQL["accounts"] {
		if len(row) == 4 && val(row, 0) == p.User && val(row, 1) == p.HostOrDatabase {
			return row
		}
	}
	return nil
}
func (v *lifecycleDBWriterLease) mongoUser(c dbcensus.Catalog, p lifecycleDBPrincipalExpected) bson.M {
	for _, u := range c.Mongo["users"] {
		if u["user"] == p.User && u["db"] == p.HostOrDatabase {
			return u
		}
	}
	return nil
}
func (v *lifecycleDBWriterLease) validateOriginalPrincipals(ctx context.Context, actors []stop.DatabasePrincipal, aiUser string) error {
	if len(actors) != 2 || len(v.input.OriginalActors) != 2 || digest(actors) != digest(v.input.OriginalActors) {
		return lifecycleError("lifecycle_database_original_config_changed")
	}
	for _, scope := range []struct {
		mongo      bool
		principals []lifecycleDBPrincipalExpected
	}{{false, v.input.SQLPrincipals}, {true, v.input.MongoPrincipals}} {
		for _, p := range scope.principals {
			for _, owner := range p.Owners {
				if owner == "approved_maintenance" {
					continue
				}
				if owner == "qs-ai" {
					if scope.mongo || p.User != aiUser {
						return lifecycleError("lifecycle_database_original_config_changed")
					}
					continue
				}
				matched := false
				for _, a := range actors {
					if a.Component != owner {
						continue
					}
					user := a.SQLUser
					if scope.mongo {
						user = a.MongoUser
					}
					if p.User == user && a.SQLDatabase == os.Getenv("MYSQL_DATABASE") && a.MongoDatabase == v.host.owner.originalDB.Name() && (!scope.mongo || p.HostOrDatabase == "admin") {
						matched = true
					}
				}
				if !matched {
					return lifecycleError("lifecycle_database_original_config_changed")
				}
			}
		}
	}
	return ctx.Err()
}

func (v *lifecycleDBWriterLease) nativeIdentityAndAuthentication(ctx context.Context) error {
	if v == nil || v.self != v || v.host == nil || v.host.dbWriters != nil && v.host.dbWriters != v || v.host.owner == nil || v.window == nil || v.host.services == nil || v.host.services.window != v.window {
		return lifecycleError("lifecycle_database_writer_binding_rejected")
	}
	for _, params := range v.baseline.Mongo["authentication_parameters"] {
		raw, e := bson.Marshal(params)
		var native struct {
			Mechanisms []string `bson:"authenticationMechanisms"`
		}
		if e != nil || bson.Unmarshal(raw, &native) != nil || len(native.Mechanisms) == 0 {
			return lifecycleError("lifecycle_mongodb_external_authentication_unproven")
		}
		for _, m := range native.Mechanisms {
			if m != "SCRAM-SHA-1" && m != "SCRAM-SHA-256" && m != "MONGODB-X509" {
				return lifecycleError("lifecycle_mongodb_external_authentication_unproven")
			}
		}
	}
	d, e := v.window.Diagnostic(ctx)
	if e != nil || d.Binding != v.binding || !d.DirectoryLeaseHeld || d.RemainingMilliseconds <= 0 {
		return lifecycleError("lifecycle_database_writer_binding_rejected")
	}
	var id uint64
	var user, login, uuid, db, version, mandatory string
	var autocommit int
	e = v.host.owner.originalConn.QueryRowContext(ctx, "SELECT CONNECTION_ID(),CURRENT_USER(),USER(),@@server_uuid,DATABASE(),VERSION(),@@mandatory_roles,@@autocommit").Scan(&id, &user, &login, &uuid, &db, &version, &mandatory, &autocommit)
	if e != nil || id == 0 || autocommit != 1 || mandatory != "" || !strings.HasPrefix(version, "8.") || hashParts("mysql_database_identity_v1", uuid, db) != v.sqlIdentity || user != v.input.SQLObserver.User+"@"+v.input.SQLObserver.HostOrDatabase || !strings.HasPrefix(login, v.input.SQLObserver.User+"@") || v.connectionID != 0 && v.connectionID != id {
		return lifecycleError("lifecycle_database_identity_or_authentication_rejected")
	}
	if v.host.dbWriters == nil {
		var head uint64
		var dirty bool
		if v.host.owner.originalConn.QueryRowContext(ctx, "SELECT version,dirty FROM schema_migrations LIMIT 1").Scan(&head, &dirty) != nil || head != 99 || dirty {
			return lifecycleError("lifecycle_database_original_head_rejected")
		}
	}
	v.connectionID = id
	v.sqlDatabase = db
	admin := v.host.owner.originalMongo.Database("admin")
	var auth struct {
		AuthInfo struct {
			Users []struct {
				User string `bson:"user"`
				DB   string `bson:"db"`
			} `bson:"authenticatedUsers"`
		} `bson:"authInfo"`
	}
	if admin.RunCommand(ctx, bson.D{{Key: "connectionStatus", Value: 1}, {Key: "showPrivileges", Value: true}}).Decode(&auth) != nil || len(auth.AuthInfo.Users) != 1 || auth.AuthInfo.Users[0].User != v.input.MongoObserver.User || auth.AuthInfo.Users[0].DB != v.input.MongoObserver.HostOrDatabase {
		return lifecycleError("lifecycle_database_identity_or_authentication_rejected")
	}
	var startup struct {
		Parsed struct {
			Security struct {
				Authorization string `bson:"authorization"`
			} `bson:"security"`
		} `bson:"parsed"`
	}
	// Decode only the authorization field. Native argv/LDAP/credential bytes
	// are neither retained nor exported; this is not an authentication platform.
	if admin.RunCommand(ctx, bson.D{{Key: "getCmdLineOpts", Value: 1}}).Decode(&startup) != nil || startup.Parsed.Security.Authorization != "enabled" {
		return lifecycleError("lifecycle_mongodb_authorization_unproven")
	}
	var hello bson.Raw
	if admin.RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello) != nil || hello.Lookup("msg").Type != 0 {
		return lifecycleError("lifecycle_mongodb_single_node_scope_unproven")
	}
	connection := hello.Lookup("connectionId")
	var mongoID int64
	if connection.Type == bson.TypeInt64 {
		mongoID = connection.Int64()
	} else if connection.Type == bson.TypeInt32 {
		mongoID = int64(connection.Int32())
	}
	if mongoID <= 0 || v.mongoConnectionID != 0 && v.mongoConnectionID != mongoID {
		return lifecycleError("lifecycle_database_identity_or_authentication_rejected")
	}
	v.mongoConnectionID = mongoID
	hosts := hello.Lookup("hosts")
	if hosts.Type != bson.TypeArray {
		return lifecycleError("lifecycle_mongodb_single_node_scope_unproven")
	}
	items, e := hosts.Array().Values()
	if e != nil || len(items) != 1 || hello.Lookup("passives").Type != 0 || hello.Lookup("arbiters").Type != 0 {
		return lifecycleError("lifecycle_mongodb_single_node_scope_unproven")
	}
	stable := bson.D{}
	for _, name := range []string{"setName", "hosts", "me"} {
		value := hello.Lookup(name)
		if value.Type != 0 {
			var decoded any
			if value.Unmarshal(&decoded) != nil {
				return lifecycleError("lifecycle_database_identity_rejected")
			}
			stable = append(stable, bson.E{Key: name, Value: decoded})
		}
	}
	cur, e := v.host.owner.originalDB.ListCollections(ctx, bson.D{{Key: "name", Value: "schema_migrations"}}, options.ListCollections().SetAuthorizedCollections(false))
	if e != nil {
		return lifecycleError("lifecycle_database_identity_rejected")
	}
	var cols []bson.Raw
	e = cur.All(ctx, &cols)
	ce := cur.Close(ctx)
	if e != nil || ce != nil || len(cols) != 1 {
		return lifecycleError("lifecycle_database_identity_rejected")
	}
	u := cols[0].Lookup("info", "uuid")
	if u.Type != bson.TypeBinary {
		return lifecycleError("lifecycle_database_identity_rejected")
	}
	sub, raw := u.Binary()
	canonical, _ := json.Marshal(stable)
	if sub != 4 || len(raw) != 16 || hashParts("mongodb_database_identity_v1", string(canonical), v.host.owner.originalDB.Name(), hex.EncodeToString(raw)) != v.mongoIdentity {
		return lifecycleError("lifecycle_database_identity_rejected")
	}
	if v.host.dbWriters == nil {
		var head struct {
			Version int64 `bson:"version"`
			Dirty   bool  `bson:"dirty"`
		}
		if v.host.owner.originalDB.Collection("schema_migrations").FindOne(ctx, bson.D{}).Decode(&head) != nil || head.Version != 38 || head.Dirty {
			return lifecycleError("lifecycle_database_original_head_rejected")
		}
	}
	return nil
}

func lifecycleDBSQLWriters(c dbcensus.Catalog, database string) (map[string]bool, error) {
	writes := map[string]bool{}
	for _, row := range c.SQL["account_grants"] {
		if len(row) != 3 {
			return nil, lifecycleError("lifecycle_database_grant_schema_unproven")
		}
		yes, e := lifecycleSQLGrantMayWrite(val(row, 2), database)
		if e != nil {
			return nil, e
		}
		if yes {
			writes[lifecycleDBPrincipalKey(val(row, 0), val(row, 1))] = true
		}
	}
	for _, row := range c.SQL["dynamic_grants"] {
		if len(row) != 4 {
			return nil, lifecycleError("lifecycle_database_grant_schema_unproven")
		}
		switch val(row, 2) {
		case "ROLE_ADMIN", "SYSTEM_VARIABLES_ADMIN", "CONNECTION_ADMIN", "SYSTEM_USER":
			writes[lifecycleDBPrincipalKey(val(row, 0), val(row, 1))] = true
		}
	}
	// Any granted role can be activated, not just a default role. Bound the
	// propagation by account count and fail if an edge names an absent principal.
	for n := 0; n <= len(c.SQL["accounts"]); n++ {
		changed := false
		for _, row := range c.SQL["role_edges"] {
			if len(row) != 5 {
				return nil, lifecycleError("lifecycle_database_grant_schema_unproven")
			}
			from, to := lifecycleDBPrincipalKey(val(row, 1), val(row, 0)), lifecycleDBPrincipalKey(val(row, 3), val(row, 2))
			if writes[from] && !writes[to] {
				writes[to] = true
				changed = true
			}
		}
		if !changed {
			break
		}
		if n == len(c.SQL["accounts"]) {
			return nil, lifecycleError("lifecycle_database_role_scope_unproven")
		}
	}
	for _, row := range c.SQL["proxy_grants"] {
		if len(row) != 5 || writes[lifecycleDBPrincipalKey(val(row, 1), val(row, 0))] || writes[lifecycleDBPrincipalKey(val(row, 3), val(row, 2))] {
			return nil, lifecycleError("lifecycle_database_proxy_scope_unproven")
		}
	}
	return writes, nil
}

func (v *lifecycleDBWriterLease) validateWriterCoverage(c dbcensus.Catalog, locked bool) error {
	if lifecycleDBStaticHash(c) != v.staticSHA {
		return lifecycleError("lifecycle_database_catalog_changed")
	}
	owned := map[string]bool{}
	names := map[string]int{}
	for _, row := range c.SQL["accounts"] {
		names[val(row, 0)]++
	}
	for _, p := range v.input.SQLPrincipals {
		owned[lifecycleDBPrincipalKey(p.User, p.HostOrDatabase)] = true
		if p.User == v.input.SQLObserver.User {
			return lifecycleError("lifecycle_database_shared_maintenance_account")
		}
	}
	writes, e := lifecycleDBSQLWriters(c, v.sqlDatabase)
	if e != nil {
		return e
	}
	observer := lifecycleDBPrincipalKey(v.input.SQLObserver.User, v.input.SQLObserver.HostOrDatabase)
	for _, row := range c.SQL["accounts"] {
		if len(row) != 4 || val(row, 3) != "Y" && val(row, 3) != "N" {
			return lifecycleError("lifecycle_database_principal_rejected")
		}
		user, host := val(row, 0), val(row, 1)
		key := lifecycleDBPrincipalKey(user, host)
		if names[user] != 1 && (owned[key] || user == v.input.SQLObserver.User) {
			return lifecycleError("lifecycle_database_ambiguous_session_owner")
		}
		original := v.sqlAccount(v.baseline, lifecycleDBPrincipalExpected{User: user, HostOrDatabase: host})
		if original == nil || !owned[key] && val(row, 3) != val(original, 3) || !locked && owned[key] && val(row, 3) != val(original, 3) {
			return lifecycleError("lifecycle_database_account_restore_conflict")
		}
		if owned[key] && (val(row, 2) != "caching_sha2_password" && val(row, 2) != "mysql_native_password" || locked && val(row, 3) != "Y") {
			return lifecycleError("lifecycle_database_admission_unproven")
		}
		if writes[key] && key != observer && val(row, 3) != "Y" && !owned[key] {
			return lifecycleError("lifecycle_database_unowned_target_writer")
		}
	}
	for _, p := range v.input.SQLPrincipals {
		if v.sqlAccount(c, p) == nil || !writes[lifecycleDBPrincipalKey(p.User, p.HostOrDatabase)] {
			return lifecycleError("lifecycle_database_principal_rejected")
		}
	}
	for _, row := range c.SQL["connections"] {
		if len(row) != 6 {
			return lifecycleError("lifecycle_database_session_schema_unproven")
		}
		id, e := strconv.ParseUint(val(row, 0), 10, 64)
		if e != nil || id == 0 {
			return lifecycleError("lifecycle_database_session_schema_unproven")
		}
		if val(row, 1) == v.input.SQLObserver.User && id != v.connectionID {
			return lifecycleError("lifecycle_database_shared_maintenance_account")
		}
		if val(row, 1) != v.input.SQLObserver.User {
			for key, yes := range writes {
				if yes && strings.HasPrefix(key, val(row, 1)+"\x00") && !owned[key] {
					return lifecycleError("lifecycle_database_unowned_target_writer")
				}
			}
		}
		if locked {
			for _, p := range v.input.SQLPrincipals {
				if val(row, 1) == p.User {
					return lifecycleError("lifecycle_database_session_drain_unproven")
				}
			}
		}
	}
	for _, u := range c.Mongo["users"] {
		user, ok := u["user"].(string)
		authDB, dbok := u["db"].(string)
		if !ok || !dbok {
			return lifecycleError("lifecycle_database_grant_schema_unproven")
		}
		p := lifecycleDBPrincipalExpected{User: user, HostOrDatabase: authDB}
		listed := false
		for _, expect := range v.input.MongoPrincipals {
			if expect.User == user && expect.HostOrDatabase == authDB {
				listed = true
			}
		}
		yes, e := lifecycleMongoMayWrite(u, v.host.owner.originalDB.Name())
		if e != nil {
			return e
		}
		if yes && !listed && (user != v.input.MongoObserver.User || authDB != v.input.MongoObserver.HostOrDatabase) {
			return lifecycleError("lifecycle_database_unowned_target_writer")
		}
		if !listed && lifecycleDBDigest(u) != lifecycleDBDigest(v.mongoUser(v.baseline, p)) {
			return lifecycleError("lifecycle_database_catalog_changed")
		}
		if listed {
			if user == v.input.MongoObserver.User {
				return lifecycleError("lifecycle_database_shared_maintenance_account")
			}
			roles, e := lifecycleDBArray(u["roles"])
			if e != nil || locked && len(roles) != 0 {
				return lifecycleError("lifecycle_database_admission_unproven")
			}
			base := v.mongoUser(v.baseline, p)
			if base == nil {
				return lifecycleError("lifecycle_database_principal_rejected")
			}
			baseRoles, e := lifecycleDBArray(base["roles"])
			if e != nil || !locked && lifecycleDBDigest(roles) != lifecycleDBDigest(baseRoles) {
				return lifecycleError("lifecycle_database_catalog_changed")
			}
		}
	}
	for _, p := range v.input.MongoPrincipals {
		u := v.mongoUser(c, p)
		yes, e := lifecycleMongoMayWrite(v.mongoUser(v.baseline, p), v.host.owner.originalDB.Name())
		if u == nil || e != nil || !yes {
			return lifecycleError("lifecycle_database_principal_rejected")
		}
	}
	for _, op := range c.Mongo["connections_and_idle_operations"] {
		users, e := lifecycleDBArray(op["effectiveUsers"])
		if e != nil {
			return lifecycleError("lifecycle_database_session_schema_unproven")
		}
		for _, u := range users {
			if u["user"] == v.input.MongoObserver.User && u["db"] == v.input.MongoObserver.HostOrDatabase {
				raw, e := bson.Marshal(op)
				var native struct {
					ID int64 `bson:"connectionId"`
				}
				if e != nil || bson.Unmarshal(raw, &native) != nil || native.ID != v.mongoConnectionID {
					return lifecycleError("lifecycle_database_shared_maintenance_account")
				}
			}
		}
	}
	return nil
}

func (v *lifecycleDBWriterLease) setSQLLock(ctx context.Context, p lifecycleDBPrincipalExpected, locked bool) error {
	u, e := lifecycleDBLiteral(p.User)
	if e != nil {
		return e
	}
	host, e := lifecycleDBLiteral(p.HostOrDatabase)
	if e != nil {
		return e
	}
	var current string
	if v.host.owner.originalConn.QueryRowContext(ctx, "SELECT account_locked FROM mysql.user WHERE User=? AND Host=?", p.User, p.HostOrDatabase).Scan(&current) != nil || current != "Y" && current != "N" {
		return lifecycleError("lifecycle_database_account_state_unknown")
	}
	expected := "N"
	next := "Y"
	verb := "LOCK"
	if !locked {
		expected, next, verb = "Y", "N", "UNLOCK"
	}
	if current == next && !locked {
		return nil
	}
	if current != expected {
		return lifecycleError("lifecycle_database_account_restore_conflict")
	}
	if _, e = v.host.owner.originalConn.ExecContext(ctx, "ALTER USER "+u+"@"+host+" ACCOUNT "+verb); e != nil {
		return lifecycleError("lifecycle_database_account_effect_unknown")
	}
	if v.host.owner.originalConn.QueryRowContext(ctx, "SELECT account_locked FROM mysql.user WHERE User=? AND Host=?", p.User, p.HostOrDatabase).Scan(&current) != nil || current != next {
		return lifecycleError("lifecycle_database_account_effect_unknown")
	}
	return nil
}
func (v *lifecycleDBWriterLease) drainSQL(ctx context.Context) error {
	for _, p := range v.input.SQLPrincipals {
		rows, e := v.host.owner.originalConn.QueryContext(ctx, "SELECT ID FROM information_schema.PROCESSLIST WHERE USER=? ORDER BY ID", p.User)
		if e != nil {
			return lifecycleError("lifecycle_database_session_drain_unproven")
		}
		ids := []uint64{}
		for rows.Next() {
			var id uint64
			if rows.Scan(&id) != nil || id == 0 || id == v.connectionID || len(ids) >= 32768 {
				_ = rows.Close()
				return lifecycleError("lifecycle_database_session_drain_unproven")
			}
			ids = append(ids, id)
		}
		e = rows.Err()
		ce := rows.Close()
		if e != nil || ce != nil {
			return lifecycleError("lifecycle_database_session_drain_unproven")
		}
		for _, id := range ids {
			var owner string
			e = v.host.owner.originalConn.QueryRowContext(ctx, "SELECT USER FROM information_schema.PROCESSLIST WHERE ID=?", id).Scan(&owner)
			if errors.Is(e, sql.ErrNoRows) {
				continue
			}
			if e != nil || owner != p.User {
				return lifecycleError("lifecycle_database_session_owner_changed")
			}
			if _, e = v.host.owner.originalConn.ExecContext(ctx, "KILL CONNECTION "+strconv.FormatUint(id, 10)); e != nil {
				return lifecycleError("lifecycle_database_session_effect_unknown")
			}
		}
	}
	return nil
}
func (v *lifecycleDBWriterLease) readMongoRoles(ctx context.Context, p lifecycleDBPrincipalExpected) ([]bson.M, error) {
	var reply struct {
		Users []struct {
			Roles []bson.M `bson:"roles"`
		} `bson:"users"`
	}
	if v.host.owner.originalMongo.Database(p.HostOrDatabase).RunCommand(ctx, bson.D{{Key: "usersInfo", Value: bson.D{{Key: "user", Value: p.User}, {Key: "db", Value: p.HostOrDatabase}}}}).Decode(&reply) != nil || len(reply.Users) != 1 {
		return nil, lifecycleError("lifecycle_database_account_state_unknown")
	}
	return reply.Users[0].Roles, nil
}
func (v *lifecycleDBWriterLease) setMongoRoles(ctx context.Context, p lifecycleDBPrincipalExpected, original []bson.M, restore bool) error {
	current, e := v.readMongoRoles(ctx, p)
	if e != nil {
		return e
	}
	if restore && lifecycleDBDigest(current) == lifecycleDBDigest(original) {
		return nil
	}
	if restore && len(current) != 0 || !restore && lifecycleDBDigest(current) != lifecycleDBDigest(original) {
		return lifecycleError("lifecycle_database_account_restore_conflict")
	}
	command := "revokeRolesFromUser"
	if restore {
		command = "grantRolesToUser"
	}
	if v.host.owner.originalMongo.Database(p.HostOrDatabase).RunCommand(ctx, bson.D{{Key: command, Value: p.User}, {Key: "roles", Value: original}, {Key: "writeConcern", Value: bson.D{{Key: "w", Value: "majority"}}}}).Err() != nil {
		return lifecycleError("lifecycle_database_account_effect_unknown")
	}
	if v.host.owner.originalMongo.Database("admin").RunCommand(ctx, bson.D{{Key: "invalidateUserCache", Value: 1}}).Err() != nil {
		return lifecycleError("lifecycle_database_authorization_cache_unproven")
	}
	current, e = v.readMongoRoles(ctx, p)
	if e != nil || restore && lifecycleDBDigest(current) != lifecycleDBDigest(original) || !restore && len(current) != 0 {
		return lifecycleError("lifecycle_database_account_effect_unknown")
	}
	return nil
}
func (v *lifecycleDBWriterLease) drainMongo(ctx context.Context) error {
	// Killing sessions alone cannot prevent reconnect or kill prepared tx. The
	// roles have actually been removed; a full native read below rejects any tx.
	if e := v.checkMongoTransactions(ctx); e != nil {
		return e
	}
	for _, p := range v.input.MongoPrincipals {
		pattern := bson.D{{Key: "users", Value: bson.A{bson.D{{Key: "user", Value: p.User}, {Key: "db", Value: p.HostOrDatabase}}}}}
		if v.host.owner.originalMongo.Database("admin").RunCommand(ctx, bson.D{{Key: "killAllSessionsByPattern", Value: bson.A{pattern}}}).Err() != nil {
			return lifecycleError("lifecycle_database_session_effect_unknown")
		}
	}
	return v.checkMongoTransactions(ctx)
}
func (v *lifecycleDBWriterLease) checkMongoTransactions(ctx context.Context) error {
	cur, e := v.host.owner.originalMongo.Database("admin").Aggregate(ctx, mongo.Pipeline{bson.D{{Key: "$currentOp", Value: bson.D{{Key: "allUsers", Value: true}, {Key: "idleSessions", Value: true}, {Key: "idleConnections", Value: true}, {Key: "localOps", Value: true}}}}, bson.D{{Key: "$project", Value: bson.D{{Key: "effectiveUsers", Value: 1}, {Key: "transaction", Value: 1}, {Key: "active", Value: 1}, {Key: "connectionId", Value: 1}}}}})
	if e != nil {
		return lifecycleError("lifecycle_database_session_drain_unproven")
	}
	count := 0
	for cur.Next(ctx) {
		count++
		if count > 32768 {
			_ = cur.Close(ctx)
			return lifecycleError("lifecycle_database_session_drain_unproven")
		}
		var op bson.M
		if cur.Decode(&op) != nil {
			_ = cur.Close(ctx)
			return lifecycleError("lifecycle_database_session_schema_unproven")
		}
		if _, has := op["transaction"]; has {
			users, e := lifecycleDBArray(op["effectiveUsers"])
			if e != nil || len(users) == 0 {
				_ = cur.Close(ctx)
				return lifecycleError("lifecycle_database_transaction_unproven")
			}
			for _, user := range users {
				for _, p := range v.input.MongoPrincipals {
					if user["user"] == p.User && user["db"] == p.HostOrDatabase {
						_ = cur.Close(ctx)
						return lifecycleError("lifecycle_database_transaction_unproven")
					}
				}
			}
			// Unknown or prepared state on any other identity is not ignored.
			raw, e := bson.Marshal(op["transaction"])
			var tx struct {
				Prepared           *bool  `bson:"prepared"`
				TimePreparedMicros *int64 `bson:"timePreparedMicros"`
			}
			if e != nil || bson.Unmarshal(raw, &tx) != nil || tx.Prepared != nil && *tx.Prepared || tx.TimePreparedMicros != nil {
				_ = cur.Close(ctx)
				return lifecycleError("lifecycle_database_prepared_transaction_unproven")
			}
		}
	}
	e = cur.Err()
	ce := cur.Close(ctx)
	if e != nil || ce != nil {
		return lifecycleError("lifecycle_database_session_drain_unproven")
	}
	return nil
}

func (v *lifecycleDBWriterLease) check(ctx context.Context, restored bool) error {
	if e := v.nativeIdentityAndAuthentication(ctx); e != nil {
		return e
	}
	c, e := dbcensus.ObserveConnection(ctx, v.host.owner.originalConn, v.host.owner.originalMongo)
	if e != nil && !errors.Is(e, dbcensus.ErrIncomplete) {
		return e
	}
	if e = lifecycleDBRequiredSections(c); e != nil {
		return e
	}
	if e = v.validateWriterCoverage(c, !restored); e != nil {
		return e
	}
	if !restored {
		if e = v.checkSQLPreparedAndDelegated(ctx); e != nil {
			return e
		}
		if e = v.checkMongoTransactions(ctx); e != nil {
			return e
		}
	}
	return nil
}

func (v *lifecycleDBWriterLease) checkSQLPreparedAndDelegated(ctx context.Context) error {
	rows, e := v.host.owner.originalConn.QueryContext(ctx, "XA RECOVER")
	if e != nil {
		return lifecycleError("lifecycle_database_prepared_transaction_unproven")
	}
	present := rows.Next()
	readErr := rows.Err()
	closeErr := rows.Close()
	if present || readErr != nil || closeErr != nil {
		return lifecycleError("lifecycle_database_prepared_transaction_unproven")
	}
	// ACCOUNT LOCK does not stop a scheduled event or a different user's
	// definer trigger/routine. Require actual absence of these four-name
	// dependencies, including definitions hidden by insufficient metadata rights.
	for _, view := range []struct{ table, body string }{{"EVENTS", "EVENT_DEFINITION"}, {"ROUTINES", "ROUTINE_DEFINITION"}, {"TRIGGERS", "ACTION_STATEMENT"}} {
		query := "SELECT COUNT(*) FROM information_schema." + view.table + " WHERE (" + view.body + " IS NULL OR " + view.body + " LIKE ? ESCAPE '=' OR " + view.body + " LIKE ? ESCAPE '=' OR " + view.body + " LIKE ? ESCAPE '=')"
		var count int
		if v.host.owner.originalConn.QueryRowContext(ctx, query, "%domain=_event=_outbox%", "%ai=_bridge=_commands%", "%ai=_messaging=_legacy=_commands%").Scan(&count) != nil || count != 0 {
			return lifecycleError("lifecycle_database_delegated_target_writer_unproven")
		}
	}
	return nil
}
func (h *lifecycleFixedHost) checkDatabaseWriterLease(ctx context.Context, r lifecycleRequest) error {
	if h == nil || h.dbWriters == nil || h.dbWriters.self != h.dbWriters || h.dbWriters.host != h || h.dbWriters.binding != lifecycleWindowBinding(r) || h.dbWriters.actualRunID != r.ActualRunID || !h.dbWriters.installed {
		return lifecycleError("lifecycle_database_writer_lease_missing")
	}
	if h.dbWriters.restored {
		if e := h.databaseWriterTargetsAbsent(ctx); e != nil {
			return e
		}
	}
	return h.dbWriters.check(ctx, h.dbWriters.restored)
}
func (h *lifecycleFixedHost) databaseWriterTargetsAbsent(ctx context.Context) error {
	if h == nil || h.owner == nil || h.owner.originalConn == nil || h.owner.originalDB == nil {
		return lifecycleError("lifecycle_database_forward_restore_unproven")
	}
	for _, t := range targets {
		if t[0] == "mysql" {
			var count int
			if h.owner.originalConn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?", t[1]).Scan(&count) != nil || count != 0 {
				return lifecycleError("lifecycle_database_forward_restore_unproven")
			}
		} else {
			cur, e := h.owner.originalDB.ListCollections(ctx, bson.D{{Key: "name", Value: t[1]}}, options.ListCollections().SetAuthorizedCollections(false))
			if e != nil {
				return e
			}
			present := cur.Next(ctx)
			ce := cur.Err()
			closeErr := cur.Close(ctx)
			if present || ce != nil || closeErr != nil {
				return lifecycleError("lifecycle_database_forward_restore_unproven")
			}
		}
	}
	return nil
}
func (h *lifecycleFixedHost) restoreDatabaseWriterLease(ctx context.Context, r lifecycleRequest, forward bool) error {
	if h == nil || h.dbWriters == nil {
		if forward {
			return lifecycleError("lifecycle_database_writer_lease_missing")
		}
		return nil
	}
	v := h.dbWriters
	if v.self != v || v.host != h || v.binding != lifecycleWindowBinding(r) || v.actualRunID != r.ActualRunID || h.services == nil || v.window != h.services.window {
		return lifecycleError("lifecycle_database_writer_binding_rejected")
	}
	if e := v.nativeIdentityAndAuthentication(ctx); e != nil {
		return e
	}
	if forward {
		if !v.installed || h.preBComparison == nil || h.acceptancePlan == nil || h.acceptancePair == nil {
			return lifecycleError("lifecycle_database_forward_restore_unproven")
		}
		if e := h.databaseWriterTargetsAbsent(ctx); e != nil {
			return e
		}
	}
	// Compare-before/apply/recheck under the original isolated maintenance
	// owner. Native ALTER USER/user-management commands have no SQL WHERE CAS;
	// no permission to overwrite an unexpected state or different conclusion.
	c, e := dbcensus.ObserveConnection(ctx, h.owner.originalConn, h.owner.originalMongo)
	if e != nil && !errors.Is(e, dbcensus.ErrIncomplete) {
		return e
	}
	if e = lifecycleDBRequiredSections(c); e != nil || lifecycleDBStaticHash(c) != v.staticSHA {
		return lifecycleError("lifecycle_database_account_restore_conflict")
	}
	var result error
	for _, p := range v.input.MongoPrincipals {
		if !v.mongoAttempted[lifecycleDBPrincipalKey(p.User, p.HostOrDatabase)] {
			continue
		}
		roles, e := lifecycleDBArray(v.mongoUser(v.baseline, p)["roles"])
		if e != nil {
			result = errors.Join(result, e)
			continue
		}
		result = errors.Join(result, v.setMongoRoles(ctx, p, roles, true))
	}
	for _, p := range v.input.SQLPrincipals {
		if !v.sqlAttempted[lifecycleDBPrincipalKey(p.User, p.HostOrDatabase)] {
			continue
		}
		if val(v.sqlAccount(v.baseline, p), 3) != "N" {
			result = errors.Join(result, lifecycleError("lifecycle_database_account_restore_conflict"))
			continue
		}
		result = errors.Join(result, v.setSQLLock(ctx, p, false))
	}
	if result != nil {
		return result
	}
	if e = v.check(ctx, true); e != nil {
		return e
	}
	v.restored = true
	return nil
}
