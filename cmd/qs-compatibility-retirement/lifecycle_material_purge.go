package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
	"github.com/FangcunMount/qs-server/pkg/version"
)

// Issued only after this host's native data, runtime and current-ledger reads.
// Sealed becomes true only after original D zero/terminal and actual A/AI FD
// handoff. A DTO, restored fixture or process exit cannot construct this owner.
type lifecycleAcceptedMaterials struct {
	self    *lifecycleAcceptedMaterials
	host    *lifecycleFixedHost
	binding lifecycleMaterialBinding
	catalog *lifecycleBatchMaterials
	runtime *lifecycleControlledRuntime
	sealed  bool
}

type lifecycleMaterialBinding struct {
	original, tool, operation, originalRun, actualRun, manifest, archive, request string
}

func lifecycleMaterialsBinding(r lifecycleRequest) lifecycleMaterialBinding {
	return lifecycleMaterialBinding{r.OriginalSourceSHA, r.ToolSourceSHA, r.OperationID, r.Recovery.OriginalRunID, r.ActualRunID, r.ManifestSHA256, r.Recovery.ArchiveSHA256, r.requestSHA256}
}
func (b lifecycleMaterialBinding) valid() bool {
	return shaRE.MatchString(b.original) && shaRE.MatchString(b.tool) && runRE.MatchString(b.operation) && runRE.MatchString(b.originalRun) && runRE.MatchString(b.actualRun) && hashRE.MatchString(b.manifest) && hashRE.MatchString(b.archive) && hashRE.MatchString(b.request)
}

func (b lifecycleMaterialBinding) digest() string {
	return digestRaw([]byte(strings.Join([]string{b.original, b.tool, b.operation, b.originalRun, b.actualRun, b.manifest, b.archive, b.request}, "\n")))
}

// Scopes are completed only by the future actual producer's explicit, complete
// registration. They are not inferred from directory names, glob matches, an
// archive's absence, or a JSON "complete" flag. Missing any scope blocks every
// mutation, including engine stop. Ordinary backups/previous batches/business
// evidence and current MQ facts have no registration path in this catalog.
type lifecycleMaterialScope uint8

const (
	lifecycleOriginalInventoryMaterials lifecycleMaterialScope = iota
	lifecycleFullSourceCASMaterials
	lifecycleRootStagingMaterials
	lifecycleAPIJournalMaterials
	lifecycleLocalServiceMaterials
	lifecycleRemoteServiceMaterials
	lifecycleArchiveMaterials
	lifecycleRestoreMaterials
	lifecycleMaterialScopeCount
)

// Only purgeAcceptedRemoteMaterials constructs this D-scoped observation from
// the original native owned-cleanup reply AND that same SSH child's successful
// actual terminal result. Neither SSH Wait alone nor a saved response/copy or
// new connection constructs it. All other material scopes remain mandatory.
type lifecycleRemoteMaterialZero struct {
	self     *lifecycleRemoteMaterialZero
	binding  lifecycleMaterialBinding
	native   *stop.RemoteMaterialZero
	services *lifecycleServiceController
	terminal *lifecycleDTerminal
}

type lifecycleBatchMaterials struct {
	self                             *lifecycleBatchMaterials
	binding                          lifecycleMaterialBinding
	scopes                           map[lifecycleMaterialScope]struct{}
	remote                           *lifecycleRemoteMaterialZero
	local                            *stop.RootRemoteMaterials
	directories                      []*lifecycleMaterialDirectory
	archive                          *lifecycleMaterialDirectory
	census                           *lifecycleMaterialDirectory
	engines                          []*lifecycleEnginePurge
	journal                          *lifecycleMaterialDirectory
	started, purged, closed, unknown bool
	zeroVerified                     bool
	localSealed                      bool
}

// The fixed backup producer registered exactly these six files before copying
// any body. Reopening them consumes the native Archive's verified immutable
// assets; ReadDir may reject extras but cannot expand this deletion scope.
func openLifecycleOriginalArchiveMaterials(ctx context.Context, r lifecycleRequest, a *backup.Archive) (owned *lifecycleMaterialDirectory, result error) {
	if ctx == nil || ctx.Err() != nil || a == nil || backup.VerifyHostArchiveBinding(ctx, a, r.Approval) != nil || a.Summary().ArchiveSHA256 != r.Recovery.ArchiveSHA256 {
		return nil, lifecycleError("lifecycle_material_registration_rejected")
	}
	d, err := openLifecycleMaterialDirectory(r.ArchiveDirectory, 0)
	if err != nil {
		return nil, err
	}
	defer func() {
		if result != nil {
			_ = d.close()
		}
	}()
	d.externalArchive = true // The existing exact backup backend removes it.
	assets := a.TemporarySourceAssets()
	names := []string{}
	for index, asset := range assets {
		if ctx.Err() != nil || asset.Filename != lifecycleSourceNames[index+3] {
			return nil, lifecycleError("lifecycle_material_registration_rejected")
		}
		if err = d.register(asset.Filename, asset.SHA256, 0, 0600); err != nil {
			return nil, err
		}
		if d.files[asset.Filename].info.Size() != asset.Bytes {
			return nil, lifecycleError("lifecycle_material_registration_rejected")
		}
		names = append(names, asset.Filename)
	}
	names = append(names, "archive.private.json")
	if err = d.register("archive.private.json", r.Recovery.ArchiveSHA256, 0, 0600); err != nil {
		return nil, err
	}
	// This exact order and schema match backup.registration's original writer.
	registration := struct {
		Version                int             `json:"version"`
		Approval               backup.Approval `json:"approval"`
		Files                  []string        `json:"files"`
		ContainsOriginalBodies bool            `json:"contains_original_bodies"`
		PurgeAfterAcceptance   bool            `json:"purge_after_acceptance"`
		ResumeAllowed          bool            `json:"resume_allowed"`
	}{1, r.Approval, names, true, true, false}
	raw, err := json.Marshal(registration)
	if err != nil {
		return nil, err
	}
	if err = d.register("assets.private.json", digestRaw(raw), 0, 0600); err != nil {
		return nil, err
	}
	if err = d.checkComplete(false); err != nil {
		return nil, err
	}
	return d, nil
}

func registerLifecycleCurrentRestoreMetadata(ctx context.Context, d *lifecycleMaterialDirectory, owner *lifecyclePreparationOwner, r lifecycleRequest) error {
	if ctx == nil || ctx.Err() != nil || d == nil || d.path != r.prepareRoot || owner == nil || len(owner.engines) != 2 || len(owner.materialRecords) != 5 {
		return lifecycleError("lifecycle_restore_owned_material_binding_rejected")
	}
	expected := map[string]string{"lifecycle-restore-" + r.ActualRunID + ".registration.private.json": owner.materialRecords["lifecycle-restore-"+r.ActualRunID+".registration.private.json"]}
	for _, engine := range owner.engines {
		if engine == nil || !lifecycleEngineWiresTerminal(engine) || len(engine.materialRecords) != 2 {
			return lifecycleError("lifecycle_restore_owned_material_binding_rejected")
		}
		nonce, err := hex.DecodeString(engine.Owner)
		if err != nil || len(nonce) != 16 || engine.Owner != hex.EncodeToString(nonce) || engine.Labels["qs.retirement.operation"] != r.OperationID || engine.Labels["qs.retirement.run"] != r.ActualRunID {
			return lifecycleError("lifecycle_restore_owned_material_binding_rejected")
		}
		for _, suffix := range []string{".intent.private.json", ".created.private.json"} {
			name := "restore-" + engine.Owner + suffix
			hash := engine.materialRecords[name]
			if expected[name] != "" || !hashRE.MatchString(hash) || owner.materialRecords[name] != hash {
				return lifecycleError("lifecycle_restore_owned_material_binding_rejected")
			}
			expected[name] = hash
		}
	}
	if !reflect.DeepEqual(expected, owner.materialRecords) {
		return lifecycleError("lifecycle_restore_owned_material_binding_rejected")
	}
	for name, hash := range expected {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := d.register(name, hash, 0, 0600); err != nil {
			return err
		}
	}
	return nil
}

// Compose only directly available original owners after the native read/fence.
// Retain partial FD ownership even on failure so host Close can release it.
// Root/inventory/source/restore-receipt/archive registration remains absent;
// no complete batch is inferred from A's actual owner or these resource leaves.
func (h *lifecycleFixedHost) composeNativeMaterialOwners(ctx context.Context, r lifecycleRequest) (result error) {
	b := lifecycleMaterialsBinding(r)
	if h == nil || ctx == nil || ctx.Err() != nil || runtime.GOOS != "linux" || os.Getuid() != 0 || os.Geteuid() != 0 || version.GitCommit != sourceSHA || !b.valid() || b.tool != sourceSHA || r.prepareRoot != lifecycleInvocationBatch(r.OperationID, r.ActualRunID) || h.materials != nil || h.acceptedMaterials != nil || h.currentMQ == nil || h.api == nil || h.api.self != h.api || h.api.unknown || h.api.materials == nil || h.api.acceptance == nil || h.api.acceptance.self != h.api.acceptance || h.api.acceptance.owner != h.api || lifecycleMaterialsBinding(h.api.request) != b || h.api.dir != filepath.Join(r.prepareRoot, "api-transition") || h.api.materials.path != h.api.dir || !h.api.bProgramVerified || !h.api.rollbackProgramVerified || h.restoreOwner == nil || len(h.restoreOwner.engines) != 2 {
		return lifecycleError("lifecycle_actual_complete_material_scope_missing")
	}
	c := &lifecycleBatchMaterials{binding: b, scopes: map[lifecycleMaterialScope]struct{}{}}
	c.self = c
	h.materials = c
	defer func() {
		if result != nil {
			c.unknown = true // A failed partial handoff can never be completed/reused.
		}
	}()
	if h.services == nil || h.services.materials == nil || !h.services.identity.matches(r) || h.services.materials.CheckLocalMaterials(ctx, h.services.issuer, h.services.local) != nil {
		return lifecycleError("lifecycle_actual_complete_material_scope_missing")
	}
	c.local = h.services.materials
	c.scopes[lifecycleLocalServiceMaterials] = struct{}{}
	root, e := openLifecycleMaterialDirectory(r.prepareRoot, 0)
	if e != nil {
		return e
	}
	c.directories = append(c.directories, root)
	journalPath := filepath.Join(r.prepareRoot, "material-purge-receipts")
	if e = os.Mkdir(journalPath, 0700); e != nil {
		return lifecycleError("lifecycle_material_registration_rejected")
	}
	c.journal, e = openLifecycleMaterialDirectory(journalPath, 0)
	if e != nil {
		return e
	}
	if e = root.registerChild(c.journal); e != nil {
		return e
	}
	if e = validateLifecycleAPIInvocation(r); e != nil {
		return e
	}
	intentRaw, e := readLifecycleAPIRecord(filepath.Join(r.prepareRoot, "native-call.intent.private.json"))
	if e != nil {
		return e
	}
	for name, hash := range map[string]string{"native-call.intent.private.json": digestRaw(intentRaw), "lifecycle-request.json": r.requestSHA256, "manifest.json": r.ManifestSHA256} {
		if e = root.register(name, hash, 0, 0600); e != nil {
			return e
		}
	}
	if v := h.dbWriters; v != nil && v.inputRecord != nil {
		f := v.inputRecord
		if v.self != v || v.host != h || v.binding != lifecycleWindowBinding(r) || v.actualRunID != r.ActualRunID || !v.installed || !v.restored || f.Path != filepath.Join(r.prepareRoot, "database-writer-expectations.private.json") || !hashRE.MatchString(f.SHA256) {
			return lifecycleError("lifecycle_database_writer_record_rejected")
		}
		if e = root.register(filepath.Base(f.Path), f.SHA256, 0, 0600); e != nil {
			return e
		}
	}
	if e = root.registerChild(h.api.materials); e != nil {
		return e
	}
	if e = h.api.materials.checkComplete(false); e != nil {
		return e
	}
	c.scopes[lifecycleAPIJournalMaterials] = struct{}{}
	if h.inventoryMaterials == nil || h.inventoryMaterials.checkComplete(false) != nil || h.rootStagingMaterials == nil || h.rootStagingMaterials.checkComplete(false) != nil || h.historicalWriteMaterials == nil || h.historicalWriteMaterials.checkComplete(false) != nil || h.archiveMaterials == nil || h.archiveMaterials.checkComplete(false) != nil {
		return lifecycleError("lifecycle_actual_complete_material_scope_missing")
	}
	var operation *lifecycleMaterialDirectory
	operationPath := filepath.Join("/opt/backups/qs-server/compatibility-retirement", r.OperationID)
	for _, d := range h.historicalWritePreviousMaterials {
		if d != nil && d.path == operationPath {
			if operation != nil {
				return lifecycleError("lifecycle_material_registration_rejected")
			}
			operation = d
		}
	}
	invocation, e := decodeLifecycleAPIInvocationIntent(intentRaw)
	if e != nil {
		return lifecycleError("lifecycle_material_registration_rejected")
	}
	c.census, e = openLifecycleOriginalCensusMaterials(ctx, r, invocation.SourceUID)
	if e != nil {
		return e
	}
	c.directories = append(c.directories, c.census)
	if e = completeLifecycleOriginalOperationMaterials(ctx, r, operation, h.inventoryMaterials, h.historicalWriteMaterials, h.historicalWriteRegistrationMaterials, h.historicalWritePreviousMaterials, h.archiveMaterials, invocation.SourceUID); e != nil {
		return e
	}
	c.directories = append(c.directories, operation, h.rootStagingMaterials)
	c.scopes[lifecycleOriginalInventoryMaterials] = struct{}{}
	c.scopes[lifecycleFullSourceCASMaterials] = struct{}{}
	if h.preparationInvocationMaterials != nil {
		if e = h.preparationInvocationMaterials.checkComplete(false); e != nil {
			return e
		}
		c.directories = append(c.directories, h.preparationInvocationMaterials)
	}
	c.scopes[lifecycleRootStagingMaterials] = struct{}{}
	c.archive = h.archiveMaterials
	c.scopes[lifecycleArchiveMaterials] = struct{}{}
	if e = registerLifecycleCurrentRestoreMetadata(ctx, root, h.restoreOwner, r); e != nil {
		return e
	}
	for i, engine := range h.restoreOwner.engines {
		if engine == nil || i == 0 && engine.Kind != "mysql" || i == 1 && engine.Kind != "mongodb" {
			return lifecycleError("lifecycle_restore_owned_material_binding_rejected")
		}
		registered, err := registerLifecycleEnginePurge(ctx, b, engine)
		if err != nil {
			return err
		}
		c.engines = append(c.engines, registered)
	}
	if e = checkLifecycleRestoreMaterialSet(ctx, c.engines, false); e != nil {
		return e
	}
	c.scopes[lifecycleRestoreMaterials] = struct{}{}
	// The preceding history producers, local service issuer and live journals
	// still have separate native owners. A partial composition is not acceptance.
	return nil
}

func lifecycleMaterialPathsMatch(c *lifecycleBatchMaterials, r lifecycleRequest) bool {
	if c.archive == nil || c.census == nil || c.archive.path != r.ArchiveDirectory || r.prepareRoot != lifecycleInvocationBatch(r.OperationID, r.ActualRunID) || c.journal == nil || c.journal.path != filepath.Join(r.prepareRoot, "material-purge-receipts") {
		return false
	}
	original := filepath.Join("/opt/backups/qs-server/compatibility-retirement", r.OperationID)
	allowed := map[string]bool{
		original: true,
		filepath.Join(original, "inventory-"+r.Recovery.OriginalRunID): true,
		lifecycleRootBatch(r.OperationID, r.ActualRunID):               true,
		r.prepareRoot: true,
		lifecycleServicesRoot(r.OperationID, "server-a"): true,
		c.census.path: true, // Exact original native census owner, separately registered.
	}
	// The original preparation root is supplied by an independently approved
	// source-copy intent and registered from its actual producer tuple/bytes.
	// Current-run names and archive paths never imply a previous preparation run.
	if lifecycleSourceCopyReferenceValid(r) {
		root := filepath.Dir(r.SourceCopyIntent.Path)
		run := strings.TrimPrefix(filepath.Base(root), r.OperationID+"-")
		allowed[root] = true
		allowed[lifecycleInvocationBatch(r.OperationID, run)] = true
	}
	if lifecycleHistoricalWriteReferenceValid(r) {
		allowed[filepath.Dir(r.HistoricalWriteReport.Path)] = true
	}
	for _, d := range c.directories {
		if d == nil || !allowed[d.path] || d.path == c.archive.path {
			return false
		}
	}
	return len(c.directories) > 0
}

func (h *lifecycleFixedHost) acceptedBatchMaterials(ctx context.Context, r lifecycleRequest) (*lifecycleBatchMaterials, error) {
	if h == nil || ctx == nil || ctx.Err() != nil || runtime.GOOS != "linux" || os.Getuid() != 0 || os.Geteuid() != 0 || version.GitCommit != sourceSHA {
		return nil, lifecycleError("lifecycle_actual_batch_acceptance_missing")
	}
	a := h.acceptedMaterials
	if a == nil || a.self != a || a.host != h || !a.sealed || a.runtime.validate(h) != nil || a.binding != lifecycleMaterialsBinding(r) || !a.binding.valid() || a.binding.tool != sourceSHA || a.catalog == nil || a.catalog != h.materials || a.catalog.self != a.catalog || a.catalog.binding != a.binding {
		return nil, lifecycleError("lifecycle_actual_batch_acceptance_missing")
	}
	c := a.catalog
	if c.closed || c.unknown || c.archive == nil || c.journal == nil || len(c.engines) != 2 || h.restoreOwner == nil || len(h.restoreOwner.engines) != 2 || h.services == nil || c.local == nil || c.local != h.services.materials || c.remote == nil || c.remote.self != c.remote || c.remote.binding != c.binding || c.remote.native == nil || c.remote.services != h.services || len(c.scopes) != int(lifecycleMaterialScopeCount) || !lifecycleMaterialPathsMatch(c, r) {
		return nil, lifecycleError("lifecycle_actual_complete_material_scope_missing")
	}
	if z, e := c.remote.native.Snapshot(); e != nil || z.RemainingTemporaryFiles != 0 {
		return nil, lifecycleError("lifecycle_material_zero_unproven")
	}
	if c.remote.terminal == nil || c.remote.terminal.native != c.remote.native || c.remote.terminal.validate(ctx, h, r) != nil {
		return nil, lifecycleError("lifecycle_material_zero_unproven")
	}
	for scope := lifecycleMaterialScope(0); scope < lifecycleMaterialScopeCount; scope++ {
		if _, ok := c.scopes[scope]; !ok {
			return nil, lifecycleError("lifecycle_actual_complete_material_scope_missing")
		}
	}
	for i, engine := range c.engines {
		if engine == nil || engine.engine != h.restoreOwner.engines[i] || engine.binding != c.binding {
			return nil, lifecycleError("lifecycle_material_registration_rejected")
		}
	}
	return c, nil
}

// Consume only the original A owner after the native D zero/terminal and the
// last broader writer check. Seal closes the actual Lease and budget issuer;
// callbacks bind existing held identities, never infer ownership from a tree.
func (h *lifecycleFixedHost) registerLocalServiceMaterials(ctx context.Context, r lifecycleRequest, c *lifecycleBatchMaterials) (result error) {
	if h == nil || h.services == nil || c == nil || c != h.materials || c.self != c || c.binding != lifecycleMaterialsBinding(r) || c.closed || c.unknown || c.started || c.local == nil || c.local != h.services.materials || c.remote == nil || c.remote.terminal == nil || h.acceptedMaterials == nil || h.acceptedMaterials.catalog != c {
		return lifecycleError("lifecycle_actual_complete_material_scope_missing")
	}
	if c.localSealed {
		return c.local.LocalMaterialsSealed(h.services.issuer, h.services.local)
	}
	if result = h.observeWholeWriterScopesAfterDTerminal(ctx, r, c.remote.terminal); result != nil {
		return result
	}
	defer func() {
		if result != nil {
			c.unknown = true
		}
	}()
	root := lifecycleServicesRoot(r.OperationID, "server-a")
	dirs := map[string]*lifecycleMaterialDirectory{}
	result = c.local.SealLocalMaterials(ctx, h.services.issuer, h.services.local, h.services.remote, c.remote.native, func(path string, held *os.File) error {
		if dirs[path] != nil || path != root && path != filepath.Join(root, "service-journal") && path != filepath.Join(root, "budget-issuer") && path != filepath.Join(root, "ssh") || held == nil {
			return lifecycleError("lifecycle_material_registration_rejected")
		}
		d, e := openLifecycleMaterialDirectory(path, 0)
		if e != nil {
			return e
		}
		before, e := held.Stat()
		if e != nil || !sameLifecycleFile(before, d.info) {
			_ = d.close()
			return lifecycleError("lifecycle_material_registration_rejected")
		}
		dirs[path] = d
		if path == root {
			c.directories = append(c.directories, d)
		} else if dirs[root] == nil || dirs[root].registerChild(d) != nil {
			_ = d.close()
			return lifecycleError("lifecycle_material_registration_rejected")
		}
		return nil
	}, func(path string, held *os.File, hash string) error {
		d := dirs[filepath.Dir(path)]
		if d == nil || held == nil {
			return lifecycleError("lifecycle_material_registration_rejected")
		}
		before, e := held.Stat()
		if e != nil || d.register(filepath.Base(path), hash, 0, before.Mode().Perm()) != nil {
			return lifecycleError("lifecycle_material_registration_rejected")
		}
		after, e := held.Stat()
		if e != nil || !sameLifecycleFile(before, after) || !sameLifecycleFile(after, d.files[filepath.Base(path)].info) {
			return lifecycleError("lifecycle_material_registration_rejected")
		}
		return nil
	})
	if result != nil {
		return result
	}
	if len(dirs) != 4 || dirs[root] == nil || dirs[root].checkComplete(false) != nil || c.local.LocalMaterialsSealed(h.services.issuer, h.services.local) != nil {
		return lifecycleError("lifecycle_actual_complete_material_scope_missing")
	}
	c.localSealed = true
	return nil
}

// Consume the original runtime acceptance before D closes. Register both real
// journal identities before closing their writers; this grants no purge permit.
func (h *lifecycleFixedHost) registerAIStoppedMaterials(ctx context.Context, r lifecycleRequest, c *lifecycleBatchMaterials) error {
	if h == nil || h.aiStopped == nil || h.acceptedMaterials == nil || h.acceptedMaterials.self != h.acceptedMaterials || h.acceptedMaterials.host != h || h.acceptedMaterials.runtime.validate(h) != nil || h.acceptedMaterials.catalog != c || c == nil || c != h.materials || c.self != c || c.binding != lifecycleMaterialsBinding(r) || c.closed || c.unknown || c.remote != nil || r.prepareRoot != lifecycleInvocationBatch(r.OperationID, r.ActualRunID) {
		return lifecycleError("lifecycle_actual_complete_material_scope_missing")
	}
	// Handoff is completed before D closes; later purge never reopens a writer.
	if e := h.CheckWholeWriterFence(ctx, r); e != nil {
		return e
	}
	var directory *lifecycleMaterialDirectory
	for _, d := range c.directories {
		if d != nil && d.path == r.prepareRoot {
			if directory != nil {
				return lifecycleError("lifecycle_material_registration_rejected")
			}
			directory = d
		}
	}
	if directory == nil || directory.unchanged() != nil {
		return lifecycleError("lifecycle_actual_complete_material_scope_missing")
	}
	names := map[string]bool{"qs-ai-original-stop-carrier.jsonl": true, "qs-ai-external-stopped-final-verify.exec.jsonl": true}
	// A known completed handoff is revalidated without reopening a writer or
	// repeating registration. Missing/partial entries never become completion.
	if directory.files["qs-ai-original-stop-carrier.jsonl"] != nil && directory.files["qs-ai-external-stopped-final-verify.exec.jsonl"] != nil && h.aiStopped.VerifyResumed(ctx) == nil {
		return nil
	}
	return h.aiStopped.SealTemporaryJournals(ctx, func(path string, writer *os.File, expected string) error {
		name := filepath.Base(path)
		if filepath.Dir(path) != r.prepareRoot || !names[name] || writer == nil {
			return lifecycleError("lifecycle_material_registration_rejected")
		}
		before, e := writer.Stat()
		if e != nil {
			return e
		}
		if e = directory.register(name, expected, 0, 0600); e != nil {
			return e
		}
		registered := directory.files[name]
		after, e := writer.Stat()
		if e != nil || !sameLifecycleFile(before, after) || !sameLifecycleFile(after, registered.info) {
			return lifecycleError("lifecycle_material_registration_rejected")
		}
		return directory.checkFile(registered)
	})
}

// Registration holds real directory/file FDs and exact inode/UID/mode/nlink,
// source digest and size. Expected names/digests alone are never delete permits.
// All file names must be explicit products of the same trusted owner; a tree
// walker may check for foreign files but never adds them to this catalog.
type lifecycleMaterialFile struct {
	name, hash string
	file       *os.File
	info       os.FileInfo
	removed    bool
	retained   bool
}
type lifecycleMaterialDirectory struct {
	path            string
	file            *os.File
	info            os.FileInfo
	files           map[string]*lifecycleMaterialFile
	children        map[string]*lifecycleMaterialDirectory
	unknown, closed bool
	externalArchive bool
}

func openLifecycleMaterialDirectory(path string, uid uint32) (*lifecycleMaterialDirectory, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || lifecycleSourcePrivateDirectory(path, uid) != nil {
		return nil, lifecycleError("lifecycle_material_registration_rejected")
	}
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_DIRECTORY, 0)
	if e != nil {
		return nil, lifecycleError("lifecycle_material_registration_rejected")
	}
	info, e := f.Stat()
	named, ne := os.Lstat(path)
	st, ok := infoStat(info)
	if e != nil || ne != nil || !ok || !info.IsDir() || info.Mode().Perm() != 0700 || st.Uid != uid || !sameLifecycleFile(info, named) {
		_ = f.Close()
		return nil, lifecycleError("lifecycle_material_registration_rejected")
	}
	return &lifecycleMaterialDirectory{path: path, file: f, info: info, files: map[string]*lifecycleMaterialFile{}, children: map[string]*lifecycleMaterialDirectory{}}, nil
}
func (d *lifecycleMaterialDirectory) unchanged() error {
	if d == nil || d.closed || d.unknown || d.file == nil {
		return lifecycleError("lifecycle_material_registration_rejected")
	}
	held, he := d.file.Stat()
	named, ne := os.Lstat(d.path)
	a, ao := infoStat(d.info)
	b, bo := infoStat(held)
	c, co := infoStat(named)
	// Directory timestamps/nlink change through owned child removal. Identity,
	// owner and permissions do not. This is not protection against a root writer
	// ignoring the separate full-host fence, which remains a mandatory gate.
	if he != nil || ne != nil || !ao || !bo || !co || !held.IsDir() || !named.IsDir() || a.Dev != b.Dev || a.Ino != b.Ino || a.Dev != c.Dev || a.Ino != c.Ino || a.Uid != b.Uid || a.Uid != c.Uid || a.Gid != b.Gid || a.Gid != c.Gid || d.info.Mode() != held.Mode() || d.info.Mode() != named.Mode() {
		return lifecycleError("lifecycle_material_directory_changed")
	}
	return nil
}
func (d *lifecycleMaterialDirectory) register(name, expected string, uid uint32, mode os.FileMode) error {
	return d.registerWithMaximum(name, expected, uid, mode, 2<<30)
}

// Only the original historical producer's two named spools use its existing
// 16GiB per-file budget. Other registrations retain their 2GiB bound.
func (d *lifecycleMaterialDirectory) registerHistoricalCASSpool(name, expected string, uid uint32) error {
	if name != "prepared-mongo-private.bin" && name != "prepared-sql-private.bin" {
		return lifecycleError("lifecycle_material_registration_rejected")
	}
	return d.registerWithMaximum(name, expected, uid, 0600, 16<<30)
}

func (d *lifecycleMaterialDirectory) registerWithMaximum(name, expected string, uid uint32, mode os.FileMode, maximum int64) error {
	if d.unchanged() != nil || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00") || !hashRE.MatchString(expected) || d.files[name] != nil || d.children[name] != nil || mode.Perm() != mode || mode&022 != 0 {
		return lifecycleError("lifecycle_material_registration_rejected")
	}
	fd, e := unix.Openat(int(d.file.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if e != nil {
		return lifecycleError("lifecycle_material_registration_rejected")
	}
	f := os.NewFile(uintptr(fd), filepath.Join(d.path, name))
	info, e := f.Stat()
	st, ok := infoStat(info)
	if e != nil || !ok || !info.Mode().IsRegular() || info.Mode().Perm() != mode || st.Uid != uid || st.Nlink != 1 || info.Size() < 0 || info.Size() > maximum {
		_ = f.Close()
		return lifecycleError("lifecycle_material_registration_rejected")
	}
	v := &lifecycleMaterialFile{name: name, hash: expected, file: f, info: info}
	if e = d.checkFile(v); e != nil {
		_ = f.Close()
		return e
	}
	d.files[name] = v
	return nil
}

// retainReceipt is for an explicitly registered immutable, body-free receipt.
// It grants no authority and never exempts any unregistered directory member.
func (d *lifecycleMaterialDirectory) retainReceipt(name, expected string, uid uint32) error {
	if e := d.register(name, expected, uid, 0600); e != nil {
		return e
	}
	d.files[name].retained = true
	return nil
}
func (d *lifecycleMaterialDirectory) registerChild(child *lifecycleMaterialDirectory) error {
	if d.unchanged() != nil || child == nil || child.unchanged() != nil || filepath.Dir(child.path) != d.path || d.files[filepath.Base(child.path)] != nil || d.children[filepath.Base(child.path)] != nil {
		return lifecycleError("lifecycle_material_registration_rejected")
	}
	d.children[filepath.Base(child.path)] = child
	return nil
}
func (d *lifecycleMaterialDirectory) checkFile(v *lifecycleMaterialFile) error {
	if d.unchanged() != nil || v == nil || v.file == nil || v.removed {
		return lifecycleError("lifecycle_material_registration_rejected")
	}
	before, e := v.file.Stat()
	named, ne := os.Lstat(filepath.Join(d.path, v.name))
	original, originalOK := infoStat(v.info)
	held, heldOK := infoStat(before)
	located, locatedOK := infoStat(named)
	if e != nil || ne != nil || !originalOK || !heldOK || !locatedOK || original.Gid != held.Gid || original.Gid != located.Gid || !sameLifecycleFile(v.info, before) || !sameLifecycleFile(v.info, named) {
		return lifecycleError("lifecycle_material_bytes_or_identity_changed")
	}
	if _, e = v.file.Seek(0, io.SeekStart); e != nil {
		return lifecycleError("lifecycle_material_bytes_or_identity_changed")
	}
	hash := sha256.New()
	n, e := io.Copy(hash, io.LimitReader(v.file, v.info.Size()+1))
	after, ae := v.file.Stat()
	if e != nil || ae != nil || n != v.info.Size() || hex.EncodeToString(hash.Sum(nil)) != v.hash || !sameLifecycleFile(v.info, after) {
		return lifecycleError("lifecycle_material_bytes_or_identity_changed")
	}
	return nil
}
func (d *lifecycleMaterialDirectory) checkComplete(zero bool) error {
	if e := d.unchanged(); e != nil {
		return e
	}
	// The archive is explicitly purged by backup.PurgeRegistered after the host
	// copies. Its separate, mandatory zero check below never uses this exemption.
	if zero && d.externalArchive {
		return nil
	}
	entries, e := os.ReadDir(d.path)
	if e != nil {
		return lifecycleError("lifecycle_material_directory_read_unknown")
	}

	for _, entry := range entries {
		if child := d.children[entry.Name()]; child != nil {
			if e = child.checkComplete(zero); e != nil {
				return e
			}
			continue
		}
		v := d.files[entry.Name()]
		if v == nil || v.removed || entry.IsDir() || zero && !v.retained {
			return lifecycleError("lifecycle_material_remaining_or_foreign")
		}
		if e = d.checkFile(v); e != nil {
			return e
		}
	}
	expected := len(d.children)
	for _, v := range d.files {
		if !zero || v.retained {
			expected++
		}
	}
	if len(entries) != expected {
		return lifecycleError("lifecycle_material_missing_or_unknown")
	}
	return nil
}

// The accepted caller writes an exclusive, durable body-free intent before the
// first deletion. Any syscall/readback/fsync failure is sticky unknown; this
// object cannot retry or infer success from subsequent absence.
func (d *lifecycleMaterialDirectory) purge(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return lifecycleError("lifecycle_material_purge_cancelled")
	}
	if e := d.checkComplete(false); e != nil {
		return e
	}
	children := make([]string, 0, len(d.children))
	for n := range d.children {
		children = append(children, n)
	}
	sort.Strings(children)
	for _, n := range children {
		c := d.children[n]
		if c.externalArchive {
			continue
		}
		if e := c.purge(ctx); e != nil {
			d.unknown = true
			return e
		}
		if e := d.unchanged(); e != nil {
			d.unknown = true
			return e
		}
		if e := c.unchanged(); e != nil {
			d.unknown = true
			return e
		}
		entries, readErr := os.ReadDir(c.path)
		if readErr != nil {
			d.unknown = true
			return lifecycleError("lifecycle_material_directory_read_unknown")
		}
		if len(entries) != 0 {
			continue
		}
		if ctx.Err() != nil {
			d.unknown = true
			return lifecycleError("lifecycle_material_purge_cancelled")
		}
		if e := unix.Unlinkat(int(d.file.Fd()), n, unix.AT_REMOVEDIR); e != nil {
			d.unknown = true
			return lifecycleError("lifecycle_material_remove_result_unknown")
		}
		if e := d.file.Sync(); e != nil {
			d.unknown = true
			return lifecycleError("lifecycle_material_remove_result_unknown")
		}
		if e := c.close(); e != nil {
			d.unknown = true
			return e
		}
		delete(d.children, n)
	}
	names := make([]string, 0, len(d.files))
	for n := range d.files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if ctx.Err() != nil {
			d.unknown = true
			return lifecycleError("lifecycle_material_purge_cancelled")
		}
		v := d.files[n]
		if v.retained {
			continue
		}
		if e := d.checkFile(v); e != nil {
			d.unknown = true
			return e
		}
		if ctx.Err() != nil {
			d.unknown = true
			return lifecycleError("lifecycle_material_purge_cancelled")
		}
		if e := unix.Unlinkat(int(d.file.Fd()), n, 0); e != nil {
			d.unknown = true
			return lifecycleError("lifecycle_material_remove_result_unknown")
		}
		if e := d.file.Sync(); e != nil {
			d.unknown = true
			return lifecycleError("lifecycle_material_remove_result_unknown")
		}
		if e := v.file.Close(); e != nil {
			d.unknown = true
			return lifecycleError("lifecycle_material_file_close_unknown")
		}
		v.file = nil
		v.removed = true
	}
	return d.checkComplete(true)
}
func (d *lifecycleMaterialDirectory) close() error {
	if d == nil || d.closed {
		return nil
	}
	d.closed = true
	var result error
	for _, c := range d.children {
		if e := c.close(); result == nil {
			result = e
		}
	}
	for _, v := range d.files {
		if v.file != nil {
			if e := v.file.Close(); result == nil {
				result = e
			}
			v.file = nil
		}
	}
	if d.file != nil {
		if e := d.file.Close(); result == nil {
			result = e
		}
		d.file = nil
	}
	if result != nil {
		return lifecycleError("lifecycle_material_file_close_unknown")
	}
	return nil
}
func (c *lifecycleBatchMaterials) preflight(ctx context.Context) error {
	if c == nil || c.self != c || !c.binding.valid() || c.started || c.closed || c.unknown || ctx == nil || ctx.Err() != nil || c.journal == nil || c.archive == nil {
		return lifecycleError("lifecycle_material_registration_rejected")
	}
	if c.local != nil && !c.localSealed {
		return lifecycleError("lifecycle_actual_complete_material_scope_missing")
	}
	if e := c.journal.checkComplete(false); e != nil {
		return e
	}
	if e := c.archive.checkComplete(false); e != nil {
		return e
	}
	for _, d := range c.directories {
		if e := d.checkComplete(false); e != nil {
			return e
		}
	}
	for _, e := range c.engines {
		if err := e.check(ctx); err != nil {
			return err
		}
	}
	return checkLifecycleRestoreMaterialSet(ctx, c.engines, false)
}
func (c *lifecycleBatchMaterials) purge(ctx context.Context) error {
	if e := c.preflight(ctx); e != nil {
		return e
	}
	// Only hashes, inode identities and native resource IDs enter the retained
	// receipt. No content, account, key, runtime Env or old message body is saved.
	files := map[string]map[string]any{}
	var add func(*lifecycleMaterialDirectory)
	add = func(d *lifecycleMaterialDirectory) {
		for _, v := range d.files {
			st, _ := infoStat(v.info)
			files[filepath.Join(d.path, v.name)] = map[string]any{"sha256": v.hash, "device": st.Dev, "inode": st.Ino, "uid": st.Uid, "mode": v.info.Mode().Perm(), "bytes": v.info.Size()}
		}
		for _, ch := range d.children {
			add(ch)
		}
	}
	for _, d := range c.directories {
		add(d)
	}
	add(c.archive)
	resources := []map[string]any{}
	for _, engine := range c.engines {
		volumes := []map[string]any{}
		for _, volume := range engine.volumes {
			st, _ := infoStat(volume.inode)
			volumes = append(volumes, map[string]any{"name": volume.Name, "registration_sha256": digest(volume), "device": st.Dev, "inode": st.Ino})
		}
		resources = append(resources, map[string]any{"container_id": engine.identity.ID, "image_id": engine.identity.ImageID, "owner": engine.identity.Owner, "volumes": volumes})
	}
	raw, e := json.Marshal(map[string]any{"kind": "same_process_accepted_material_purge_intent", "binding_sha256": c.binding.digest(), "files": files, "resources": resources, "drop_authority": false})
	if e != nil {
		return lifecycleError("lifecycle_material_purge_intent_unknown")
	}
	if ctx.Err() != nil {
		return lifecycleError("lifecycle_material_purge_cancelled")
	}
	c.started = true // No retry after attempting an exclusive mutation intent.
	if e = writeLifecycleRaw(filepath.Join(c.journal.path, "material-purge.intent.private.json"), append(raw, '\n'), 0600); e != nil {
		c.unknown = true
		return e
	}
	if e = c.journal.retainReceipt("material-purge.intent.private.json", digestRaw(append(raw, '\n')), uint32(os.Getuid())); e != nil {
		c.unknown = true
		return e
	}
	for _, engine := range c.engines {
		if e = engine.purge(ctx); e != nil {
			c.unknown = true
			return e
		}
	}
	for _, d := range c.directories {
		if e = d.purge(ctx); e != nil {
			c.unknown = true
			return e
		}
	}
	c.purged = true
	return nil
}
func (c *lifecycleBatchMaterials) verifyZero(ctx context.Context) error {
	if c == nil || c.self != c || ctx == nil || ctx.Err() != nil || !c.purged || c.unknown || c.closed {
		return lifecycleError("lifecycle_material_zero_unproven")
	}
	for _, d := range c.directories {
		if e := d.checkComplete(true); e != nil {
			c.unknown = true
			return e
		}
	}
	// Archive deletion is performed separately by backup.PurgeRegistered. Still
	// hold/check its actual directory identity and require it empty, then close
	// every registered source FD: unlinked-but-open bodies are not "zero".
	if e := c.archive.unchanged(); e != nil {
		c.unknown = true
		return e
	}
	entries, err := os.ReadDir(c.archive.path)
	if err != nil || len(entries) != 0 {
		c.unknown = true
		return lifecycleError("lifecycle_material_remaining_or_foreign")
	}
	if e := c.archive.unchanged(); e != nil {
		c.unknown = true
		return e
	}
	for _, e := range c.engines {
		if err := e.verifyZero(ctx); err != nil {
			c.unknown = true
			return err
		}
	}
	if e := checkLifecycleRestoreMaterialSet(ctx, c.engines, true); e != nil {
		c.unknown = true
		return e
	}
	for _, d := range c.directories {
		if e := d.close(); e != nil {
			c.unknown = true
			return e
		}
	}
	if e := c.archive.close(); e != nil {
		c.unknown = true
		return e
	}
	if ctx.Err() != nil {
		c.unknown = true
		return lifecycleError("lifecycle_material_purge_cancelled")
	}
	c.zeroVerified = true // Actual zero reads and every retained source FD closed.
	return nil
}
func (c *lifecycleBatchMaterials) close() error {
	if c == nil || c.closed {
		return nil
	}
	c.closed = true
	var result error
	for _, d := range c.directories {
		if e := d.close(); result == nil {
			result = e
		}
	}
	for _, d := range []*lifecycleMaterialDirectory{c.archive, c.journal} {
		if e := d.close(); result == nil {
			result = e
		}
	}
	return result
}
