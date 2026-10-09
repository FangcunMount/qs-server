package main

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func TestAPrepareDescriptorRejectsEffectfulFieldsEvenNull(t *testing.T) {
	for _, key := range []string{"resume", "resume_kind", "ApprovedBSourceSHA", "migration_intent_sha256", "drop_ready"} {
		t.Run(key, func(t *testing.T) {
			for _, value := range []string{"null", "{}", `""`} {
				raw := []byte(`{"` + key + `":` + value + `}`)
				if _, err := decodeLifecycleStagingRequest(raw); err == nil {
					t.Fatalf("accepted effectful field %s", key)
				}
			}
		})
	}
}

func TestAPrepareEffectsRefuseBeforePrivateInputsOrConnections(t *testing.T) {
	for _, stage := range []string{"apply", "verify", "recover", "purge"} {
		t.Run(stage, func(t *testing.T) {
			r, err := runLifecycleCLI(context.Background(), "lifecycle-"+stage, "/nonexistent", "", "", "")
			if err == nil || lifecycleCategory(err) != "lifecycle_actual_host_adapters_missing" || r.Complete || r.ExecutionAllowed || r.DropReady || r.ArchiveBindingComplete {
				t.Fatalf("effectful A caller was not closed: %+v %v", r, err)
			}
		})
	}
}

func TestAPrepareImmutableImageComparisonRejectsMismatches(t *testing.T) {
	id := "sha256:" + strings.Repeat("a", 64)
	good, _ := json.Marshal([]lifecycleActualImage{{ID: id, Architecture: runtime.GOARCH}})
	if _, err := decodeLifecycleActualImage(good, id, runtime.GOARCH); err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{[]byte(`[]`), []byte(`null`), []byte(`{}`), []byte(`[{"Id":"tag","Architecture":"` + runtime.GOARCH + `"}]`), []byte(`[{"Id":"` + id + `","Architecture":"unknown"}]`), append(append([]byte(nil), good...), []byte(`{}`)...)} {
		if _, err := decodeLifecycleActualImage(raw, id, runtime.GOARCH); err == nil {
			t.Fatalf("image facts accepted mismatch %s", raw)
		}
	}
	expected := &lifecycleRestoreEngines{MySQLImageID: id, MongoImageID: id, Architecture: runtime.GOARCH}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, refusedContext := range []context.Context{nil, ctx} {
		if verifyLifecycleRestoreImages(refusedContext, expected) == nil {
			t.Fatal("missing or expired context reached actual image observer")
		}
	}
}
