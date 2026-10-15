package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

var testUUID = primitive.Binary{Subtype: 4, Data: []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}}
var testProcess = primitive.NewObjectID()

type fakeCommander struct {
	responses  []bson.D
	errors     map[int]error
	commands   []bson.D
	dbs        []string
	closed     bool
	closeError error
}

func raw(d bson.D) bson.Raw {
	b, err := bson.Marshal(d)
	if err != nil {
		panic("fixture_encode_failed")
	}
	return b
}
func (f *fakeCommander) Command(_ context.Context, db string, d bson.D) (bson.Raw, error) {
	f.commands = append(f.commands, d)
	f.dbs = append(f.dbs, db)
	i := len(f.commands) - 1
	if err := f.errors[i]; err != nil {
		return nil, err
	}
	if i >= len(f.responses) {
		return nil, errors.New("fixture-response-private-marker")
	}
	return raw(f.responses[i]), nil
}
func (f *fakeCommander) Close(context.Context) error { f.closed = true; return f.closeError }
func cursor(batch bson.A) bson.D {
	return bson.D{{Key: "ok", Value: 1.0}, {Key: "cursor", Value: bson.D{{Key: "id", Value: int64(0)}, {Key: "firstBatch", Value: batch}}}}
}
func fixtureSnapshot() []bson.D {
	return []bson.D{
		{{Key: "setName", Value: "rs0"}, {Key: "hosts", Value: bson.A{"mongo:27017"}}, {Key: "isWritablePrimary", Value: true}, {Key: "topologyVersion", Value: bson.D{{Key: "processId", Value: testProcess}}}},
		{{Key: "authInfo", Value: bson.D{{Key: "authenticatedUsers", Value: bson.A{bson.D{{Key: "user", Value: "fixture-account-private"}, {Key: "db", Value: "admin"}}}}, {Key: "authenticatedUserPrivileges", Value: bson.A{bson.D{{Key: "resource", Value: bson.D{{Key: "cluster", Value: true}}}, {Key: "actions", Value: bson.A{"getParameter"}}}, bson.D{{Key: "resource", Value: bson.D{{Key: "db", Value: "qs"}, {Key: "collection", Value: ""}}}, {Key: "actions", Value: bson.A{"find", "listCollections"}}}}}}}},
		cursor(bson.A{bson.D{{Key: "version", Value: int64(39)}, {Key: "dirty", Value: false}}}),
		cursor(bson.A{bson.D{{Key: "name", Value: "system.profile"}, {Key: "info", Value: bson.D{{Key: "uuid", Value: testUUID}}}}}),
	}
}
func fixtureCommands() []bson.D {
	v := fixtureSnapshot()
	v = append(v, bson.D{{Key: "version", Value: "7.0.31"}}, bson.D{{Key: "featureCompatibilityVersion", Value: bson.D{{Key: "version", Value: "6.0"}}}, {Key: "ttlMonitorEnabled", Value: true}})
	return append(v, fixtureSnapshot()...)
}
func fixtureInput() input {
	return input{SourceSHA: strings.Repeat("a", 40), RunID: "12345-1", Expected: expectedTarget, SourceBinding: strings.Repeat("b", 64), ToolHash: strings.Repeat("c", 64), Port: 37018, Account: "fixture-account-private", Password: "fixture-password-private", Database: "qs"}
}
func assertPrivate(t *testing.T, v receipt) {
	t.Helper()
	b, _ := json.Marshal(v)
	for _, marker := range []string{"fixture-account-private", "fixture-password-private", "fixture-response-private-marker", "mongo:27017", "127.0.0.1", "authenticatedUserPrivileges"} {
		if bytes.Contains(b, []byte(marker)) {
			t.Fatal("private_metadata_leaked")
		}
	}
	if v.Complete || v.Eligible || v.Backup || v.Business {
		t.Fatal("tool_claimed_transport_or_business")
	}
}
func TestMongoFixedIdentityEncodingGolden(t *testing.T) {
	want := "ffe0617bf8b1accbbd923ecb5a2b25d69df63d6543c5348f6ff512284945d69c"
	if got := identityHash(testUUID, []string{"mongo:27017"}); got != want {
		t.Fatalf("identity_golden_mismatch: %s", got)
	}
}
func TestMongoObservationActualFCVAndExactReadAllowlist(t *testing.T) {
	f := &fakeCommander{responses: fixtureCommands()}
	in := fixtureInput()
	got := probe(context.Background(), in, identityHash(testUUID, []string{"mongo:27017"}), f)
	if !got.Observation || got.Error != "none" || !f.closed || !got.Release.Client || !got.Release.Cursors || got.Budget.Queries != 10 || !got.FCV.Known || got.FCV.Major != 6 || got.FCV.Minor != 0 {
		t.Fatal("actual_fcv_observation_failed")
	}
	want := []string{"hello", "connectionStatus", "find", "listCollections", "buildInfo", "getParameter", "hello", "connectionStatus", "find", "listCollections"}
	for i, d := range f.commands {
		if d[0].Key != want[i] {
			t.Fatal("command_allowlist_failed")
		}
	}
	if !reflect.DeepEqual(f.commands[5], bson.D{{Key: "getParameter", Value: 1}, {Key: "featureCompatibilityVersion", Value: 1}, {Key: "ttlMonitorEnabled", Value: 1}}) {
		t.Fatal("fcv_parameter_scope_expanded")
	}
	for _, index := range []int{2, 8} {
		d := raw(f.commands[index])
		p, ok := d.Lookup("projection").DocumentOK()
		if !ok || len(p) != len(raw(bson.D{{Key: "_id", Value: 0}, {Key: "version", Value: 1}, {Key: "dirty", Value: 1}})) {
			t.Fatal("head_projection_expanded")
		}
		if v, ok := d.Lookup("singleBatch").BooleanOK(); !ok || !v {
			t.Fatal("head_cursor_unbounded")
		}
	}
	assertPrivate(t, got)
}

func TestMongoTTLMetadataIsObservedAndNeverGuessedDisabled(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		value                   any
		include, known, enabled bool
	}{
		{"enabled", true, true, true, true}, {"disabled", false, true, true, false},
		{"missing", nil, false, false, false}, {"string", "false", true, false, false}, {"integer", 0, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeCommander{responses: fixtureCommands()}
			d := bson.D{{Key: "featureCompatibilityVersion", Value: bson.D{{Key: "version", Value: "7.0"}}}}
			if tc.include {
				d = append(d, bson.E{Key: "ttlMonitorEnabled", Value: tc.value})
			}
			f.responses[5] = d
			got := probe(context.Background(), fixtureInput(), identityHash(testUUID, []string{"mongo:27017"}), f)
			if got.Observation != tc.known || got.TTL.Known != tc.known || got.TTL.Enabled != tc.enabled || !f.closed {
				t.Fatal("ttl_metadata_unknown_or_disabled_inferred")
			}
		})
	}
	for _, responseErr := range []error{mongo.CommandError{Code: 13}, errors.New("private-read-error")} {
		f := &fakeCommander{responses: fixtureCommands(), errors: map[int]error{5: responseErr}}
		got := probe(context.Background(), fixtureInput(), identityHash(testUUID, []string{"mongo:27017"}), f)
		if got.TTL.Known || got.TTL.Enabled || got.Observation {
			t.Fatal("denied_parameter_inferred_ttl_disabled")
		}
	}
}
func TestMongoObservationFailuresCloseAndNeverElevate(t *testing.T) {
	for _, tc := range []struct {
		name     string
		index    int
		err      error
		category string
	}{
		{"auth_unknown", 1, errors.New("fixture-password-private"), "authorization_unknown"},
		{"permission_denied", 5, mongo.CommandError{Code: 13, Message: "fixture-password-private"}, "fcv_permission_denied"},
		{"other_error", 5, mongo.CommandError{Code: 18, Message: "fixture-password-private"}, "fcv_read_failed"},
		{"final_drift", 6, errors.New("fixture-password-private"), "identity_changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeCommander{responses: fixtureCommands(), errors: map[int]error{tc.index: tc.err}}
			got := probe(context.Background(), fixtureInput(), identityHash(testUUID, []string{"mongo:27017"}), f)
			if got.Observation || got.Error != tc.category || !f.closed || !got.Release.Client {
				t.Fatal("failure_not_closed")
			}
			if tc.index == 5 && len(f.commands) != 10 {
				t.Fatal("failed_fcv_identity_not_rechecked")
			}
			assertPrivate(t, got)
		})
	}
}
func TestMongoWrongTargetAndDirtyStopBeforeParameter(t *testing.T) {
	for _, tc := range []string{"target", "dirty", "multiple_heads", "secondary", "retired"} {
		t.Run(tc, func(t *testing.T) {
			f := &fakeCommander{responses: fixtureCommands()}
			expected := identityHash(testUUID, []string{"mongo:27017"})
			switch tc {
			case "target":
				expected = strings.Repeat("0", 64)
			case "dirty":
				f.responses[2] = cursor(bson.A{bson.D{{Key: "version", Value: int64(39)}, {Key: "dirty", Value: true}}})
			case "multiple_heads":
				f.responses[2] = cursor(bson.A{bson.D{{Key: "version", Value: int64(39)}, {Key: "dirty", Value: false}}, bson.D{{Key: "version", Value: int64(39)}, {Key: "dirty", Value: false}}})
			case "secondary":
				f.responses[0][2].Value = false
			case "retired":
				f.responses[3] = cursor(bson.A{bson.D{{Key: "name", Value: "system.profile"}, {Key: "info", Value: bson.D{{Key: "uuid", Value: testUUID}}}}, bson.D{{Key: "name", Value: "domain_event_outbox"}}})
			}
			got := probe(context.Background(), fixtureInput(), expected, f)
			if got.Observation || got.FCV.Known || !f.closed {
				t.Fatal("invalid_target_accepted")
			}
			for _, c := range f.commands {
				if c[0].Key == "getParameter" {
					t.Fatal("invalid_target_read_parameter")
				}
			}
			assertPrivate(t, got)
		})
	}
}
func TestMongoFCVTransitionAndUnexpectedEncoding(t *testing.T) {
	for _, tc := range []struct {
		name              string
		doc               bson.D
		valid, transition bool
	}{
		{"stable", bson.D{{Key: "version", Value: "7.0"}}, true, false},
		{"target", bson.D{{Key: "version", Value: "6.0"}, {Key: "targetVersion", Value: "7.0"}}, true, true},
		{"previous", bson.D{{Key: "version", Value: "6.0"}, {Key: "previousVersion", Value: "7.0"}}, true, true},
		{"cleanup", bson.D{{Key: "version", Value: "6.0"}, {Key: "isCleaningServerMetadata", Value: true}}, true, true},
		{"timestamp_stable", bson.D{{Key: "version", Value: "7.0"}, {Key: "changeTimestamp", Value: primitive.Timestamp{T: 1, I: 2}}}, true, false},
		{"duplicate", bson.D{{Key: "version", Value: "7.0"}, {Key: "version", Value: "6.0"}}, false, false},
		{"missing", bson.D{}, false, false}, {"number", bson.D{{Key: "version", Value: 7}}, false, false},
		{"noncanonical", bson.D{{Key: "version", Value: "07.0"}}, false, false},
		{"unknown", bson.D{{Key: "version", Value: "7.0"}, {Key: "future", Value: true}}, false, false},
		{"wrong_bool", bson.D{{Key: "version", Value: "7.0"}, {Key: "isCleaningServerMetadata", Value: "false"}}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseFCV(raw(bson.D{{Key: "featureCompatibilityVersion", Value: tc.doc}}))
			if (err == nil) != tc.valid || got.Transition != tc.transition {
				t.Fatal("fcv_shape_semantics_failed")
			}
			if !tc.valid && got.Known {
				t.Fatal("malformed_fcv_known")
			}
		})
	}
}
func TestMongoProcessChangeAndDisconnectFailureCannotComplete(t *testing.T) {
	for _, tc := range []string{"process", "disconnect"} {
		t.Run(tc, func(t *testing.T) {
			f := &fakeCommander{responses: fixtureCommands()}
			if tc == "process" {
				f.responses[6][3].Value = bson.D{{Key: "processId", Value: primitive.NewObjectID()}}
			} else {
				f.closeError = errors.New("fixture-password-private")
			}
			got := probe(context.Background(), fixtureInput(), identityHash(testUUID, []string{"mongo:27017"}), f)
			if got.Observation || !f.closed {
				t.Fatal("release_or_identity_failure_accepted")
			}
			assertPrivate(t, got)
		})
	}
}
func TestMongoStdinInputStrictAndFixed(t *testing.T) {
	in := fixtureInput()
	b, _ := json.Marshal(in)
	if _, err := decodeInput(bytes.NewReader(b)); err != nil {
		t.Fatal("valid_input_failed")
	}
	for _, bad := range [][]byte{append(b, []byte("{}")...), bytes.Replace(b, []byte(`"database":"qs"`), []byte(`"database":"other"`), 1), bytes.Replace(b, []byte(`"port":37018`), []byte(`"port":27017`), 1), bytes.Replace(b, []byte(`"database":"qs"`), []byte(`"database":"qs","database":"qs"`), 1), []byte(strings.Repeat("x", 16385))} {
		if _, err := decodeInput(bytes.NewReader(bad)); err == nil {
			t.Fatal("unsafe_input_accepted")
		}
	}
}

func TestMongoLiveCursorReleaseNeedsSpecificClosedIdentity(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		killed, notFound, alive, unknown bson.A
		closed                           bool
	}{
		{"killed", bson.A{int64(42)}, bson.A{}, bson.A{}, bson.A{}, true},
		{"not_found", bson.A{}, bson.A{int64(42)}, bson.A{}, bson.A{}, true},
		{"unknown_ok1", bson.A{}, bson.A{}, bson.A{}, bson.A{int64(42)}, false},
		{"alive_ok1", bson.A{}, bson.A{}, bson.A{int64(42)}, bson.A{}, false},
		{"unreported_ok1", bson.A{}, bson.A{}, bson.A{}, bson.A{}, false},
		{"different_id", bson.A{int64(43)}, bson.A{}, bson.A{}, bson.A{}, false},
		{"duplicate_identity", bson.A{int64(42)}, bson.A{int64(42)}, bson.A{}, bson.A{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeCommander{responses: []bson.D{{{Key: "ok", Value: 1.0}, {Key: "cursorsKilled", Value: tc.killed}, {Key: "cursorsNotFound", Value: tc.notFound}, {Key: "cursorsAlive", Value: tc.alive}, {Key: "cursorsUnknown", Value: tc.unknown}}}}
			r := reader{c: f, cursorReleased: true}
			_, err := r.batch(context.Background(), raw(bson.D{{Key: "cursor", Value: bson.D{{Key: "id", Value: int64(42)}, {Key: "firstBatch", Value: bson.A{}}}}}), "schema_migrations")
			if err == nil || r.cursorReleased != tc.closed || len(f.commands) != 1 || f.commands[0][0].Key != "killCursors" {
				t.Fatal("cursor_release_identity_overclaimed")
			}
		})
	}
}
