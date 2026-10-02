// hotrank_day_rebuild is an operator-run, complete-day source reconstruction.
// Default inspection is read-only. Connection secrets are accepted only on stdin.
package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"time"

	rank "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog/hotrank"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// The additional non-partial index is explicitly provisioned by this command,
// outside the startup migration version. Older binaries retain schema36 support.
//
//go:embed index.up.json
var dayIndexCommand []byte

type connections struct {
	MongoURI        string `json:"mongo_uri"`
	MongoDB         string `json:"mongo_db"`
	RedisAddress    string `json:"redis_address"`
	RedisUsername   string `json:"redis_username,omitempty"`
	RedisPassword   string `json:"redis_password,omitempty"`
	RedisDB         *int   `json:"redis_db"`
	RedisNamespace  string `json:"redis_namespace"`
	RedisTLS        bool   `json:"redis_tls,omitempty"`
	RedisServerName string `json:"redis_server_name,omitempty"`
}

func main() { os.Exit(run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
func run(parent context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var day, mode, requestID, operator, expected, auditDir string
	f := flag.NewFlagSet("hotrank_day_rebuild", flag.ContinueOnError)
	f.SetOutput(stderr)
	f.StringVar(&day, "day", "", "complete UTC+8 business day, YYYYMMDD")
	f.StringVar(&mode, "mode", "inspect", "index-spec/inspect/receipt (read-only), prepare-index, or apply")
	f.StringVar(&requestID, "request-id", "", "canonical UUID for one operator action")
	f.StringVar(&operator, "operator", "", "operator identity recorded in the original request receipt")
	f.StringVar(&expected, "expected-fingerprint", "", "exact source manifest approved for apply")
	f.StringVar(&auditDir, "audit-dir", "", "existing private local directory for durable operation evidence")
	if err := f.Parse(args); err != nil {
		return 1
	}
	diagnostic := func(message string) int { _, _ = fmt.Fprintln(stderr, message); return 1 }
	indexHash := sha256.Sum256(dayIndexCommand)
	indexFingerprint := hex.EncodeToString(indexHash[:])
	if mode == "index-spec" && f.NArg() == 0 {
		if err := json.NewEncoder(stdout).Encode(struct {
			Fingerprint string          `json:"fingerprint"`
			Command     json.RawMessage `json:"command"`
			ReadOnly    bool            `json:"read_only"`
		}{indexFingerprint, dayIndexCommand, true}); err != nil {
			return diagnostic("Index specification output failed")
		}
		return 0
	}
	if f.NArg() != 0 || day == "" || (mode != "inspect" && mode != "receipt" && mode != "apply" && mode != "prepare-index") {
		return diagnostic("Explicit day and supported mode required")
	}
	if mode != "inspect" && requestID == "" {
		return diagnostic("Original request UUID required")
	}
	mutating := mode == "apply" || mode == "prepare-index"
	if mutating && (operator == "" || len(operator) > 128 || len(expected) != 64) {
		return diagnostic("Mutation requires operator and exact approved fingerprint")
	}
	if mode == "prepare-index" && expected != indexFingerprint {
		return diagnostic("Index approval fingerprint differs from the embedded command")
	}
	var cfg connections
	decoder := json.NewDecoder(io.LimitReader(stdin, 16385))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return diagnostic("Invalid private connection input")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return diagnostic("Invalid private connection input")
	}
	if cfg.MongoURI == "" || cfg.MongoDB == "" || cfg.RedisAddress == "" || cfg.RedisDB == nil || *cfg.RedisDB < 0 || *cfg.RedisDB > 15 {
		return diagnostic("Explicit host Mongo and Redis connections and Redis DB required")
	}
	var journal *operationJournal
	if mutating {
		var err error
		target, parseErr := url.Parse(cfg.MongoURI)
		if parseErr != nil || target.Host == "" {
			return diagnostic("Invalid private Mongo connection input")
		}
		targetRaw, _ := json.Marshal(struct {
			MongoHost, MongoDB, RedisAddress, Namespace string
			RedisDB                                     int
		}{target.Host, cfg.MongoDB, cfg.RedisAddress, cfg.RedisNamespace, *cfg.RedisDB})
		targetHash := sha256.Sum256(targetRaw)
		journal, err = openOperationJournal(auditDir, operationIntent{Mode: mode, Day: day, RequestID: requestID, Operator: operator, Fingerprint: expected, TargetFingerprint: hex.EncodeToString(targetHash[:])})
		if err != nil {
			return diagnostic("Private durable operation audit unavailable or request conflicts; no mutation attempted")
		}
	}
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()
	ro := &redis.Options{Addr: cfg.RedisAddress, Username: cfg.RedisUsername, Password: cfg.RedisPassword, DB: *cfg.RedisDB, MaxRetries: -1, DialTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	if cfg.RedisTLS {
		ro.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: cfg.RedisServerName}
	}
	client := redis.NewClient(ro)
	defer func() { _ = client.Close() }()
	store := rank.NewRedisScaleHotRankProjection(client, keyspace.NewBuilderWithNamespace(cfg.RedisNamespace))
	output := func(value any) int {
		if err := json.NewEncoder(stdout).Encode(value); err != nil {
			return diagnostic("Evidence output failed; inspect the original request receipt before any further action")
		}
		return 0
	}
	if mode == "receipt" {
		receipt, err := store.RebuildReceipt(ctx, day, requestID)
		if err != nil {
			return diagnostic("Original receipt query failed; no repair attempted")
		}
		return output(struct {
			Mode    string               `json:"mode"`
			Receipt *rank.RebuildReceipt `json:"receipt"`
		}{mode, receipt})
	}
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURI).SetServerSelectionTimeout(3*time.Second))
	if err != nil {
		return diagnostic("Host Mongo connection failed")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = mongoClient.Disconnect(cleanup)
	}()
	source, err := rank.NewMongoDaySource(mongoClient.Database(cfg.MongoDB))
	if err != nil {
		return diagnostic("Source initialization failed")
	}
	if mode == "prepare-index" {
		var commands []json.RawMessage
		if json.Unmarshal(dayIndexCommand, &commands) != nil || len(commands) != 1 {
			return diagnostic("Embedded index command invalid")
		}
		var command bson.D
		if bson.UnmarshalExtJSON(commands[0], false, &command) != nil {
			return diagnostic("Embedded index command invalid")
		}
		if err := mongoClient.Database(cfg.MongoDB).RunCommand(ctx, command).Err(); err != nil {
			return diagnostic("Index creation not confirmed; inspect exact existing index before retry")
		}
		if err := source.RequireIndex(ctx); err != nil {
			return diagnostic("Index definition not verified")
		}
		if err := journal.confirm(journal.intent); err != nil {
			return diagnostic("Index exists but durable confirmation failed; inspect index and original audit before any further action")
		}
		return output(journal.intent)
	}
	var lease rank.RebuildLease
	if mode == "apply" {
		lease, err = store.AcquireRebuild(ctx, day, requestID)
		if errors.Is(err, rank.ErrRebuildAlreadyApplied) {
			receipt, readErr := store.RebuildReceipt(ctx, day, requestID)
			if readErr != nil || receipt == nil || receipt.Operator != operator || receipt.Fingerprint != expected {
				return diagnostic("Original request receipt conflict; no new repair attempted")
			}
			if err := journal.confirm(receipt); err != nil {
				return diagnostic("Original receipt exists but durable audit confirmation failed")
			}
			return output(receipt)
		}
		if err != nil {
			return diagnostic("Day lease unavailable; no repair attempted")
		}
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = store.ReleaseRebuild(cleanup, lease)
		}()
	}
	snapshot, err := source.Capture(ctx, day)
	if err != nil {
		return diagnostic("Complete source capture failed or conflicted; no counts changed")
	}
	fingerprint, counts, err := snapshot.Manifest()
	if err != nil {
		return diagnostic("Source manifest invalid; no counts changed")
	}
	if mode == "inspect" {
		return output(struct {
			Day              string         `json:"day"`
			Complete         bool           `json:"complete"`
			FactCount        int            `json:"fact_count"`
			Fingerprint      string         `json:"fingerprint"`
			Counts           map[string]int `json:"counts"`
			ReadOnly         bool           `json:"read_only"`
			IndexFingerprint string         `json:"index_fingerprint"`
		}{day, snapshot.Complete, len(snapshot.Facts), fingerprint, counts, true, indexFingerprint})
	}
	if fingerprint != expected {
		return diagnostic("Source changed since approval; no counts changed. Inspect a new complete manifest")
	}
	receipt, err := store.ApplyDayRebuild(ctx, lease, snapshot, operator)
	if err != nil {
		return diagnostic("Apply was not confirmed. Query the same request receipt; do not increment, clear dedup keys or reuse a new request blindly")
	}
	if err := journal.confirm(receipt); err != nil {
		return diagnostic("Redis receipt confirmed but durable audit confirmation failed; query the same request receipt, do not create a new request")
	}
	return output(receipt)
}
