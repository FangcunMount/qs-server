package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type lifecycleFinalFileBinding struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Only expected runtime identity and exact private input bytes are supplied.
// The current run is derived by the native host; no imported Q/closure flag,
// command, executable, database connection or new source identity is accepted.
type lifecycleFinalHistoryInput struct {
	AssetsDirectory      string                                  `json:"assets_directory"`
	RuntimeSourceSHA     string                                  `json:"runtime_source_sha"`
	ImageID              string                                  `json:"image_id"`
	ContainerID          string                                  `json:"container_id"`
	RuntimeBindingSHA256 string                                  `json:"runtime_binding_sha256"`
	AIBounds             lifecycleFinalFileBinding               `json:"ai_bounds"`
	PeerBounds           lifecycleFinalFileBinding               `json:"peer_bounds"`
	Protection           lifecycleFinalFileBinding               `json:"protection"`
	StopConstraints      *retirement.AIStoppedRuntimeConstraints `json:"stop_constraints,omitempty"`
}

func (v *lifecycleFinalHistoryInput) valid(r lifecycleRequest) bool {
	if v == nil || !shaRE.MatchString(v.RuntimeSourceSHA) || !strings.HasPrefix(v.ImageID, "sha256:") || !hashRE.MatchString(strings.TrimPrefix(v.ImageID, "sha256:")) || !hashRE.MatchString(v.ContainerID) || !hashRE.MatchString(v.RuntimeBindingSHA256) || !filepath.IsAbs(v.AssetsDirectory) || filepath.Clean(v.AssetsDirectory) != v.AssetsDirectory {
		return false
	}
	if v.StopConstraints != nil && !v.StopConstraints.Valid() {
		return false
	}
	root := filepath.Join("/opt/backups/qs-server/compatibility-retirement", r.OperationID)
	paths := map[string]bool{}
	for _, file := range []lifecycleFinalFileBinding{v.AIBounds, v.PeerBounds, v.Protection} {
		if !lifecycleOwnedPath(root, file.Path) || !hashRE.MatchString(file.SHA256) || paths[file.Path] {
			return false
		}
		paths[file.Path] = true
	}
	return true
}

func lifecycleFinalFileSame(a, b os.FileInfo) bool {
	if a == nil || b == nil || !os.SameFile(a, b) || a.Mode() != b.Mode() || a.Size() != b.Size() || !a.ModTime().Equal(b.ModTime()) {
		return false
	}
	x, xok := a.Sys().(*syscall.Stat_t)
	y, yok := b.Sys().(*syscall.Stat_t)
	return xok && yok && x.Uid == y.Uid && x.Gid == y.Gid && x.Nlink == 1 && y.Nlink == 1
}

func lifecycleFinalExternalInput(r lifecycleRequest) (*retirement.AIExternalExecutionInput, error) {
	if r.FinalHistory == nil {
		return nil, nil
	}
	v := r.FinalHistory
	if !v.valid(r) {
		return nil, lifecycleError("lifecycle_final_historical_input_rejected")
	}
	parts := strings.Split(r.ActualRunID, "-")
	if len(parts) != 2 {
		return nil, lifecycleError("lifecycle_final_historical_input_rejected")
	}
	for _, part := range parts {
		n, e := strconv.ParseUint(part, 10, 64)
		if e != nil || n == 0 || strconv.FormatUint(n, 10) != part {
			return nil, lifecycleError("lifecycle_final_historical_input_rejected")
		}
	}
	// Reuse the root once caller's physical invocation provenance. SourceUID
	// comes from its actual protected intent, never a caller JSON field or an
	// environment value. The bound input bytes remain original nonroot files.
	if e := validateLifecycleAPIInvocation(r); e != nil {
		return nil, e
	}
	intentRaw, e := readLifecycleAPIRecord(filepath.Join(r.prepareRoot, "native-call.intent.private.json"))
	if e != nil {
		return nil, e
	}
	intent, e := decodeLifecycleAPIInvocationIntent(intentRaw)
	if e != nil {
		return nil, e
	}
	var raw [3]json.RawMessage
	for i, f := range []lifecycleFinalFileBinding{v.AIBounds, v.PeerBounds, v.Protection} {
		if e := readLifecyclePrivateAs(f.Path, f.SHA256, &raw[i], 4<<20, intent.SourceUID); e != nil {
			return nil, e
		}
	}
	peer := retirement.AIExternalPeerConnection{}
	// Connection credentials remain exclusively in the actual host environment.
	if peer.Host, e = lifecycleConnectionValue("", "MYSQL_HOST"); e != nil {
		return nil, e
	}
	if peer.Database, e = lifecycleConnectionValue("", "MYSQL_DATABASE"); e != nil {
		return nil, e
	}
	if peer.Username, e = lifecycleConnectionValue("", "MYSQL_USERNAME"); e != nil {
		return nil, e
	}
	if peer.Password, e = lifecycleConnectionValue("", "MYSQL_PASSWORD"); e != nil {
		return nil, e
	}
	if peer.Port, e = envPort("MYSQL_PORT", 3306); e != nil {
		return nil, lifecycleError("lifecycle_connection_input_rejected")
	}
	return &retirement.AIExternalExecutionInput{OperationDirectory: filepath.Join("/opt/backups/qs-server/compatibility-retirement", r.OperationID), AssetsDirectory: v.AssetsDirectory, RunID: parts[0], RuntimeSourceSHA: v.RuntimeSourceSHA, ImageID: v.ImageID, ContainerID: v.ContainerID, ApprovedAIRuntimeBindingSHA256: v.RuntimeBindingSHA256, AIBounds: raw[0], PeerBounds: raw[1], ProtectionJSON: raw[2], ApprovedAIBoundsSHA256: v.AIBounds.SHA256, ApprovedPeerBoundsSHA256: v.PeerBounds.SHA256, PeerConnection: peer, SudoDocker: true}, nil
}

func (h *lifecycleFixedHost) finalDifferenceAndEOF(ctx context.Context, r lifecycleRequest, a *backup.Archive) (result error) {
	if h == nil || h.dataBaseline != nil || h.owner == nil || h.owner.originalConn == nil || h.owner.originalMongo == nil || h.owner.originalDB == nil || h.services == nil || !h.services.managementReady || h.services.window == nil || !h.services.identity.matches(r) || ctx == nil || ctx.Err() != nil {
		return lifecycleError("lifecycle_final_historical_scope_rejected")
	}
	q, c, e := h.services.window.ForwardContext(ctx)
	if e != nil {
		return e
	}
	defer c()
	defer func() {
		if result == nil && q.Err() != nil {
			result = q.Err()
		}
	}()
	if e = h.services.Check(q); e != nil {
		return e
	}
	sources, e := backup.OpenHostHistoricalSources(q, a, r.Approval)
	if e != nil {
		return e
	}
	defer func() {
		if e := sources.Close(); e != nil && result == nil {
			result = e
		}
	}()
	binding, e := sources.Binding(q)
	if e != nil {
		return e
	}
	if binding.SourceSHA != r.OriginalSourceSHA || binding.OperationID != r.OperationID || binding.SQLHead != r.Recovery.SQLHead || binding.MongoHead != r.Recovery.MongoHead {
		return lifecycleError("lifecycle_final_historical_scope_rejected")
	}
	copies, e := sources.Copies(q)
	if e != nil {
		return e
	}
	external, e := lifecycleFinalExternalInput(r)
	if e != nil {
		return e
	}
	// Borrow the already owned dedicated connection. Do not ask the max-one
	// pool for another connection and do not return a transaction to DDL code.
	tx, e := h.owner.originalConn.BeginTx(q, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return lifecycleError("lifecycle_final_historical_scope_rejected")
	}
	defer func() {
		if e := tx.Rollback(); e != nil && !errors.Is(e, sql.ErrTxDone) && result == nil {
			result = lifecycleError("lifecycle_final_historical_scope_cleanup_failed")
		}
	}()
	g, e := gorm.Open(gormmysql.New(gormmysql.Config{Conn: tx, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		return lifecycleError("lifecycle_final_historical_scope_rejected")
	}
	session, e := h.owner.originalMongo.StartSession()
	if e != nil {
		return lifecycleError("lifecycle_final_historical_scope_rejected")
	}
	defer session.EndSession(context.Background())
	if e = session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot()).SetReadPreference(readpref.Primary())); e != nil {
		return lifecycleError("lifecycle_final_historical_scope_rejected")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// Require the real original-session server response before any DROP;
		// the pinned driver's ordinary Abort ignores network/command errors.
		nativeError := lifecycleAbortComparisonMongo(cleanup, h.owner.originalMongo, session)
		localError := session.AbortTransaction(cleanup)
		if (nativeError != nil || localError != nil) && result == nil {
			result = lifecycleError("lifecycle_final_historical_scope_cleanup_failed")
		}
	}()
	paired := mongo.NewSessionContext(hostmysql.WithTx(q, g), session)
	borrowed := backup.BorrowedSources{SQL: tx, Mongo: h.owner.originalDB}
	if e = backup.VerifyHostOriginalSources(paired, a, borrowed); e != nil {
		return e
	}
	if h.aiStopped == nil || h.aiStopped.CheckStopped(paired) != nil {
		return lifecycleError("lifecycle_ai_original_stopped_runtime_unproven")
	}
	o, e := retirement.PrepareFinalHistoricalEOF(paired, retirement.FinalHistoricalInput{Binding: retirement.HistoricalCoordinatorBinding{SourceSHA: r.OriginalSourceSHA, OperationID: r.OperationID}, SQLIdentity: binding.SQLIdentitySHA256, SQLHead: binding.SQLHead, MongoDatabase: h.owner.originalDB, MongoConfig: retirement.MongoOwnerConfig{ExpectedIdentityHash: binding.MongoIdentitySHA256, ExpectedMigrationVersion: int64(binding.MongoHead)}, Copies: copies, External: external, StoppedExternal: h.aiStopped})
	if e != nil {
		return e
	}
	if e = o.ValidateBorrowedSnapshot(paired); e != nil {
		return e
	}
	if e = backup.VerifyHostOriginalSources(paired, a, borrowed); e != nil {
		return e
	}
	baseline, e := backup.CaptureCompleteNonTargetData(paired, a, borrowed, o)
	if e != nil {
		return e
	}
	if e = sources.Verify(q); e != nil {
		return e
	}
	if e = h.services.Check(q); e != nil {
		return e
	}
	if q.Err() == nil {
		h.dataBaseline = baseline
	}
	return q.Err() // All borrowed RO scopes are ended by this host before DDL.
}
