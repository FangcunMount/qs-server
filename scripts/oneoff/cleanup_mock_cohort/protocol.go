package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A physical identity always includes its store, object and typed primary key.
// Numeric values in different identity domains are deliberately never joined.
type cell struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Null bool   `json:"null"`
	Raw  []byte `json:"raw"`
}

type object struct {
	Store       string `json:"store"`
	Name        string `json:"name"`
	Key         []cell `json:"key"`
	Fingerprint string `json:"fingerprint"`
}

type iamScope struct {
	Users      []string `json:"user_ids"`
	Profiles   []string `json:"profile_ids"`
	Identities []string `json:"login_identity_ids"`
}

type binding struct {
	SQLUUID       string `json:"mysql_server_uuid"`
	SQLDatabase   string `json:"mysql_database"`
	MongoIdentity string `json:"mongo_identity_sha256"`
	MongoDatabase string `json:"mongo_database"`
	SQLVersion    int64  `json:"mysql_version"`
	MongoVersion  int64  `json:"mongo_version"`
}

type selection struct {
	Source   string `json:"source"`
	OrgID    string `json:"org_id"`
	From     string `json:"from_shanghai"`
	Through  string `json:"through_shanghai"`
	Evidence string `json:"owner_declared_cohort"`
}

type qsScope struct {
	Graph      *graphBinding `json:"graph"`
	Selector   selection     `json:"selector"`
	Binding    binding       `json:"binding"`
	Objects    []object      `json:"objects"`
	Exclusions []string      `json:"exclusions"`
}

type group struct {
	Format       int             `json:"format_version"`
	Kind         string          `json:"kind"`
	Operation    string          `json:"operation_id"`
	OwnerSHA     string          `json:"owner_confirmation_sha256"`
	ProtectedSHA string          `json:"protected_manifest_sha256"`
	IAM          json.RawMessage `json:"iam"`
	QS           qsScope         `json:"qs"`
}

type protected struct {
	Format int             `json:"format_version"`
	Kind   string          `json:"kind"`
	IAM    json.RawMessage `json:"iam"`
	QS     protectionScope `json:"qs"`
}

type protectionScope struct {
	System []object `json:"system"`
	Clack  []object `json:"clack"`
	Other  []object `json:"other"`
}

// Records preserve SQL NULL, SQL column type and unmodified driver bytes, or
// complete BSON including its original field order and BSON types.
type record struct {
	Object     object `json:"object"`
	Columns    []cell `json:"columns"`
	BSON       []byte `json:"bson"`
	Definition []byte `json:"definition"`
}

type material struct {
	Format       int         `json:"format_version"`
	Kind         string      `json:"kind"`
	Operation    string      `json:"operation_id"`
	GroupSHA     string      `json:"group_manifest_sha256"`
	ProtectedSHA string      `json:"protected_manifest_sha256"`
	ToolSource   string      `json:"tool_source"`
	Binding      binding     `json:"binding"`
	Records      []recordRef `json:"records"`
	Protected    []recordRef `json:"protected"`
}

type recordRef struct {
	Object object `json:"object"`
	File   string `json:"file"`
	SHA    string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

type approval struct {
	Format        int       `json:"format_version"`
	Kind          string    `json:"kind"`
	Action        string    `json:"action"`
	Operation     string    `json:"operation_id"`
	GroupSHA      string    `json:"group_manifest_sha256"`
	ArchiveSHA    string    `json:"archive_sha256"`
	ProtectedSHA  string    `json:"protected_manifest_sha256"`
	ToolSource    string    `json:"tool_source"`
	FreezeSHA     string    `json:"freeze_receipt_sha256"`
	SettlementSHA string    `json:"settlement_receipt_sha256"`
	Issued        time.Time `json:"issued_at"`
	Expires       time.Time `json:"expires_at"`
}

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,95}$`)
var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
var keyIdentifier = regexp.MustCompile(`^[_a-z][_a-z0-9]{0,63}$`)

func digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func encode(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}

func strictDecode(raw []byte, out any) error {
	// encoding/json otherwise accepts duplicated keys, including nested ones.
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		t, err := d.Token()
		if err != nil {
			return err
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					k, e := d.Token()
					if e != nil {
						return e
					}
					key, ok := k.(string)
					if !ok || seen[key] {
						return errors.New("duplicate JSON key")
					}
					seen[key] = true
					if e = walk(); e != nil {
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
				return errors.New("unexpected JSON delimiter")
			}
			_, err = d.Token()
			return err
		}
		return nil
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing JSON")
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	// Reject case aliases, missing required members and null scalar values. This
	// compares decoded values only; the original input bytes/SHA stay untouched.
	var original, canonical any
	a := json.NewDecoder(bytes.NewReader(raw))
	a.UseNumber()
	if e := a.Decode(&original); e != nil {
		return e
	}
	a = json.NewDecoder(bytes.NewReader(encode(out)))
	a.UseNumber()
	if e := a.Decode(&canonical); e != nil {
		return e
	}
	if !reflect.DeepEqual(original, canonical) {
		return errors.New("nonliteral, missing or null JSON member")
	}
	return nil
}

func objectID(o object) string {
	return digest(encode(struct {
		Store, Name string
		Key         []cell
	}{o.Store, o.Name, o.Key}))
}
func fingerprint(r record) string { r.Object.Fingerprint = ""; return digest(encode(r)) }

func validateObject(o object) error {
	if (o.Store != "mysql" && o.Store != "mongo") || !identifier.MatchString(o.Name) || len(o.Key) == 0 {
		return errors.New("invalid physical object identity")
	}
	last := ""
	for _, k := range o.Key {
		if !keyIdentifier.MatchString(k.Name) || k.Name <= last || k.Null || k.Raw == nil || k.Type == "" {
			return errors.New("invalid or unsorted typed primary key")
		}
		last = k.Name
	}
	if o.Store == "mongo" && (len(o.Key) != 1 || o.Key[0].Name != "_id") {
		return errors.New("mongo requires its physical typed _id")
	}
	return nil
}

func validateGroup(g group, p protected) error {
	if g.Format != 1 || g.Kind != "mock_cohort" || !idPattern.MatchString(g.Operation) || !hashPattern.MatchString(g.OwnerSHA) || !hashPattern.MatchString(g.ProtectedSHA) {
		return errors.New("invalid cohort authority")
	}
	if g.QS.Binding.SQLVersion != 101 || g.QS.Binding.MongoVersion != 39 {
		return errors.New("only explicit clean 101/39 baseline is supported; migrations are never run")
	}
	if g.QS.Selector.Source == "" || g.QS.Selector.OrgID == "" || g.QS.Selector.Evidence == "" {
		return errors.New("owner cohort selector is required")
	}
	from, e := time.Parse(time.RFC3339, g.QS.Selector.From)
	if e != nil {
		return e
	}
	through, e := time.Parse(time.RFC3339, g.QS.Selector.Through)
	if e != nil || !from.Before(through) || !strings.HasSuffix(g.QS.Selector.From, "+08:00") || !strings.HasSuffix(g.QS.Selector.Through, "+08:00") {
		return errors.New("explicit Shanghai selection window required")
	}
	var iam iamScope
	if e := strictDecode(g.IAM, &iam); e != nil {
		return e
	}
	for _, ids := range [][]string{iam.Users, iam.Profiles, iam.Identities} {
		if ids == nil {
			return errors.New("explicit IAM typed arrays required")
		}
		seen := map[string]bool{}
		for _, id := range ids {
			n, e := strconv.ParseUint(id, 10, 64)
			if e != nil || n == 0 || n > math.MaxInt64 || strconv.FormatUint(n, 10) != id || seen[id] {
				return errors.New("IAM IDs must be unique canonical decimal strings within their own domain")
			}
			seen[id] = true
		}
	}
	var protectedIAM iamScope
	if e := strictDecode(p.IAM, &protectedIAM); e != nil {
		return e
	}
	if protectedIAM.Users == nil || protectedIAM.Profiles == nil || protectedIAM.Identities == nil || p.QS.Other == nil {
		return errors.New("shared protection arrays must be explicit")
	}
	for i, ids := range [][]string{iam.Users, iam.Profiles, iam.Identities} {
		guards := [][]string{protectedIAM.Users, protectedIAM.Profiles, protectedIAM.Identities}[i]
		for _, id := range ids {
			for _, guard := range guards {
				if id == guard {
					return errors.New("protected IAM domain intersection")
				}
			}
		}
	}
	if p.Format != 1 || p.Kind != "mock_cohort_protected" || len(p.QS.System) == 0 || len(p.QS.Clack) == 0 {
		return errors.New("system and clack protections must be explicit")
	}
	guards := map[string]bool{}
	for _, o := range append(append(append([]object{}, p.QS.System...), p.QS.Clack...), p.QS.Other...) {
		if e := validateObject(o); e != nil {
			return e
		}
		guards[objectID(o)] = true
	}
	seen := map[string]bool{}
	if len(g.QS.Objects) == 0 {
		return errors.New("explicit physical target objects are required")
	}
	for _, o := range g.QS.Objects {
		if e := validateObject(o); e != nil {
			return e
		}
		k := objectID(o)
		if guards[k] || seen[k] {
			return fmt.Errorf("protected or duplicated physical object: %s/%s", o.Store, o.Name)
		}
		seen[k] = true
	}
	return nil
}

func sortedObjects(in []object) []object {
	out := append([]object{}, in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Store != out[j].Store {
			return out[i].Store < out[j].Store
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return objectID(out[i]) < objectID(out[j])
	})
	return out
}
