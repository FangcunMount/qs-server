//go:build integration

package migration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
)

func TestM3InitialBindingCLINative(t *testing.T) {
	t.Setenv("QS_M3_REQUIRE_DATABASE", "1")
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	sourceRaw, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal("source identity unavailable")
	}
	source := strings.TrimSpace(string(sourceRaw))
	bin := filepath.Join(t.TempDir(), "qs-migrate-offline")
	buildCtx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	build := exec.CommandContext(buildCtx, "go", "build", "-ldflags", "-X github.com/FangcunMount/qs-server/pkg/version.GitCommit="+source, "-o", bin, "./scripts/maintenance/qs-migrate-offline")
	build.Dir = repo
	build.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	build.Cancel = func() error { return syscall.Kill(-build.Process.Pid, syscall.SIGKILL) }
	build.WaitDelay = 5 * time.Second
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("maintenance binary build failed: %v: %s", err, out)
	}
	type invocation struct {
		ok     bool
		output []byte
	}
	invoke := func(n nativeSchemaPair, args ...string) invocation {
		cfg, err := drivermysql.ParseDSN(os.Getenv("QS_M3_TEST_MYSQL_DSN"))
		if err != nil {
			return invocation{}
		}
		cfg.DBName = n.cfg.MySQLDatabase
		base := []string{"--mysql-database", n.cfg.MySQLDatabase, "--mongo-database", n.cfg.MongoDatabase, "--timeout", "30s"}
		ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, append(base, args...)...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "QS_OFFLINE_MYSQL_DSN="+cfg.FormatDSN(), "QS_OFFLINE_MONGO_URI="+os.Getenv("QS_SERVER_TEST_MONGO_URI"))
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
		cmd.WaitDelay = 5 * time.Second
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &bytes.Buffer{}
		err = cmd.Run()
		return invocation{err == nil, out.Bytes()}
	}
	preview := func(t *testing.T, n nativeSchemaPair, owner string) (string, string) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "initial.json")
		got := invoke(n, "--binding-action", "preview", "--binding-plan", path, "--binding-operation-id", "owned-ci-op", "--binding-owner", owner, "--binding-store-id", "owned-original", "--binding-writer-epoch", "m3-1")
		if !got.ok {
			t.Fatal("actual initial owner preview refused")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("private original plan not preserved")
		}
		return path, hex.EncodeToString(sum[:])
	}
	verify := func(t *testing.T, n nativeSchemaPair, path, digest string) {
		t.Helper()
		got := invoke(n, "--binding-action", "verify", "--binding-plan", path, "--binding-reviewed-sha256", digest)
		if !got.ok {
			t.Fatal("original binding verification failed")
		}
	}
	t.Run("two-real-installers-one-winner-per-owner-and-original-pair-unchanged", func(t *testing.T) {
		n, _ := offlineCurrentNativePair(t)
		sqlBefore, dirtyBefore, mongoBefore, mongoDirtyBefore := n.heads(t)
		for _, owner := range []string{"qs.mysql", "qs.mongo"} {
			path, digest := preview(t, n, owner)
			if got := invoke(n, "--binding-action", "apply", "--binding-plan", path, "--binding-reviewed-sha256", digest); got.ok {
				t.Fatal("missing freeze attestation accepted")
			}
			if got := invoke(n, "--binding-action", "apply", "--binding-plan", path, "--binding-reviewed-sha256", strings.Repeat("f", 64), "--binding-writers-excluded"); got.ok {
				t.Fatal("unreviewed raw bytes accepted")
			}
			results := make([]invocation, 2)
			var group sync.WaitGroup
			for i := range results {
				group.Add(1)
				go func(i int) {
					defer group.Done()
					results[i] = invoke(n, "--binding-action", "apply", "--binding-plan", path, "--binding-reviewed-sha256", digest, "--binding-writers-excluded")
				}(i)
			}
			group.Wait()
			passed := 0
			for _, got := range results {
				if got.ok {
					passed++
					var report struct {
						Result InitialBindingResult `json:"result"`
					}
					if err := json.Unmarshal(got.output, &report); err != nil || !report.Result.Verified || report.Result.Outcome != "committed" || report.Result.CutoverAuthorized {
						t.Fatal("commit/readback proof absent", err)
					}
				}
			}
			if passed != 1 {
				t.Fatalf("expected one actual installer winner for %s, got %d", owner, passed)
			}
			verify(t, n, path, digest)
			if got := invoke(n, "--binding-action", "apply", "--binding-plan", path, "--binding-reviewed-sha256", digest, "--binding-writers-excluded"); got.ok {
				t.Fatal("existing owner binding overwritten")
			}
		}
		sqlAfter, dirtyAfter, mongoAfter, mongoDirtyAfter := n.heads(t)
		if sqlBefore != sqlAfter || dirtyBefore != dirtyAfter || mongoBefore != mongoAfter || mongoDirtyBefore != mongoDirtyAfter {
			t.Fatal("binding installer changed migration metadata")
		}
		var bindings int
		if err := n.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM rm_tx_binding").Scan(&bindings); err != nil || bindings != 1 {
			t.Fatal("SQL binding count", err)
		}
		count, err := n.mongoDB.Collection("rm_tx_binding").CountDocuments(t.Context(), bson.M{})
		if err != nil || count != 1 {
			t.Fatal("Mongo binding count", err)
		}
	})
	t.Run("actual-new-protocol-responsibility-refuses-without-replacing-any-evidence", func(t *testing.T) {
		n, _ := offlineCurrentNativePair(t)
		sqlPlan, sqlDigest := preview(t, n, "qs.mysql")
		mongoPlan, mongoDigest := preview(t, n, "qs.mongo")
		if _, err := n.db.ExecContext(t.Context(), `INSERT INTO rm_tx_request VALUES(?,?,?,?,?,?)`, bytes.Repeat([]byte{'r'}, 32), []byte("qs.mysql"), []byte("owned"), []byte("new"), bytes.Repeat([]byte{'h'}, 32), bytes.Repeat([]byte{'e'}, 32)); err != nil {
			t.Fatal(err)
		}
		if _, err := n.mongoDB.Collection("rm_tx_request").InsertOne(t.Context(), bson.M{"_id": "owned-malformed-responsibility"}); err != nil {
			t.Fatal(err)
		}
		for _, p := range [][2]string{{sqlPlan, sqlDigest}, {mongoPlan, mongoDigest}} {
			if got := invoke(n, "--binding-action", "apply", "--binding-plan", p[0], "--binding-reviewed-sha256", p[1], "--binding-writers-excluded"); got.ok {
				t.Fatal("new responsibility bypassed")
			}
		}
		var bindings, requests int
		if err := n.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM rm_tx_binding").Scan(&bindings); err != nil || bindings != 0 {
			t.Fatal("SQL binding created despite original responsibility", err)
		}
		if err := n.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM rm_tx_request").Scan(&requests); err != nil || requests != 1 {
			t.Fatal("SQL responsibility removed", err)
		}
		count, err := n.mongoDB.Collection("rm_tx_binding").CountDocuments(t.Context(), bson.M{})
		if err != nil || count != 0 {
			t.Fatal("Mongo binding created despite original responsibility", err)
		}
		count, err = n.mongoDB.Collection("rm_tx_request").CountDocuments(t.Context(), bson.M{})
		if err != nil || count != 1 {
			t.Fatal("Mongo responsibility removed", err)
		}
	})
}
