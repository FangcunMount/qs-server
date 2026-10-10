package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// Image IDs/architecture are expected facts inside the existing hashed private
// request, obtained from actual runtime inspection. Tags/defaults are forbidden.
type lifecycleRestoreEngines struct {
	MySQLImageID string `json:"mysql_image_id"`
	MongoImageID string `json:"mongodb_image_id"`
	Architecture string `json:"architecture"`
}

func (e *lifecycleRestoreEngines) valid() bool {
	return e != nil && strings.HasPrefix(e.MySQLImageID, "sha256:") && hashRE.MatchString(strings.TrimPrefix(e.MySQLImageID, "sha256:")) && strings.HasPrefix(e.MongoImageID, "sha256:") && hashRE.MatchString(strings.TrimPrefix(e.MongoImageID, "sha256:")) && (e.Architecture == "amd64" || e.Architecture == "arm64") && e.Architecture == runtime.GOARCH
}

type lifecycleOwnedEngine struct {
	wireClosed                                                       bool
	wireMu                                                           sync.Mutex
	wires                                                            []*lifecycleWireConn
	sessionContext                                                   context.Context
	ID, Name, ImageID, Kind, Tool, Archive, Namespace, Owner, Docker string
	Volumes                                                          []string
	Labels, ContainerLabels                                          map[string]string
	materialRecords                                                  map[string]string
}
type lifecycleEngineInspection struct {
	ID         string `json:"Id"`
	Name       string
	Image      string
	Config     struct{ Labels map[string]string }
	HostConfig struct {
		NetworkMode  string
		PortBindings map[string][]struct{ HostIP, HostPort string }
	}
	State           struct{ Running bool }
	NetworkSettings struct {
		Ports map[string][]struct{ HostIP, HostPort string }
	}
	ExecIDs json.RawMessage
	Mounts  []struct {
		Type, Name, Source, Destination string
		RW                              bool
	}
}

func lifecycleDockerExecutable() (string, error) {
	p, e := exec.LookPath("docker")
	if e != nil {
		return "", lifecycleError("lifecycle_docker_unavailable")
	}
	p, e = filepath.EvalSymlinks(p)
	if e != nil {
		return "", lifecycleError("lifecycle_docker_unavailable")
	}
	f, e := os.Stat(p)
	st, ok := infoStat(f)
	if e != nil || f == nil || !f.Mode().IsRegular() || f.Mode().Perm()&022 != 0 || !ok || st.Uid != 0 {
		return "", lifecycleError("lifecycle_docker_executable_rejected")
	}
	return p, nil
}
func lifecycleDockerSocket() error {
	info, e := os.Lstat("/run/docker.sock")
	st, ok := infoStat(info)
	parent, pErr := os.Lstat("/run")
	if e != nil || info == nil || info.Mode()&os.ModeSocket == 0 || !ok || st.Uid != 0 || st.Nlink != 1 || pErr != nil || parent == nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 || parent.Mode().Perm()&022 != 0 {
		return lifecycleError("lifecycle_fixed_local_docker_socket_unproven")
	}
	return nil
}
func lifecycleDocker(ctx context.Context, p string, args ...string) ([]byte, error) {
	if lifecycleDockerSocket() != nil {
		return nil, lifecycleError("lifecycle_fixed_local_docker_socket_unproven")
	}
	q, c := context.WithTimeout(ctx, 60*time.Second)
	defer c()
	cmd := exec.CommandContext(q, p, append([]string{"--host", "unix:///run/docker.sock"}, args...)...)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if cmd.Run() != nil || out.Len() > 4<<20 {
		return nil, lifecycleError("lifecycle_owned_engine_command_failed")
	}
	return out.Bytes(), nil
}
func (e *lifecycleOwnedEngine) check(ctx context.Context, running bool, noExec ...bool) error {
	id := e.ID
	if id == "" {
		id = e.Name
	}
	raw, err := lifecycleDocker(ctx, e.Docker, "inspect", id)
	var values []lifecycleEngineInspection
	if err != nil || json.Unmarshal(raw, &values) != nil || len(values) != 1 {
		return lifecycleError("lifecycle_restore_runtime_unproven")
	}
	v := values[0]
	if len(noExec) > 0 && noExec[0] {
		var actual []string
		if len(v.ExecIDs) == 0 || json.Unmarshal(v.ExecIDs, &actual) != nil || len(actual) != 0 {
			return lifecycleError("lifecycle_restore_exec_remaining_or_unknown")
		}
	}
	if !hashRE.MatchString(v.ID) || (e.ID != "" && v.ID != e.ID) || strings.TrimPrefix(v.Name, "/") != e.Name || v.Image != e.ImageID || v.HostConfig.NetworkMode != "none" || (running && !v.State.Running) || len(v.HostConfig.PortBindings) != 0 || !reflect.DeepEqual(v.Config.Labels, e.ContainerLabels) {
		return lifecycleError("lifecycle_restore_runtime_binding_rejected")
	}
	for _, p := range v.NetworkSettings.Ports {
		if len(p) != 0 {
			return lifecycleError("lifecycle_restore_published_port_rejected")
		}
	}
	expected := map[string]string{e.Volumes[0]: "/var/lib/mysql"}
	if e.Kind == "mongodb" {
		expected = map[string]string{e.Volumes[0]: "/data/db", e.Volumes[1]: "/data/configdb"}
	}
	seen := map[string]bool{}
	binds := map[string]bool{}
	for _, m := range v.Mounts {
		if m.Type == "volume" && m.RW && expected[m.Name] == m.Destination && !seen[m.Name] {
			seen[m.Name] = true
			continue
		}
		if m.Type == "bind" && !m.RW && ((m.Source == e.Tool && m.Destination == "/tool/restore-native") || (m.Source == e.Archive && m.Destination == "/backup")) && !binds[m.Destination] {
			binds[m.Destination] = true
			continue
		}
		return lifecycleError("lifecycle_restore_mount_rejected")
	}
	if len(seen) != len(expected) || len(binds) != 2 || len(v.Mounts) != len(expected)+2 {
		return lifecycleError("lifecycle_restore_mount_rejected")
	}
	if e.ID == "" {
		e.ID = v.ID
	}
	return nil
}
func (e *lifecycleOwnedEngine) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// Drivers may resolve their fixed placeholder differently; they cannot choose
	// another engine or socket. The selected namespace is checked on actual wire.
	if (e.Kind == "mysql" && network != "qs-retirement-wire") || (e.Kind == "mongodb" && (network != "tcp" || address != "127.0.0.1:27017")) {
		return nil, lifecycleError("lifecycle_restore_dial_binding_rejected")
	}
	if err := e.check(ctx, true); err != nil {
		return nil, err
	}
	if e.sessionContext == nil || e.sessionContext.Err() != nil {
		return nil, lifecycleError("lifecycle_restore_live_session_missing")
	}
	e.wireMu.Lock()
	if e.wireClosed {
		e.wireMu.Unlock()
		return nil, lifecycleError("lifecycle_restore_wire_owner_closed")
	}
	conn, err := openLifecycleWireConn(e.sessionContext, e.Docker, e.ID, e.Kind)
	if err == nil {
		e.wires = append(e.wires, conn)
	}
	e.wireMu.Unlock()
	if err == nil && ctx.Err() != nil {
		_ = conn.Close()
		return nil, ctx.Err()
	}
	return conn, err
}
func (e *lifecycleOwnedEngine) closeWires() error {
	e.wireMu.Lock()
	e.wireClosed = true
	wires := append([]*lifecycleWireConn(nil), e.wires...)
	e.wireMu.Unlock()
	for _, c := range wires {
		if c.Close() != nil {
			return lifecycleError("lifecycle_restore_wire_reap_failed")
		}
	}
	return nil
}

type lifecycleActualImage struct {
	ID           string `json:"Id"`
	Architecture string
	Config       struct{ Labels map[string]string }
}

func decodeLifecycleActualImage(raw []byte, image, architecture string) (lifecycleActualImage, error) {
	var images []lifecycleActualImage
	if json.Unmarshal(raw, &images) != nil || len(images) != 1 || images[0].ID != image || images[0].Architecture != architecture {
		return lifecycleActualImage{}, lifecycleError("lifecycle_restore_actual_image_rejected")
	}
	return images[0], nil
}
func readLifecycleActualImage(ctx context.Context, docker, image, architecture string) (lifecycleActualImage, error) {
	raw, err := lifecycleDocker(ctx, docker, "image", "inspect", image)
	if err != nil {
		return lifecycleActualImage{}, err
	}
	return decodeLifecycleActualImage(raw, image, architecture)
}
func verifyLifecycleRestoreImages(ctx context.Context, expected *lifecycleRestoreEngines) error {
	if ctx == nil || ctx.Err() != nil || !expected.valid() {
		return lifecycleError("lifecycle_restore_engine_approval_missing")
	}
	docker, err := lifecycleDockerExecutable()
	if err != nil {
		return err
	}
	for _, image := range []string{expected.MySQLImageID, expected.MongoImageID} {
		if _, err = readLifecycleActualImage(ctx, docker, image, expected.Architecture); err != nil {
			return err
		}
	}
	return nil
}
func startLifecycleOwnedEngine(ctx context.Context, r lifecycleRequest, kind, image, namespace string) (*lifecycleOwnedEngine, error) {
	if !r.RestoreEngines.valid() || privateDir(r.prepareRoot) != nil || strings.ContainsAny(r.ArchiveDirectory, ",\r\n\x00") {
		return nil, lifecycleError("lifecycle_restore_engine_approval_missing")
	}
	docker, err := lifecycleDockerExecutable()
	if err != nil {
		return nil, err
	}
	tool, err := os.Executable()
	if err != nil {
		return nil, lifecycleError("lifecycle_restore_tool_unavailable")
	}
	tool, err = filepath.EvalSymlinks(tool)
	if err != nil {
		return nil, lifecycleError("lifecycle_restore_tool_unavailable")
	}
	f, err := os.OpenFile(tool, os.O_RDONLY, 0)
	if err != nil {
		return nil, lifecycleError("lifecycle_restore_tool_unavailable")
	}
	info, err := f.Stat()
	st, ok := infoStat(info)
	if err != nil || info == nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0700 || !ok || st.Uid != 0 || st.Nlink != 1 {
		_ = f.Close()
		return nil, lifecycleError("lifecycle_restore_tool_rejected")
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		_ = f.Close()
		return nil, lifecycleError("lifecycle_restore_tool_hash_failed")
	}
	if f.Close() != nil {
		return nil, lifecycleError("lifecycle_restore_tool_hash_failed")
	}
	actualImage, err := readLifecycleActualImage(ctx, docker, image, r.RestoreEngines.Architecture)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, 16)
	if _, err = rand.Read(nonce); err != nil {
		return nil, lifecycleError("lifecycle_restore_name_failed")
	}
	owner := hex.EncodeToString(nonce)
	e := &lifecycleOwnedEngine{sessionContext: ctx, Name: "qs-retirement-restore-" + owner, Owner: owner, Kind: kind, ImageID: image, Tool: tool, Archive: r.ArchiveDirectory, Namespace: namespace, Docker: docker, Labels: map[string]string{"codex.task": "qs-compatibility-retirement", "codex.owner": owner, "qs.retirement.operation": r.OperationID, "qs.retirement.run": r.ActualRunID}, Volumes: []string{"qs-retirement-data-" + owner}}
	if kind == "mongodb" {
		e.Volumes = append(e.Volumes, "qs-retirement-config-"+owner)
	}
	e.ContainerLabels = map[string]string{}
	e.materialRecords = map[string]string{}
	for k, v := range actualImage.Config.Labels {
		e.ContainerLabels[k] = v
	}
	for k, v := range e.Labels {
		e.ContainerLabels[k] = v
	}
	// Never adopt an earlier name/volume even after an unknown create response.
	existing, err := lifecycleDocker(ctx, docker, "ps", "--all", "--filter", "name=^/"+e.Name+"$", "--format", "{{.ID}}")
	if err != nil || strings.TrimSpace(string(existing)) != "" {
		return nil, lifecycleError("lifecycle_restore_name_exists_or_unknown")
	}
	for _, v := range e.Volumes {
		existing, err = lifecycleDocker(ctx, docker, "volume", "ls", "--filter", "name=^"+v+"$", "--format", "{{.Name}}")
		if err != nil || strings.TrimSpace(string(existing)) != "" {
			return nil, lifecycleError("lifecycle_restore_volume_exists_or_unknown")
		}
	}
	// Durable intent precedes the first engine/volume write. Failures deliberately
	// retain precisely registered resources; no automatic delete/retry/adoption.
	registry := map[string]any{"format_version": 1, "kind": "temporary_network_none_restore_intent", "original_source_sha": r.OriginalSourceSHA, "tool_source_sha": r.ToolSourceSHA, "operation_id": r.OperationID, "actual_run_id": r.ActualRunID, "manifest_sha256": r.ManifestSHA256, "archive_sha256": r.Recovery.ArchiveSHA256, "namespace": namespace, "owner": owner, "container_name": e.Name, "image_id": image, "architecture": actualImage.Architecture, "labels": e.Labels, "container_labels": e.ContainerLabels, "volumes": e.Volumes, "tool_sha256": hex.EncodeToString(h.Sum(nil)), "network": "none", "drop_authority": false, "purge_after_acceptance_required": true}
	if writeLifecycleMaterialJSON(filepath.Join(r.prepareRoot, "restore-"+owner+".intent.private.json"), registry, e.materialRecords) != nil {
		return nil, lifecycleError("lifecycle_restore_registry_exists_or_unknown")
	}
	labelArgs := []string{}
	for _, k := range []string{"codex.task", "codex.owner", "qs.retirement.operation", "qs.retirement.run"} {
		labelArgs = append(labelArgs, "--label", k+"="+e.Labels[k])
	}
	for _, v := range e.Volumes {
		args := append([]string{"volume", "create", "--name", v}, labelArgs...)
		if _, err = lifecycleDocker(ctx, docker, args...); err != nil {
			return nil, err
		}
	}
	args := []string{"run", "--detach", "--name", e.Name, "--network", "none", "--memory", "2g", "--cpus", "2"}
	args = append(args, labelArgs...)
	args = append(args, "--mount", "type=bind,src="+e.Archive+",dst=/backup,readonly", "--mount", "type=bind,src="+tool+",dst=/tool/restore-native,readonly")
	switch kind {
	case "mysql":
		args = append(args, "--mount", "type=volume,src="+e.Volumes[0]+",dst=/var/lib/mysql", "--env", "MYSQL_ALLOW_EMPTY_PASSWORD=1", image)
	case "mongodb":
		args = append(args, "--mount", "type=volume,src="+e.Volumes[0]+",dst=/data/db", "--mount", "type=volume,src="+e.Volumes[1]+",dst=/data/configdb", image, "mongod", "--bind_ip", "127.0.0.1", "--setParameter", "ttlMonitorEnabled=false")
	default:
		return nil, lifecycleError("lifecycle_restore_kind_rejected")
	}
	raw, err := lifecycleDocker(ctx, docker, args...)
	if err != nil {
		return nil, err
	}
	cid := strings.TrimSpace(string(raw))
	if !hashRE.MatchString(cid) {
		return nil, lifecycleError("lifecycle_restore_create_result_unknown")
	}
	e.ID = cid
	if err = e.check(ctx, true); err != nil {
		return nil, err
	}
	if writeLifecycleMaterialJSON(filepath.Join(r.prepareRoot, "restore-"+owner+".created.private.json"), map[string]any{"container_id": e.ID, "owner": owner, "operation_id": r.OperationID, "actual_run_id": r.ActualRunID, "drop_authority": false}, e.materialRecords) != nil {
		return nil, lifecycleError("lifecycle_restore_created_record_failed")
	}
	return e, nil
}
func lifecycleOwnedSQLPool(ctx context.Context, e *lifecycleOwnedEngine) (*sql.DB, error) {
	cfg := mysql.NewConfig()
	cfg.User = "root"
	cfg.Net = "qs-retirement-wire"
	cfg.DialFunc = e.DialContext
	cfg.Addr = "owned-local-socket"
	cfg.Timeout = 10 * time.Second
	cfg.ReadTimeout = 30 * time.Second
	cfg.WriteTimeout = 30 * time.Second
	cfg.MultiStatements = true
	if cfg.Apply(mysql.Charset("utf8mb4", "")) != nil {
		return nil, lifecycleError("lifecycle_restore_sql_configuration_rejected")
	}
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, lifecycleError("lifecycle_restore_sql_configuration_rejected")
	}
	pool := sql.OpenDB(connector)
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(1)
	// Engine init may expose a temporary socket. Wait on actual native queries,
	// never imported readiness. The 600-second parent context includes this wait.
	for {
		var skip int
		if pool.QueryRowContext(ctx, "SELECT @@skip_networking").Scan(&skip) == nil && skip == 0 {
			break
		}
		if ctx.Err() != nil {
			_ = pool.Close()
			return nil, lifecycleError("lifecycle_restore_engine_not_ready")
		}
		select {
		case <-ctx.Done():
			_ = pool.Close()
			return nil, lifecycleError("lifecycle_restore_engine_not_ready")
		case <-time.After(time.Second):
		}
	}
	if _, err = pool.ExecContext(ctx, "CREATE DATABASE `"+e.Namespace+"` CHARACTER SET utf8mb4"); err != nil {
		_ = pool.Close()
		return nil, lifecycleError("lifecycle_restore_namespace_create_failed")
	}
	// Every subsequent actual native connection selects the exact new namespace
	// in the MySQL handshake, including reconnects. Never rely on pooled USE state.
	if pool.Close() != nil {
		return nil, lifecycleError("lifecycle_restore_handle_close_failed")
	}
	cfg.DBName = e.Namespace
	connector, err = mysql.NewConnector(cfg)
	if err != nil {
		return nil, lifecycleError("lifecycle_restore_sql_configuration_rejected")
	}
	pool = sql.OpenDB(connector)
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(1)
	if pool.PingContext(ctx) != nil {
		_ = pool.Close()
		return nil, lifecycleError("lifecycle_restore_namespace_select_failed")
	}
	return pool, nil
}
func lifecycleOwnedMongo(ctx context.Context, e *lifecycleOwnedEngine) (*mongo.Client, *mongo.Database, error) {
	opts := options.Client().SetHosts([]string{"127.0.0.1:27017"}).SetDirect(true).SetDialer(e).SetConnectTimeout(10 * time.Second).SetServerSelectionTimeout(10 * time.Second).SetSocketTimeout(30 * time.Second).SetMaxPoolSize(1)
	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, nil, lifecycleError("lifecycle_restore_mongo_connect_failed")
	}
	for client.Ping(ctx, readpref.Primary()) != nil {
		if ctx.Err() != nil {
			_ = client.Disconnect(context.Background())
			return nil, nil, lifecycleError("lifecycle_restore_engine_not_ready")
		}
	}
	return client, client.Database(e.Namespace), nil
}
