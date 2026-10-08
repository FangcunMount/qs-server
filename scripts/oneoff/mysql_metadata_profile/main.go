package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

const (
	totalLimit     = 2 * time.Minute
	queryLimit     = 15 * time.Second
	identitySQL    = "SELECT CONNECTION_ID(), @@server_uuid, DATABASE(), VERSION(), CURRENT_ROLE()"
	currentSQL     = "SHOW GRANTS FOR CURRENT_USER"
	rdsSQL         = "SHOW GRANTS FOR CURRENT_USER USING `rds_superuser_role`@`%`"
	mandatorySQL   = "SELECT @@GLOBAL.mandatory_roles"
	roleNotGranted = 3530 // ER_ROLE_NOT_GRANTED, MySQL 8.0 official error reference.
)

var (
	shaRE            = regexp.MustCompile(`^[0-9a-f]{40}$`)
	hashRE           = regexp.MustCompile(`^[0-9a-f]{64}$`)
	runRE            = regexp.MustCompile(`^[0-9]{1,20}-[0-9]{1,4}$`)
	uuidRE           = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	identifier       = "`(?:[^`\\r\\n]|``){0,255}`"
	account          = identifier + "@" + identifier
	scope            = "(?:\\*\\.\\*|" + identifier + "\\.(?:\\*|" + identifier + "))"
	privilegeGrant   = regexp.MustCompile("^GRANT (.+) ON (" + scope + ") TO (" + account + ")(?: WITH GRANT OPTION)?$")
	privilegeRevoke  = regexp.MustCompile("^REVOKE (.+) ON (" + scope + ") FROM (" + account + ")$")
	roleGrant        = regexp.MustCompile("^GRANT " + account + "(?:," + account + ")* TO " + account + "(?: WITH ADMIN OPTION)?$")
	proxyGrant       = regexp.MustCompile("^GRANT PROXY ON " + account + " TO " + account + "(?: WITH GRANT OPTION)?$")
	dynamicPrivilege = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
)

type binding struct{ SHA, RunID, Expected string }
type connection struct {
	Host, User, Password, Database string
	Port                           int
}
type identity struct {
	ID                             uint64
	UUID, Database, Version, Roles string
}
type receipt struct {
	FormatVersion       int    `json:"format_version"`
	SourceSHA           string `json:"source_sha,omitempty"`
	RunID               string `json:"run_id,omitempty"`
	ExpectedTargetHash  string `json:"expected_target_hash,omitempty"`
	SourceTargetHash    string `json:"source_target_hash,omitempty"`
	CurrentUnrestricted bool   `json:"current_unrestricted_metadata_grants"`
	RDSAvailable        *bool  `json:"rds_role_grants_available"`
	RDSUnrestricted     *bool  `json:"rds_role_unrestricted_metadata_grants"`
	AssignedRoles       *bool  `json:"assigned_roles_present"`
	MandatoryRoles      *bool  `json:"mandatory_roles_present"`
	DiagnosticOnly      bool   `json:"diagnostic_only"`
	Complete            bool   `json:"complete"`
	ErrorCategory       string `json:"error_category"`
}

func baseReceipt(b binding) receipt {
	r := receipt{FormatVersion: 1, DiagnosticOnly: true, ErrorCategory: "input_invalid"}
	if shaRE.MatchString(b.SHA) {
		r.SourceSHA = b.SHA
	}
	if runRE.MatchString(b.RunID) {
		r.RunID = b.RunID
	}
	if hashRE.MatchString(b.Expected) {
		r.ExpectedTargetHash = b.Expected
	}
	return r
}
func validBinding(b binding) bool {
	return shaRE.MatchString(b.SHA) && runRE.MatchString(b.RunID) && hashRE.MatchString(b.Expected)
}
func hashText(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = h.Write([]byte{1})
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len([]byte(p))))
		_, _ = h.Write(n[:])
		_, _ = h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}
func readIdentity(ctx context.Context, c *sql.Conn) (identity, error) {
	ctx, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	var v identity
	err := c.QueryRowContext(ctx, identitySQL).Scan(&v.ID, &v.UUID, &v.Database, &v.Version, &v.Roles)
	if err != nil {
		return identity{}, errors.New("identity_read_failed")
	}
	if v.ID == 0 || !uuidRE.MatchString(v.UUID) || len(v.Database) == 0 || len(v.Database) > 255 || len(v.Version) == 0 || len(v.Version) > 128 || len(v.Roles) == 0 || len(v.Roles) > 65536 {
		return identity{}, errors.New("identity_invalid")
	}
	return v, nil
}
func readGrants(ctx context.Context, c *sql.Conn, query string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	rows, err := c.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	grants := make([]string, 0, 16)
	bytes := 0
	for rows.Next() {
		var s string
		if err = rows.Scan(&s); err != nil {
			return nil, err
		}
		bytes += len(s)
		if len(grants) >= 100 || len(s) > 16384 || bytes > 1024*1024 {
			return nil, errors.New("grant_output_limit")
		}
		grants = append(grants, s)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(grants) == 0 {
		return nil, errors.New("grant_output_empty")
	}
	return grants, nil
}

// Only presence is retained; mandatory role names are never exposed or used to
// construct another query. Unknown/null or oversized values fail closed.
func readMandatoryRoles(ctx context.Context, c *sql.Conn) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	var roles string
	if err := c.QueryRowContext(ctx, mandatorySQL).Scan(&roles); err != nil || len(roles) > 65536 {
		return false, errors.New("mandatory_roles_query_failed")
	}
	return roles != "", nil
}

// Recognise static privilege and global dynamic-token syntax, but only the
// exact four static privileges (or global ALL PRIVILEGES) qualify this boolean.
// Unknown statements and malformed rows invalidate all otherwise positive rows.
func unrestricted(grants []string) (bool, bool) {
	known := map[string]bool{}
	for _, s := range strings.Split("USAGE|ALL PRIVILEGES|SELECT|INSERT|UPDATE|DELETE|CREATE|DROP|RELOAD|SHUTDOWN|PROCESS|FILE|REFERENCES|INDEX|ALTER|SHOW DATABASES|SUPER|CREATE TEMPORARY TABLES|LOCK TABLES|EXECUTE|REPLICATION SLAVE|REPLICATION CLIENT|CREATE VIEW|SHOW VIEW|CREATE ROUTINE|ALTER ROUTINE|CREATE USER|EVENT|TRIGGER|CREATE TABLESPACE|CREATE ROLE|DROP ROLE|GRANT OPTION", "|") {
		known[s] = true
	}
	global := map[string]bool{}
	restricted := false
	for _, s := range grants {
		if len(s) > 16384 || strings.ContainsAny(s, "\r\n\x00") {
			return false, false
		}
		if strings.HasPrefix(s, "REVOKE ") {
			m := privilegeRevoke.FindStringSubmatch(s)
			if m == nil {
				return false, false
			}
			for _, p := range strings.Split(m[1], ",") {
				p = strings.TrimSpace(p)
				if !known[p] && (m[2] != "*.*" || !dynamicPrivilege.MatchString(p)) {
					return false, false
				}
			}
			restricted = true
			continue
		}
		if roleGrant.MatchString(s) || proxyGrant.MatchString(s) {
			continue
		}
		m := privilegeGrant.FindStringSubmatch(s)
		if m == nil {
			return false, false
		}
		for _, p := range strings.Split(m[1], ",") {
			p = strings.TrimSpace(p)
			if !known[p] && (m[2] != "*.*" || !dynamicPrivilege.MatchString(p)) {
				return false, false
			}
			if m[2] == "*.*" {
				global[p] = true
				if p == "ALL PRIVILEGES" {
					for _, q := range []string{"SELECT", "SHOW VIEW", "TRIGGER", "EVENT"} {
						global[q] = true
					}
				}
			}
		}
	}
	return !restricted && global["SELECT"] && global["SHOW VIEW"] && global["TRIGGER"] && global["EVENT"], true
}
func probe(ctx context.Context, db *sql.DB, cfg connection, b binding) receipt {
	r := baseReceipt(b)
	if !validBinding(b) {
		return r
	}
	ctx, cancel := context.WithTimeout(ctx, totalLimit)
	defer cancel()
	c, err := db.Conn(ctx)
	if err != nil {
		r.ErrorCategory = "connection_failed"
		return r
	}
	defer func() { _ = c.Close() }()
	before, err := readIdentity(ctx, c)
	if err != nil {
		r.ErrorCategory = "identity_read_failed"
		return r
	}
	if before.Database != cfg.Database {
		r.ErrorCategory = "target_identity_mismatch"
		return r
	}
	actual := hashText(cfg.Host, strconv.Itoa(cfg.Port), cfg.Database, before.UUID)
	if actual != b.Expected {
		r.ErrorCategory = "target_hash_mismatch"
		return r
	}
	r.SourceTargetHash = actual
	currentRows, err := readGrants(ctx, c, currentSQL)
	category := "none"
	current := false
	if err != nil {
		category = "current_grants_query_failed"
	} else {
		var known bool
		current, known = unrestricted(currentRows)
		if !known {
			category = "current_grants_rejected"
		}
	}
	var assigned, mandatory bool
	if category == "none" {
		// currentRows have already passed the complete canonical syntax check.
		// A role grant establishes presence only, never effective privileges.
		for _, row := range currentRows {
			assigned = assigned || roleGrant.MatchString(row)
		}
		mandatory, err = readMandatoryRoles(ctx, c)
		if err != nil {
			category = "mandatory_roles_query_failed"
		}
	}
	var available, potential *bool
	if category == "none" {
		rows, e := readGrants(ctx, c, rdsSQL)
		if e != nil {
			var me *mysql.MySQLError
			if errors.As(e, &me) && me.Number == roleNotGranted && me.SQLState == [5]byte{'H', 'Y', '0', '0', '0'} {
				v := false
				available = &v
			} else {
				category = "rds_role_query_failed"
			}
		} else {
			v := true
			available = &v
			p, known := unrestricted(rows)
			potential = &p
			if !known {
				category = "rds_role_grants_rejected"
			}
		}
	}
	// No positive output survives an unverified/lost or role-changed pinned session.
	after, e := readIdentity(ctx, c)
	if e != nil {
		r.ErrorCategory = "identity_final_failed"
		return r
	}
	if before != after {
		r.ErrorCategory = "session_identity_changed"
		return r
	}
	if category != "none" {
		r.ErrorCategory = category
		return r
	}
	r.CurrentUnrestricted = current
	r.RDSAvailable = available
	r.RDSUnrestricted = potential
	r.AssignedRoles = &assigned
	r.MandatoryRoles = &mandatory
	r.Complete = true
	r.ErrorCategory = "none"
	return r
}
func envConnection(get func(string) string) (connection, bool) {
	port := get("MYSQL_PORT")
	p, err := strconv.Atoi(port)
	c := connection{Host: get("MYSQL_HOST"), Port: p, User: get("MYSQL_USERNAME"), Password: get("MYSQL_PASSWORD"), Database: get("MYSQL_DATABASE")}
	if err != nil || p < 1 || p > 65535 || len(port) > 5 {
		return connection{}, false
	}
	for _, v := range []string{c.Host, c.User, c.Password, c.Database} {
		if len(v) == 0 || len(v) > 4096 || strings.ContainsRune(v, 0) {
			return connection{}, false
		}
	}
	if strings.ContainsAny(c.Host, "\r\n\t ") || len(c.Database) > 255 {
		return connection{}, false
	}
	return c, true
}
func mysqlConfig(c connection) *mysql.Config {
	x := mysql.NewConfig()
	x.User = c.User
	x.Passwd = c.Password
	x.Net = "tcp"
	x.Addr = net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	x.DBName = c.Database
	x.Timeout = 5 * time.Second
	x.ReadTimeout = queryLimit
	x.WriteTimeout = queryLimit
	x.MultiStatements = false
	x.InterpolateParams = false
	x.Logger = &mysql.NopLogger{}
	return x
}
func run(args []string, get func(string) string, out io.Writer) int {
	_ = mysql.SetLogger(&mysql.NopLogger{})
	b := binding{SHA: get("PROFILE_SOURCE_SHA"), RunID: get("PROFILE_RUN_ID"), Expected: get("PROFILE_EXPECTED_TARGET_HASH")}
	r := baseReceipt(b)
	if len(args) == 0 && validBinding(b) {
		r = baseReceipt(b)
		if c, ok := envConnection(get); ok {
			connector, err := mysql.NewConnector(mysqlConfig(c))
			if err != nil {
				r.ErrorCategory = "connection_config_failed"
			} else {
				db := sql.OpenDB(connector)
				db.SetMaxOpenConns(1)
				db.SetMaxIdleConns(0)
				ctx, cancel := context.WithTimeout(context.Background(), totalLimit)
				r = probe(ctx, db, c, b)
				cancel()
				_ = db.Close()
			}
		} else {
			r.ErrorCategory = "connection_input_invalid"
		}
	}
	if err := json.NewEncoder(out).Encode(r); err != nil {
		return 1
	}
	if r.Complete {
		return 0
	}
	return 1
}
func main() { os.Exit(run(os.Args[1:], os.Getenv, os.Stdout)) }
