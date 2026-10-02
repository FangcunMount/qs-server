//go:build integration && reliable_messaging_m4

package runtimeclosure_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	apphot "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/hotrank"
	prepare "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	redishot "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog/hotrank"
	stage "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/hotrank"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	payload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/reliable-messaging/outbox"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	"github.com/FangcunMount/reliable-messaging/transport"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/google/uuid"
	driver "github.com/nsqio/go-nsq"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestHotRankDayRecoveryAfterActualNSQConfirmationLoss(t *testing.T) {
	uri := os.Getenv("M6_HOTRANK_MONGO_URI")
	if uri == "" {
		t.Skip("owned disposable Mongo/Redis/NSQ proof requires explicit environment")
	}
	redisAddress := os.Getenv("M6_HOTRANK_REDIS_ADDRESS")
	nsqAddress := os.Getenv("M6_HOTRANK_NSQ_ADDRESS")
	nsqHTTP := os.Getenv("M6_HOTRANK_NSQ_HTTP")
	container := os.Getenv("M6_HOTRANK_NSQ_CONTAINER")
	owner := os.Getenv("M6_HOTRANK_OWNER")
	if !strings.HasPrefix(uri, "mongodb://127.0.0.1:") || !strings.Contains(uri, "directConnection=true") || !strings.HasPrefix(redisAddress, "127.0.0.1:") || !strings.HasPrefix(nsqAddress, "127.0.0.1:") || !strings.HasPrefix(nsqHTTP, "http://127.0.0.1:") || !strings.HasPrefix(container, "m6-hotrank-") || owner == "" {
		t.Fatal("only owned loopback disposable resources allowed")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer mongoClient.Disconnect(context.Background())
	db := mongoClient.Database("m6_hotrank_" + strings.ReplaceAll(owner, "-", ""))
	defer db.Drop(context.Background())
	for _, name := range []string{"answersheets", "rm_outbox"} {
		if err := db.CreateCollection(ctx, name); err != nil {
			t.Fatal(err)
		}
	}
	zone := time.FixedZone("UTC+8", 8*3600)
	now := time.Now().In(zone)
	day := now.Format("20060102")
	tool := os.Getenv("M6_HOTRANK_TOOL")
	if !filepath.IsAbs(tool) || filepath.Base(tool) != "hotrank-day-rebuild" {
		t.Fatal("owned operator tool required")
	}
	auditDir := t.TempDir()
	if err := os.Chmod(auditDir, 0700); err != nil {
		t.Fatal(err)
	}
	privateInput, _ := json.Marshal(map[string]any{"mongo_uri": uri, "mongo_db": db.Name(), "redis_address": redisAddress, "redis_db": 0, "redis_namespace": "m6-hotrank-" + owner})
	runTool := func(args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(ctx, tool, args...)
		cmd.Stdin = bytes.NewReader(privateInput)
		raw, err := cmd.Output()
		if err != nil {
			t.Fatalf("owned operator command: %s err=%v", raw, err)
		}
		return raw
	}
	raw, err := os.ReadFile("../../../../scripts/oneoff/hotrank_day_rebuild/index.up.json")
	if err != nil {
		t.Fatal(err)
	}
	indexHash := sha256.Sum256(raw)
	indexSource, err := redishot.NewMongoDaySource(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := indexSource.RequireIndex(ctx); err == nil {
		t.Fatal("missing complete-day index accepted")
	}
	runTool("--mode", "prepare-index", "--day", day, "--request-id", uuid.NewString(), "--operator", "isolated-m6-operator", "--expected-fingerprint", hex.EncodeToString(indexHash[:]), "--audit-dir", auditDir)
	wire, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	catalog := eventcatalog.NewCatalog(wire)
	stager, err := stage.NewStager(db.Collection("rm_outbox"), catalog, "qs-apiserver")
	if err != nil {
		t.Fatal(err)
	}
	events := make([]event.DomainEvent, 0, 4)
	session, err := mongoClient.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(ctx)
	for i := 0; i < 4; i++ {
		code := "Q-A"
		if i == 3 {
			code = "Q-B"
		}
		id := uint64(100 + i)
		submitted := now.Add(time.Duration(i) * time.Nanosecond)
		evt := event.New("answersheet.submitted", "AnswerSheet", strconv.FormatUint(id, 10), payload.AnswerSheetSubmittedData{AnswerSheetID: strconv.FormatUint(id, 10), QuestionnaireCode: code, QuestionnaireVersion: "v1", OrgID: 1, TesteeID: 7, FillerID: 8, SubmittedAt: submitted})
		events = append(events, evt)
		_, err := session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) {
			row := bson.M{"domain_id": int64(id), "org_id": int64(1), "testee_id": int64(7), "filler_id": int64(8), "questionnaire_code": code, "questionnaire_version": "v1", "filled_at": submitted, "durable_acceptance": bson.M{"schema_version": 1, "event_id": evt.EventID(), "accepted_at": evt.OccurredAt()}, "deleted_at": nil}
			if i == 0 {
				row["deleted_at"] = now
			} // Previously counted, then soft deleted; still a submission fact.
			if _, err := db.Collection("answersheets").InsertOne(tx, row); err != nil {
				return nil, err
			}
			return nil, stager.Stage(tx, evt)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	prepared, err := prepare.PrepareIntents(events, catalog, "qs-apiserver")
	if err != nil {
		t.Fatal(err)
	}
	producer, err := driver.NewProducer(nsqAddress, driver.NewConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Stop()
	publisher, err := sdknsq.New(producer, map[string]string{"qs.evaluation.lifecycle": "qs.evaluation.lifecycle"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	store, err := sdkmongo.New(db.Collection("rm_outbox"))
	if err != nil {
		t.Fatal(err)
	}
	var confirmed atomic.Int32
	relayCtx, stopRelay := context.WithCancel(ctx)
	defer stopRelay()
	runner, err := relay.New(store, publisher, relay.Config{Concurrency: 1, PollInterval: 20 * time.Millisecond, Lease: 10 * time.Second, PublishTimeout: 2 * time.Second, WriteTimeout: time.Second, Retry: func(outbox.Claim, transport.Outcome) relay.RetryDecision {
		return relay.RetryDecision{Delay: time.Second}
	}, Observe: func(e relay.Event) {
		if e.Kind == "write_succeeded" && e.Outcome == transport.Confirmed && confirmed.Add(1) == 4 {
			stopRelay()
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Run(relayCtx); err != nil || confirmed.Load() != 4 {
		t.Fatalf("actual Relay confirmations=%d err=%v", confirmed.Load(), err)
	}
	if depth := hotProofNSQDepth(t, nsqHTTP); depth != 4 {
		t.Fatalf("pre-kill depth=%d want4", depth)
	}
	originalIDs := make([]string, 0, 4)
	for _, e := range events {
		originalIDs = append(originalIDs, e.EventID())
	}
	count, err := db.Collection("rm_outbox").CountDocuments(ctx, bson.M{"message_id": bson.M{"$in": originalIDs}, "state": "published"})
	if err != nil || count != 4 {
		t.Fatal("original published intents missing", count, err)
	}
	client := redis.NewClient(&redis.Options{Addr: redisAddress, MaxRetries: -1})
	defer client.Close()
	projection := redishot.NewRedisScaleHotRankProjection(client, keyspace.NewBuilderWithNamespace("m6-hotrank-"+owner))
	consumer := apphot.NewEventConsumer(projection)
	lateWire, recognized, err := legacy.Decode(prepared[0].Message.Input().Payload)
	if err != nil || !recognized {
		t.Fatal(err)
	}
	if err := consumer(ctx, "answersheet.submitted", lateWire.Payload); err != nil {
		t.Fatal(err)
	} // One existing effect before broker loss.
	nsqHTTP = hotProofRestartOwnedNSQ(t, container, owner)
	if depth := hotProofNSQDepth(t, nsqHTTP); depth != 0 {
		t.Fatalf("actual post-kill depth=%d want0", depth)
	}
	source, err := redishot.NewMongoDaySource(db)
	if err != nil {
		t.Fatal(err)
	}
	// Inspection is read-only; acquire the fence before the source used for apply.
	inspected, err := source.Capture(ctx, day)
	if err != nil || len(inspected.Facts) != 4 {
		t.Fatal("whole source capture", len(inspected.Facts), err)
	}
	fingerprint, _, err := inspected.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	lease, err := projection.AcquireRebuild(ctx, day, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	captured, err := source.Capture(ctx, day)
	if err != nil {
		t.Fatal(err)
	}
	if again, _, err := captured.Manifest(); err != nil || again != fingerprint {
		t.Fatal("fixed original source drift", err)
	}
	receipt, err := projection.ApplyDayRebuild(ctx, lease, captured, "isolated-m6-operator")
	if err != nil || receipt.FactCount != 4 {
		t.Fatal("source rebuild", receipt, err)
	}
	for _, p := range prepared {
		decoded, recognized, err := legacy.Decode(p.Message.Input().Payload)
		if err != nil || !recognized {
			t.Fatal(err)
		}
		if err := consumer(ctx, "answersheet.submitted", decoded.Payload); err != nil {
			t.Fatal("late original event", err)
		}
	}
	items, err := projection.Top(ctx, port.Query{WindowDays: 1, Limit: 10})
	if err != nil || len(items) != 2 || items[0].QuestionnaireCode != "Q-A" || items[0].Score != 3 || items[1].Score != 1 {
		t.Fatalf("authoritative counts=%+v err=%v", items, err)
	}
	if _, err := projection.ApplyDayRebuild(ctx, lease, captured, "isolated-m6-operator"); err != nil {
		t.Fatal("original receipt repeat", err)
	}
	count, err = db.Collection("rm_outbox").CountDocuments(ctx, bson.M{"message_id": bson.M{"$in": originalIDs}, "state": "published"})
	if err != nil || count != 4 {
		t.Fatal("repair changed original Outbox state", count, err)
	}
	// Exercise the actual operator binary, including its durable journal.
	inspectedRaw := runTool("--mode", "inspect", "--day", day)
	var inspectedCLI struct {
		Fingerprint string `json:"fingerprint"`
		ReadOnly    bool   `json:"read_only"`
	}
	if json.Unmarshal(inspectedRaw, &inspectedCLI) != nil || !inspectedCLI.ReadOnly || inspectedCLI.Fingerprint != fingerprint {
		t.Fatal("operator source approval differs")
	}
	operatorID := uuid.NewString()
	applyArgs := []string{"--mode", "apply", "--day", day, "--request-id", operatorID, "--operator", "isolated-m6-operator", "--expected-fingerprint", fingerprint, "--audit-dir", auditDir}
	first := runTool(applyArgs...)
	repeated := runTool(applyArgs...)
	if !bytes.Equal(first, repeated) {
		t.Fatal("operator repeat did not return original receipt")
	}
	for _, kind := range []string{"intent", "confirmed"} {
		stat, err := os.Stat(filepath.Join(auditDir, operatorID+"."+kind+".json"))
		if err != nil || stat.Mode().Perm() != 0600 {
			t.Fatal("operator durable audit missing", err)
		}
	}
	t.Run("whole-day-source-refuses-legacy-and-corrupt-original", func(t *testing.T) {
		if _, err := db.Collection("answersheets").InsertOne(ctx, bson.M{"domain_id": int64(999), "filled_at": now}); err != nil {
			t.Fatal(err)
		}
		if _, err := source.Capture(ctx, day); err == nil {
			t.Fatal("legacy fact silently omitted")
		}
		if _, err := db.Collection("answersheets").DeleteOne(ctx, bson.M{"domain_id": int64(999)}); err != nil {
			t.Fatal(err)
		}
		original := prepared[0].Message.Fingerprint()
		if _, err := db.Collection("rm_outbox").UpdateOne(ctx, bson.M{"message_id": originalIDs[0]}, bson.M{"$set": bson.M{"fingerprint": []byte("corrupt")}}); err != nil {
			t.Fatal(err)
		}
		if _, err := source.Capture(ctx, day); err == nil {
			t.Fatal("corrupt original accepted")
		}
		if _, err := db.Collection("rm_outbox").UpdateOne(ctx, bson.M{"message_id": originalIDs[0]}, bson.M{"$set": bson.M{"fingerprint": original[:]}}); err != nil {
			t.Fatal(err)
		}
		tooMany := make([]any, redishot.MaxRebuildFacts+1)
		for i := range tooMany {
			tooMany[i] = bson.M{"domain_id": int64(1000 + i), "filled_at": now}
		}
		if _, err := db.Collection("answersheets").InsertMany(ctx, tooMany); err != nil {
			t.Fatal(err)
		}
		if _, err := source.Capture(ctx, day); err == nil || !strings.Contains(err.Error(), "exceeds bounded budget") {
			t.Fatal("oversized day was not refused as a whole", err)
		}
		if _, err := db.Collection("answersheets").DeleteMany(ctx, bson.M{"domain_id": bson.M{"$gte": int64(1000)}}); err != nil {
			t.Fatal(err)
		}
		after, err := projection.Top(ctx, port.Query{WindowDays: 1, Limit: 10})
		if err != nil || len(after) != 2 || after[0].Score != 3 || after[1].Score != 1 {
			t.Fatal("refused source changed rank", err)
		}
	})
	t.Logf("actual NSQ confirmation loss: published=4 depth4->0; snapshot=4 including soft-deleted fact; authoritative rank Q-A=3 Q-B=1; original wire duplicates and receipt repeat do not increment; original Outbox remains published; actual CLI prepare/inspect/apply/repeat and protected durable audit verified; whole-day legacy/corrupt/oversized sources refused")
}
func hotProofNSQDepth(t *testing.T, address string) int64 {
	t.Helper()
	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(address + "/stats?format=json")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 65536))
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Topics []struct {
			Name  string `json:"topic_name"`
			Depth int64  `json:"depth"`
		} `json:"topics"`
	}
	if response.StatusCode != 200 || json.Unmarshal(raw, &data) != nil {
		t.Fatal("bounded NSQ stats failed")
	}
	for _, topic := range data.Topics {
		if topic.Name == "qs.evaluation.lifecycle" {
			return topic.Depth
		}
	}
	return 0
}

type hotProofContainer struct {
	ID     string `json:"Id"`
	Config struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	State struct {
		ExitCode int  `json:"ExitCode"`
		Running  bool `json:"Running"`
	} `json:"State"`
	NetworkSettings struct {
		Ports map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
	} `json:"NetworkSettings"`
}

func hotProofRestartOwnedNSQ(t *testing.T, name, owner string) string {
	t.Helper()
	inspect := func() hotProofContainer {
		raw, err := exec.Command("docker", "inspect", name).Output()
		if err != nil {
			t.Fatal(err)
		}
		var values []hotProofContainer
		if err := json.Unmarshal(raw, &values); err != nil || len(values) != 1 {
			t.Fatal("invalid owned container", err)
		}
		if values[0].Config.Labels["m6-hotrank-proof-owner"] != owner {
			t.Fatal("unrelated broker, refusing fault injection")
		}
		return values[0]
	}
	original := inspect()
	if output, err := exec.Command("docker", "kill", "--signal=KILL", original.ID).CombinedOutput(); err != nil {
		t.Fatal(fmt.Sprintf("owned broker kill failed: %s", output), err)
	}
	killed := inspect()
	if killed.ID != original.ID || killed.State.ExitCode != 137 {
		t.Fatal("actual SIGKILL exit137 not observed")
	}
	if output, err := exec.Command("docker", "start", original.ID).CombinedOutput(); err != nil {
		t.Fatal(fmt.Sprintf("same broker restart failed: %s", output), err)
	}
	restarted := inspect()
	if restarted.ID != original.ID {
		t.Fatal("broker identity changed")
	}
	// Docker can allocate a new ephemeral host port when starting the same
	// container. Query the actual loopback mapping after the verified restart.
	bindings := restarted.NetworkSettings.Ports["4151/tcp"]
	if len(bindings) != 1 || bindings[0].HostIP != "127.0.0.1" {
		t.Fatal("owned broker restart lost loopback isolation")
	}
	port, err := strconv.Atoi(bindings[0].HostPort)
	if err != nil || port < 1 || port > 65535 {
		t.Fatal("invalid owned broker HTTP mapping")
	}
	address := "http://127.0.0.1:" + bindings[0].HostPort
	t.Logf("owned broker same ID, exit137; HTTP mapping %s -> %s", os.Getenv("M6_HOTRANK_NSQ_HTTP"), address)
	client := http.Client{Timeout: time.Second}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		r, err := client.Get(address + "/ping")
		if err == nil {
			r.Body.Close()
			if r.StatusCode == 200 {
				return address
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	state := inspect()
	logs, _ := exec.Command("docker", "logs", "--tail", "30", original.ID).CombinedOutput()
	t.Fatalf("owned broker restart readiness timeout, running=%v exit=%d logs=%s", state.State.Running, state.State.ExitCode, logs)
	return ""
}
