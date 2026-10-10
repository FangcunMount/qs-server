//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	"github.com/go-jose/go-jose/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const nativeHistoryAISource = "90403f759d1d034c7adfbbdaf5f2bd1803daa931"

// Only the verified GitHub job fixture uses this caller. Output and stderr stay
// private; the real fixed Python producer, not a test footer, creates facts.
func nativeHistoryAICommand(ctx context.Context, input []byte, limit int64, args ...string) ([]byte, error) {
	work, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(work, args[0], args[1:]...)
	cmd.Stdin, cmd.Stderr = bytes.NewReader(input), io.Discard
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(pipe, limit+1))
	if readErr != nil || int64(len(raw)) > limit {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, errors.New("owned_ai_fixture_output_limit")
	}
	if err = cmd.Wait(); err != nil {
		return raw, errors.New("owned_ai_fixture_command_rejected")
	}
	return raw, nil
}

func TestHistoryAIFixtureCommandKeepsOutputBoundedAndFailurePrivate(t *testing.T) {
	if raw, err := nativeHistoryAICommand(t.Context(), nil, 8, "/bin/sh", "-c", "printf fixed"); err != nil || string(raw) != "fixed" {
		t.Fatal("bounded native fixture command failed")
	}
	if raw, err := nativeHistoryAICommand(t.Context(), nil, 4, "/bin/sh", "-c", "printf fixture-output-exceeds-bound"); err == nil || raw != nil {
		t.Fatal("native fixture output bound bypassed")
	}
	if raw, err := nativeHistoryAICommand(t.Context(), nil, 8, "/bin/sh", "-c", "printf fixed; exit 3"); err == nil || string(raw) != "fixed" {
		t.Fatal("native fixture failed process became success or lost bounded diagnostic")
	}
}

// Reuses the original qs-ai smoke's real runtime/MQ dependencies. It never
// replaces an existing /qs-ai, changes the shared job databases, or imports Q.
func nativeHistoryExternalFixture(t *testing.T, pool *sql.DB, client *mongo.Client, db *mongo.Database, a *approvedInputs) retirement.AIExternalExecutionInput {
	t.Helper()
	requireNative(t)
	if os.Getenv("QS_HISTORY_CLI_CI_INTEGRATION") != "1" || os.Getenv("QS_HISTORY_AI_RUNTIME_SOURCE") != nativeHistoryAISource || a == nil || a.request.OperationID != "123-1" || a.request.RunID != "125-1" {
		t.Fatal("actual GitHub AI runtime fixture required")
	}
	run := func(input []byte, limit int64, args ...string) []byte {
		t.Helper()
		raw, err := nativeHistoryAICommand(t.Context(), input, limit, args...)
		if err != nil {
			t.Fatal("owned AI fixture command failed", args[0], "private_stdout_sha256", rawHash(raw))
		}
		return raw
	}
	docker := func(input []byte, limit int64, args ...string) []byte {
		t.Helper()
		return run(input, limit, append([]string{"docker", "--host", "unix:///var/run/docker.sock"}, args...)...)
	}
	imageRef := "qs-retirement-ai-native:" + nativeHistoryAISource
	var images []struct {
		ID               string `json:"Id"`
		OS, Architecture string
		Config           struct {
			Env    []string
			Labels map[string]string
		}
	}
	if json.Unmarshal(docker(nil, 1<<20, "image", "inspect", imageRef), &images) != nil || len(images) != 1 || !strings.HasPrefix(images[0].ID, "sha256:") || images[0].OS != "linux" || images[0].Architecture != "amd64" || images[0].Config.Labels["org.opencontainers.image.revision"] != nativeHistoryAISource {
		t.Fatal("actual fixed qs-ai image identity rejected")
	}
	if !strings.Contains("\x00"+strings.Join(images[0].Config.Env, "\x00")+"\x00", "\x00QS_AI_RELEASE_SHA="+nativeHistoryAISource+"\x00") {
		t.Fatal("actual image release mismatch")
	}
	if len(bytes.TrimSpace(docker(nil, 64<<10, "ps", "-aq", "--no-trunc", "--filter", "name=^/qs-ai$"))) != 0 {
		t.Fatal("refusing existing qs-ai runtime")
	}
	// The genuine job service provides the already-isolated bridge. No network
	// is attached to or disconnected from another fixture by this caller.
	var mysql []struct {
		ID              string `json:"Id"`
		NetworkSettings struct {
			Networks map[string]struct{ NetworkID, IPAddress string }
		}
	}
	mysqlCID := os.Getenv("QS_HISTORY_CI_MYSQL_CONTAINER")
	if json.Unmarshal(docker(nil, 1<<20, "inspect", mysqlCID), &mysql) != nil || len(mysql) != 1 || mysql[0].ID != mysqlCID || len(mysql[0].NetworkSettings.Networks) != 1 {
		t.Fatal("actual job MySQL network rejected")
	}
	var network, mysqlIP string
	for _, n := range mysql[0].NetworkSettings.Networks {
		network, mysqlIP = n.NetworkID, n.IPAddress
	}
	if !hashPattern.MatchString(network) || mysqlIP == "" {
		t.Fatal("actual isolated job network missing")
	}
	for _, root := range []string{"/opt/qs-ai", "/data/infra/ssl/grpc", "/data/infra/qs-ai-messaging"} {
		if _, err := os.Lstat(root); !os.IsNotExist(err) {
			t.Fatal("refusing existing fixed AI material namespace")
		}
	}
	token := nativeToken(t)
	aiDB := "qs_history_ai_native_" + token
	if _, err := pool.ExecContext(t.Context(), "CREATE DATABASE `"+aiDB+"` CHARACTER SET utf8mb4"); err != nil {
		t.Fatal("owned AI namespace create")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var remaining int
		if _, err := pool.ExecContext(ctx, "DROP DATABASE `"+aiDB+"`"); err != nil || pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name=?", aiDB).Scan(&remaining) != nil || remaining != 0 {
			t.Error("owned AI namespace cleanup not proven")
		} else {
			t.Log("owned_ai_namespace_cleanup_zero")
		}
	})
	// Exact frozen physical 0040 DDL; no schema approximation or Base DTO.
	assets, err := filepath.Abs("../../scripts/database")
	if err != nil {
		t.Fatal("original asset directory")
	}
	layout, err := os.ReadFile(filepath.Join(assets, "qs-ai-retirement-0040-layout.py"))
	if err != nil || rawHash(layout) != "690d68be91713641ad6ce13ad9ad126828c64fe780bda05522bd68697ead577f" {
		t.Fatal("actual 0040 layout asset rejected")
	}
	marker := []byte("_SOURCE_CONTRACT = r\"\"\"")
	start := bytes.Index(layout, marker)
	if start < 0 {
		t.Fatal("fixed 0040 source contract absent")
	}
	start += len(marker)
	end := bytes.Index(layout[start:], []byte("\"\"\""))
	var contract struct {
		Head   string `json:"head"`
		Tables []struct {
			Name    string   `json:"name"`
			SQL     string   `json:"create_sql"`
			Indexes []string `json:"index_sql"`
			Foreign []struct {
				Table string `json:"table"`
			} `json:"foreign_keys"`
		} `json:"tables"`
	}
	if end < 0 || json.Unmarshal(layout[start:start+end], &contract) != nil || contract.Head != "0040_module_table_names" || len(contract.Tables) != 43 {
		t.Fatal("fixed physical contract rejected")
	}
	conn, err := pool.Conn(t.Context())
	if err != nil {
		t.Fatal("owned AI schema connection")
	}
	defer func() {
		if conn.Close() != nil {
			t.Error("owned AI schema connection close")
		}
	}()
	if _, err = conn.ExecContext(t.Context(), "USE `"+aiDB+"`"); err != nil {
		t.Fatal("owned AI schema selection")
	}
	done := map[string]bool{}
	for len(done) < len(contract.Tables) {
		progress := false
		for _, table := range contract.Tables {
			if done[table.Name] {
				continue
			}
			ready := true
			for _, foreign := range table.Foreign {
				if !done[foreign.Table] {
					ready = false
				}
			}
			if !ready {
				continue
			}
			for _, q := range append([]string{table.SQL}, table.Indexes...) {
				if _, err = conn.ExecContext(t.Context(), q); err != nil {
					t.Fatal("actual 0040 physical DDL rejected", rawHash([]byte(q)))
				}
			}
			done[table.Name], progress = true, true
		}
		if !progress {
			t.Fatal("fixed physical FK closure cycle")
		}
	}
	for _, q := range []string{"CREATE TABLE alembic_version(version_num VARCHAR(32) PRIMARY KEY) ENGINE=InnoDB", "INSERT INTO alembic_version VALUES ('0040_module_table_names')"} {
		if _, err = conn.ExecContext(t.Context(), q); err != nil {
			t.Fatal("actual AI schema head seed")
		}
	}
	// Return the borrowed pool connection to its original namespace before any
	// further peer reads. An actual server UUID/database view binds both sides.
	if _, err = conn.ExecContext(t.Context(), "USE `"+db.Name()+"`"); err != nil {
		t.Fatal("owned peer schema restore")
	}
	// Only the native test's random user receives its own business write role;
	// existing fixture cleanup drops that exact user/role/database and verifies 0.
	if client.Database("admin").RunCommand(t.Context(), bson.D{{Key: "grantRolesToUser", Value: os.Getenv("MONGODB_USERNAME")}, {Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "readWrite"}, {Key: "db", Value: db.Name()}}}}}).Err() != nil {
		t.Fatal("owned Mongo write grant rejected")
	}
	uid, gid := strconv.Itoa(os.Geteuid()), strconv.Itoa(os.Getegid())
	keyRoot := "/data/infra/qs-ai-messaging/versions/" + token
	ownedFiles := map[string]string{}
	ownedFileInfo := map[string]os.FileInfo{}
	ownedDirectoryInfo := map[string]os.FileInfo{}
	release := nativeHistoryAISource + "-125-1"
	releaseDir := "/opt/qs-ai/releases/" + release
	ownedDirectories := map[string]bool{"/opt/qs-ai": true, "/opt/qs-ai/releases": true, releaseDir: true, "/data/infra/ssl/grpc": true, "/data/infra/ssl/grpc/ca": true, "/data/infra/ssl/grpc/server": true, "/data/infra/qs-ai-messaging": true, "/data/infra/qs-ai-messaging/versions": true, keyRoot: true}
	write := func(path string, raw []byte, mode os.FileMode) {
		t.Helper()
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			t.Fatal("owned fixed fixture material create")
		}
		_, err = f.Write(raw)
		syncErr, closeErr := f.Sync(), f.Close()
		if err != nil || syncErr != nil || closeErr != nil {
			t.Fatal("owned fixed fixture material persist")
		}
		st, err := os.Lstat(path)
		if err != nil || !st.Mode().IsRegular() {
			t.Fatal("owned fixed fixture file identity")
		}
		ownedFileInfo[path] = st
		ownedFiles[path] = rawHash(raw)
	}
	t.Cleanup(func() {
		paths := make([]string, 0, len(ownedFiles))
		for path := range ownedFiles {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		ok := true
		for _, path := range paths {
			raw, err := os.ReadFile(path)
			s, se := os.Lstat(path)
			if err != nil || se != nil || !s.Mode().IsRegular() || !os.SameFile(s, ownedFileInfo[path]) || s.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) || s.Sys().(*syscall.Stat_t).Nlink != 1 || rawHash(raw) != ownedFiles[path] {
				t.Error("owned AI material changed; preserved")
				ok = false
				continue
			}
			if os.Remove(path) != nil {
				t.Error("owned AI material remove failed")
				ok = false
			}
		}
		// Only empty directories created in the initially absent fixed namespaces.
		for _, root := range []string{"/opt/qs-ai", "/data/infra/ssl/grpc", "/data/infra/qs-ai-messaging"} {
			if _, e := os.Lstat(root); os.IsNotExist(e) {
				continue
			}
			var dirs []string
			walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
				if e != nil {
					return e
				}
				st, se := os.Lstat(path)
				if !d.IsDir() || !ownedDirectories[path] || se != nil || ownedDirectoryInfo[path] == nil || !os.SameFile(st, ownedDirectoryInfo[path]) {
					return errors.New("unknown_owned_fixture_material")
				}
				dirs = append(dirs, path)
				return nil
			})
			if walkErr != nil {
				t.Error("owned AI fixed namespace unknown; preserved")
				ok = false
				continue
			}
			for i := len(dirs) - 1; i >= 0; i-- {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				_, err := nativeHistoryAICommand(ctx, nil, 64<<10, "sudo", "-n", "rmdir", dirs[i])
				cancel()
				if err != nil {
					t.Error("owned AI directory remove failed")
					ok = false
				}
			}
			if _, err := os.Lstat(root); !os.IsNotExist(err) {
				t.Error("owned AI fixed namespace remains")
				ok = false
			}
		}
		if ok {
			t.Log("owned_ai_release_tls_jose_materials_cleanup_zero")
		}
	})
	for _, path := range []string{"/opt/qs-ai", "/data/infra/ssl/grpc/ca", "/data/infra/ssl/grpc/server", keyRoot} {
		run(nil, 64<<10, "sudo", "-n", "install", "-d", "-m", "0755", "-o", uid, "-g", gid, path)
	}
	for path := range ownedDirectories {
		if st, e := os.Lstat(path); e == nil && st.IsDir() {
			ownedDirectoryInfo[path] = st
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal("owned TLS key")
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "qs-ai-grpc"}, DNSNames: []string{"localhost", "qs-ai-grpc"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal("owned TLS certificate")
	}
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal("owned TLS key encoding")
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	write("/data/infra/ssl/grpc/ca/ca-chain.crt", certPEM, 0644)
	write("/data/infra/ssl/grpc/server/qs-ai-fullchain.crt", certPEM, 0644)
	write("/data/infra/ssl/grpc/server/qs-ai.key", pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}), 0644)
	for _, role := range []string{"ai.sign", "ai.encrypt", "qs.sign", "qs.encrypt"} {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal("owned JOSE key")
		}
		j := jose.JSONWebKey{Key: k, KeyID: role + ".native"}
		if strings.HasPrefix(role, "qs.") {
			j = j.Public()
		}
		raw, err := j.MarshalJSON()
		if err != nil {
			t.Fatal("owned JOSE encoding")
		}
		write(keyRoot+"/"+role+".native.json", raw, 0644)
	}
	root := privateTestDir(t)
	// Register before any Engine creation. Unknown/partial creation is read
	// back by fixed name plus this random owner label; a different object stays.
	ownedContainers := map[string]string{}
	t.Cleanup(func() {
		for _, name := range []string{"qs-ai", "qs-history-ai-provision-" + token, "qs-history-ai-nsq-" + token} {
			expectedCID, attempted := ownedContainers[name]
			if !attempted {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			raw, e := nativeHistoryAICommand(ctx, nil, 64<<10, "docker", "--host", "unix:///var/run/docker.sock", "ps", "-aq", "--no-trunc", "--filter", "name=^/"+name+"$")
			if e != nil {
				cancel()
				t.Error("owned AI container inventory unknown")
				continue
			}
			if len(bytes.TrimSpace(raw)) == 0 {
				cancel()
				t.Log("owned_ai_container_cleanup_zero", name)
				continue
			}
			actualCID := strings.TrimSpace(string(raw))
			raw, e = nativeHistoryAICommand(ctx, nil, 1<<20, "docker", "--host", "unix:///var/run/docker.sock", "inspect", actualCID)
			var rows []struct {
				ID     string `json:"Id"`
				Name   string
				Config struct{ Labels map[string]string }
			}
			if e != nil || json.Unmarshal(raw, &rows) != nil || len(rows) != 1 || rows[0].ID != actualCID || rows[0].Name != "/"+name || rows[0].Config.Labels["qs.retirement.fixture"] != token || (expectedCID != "" && expectedCID != actualCID) {
				cancel()
				t.Error("owned AI fixture CID ownership unknown; preserved")
				continue
			}
			_, e = nativeHistoryAICommand(ctx, nil, 64<<10, "docker", "--host", "unix:///var/run/docker.sock", "rm", "-f", actualCID)
			if e != nil {
				cancel()
				t.Error("owned AI fixture CID cleanup failed")
				continue
			}
			raw, e = nativeHistoryAICommand(ctx, nil, 64<<10, "docker", "--host", "unix:///var/run/docker.sock", "ps", "-aq", "--no-trunc", "--filter", "id="+actualCID)
			cancel()
			if e != nil || len(bytes.TrimSpace(raw)) != 0 {
				t.Error("owned AI fixture CID remains")
			} else {
				t.Log("owned_ai_container_cleanup_zero", actualCID)
			}
		}
	})
	nsqName := "qs-history-ai-nsq-" + token
	ownedContainers[nsqName] = ""
	nsqCID := strings.TrimSpace(string(docker(nil, 64<<10, "create", "--name", nsqName, "--label", "qs.retirement.fixture="+token, "--network", network, "--read-only", "--tmpfs", "/data", "nsqio/nsq:v1.3.0", "/nsqd", "--data-path=/data", "--broadcast-address="+nsqName)))
	if !hashPattern.MatchString(nsqCID) {
		t.Fatal("actual owned NSQ CID rejected")
	}
	ownedContainers[nsqName] = nsqCID
	docker(nil, 64<<10, "start", nsqCID)
	nsqImage := strings.TrimSpace(string(docker(nil, 64<<10, "inspect", "--format", "{{.Image}}", nsqCID)))
	if !strings.HasPrefix(nsqImage, "sha256:") || !hashPattern.MatchString(strings.TrimPrefix(nsqImage, "sha256:")) {
		t.Fatal("actual owned NSQ image identity rejected")
	}
	// Original qs-ai smoke's topology producer uses this exact image SDK. It
	// writes only the UUID-owned broker; no imported topic hash is trusted.
	provisionCode := `import json,sys,time,urllib.request,urllib.parse
from reliable_messaging.wire import failed_topic,FAILED_CHANNEL
origin='http://'+sys.argv[1]+':4151'
for attempt in range(30):
 try:
  with urllib.request.urlopen(origin+'/info',timeout=2) as r: assert json.load(r)['version']=='1.3.0'
  break
 except OSError: time.sleep(1)
else: raise RuntimeError('owned_nsq_not_ready')
topology={'qs.ai.commands.v1':'qs-ai.commands.v1','qs.ai.events.v1':'qs-server.ai-events.v1','qs.ai.acks.v1':'qs-ai.acks.v1'}
topology.update({failed_topic(t,c):FAILED_CHANNEL for t,c in list(topology.items())})
for topic,channel in topology.items():
 query=urllib.parse.urlencode({'topic':topic,'channel':channel})
 for endpoint in ('topic/create','channel/create'):
  request=urllib.request.Request(origin+'/'+endpoint+'?'+query,data=b'',method='POST')
  with urllib.request.urlopen(request,timeout=5): pass
`
	provisionName := "qs-history-ai-provision-" + token
	ownedContainers[provisionName] = ""
	provisionCID := strings.TrimSpace(string(docker(nil, 64<<10, "create", "--name", provisionName, "--label", "qs.retirement.fixture="+token, "--network", network, "--read-only", "--tmpfs", "/tmp", images[0].ID, "/app/.venv/bin/python", "-c", provisionCode, nsqName)))
	if !hashPattern.MatchString(provisionCID) {
		t.Fatal("actual owned topology producer CID rejected")
	}
	ownedContainers[provisionName] = provisionCID
	docker(nil, 64<<10, "start", "-a", provisionCID)
	options := map[string]any{"enabled": true, "nsqd": map[string]string{nsqName + ":4150": "http://" + nsqName + ":4151"}, "signing_key_file": "/run/qs-ai-jose/ai.sign.native.json", "decrypt_key_files": map[string]string{"ai.encrypt.native": "/run/qs-ai-jose/ai.encrypt.native.json"}, "qs_signer_files": map[string]string{"qs.sign.native": "/run/qs-ai-jose/qs.sign.native.json"}, "qs_recipient_key_file": "/run/qs-ai-jose/qs.encrypt.native.json", "max_in_flight": 1}
	messaging := string(reverseNativeJSON(t, options))
	env := map[string]string{"QS_AI_ENVIRONMENT": "production", "QS_AI_RELEASE_SHA": nativeHistoryAISource, "QS_AI_DATABASE_URL": "mysql+asyncmy://root:root@" + mysqlIP + ":3306/" + aiDB, "QS_AI_MESSAGING": messaging, "QS_AI_GENERATION__ENABLED": "false", "QS_AI_EVALUATION__ENABLED": "false", "QS_AI_GRPC__ACCESS_ADDRESS": "qs-ai-grpc:50061"}
	var volumes []map[string]any
	for _, role := range []string{"ai.sign", "ai.encrypt", "qs.sign", "qs.encrypt"} {
		volumes = append(volumes, map[string]any{"type": "bind", "source": keyRoot + "/" + role + ".native.json", "target": "/run/qs-ai-jose/" + role + ".native.json", "read_only": true, "bind": map[string]bool{"create_host_path": false}})
	}
	composeRaw, err := os.ReadFile(filepath.Join(os.Getenv("QS_HISTORY_AI_SOURCE_DIR"), "deploy/serverA/compose.yaml"))
	if err != nil {
		t.Fatal("actual original runtime compose missing")
	}
	// JSON is a Compose file. Keep the actual command/image contract and exact
	// original healthcheck argv; no script process stands in for the application.
	baseVolumes := []map[string]any{{"type": "bind", "source": "/data/infra/ssl/grpc/ca/ca-chain.crt", "target": "/run/qs-ai-tls/ca-chain.crt", "read_only": true}, {"type": "bind", "source": "/data/infra/ssl/grpc/server/qs-ai-fullchain.crt", "target": "/run/qs-ai-tls/qs-ai-fullchain.crt", "read_only": true}, {"type": "bind", "source": "/data/infra/ssl/grpc/server/qs-ai.key", "target": "/run/qs-ai-tls/qs-ai.key", "read_only": true}}
	compose := map[string]any{"services": map[string]any{"qs-ai": map[string]any{"image": images[0].ID, "container_name": "qs-ai", "restart": "unless-stopped", "labels": map[string]string{"org.opencontainers.image.revision": nativeHistoryAISource, "qs.retirement.fixture": token}, "environment": env, "read_only": true, "tmpfs": []string{"/tmp:rw,noexec,nosuid,size=64m"}, "cap_drop": []string{"ALL"}, "security_opt": []string{"no-new-privileges:true"}, "healthcheck": map[string]any{"test": []string{"CMD", "/app/.venv/bin/python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8000/readyz', timeout=3)"}, "interval": "5s", "timeout": "5s", "retries": 12}, "volumes": append(baseVolumes, volumes...), "networks": map[string]any{"native": map[string]any{"aliases": []string{"qs-ai-grpc"}}}}}, "networks": map[string]any{"native": map[string]any{"external": true, "name": network}}}
	if os.MkdirAll(releaseDir, 0700) != nil {
		t.Fatal("owned actual release directory")
	}
	for _, path := range []string{"/opt/qs-ai/releases", releaseDir} {
		st, e := os.Lstat(path)
		if e != nil || !st.IsDir() {
			t.Fatal("actual owned release directory identity")
		}
		ownedDirectoryInfo[path] = st
	}
	write("/opt/qs-ai/state.json", reverseNativeJSON(t, map[string]string{"current": release, "previous": ""}), 0600)
	write(releaseDir+"/manifest.json", reverseNativeJSON(t, map[string]string{"revision": nativeHistoryAISource, "image_id": images[0].ID, "messaging_binding_sha256": jsonHash(map[string]any{"options": messaging, "volumes": volumes})}), 0600)
	write(releaseDir+"/runtime.json", reverseNativeJSON(t, map[string]any{"services": map[string]any{"qs-ai": map[string]any{"environment": env, "volumes": volumes}}}), 0600)
	write(releaseDir+"/compose.yaml", composeRaw, 0600)
	write(releaseDir+"/image.env", []byte("QS_AI_IMAGE="+images[0].ID+"\n"), 0600)
	composePath := filepath.Join(root, "runtime.compose.private.json")
	writeFixtureJSON(t, composePath, compose)
	ownedContainers["qs-ai"] = ""
	docker(nil, 64<<10, "compose", "-p", "qs-history-ai-"+token, "-f", composePath, "create", "--no-build", "--pull", "never")
	cid := strings.TrimSpace(string(docker(nil, 64<<10, "inspect", "--format", "{{.Id}}", "qs-ai")))
	if !hashPattern.MatchString(cid) {
		t.Fatal("actual runtime CID rejected")
	}
	ownedContainers["qs-ai"] = cid
	docker(nil, 64<<10, "start", cid)
	deadline := time.Now().Add(90 * time.Second)
	for {
		raw := docker(nil, 64<<10, "inspect", "--format", "{{.State.Running}} {{if .State.Health}}{{.State.Health.Status}}{{end}}", cid)
		if strings.TrimSpace(string(raw)) == "true healthy" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("actual qs-ai readiness not reached")
		}
		time.Sleep(time.Second)
	}
	binding, err := retirement.ObserveAIExternalRuntimeBinding(t.Context(), nativeHistoryAISource, images[0].ID, cid, false)
	if err != nil {
		t.Fatal("actual runtime release/mount binding", err)
	}
	operation := filepath.Join(root, "backups", "qs-server", "compatibility-retirement", a.request.OperationID)
	if os.MkdirAll(operation, 0700) != nil {
		t.Fatal("owned original operation directory")
	}
	peer := retirement.AIExternalPeerConnection{Host: mysqlIP, Port: 3306, Database: db.Name(), Username: "root", Password: "root"}
	modules := map[string]string{}
	for _, name := range []string{"qs-ai-retirement-readonly-verifier.py", "qs-ai-retirement-readonly-observer.py", "qs-ai-retirement-0040-layout.py"} {
		raw, e := os.ReadFile(filepath.Join(assets, name))
		if e != nil {
			t.Fatal("original fixed module missing")
		}
		modules[name] = base64.StdEncoding.EncodeToString(raw)
	}
	sections := map[string]any{}
	for _, i := range []int{1, 2} {
		x := a.expected[i]
		sections[x.Boundary.Name] = map[string]any{"rows": x.Records, "source_bytes": x.Bytes, "source_sha256": x.DataHash}
	}
	packet := map[string]any{"protocol": "qs-ai-readonly-bounds-discovery-input/v1", "source_sha": a.request.SourceSHA, "operation_id": a.request.OperationID, "run_id": "125", "runtime_source_sha": nativeHistoryAISource, "runtime_binding_sha256": binding, "runtime_messaging": options, "image_id": images[0].ID, "container_id": cid, "original_sections": sections, "peer_connection": map[string]any{"host": mysqlIP, "port": 3306, "database": db.Name(), "username": "root", "password": "root"}, "expected_ai": map[string]string{"identity_hash": "", "head": ""}, "expected_peer": map[string]string{"identity_hash": a.inventory.Identities["mysql"], "head": "99"}, "modules": modules}
	hostProgram, err := os.ReadFile(filepath.Join(assets, "qs-ai-retirement-readonly-host.py"))
	if err != nil {
		t.Fatal("original fixed discovery host missing")
	}
	raw := docker(reverseNativeJSON(t, packet), 32<<20, "exec", "-i", cid, "/app/.venv/bin/python", "-c", string(hostProgram))
	var discovered struct {
		Protocol string `json:"protocol"`
		Source   string `json:"source_sha"`
		Op       string `json:"operation_id"`
		Run      string `json:"run_id"`
		Runtime  string `json:"runtime_source_sha"`
		Binding  string `json:"runtime_binding_sha256"`
		Image    string `json:"image_id"`
		CID      string `json:"container_id"`
		Epochs   int    `json:"independent_epochs"`
		Bounds   map[string]struct {
			Bytes string `json:"bytes"`
			SHA   string `json:"sha256"`
		} `json:"bounds"`
		Approval  bool `json:"independent_approval"`
		Business  bool `json:"business_closure"`
		Fence     bool `json:"fence"`
		DropReady bool `json:"drop_ready"`
		CAS       bool `json:"cas_authority"`
	}
	if json.Unmarshal(raw, &discovered) != nil || discovered.Protocol != "qs-ai-readonly-bounds-discovery-facts/v1" || discovered.Epochs != 2 || len(discovered.Bounds) != 2 || discovered.Source != a.request.SourceSHA || discovered.Op != a.request.OperationID || discovered.Run != "125" || discovered.Runtime != nativeHistoryAISource || discovered.Binding != binding || discovered.Image != images[0].ID || discovered.CID != cid || discovered.Approval || discovered.Business || discovered.Fence || discovered.DropReady || discovered.CAS {
		t.Fatal("actual fixed discovery result rejected")
	}
	aiBounds, err := base64.StdEncoding.DecodeString(discovered.Bounds["ai"].Bytes)
	if err != nil || rawHash(aiBounds) != discovered.Bounds["ai"].SHA {
		t.Fatal("actual AI discovery bytes mismatch")
	}
	peerBounds, err := base64.StdEncoding.DecodeString(discovered.Bounds["peer"].Bytes)
	if err != nil || rawHash(peerBounds) != discovered.Bounds["peer"].SHA {
		t.Fatal("actual peer discovery bytes mismatch")
	}
	if repeated, e := retirement.ObserveAIExternalRuntimeBinding(t.Context(), nativeHistoryAISource, images[0].ID, cid, false); e != nil || repeated != binding {
		t.Fatal("actual runtime changed during discovery")
	}
	t.Log("actual_ai_runtime_fixture", nativeHistoryAISource, images[0].ID, cid, nsqImage, nsqCID, provisionCID)
	return retirement.AIExternalExecutionInput{Binding: retirement.HistoricalCoordinatorBinding{SourceSHA: a.request.SourceSHA, OperationID: a.request.OperationID}, OperationDirectory: operation, AssetsDirectory: assets, RunID: "125", RuntimeSourceSHA: nativeHistoryAISource, ImageID: images[0].ID, ContainerID: cid, AIBounds: aiBounds, PeerBounds: peerBounds, ApprovedAIBoundsSHA256: rawHash(aiBounds), ApprovedPeerBoundsSHA256: rawHash(peerBounds), ApprovedAIRuntimeBindingSHA256: binding, PeerConnection: peer, ProtectionJSON: []byte(`{"decrypt_keys":{},"trusted_signers":{}}`)}
}

func reverseNativeStatement(t *testing.T, pool *sql.DB, q string, args ...any) {
	t.Helper()
	if _, e := pool.ExecContext(t.Context(), q, args...); e != nil {
		t.Fatal("owned reverse fixture statement rejected", rawHash([]byte(q)))
	}
}
func reverseNativeJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, e := json.Marshal(v)
	if e != nil {
		t.Fatal("synthetic body serialize")
	}
	return raw
}
func reverseNativeProtector(t *testing.T, aggregate string) func(pb.MessagingKind, string, string, *pb.MessagingBody) *app.PreparedMessaging {
	t.Helper()
	key := func(id string) jose.JSONWebKey {
		v, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			t.Fatal("synthetic fixture key")
		}
		return jose.JSONWebKey{Key: v, KeyID: id}
	}
	sign, encrypt := key("pipeline-reverse-sign"), key("pipeline-reverse-encrypt")
	return func(kind pb.MessagingKind, id, correlation string, body *pb.MessagingBody) *app.PreparedMessaging {
		v, e := app.ProtectMessaging(kind, id, aggregate, correlation, "7", "", body, sign, encrypt.Public())
		if e != nil {
			t.Fatal("synthetic original protocol envelope")
		}
		return v
	}
}
func reverseNativeBox(t *testing.T, pool *sql.DB, m *app.PreparedMessaging, ordered bool) {
	t.Helper()
	flag := 0
	if ordered {
		flag = 1
	}
	reverseNativeStatement(t, pool, "INSERT INTO ai_messaging_outbox VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", "qs-server", "qs-ai", m.Envelope.MessageId, m.Envelope.BodySha256, m.Body, m.Wire, rawHash(m.Wire), int(m.Envelope.Kind), 7, m.Topic, m.Envelope.AggregateKey, 1, flag, flag, "confirmed", 3, "2026-10-08 11:12:13.123456", "2026-10-08 11:12:13.123456", "2026-10-08 11:12:14.123456", "2026-10-08 11:12:14.123456", "")
}
func reverseNativeIncoming(t *testing.T, pool *sql.DB, protect func(pb.MessagingKind, string, string, *pb.MessagingBody) *app.PreparedMessaging, m *app.PreparedMessaging, ackID string) {
	t.Helper()
	ack := protect(pb.MessagingKind_EVENT_ACKNOWLEDGEMENT, ackID, "", &pb.MessagingBody{Value: &pb.MessagingBody_EventAcknowledgement{EventAcknowledgement: &pb.MessagingEventAcknowledgement{EventId: m.Envelope.MessageId, EventBodySha256: m.Envelope.BodySha256, EventKind: m.Envelope.Kind, Outcome: pb.MessagingEventAcknowledgement_STORED}}})
	reverseNativeBox(t, pool, ack, false)
	reverseNativeStatement(t, pool, "INSERT INTO ai_messaging_inbox VALUES(?,?,?,?,?,?,?,?,?,?)", "qs-ai", m.Envelope.MessageId, m.Envelope.BodySha256, m.Body, rawHash(m.Wire), int(m.Envelope.Kind), m.Envelope.AggregateKey, ackID, "2026-10-08 11:12:14.123456", "stored")
}
func reverseNativeMappedGraph(t *testing.T, pool *sql.DB) {
	t.Helper()
	requestID := "10000000-0000-4000-8000-000000000001"
	sessionID := "10000000-0000-4000-8000-000000000002"
	eventID := "10000000-0000-4000-8000-000000000003"
	receiptID := "10000000-0000-4000-8000-000000000004"
	runID := "10000000-0000-4000-8000-000000000005"
	request := app.Start{RequestID: requestID, Actor: app.Actor{OrgID: "7", SubjectID: "42"}, TesteeID: "9", AssessmentIDs: []string{"10022"}, Goal: "private fixture goal", Evidence: []app.EvidenceItem{{AssessmentID: "10022", TesteeID: "9", ReportID: "fixture-report", SourceVersion: "v1", Facts: []app.Fact{{Ref: "score", Value: "3"}}}}}
	raw := reverseNativeJSON(t, request)
	projection := app.Event{EventID: eventID, RequestID: requestID, SessionID: sessionID, Actor: request.Actor, TesteeID: request.TesteeID, Version: 2, Status: "cancelled"}
	projectionRaw := reverseNativeJSON(t, projection)
	reverseNativeStatement(t, pool, "INSERT INTO assessment(id,org_id,testee_id,questionnaire_code,questionnaire_version,answer_sheet_id,origin_type,status,created_at,updated_at,version) VALUES(10022,7,9,'fixture','1',10023,'adhoc','evaluated',UTC_TIMESTAMP(),UTC_TIMESTAMP(),1)")
	reverseNativeStatement(t, pool, "INSERT INTO ai_bridge_requests(request_id,request_hash,payload,session_id,version,status,projection,organization_id,subject_id,testee_id) VALUES(?,?,?,?,2,'cancelled',?,7,'42',9)", requestID, rawHash(raw), raw, sessionID, projectionRaw)
	reverseNativeStatement(t, pool, "INSERT INTO ai_bridge_request_assessments VALUES(?,10022)", requestID)
	protect := reverseNativeProtector(t, requestID)
	start := &pb.StartCommand{RequestId: requestID, Actor: &pb.Actor{OrgId: "7", SubjectId: "42"}, TesteeId: "9", AssessmentIds: []string{"10022"}, Goal: request.Goal, Evidence: []*pb.EvidenceItem{{AssessmentId: "10022", TesteeId: "9", ReportId: "fixture-report", SourceVersion: "v1", Facts: []*pb.Fact{{Ref: "score", Value: "3"}}}}}
	command := protect(pb.MessagingKind_START, requestID, "", &pb.MessagingBody{Value: &pb.MessagingBody_Start{Start: start}})
	receipt := protect(pb.MessagingKind_COMMAND_RECEIPT, receiptID, requestID, &pb.MessagingBody{Value: &pb.MessagingBody_CommandReceipt{CommandReceipt: &pb.MessagingCommandReceipt{CommandId: requestID, CommandBodySha256: command.Envelope.BodySha256, Decision: pb.MessagingDecision_ACCEPTED, Code: "accepted", OriginalReceipt: &pb.MessagingCommandReceipt_WorkflowReceipt{WorkflowReceipt: &pb.Receipt{SessionId: sessionID, RunId: runID, Version: 1, Status: "queued"}}}}})
	reverseNativeStatement(t, pool, "INSERT INTO ai_messaging_operations VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,NULL,NULL)", requestID, int(pb.MessagingKind_START), command.Envelope.BodySha256, 7, "42", requestID, requestID, 1, "accepted", "accepted", receiptID, receipt.Body, "2026-10-08 11:12:13.123456", "2026-10-08 11:12:14.123456")
	reverseNativeBox(t, pool, command, true)
	reverseNativeIncoming(t, pool, protect, receipt, "10000000-0000-4000-8000-000000000006")
	state := protect(pb.MessagingKind_INTERPRETATION_STATE, eventID, "", &pb.MessagingBody{Value: &pb.MessagingBody_InterpretationState{InterpretationState: &pb.StateEvent{EventId: eventID, RequestId: requestID, SessionId: sessionID, Actor: &pb.Actor{OrgId: "7", SubjectId: "42"}, TesteeId: "9", Version: 2, Status: "cancelled"}}})
	reverseNativeIncoming(t, pool, protect, state, "10000000-0000-4000-8000-000000000007")
	reverseNativeStatement(t, pool, "INSERT INTO ai_bridge_events VALUES(?,?,2,?)", eventID, requestID, rawHash(projectionRaw))
	reverseNativeStatement(t, pool, "INSERT INTO ai_messaging_aggregates VALUES(?,2)", requestID)
	reverseNativeStatement(t, pool, "INSERT INTO ai_bridge_commands VALUES(?,?,'start',?,?,0,3,?)", requestID, requestID, raw, rawHash(raw), "2026-10-08 11:12:13.123456")
	var actual []byte
	if pool.QueryRowContext(t.Context(), "SELECT CAST(payload AS BINARY) FROM ai_bridge_commands WHERE command_id=?", requestID).Scan(&actual) != nil {
		t.Fatal("actual source binary copy")
	}
	reverseNativeStatement(t, pool, "INSERT INTO ai_messaging_legacy_commands VALUES(?,?,'start',?,?,3,?,'',?,?)", requestID, requestID, actual, rawHash(raw), "2026-10-08 11:12:13.123456", command.Envelope.BodySha256, "2026-10-08 11:12:14.123456")
}
func reverseNativeEvaluationGraph(t *testing.T, pool *sql.DB) string {
	t.Helper()
	run := "20000000-0000-4000-8000-000000000001"
	id := "20000000-0000-4000-8000-000000000002"
	receiptID := "20000000-0000-4000-8000-000000000003"
	protect := reverseNativeProtector(t, run)
	command := protect(pb.MessagingKind_EVALUATION_START, id, "", &pb.MessagingBody{Value: &pb.MessagingBody_EvaluationStart{EvaluationStart: &pb.EvaluationStartCommand{Scope: &pb.EvaluationQuery{RunId: run, OrganizationId: 7, OperatorUserId: 42}, ExpectedVersion: 1, Confirm: true, Reason: "original fixture"}}})
	receipt := protect(pb.MessagingKind_COMMAND_RECEIPT, receiptID, id, &pb.MessagingBody{Value: &pb.MessagingBody_CommandReceipt{CommandReceipt: &pb.MessagingCommandReceipt{CommandId: id, CommandBodySha256: command.Envelope.BodySha256, Decision: pb.MessagingDecision_ACCEPTED, Code: "accepted", OriginalReceipt: &pb.MessagingCommandReceipt_EvaluationReceipt{EvaluationReceipt: &pb.EvaluationState{RunId: run, Version: 1}}}}})
	reverseNativeStatement(t, pool, "INSERT INTO ai_messaging_operations VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,NULL,NULL)", id, int(pb.MessagingKind_EVALUATION_START), command.Envelope.BodySha256, 7, "42", run, run, 1, "accepted", "accepted", receiptID, receipt.Body, "2026-10-08 11:12:13.123456", "2026-10-08 11:12:14.123456")
	reverseNativeBox(t, pool, command, true)
	reverseNativeIncoming(t, pool, protect, receipt, "20000000-0000-4000-8000-000000000004")
	reverseNativeStatement(t, pool, "INSERT INTO ai_messaging_aggregates VALUES(?,2)", run)
	return id
}
func TestHistoryAIReverseNativeWholePipelineEmptyOrphanMappedAndEvaluation(t *testing.T) {
	for _, which := range []string{"empty_open_admission", "empty_closed_admission", "empty_current_orphan", "mapped_undelivered", "outside_evaluation"} {
		t.Run(which, func(t *testing.T) {
			testSource(t)
			pool, client, db, _ := nativeFixture(t, false)
			switch which {
			case "empty_closed_admission":
				reverseNativeStatement(t, pool, "UPDATE ai_messaging_admission SET closed=1,revision=revision+1")
			case "empty_current_orphan":
				reverseNativeStatement(t, pool, "INSERT INTO ai_messaging_aggregates VALUES(?,2)", "30000000-0000-4000-8000-000000000001")
			case "mapped_undelivered":
				reverseNativeMappedGraph(t, pool)
			case "outside_evaluation":
				reverseNativeEvaluationGraph(t, pool)
			}
			_, _, a := nativeInputs(t, pool, client, db)
			host, e := openDatabases(t.Context(), a)
			if e != nil {
				t.Fatal(safeCategory(e))
			}
			defer func() {
				if host.close() != nil {
					t.Error("owned host close")
				}
			}()
			r, e := executePipeline(t.Context(), a, host)
			if e != nil {
				t.Fatal("actual whole reverse pipeline", safeCategory(e))
			}
			if !r.CompletedReadOnlyPipeline || r.IndependentEpochs != 2 || r.AIReverseGlobal.LedgerCount != 14 || !r.AIReverseGlobal.WholeLedgerEOF || !r.AIReverseGlobal.IndependentEpochRechecked {
				t.Fatal("whole actual 14-table independent epoch coverage missing")
			}
			if which == "empty_current_orphan" {
				if r.AIReverseGlobal.Blocking == 0 || r.BlockingReasons["ai_reverse_global_blocking_responsibility"] == 0 || r.BlockingReasons["ai_reverse_global_unknown_responsibility"] == 0 {
					t.Fatal("empty old sources hid actual unreferenced current orphan")
				}
			} else if r.AIReverseGlobal.Blocking != 0 || r.AIReverseGlobal.Unknown != 0 {
				t.Fatal("legal mapped/unrelated current traffic falsely blocked")
			}
			if which == "mapped_undelivered" {
				var delivered int
				if pool.QueryRow("SELECT delivered FROM ai_bridge_commands").Scan(&delivered) != nil || delivered != 0 || r.AIReverseGlobal.Related == 0 || r.LocalCandidates != 2 {
					t.Fatal("handoff source identity/count/delivery changed")
				}
			}
			if r.DropReady || r.CASComplete || r.WriterFenceProven || r.FullExternalAIClosureVerified || r.MutationBackendEnabled {
				t.Fatal("readonly reverse pipeline granted production authority")
			}
		})
	}
}
func TestHistoryAIReverseNativeFirstHandleSurvivesRollbackAndRejectsRealDrift(t *testing.T) {
	for _, which := range []string{"identical", "admission_drift", "evaluation_original_run_drift", "current_row_deleted", "schema", "NULL_clock", "source_reader_cursor", "source_expected_changed", "proof_mixed_coordinator"} {
		t.Run(which, func(t *testing.T) {
			testSource(t)
			pool, client, db, _ := nativeFixture(t, false)
			operation := reverseNativeEvaluationGraph(t, pool)
			_, _, a := nativeInputs(t, pool, client, db)
			host, e := openDatabases(t.Context(), a)
			if e != nil {
				t.Fatal(safeCategory(e))
			}
			defer func() {
				if host.close() != nil {
					t.Error("owned host close")
				}
			}()
			var first *epochResult
			if e = host.epoch(t.Context(), func(ctx context.Context) error {
				var err error
				first, err = buildEpoch(ctx, a, host)
				if err != nil {
					return err
				}
				if proof, e := first.recheckAIReverse(ctx, first, a); e == nil || proof != nil {
					return fixedError("history_test_same_epoch_wrongly_accepted")
				}
				before := jsonHash(first.coordinator)
				originalSQL, originalCoordinator := first.sql, first.aiReverseCoordinator
				if err = first.compactOrigin(ctx); err != nil {
					return err
				}
				if a.rewind() != nil {
					return fixedError("history_asset_read_failed")
				}
				if proof, e := first.reverseAnchor.RecheckFresh(ctx, originalSQL, a.inventory.Migrations["mysql"], originalCoordinator, a.copies()); e == nil || proof != nil {
					return fixedError("history_test_live_old_tx_accepted")
				}
				if originalSQL.ValidateBorrowedSnapshot(ctx) != nil {
					return fixedError("history_test_host_scope_ended")
				}
				if first.reverseAnchor == nil || first.aiReverse != nil || first.aiReverseCoordinator != nil || first.sql != nil || first.mongo != nil || first.origin != nil || first.anchor != nil || jsonHash(first.coordinator) != before {
					return fixedError("history_test_reverse_handle_lost")
				}
				if err = first.compactOrigin(ctx); err == nil {
					return fixedError("history_test_compaction_repeated")
				}
				return nil
			}); e != nil {
				t.Fatal("actual first borrowed epoch", safeCategory(e))
			}
			switch which {
			case "admission_drift":
				reverseNativeStatement(t, pool, "UPDATE ai_messaging_admission SET closed=NOT closed,revision=revision+1")
			case "evaluation_original_run_drift":
				reverseNativeStatement(t, pool, "UPDATE ai_messaging_operations SET resource_id=? WHERE command_id=?", "20000000-0000-4000-8000-000000000009", operation)
			case "current_row_deleted":
				reverseNativeStatement(t, pool, "DELETE FROM ai_messaging_outbox WHERE kind=?", int(pb.MessagingKind_EVENT_ACKNOWLEDGEMENT))
			case "schema":
				reverseNativeStatement(t, pool, "ALTER TABLE ai_messaging_observations ADD COLUMN future_unknown BIGINT NULL")
			case "NULL_clock":
				reverseNativeStatement(t, pool, "UPDATE ai_messaging_operations SET decided_at=NULL WHERE command_id=?", operation)
			}
			// Deliberately skip compareEpochs: the real primitive must reject raw
			// drift by actual ended/new transaction and complete native reread, even
			// when no diagnostic DTO comparison is invoked by this test.
			e = host.epoch(t.Context(), func(ctx context.Context) error {
				second, err := buildEpoch(ctx, a, host)
				if err != nil {
					return err
				}
				if which == "source_reader_cursor" {
					if a.rewind() != nil {
						return fixedError("history_asset_read_failed")
					}
					copies := a.copies()
					for _, copy := range copies {
						if _, e := io.Copy(io.Discard, copy.Input); e != nil {
							return fixedError("history_asset_read_failed")
						}
					}
					proof, e := first.reverseAnchor.RecheckFresh(ctx, second.sql, a.inventory.Migrations["mysql"], second.aiReverseCoordinator, copies)
					if e == nil || proof != nil {
						return fixedError("history_test_cursor_repaired")
					}
					return fixedError("history_ai_reverse_independent_epoch_failed")
				}
				if which == "source_expected_changed" {
					if a.rewind() != nil {
						return fixedError("history_asset_read_failed")
					}
					copies := a.copies()
					copies[0].Expected.Boundary.UpperToken = "invalid-alternate-boundary"
					proof, e := first.reverseAnchor.RecheckFresh(ctx, second.sql, a.inventory.Migrations["mysql"], second.aiReverseCoordinator, copies)
					if e == nil || proof != nil {
						return fixedError("history_test_expected_replaced")
					}
					return fixedError("history_ai_reverse_independent_epoch_failed")
				}
				aiProof, err := first.recheckAIReverse(ctx, second, a)
				if err != nil {
					return err
				}
				if a.rewind() != nil {
					return fixedError("history_asset_read_failed")
				}
				coordinator := second.aiReverseCoordinator
				if which == "proof_mixed_coordinator" {
					if a.rewind() != nil {
						return fixedError("history_asset_read_failed")
					}
					coordinator, err = retirement.PrepareHistoricalCoordinator(ctx, retirement.HistoricalCoordinatorBinding{SourceSHA: a.request.SourceSHA, OperationID: a.request.OperationID}, a.copies(), retirement.DefaultHistoricalCoordinatorLimits())
					if err != nil {
						return fixedError("history_test_new_coordinator_failed")
					}
					if a.rewind() != nil {
						return fixedError("history_asset_read_failed")
					}
				}
				proof, err := first.reverseAnchor.RecheckOrigin(ctx, aiProof, second.sql, second.mongo, coordinator, a.readers())
				if err != nil {
					return fixedError("history_actual_origin_independent_epoch_failed")
				}
				r := proof.Report()
				if !r.ActualOriginMatched || !r.IndependentEpochRechecked || !r.SourceFilesMatched || r.CASAuthorized || r.DropReady || !r.IndependentApprovalRequired || !r.FirstAuthMetadataContinuityUnproven {
					return fixedError("history_test_origin_authority_changed")
				}
				return nil
			})
			if which == "identical" {
				if e != nil {
					t.Fatal("ended-old/new actual graphless AI/origin proof", safeCategory(e))
				}
			} else {
				expected := "history_ai_reverse_independent_epoch_failed"
				if which == "schema" {
					expected = "history_ai_reverse_scan_failed"
				}
				if which == "proof_mixed_coordinator" {
					expected = "history_actual_origin_independent_epoch_failed"
				}
				if e == nil || safeCategory(e) != expected {
					t.Fatal("actual AI/schema/NULL/source/proof drift not rejected at its intended gate", safeCategory(e))
				}
			}
		})
	}
}
