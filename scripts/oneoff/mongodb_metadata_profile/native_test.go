//go:build integration

package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const nativeIndex = "mongo@sha256:43fddee7e532a920f3dfdee9e8f4834398c155c26bcb92d790cc1cd3c630fc40"
const nativeARM = "mongo@sha256:eae7853617a083d7f9d0ceb5ff2964165b828efc72101d173662ff3a12ce1692"
const nativeAMD = "mongo@sha256:a8f480ae7d56f04302f9eb2e00ac2e41f7bbb7ff040730bbdc3a008d6b1fd50c"

func ownedNativeInput(t *testing.T) input {
	t.Helper()
	owner := os.Getenv("QS_MONGO_METADATA_NATIVE_OWNER")
	name := os.Getenv("QS_MONGO_METADATA_NATIVE_CONTAINER")
	port, err := strconv.Atoi(os.Getenv("QS_MONGO_METADATA_NATIVE_PORT"))
	pw := os.Getenv("QS_MONGO_METADATA_NATIVE_PASSWORD")
	if os.Getenv("QS_MONGO_METADATA_NATIVE_REQUIRED") != "1" || !regexp.MustCompile(`^[a-z0-9]{8,24}$`).MatchString(owner) || name != "qs-mongo-profile-owned-"+owner || err != nil || port < 1024 || port > 65535 || port == 27017 || len(pw) < 12 || len(pw) > 4096 {
		t.Fatal("owned_native_input_guard_failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, "docker", "inspect", name).Output()
	var inspect []struct {
		Name   string
		Config struct {
			Image  string
			Labels map[string]string
		}
		State           struct{ Running bool }
		NetworkSettings struct {
			Ports    map[string][]struct{ HostIp, HostPort string }
			Networks map[string]struct {
				NetworkID string
				Aliases   []string
			}
		}
	}
	if err != nil || json.Unmarshal(data, &inspect) != nil || len(inspect) != 1 || inspect[0].Name != "/"+name || !inspect[0].State.Running || inspect[0].Config.Labels["qs.mongo-metadata.fixture"] != owner {
		t.Fatal("owned_native_container_guard_failed")
	}
	v := inspect[0]
	if v.Config.Image != nativeIndex && v.Config.Image != nativeARM && v.Config.Image != nativeAMD {
		t.Fatal("owned_native_image_guard_failed")
	}
	ports := v.NetworkSettings.Ports["27017/tcp"]
	if len(ports) != 1 || ports[0].HostIp != "127.0.0.1" || ports[0].HostPort != strconv.Itoa(port) || len(v.NetworkSettings.Networks) != 1 {
		t.Fatal("owned_native_endpoint_guard_failed")
	}
	for name, n := range v.NetworkSettings.Networks {
		if !strings.HasPrefix(name, "qs-mongo-profile-") {
			t.Fatal("owned_native_network_guard_failed")
		}
		found := false
		for _, a := range n.Aliases {
			if a == "mongo" {
				found = true
			}
		}
		if !found {
			t.Fatal("owned_native_alias_guard_failed")
		}
		data, err = exec.CommandContext(ctx, "docker", "network", "inspect", "--format", "{{json .Labels}}", n.NetworkID).Output()
		var labels map[string]string
		if err != nil || json.Unmarshal(data, &labels) != nil || labels["qs.mongo-metadata.fixture"] != owner {
			t.Fatal("owned_native_network_owner_failed")
		}
	}
	return input{SourceSHA: strings.Repeat("a", 40), RunID: "12345-1", Expected: expectedTarget, SourceBinding: strings.Repeat("b", 64), ToolHash: strings.Repeat("c", 64), Port: port, Account: "metadata_fixture", Password: pw, Database: "qs"}
}
func nativeCommand(t *testing.T, c commander, db string, d bson.D) bson.Raw {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := c.Command(ctx, db, d)
	if err != nil {
		t.Fatal("owned_native_fixture_command_failed")
	}
	return raw
}

type nativeHook struct {
	commander
	hook func()
	once bool
}

func (c *nativeHook) Command(ctx context.Context, db string, d bson.D) (bson.Raw, error) {
	raw, err := c.commander.Command(ctx, db, d)
	if d[0].Key == "getParameter" && !c.once {
		c.once = true
		c.hook()
	}
	return raw, err
}

// All setup writes are guarded by an independently owned Docker endpoint. The
// production stdin entry point cannot substitute this fixture's target hash.
func TestMongoMetadataProfileNativeOwnedReplicaSet(t *testing.T) {
	in := ownedNativeInput(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := connect(ctx, in)
	if err != nil {
		t.Fatal("owned_native_admin_connect_failed")
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if admin.Close(c) != nil {
			t.Error("owned_native_admin_release_failed")
		}
	}()
	hello := nativeCommand(t, admin, "admin", bson.D{{Key: "hello", Value: 1}})
	build := nativeCommand(t, admin, "admin", bson.D{{Key: "buildInfo", Value: 1}})
	set, ok := hello.Lookup("setName").StringValueOK()
	version, vok := build.Lookup("version").StringValueOK()
	hosts, hOk := hello.Lookup("hosts").ArrayOK()
	values, hErr := hosts.Values()
	if !ok || set != "rs0" || !vok || version != "7.0.31" || !hOk || hErr != nil || len(values) != 1 || values[0].StringValue() != "mongo:27017" {
		t.Fatal("owned_native_server_identity_failed")
	}
	// Only the owned fixture is mutated to create the minimal catalog/head.
	nativeCommand(t, admin, "qs", bson.D{{Key: "create", Value: "system.profile"}, {Key: "capped", Value: true}, {Key: "size", Value: 1048576}})
	nativeCommand(t, admin, "qs", bson.D{{Key: "insert", Value: "schema_migrations"}, {Key: "documents", Value: bson.A{bson.D{{Key: "_id", Value: "fixture-head"}, {Key: "version", Value: int64(39)}, {Key: "dirty", Value: false}}}}})
	nativeCommand(t, admin, "admin", bson.D{{Key: "createUser", Value: "metadata_readonly"}, {Key: "pwd", Value: in.Password}, {Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "read"}, {Key: "db", Value: "qs"}}}}})
	raw := nativeCommand(t, admin, "qs", bson.D{{Key: "listCollections", Value: 1}, {Key: "filter", Value: bson.D{{Key: "name", Value: "system.profile"}}}, {Key: "cursor", Value: bson.D{{Key: "batchSize", Value: 2}}}})
	r := reader{c: admin, cursorReleased: true}
	docs, err := r.batch(ctx, raw, "$cmd.listCollections")
	if err != nil || len(docs) != 1 {
		t.Fatal("owned_native_catalog_failed")
	}
	subtype, data, ok := docs[0].Lookup("info", "uuid").BinaryOK()
	if !ok || subtype != 4 || len(data) != 16 {
		t.Fatal("owned_native_catalog_type_failed")
	}
	expected := identityHash(primitive.Binary{Subtype: subtype, Data: data}, []string{"mongo:27017"})
	in.Expected = expected
	ttlRaw := nativeCommand(t, admin, "admin", bson.D{{Key: "getParameter", Value: 1}, {Key: "ttlMonitorEnabled", Value: 1}})
	actualTTL, ttlValid := ttlRaw.Lookup("ttlMonitorEnabled").BooleanOK()
	if !ttlValid {
		t.Fatal("owned_native_ttl_metadata_not_boolean")
	}
	probeOne := func(in input, expected string, hook func()) receipt {
		t.Helper()
		client, err := connect(ctx, in)
		if err != nil {
			t.Fatal("owned_native_probe_connect_failed")
		}
		if hook != nil {
			client = &nativeHook{commander: client, hook: hook}
		}
		got := probe(ctx, in, expected, client)
		if !got.Release.Client || !got.Release.Cursors {
			t.Fatal("owned_native_probe_release_failed")
		}
		encoded, _ := json.Marshal(got)
		if strings.Contains(string(encoded), in.Password) || strings.Contains(string(encoded), in.Account) {
			t.Fatal("owned_native_private_output_leaked")
		}
		return got
	}
	t.Run("same_patch_authenticated_actual_fcv", func(t *testing.T) {
		got := probeOne(in, expected, nil)
		if !got.Observation || got.Complete || got.Eligible || got.Error != "none" || !got.FCV.Known || got.FCV.Major != 7 || got.FCV.Minor != 0 || got.Budget.Queries != 10 || !got.TTL.Known || got.TTL.Enabled != actualTTL {
			t.Fatal("native_actual_fcv_observation_failed")
		}
	})
	t.Run("actual_restricted_permission_denied", func(t *testing.T) {
		limited := in
		limited.Account = "metadata_readonly"
		got := probeOne(limited, expected, nil)
		if got.Observation || got.Error != "fcv_permission_denied" || got.FCV.Known || got.TTL.Known || got.TTL.Enabled || got.Budget.Queries != 10 || got.Authorization.Parameter == nil || *got.Authorization.Parameter {
			t.Fatal("native_unauthorized_did_not_remain_unknown")
		}
	})
	t.Run("same_named_other_target_rejected_before_fcv", func(t *testing.T) {
		got := probeOne(in, strings.Repeat("0", 64), nil)
		if got.Observation || got.FCV.Known || got.Error != "target_identity_mismatch" || got.Budget.Queries != 4 {
			t.Fatal("native_wrong_target_not_stopped")
		}
	})
	t.Run("dirty_rejected_before_fcv", func(t *testing.T) {
		nativeCommand(t, admin, "qs", bson.D{{Key: "update", Value: "schema_migrations"}, {Key: "updates", Value: bson.A{bson.D{{Key: "q", Value: bson.D{}}, {Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "dirty", Value: true}}}}}}}}})
		got := probeOne(in, expected, nil)
		nativeCommand(t, admin, "qs", bson.D{{Key: "update", Value: "schema_migrations"}, {Key: "updates", Value: bson.A{bson.D{{Key: "q", Value: bson.D{}}, {Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "dirty", Value: false}}}}}}}}})
		if got.Observation || got.FCV.Known || got.Error != "migration_state_invalid" || got.Budget.Queries != 3 {
			t.Fatal("native_dirty_not_stopped")
		}
	})
	t.Run("retired_namespace_reappearance_rejected", func(t *testing.T) {
		nativeCommand(t, admin, "qs", bson.D{{Key: "create", Value: "domain_event_outbox"}})
		got := probeOne(in, expected, nil)
		nativeCommand(t, admin, "qs", bson.D{{Key: "drop", Value: "domain_event_outbox"}})
		if got.Observation || got.FCV.Known || got.Error != "target_identity_mismatch" {
			t.Fatal("native_retired_object_not_stopped")
		}
	})
	t.Run("catalog_change_after_fcv_rejected", func(t *testing.T) {
		got := probeOne(in, expected, func() {
			nativeCommand(t, admin, "qs", bson.D{{Key: "drop", Value: "system.profile"}})
			nativeCommand(t, admin, "qs", bson.D{{Key: "create", Value: "system.profile"}, {Key: "capped", Value: true}, {Key: "size", Value: 1048576}})
		})
		if got.Observation || got.Error != "identity_changed" || !got.FCV.Known || got.Budget.Queries != 10 {
			t.Fatal("native_catalog_drift_not_stopped")
		}
	})
}
