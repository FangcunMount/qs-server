package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
	"github.com/FangcunMount/qs-server/pkg/version"
)

// This is the seam for the future real runtime/data acceptance producer. There
// is deliberately no constructor, request field, JSON decoder or receipt loader
// for it yet. VerifyAcceptance still fails, and effectsPreflight still fails.
// In particular an absent archive, restored fixture, success DTO or remote
// process exit cannot mint this same-process accepted-batch capability.
type lifecycleAcceptedMaterials struct {
	self    *lifecycleAcceptedMaterials
	host    *lifecycleFixedHost
	binding lifecycleMaterialBinding
	catalog *lifecycleBatchMaterials
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
	directories                      []*lifecycleMaterialDirectory
	archive                          *lifecycleMaterialDirectory
	engines                          []*lifecycleEnginePurge
	journal                          *lifecycleMaterialDirectory
	started, purged, closed, unknown bool
}

func lifecycleMaterialPathsMatch(c *lifecycleBatchMaterials, r lifecycleRequest) bool {
	if c.archive.path != r.ArchiveDirectory || r.prepareRoot != lifecycleInvocationBatch(r.OperationID, r.ActualRunID) || c.journal.path != filepath.Join(r.prepareRoot, "material-purge-receipts") {
		return false
	}
	original := filepath.Join("/opt/backups/qs-server/compatibility-retirement", r.OperationID)
	allowed := map[string]bool{
		filepath.Join(original, "inventory-"+r.Recovery.OriginalRunID): true,
		lifecycleRootBatch(r.OperationID, r.ActualRunID):               true,
		r.prepareRoot: true,
		lifecycleServicesRoot(r.OperationID, "server-a"): true,
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
	if a == nil || a.self != a || a.host != h || a.binding != lifecycleMaterialsBinding(r) || !a.binding.valid() || a.binding.tool != sourceSHA || a.catalog == nil || a.catalog.self != a.catalog || a.catalog.binding != a.binding {
		return nil, lifecycleError("lifecycle_actual_batch_acceptance_missing")
	}
	c := a.catalog
	if c.closed || c.unknown || c.archive == nil || c.journal == nil || len(c.engines) != 2 || h.restoreOwner == nil || len(h.restoreOwner.engines) != 2 || c.remote == nil || c.remote.self != c.remote || c.remote.binding != c.binding || c.remote.native == nil || c.remote.services != h.services || len(c.scopes) != int(lifecycleMaterialScopeCount) || !lifecycleMaterialPathsMatch(c, r) {
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
	if e != nil || !ok || !info.Mode().IsRegular() || info.Mode().Perm() != mode || st.Uid != uid || st.Nlink != 1 || info.Size() < 0 || info.Size() > 2<<30 {
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
