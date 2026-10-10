package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	aibridge "github.com/FangcunMount/qs-server/internal/apiserver/infra/aibridge"
	opts "github.com/FangcunMount/qs-server/internal/apiserver/options"
	jose "github.com/go-jose/go-jose/v4"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"text/template"
)

func prepareFactsRequestFixture(t *testing.T) prepareFactsRequest {
	t.Helper()
	previous := sourceSHA
	sourceSHA = strings.Repeat("a", 40)
	t.Cleanup(func() { sourceSHA = previous })
	return prepareFactsRequest{FormatVersion: 1, Kind: "readonly_prepare_facts_request", SourceSHA: sourceSHA,
		OperationID: "123-1", ActualRunID: "789-1", TargetHash: digest(targets), DatabaseScope: "mysql-and-mongodb",
		ObservationApprovalSHA256: strings.Repeat("b", 64), Inventory: prepareFactsProducer{OperationID: "123-1", RunID: "456-1", SourceSHA: strings.Repeat("c", 40), ReportSHA256: strings.Repeat("d", 64), RequestSHA256: strings.Repeat("e", 64)},
		RestoreEngines: &lifecycleRestoreEngines{MySQLImageID: "sha256:" + strings.Repeat("1", 64), MongoImageID: "sha256:" + strings.Repeat("2", 64), Architecture: runtime.GOARCH}, ArchiveDirectory: "/opt/backups/qs-server/compatibility-retirement/123-1/temporary-archive"}
}

func TestPrepareFactsKeepsOriginalProducerAndRejectsAuthorityFields(t *testing.T) {
	r := prepareFactsRequestFixture(t)
	if r.SourceSHA == r.Inventory.SourceSHA || validatePrepareFactsRequest(r, "123-1", "789-1") != nil {
		t.Fatal("separate producer binding rejected")
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{`"drop_ready":true`, `"ordered_mongo_schema_sha256":"approved"`, `"manifest_sha256":"approved"`} {
		var got prepareFactsRequest
		if decodePrepareFacts(append(raw[:len(raw)-1], []byte(","+extra+"}")...), &got) == nil {
			t.Fatal("authority field accepted")
		}
	}
	var got prepareFactsRequest
	if decodePrepareFacts([]byte(strings.Replace(string(raw), `"source_sha"`, `"Source_SHA"`, 1)), &got) == nil {
		t.Fatal("case alias accepted")
	}
	if decodePrepareFacts([]byte(strings.Replace(string(raw), `"format_version":1`, `"format_version":1,"format_version":1`, 1)), &got) == nil {
		t.Fatal("duplicate accepted")
	}
	mutations := []func(*prepareFactsRequest){
		func(v *prepareFactsRequest) { v.Inventory.SourceSHA = "unknown" },
		func(v *prepareFactsRequest) { v.Inventory.RunID = v.ActualRunID },
		func(v *prepareFactsRequest) { v.Inventory.OperationID = "999-1" },
		func(v *prepareFactsRequest) {
			v.RestoreEngines = &lifecycleRestoreEngines{MySQLImageID: "mysql:8", MongoImageID: "mongo:7", Architecture: runtime.GOARCH}
		},
		func(v *prepareFactsRequest) {
			v.ArchiveDirectory = "/opt/backups/qs-server/compatibility-retirement/123-1/inventory-456-1/child"
		},
	}
	for _, mutate := range mutations {
		got := r
		mutate(&got)
		if validatePrepareFactsRequest(got, "123-1", "789-1") == nil {
			t.Fatal("invalid binding accepted")
		}
	}
}

func TestPrepareFactsHashesPrivateBytesWithoutCopyOrDisclosure(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "source.private")
	body := []byte("PRIVATE_SOURCE_BODY_NOT_A_RECEIPT")
	if err = os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	fact, kept, err := hashPrepareFactsFile(context.Background(), path, uint32(os.Getuid()), 4096, false)
	if err != nil || kept != nil || fact.SHA256 != digestRaw(body) || fact.Bytes != uint64(len(body)) {
		t.Fatalf("actual hash failed: %v", err)
	}
	encoded, err := json.Marshal(fact)
	if err != nil || strings.Contains(string(encoded), string(body)) {
		t.Fatal("body disclosed")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("source copied")
	}
	if err = os.Link(path, filepath.Join(dir, "hardlink")); err != nil {
		t.Fatal(err)
	}
	if _, _, err = hashPrepareFactsFile(context.Background(), path, uint32(os.Getuid()), 4096, false); err == nil {
		t.Fatal("hardlink accepted")
	}
	if err = os.Remove(filepath.Join(dir, "hardlink")); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err = hashPrepareFactsFile(context.Background(), path, uint32(os.Getuid()), 4096, false); err == nil {
		t.Fatal("public source accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err = hashPrepareFactsFile(ctx, path, uint32(os.Getuid()), 4096, false); err == nil {
		t.Fatal("cancelled source read accepted")
	}
}

func TestPrepareFactsReportsActualCapacityWithoutReadiness(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	fact, err := observePrepareFactsFS("archive", dir)
	if err != nil || fact.Scope != "archive" || fact.PathSHA256 != digestRaw([]byte(dir)) || fact.TotalBytes == 0 || fact.FreeBytes > fact.TotalBytes || fact.AvailableBytes > fact.TotalBytes {
		t.Fatalf("actual statfs observation failed: %v", err)
	}
	parent, err := prepareFactsArchiveParent(filepath.Join(dir, "new", "leaf"))
	if err != nil || parent != dir {
		t.Fatal("wrong actual archive parent")
	}
	if err = os.Symlink(dir, filepath.Join(dir, "redirect")); err != nil {
		t.Fatal(err)
	}
	if _, err = observePrepareFactsFS("archive", filepath.Join(dir, "redirect")); err == nil {
		t.Fatal("symlink accepted")
	}
	receipt := prepareFactsReceipt{FormatVersion: 1, Kind: "readonly_prepare_facts_observation", DiagnosticOnly: true, FactsObservationComplete: true}
	if receipt.Complete || receipt.DropReady || receipt.ExecutionAllowed || receipt.OrderedMongoSchemaSHA256 != "" {
		t.Fatal("capacity observation minted authority")
	}
}

func TestPrepareFactsRequiresOriginalCompleteInventoryAndExactFourSources(t *testing.T) {
	r := prepareFactsRequestFixture(t)
	req := request{FormatVersion: 2, Kind: "readonly_inventory_request", OperationID: r.OperationID, SourceSHA: r.Inventory.SourceSHA,
		TargetHash: digest(targets), DatabaseScope: "mysql-and-mongodb", Limits: productionLimits(),
		Identities: map[string]string{"mysql": strings.Repeat("1", 64), "mongodb": strings.Repeat("2", 64)},
		Migrations: map[string]uint64{"mysql": 99, "mongodb": 38}, BoundaryRunID: "222-1", BoundaryReportHash: strings.Repeat("3", 64)}
	observed := report{FormatVersion: 2, Kind: "readonly_compatibility_inventory", SourceSHA: req.SourceSHA, OperationID: r.OperationID,
		RunID: r.Inventory.RunID, RequestHash: r.Inventory.RequestSHA256, TargetHash: digest(targets), Complete: true, DiagnosticOnly: true,
		BoundaryReportHash: req.BoundaryReportHash, ErrorCategory: "none", DatabaseBindings: map[string]databaseInventory{}}
	for _, db := range []string{"mysql", "mongodb"} {
		observed.DatabaseBindings[db] = databaseInventory{IdentityHash: req.Identities[db], DatabaseAnchorHash: strings.Repeat("4", 64), Version: req.Migrations[db], MetadataComplete: true, ExpectedIdentityMatch: true, ExpectedMigrationMatch: true, CatalogHash: strings.Repeat("5", 64)}
	}
	for i, target := range targets {
		b := targetBoundary{Database: target[0], Name: target[1], Kind: target[2], Present: true, Empty: true, SchemaHash: strings.Repeat("6", 64), IdentityHash: strings.Repeat("7", 64)}
		req.Boundaries = append(req.Boundaries, b)
		observed.Targets = append(observed.Targets, snapshot{Database: target[0], Name: target[1], Kind: target[2], Present: true, Complete: true, ErrorCategory: "none", Boundary: &b, Passes: 2, SourceFile: lifecycleSourceNames[i+3]})
	}
	if e := validatePrepareFactsInventory(r, req, observed); e != nil {
		t.Fatal(e)
	}
	mutations := []func(*report){func(v *report) { v.Complete = false }, func(v *report) { v.SourceSHA = sourceSHA }, func(v *report) { v.RequestHash = strings.Repeat("8", 64) }, func(v *report) { v.Targets[3].SourceFile = "wrong.source" }, func(v *report) { b := v.DatabaseBindings["mongodb"]; b.Dirty = true; v.DatabaseBindings["mongodb"] = b }, func(v *report) { v.Targets[0].Passes = 1 }}
	for _, mutate := range mutations {
		encoded, e := json.Marshal(observed)
		if e != nil {
			t.Fatal(e)
		}
		var changed report
		if e = json.Unmarshal(encoded, &changed); e != nil {
			t.Fatal(e)
		}
		mutate(&changed)
		if validatePrepareFactsInventory(r, req, changed) == nil {
			t.Fatal("unproven inventory accepted")
		}
	}
	req.Migrations["mysql"] = 100
	if validatePrepareFactsInventory(r, req, observed) == nil {
		t.Fatal("different schema phase accepted")
	}
}

func TestPrepareFactsRequestPathBindsActualRunWithoutChangingProducer(t *testing.T) {
	r := prepareFactsRequestFixture(t)
	path := "/opt/backups/qs-server/compatibility-retirement/123-1/prepare-facts-request-789-1.json"
	if !prepareFactsBoundRequestPath(path, r.OperationID, r.ActualRunID) {
		t.Fatal("exact actual run path rejected")
	}
	for _, invalid := range []struct{ path, op, run string }{
		{path, "123-1", "790-1"},
		{path, "124-1", "789-1"},
		{"/opt/backups/qs-server/compatibility-retirement/123-1/prepare-facts-request.json", "123-1", "789-1"},
		{path, "123-1", "../789-1"},
		{"/opt/backups/qs-server/compatibility-retirement/123-1/./prepare-facts-request-789-1.json", "123-1", "789-1"},
	} {
		if prepareFactsBoundRequestPath(invalid.path, invalid.op, invalid.run) {
			t.Fatal("conflicting run/path accepted")
		}
	}
	original := r.Inventory
	r.ActualRunID = "790-1"
	if validatePrepareFactsRequest(r, "123-1", "790-1") != nil || r.Inventory != original {
		t.Fatal("new actual run changed original inventory producer")
	}
	if validatePrepareFactsRequest(r, "123-1", "789-1") == nil {
		t.Fatal("request actual run conflict accepted")
	}
}

func TestPrepareFactsProtectionInspectFormatIncludesRuntimeArguments(t *testing.T) {
	tmpl, e := template.New("actual-inspect").Funcs(template.FuncMap{"json": func(v any) (string, error) {
		raw, e := json.Marshal(v)
		return string(raw), e
	}}).Parse(prepareFactsMQInspect)
	if e != nil {
		t.Fatal(e)
	}
	actual := map[string]any{"Id": strings.Repeat("1", 64), "Name": "/qs-apiserver", "Image": "sha256:" + strings.Repeat("2", 64), "State": map[string]any{"Running": true, "Pid": 123}, "Config": map[string]any{"User": "www", "Entrypoint": []string{"/app/qs-apiserver"}, "Cmd": []string{"--config=/app/configs/apiserver.prod.yaml"}, "Labels": map[string]string{"com.docker.compose.service": "qs-apiserver"}}, "Mounts": []any{}}
	var rendered bytes.Buffer
	var decoded prepareFactsMQContainer
	if e = tmpl.Execute(&rendered, actual); e != nil || json.Unmarshal(rendered.Bytes(), &decoded) != nil || len(decoded.Entrypoint) != 1 || decoded.Entrypoint[0] != "/app/qs-apiserver" || len(decoded.Command) != 1 || decoded.Command[0] != "--config=/app/configs/apiserver.prod.yaml" || decoded.User != "www" || decoded.PID != 123 {
		t.Fatal("actual fixed inspect format lost required runtime fields", e)
	}
}

func TestPrepareFactsProtectionUsesExistingHostLoaderAndClosedPrivatePacket(t *testing.T) {
	key := func(kid string) jose.JSONWebKey {
		t.Helper()
		k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			t.Fatal(e)
		}
		return jose.JSONWebKey{Key: k, KeyID: kid}
	}
	write := func(k jose.JSONWebKey) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), k.KeyID+".json")
		raw, e := json.Marshal(k)
		if e != nil || os.WriteFile(p, raw, 0600) != nil {
			t.Fatal("fixture creation")
		}
		return p
	}
	sign, crypt, old, aiSign, aiCrypt := key("qs.sign.v1"), key("qs.encrypt.v1"), key("qs.encrypt.old"), key("ai.sign.v1"), key("ai.encrypt.v1")
	options := opts.AIWorkflowMessagingOptions{SigningKeyFile: write(sign), AIRecipientKeyFile: write(aiCrypt.Public()), DecryptKeyFiles: map[string]string{crypt.KeyID: write(crypt), old.KeyID: write(old)}, AISignerFiles: map[string]string{aiSign.KeyID: write(aiSign.Public())}}
	keys, e := aibridge.LoadMessagingKeys(options)
	if e != nil {
		t.Fatal(e)
	}
	raw, publics, e := prepareFactsProtectionBytes(keys)
	if e != nil || len(publics) != 5 {
		t.Fatal("real host keys rejected")
	}
	var packet map[string]json.RawMessage
	if json.Unmarshal(raw, &packet) != nil || len(packet) != 2 || packet["decrypt_keys"] == nil || packet["trusted_signers"] == nil {
		t.Fatal("closed DTO rejected")
	}
	var decrypt map[string]map[string]any
	var signers map[string]map[string]json.RawMessage
	if json.Unmarshal(packet["decrypt_keys"], &decrypt) != nil || json.Unmarshal(packet["trusted_signers"], &signers) != nil || len(decrypt) != 2 || len(signers) != 1 {
		t.Fatal("ring scope wrong")
	}
	if decrypt[crypt.KeyID]["d"] == nil || decrypt[old.KeyID]["d"] == nil || len(signers[aiSign.KeyID]) != 2 || string(signers[aiSign.KeyID]["producer"]) != `"qs-ai"` {
		t.Fatal("private/producer role lost")
	}
	var public map[string]any
	if json.Unmarshal(signers[aiSign.KeyID]["key"], &public) != nil || public["d"] != nil {
		t.Fatal("private signer leaked")
	}
	for _, mutation := range []string{"empty", "public_decrypt", "private_signer", "wrong_producer", "wrong_kid"} {
		t.Run(mutation, func(t *testing.T) {
			clone, e := aibridge.LoadMessagingKeys(options)
			if e != nil {
				t.Fatal(e)
			}
			switch mutation {
			case "empty":
				clone.Ring.Decrypt = nil
			case "public_decrypt":
				clone.Ring.Decrypt[crypt.KeyID] = crypt.Public()
			case "private_signer":
				v := clone.Ring.Signers[aiSign.KeyID]
				v.Key = aiSign
				clone.Ring.Signers[aiSign.KeyID] = v
			case "wrong_producer":
				v := clone.Ring.Signers[aiSign.KeyID]
				v.Producer = "other"
				clone.Ring.Signers[aiSign.KeyID] = v
			case "wrong_kid":
				v := clone.Ring.Decrypt[crypt.KeyID]
				v.KeyID = "qs.encrypt.other"
				clone.Ring.Decrypt[crypt.KeyID] = v
			}
			if _, _, e = prepareFactsProtectionBytes(clone); e == nil {
				t.Fatal("unsafe packet accepted")
			}
		})
	}
	receipt := prepareFactsReceipt{Protection: &prepareFactsProtection{SHA256: digestRaw(raw), DecryptKeys: 2, TrustedSigners: 1, BindingSHA256: strings.Repeat("a", 64), SourceSHA: strings.Repeat("b", 40)}}
	publicReceipt, e := json.Marshal(receipt)
	if e != nil || strings.Contains(string(publicReceipt), `"decrypt_keys"`) || strings.Contains(string(publicReceipt), `"trusted_signers"`) || receipt.DropReady || receipt.Complete || receipt.ExecutionAllowed {
		t.Fatal("receipt leaked keys or authority")
	}
}

func TestPrepareFactsProtectionActualFDRejectsUnsafePrivateFiles(t *testing.T) {
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("dir")
	}
	p := filepath.Join(dir, "key.json")
	raw := []byte("synthetic-key-bytes")
	if os.WriteFile(p, raw, 0600) != nil {
		t.Fatal("fixture")
	}
	f, got, e := prepareFactsKeyFile(p, uint32(os.Getuid()), true, 16384)
	if e != nil || string(got) != string(raw) || f == nil {
		t.Fatal("actual FD rejected", e)
	}
	if f.Close() != nil {
		t.Fatal("close")
	}
	if os.Chmod(p, 0644) != nil {
		t.Fatal("chmod")
	}
	if f, _, e = prepareFactsKeyFile(p, uint32(os.Getuid()), true, 16384); e == nil || f != nil {
		t.Fatal("public private key accepted")
	}
	if os.Chmod(p, 0600) != nil {
		t.Fatal("chmod")
	}
	link := filepath.Join(dir, "hard")
	if os.Link(p, link) != nil {
		t.Fatal("link")
	}
	if f, _, e = prepareFactsKeyFile(p, uint32(os.Getuid()), true, 16384); e == nil || f != nil {
		t.Fatal("hardlink accepted")
	}
	if os.Remove(link) != nil {
		t.Fatal("unlink")
	}
	if os.Symlink(p, link) != nil {
		t.Fatal("symlink")
	}
	if f, _, e = prepareFactsKeyFile(link, uint32(os.Getuid()), true, 16384); e == nil || f != nil {
		t.Fatal("symlink accepted")
	}
}

func TestPrepareFactsProtectionReaderUIDComesFromActualProcess(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getuid() == 0 {
		t.Skip("actual nonroot Linux /proc required")
	}
	uid, e := prepareFactsContainerReaderUID(os.Getpid(), strconv.Itoa(os.Getuid()))
	if e != nil || uid != uint32(os.Getuid()) {
		t.Fatal("actual process UID rejected", e)
	}
	if _, e = prepareFactsContainerReaderUID(os.Getpid(), strconv.Itoa(os.Getuid()+1)); e == nil {
		t.Fatal("Config.User/actual process mismatch accepted")
	}
	for _, user := range []string{"root", "0", "1000:1000:1000", "untrusted-user", ""} {
		if _, e = prepareFactsContainerReaderUID(os.Getpid(), user); e == nil {
			t.Fatal("unbound reader accepted")
		}
	}
	if _, e = prepareFactsContainerReaderUID(0, "www"); e == nil {
		t.Fatal("missing PID accepted")
	}
}
