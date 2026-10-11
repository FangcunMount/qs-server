package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"

	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

func TestLifecyclePrivateRequestRejectsAliasesLinksAndStreams(t *testing.T) {
	dir := t.TempDir()
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("private directory")
	}
	path := filepath.Join(dir, "request.json")
	for _, raw := range []string{`{"format_version":1,"Format_Version":1}`, `{"format_version":1,"recovery":{"Source_SHA":"x"}}`, `{"format_version":1,"format_version":1}`, `{"format_version":1} {}`} {
		if os.WriteFile(path, []byte(raw), 0600) != nil {
			t.Fatal("write private")
		}
		var request lifecycleRequest
		if readLifecyclePrivate(path, digestRaw([]byte(raw)), &request) == nil {
			t.Fatal("ambiguous request accepted")
		}
	}
	raw := []byte(`{"format_version":1}`)
	if os.WriteFile(path, raw, 0600) != nil || os.Link(path, filepath.Join(dir, "linked")) != nil {
		t.Fatal("link fixture")
	}
	var request lifecycleRequest
	if readLifecyclePrivate(path, digestRaw(raw), &request) == nil {
		t.Fatal("hardlinked input accepted")
	}
	if os.Remove(path) != nil || syscall.Mkfifo(path, 0600) != nil {
		t.Fatal("stream fixture")
	}
	if readLifecyclePrivate(path, digestRaw(raw), &request) == nil {
		t.Fatal("FIFO accepted")
	}
}
func TestLifecycleRequiresActualOpaqueRestoreProofs(t *testing.T) {
	prepared := &lifecyclePreparation{Borrowed: backup.TargetRecoveryBorrowed{SQL: new(sql.Conn), Mongo: new(mongo.Database)}, SQLRestore: new(backup.RestoreVerification), MongoRestore: new(backup.RestoreVerification)}
	if lifecyclePreparationMatches(new(backup.Archive), prepared) {
		t.Fatal("zero/imported proof accepted")
	}
}
func TestLifecycleConnectionInputsRejectLineInjectionBeforeConnect(t *testing.T) {
	t.Setenv("RETIREMENT_RESTORE_MYSQL_HOST", "host\nMYSQL_DATABASE=original")
	if _, err := lifecycleConnectionValue("RETIREMENT_RESTORE_", "MYSQL_HOST"); err == nil {
		t.Fatal("multiline credential accepted")
	}
	if _, _, err := prepareLifecycleNative(context.Background(), lifecycleRequest{}, nil); err == nil {
		t.Fatal("missing real archive/restore accepted")
	}
}

func TestLifecycleMongoUsesOnlyApprovedSeedWithoutReplicaDiscovery(t *testing.T) {
	opts := lifecycleMongoOptions("approved-seed.invalid", 27017, "fixture-user", "fixture-secret")
	if err := opts.Validate(); err != nil {
		t.Fatal("approved single-seed options rejected")
	}
	if opts.Direct == nil || !*opts.Direct || !reflect.DeepEqual(opts.Hosts, []string{"approved-seed.invalid:27017"}) {
		t.Fatal("root CLI could switch to an unresolvable replica-set member")
	}
	if opts.ReadPreference == nil || opts.ReadPreference.Mode() != readpref.PrimaryMode || opts.ReadConcern == nil || opts.ReadConcern.Level != "majority" {
		t.Fatal("direct seed weakened original primary/majority requirements")
	}
	if opts.ServerSelectionTimeout == nil || *opts.ServerSelectionTimeout != 10*time.Second || opts.SocketTimeout == nil || *opts.SocketTimeout != 30*time.Second || !reflect.DeepEqual(opts.Compressors, []string{"zstd", "snappy"}) {
		t.Fatal("bounded compressed connection options changed")
	}
}
