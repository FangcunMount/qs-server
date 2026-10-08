//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const permissionNativeManifest = "/private/tmp/qs-compatibility-retirement/owned-mongo.json"
const permissionInspectFormat = `{"id":{{json .Id}},"name":{{json .Name}},"image":{{json .Image}},"labels":{{json .Config.Labels}},"mounts":{{json .Mounts}},"ports":{{json .NetworkSettings.Ports}},"requested_ports":{{json .HostConfig.PortBindings}},"running":{{json .State.Running}}}`

type permissionContainer struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Image  string            `json:"image"`
	Labels map[string]string `json:"labels"`
	Mounts []struct {
		Type        string
		Name        string
		Source      string
		Destination string
		RW          bool
	} `json:"mounts"`
	Ports          map[string][]struct{ HostIP, HostPort string } `json:"ports"`
	RequestedPorts map[string][]struct{ HostIP, HostPort string } `json:"requested_ports"`
	Running        bool                                           `json:"running"`
}

type permissionRootFixture struct {
	ContainerID   string   `json:"container_id"`
	ContainerName string   `json:"container_name"`
	ImageID       string   `json:"image_id"`
	Volumes       []string `json:"volumes"`
	Ready         bool     `json:"ready"`
}

func permissionDocker(ctx context.Context, input []byte, args ...string) ([]byte, error) {
	bounded, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(bounded, "docker", args...)
	cmd.Stdin = bytes.NewReader(input)
	// Only fixed Docker projections are returned. stderr and every command's
	// raw output stay private and never become test/error/receipt text.
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, errors.New("owned_docker_command_failed")
	}
	if stdout.Len() > 256*1024 || stderr.Len() > 256*1024 {
		return nil, errors.New("owned_docker_response_budget_exceeded")
	}
	return stdout.Bytes(), nil
}

func permissionInspect(ctx context.Context, identifier string) (permissionContainer, error) {
	var result permissionContainer
	raw, err := permissionDocker(ctx, nil, "inspect", "--format", permissionInspectFormat, identifier)
	if err != nil || json.Unmarshal(raw, &result) != nil {
		return result, errors.New("owned_container_inspection_failed")
	}
	return result, nil
}

func permissionNativeRoot(t *testing.T) permissionContainer {
	t.Helper()
	if os.Getenv("QS_RETIREMENT_ANCHOR_PERMISSION_NATIVE") != "1" {
		if os.Getenv("QS_RETIREMENT_ANCHOR_PERMISSION_NATIVE_REQUIRED") == "1" {
			t.Fatal("permission_native_fixture_opt_in_missing")
		}
		t.Skip("owned permission/topology native fixture not requested")
	}
	raw, err := os.ReadFile(permissionNativeManifest)
	var expected permissionRootFixture
	if err != nil || json.Unmarshal(raw, &expected) != nil || !expected.Ready || len(expected.ContainerID) != 64 || !strings.HasPrefix(expected.ImageID, "sha256:") {
		t.Fatal("owned_root_fixture_manifest_rejected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	actual, err := permissionInspect(ctx, expected.ContainerID)
	if err != nil || actual.ID != expected.ContainerID || strings.TrimPrefix(actual.Name, "/") != expected.ContainerName || actual.Image != expected.ImageID || !actual.Running || actual.Labels["codex.task"] == "" {
		t.Fatal("owned_root_fixture_identity_rejected")
	}
	ports := actual.Ports["27017/tcp"]
	if len(actual.Ports) != 1 || len(ports) != 1 || ports[0].HostIP != "127.0.0.1" || ports[0].HostPort != "33317" {
		t.Fatal("owned_root_fixture_loopback_rejected")
	}
	seen := make(map[string]bool)
	for _, mount := range actual.Mounts {
		if mount.Type != "volume" {
			t.Fatal("owned_root_fixture_mount_rejected")
		}
		seen[mount.Name] = true
	}
	if len(seen) != len(expected.Volumes) {
		t.Fatal("owned_root_fixture_volume_rejected")
	}
	for _, volume := range expected.Volumes {
		if !seen[volume] {
			t.Fatal("owned_root_fixture_volume_rejected")
		}
	}
	if _, err := permissionDocker(ctx, nil, "image", "inspect", "--format", "{{.Id}}", actual.Image); err != nil {
		t.Fatal("cached_owned_image_unavailable")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		after, err := permissionInspect(ctx, actual.ID)
		if err != nil || !reflect.DeepEqual(after, actual) {
			t.Error("shared_root_fixture_changed")
		}
	})
	return actual
}

type permissionOwnedFixture struct {
	Name, ID, Image, Owner, Port, Replica, Keyfile, Manifest string
	Labels                                                   map[string]string
	ContainerLabels                                          map[string]string
	Volumes                                                  []string
}

func permissionPrivateJSON(path string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return errors.New("owned_manifest_encode_failed")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("owned_manifest_create_failed")
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		return errors.New("owned_manifest_write_failed")
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return errors.New("owned_manifest_sync_failed")
	}
	if err := f.Close(); err != nil {
		return errors.New("owned_manifest_close_failed")
	}
	return nil
}

func (f *permissionOwnedFixture) check(ctx context.Context, requireRunning bool) error {
	identifier := f.ID
	if identifier == "" {
		identifier = f.Name
	}
	actual, err := permissionInspect(ctx, identifier)
	if err != nil {
		return err
	}
	if (f.ID != "" && actual.ID != f.ID) || strings.TrimPrefix(actual.Name, "/") != f.Name || actual.Image != f.Image || len(actual.ID) != 64 || (requireRunning && !actual.Running) || !reflect.DeepEqual(actual.Labels, f.ContainerLabels) {
		return errors.New("owned_native_container_identity_rejected")
	}
	for key, value := range f.Labels {
		if actual.Labels[key] != value {
			return errors.New("owned_native_label_rejected")
		}
	}
	requested := actual.RequestedPorts["27017/tcp"]
	if len(actual.RequestedPorts) != 1 || len(requested) != 1 || requested[0].HostIP != "127.0.0.1" || (f.Port != "" && requested[0].HostPort != "" && requested[0].HostPort != f.Port) {
		return errors.New("owned_native_loopback_rejected")
	}
	ports := actual.Ports["27017/tcp"]
	if actual.Running {
		if len(actual.Ports) != 1 || len(ports) != 1 || ports[0].HostIP != "127.0.0.1" {
			return errors.New("owned_native_loopback_rejected")
		}
		port, err := strconv.Atoi(ports[0].HostPort)
		if err != nil || port < 1024 || port > 65535 || (f.Port != "" && f.Port != ports[0].HostPort) {
			return errors.New("owned_native_port_rejected")
		}
		f.Port = ports[0].HostPort
	}
	expectedMounts := map[string]string{f.Volumes[0]: "/data/db", f.Volumes[1]: "/data/configdb"}
	if f.Keyfile != "" {
		expectedMounts[f.Volumes[2]] = "/run/qs-anchor-keyfiles"
	}
	seen := make(map[string]bool)
	for _, mount := range actual.Mounts {
		if mount.Type != "volume" || !mount.RW || expectedMounts[mount.Name] != mount.Destination || seen[mount.Name] {
			return errors.New("owned_native_mount_rejected")
		}
		seen[mount.Name] = true
	}
	if len(seen) != len(f.Volumes) || len(actual.Mounts) != len(f.Volumes) {
		return errors.New("owned_native_volume_rejected")
	}
	for _, volume := range f.Volumes {
		if !seen[volume] {
			return errors.New("owned_native_volume_rejected")
		}
	}
	f.ID = actual.ID
	return nil
}

func permissionOwnedMongo(t *testing.T, root permissionContainer, authReplica bool) *permissionOwnedFixture {
	t.Helper()
	owner := primitive.NewObjectID().Hex()
	dir, err := os.MkdirTemp("/private/tmp/qs-compatibility-retirement", "anchor-permission-owned-")
	if err != nil {
		t.Fatal("owned_native_private_directory_failed")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal("owned_native_private_directory_failed")
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal("owned_native_private_directory_identity_failed")
	}
	f := &permissionOwnedFixture{Name: "qs-anchor-permission-" + owner, Image: root.Image, Owner: owner,
		Labels:  map[string]string{"codex.task": "qs-anchor-permission-native", "codex.owner": owner},
		Volumes: []string{"qs-anchor-db-" + owner, "qs-anchor-config-" + owner}}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	imageLabels, err := permissionDocker(ctx, nil, "image", "inspect", "--format", "{{json .Config.Labels}}", root.Image)
	if err != nil || json.Unmarshal(imageLabels, &f.ContainerLabels) != nil {
		t.Fatal("owned_native_image_labels_unavailable")
	}
	if f.ContainerLabels == nil {
		f.ContainerLabels = make(map[string]string)
	}
	for key, value := range f.Labels {
		f.ContainerLabels[key] = value
	}
	if authReplica {
		f.Replica, f.Keyfile = "qsanchor_"+owner, filepath.Join(dir, "replica.key")
		f.Volumes = append(f.Volumes, "qs-anchor-key-"+owner)
		key := make([]byte, 48)
		if _, err := rand.Read(key); err != nil || os.WriteFile(f.Keyfile, []byte(base64.StdEncoding.EncodeToString(key)), 0600) != nil {
			t.Fatal("owned_native_keyfile_failed")
		}
	}
	f.Manifest = filepath.Join(dir, "requested.json")
	if err := permissionPrivateJSON(f.Manifest, f); err != nil {
		t.Fatal("owned_native_registration_failed")
	}
	// Registered before creation: uncertain Docker outcomes are looked up only
	// by this unique name, then checked against exact ownership before removal.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		cleaned := f.cleanup(ctx) == nil
		if !cleaned {
			t.Error("owned_native_resource_cleanup_failed")
		}
		privateCleaned := false
		if cleaned {
			privateCleaned = os.RemoveAll(dir) == nil
			_, statErr := os.Stat(dir)
			privateCleaned = privateCleaned && errors.Is(statErr, os.ErrNotExist)
		}
		if !privateCleaned {
			t.Error("owned_native_private_material_cleanup_failed")
		}
		if cleaned && privateCleaned {
			t.Log("owned_container_remaining=0 owned_volumes_remaining=0 keyfile_and_credentials_removed=true")
		}
	})
	for _, volume := range f.Volumes {
		if _, err := permissionDocker(ctx, nil, "volume", "create", "--label", "codex.task="+f.Labels["codex.task"], "--label", "codex.owner="+owner, volume); err != nil {
			t.Fatal("owned_native_volume_create_failed")
		}
	}
	args := []string{"run", "--detach", "--name", f.Name, "--label", "codex.task=" + f.Labels["codex.task"], "--label", "codex.owner=" + owner, "--publish", "127.0.0.1::27017", "--mount", "type=volume,src=" + f.Volumes[0] + ",dst=/data/db", "--mount", "type=volume,src=" + f.Volumes[1] + ",dst=/data/configdb", "--user", "0"}
	if authReplica {
		// Key bytes enter only this owned volume through stdin. A fixed shell
		// waits for the completed file, then becomes mongod; no host bind or
		// credential argument is used, and no additional container is created.
		waitForKey := "while [ ! -f /run/qs-anchor-keyfiles/ready ]; do sleep 0.1; done; exec mongod --bind_ip_all --replSet " + f.Replica + " --auth --keyFile /run/qs-anchor-keyfiles/keyfile"
		args = append(args, "--mount", "type=volume,src="+f.Volumes[2]+",dst=/run/qs-anchor-keyfiles", "--entrypoint", "sh", f.Image, "-c", waitForKey)
	} else {
		args = append(args, "--entrypoint", "mongod", f.Image, "--bind_ip_all")
	}
	if _, err := permissionDocker(ctx, nil, args...); err != nil {
		t.Fatal("owned_native_container_start_failed")
	}
	if err := f.check(ctx, true); err != nil {
		t.Fatal("owned_native_container_binding_failed")
	}
	if authReplica {
		key, err := os.ReadFile(f.Keyfile)
		if err != nil {
			t.Fatal("owned_native_keyfile_read_failed")
		}
		writeKey := "umask 077; cat > /run/qs-anchor-keyfiles/keyfile && chmod 400 /run/qs-anchor-keyfiles/keyfile && touch /run/qs-anchor-keyfiles/ready"
		if _, err := permissionDocker(ctx, key, "exec", "-i", f.ID, "sh", "-c", writeKey); err != nil {
			t.Fatal("owned_native_keyfile_install_failed")
		}
	}
	if err := permissionPrivateJSON(filepath.Join(dir, "created.json"), f); err != nil {
		t.Fatal("owned_native_created_registration_failed")
	}
	return f
}

func (f *permissionOwnedFixture) cleanup(ctx context.Context) error {
	var result error
	if err := f.check(ctx, false); err == nil {
		if _, err := permissionDocker(ctx, nil, "rm", "--force", f.ID); err != nil {
			result = errors.New("owned_native_container_remove_failed")
		}
	} else {
		// A failed inspect is not proof of absence. An exact filtered inventory
		// must show zero before any missing container is considered cleaned.
		raw, e := permissionDocker(ctx, nil, "ps", "--all", "--filter", "name=^/"+f.Name+"$", "--format", "{{.ID}}")
		if e != nil || strings.TrimSpace(string(raw)) != "" {
			return errors.New("owned_native_cleanup_identity_unproven")
		}
	}
	for _, volume := range f.Volumes {
		raw, err := permissionDocker(ctx, nil, "volume", "inspect", "--format", "{{json .Labels}}", volume)
		if err != nil {
			remaining, e := permissionDocker(ctx, nil, "volume", "ls", "--filter", "name=^"+volume+"$", "--format", "{{.Name}}")
			if e != nil || strings.TrimSpace(string(remaining)) != "" {
				result = errors.New("owned_native_volume_absence_unproven")
			}
			continue
		}
		var labels map[string]string
		if json.Unmarshal(raw, &labels) != nil || !reflect.DeepEqual(labels, f.Labels) {
			result = errors.New("owned_native_volume_ownership_rejected")
			continue
		}
		if _, err := permissionDocker(ctx, nil, "volume", "rm", volume); err != nil {
			result = errors.New("owned_native_volume_remove_failed")
		}
	}
	containers, err := permissionDocker(ctx, nil, "ps", "--all", "--filter", "label=codex.owner="+f.Owner, "--format", "{{.ID}}")
	volumes, ve := permissionDocker(ctx, nil, "volume", "ls", "--filter", "label=codex.owner="+f.Owner, "--format", "{{.Name}}")
	if err != nil || ve != nil || strings.TrimSpace(string(containers)) != "" || strings.TrimSpace(string(volumes)) != "" {
		return errors.New("owned_native_resources_remaining")
	}
	return result
}

func permissionConnect(t *testing.T, port, replica, user, password, authDB string) *mongo.Client {
	t.Helper()
	opts := options.Client().SetHosts([]string{"127.0.0.1:" + port}).SetDirect(true).SetConnectTimeout(5 * time.Second).SetServerSelectionTimeout(5 * time.Second)
	if replica != "" {
		opts.SetReplicaSet(replica)
	}
	if user != "" {
		opts.SetAuth(options.Credential{Username: user, Password: password, AuthSource: authDB, AuthMechanism: "SCRAM-SHA-256"})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		t.Fatal("owned_native_mongo_connect_failed")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.Disconnect(ctx); err != nil {
			t.Error("owned_native_mongo_disconnect_failed")
		}
	})
	return client
}

func permissionHello(t *testing.T, client *mongo.Client, wantReplica bool) bson.Raw {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		var hello bson.Raw
		err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello)
		cancel()
		if err == nil {
			writable, _ := hello.Lookup("isWritablePrimary").BooleanOK()
			if writable && (hello.Lookup("setName").Type != 0) == wantReplica {
				return hello
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("owned_native_topology_not_ready")
	return nil
}

func permissionActualCode(t *testing.T, err error, expected int32) {
	t.Helper()
	var command mongo.CommandError
	if !errors.As(err, &command) || command.Code != expected {
		t.Fatal("actual_native_server_code_not_observed")
	}
}

func TestMongoAnchorPermissionNativeActualUnauthorized13(t *testing.T) {
	root := permissionNativeRoot(t)
	f := permissionOwnedMongo(t, root, true)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	// Localhost exception is used only inside this new uninitialized container.
	// No auth configuration or users are changed on the shared root fixture.
	init := `const v=JSON.parse(require("fs").readFileSync(0,"utf8"));const r=db.getSiblingDB("admin").runCommand({replSetInitiate:{_id:v.replica,members:[{_id:0,host:"127.0.0.1:27017"}]}});if(r.ok!==1){quit(2);}`
	input, _ := json.Marshal(map[string]string{"replica": f.Replica})
	for attempts := 0; ; attempts++ {
		if _, err := permissionDocker(ctx, input, "exec", "-i", f.ID, "mongosh", "--quiet", "--norc", "--eval", init); err == nil {
			break
		}
		if attempts >= 30 {
			t.Fatal("owned_auth_replica_initiate_failed")
		}
		time.Sleep(200 * time.Millisecond)
	}
	initial := permissionConnect(t, f.Port, f.Replica, "", "", "")
	permissionHello(t, initial, true)
	adminUser := "admin_" + primitive.NewObjectID().Hex()
	password := make([]byte, 32)
	if _, err := rand.Read(password); err != nil {
		t.Fatal("owned_admin_credentials_failed")
	}
	adminPassword := base64.RawURLEncoding.EncodeToString(password)
	credentials := map[string]string{"user": adminUser, "password": adminPassword}
	if err := permissionPrivateJSON(filepath.Join(filepath.Dir(f.Manifest), "credentials.json"), credentials); err != nil {
		t.Fatal("owned_credentials_registration_failed")
	}
	create := `const v=JSON.parse(require("fs").readFileSync(0,"utf8"));const r=db.getSiblingDB("admin").runCommand({createUser:v.user,pwd:v.password,roles:[{role:"root",db:"admin"}],mechanisms:["SCRAM-SHA-256"]});if(r.ok!==1){quit(2);}`
	input, _ = json.Marshal(credentials)
	if _, err := permissionDocker(ctx, input, "exec", "-i", f.ID, "mongosh", "--quiet", "--norc", "--eval", create); err != nil {
		t.Fatal("owned_admin_create_failed")
	}
	admin := permissionConnect(t, f.Port, f.Replica, adminUser, adminPassword, "admin")
	hello := permissionHello(t, admin, true)
	var build bson.Raw
	if err := admin.Database("admin").RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&build); err != nil {
		t.Fatal("owned_native_version_read_failed")
	}
	version, _ := build.Lookup("version").StringValueOK()
	if !strings.HasPrefix(version, "7.") {
		t.Fatal("owned_native_mongo7_required")
	}
	name := "qs_anchor_permission_" + primitive.NewObjectID().Hex()
	db := admin.Database(name)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := db.Drop(ctx); err != nil {
			t.Error("owned_native_database_cleanup_failed")
			return
		}
		names, err := admin.ListDatabaseNames(ctx, bson.D{{Key: "name", Value: name}})
		if err != nil || len(names) != 0 {
			t.Error("owned_native_database_absence_unproven")
			return
		}
		t.Log("owned_database_remaining=0")
	})
	if _, err := db.Collection("sentinel").InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "value", Value: "retained"}}); err != nil {
		t.Fatal("owned_native_fact_setup_failed")
	}
	anchor, err := mongoDatabaseAnchor(ctx, db, hello)
	if err != nil || !hashRE.MatchString(anchor) {
		t.Fatal("actual_admin_stable_anchor_failed")
	}
	user := "reader_" + primitive.NewObjectID().Hex()
	if _, err := rand.Read(password); err != nil {
		t.Fatal("owned_reader_credentials_failed")
	}
	readerPassword := base64.RawURLEncoding.EncodeToString(password)
	if err := db.RunCommand(ctx, bson.D{{Key: "createUser", Value: user}, {Key: "pwd", Value: readerPassword}, {Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "read"}, {Key: "db", Value: name}}}}, {Key: "mechanisms", Value: bson.A{"SCRAM-SHA-256"}}}).Err(); err != nil {
		t.Fatal("actual_admin_test_user_create_failed")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := db.RunCommand(ctx, bson.D{{Key: "dropUser", Value: user}}).Err(); err != nil {
			t.Error("owned_native_user_cleanup_failed")
			return
		}
		var result struct {
			Users []bson.Raw `bson:"users"`
		}
		if err := db.RunCommand(ctx, bson.D{{Key: "usersInfo", Value: user}}).Decode(&result); err != nil || len(result.Users) != 0 {
			t.Error("owned_native_user_absence_unproven")
			return
		}
		t.Log("owned_restricted_user_remaining=0")
	})
	reader := permissionConnect(t, f.Port, f.Replica, user, readerPassword, name)
	limitedHello := permissionHello(t, reader, true)
	if err := reader.Database(name).Collection("sentinel").FindOne(ctx, bson.D{{Key: "_id", Value: 1}}).Err(); err != nil {
		t.Fatal("actual_reader_authentication_not_proven")
	}
	var forbidden bson.Raw
	err = reader.Database("admin").RunCommand(ctx, bson.D{{Key: "replSetGetConfig", Value: 1}}).Decode(&forbidden)
	permissionActualCode(t, err, 13)
	denied, err := mongoDatabaseAnchor(ctx, reader.Database(name), limitedHello)
	if err == nil || err.Error() != "mongo_replica_anchor_not_authorized" || denied != "" {
		t.Fatal("actual_native_13_not_classified_without_anchor")
	}
	after, err := mongoDatabaseAnchor(ctx, db, hello)
	if err != nil || after != anchor {
		t.Fatal("restricted_read_changed_admin_anchor")
	}
	t.Log("actual_server_code=13 category=mongo_replica_anchor_not_authorized admin_anchor_verified=true denied_anchor_empty=true")
}

func TestMongoAnchorPermissionNativeActualStandalone76(t *testing.T) {
	root := permissionNativeRoot(t)
	f := permissionOwnedMongo(t, root, false)
	client := permissionConnect(t, f.Port, "", "", "", "")
	hello := permissionHello(t, client, false)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var build bson.Raw
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&build); err != nil {
		t.Fatal("owned_native_version_read_failed")
	}
	version, _ := build.Lookup("version").StringValueOK()
	if !strings.HasPrefix(version, "7.") {
		t.Fatal("owned_native_mongo7_required")
	}
	var config bson.Raw
	err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "replSetGetConfig", Value: 1}}).Decode(&config)
	permissionActualCode(t, err, 76)
	anchor, err := mongoDatabaseAnchor(ctx, client.Database("qs_anchor_permission_"+primitive.NewObjectID().Hex()), hello)
	if err == nil || err.Error() != "mongo_replica_anchor_replication_not_enabled" || anchor != "" {
		t.Fatal("actual_native_76_not_classified_without_anchor")
	}
	t.Log("actual_server_code=76 category=mongo_replica_anchor_replication_not_enabled standalone_hello_verified=true denied_anchor_empty=true")
}
