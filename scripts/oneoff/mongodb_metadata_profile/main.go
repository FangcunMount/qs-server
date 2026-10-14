// Command mongodb_metadata_profile reads one fixed Mongo target over an owned
// loopback tunnel. Its raw receipt is private to the Python transport; it never
// attests backup privileges, a backup, a maintenance freeze or business readiness.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

const expectedTarget = "ab3c60ccc9106a110d85946468f0cec60a18614e54fde49dadfa8701a362036f"
const maxResponse = 2 << 20

var shaRE = regexp.MustCompile(`^[0-9a-f]{40}$`)
var hashRE = regexp.MustCompile(`^[0-9a-f]{64}$`)
var runRE = regexp.MustCompile(`^[0-9]{1,20}-[0-9]{1,4}$`)
var fcvRE = regexp.MustCompile(`^([0-9]{1,3})\.([0-9]{1,3})$`)

type input struct {
	SourceSHA     string `json:"source_sha"`
	RunID         string `json:"run_id"`
	Expected      string `json:"expected_target_hash"`
	SourceBinding string `json:"source_binding_hash"`
	ToolHash      string `json:"tool_sha256"`
	Port          int    `json:"port"`
	Account       string `json:"username"`
	Password      string `json:"password"`
	Database      string `json:"database"`
}
type targetCheck struct {
	Namespace        bool   `json:"namespace_matches_expected"`
	Replica          bool   `json:"replica_set_matches_expected"`
	Members          bool   `json:"advertised_members_match_expected"`
	Catalog          bool   `json:"catalog_fingerprint_matches_expected"`
	Unchanged        bool   `json:"identity_unchanged"`
	ProcessUnchanged bool   `json:"process_identity_unchanged"`
	Primary          bool   `json:"writable_primary"`
	Head             uint64 `json:"head_version"`
	Dirty            bool   `json:"dirty"`
	RetiredAbsent    bool   `json:"retired_namespace_absent"`
}
type authorization struct {
	Authenticated bool  `json:"authenticated_as_requested"`
	AuthDB        bool  `json:"authentication_database_matches_expected"`
	Observed      bool  `json:"privileges_observed"`
	Parameter     *bool `json:"get_parameter_hint"`
	Catalog       *bool `json:"catalog_metadata_hint"`
	Migration     *bool `json:"migration_read_hint"`
}
type build struct {
	Known   bool   `json:"known"`
	Major   uint64 `json:"major"`
	Minor   uint64 `json:"minor"`
	Patch   uint64 `json:"patch"`
	Matches bool   `json:"matches_expected"`
}
type fcv struct {
	Known              bool   `json:"known"`
	Major              uint64 `json:"major"`
	Minor              uint64 `json:"minor"`
	Target             bool   `json:"target_present"`
	TargetMajor        uint64 `json:"target_major"`
	TargetMinor        uint64 `json:"target_minor"`
	Previous           bool   `json:"previous_present"`
	PreviousMajor      uint64 `json:"previous_major"`
	PreviousMinor      uint64 `json:"previous_minor"`
	Cleanup            bool   `json:"cleanup_present"`
	CleanupValue       bool   `json:"cleanup_value"`
	Timestamp          bool   `json:"change_timestamp_present"`
	TimestampSeconds   uint64 `json:"change_timestamp_seconds"`
	TimestampIncrement uint64 `json:"change_timestamp_increment"`
	Transition         bool   `json:"transition_observed"`
}
type release struct {
	Cursors       bool `json:"cursors_closed"`
	Client        bool `json:"client_disconnected"`
	Tunnel        bool `json:"tunnel_terminated"`
	Waited        bool `json:"tunnel_waited"`
	Key           bool `json:"key_unlinked"`
	Hosts         bool `json:"known_hosts_unlinked"`
	Directory     bool `json:"temp_directory_released"`
	TunnelProcess bool `json:"owned_tunnel_process_released"`
	ReaderProcess bool `json:"reader_process_released"`
}
type budget struct {
	Elapsed uint64 `json:"elapsed_milliseconds"`
	Queries uint64 `json:"logical_queries"`
}
type ttlMonitor struct {
	Known   bool `json:"known"`
	Enabled bool `json:"enabled"`
}
type receipt struct {
	Protocol      string        `json:"protocol"`
	Operation     string        `json:"operation"`
	Database      string        `json:"database"`
	SourceSHA     string        `json:"source_sha"`
	RunID         string        `json:"run_id"`
	ToolHash      string        `json:"tool_sha256"`
	SourceBinding string        `json:"source_binding_hash"`
	Expected      string        `json:"expected_target_hash"`
	Target        string        `json:"target_hash"`
	Observation   bool          `json:"fcv_observation_complete"`
	Complete      bool          `json:"complete"`
	Eligible      bool          `json:"eligible_fcv_readiness"`
	Backup        bool          `json:"backup_restore_proven"`
	Business      bool          `json:"business_acceptance_proven"`
	Error         string        `json:"error_category"`
	TargetCheck   targetCheck   `json:"target_check"`
	Authorization authorization `json:"authorization"`
	Build         build         `json:"server_build"`
	FCV           fcv           `json:"feature_compatibility"`
	TTL           ttlMonitor    `json:"ttl_monitor"`
	Release       release       `json:"release"`
	Budget        budget        `json:"budget"`
}
type commander interface {
	Command(context.Context, string, bson.D) (bson.Raw, error)
	Close(context.Context) error
}
type mongoCommander struct{ client *mongo.Client }

func (m *mongoCommander) Command(ctx context.Context, db string, d bson.D) (bson.Raw, error) {
	var raw bson.Raw
	err := m.client.Database(db).RunCommand(ctx, d).Decode(&raw)
	return raw, err
}
func (m *mongoCommander) Close(ctx context.Context) error { return m.client.Disconnect(ctx) }

func connect(ctx context.Context, in input) (commander, error) {
	// No URI string or inherited production environment enters the driver.
	o := options.Client().SetHosts([]string{"127.0.0.1:" + strconv.Itoa(in.Port)}).
		SetAuth(options.Credential{AuthSource: "admin", Username: in.Account, Password: in.Password}).
		SetDirect(true).SetReplicaSet("rs0").SetReadPreference(readpref.Primary()).
		SetMaxPoolSize(1).SetMinPoolSize(0).SetRetryReads(false).
		SetConnectTimeout(5 * time.Second).SetServerSelectionTimeout(5 * time.Second).
		SetSocketTimeout(5 * time.Second).SetHeartbeatInterval(60 * time.Second).
		SetAppName("qs-mongo-metadata-profile")
	c, err := mongo.Connect(ctx, o)
	if err != nil {
		return nil, errors.New("tool_failed")
	}
	return &mongoCommander{client: c}, nil
}

func decodeInput(rd io.Reader) (input, error) {
	var in input
	raw, err := io.ReadAll(io.LimitReader(rd, 16385))
	if err != nil || len(raw) > 16384 {
		return in, errors.New("input_invalid")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	t, err := d.Token()
	if err != nil || t != json.Delim('{') {
		return in, errors.New("input_invalid")
	}
	seen := map[string]bool{}
	for d.More() {
		t, err = d.Token()
		k, ok := t.(string)
		if err != nil || !ok || seen[k] {
			return in, errors.New("input_invalid")
		}
		seen[k] = true
		var v json.RawMessage
		if d.Decode(&v) != nil {
			return in, errors.New("input_invalid")
		}
	}
	if _, err = d.Token(); err != nil {
		return in, errors.New("input_invalid")
	}
	if _, err = d.Token(); err != io.EOF {
		return in, errors.New("input_invalid")
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || len(seen) != 9 || !shaRE.MatchString(in.SourceSHA) || !runRE.MatchString(in.RunID) || !hashRE.MatchString(in.SourceBinding) || !hashRE.MatchString(in.ToolHash) || in.Expected != expectedTarget || in.Database != "qs" || in.Port < 1024 || in.Port > 65535 || in.Port == 27017 || len(in.Account) < 1 || len(in.Account) > 256 || strings.ContainsAny(in.Account, "\x00\r\n") || len(in.Password) < 1 || len(in.Password) > 4096 {
		return in, errors.New("input_invalid")
	}
	return in, nil
}

type reader struct {
	c              commander
	count          uint64
	cursorReleased bool
}

func (r *reader) command(ctx context.Context, db string, d bson.D) (bson.Raw, error) {
	if r.count >= 10 {
		return nil, errors.New("reading_budget_exceeded")
	}
	r.count++
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := r.c.Command(c, db, d)
	if err == nil && (len(raw) > maxResponse || raw.Validate() != nil) {
		return nil, errors.New("receipt_invalid")
	}
	return raw, err
}

// RunCommand leaves no managed driver cursor. A surprising live server cursor is
// killed explicitly under a separate release budget and cannot become success.
func (r *reader) batch(ctx context.Context, raw bson.Raw, collection string) ([]bson.Raw, error) {
	r.cursorReleased = false
	cur, ok := raw.Lookup("cursor").DocumentOK()
	if !ok {
		return nil, errors.New("receipt_invalid")
	}
	v := cur.Lookup("id")
	id, ok := v.Int64OK()
	if !ok {
		return nil, errors.New("receipt_invalid")
	}
	if id != 0 {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		closed, e := r.c.Command(c, "qs", bson.D{{Key: "killCursors", Value: collection}, {Key: "cursors", Value: bson.A{id}}})
		cancel()
		r.cursorReleased = e == nil && cursorReleaseConfirmed(closed, id)
		return nil, errors.New("cursor_release_failed")
	}
	r.cursorReleased = true
	a, ok := cur.Lookup("firstBatch").ArrayOK()
	if !ok {
		return nil, errors.New("receipt_invalid")
	}
	values, err := a.Values()
	if err != nil || len(values) > 2 {
		return nil, errors.New("receipt_invalid")
	}
	out := make([]bson.Raw, 0, len(values))
	for _, v := range values {
		d, ok := v.DocumentOK()
		if !ok {
			return nil, errors.New("receipt_invalid")
		}
		out = append(out, d)
	}
	return out, nil
}

func cursorReleaseConfirmed(raw bson.Raw, id int64) bool {
	if len(raw) > maxResponse || raw.Validate() != nil {
		return false
	}
	elements, err := raw.Elements()
	if err != nil {
		return false
	}
	seen := map[string]bool{}
	for _, element := range elements {
		key := element.Key()
		if key == "ok" || key == "cursorsKilled" || key == "cursorsNotFound" || key == "cursorsAlive" || key == "cursorsUnknown" {
			if seen[key] {
				return false
			}
			seen[key] = true
		}
	}
	ok := raw.Lookup("ok")
	if n, valid := ok.DoubleOK(); !valid || n != 1 {
		if n, valid := integer(ok); !valid || n != 1 {
			return false
		}
	}
	confirmed := 0
	for _, key := range []string{"cursorsKilled", "cursorsNotFound", "cursorsAlive", "cursorsUnknown"} {
		array, valid := raw.Lookup(key).ArrayOK()
		if !valid {
			return false
		}
		values, err := array.Values()
		if err != nil || len(values) > 1 {
			return false
		}
		if key == "cursorsAlive" || key == "cursorsUnknown" {
			if len(values) != 0 {
				return false
			}
			continue
		}
		for _, value := range values {
			n, valid := value.Int64OK()
			if !valid || n != id {
				return false
			}
			confirmed++
		}
	}
	return confirmed == 1
}

type snapshot struct {
	hash    string
	process primitive.ObjectID
	check   targetCheck
	auth    authorization
}

func identityHash(rawUUID primitive.Binary, members []string) string {
	h := sha256.Sum256(append([]byte{rawUUID.Subtype}, rawUUID.Data...))
	// Fields are deliberately alphabetic; this byte encoding is golden tested
	// against the Python-approved original catalog metadata.
	v := struct {
		Members   []string `json:"advertised_members"`
		Catalog   string   `json:"catalog_binary_sha256"`
		Namespace string   `json:"namespace"`
		Set       string   `json:"replica_set"`
	}{members, hex.EncodeToString(h[:]), "qs", "rs0"}
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func integer(v bson.RawValue) (uint64, bool) {
	var n int64
	switch v.Type {
	case bsontype.Int32:
		n = int64(v.Int32())
	case bsontype.Int64:
		n = v.Int64()
	default:
		return 0, false
	}
	return uint64(n), n >= 0
}
func boolptr(b bool) *bool { return &b }
func parseAuthorization(raw bson.Raw, account string) (authorization, error) {
	var out authorization
	a, ok := raw.Lookup("authInfo").DocumentOK()
	if !ok {
		return out, errors.New("authorization_unknown")
	}
	users, ok := a.Lookup("authenticatedUsers").ArrayOK()
	if !ok {
		return out, errors.New("authorization_unknown")
	}
	values, err := users.Values()
	if err != nil || len(values) != 1 {
		return out, errors.New("authentication_mismatch")
	}
	u, ok := values[0].DocumentOK()
	if !ok {
		return out, errors.New("authentication_mismatch")
	}
	name, nok := u.Lookup("user").StringValueOK()
	db, dok := u.Lookup("db").StringValueOK()
	if !nok || !dok || name != account || db != "admin" {
		return out, errors.New("authentication_mismatch")
	}
	out.Authenticated = true
	out.AuthDB = true
	p, ok := a.Lookup("authenticatedUserPrivileges").ArrayOK()
	if !ok {
		return out, errors.New("authorization_unknown")
	}
	privs, err := p.Values()
	if err != nil || len(privs) > 512 {
		return out, errors.New("authorization_unknown")
	}
	parameter, catalog, migration := false, false, false
	for _, v := range privs {
		d, ok := v.DocumentOK()
		if !ok {
			return out, errors.New("authorization_unknown")
		}
		resource, ok := d.Lookup("resource").DocumentOK()
		if !ok {
			return out, errors.New("authorization_unknown")
		}
		actions, ok := d.Lookup("actions").ArrayOK()
		if !ok {
			return out, errors.New("authorization_unknown")
		}
		acts, err := actions.Values()
		if err != nil || len(acts) > 256 {
			return out, errors.New("authorization_unknown")
		}
		any, _ := resource.Lookup("anyResource").BooleanOK()
		cluster, _ := resource.Lookup("cluster").BooleanOK()
		db, dbOK := resource.Lookup("db").StringValueOK()
		coll, collectionOK := resource.Lookup("collection").StringValueOK()
		namespaceResource := dbOK && collectionOK
		for _, av := range acts {
			s, ok := av.StringValueOK()
			if !ok || len(s) > 128 {
				return out, errors.New("authorization_unknown")
			}
			if (any || cluster) && (s == "getParameter" || s == "anyAction") {
				parameter = true
			}
			if (any || namespaceResource && (db == "qs" || db == "") && coll == "") && (s == "listCollections" || s == "anyAction") {
				catalog = true
			}
			if (any || namespaceResource && (db == "qs" || db == "") && (coll == "" || coll == "schema_migrations")) && (s == "find" || s == "anyAction") {
				migration = true
			}
		}
	}
	out.Observed = true
	out.Parameter = boolptr(parameter)
	out.Catalog = boolptr(catalog)
	out.Migration = boolptr(migration)
	return out, nil
}
func (r *reader) snapshot(ctx context.Context, in input, expected string) (snapshot, error) {
	var s snapshot
	s.check.Namespace = in.Database == "qs"
	raw, err := r.command(ctx, "admin", bson.D{{Key: "hello", Value: 1}})
	if err != nil {
		return s, errors.New("target_identity_mismatch")
	}
	set, ok := raw.Lookup("setName").StringValueOK()
	s.check.Replica = ok && set == "rs0"
	primary, ok := raw.Lookup("isWritablePrimary").BooleanOK()
	s.check.Primary = ok && primary
	process, ok := raw.Lookup("topologyVersion", "processId").ObjectIDOK()
	if !ok {
		return s, errors.New("target_identity_mismatch")
	}
	s.process = process
	h, ok := raw.Lookup("hosts").ArrayOK()
	if !ok {
		return s, errors.New("target_identity_mismatch")
	}
	values, err := h.Values()
	if err != nil || len(values) != 1 {
		return s, errors.New("target_identity_mismatch")
	}
	members := []string{}
	for _, v := range values {
		name, ok := v.StringValueOK()
		if !ok {
			return s, errors.New("target_identity_mismatch")
		}
		members = append(members, name)
	}
	sort.Strings(members)
	s.check.Members = members[0] == "mongo:27017"
	if !s.check.Namespace || !s.check.Replica || !s.check.Primary || !s.check.Members {
		return s, errors.New("target_identity_mismatch")
	}
	raw, err = r.command(ctx, "admin", bson.D{{Key: "connectionStatus", Value: 1}, {Key: "showPrivileges", Value: true}})
	if err != nil {
		return s, errors.New("authorization_unknown")
	}
	s.auth, err = parseAuthorization(raw, in.Account)
	if err != nil {
		return s, err
	}
	raw, err = r.command(ctx, "qs", bson.D{{Key: "find", Value: "schema_migrations"}, {Key: "filter", Value: bson.D{}}, {Key: "projection", Value: bson.D{{Key: "_id", Value: 0}, {Key: "version", Value: 1}, {Key: "dirty", Value: 1}}}, {Key: "limit", Value: 2}, {Key: "batchSize", Value: 2}, {Key: "singleBatch", Value: true}, {Key: "maxTimeMS", Value: 5000}})
	if err != nil {
		return s, errors.New("migration_state_invalid")
	}
	docs, err := r.batch(ctx, raw, "schema_migrations")
	if err != nil {
		return s, err
	}
	if len(docs) != 1 {
		return s, errors.New("migration_state_invalid")
	}
	s.check.Head, ok = integer(docs[0].Lookup("version"))
	if !ok {
		return s, errors.New("migration_state_invalid")
	}
	s.check.Dirty, ok = docs[0].Lookup("dirty").BooleanOK()
	if !ok || s.check.Dirty || s.check.Head != 39 {
		return s, errors.New("migration_state_invalid")
	}
	raw, err = r.command(ctx, "qs", bson.D{{Key: "listCollections", Value: 1}, {Key: "filter", Value: bson.D{{Key: "name", Value: bson.D{{Key: "$in", Value: bson.A{"system.profile", "domain_event_outbox"}}}}}}, {Key: "nameOnly", Value: false}, {Key: "authorizedCollections", Value: false}, {Key: "cursor", Value: bson.D{{Key: "batchSize", Value: 3}}}, {Key: "maxTimeMS", Value: 5000}})
	if err != nil {
		return s, errors.New("target_identity_mismatch")
	}
	docs, err = r.batch(ctx, raw, "$cmd.listCollections")
	if err != nil {
		return s, err
	}
	profiles := 0
	s.check.RetiredAbsent = true
	for _, d := range docs {
		name, ok := d.Lookup("name").StringValueOK()
		if !ok {
			return s, errors.New("target_identity_mismatch")
		}
		switch name {
		case "domain_event_outbox":
			s.check.RetiredAbsent = false
		case "system.profile":
			profiles++
			subtype, data, ok := d.Lookup("info", "uuid").BinaryOK()
			if !ok || subtype != 4 || len(data) != 16 {
				return s, errors.New("target_identity_mismatch")
			}
			s.hash = identityHash(primitive.Binary{Subtype: subtype, Data: data}, members)
		default:
			return s, errors.New("target_identity_mismatch")
		}
	}
	s.check.Catalog = profiles == 1 && s.hash == expected
	if !s.check.Catalog || !s.check.RetiredAbsent {
		return s, errors.New("target_identity_mismatch")
	}
	return s, nil
}
func versionParts(v bson.RawValue) (uint64, uint64, error) {
	s, ok := v.StringValueOK()
	if !ok {
		return 0, 0, errors.New("fcv_shape_invalid")
	}
	m := fcvRE.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, errors.New("fcv_shape_invalid")
	}
	a, _ := strconv.ParseUint(m[1], 10, 64)
	b, _ := strconv.ParseUint(m[2], 10, 64)
	if s != fmt.Sprintf("%d.%d", a, b) {
		return 0, 0, errors.New("fcv_shape_invalid")
	}
	return a, b, nil
}
func parseFCV(raw bson.Raw) (fcv, error) {
	var out fcv
	d, ok := raw.Lookup("featureCompatibilityVersion").DocumentOK()
	if !ok {
		return out, errors.New("fcv_shape_invalid")
	}
	elements, err := d.Elements()
	if err != nil {
		return out, errors.New("fcv_shape_invalid")
	}
	seen := map[string]bool{}
	for _, e := range elements {
		k := e.Key()
		if seen[k] {
			return fcv{}, errors.New("fcv_shape_invalid")
		}
		seen[k] = true
		v := e.Value()
		switch k {
		case "version":
			out.Major, out.Minor, err = versionParts(v)
		case "targetVersion":
			out.Target = true
			out.TargetMajor, out.TargetMinor, err = versionParts(v)
		case "previousVersion":
			out.Previous = true
			out.PreviousMajor, out.PreviousMinor, err = versionParts(v)
		case "isCleaningServerMetadata":
			out.Cleanup = true
			out.CleanupValue, ok = v.BooleanOK()
			if !ok {
				err = errors.New("fcv_shape_invalid")
			}
		case "changeTimestamp":
			out.Timestamp = true
			var t, i uint32
			t, i, ok = v.TimestampOK()
			if !ok {
				err = errors.New("fcv_shape_invalid")
			}
			out.TimestampSeconds = uint64(t)
			out.TimestampIncrement = uint64(i)
		case "_id":
			s, sok := v.StringValueOK()
			if !sok || s != "featureCompatibilityVersion" {
				err = errors.New("fcv_shape_invalid")
			}
		default:
			err = errors.New("fcv_shape_invalid")
		}
		if err != nil {
			return fcv{}, err
		}
	}
	if !seen["version"] {
		return fcv{}, errors.New("fcv_shape_invalid")
	}
	out.Known = true
	out.Transition = out.Target || out.Previous || out.CleanupValue
	return out, nil
}
func parseTTL(raw bson.Raw) (ttlMonitor, error) {
	value, valid := raw.Lookup("ttlMonitorEnabled").BooleanOK()
	if !valid {
		return ttlMonitor{}, errors.New("ttl_metadata_invalid")
	}
	return ttlMonitor{Known: true, Enabled: value}, nil
}
func newReceipt(in input) receipt {
	return receipt{Protocol: "mongo_admin_metadata_v1", Operation: "metadata-visibility-admin-profile", Database: "mongodb", SourceSHA: in.SourceSHA, RunID: in.RunID, ToolHash: in.ToolHash, SourceBinding: in.SourceBinding, Expected: in.Expected, Error: "none"}
}
func probe(ctx context.Context, in input, expected string, c commander) (out receipt) {
	out = newReceipt(in)
	start := time.Now()
	r := reader{c: c, cursorReleased: true}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := c.Close(closeCtx)
		cancel()
		out.Release.Client = err == nil
		out.Release.Cursors = r.cursorReleased
		if err != nil {
			out.Error = "client_release_failed"
		}
		if !r.cursorReleased {
			out.Error = "cursor_release_failed"
		}
		out.Observation = out.Error == "none" && out.Release.Client && out.Release.Cursors
		// The transport owns the remaining releases; the tool cannot attest them.
		out.Complete = false
		out.Eligible = false
		out.Budget.Elapsed = uint64(time.Since(start).Milliseconds())
		out.Budget.Queries = r.count
	}()
	before, err := r.snapshot(ctx, in, expected)
	out.Target = before.hash
	out.TargetCheck = before.check
	out.Authorization = before.auth
	if err != nil {
		out.Error = err.Error()
		return out
	}
	raw, err := r.command(ctx, "admin", bson.D{{Key: "buildInfo", Value: 1}})
	if err != nil {
		out.Error = "build_metadata_invalid"
		return out
	}
	ver, ok := raw.Lookup("version").StringValueOK()
	if !ok || ver != "7.0.31" {
		out.Error = "build_version_mismatch"
		return out
	}
	out.Build = build{Known: true, Major: 7, Minor: 0, Patch: 31, Matches: true}
	raw, err = r.command(ctx, "admin", bson.D{{Key: "getParameter", Value: 1}, {Key: "featureCompatibilityVersion", Value: 1}, {Key: "ttlMonitorEnabled", Value: 1}})
	if err != nil {
		var ce mongo.CommandError
		if errors.As(err, &ce) && ce.Code == 13 {
			out.Error = "fcv_permission_denied"
		} else {
			out.Error = "fcv_read_failed"
		}
	} else {
		out.FCV, err = parseFCV(raw)
		if err != nil {
			out.Error = "fcv_shape_invalid"
		} else {
			out.TTL, err = parseTTL(raw)
			if err != nil {
				out.Error = "ttl_metadata_invalid"
			} else if out.FCV.Transition {
				out.Error = "fcv_transition_observed"
			}
		}
	}
	after, err := r.snapshot(ctx, in, expected)
	if err != nil {
		out.Error = "identity_changed"
		return out
	}
	out.TargetCheck.Unchanged = before.hash == after.hash && before.check == after.check
	out.TargetCheck.ProcessUnchanged = before.process == after.process
	if !out.TargetCheck.Unchanged || !out.TargetCheck.ProcessUnchanged || !sameAuthorization(before.auth, after.auth) {
		out.Error = "identity_changed"
	}
	return out
}
func sameAuthorization(a, b authorization) bool {
	return a.Authenticated == b.Authenticated && a.AuthDB == b.AuthDB && a.Observed == b.Observed && a.Parameter != nil && b.Parameter != nil && *a.Parameter == *b.Parameter && a.Catalog != nil && b.Catalog != nil && *a.Catalog == *b.Catalog && a.Migration != nil && b.Migration != nil && *a.Migration == *b.Migration
}
func run(rd io.Reader, wr io.Writer) (code int) {
	in, err := decodeInput(rd)
	out := newReceipt(in)
	code = 1
	defer func() {
		if json.NewEncoder(wr).Encode(out) != nil {
			code = 1
		}
	}()
	if err != nil {
		out.Error = "input_invalid"
		return code
	}
	exe, err := os.Executable()
	if err != nil {
		out.Error = "source_binding_failed"
		return code
	}
	b, err := os.ReadFile(exe)
	if err != nil {
		out.Error = "source_binding_failed"
		return code
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != in.ToolHash {
		out.Error = "source_binding_failed"
		return code
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := connect(ctx, in)
	if err != nil {
		out.Error = "tool_failed"
		return code
	}
	out = probe(ctx, in, expectedTarget, c)
	if out.Observation {
		return 0
	}
	return 1
}
func main() {
	if len(os.Args) != 1 {
		os.Exit(1)
	}
	os.Exit(run(os.Stdin, os.Stdout))
}
