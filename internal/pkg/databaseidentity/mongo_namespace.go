// Package databaseidentity validates metadata observations. Its exported
// values are not an authorization capability, a business verdict or a DDL gate.
package databaseidentity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const MongoNamespaceAnchorKind = "selected_namespace_kept_uuids_v1"
const MongoReplicaAnchorKind = "replica_set_uuid_v1"

var ErrMongoNamespaceAnchor = errors.New("mongo_namespace_anchor_rejected")
var ErrMongoNamespaceRead = errors.New("mongo_namespace_anchor_read_failed")

// This fixed set survives this retirement batch and its final migrations. It
// excludes the four targets, driver-recreated schema_migrations and profiling.
// Each absence is observed in a full-visibility query, never permission-filtered.
var keptMongoNames = [...]string{"answersheets", "interpret_report_artifacts", "interpretation_runs", "report_generations"}

type MongoKeptCollection struct {
	Name    string `json:"name"`
	Present bool   `json:"present"`
	UUID    string `json:"uuid"`
}

type MongoNamespaceAnchor struct {
	Kind           string                `json:"kind"`
	Hash           string                `json:"hash"`
	EndpointSHA256 string                `json:"endpoint_sha256"`
	Database       string                `json:"database"`
	ReplicaSetName string                `json:"replica_set_name"`
	Collections    []MongoKeptCollection `json:"collections"`
}

// ValidateOptionalMongoNamespaceJSON closes only this new optional metadata
// field. Legacy request grammar stays unchanged; explicit null/case aliases
// must not turn a requested profile into the legacy missing-field default.
func ValidateOptionalMongoNamespaceJSON(raw []byte, field string) error {
	if field != "mongodb_namespace_anchor" && field != "namespace_anchor" && field != "mongo_anchor_profile" {
		return ErrMongoNamespaceAnchor
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil || values == nil {
		return ErrMongoNamespaceAnchor
	}
	for key, encoded := range values {
		if !strings.EqualFold(key, field) {
			continue
		}
		if key != field || bytes.Equal(bytes.TrimSpace(encoded), []byte("null")) {
			return ErrMongoNamespaceAnchor
		}
		if field == "mongo_anchor_profile" {
			var profile string
			if json.Unmarshal(encoded, &profile) != nil || (profile != MongoNamespaceAnchorKind && profile != MongoReplicaAnchorKind) {
				return ErrMongoNamespaceAnchor
			}
		} else {
			var anchor MongoNamespaceAnchor
			if json.Unmarshal(encoded, &anchor) != nil || anchor.Validate() != nil {
				return ErrMongoNamespaceAnchor
			}
		}
	}
	return nil
}

// encoding/json otherwise accepts duplicate keys, case aliases and missing
// false-valued fields. The new metadata profile has a closed exact grammar.
func (a *MongoNamespaceAnchor) UnmarshalJSON(raw []byte) error {
	if a == nil || len(raw) > 8192 || !utf8.Valid(raw) {
		return ErrMongoNamespaceAnchor
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		token, err := d.Token()
		if err != nil {
			return ErrMongoNamespaceAnchor
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				if e != nil {
					return ErrMongoNamespaceAnchor
				}
				name, valid := key.(string)
				if !valid || seen[name] {
					return ErrMongoNamespaceAnchor
				}
				seen[name] = true
				if e := walk(); e != nil {
					return e
				}
			}
		case '[':
			for d.More() {
				if e := walk(); e != nil {
					return e
				}
			}
		default:
			return ErrMongoNamespaceAnchor
		}
		if _, e := d.Token(); e != nil {
			return ErrMongoNamespaceAnchor
		}
		return nil
	}
	if walk() != nil {
		return ErrMongoNamespaceAnchor
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrMongoNamespaceAnchor
	}
	fields := func(encoded []byte, required ...string) (map[string]json.RawMessage, error) {
		var values map[string]json.RawMessage
		if json.Unmarshal(encoded, &values) != nil || len(values) != len(required) {
			return nil, ErrMongoNamespaceAnchor
		}
		for _, name := range required {
			value, ok := values[name]
			if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return nil, ErrMongoNamespaceAnchor
			}
		}
		return values, nil
	}
	values, err := fields(raw, "kind", "hash", "endpoint_sha256", "database", "replica_set_name", "collections")
	if err != nil {
		return err
	}
	var rows []json.RawMessage
	if json.Unmarshal(values["collections"], &rows) != nil || len(rows) != len(keptMongoNames) {
		return ErrMongoNamespaceAnchor
	}
	for _, row := range rows {
		if _, err := fields(row, "name", "present", "uuid"); err != nil {
			return err
		}
	}
	type plain MongoNamespaceAnchor
	var decoded plain
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&decoded) != nil || decoder.Decode(new(any)) != io.EOF {
		return ErrMongoNamespaceAnchor
	}
	*a = MongoNamespaceAnchor(decoded)
	return nil
}

func framedHash(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		var header [9]byte
		header[0] = 1
		binary.BigEndian.PutUint64(header[1:], uint64(len(part)))
		_, _ = h.Write(header[:])
		_, _ = h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func lowerHex(value string, bytes int) bool {
	if len(value) != bytes*2 || strings.ToLower(value) != value {
		return false
	}
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == bytes
}

func boundedName(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n")
}

// Credentials are deliberately excluded. The endpoint is the exact seed route
// used by the existing connection owner; it is not asserted to be a cluster UUID.
func MongoEndpointSHA256(host string, port int, database string) (string, error) {
	if !boundedName(host, 4096) || !boundedName(database, 128) || port < 1 || port > 65535 {
		return "", ErrMongoNamespaceAnchor
	}
	return framedHash("mongodb-approved-seed-endpoint/v1", net.JoinHostPort(host, strconv.Itoa(port)), "admin", database), nil
}

func (a *MongoNamespaceAnchor) computedHash() (string, error) {
	if a == nil || a.Kind != MongoNamespaceAnchorKind || !lowerHex(a.EndpointSHA256, 32) || !boundedName(a.Database, 128) || !boundedName(a.ReplicaSetName, 128) || len(a.Collections) != len(keptMongoNames) {
		return "", ErrMongoNamespaceAnchor
	}
	parts := []string{MongoNamespaceAnchorKind, a.EndpointSHA256, a.Database, a.ReplicaSetName}
	seen := map[string]bool{}
	present := 0
	for i, row := range a.Collections {
		if row.Name != keptMongoNames[i] {
			return "", ErrMongoNamespaceAnchor
		}
		flag := "absent"
		if row.Present {
			if !lowerHex(row.UUID, 16) || row.UUID == strings.Repeat("0", 32) || seen[row.UUID] {
				return "", ErrMongoNamespaceAnchor
			}
			seen[row.UUID] = true
			present++
			flag = "present"
		} else if row.UUID != "" {
			return "", ErrMongoNamespaceAnchor
		}
		parts = append(parts, row.Name, flag, row.UUID)
	}
	if present == 0 {
		return "", ErrMongoNamespaceAnchor
	}
	return framedHash(parts...), nil
}

func (a *MongoNamespaceAnchor) Validate() error {
	hash, err := a.computedHash()
	if err != nil || !lowerHex(a.Hash, 32) || a.Hash != hash {
		return ErrMongoNamespaceAnchor
	}
	return nil
}

func (a *MongoNamespaceAnchor) Clone() *MongoNamespaceAnchor {
	if a == nil {
		return nil
	}
	out := *a
	out.Collections = append([]MongoKeptCollection(nil), a.Collections...)
	return &out
}

// Match also validates the observations. A name-only/equal-hash DTO is not a
// replacement for the actual catalog reader at the caller's before/after edges.
func MatchMongoNamespaceAnchors(expected, actual *MongoNamespaceAnchor) bool {
	if expected == nil || actual == nil {
		return expected == nil && actual == nil
	}
	if expected.Validate() != nil || actual.Validate() != nil || expected.Kind != actual.Kind || expected.Hash != actual.Hash || expected.EndpointSHA256 != actual.EndpointSHA256 || expected.Database != actual.Database || expected.ReplicaSetName != actual.ReplicaSetName {
		return false
	}
	for i := range expected.Collections {
		if expected.Collections[i] != actual.Collections[i] {
			return false
		}
	}
	return true
}

func uniqueFields(raw bson.Raw) (map[string]bson.RawValue, error) {
	if raw.Validate() != nil {
		return nil, ErrMongoNamespaceAnchor
	}
	elements, err := raw.Elements()
	if err != nil {
		return nil, ErrMongoNamespaceAnchor
	}
	fields := map[string]bson.RawValue{}
	for _, element := range elements {
		if _, found := fields[element.Key()]; found {
			return nil, ErrMongoNamespaceAnchor
		}
		fields[element.Key()] = element.Value()
	}
	return fields, nil
}

// MongoNamespaceAnchorFromMetadata consumes actual server BSON. The caller must
// establish complete visibility; ObserveMongoNamespaceAnchor does so with a
// fixed filter and authorizedCollections=false. It never calls replSetGetConfig.
func MongoNamespaceAnchorFromMetadata(hello bson.Raw, catalog []bson.Raw, database, endpoint string) (*MongoNamespaceAnchor, error) {
	if len(hello) > 64<<10 || len(catalog) > 500 {
		return nil, ErrMongoNamespaceAnchor
	}
	hf, err := uniqueFields(hello)
	if err != nil {
		return nil, err
	}
	setName, ok := hf["setName"].StringValueOK()
	if !ok || !boundedName(setName, 128) || hf["msg"].Type != 0 {
		return nil, ErrMongoNamespaceAnchor
	}
	a := &MongoNamespaceAnchor{Kind: MongoNamespaceAnchorKind, Database: database, EndpointSHA256: endpoint, ReplicaSetName: setName, Collections: make([]MongoKeptCollection, len(keptMongoNames))}
	positions := map[string]int{}
	for i, name := range keptMongoNames {
		positions[name] = i
		a.Collections[i].Name = name
	}
	seen := map[string]bool{}
	for _, raw := range catalog {
		if len(raw) > 1<<20 {
			return nil, ErrMongoNamespaceAnchor
		}
		fields, e := uniqueFields(raw)
		if e != nil {
			return nil, e
		}
		name, valid := fields["name"].StringValueOK()
		if !valid || name == "" || seen[name] {
			return nil, ErrMongoNamespaceAnchor
		}
		seen[name] = true
		i, retained := positions[name]
		if !retained {
			continue
		}
		kind, valid := fields["type"].StringValueOK()
		info, infoOK := fields["info"].DocumentOK()
		if !valid || kind != "collection" || !infoOK {
			return nil, ErrMongoNamespaceAnchor
		}
		information, e := uniqueFields(info)
		if e != nil {
			return nil, e
		}
		subtype, uuid, valid := information["uuid"].BinaryOK()
		if !valid || subtype != 4 || len(uuid) != 16 {
			return nil, ErrMongoNamespaceAnchor
		}
		a.Collections[i].Present = true
		a.Collections[i].UUID = hex.EncodeToString(uuid)
	}
	a.Hash, err = a.computedHash()
	if err != nil {
		return nil, err
	}
	return a, nil
}

// The host owns the client, connection and sessions. This helper owns only its
// cursor. Metadata cannot run inside the host's borrowed transaction; callers
// pass the original cancellable scope context, before/after the transaction.
func ObserveMongoNamespaceAnchor(ctx context.Context, db *mongo.Database, endpoint string) (*MongoNamespaceAnchor, error) {
	if ctx == nil || db == nil || ctx.Err() != nil || mongo.SessionFromContext(ctx) != nil {
		return nil, ErrMongoNamespaceRead
	}
	q, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var hello bson.Raw
	if db.Client().Database("admin").RunCommand(q, bson.D{{Key: "hello", Value: 1}}).Decode(&hello) != nil {
		return nil, ErrMongoNamespaceRead
	}
	names := bson.A{}
	for _, name := range keptMongoNames {
		names = append(names, name)
	}
	cur, err := db.ListCollections(q, bson.D{{Key: "name", Value: bson.D{{Key: "$in", Value: names}}}}, options.ListCollections().SetNameOnly(false).SetAuthorizedCollections(false))
	if err != nil {
		return nil, ErrMongoNamespaceRead
	}
	var rows []bson.Raw
	err = cur.All(q, &rows)
	closed := cur.Close(q)
	if err != nil || closed != nil || len(rows) > len(keptMongoNames) {
		return nil, ErrMongoNamespaceRead
	}
	for _, raw := range rows {
		name, ok := raw.Lookup("name").StringValueOK()
		found := false
		for _, expected := range keptMongoNames {
			if name == expected {
				found = true
			}
		}
		if !ok || !found {
			return nil, ErrMongoNamespaceAnchor
		}
	}
	return MongoNamespaceAnchorFromMetadata(hello, rows, db.Name(), endpoint)
}
