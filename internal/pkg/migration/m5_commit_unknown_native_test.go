//go:build integration

package migration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

// This fault is available only on the allocated native fixture. It forwards a
// complete COMMIT to MySQL and consumes its actual OK packet, then disconnects
// the client before returning that packet. No production hook is installed.
func m5DropCommitReply(t *testing.T, upstream string) (string, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	host, _, err := net.SplitHostPort(upstream)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		t.Fatal("owned loopback MySQL required")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var commits, dropped atomic.Int32
	var mu sync.Mutex
	connections := map[net.Conn]struct{}{}
	var wg sync.WaitGroup
	done := make(chan struct{})
	packet := func(c net.Conn) ([]byte, error) {
		header := make([]byte, 4)
		if _, err := io.ReadFull(c, header); err != nil {
			return nil, err
		}
		n := int(header[0]) | int(header[1])<<8 | int(header[2])<<16
		if n > 8<<20 {
			return nil, fmt.Errorf("owned packet budget exceeded")
		}
		raw := make([]byte, 4+n)
		copy(raw, header)
		_, err := io.ReadFull(c, raw[4:])
		return raw, err
	}
	go func() {
		defer close(done)
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			backend, err := net.DialTimeout("tcp", upstream, 5*time.Second)
			if err != nil {
				_ = client.Close()
				continue
			}
			mu.Lock()
			connections[client] = struct{}{}
			connections[backend] = struct{}{}
			mu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() {
					_ = client.Close()
					_ = backend.Close()
					mu.Lock()
					delete(connections, client)
					delete(connections, backend)
					mu.Unlock()
				}()
				var commitPending atomic.Bool
				serverDone := make(chan struct{})
				go func() {
					defer close(serverDone)
					for {
						raw, e := packet(backend)
						if e != nil {
							return
						}
						if commitPending.Load() {
							if len(raw) > 4 && raw[4] == 0 {
								dropped.Add(1)
							}
							_ = client.Close()
							_ = backend.Close()
							return
						}
						if _, e = client.Write(raw); e != nil {
							return
						}
					}
				}()
				for {
					raw, e := packet(client)
					if e != nil {
						break
					}
					if len(raw) > 5 && raw[4] == 3 && strings.EqualFold(strings.TrimSpace(string(raw[5:])), "COMMIT") {
						commits.Add(1)
						commitPending.Store(true)
					}
					if _, e = backend.Write(raw); e != nil {
						break
					}
				}
				_ = backend.Close()
				_ = client.Close()
				<-serverDone
			}()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
		mu.Lock()
		for c := range connections {
			_ = c.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	return listener.Addr().String(), &commits, &dropped
}

func testM5FrozenMainCommitReplyLost(t *testing.T) {
	const source = "4620d11e2cffbc4b67b43965812cff3340f0be43"
	const productionBinarySHA = "40969dff04a7e700544068c3361a80cdf0ea51ecbaeb1bd83b4dd5363bb7b4b9"
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	toolRepo := filepath.Join(t.TempDir(), "frozen-main")
	fetch := exec.CommandContext(t.Context(), "git", "-C", repo, "fetch", "--no-tags", "--depth=1", "origin", source)
	if raw, err := fetch.CombinedOutput(); err != nil {
		t.Fatalf("frozen main fetch failed: %v %s", err, raw)
	}
	if raw, err := exec.CommandContext(t.Context(), "git", "-C", repo, "worktree", "add", "--detach", toolRepo, source).CombinedOutput(); err != nil {
		t.Fatalf("owned frozen worktree: %v %s", err, raw)
	}
	t.Cleanup(func() { _ = exec.Command("git", "-C", repo, "worktree", "remove", "--force", toolRepo).Run() })
	bin := filepath.Join(t.TempDir(), "qs-migrate-offline")
	build := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-ldflags", "-X github.com/FangcunMount/qs-server/pkg/version.GitCommit="+source, "-o", bin, "./scripts/maintenance/qs-migrate-offline")
	build.Dir = toolRepo
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	if raw, err := build.CombinedOutput(); err != nil {
		t.Fatalf("frozen binary: %v %s", err, raw)
	}
	raw, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != productionBinarySHA {
		t.Fatal("compiled frozen CLI differs from actual production candidate", hex.EncodeToString(sum[:]))
	}
	n := openNativeSchemaPairWithPrefix(t, "qs_m5_commit_unknown_test")
	old := migrationResourcesThrough(t, 110, 44)
	n.approve(t, old)
	n.runBoth(t, n.preflight(t, old))
	n.cfg.ExpectedSourceSHA = source
	if _, err = n.db.ExecContext(t.Context(), "ALTER TABLE schema_migrations DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); err != nil {
		t.Fatal(err)
	}
	cfg, err := mysql.ParseDSN(os.Getenv("QS_M3_TEST_MYSQL_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = n.cfg.MySQLDatabase
	invoke := func(address string, args ...string) (M5RestoreResult, []byte, bool) {
		input := *cfg
		input.Addr = address
		command := exec.CommandContext(t.Context(), bin, append([]string{"--mysql-database", n.cfg.MySQLDatabase, "--mongo-database", n.cfg.MongoDatabase, "--timeout", "90s"}, args...)...)
		command.Env = append(os.Environ(), "MYSQL_TLS_MODE=DISABLED", "QS_OFFLINE_MYSQL_DSN="+input.FormatDSN(), "QS_OFFLINE_MONGO_URI="+os.Getenv("QS_SERVER_TEST_MONGO_URI"))
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		e := command.Run()
		var result M5RestoreResult
		_ = json.Unmarshal(stdout.Bytes(), &result)
		return result, stdout.Bytes(), e == nil
	}
	_, raw, ok := invoke(cfg.Addr, "--capture-original-operation", fmt.Sprintf("%d-1", time.Now().UnixNano()))
	var material M5OriginalMaterial
	if !ok || json.Unmarshal(raw, &material) != nil {
		t.Fatal("owned original export failed")
	}
	directory := t.TempDir()
	original := filepath.Join(directory, "original.json")
	if err = os.WriteFile(original, raw, 0600); err != nil {
		t.Fatal(err)
	}
	originalHash := retirementHash(raw)
	current, err := compiledMigrationResources()
	if err != nil {
		t.Fatal(err)
	}
	n.runBoth(t, n.preflight(t, current))
	if _, err = n.db.ExecContext(t.Context(), "UPDATE schema_migrations SET dirty=TRUE WHERE version=111"); err != nil {
		t.Fatal(err)
	}
	base := []string{"--restore-original", original, "--restore-original-sha256", originalHash}
	observed, _, ok := invoke(cfg.Addr, base...)
	if !ok || observed.State == nil || observed.State.SQLVersion != 111 || !observed.State.SQLDirty {
		t.Fatal("owned interrupted inspection failed")
	}
	now := time.Now().UTC().Truncate(time.Second)
	proof := strings.Repeat("b", 64)
	approval := M5RestoreApproval{FormatVersion: 1, Kind: "qs_m5_original_empty_restore", OperationID: material.OperationID, ApprovedSourceSHA: source, OriginalSHA256: originalHash, ObservedStateSHA256: observed.ObservedStateSHA256, FreezeReceiptSHA256: proof, BoundaryReceiptSHA256: proof, ReviewSHA256: proof, ExpiresAt: now.Add(10 * time.Minute).Format(time.RFC3339), RecoveryStartedAt: now.Format(time.RFC3339), DataOwner: "owned-fixture", RecoveryOwner: "owned-fixture", RuntimeFrozen: true, RuntimeNotStarted: true}
	authorization := filepath.Join(directory, "approval.json")
	raw, err = json.Marshal(approval)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(authorization, raw, 0600); err != nil {
		t.Fatal(err)
	}
	apply := append(append([]string{}, base...), "--restore-apply", "--restore-approval-file", authorization, "--restore-approval-sha256", retirementHash(raw))
	proxy, commits, dropped := m5DropCommitReply(t, cfg.Addr)
	started := time.Now()
	result, _, ok := invoke(proxy, apply...)
	if ok || !result.CommitOutcomeUnknown || !result.AuthorizationConsumed || !result.LedgerReplayAttempted || result.Verified || result.Stage != "ledger_commit" || commits.Load() != 1 || dropped.Load() != 1 {
		t.Fatal("actual commit reply loss was not preserved", result, commits.Load(), dropped.Load())
	}
	marker, err := os.ReadFile(authorization + ".consumed")
	if err != nil {
		t.Fatal(err)
	}
	inspection, _, ok := invoke(cfg.Addr, base...)
	if !ok || !inspection.AlreadyOriginal || inspection.State == nil || inspection.State.SQLVersion != 110 || inspection.State.SQLDirty || !inspection.State.TargetPresent || !reflect.DeepEqual(inspection.State.Objects, material.Baseline.Objects) {
		t.Fatal("fresh readonly connection did not verify actual complete original", inspection)
	}
	if _, _, ok = invoke(cfg.Addr, apply...); ok {
		t.Fatal("unknown commit approval replayed")
	}
	after, err := os.ReadFile(authorization + ".consumed")
	if err != nil || !bytes.Equal(marker, after) {
		t.Fatal("original consumed marker changed")
	}
	receipt, _ := json.Marshal(map[string]any{"source_sha": source, "binary_sha256": productionBinarySHA, "actual_cli": true, "actual_server_commit_ok_observed": true, "commit_reply_deliberately_lost": true, "commit_requests": commits.Load(), "commit_outcome_unknown_preserved": true, "fresh_readonly_complete110_44_verified": true, "all_retained_assets_identical": true, "consumed_approval_replay_refused": true, "seconds": time.Since(started).Seconds(), "fixture_only": true, "production_or_system_acceptance": false})
	t.Log("M5_FROZEN_MAIN_COMMIT_UNKNOWN_RECEIPT " + string(receipt))
}
