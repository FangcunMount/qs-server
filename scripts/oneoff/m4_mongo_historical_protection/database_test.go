package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.mongodb.org/mongo-driver/bson"
)

func privateDir(t *testing.T) string {
	t.Helper()
	p, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestFirstMaterialCaptureIsDurablePrivateAndNeverOverwrites(t *testing.T) {
	dir := privateDir(t)
	path := filepath.Join(dir, "first-material.json")
	raw := []byte(`{"fixed_metadata":"original"}`)
	if e := writeExclusive(path, raw); e != nil {
		t.Fatal(e)
	}
	st, e := os.Lstat(path)
	if e != nil || st.Mode().Perm() != 0600 {
		t.Fatal("private mode lost")
	}
	got, e := readPrivate(path, fingerprint(raw), maxMaterialBytes)
	if e != nil || !bytes.Equal(got, raw) {
		t.Fatal("durable first material not read back exactly")
	}
	if writeExclusive(path, []byte(`{"replacement":true}`)) == nil {
		t.Fatal("first capture overwritten")
	}
	after, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(after, raw) {
		t.Fatal("original bytes changed")
	}
	for name, prepare := range map[string]func(string){
		"public_mode": func(p string) {
			if os.Chmod(p, 0644) != nil {
				t.Fatal("chmod")
			}
		},
		"hardlink": func(p string) {
			if os.Link(p, p+".link") != nil {
				t.Fatal("link")
			}
		},
		"symlink": func(p string) {
			if os.Rename(p, p+".original") != nil || os.Symlink(p+".original", p) != nil {
				t.Fatal("symlink")
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(privateDir(t), "material")
			if writeExclusive(p, raw) != nil {
				t.Fatal("write")
			}
			prepare(p)
			if _, e := readPrivate(p, fingerprint(raw), maxMaterialBytes); e == nil {
				t.Fatal("unsafe material accepted")
			}
		})
	}
	if _, e := readPrivate(path, strings.Repeat("e", 64), maxMaterialBytes); e == nil {
		t.Fatal("self-selected material hash accepted")
	}
}
func TestCaptureNeverCertifiesExpiredBudgetOrFailedFileRelease(t *testing.T) {
	path := filepath.Join(privateDir(t), "material.json")
	parent, e := captureParent(path)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	r := receipt{}
	e = finishCapture(ctx, &r, path, parent, []byte("fixed"), func(string, []byte) error { called = true; return nil })
	if e == nil || called || r.Complete || r.MaterialHash != "" {
		t.Fatal("expired capture performed a write")
	}
	e = finishCapture(context.Background(), &r, path, parent, []byte("fixed"), func(string, []byte) error { return errors.New("private_directory_release_failed") })
	if e == nil || r.Complete || r.MaterialHash != "" {
		t.Fatal("failed fsync/release signed material")
	}
	ctx, cancel = context.WithCancel(context.Background())
	e = finishCapture(ctx, &r, path, parent, []byte("fixed"), func(p string, raw []byte) error {
		if e := writeExclusive(p, raw); e != nil {
			return e
		}
		cancel()
		return nil
	})
	if e == nil || r.Complete || r.MaterialHash != "" {
		t.Fatal("after-write cancellation signed material identity or completed")
	}
}
func TestSQLIdentityRequiresObservedSingleClean101Ledger(t *testing.T) {
	cases := []struct {
		name    string
		version uint64
		dirty   bool
		rows    int
		server  string
		valid   bool
	}{
		{"clean101", 101, false, 1, "8.0.36", true},
		{"old100", 100, false, 1, "8.0.36", false},
		{"future102", 102, false, 1, "8.0.36", false},
		{"dirty", 101, true, 1, "8.0.36", false},
		{"missing", 101, false, 0, "8.0.36", false},
		{"multiple", 101, false, 2, "8.0.36", false},
		{"different_patch", 101, false, 0, "8.0.37", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, e := sqlmock.New()
			if e != nil {
				t.Fatal(e)
			}
			defer func() { _ = db.Close() }()
			mock.ExpectQuery("SELECT @@server_uuid,DATABASE\\(\\),VERSION\\(\\)").WillReturnRows(sqlmock.NewRows([]string{"uuid", "db", "version"}).AddRow("00000000-0000-0000-0000-000000000001", "qs_owned", tc.server))
			if tc.server == "8.0.36" {
				rows := sqlmock.NewRows([]string{"version", "dirty"})
				for i := 0; i < tc.rows; i++ {
					rows.AddRow(tc.version, tc.dirty)
				}
				mock.ExpectQuery("SELECT version,dirty FROM schema_migrations ORDER BY version LIMIT 2").WillReturnRows(rows)
			}
			d := database{sqlName: "qs_owned"}
			hash, v, dirty, e := d.sqlState(context.Background(), db)
			if tc.valid {
				if e != nil || v != 101 || dirty || !nonzeroHash(hash) {
					t.Fatal("clean observation rejected")
				}
			} else if e == nil || hash != "" {
				t.Fatal("unproven ledger accepted")
			}
			if e := mock.ExpectationsWereMet(); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestCatalogComparisonKeepsRawSpecOrderAndAllMetadata(t *testing.T) {
	m := fixtureMaterial(t)
	a := [3]namespaceProof{m.Collections[0], m.Collections[1], m.Ledger.Namespace}
	b := [3]namespaceProof{m.Collections[0], m.Collections[1], m.Ledger.Namespace}
	n := int64(99)
	b[0].Count = &n
	if !catalogEqual(a, b) {
		t.Fatal("catalog comparison incorrectly depends on snapshot count")
	}
	b[0].Options = rawBSON(t, bson.D{{Key: "capped", Value: true}})
	if catalogEqual(a, b) {
		t.Fatal("collection option change hidden")
	}
	b = a
	b[0].Indexes = append([][]byte(nil), a[0].Indexes...)
	b[0].Indexes[0] = append(append([]byte(nil), b[0].Indexes[0]...), 0)
	if catalogEqual(a, b) {
		t.Fatal("ordered raw index change hidden")
	}
}
func TestCanceledEntryReleasesOnlyOwnedPoolAndDoesNotCreateMaterial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := filepath.Join(privateDir(t), "never-written")
	r := run(ctx, []string{"--action=capture", "--mysql-database=qs_owned", "--mongo-database=qs_owned", "--capture-file=" + p}, func(key string) string {
		if key == "QS_M4_OBJECT_MYSQL_DSN" {
			return "fixture:private-fixture-password@tcp(127.0.0.1:1)/qs_owned"
		}
		return "mongodb://127.0.0.1:1/qs_owned"
	}, testSource, nil)
	if r.Complete || r.Error != "mysql_connection_failed" || r.SQLPoolClosed == nil || !*r.SQLPoolClosed || r.SQLConnectionClosed != nil || r.MongoClosed != nil || r.MongoCursorsClosed != nil || !r.BudgetExceeded {
		t.Fatalf("incorrect owned release/cancellation evidence: %+v", r)
	}
	if _, e := os.Lstat(p); !os.IsNotExist(e) {
		t.Fatal("canceled first capture created a file")
	}

}

func TestClientClearsURICSOTWithoutEnablingZeroTimeout(t *testing.T) {
	opts := objectClientOptions("mongodb://127.0.0.1:27017/qs_owned?timeoutMS=5000&socketTimeoutMS=5000&connectTimeoutMS=9000&retryReads=true&retryWrites=true")
	if opts.Validate() != nil || opts.Timeout != nil || opts.SocketTimeout == nil || *opts.SocketTimeout != 0 || opts.ConnectTimeout == nil || *opts.ConnectTimeout != 5*time.Second || opts.ServerSelectionTimeout == nil || *opts.ServerSelectionTimeout != 5*time.Second || opts.RetryReads == nil || *opts.RetryReads || opts.RetryWrites == nil || *opts.RetryWrites {
		t.Fatal("explicit maxTimeMS command still subject to CSOT/URI timeout or automatic read/write retry")
	}
}
