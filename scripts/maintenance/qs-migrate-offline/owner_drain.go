package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/spf13/viper"
	"golang.org/x/sys/unix"

	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/migration"
	hostoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	txmongo "github.com/FangcunMount/reliable-messaging/storage/transaction/mongo"
	txmysql "github.com/FangcunMount/reliable-messaging/storage/transaction/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// This explicitly selected mode starts only paused native Checkers, not a
// business Container, consumers, schedulers, new sends, DDL or binding writes.
type ownerDrainArguments struct {
	enabled                      bool
	config, configSHA, operation string
	mysqlIdentity, mongoIdentity string
}

var drainOperation = regexp.MustCompile(`^[0-9]{1,20}-[0-9]{1,4}$`)
var drainDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

func registerOwnerDrainFlags(f *flag.FlagSet, d *ownerDrainArguments) {
	f.BoolVar(&d.enabled, "owner-drain", false, "explicitly register paused native owner Checkers; may persist recovery audits")
	f.StringVar(&d.config, "owner-drain-config", "", "exact reviewed original JSON application config")
	f.StringVar(&d.configSHA, "owner-drain-config-sha256", "", "reviewed original config raw digest")
	f.StringVar(&d.operation, "owner-drain-operation", "", "unique digits-attempt identity; never reuse")
	f.StringVar(&d.mysqlIdentity, "owner-drain-mysql-identity", "", "canonical reviewed MySQL identity digest")
	f.StringVar(&d.mongoIdentity, "owner-drain-mongo-identity", "", "canonical reviewed Mongo cluster identity digest")
}

func (d ownerDrainArguments) validate(a arguments) error {
	if !d.enabled {
		if d.config != "" || d.configSHA != "" || d.operation != "" || d.mysqlIdentity != "" || d.mongoIdentity != "" {
			return errors.New("owner drain requires explicit action")
		}
		return nil
	}
	if a.apply || a.protocolObservation || a.binding.action != "" || !filepath.IsAbs(d.config) || !drainDigest.MatchString(d.configSHA) || !drainDigest.MatchString(d.mysqlIdentity) || !drainDigest.MatchString(d.mongoIdentity) || !drainOperation.MatchString(d.operation) {
		return errors.New("separate reviewed original QS owner drain required")
	}
	return nil
}

func drainConfiguration(raw []byte, digest string) (*hostoptions.MessagingOptions, error) {
	h := sha256.Sum256(raw)
	if len(raw) == 0 || len(raw) > 1<<20 || hex.EncodeToString(h[:]) != digest {
		return nil, errors.New("original owner config differs")
	}
	reader := viper.New()
	reader.SetConfigType("json")
	if reader.ReadConfig(bytes.NewReader(raw)) != nil {
		return nil, errors.New("original JSON config required")
	}
	o := hostoptions.NewMessagingOptions()
	if reader.UnmarshalKey("messaging", o) != nil || o.RocketMQ == nil || o.RocketMQ.DecodeTransactions(reader, "messaging.rocketmq.transactions") != nil || !o.Enabled || o.Provider != "rocketmq" || len(o.RocketMQ.Transactions) != 2 {
		return nil, errors.New("original dual native owner config required")
	}
	for _, owner := range []string{"qs.mysql", "qs.mongo"} {
		binding, ok := o.RocketMQ.Transactions[owner]
		if !ok {
			return nil, transaction.ErrBinding
		}
		if _, err := binding.ProtocolLimits(owner); err != nil {
			return nil, err
		}
	}
	paused := false
	o.RocketMQ.PublishEnabled = &paused
	return o, nil
}

type drainOwner struct {
	runtime *messagingruntime.TransactionRuntime
	reader  transaction.ObservationReader
}
type drainOwnerReport struct {
	Owner      string                                   `json:"owner"`
	SDK        rocketmq.TransactionStats                `json:"sdk"`
	Protocol   transaction.ProtocolObservation          `json:"protocol"`
	Protection messagingruntime.ProtocolProtectionStats `json:"protection"`
}
type drainReport struct {
	SourceSHA                  string             `json:"source_sha"`
	ConfigSHA                  string             `json:"config_sha256"`
	Operation                  string             `json:"operation_id"`
	Phase                      string             `json:"phase"`
	SampledAt                  time.Time          `json:"sampled_at"`
	Owners                     []drainOwnerReport `json:"owners"`
	Available                  bool               `json:"available"`
	NoObservedUnresolved       bool               `json:"no_observed_unresolved"`
	NewSendsEnabled            bool               `json:"new_sends_enabled"`
	BusinessRuntimeStarted     bool               `json:"business_runtime_started"`
	DatabaseDDL                bool               `json:"database_ddl"`
	RecoveryAuditsMayBeWritten bool               `json:"recovery_audits_may_be_written"`
	Expired                    bool               `json:"expired"`
}

func drainSampleSafe(r drainOwnerReport) bool {
	p, s, protection := r.Protocol, r.SDK, r.Protection
	return !p.AsOf.IsZero() && s.Ready && !s.Stopping && !s.Stopped && s.ActiveSends == 0 && s.RunningChecks == 0 && s.QueuedChecks == 0 &&
		protection.Available && protection.Fresh && protection.SamplerRunning && !protection.NativeLimitActive &&
		p.ReservedExecutions == 0 && p.PreparingExecutions == 0 && p.ConfirmationGapBranches == 0 && p.UnagedConfirmationGapBranches == 0 && p.UntrackedAbortedBranches == 0 && p.UnobservedCommittedBranches == 0 && p.MetadataConflicts == 0 && p.PreparedExpiryAgeSeconds == 0 && p.OldestConfirmationObservationAgeSeconds == 0
}

// The stop request is a separately captured writer exclusion OR replacement
// owner receipt. It is not proof by itself, and never authorizes a migration.
func drainStopRequest(raw []byte, operation string) bool {
	var r struct {
		Operation        string `json:"operation_id"`
		WritersStopped   bool   `json:"writers_stopped"`
		ReplacementReady bool   `json:"replacement_owner_ready"`
		Receipt          string `json:"receipt_sha256"`
	}
	return len(raw) <= 4096 && json.Unmarshal(raw, &r) == nil && r.Operation == operation && r.WritersStopped != r.ReplacementReady && drainDigest.MatchString(r.Receipt)
}

func readDrainStop(path, operation string) bool {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return false
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	var s unix.Stat_t
	if unix.Fstat(fd, &s) != nil || s.Mode&unix.S_IFMT != unix.S_IFREG || s.Mode&0777 != 0600 || s.Nlink != 1 || s.Uid != uint32(os.Geteuid()) || s.Size > 4096 {
		return false
	}
	raw, err := io.ReadAll(io.LimitReader(f, 4097))
	return err == nil && drainStopRequest(raw, operation)
}

func drainHead(ctx context.Context, db *sql.DB, documents *mongo.Database) error {
	var rows, version uint64
	var dirty bool
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*),MIN(version),COALESCE(MAX(dirty),1) FROM schema_migrations").Scan(&rows, &version, &dirty); err != nil || rows != 1 || version != 110 || dirty {
		return errors.New("original clean SQL110 required")
	}
	n, err := documents.Collection("schema_migrations").CountDocuments(ctx, bson.D{})
	var head struct {
		Version uint `bson:"version"`
		Dirty   bool `bson:"dirty"`
	}
	if err != nil || n != 1 || documents.Collection("schema_migrations").FindOne(ctx, bson.D{}).Decode(&head) != nil || head.Version != 44 || head.Dirty {
		return errors.New("original clean Mongo44 required")
	}
	return nil
}

func drainOwners(ctx context.Context, db *sql.DB, documents *mongo.Database, o *hostoptions.MessagingOptions) ([]drainOwner, error) {
	config, err := o.RocketMQ.Load(nil, 1, 5*time.Second)
	if err != nil {
		return nil, err
	}
	owners := make([]drainOwner, 0, 2)
	for _, owner := range []string{"qs.mysql", "qs.mongo"} {
		b := o.RocketMQ.Transactions[owner]
		binding := transaction.Binding{Owner: owner, StoreID: b.StoreID, WriterEpoch: b.WriterEpoch}
		var actual transaction.Binding
		actual.Owner = owner
		if owner == "qs.mysql" {
			err = db.QueryRowContext(ctx, "SELECT store_id,writer_epoch FROM rm_tx_binding WHERE owner=?", []byte(owner)).Scan(&actual.StoreID, &actual.WriterEpoch)
		} else {
			var row struct {
				Store string `bson:"store_id"`
				Epoch string `bson:"writer_epoch"`
			}
			err = documents.Collection("rm_tx_binding").FindOne(ctx, bson.M{"_id": owner}).Decode(&row)
			actual.StoreID, actual.WriterEpoch = row.Store, row.Epoch
		}
		if err != nil || actual != binding {
			return nil, transaction.ErrBinding
		}
		topicSet := map[string]bool{}
		for _, route := range config.Routes {
			if route.Owner == owner && route.MessageType == transport.TransactionMessageType && route.RouteRevision == b.RouteRevision {
				topicSet[route.Topic] = true
			}
		}
		topics := make([]string, 0, len(topicSet))
		for topic := range topicSet {
			topics = append(topics, topic)
		}
		sort.Strings(topics)
		var store transaction.PreparedStore
		var native messagingruntime.NativeLimitReader
		if owner == "qs.mysql" {
			store, err = txmysql.New(db, binding)
			if err == nil {
				native, err = messagingruntime.NewSQLNativeLimits(db, binding, b.RouteRevision)
			}
		} else {
			store, err = txmongo.New(documents, binding)
			if err == nil {
				native, err = messagingruntime.NewMongoNativeLimits(documents, binding, b.RouteRevision)
			}
		}
		if err != nil {
			return nil, err
		}
		r, err := messagingruntime.NewTransactionalSender(o, owner, store, topics, native)
		if err != nil {
			return nil, err
		}
		owners = append(owners, drainOwner{r, store.(transaction.ObservationReader)})
	}
	return owners, nil
}

func publishDrain(path string, r drainReport) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	temp := path + ".next"
	f, err := os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(append(raw, '\n')); err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func runOwnerDrain(ctx context.Context, db *sql.DB, client *mongo.Client, pair migration.PairConfig, a ownerDrainArguments, output io.Writer) error {
	raw, err := os.ReadFile(a.config)
	if err != nil {
		return errors.New("original owner config unavailable")
	}
	o, err := drainConfiguration(raw, a.configSHA)
	if err != nil {
		return err
	}
	mysqlIdentity, mongoIdentity, err := migration.InspectPairIdentity(ctx, db, client, pair)
	if err != nil || mysqlIdentity != a.mysqlIdentity || mongoIdentity != a.mongoIdentity {
		return errors.New("reviewed original pair identity differs")
	}
	if err = drainHead(ctx, db, client.Database(pair.MongoDatabase)); err != nil {
		return err
	}
	owners, err := drainOwners(ctx, db, client.Database(pair.MongoDatabase), o)
	if err != nil {
		return err
	}
	directory := filepath.Join(os.TempDir(), "qs-owner-drain-"+a.operation)
	if err = os.Mkdir(directory, 0700); err != nil {
		return errors.New("new private owner operation required")
	}
	for _, owner := range owners {
		startup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err = owner.runtime.Start(startup)
		cancel()
		if err != nil {
			for _, started := range owners {
				cleanup, c := context.WithTimeout(context.Background(), 30*time.Second)
				closeErr := started.runtime.Close(cleanup)
				c()
				if closeErr != nil {
					for {
						time.Sleep(2 * time.Second)
					}
				} // retain borrowed pools until actual operator recovery
			}
			return errors.New("paused owner registration failed")
		}
	}
	// Do not defer Close: a lost operator/deadline must not drop the last Checker
	// while original responsibility remains. A reviewed stop request and a fresh
	// safe primary/SDK sample are both required; SIGTERM alone cannot grant it.
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		r := drainReport{SourceSHA: pair.ExpectedSourceSHA, ConfigSHA: a.configSHA, Operation: a.operation, Phase: "checking", SampledAt: time.Now().UTC(), Available: true, NoObservedUnresolved: true, RecoveryAuditsMayBeWritten: true, Expired: ctx.Err() != nil}
		for i, owner := range owners {
			q, c := context.WithTimeout(context.Background(), 15*time.Second)
			p, e := owner.reader.ObserveProtocol(q)
			c()
			item := drainOwnerReport{Owner: []string{"qs.mysql", "qs.mongo"}[i], SDK: owner.runtime.SDKStats(), Protocol: p, Protection: owner.runtime.ProtectionStats()}
			r.Owners = append(r.Owners, item)
			r.Available = r.Available && e == nil
			r.NoObservedUnresolved = r.NoObservedUnresolved && e == nil && drainSampleSafe(item)
		}
		if readDrainStop(filepath.Join(directory, "stop.json"), a.operation) {
			if r.Available && r.NoObservedUnresolved {
				for _, owner := range owners {
					cleanup, c := context.WithTimeout(context.Background(), 30*time.Second)
					err = owner.runtime.Close(cleanup)
					c()
					if err != nil {
						r.Phase = "close_incomplete"
						r.NoObservedUnresolved = false
						_ = publishDrain(filepath.Join(directory, "status.json"), r)
						for {
							time.Sleep(2 * time.Second)
						} // never release DB pools still owned by incomplete Checkers
					}
				}
				r.Phase = "stopped"
				r.NoObservedUnresolved = false
				for i, owner := range owners {
					r.Owners[i].SDK = owner.runtime.SDKStats()
					r.Owners[i].Protection = owner.runtime.ProtectionStats()
				}
				return errors.Join(publishDrain(filepath.Join(directory, "status.json"), r), json.NewEncoder(output).Encode(r))
			}
			r.Phase = "stop_refused_unresolved"
		}
		if err = publishDrain(filepath.Join(directory, "status.json"), r); err != nil { /* Preserve Checkers if evidence disk fails; never claim readiness. */
			r.Available = false
		}
		<-ticker.C
	}
}
