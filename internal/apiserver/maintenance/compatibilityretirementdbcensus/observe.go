// Package compatibilityretirementdbcensus reads bounded native account, grant
// and session catalogs through the caller's borrowed connections. It never
// opens/closes a pool, changes authentication, kills a session or mints a fence.
package compatibilityretirementdbcensus

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const dbCensusQueryBudget = 15 * time.Second
const dbCensusRowLimit = 32768
const dbCensusByteLimit = 32 << 20

// These records contain private account/grant/session facts. SQL INFO, Mongo
// command/originatingCommand/clientMetadata, password hashes, credentials and
// environment are never selected or retained. The caller owns both connections.
type Section struct {
	Name                string `json:"name"`
	EnumerationComplete bool   `json:"enumeration_complete"`
	RecheckEqual        bool   `json:"recheck_equal"`
	Items               int    `json:"items"`
	SHA256              string `json:"sha256"`
	ErrorCategory       string `json:"error_category"`
}
type dbCensusSQLQuery func(context.Context, string) ([][]*string, error)
type dbCensusMongoCommand func(context.Context, string, bson.D) (bson.M, error)
type dbCensusMongoAggregate func(context.Context, string, string, mongo.Pipeline) ([]bson.M, error)

func dbCensusSQL(ctx context.Context, db *sql.DB, q string) ([][]*string, error) {
	if ctx == nil || db == nil {
		return nil, errors.New("db_census_borrowed_sql_missing")
	}
	bounded, cancel := context.WithTimeout(ctx, dbCensusQueryBudget)
	defer cancel()
	rows, e := db.QueryContext(bounded, q)
	if e != nil {
		return nil, e
	}
	var result [][]*string
	var byteCount int
	columns, e := rows.Columns()
	if e != nil {
		_ = rows.Close()
		return nil, e
	}
	for rows.Next() {
		if len(result) >= dbCensusRowLimit {
			_ = rows.Close()
			return nil, errors.New("db_census_row_budget_exceeded")
		}
		values := make([]sql.NullString, len(columns))
		refs := make([]any, len(columns))
		for i := range values {
			refs[i] = &values[i]
		}
		if e = rows.Scan(refs...); e != nil {
			_ = rows.Close()
			return nil, e
		}
		record := make([]*string, len(values))
		for i, v := range values {
			if v.Valid {
				value := v.String
				record[i] = &value
				byteCount += len(value)
			}
		}
		if byteCount > dbCensusByteLimit {
			_ = rows.Close()
			return nil, errors.New("db_census_byte_budget_exceeded")
		}
		result = append(result, record)
	}
	e = rows.Err()
	closeErr := rows.Close()
	if e != nil {
		return nil, e
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if bounded.Err() != nil {
		return nil, bounded.Err()
	}
	return result, nil
}
func dbCensusMongoRun(ctx context.Context, c *mongo.Client, db string, command bson.D) (bson.M, error) {
	bounded, cancel := context.WithTimeout(ctx, dbCensusQueryBudget)
	defer cancel()
	var result bson.M
	if c == nil {
		return nil, errors.New("db_census_borrowed_mongo_missing")
	}
	e := c.Database(db).RunCommand(bounded, command).Decode(&result)
	if e != nil {
		return nil, e
	}
	if bounded.Err() != nil {
		return nil, bounded.Err()
	}
	raw, e := bson.Marshal(result)
	if e != nil || len(raw) > dbCensusByteLimit {
		return nil, errors.New("db_census_command_byte_budget_exceeded")
	}
	return result, nil
}
func dbCensusMongoCursor(ctx context.Context, c *mongo.Client, db, collection string, pipeline mongo.Pipeline) ([]bson.M, error) {
	if ctx == nil || c == nil {
		return nil, errors.New("db_census_borrowed_mongo_missing")
	}
	bounded, cancel := context.WithTimeout(ctx, dbCensusQueryBudget)
	defer cancel()
	var cursor *mongo.Cursor
	var e error
	if collection == "" {
		cursor, e = c.Database(db).Aggregate(bounded, pipeline, options.Aggregate().SetBatchSize(1000))
	} else {
		cursor, e = c.Database(db).Collection(collection).Aggregate(bounded, pipeline, options.Aggregate().SetBatchSize(1000))
	}
	if e != nil {
		return nil, e
	}
	var result []bson.M
	var byteCount int
	for cursor.Next(bounded) {
		if len(result) >= dbCensusRowLimit {
			_ = cursor.Close(bounded)
			return nil, errors.New("db_census_row_budget_exceeded")
		}
		byteCount += len(cursor.Current)
		if byteCount > dbCensusByteLimit {
			_ = cursor.Close(bounded)
			return nil, errors.New("db_census_byte_budget_exceeded")
		}
		var entry bson.M
		if cursor.Decode(&entry) != nil {
			_ = cursor.Close(bounded)
			return nil, errors.New("db_census_cursor_schema_rejected")
		}
		result = append(result, entry)
	}
	e = cursor.Err()
	closeErr := cursor.Close(bounded)
	if e != nil {
		return nil, e
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if bounded.Err() != nil {
		return nil, bounded.Err()
	}
	return result, nil
}
func dbCensusLiteral(v string) (string, error) {
	// Quotes are doubled; backslash is rejected rather than depending on the
	// current sql_mode. Never interpolate an account host/name with escape modes.
	if strings.ContainsAny(v, "\\\x00\r\n") || len(v) > 255 {
		return "", errors.New("db_census_account_literal_rejected")
	}
	return "'" + strings.ReplaceAll(v, "'", "''") + "'", nil
}
func dbCensusGlobalProcess(grants [][]*string) bool {
	for _, row := range grants {
		if len(row) != 1 || row[0] == nil {
			continue
		}
		s := strings.ToUpper(*row[0])
		start := strings.Index(s, " ON *.* TO ")
		if !strings.HasPrefix(s, "GRANT ") || start < 0 {
			continue
		}
		p := strings.TrimSpace(s[6:start])
		for _, item := range strings.Split(p, ",") {
			if strings.TrimSpace(item) == "PROCESS" || strings.TrimSpace(item) == "ALL PRIVILEGES" {
				return true
			}
		}
	}
	return false
}
func dbCensusSortSQL(rows [][]*string) {
	sort.Slice(rows, func(i, j int) bool {
		a, _ := json.Marshal(rows[i])
		b, _ := json.Marshal(rows[j])
		return string(a) < string(b)
	})
}
func dbCensusSQLSnapshot(ctx context.Context, q dbCensusSQLQuery) (map[string][][]*string, []Section, []string) {
	out := map[string][][]*string{}
	sections := []Section{}
	gaps := []string{}
	totalBytes := 0
	queries := []struct{ name, query string }{
		{"accounts", "SELECT User,Host,plugin,account_locked FROM mysql.user ORDER BY User,Host"},
		{"role_edges", "SELECT FROM_HOST,FROM_USER,TO_HOST,TO_USER,WITH_ADMIN_OPTION FROM mysql.role_edges ORDER BY FROM_HOST,FROM_USER,TO_HOST,TO_USER"},
		{"default_roles", "SELECT HOST,USER,DEFAULT_ROLE_HOST,DEFAULT_ROLE_USER FROM mysql.default_roles ORDER BY HOST,USER,DEFAULT_ROLE_HOST,DEFAULT_ROLE_USER"},
		{"dynamic_grants", "SELECT USER,HOST,PRIV,WITH_GRANT_OPTION FROM mysql.global_grants ORDER BY USER,HOST,PRIV"},
		{"proxy_grants", "SELECT Host,User,Proxied_host,Proxied_user,With_grant FROM mysql.proxies_priv ORDER BY Host,User,Proxied_host,Proxied_user"},
		{"observer_grants", "SHOW GRANTS FOR CURRENT_USER"},
		// INCLUDE Sleep and background command classes. INFO is deliberately absent.
		{"connections", "SELECT ID,USER,HOST,DB,COMMAND,STATE FROM information_schema.PROCESSLIST ORDER BY ID"},
	}
	for _, entry := range queries {
		rows, e := q(ctx, entry.query)
		if e == nil {
			for _, row := range rows {
				for _, v := range row {
					if v != nil {
						totalBytes += len(*v)
					}
				}
			}
			if totalBytes > 2*dbCensusByteLimit {
				e = errors.New("db_census_total_byte_budget_exceeded")
			}
		}
		s := Section{Name: "mysql_" + entry.name, ErrorCategory: "none"}
		if e != nil {
			s.ErrorCategory = "db_census_sql_query_failed_or_bounded"
			gaps = append(gaps, s.Name+"_unread")
		} else {
			dbCensusSortSQL(rows)
			out[entry.name] = rows
			s.EnumerationComplete = true
			s.Items = len(rows)
			s.SHA256 = checksum(rows)
		}
		sections = append(sections, s)
	}
	accounts, ok := out["accounts"]
	grants := [][]*string{}
	grantSection := Section{Name: "mysql_account_grants", ErrorCategory: "none", EnumerationComplete: ok}
	if ok {
		for _, row := range accounts {
			if !grantSection.EnumerationComplete {
				break
			}
			if len(row) != 4 || row[0] == nil || row[1] == nil {
				grantSection.EnumerationComplete = false
				break
			}
			user, e := dbCensusLiteral(*row[0])
			if e != nil {
				grantSection.EnumerationComplete = false
				break
			}
			host, e := dbCensusLiteral(*row[1])
			if e != nil {
				grantSection.EnumerationComplete = false
				break
			}
			actual, e := q(ctx, "SHOW GRANTS FOR "+user+"@"+host)
			if e != nil || len(actual) == 0 {
				grantSection.EnumerationComplete = false
				break
			}
			for _, g := range actual {
				if len(g) != 1 || g[0] == nil {
					grantSection.EnumerationComplete = false
					break
				}
				u, h, v := *row[0], *row[1], *g[0]
				totalBytes += len(u) + len(h) + len(v)
				if len(grants) >= dbCensusRowLimit || totalBytes > 2*dbCensusByteLimit {
					grantSection.EnumerationComplete = false
					break
				}
				grants = append(grants, []*string{&u, &h, &v})
			}
		}
	}
	if !grantSection.EnumerationComplete {
		grantSection.ErrorCategory = "db_census_sql_account_grants_incomplete"
		gaps = append(gaps, "mysql_account_grants_unread")
	}
	dbCensusSortSQL(grants)
	out["account_grants"] = grants
	grantSection.Items = len(grants)
	grantSection.SHA256 = checksum(grants)
	sections = append(sections, grantSection)
	// Successful filtered PROCESSLIST is not evidence of all sessions. Effective
	// role-only PROCESS is conservatively unknown until independently expanded.
	if !dbCensusGlobalProcess(out["observer_grants"]) {
		gaps = append(gaps, "mysql_full_process_privilege_unproven")
		for i := range sections {
			if sections[i].Name == "mysql_connections" {
				sections[i].EnumerationComplete = false
				sections[i].ErrorCategory = "db_census_sql_full_process_permission_unproven"
			}
		}
	}
	gaps = append(gaps, "mysql_external_authentication_and_direct_writer_admission_not_fenced")
	return out, sections, gaps
}

func dbCensusArray(value any) ([]bson.M, error) {
	a, ok := value.(bson.A)
	if !ok {
		if other, yes := value.([]any); yes {
			a = bson.A(other)
		} else {
			return nil, errors.New("db_census_mongo_array_rejected")
		}
	}
	if len(a) > dbCensusRowLimit {
		return nil, errors.New("db_census_row_budget_exceeded")
	}
	result := make([]bson.M, 0, len(a))
	for _, v := range a {
		m, ok := v.(bson.M)
		if !ok {
			return nil, errors.New("db_census_mongo_record_rejected")
		}
		result = append(result, m)
	}
	return result, nil
}
func dbCensusProject(records []bson.M, keys []string, required []string) ([]bson.M, error) {
	result := make([]bson.M, 0, len(records))
	for _, row := range records {
		copy := bson.M{}
		for _, key := range keys {
			if v, ok := row[key]; ok {
				copy[key] = v
			}
		}
		for _, key := range required {
			if _, ok := copy[key]; !ok {
				return nil, errors.New("db_census_mongo_projection_rejected")
			}
		}
		result = append(result, copy)
	}
	sort.Slice(result, func(i, j int) bool {
		a, _ := json.Marshal(result[i])
		b, _ := json.Marshal(result[j])
		return string(a) < string(b)
	})
	return result, nil
}
func dbCensusClusterAction(reply bson.M, action string) bool {
	auth, ok := reply["authInfo"].(bson.M)
	if !ok {
		return false
	}
	privileges, e := dbCensusArray(auth["authenticatedUserPrivileges"])
	if e != nil {
		return false
	}
	for _, p := range privileges {
		resource, ok := p["resource"].(bson.M)
		if !ok || resource["cluster"] != true {
			continue
		}
		actions, ok := p["actions"].(bson.A)
		if !ok {
			continue
		}
		for _, a := range actions {
			if a == action {
				return true
			}
		}
	}
	return false
}
func dbCensusSessionPipeline(stage string) mongo.Pipeline {
	value := bson.D{{Key: "allUsers", Value: true}}
	if stage == "$currentOp" {
		value = append(value, bson.E{Key: "idleConnections", Value: true}, bson.E{Key: "idleSessions", Value: true}, bson.E{Key: "idleCursors", Value: true}, bson.E{Key: "localOps", Value: true})
	}
	// Inclusion projection prevents credential-bearing command/client metadata
	// from leaving the Mongo server. Session identity is kept private by caller.
	projection := bson.D{{Key: "_id", Value: 1}, {Key: "opid", Value: 1}, {Key: "host", Value: 1}, {Key: "client", Value: 1}, {Key: "connectionId", Value: 1}, {Key: "active", Value: 1}, {Key: "type", Value: 1}, {Key: "op", Value: 1}, {Key: "ns", Value: 1}, {Key: "lsid", Value: 1}, {Key: "effectiveUsers", Value: 1}, {Key: "user", Value: 1}, {Key: "users", Value: 1}, {Key: "lastUse", Value: 1}}
	return mongo.Pipeline{bson.D{{Key: stage, Value: value}}, bson.D{{Key: "$project", Value: projection}}}
}
func dbCensusMongoSnapshot(ctx context.Context, run dbCensusMongoCommand, aggregate dbCensusMongoAggregate) (map[string][]bson.M, []Section, []string) {
	out := map[string][]bson.M{}
	totalBytes := 0
	sections := []Section{}
	gaps := []string{}
	add := func(name string, rows []bson.M, e error) {
		s := Section{Name: "mongodb_" + name, ErrorCategory: "none"}
		if e == nil {
			raw, marshalErr := json.Marshal(rows)
			totalBytes += len(raw)
			if marshalErr != nil || totalBytes > 2*dbCensusByteLimit {
				e = errors.New("db_census_total_byte_budget_exceeded")
			}
		}
		if e != nil {
			s.ErrorCategory = "db_census_mongo_query_failed_or_bounded"
			gaps = append(gaps, s.Name+"_unread")
		} else {
			out[name] = rows
			s.Items = len(rows)
			s.SHA256 = checksum(rows)
			s.EnumerationComplete = true
		}
		sections = append(sections, s)
	}
	status, statusErr := run(ctx, "admin", bson.D{{Key: "connectionStatus", Value: 1}, {Key: "showPrivileges", Value: true}})
	var privilege []bson.M
	if statusErr == nil {
		auth, ok := status["authInfo"].(bson.M)
		if !ok {
			statusErr = errors.New("db_census_mongo_privilege_schema_rejected")
		} else {
			privilege, e := dbCensusArray(auth["authenticatedUserPrivileges"])
			if e != nil {
				statusErr = e
			} else {
				out["observer_privileges"] = privilege
			}
		}
	}
	if statusErr == nil {
		privilege = out["observer_privileges"]
	}
	add("observer_privileges", privilege, statusErr)
	identities, rows, e := dbCensusMongoUsers(ctx, run)
	add("users", rows, e)
	databases, e := run(ctx, "admin", bson.D{{Key: "listDatabases", Value: 1}, {Key: "nameOnly", Value: true}, {Key: "authorizedDatabases", Value: false}})
	var dbs []bson.M
	if e == nil {
		dbs, e = dbCensusArray(databases["databases"])
	}
	if e == nil {
		dbs, e = dbCensusProject(dbs, []string{"name"}, []string{"name"})
	}
	add("databases", dbs, e)
	// Roles can belong to an otherwise absent logical database. Read the actual
	// full native role-definition collection, then expand every exact database
	// represented by stored definitions/users in addition to listDatabases.
	roleProjection := bson.D{{Key: "_id", Value: 1}, {Key: "role", Value: 1}, {Key: "db", Value: 1}, {Key: "roles", Value: 1}, {Key: "privileges", Value: 1}, {Key: "authenticationRestrictions", Value: 1}}
	definitions, definitionErr := aggregate(ctx, "admin", "system.roles", mongo.Pipeline{bson.D{{Key: "$project", Value: roleProjection}}})
	if definitionErr == nil {
		definitions, definitionErr = dbCensusProject(definitions, []string{"_id", "role", "db", "roles", "privileges", "authenticationRestrictions"}, []string{"role", "db"})
	}
	add("stored_role_definitions", definitions, definitionErr)
	databaseNames := map[string]bool{}
	for _, d := range dbs {
		if name, ok := d["name"].(string); ok && name != "" {
			databaseNames[name] = true
		}
	}
	for _, record := range append(append([]bson.M{}, identities...), definitions...) {
		if name, ok := record["db"].(string); ok && name != "" {
			databaseNames[name] = true
		}
		if nested, err := dbCensusArray(record["roles"]); err == nil {
			for _, role := range nested {
				if name, ok := role["db"].(string); ok && name != "" {
					databaseNames[name] = true
				}
			}
		}
	}
	orderedDBs := []string{}
	for name := range databaseNames {
		orderedDBs = append(orderedDBs, name)
	}
	sort.Strings(orderedDBs)
	roles := []bson.M{}
	roleErr := e
	if definitionErr != nil {
		roleErr = definitionErr
	}
	if roleErr == nil {
		for _, name := range orderedDBs {
			reply, e := run(ctx, name, bson.D{{Key: "rolesInfo", Value: 1}, {Key: "showBuiltinRoles", Value: true}, {Key: "showPrivileges", Value: true}, {Key: "showAuthenticationRestrictions", Value: true}})
			var actual []bson.M
			if e == nil {
				actual, e = dbCensusArray(reply["roles"])
			}
			if e == nil {
				actual, e = dbCensusProject(actual, []string{"role", "db", "roles", "inheritedRoles", "privileges", "inheritedPrivileges", "authenticationRestrictions", "inheritedAuthenticationRestrictions", "isBuiltin"}, []string{"role", "db"})
			}
			if e != nil {
				roleErr = e
				break
			}
			if len(roles)+len(actual) > dbCensusRowLimit {
				roleErr = errors.New("db_census_row_budget_exceeded")
				break
			}
			raw, marshalErr := json.Marshal(actual)
			if marshalErr != nil || len(raw) > dbCensusByteLimit {
				roleErr = errors.New("db_census_byte_budget_exceeded")
				break
			}
			roles = append(roles, actual...)
		}
	}
	add("roles", roles, roleErr)
	// getCmdLineOpts may return credential-bearing argv/LDAP startup fields.
	// Do not request it. The selected safe native parameters below cannot prove
	// loaded startup authorization or external authentication provider settings.
	add("authentication_configuration", nil, errors.New("db_census_startup_authentication_configuration_unread"))
	gaps = append(gaps, "mongodb_startup_authorization_and_external_provider_configuration_unobserved")
	params, parameterErr := run(ctx, "admin", bson.D{{Key: "getParameter", Value: 1}, {Key: "authenticationMechanisms", Value: 1}, {Key: "enableLocalhostAuthBypass", Value: 1}})
	var authParams []bson.M
	if parameterErr == nil {
		authParams, parameterErr = dbCensusProject([]bson.M{params}, []string{"authenticationMechanisms", "enableLocalhostAuthBypass"}, []string{"authenticationMechanisms", "enableLocalhostAuthBypass"})
	}
	add("authentication_parameters", authParams, parameterErr)
	for _, item := range []struct{ name, stage, db, collection, privilege string }{{"connections_and_idle_operations", "$currentOp", "admin", "", "inprog"}, {"local_logical_sessions", "$listLocalSessions", "admin", "", "listSessions"}, {"persisted_logical_sessions", "$listSessions", "config", "system.sessions", "listSessions"}} {
		rows, e := aggregate(ctx, item.db, item.collection, dbCensusSessionPipeline(item.stage))
		if e == nil {
			rows, e = dbCensusProject(rows, []string{"_id", "opid", "host", "client", "connectionId", "active", "type", "op", "ns", "lsid", "effectiveUsers", "user", "users", "lastUse"}, nil)
		}
		if !dbCensusClusterAction(status, item.privilege) {
			e = errors.New("db_census_mongo_all_users_privilege_unproven")
		}
		add(item.name, rows, e)
	}
	gaps = append(gaps, "mongodb_other_nodes_sessions_and_external_authentication_unobserved", "mongodb_external_direct_writer_admission_not_fenced")
	return out, sections, gaps
}
func dbCensusFinishSections(first, second []Section) []Section {
	out := append([]Section(nil), first...)
	for i := range out {
		out[i].RecheckEqual = false
		found := false
		for _, later := range second {
			if later.Name == out[i].Name {
				found = true
				out[i].RecheckEqual = out[i].EnumerationComplete && later.EnumerationComplete && out[i].SHA256 == later.SHA256 && out[i].Items == later.Items
				out[i].EnumerationComplete = out[i].EnumerationComplete && later.EnumerationComplete
				if later.ErrorCategory != "none" {
					out[i].ErrorCategory = later.ErrorCategory
				}
				break
			}
		}
		if !found {
			out[i].EnumerationComplete = false
			out[i].ErrorCategory = "db_census_second_enumeration_missing"
		}
	}
	return out
}

var ErrIncomplete = errors.New("db_census_catalog_or_session_permissions_incomplete")

const Budget = 120 * time.Second

// Catalog contains private account and grant facts. A host must store it only
// in its bound private batch and emit digests/counts, never this body publicly.
// Source/database identity validation remains with the caller's original host
// adapter; these records contain no imported identity or authority assertions.
type Catalog struct {
	SQL      map[string][][]*string `json:"mysql"`
	Mongo    map[string][]bson.M    `json:"mongodb"`
	Sections []Section              `json:"sections"`
	Gaps     []string               `json:"unknown"`
}

func checksum(v any) string {
	b, e := json.Marshal(v)
	if e != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (c Catalog) MembershipSHA256() string {
	return checksum(struct {
		SQL   map[string][][]*string
		Mongo map[string][]bson.M
	}{c.SQL, c.Mongo})
}
func (c Catalog) CompleteSection(name string) bool {
	for _, s := range c.Sections {
		if s.Name == name {
			return s.EnumerationComplete
		}
	}
	return false
}
func Observe(ctx context.Context, borrowedSQL *sql.DB, borrowedMongo *mongo.Client) (Catalog, error) {
	if ctx == nil || ctx.Err() != nil || borrowedSQL == nil || borrowedMongo == nil {
		return Catalog{}, errors.New("db_census_borrowed_handles_missing")
	}
	bounded, cancel := context.WithTimeout(ctx, Budget)
	defer cancel()
	q := func(ctx context.Context, query string) ([][]*string, error) {
		return dbCensusSQL(ctx, borrowedSQL, query)
	}
	run := func(ctx context.Context, db string, command bson.D) (bson.M, error) {
		return dbCensusMongoRun(ctx, borrowedMongo, db, command)
	}
	aggregate := func(ctx context.Context, db, col string, pipeline mongo.Pipeline) ([]bson.M, error) {
		return dbCensusMongoCursor(ctx, borrowedMongo, db, col, pipeline)
	}
	sqlFirst, sqlSections, sqlGaps := dbCensusSQLSnapshot(bounded, q)
	mongoFirst, mongoSections, mongoGaps := dbCensusMongoSnapshot(bounded, run, aggregate)
	_, sqlSecond, sqlGaps2 := dbCensusSQLSnapshot(bounded, q)
	_, mongoSecond, mongoGaps2 := dbCensusMongoSnapshot(bounded, run, aggregate)
	out := Catalog{SQL: sqlFirst, Mongo: mongoFirst, Sections: append(dbCensusFinishSections(sqlSections, sqlSecond), dbCensusFinishSections(mongoSections, mongoSecond)...), Gaps: []string{}}
	seen := map[string]bool{}
	for _, list := range [][]string{sqlGaps, mongoGaps, sqlGaps2, mongoGaps2} {
		for _, gap := range list {
			if !seen[gap] {
				seen[gap] = true
				out.Gaps = append(out.Gaps, gap)
			}
		}
	}
	sort.Strings(out.Gaps)
	if bounded.Err() != nil {
		return out, ErrIncomplete
	}
	for _, section := range out.Sections {
		if !section.EnumerationComplete {
			return out, ErrIncomplete
		}
	}
	return out, nil
}
