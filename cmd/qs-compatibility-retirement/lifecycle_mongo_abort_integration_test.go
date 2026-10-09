//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type comparisonAbortUnknownWire struct {
	armed atomic.Bool
	acked atomic.Bool
}

// The test alone withholds a real loopback server reply AFTER it was read and
// validated. Production sees a genuine unexpected-EOF result; no response,
// session, permission or completion flag is injected into the real producer.
type comparisonAbortUnknownConn struct {
	net.Conn
	fault   *comparisonAbortUnknownWire
	pending atomic.Bool
}

func (c *comparisonAbortUnknownConn) Write(p []byte) (int, error) {
	if len(p) >= 26 && binary.LittleEndian.Uint32(p[12:16]) == 2013 && p[20] == 0 {
		doc := bson.Raw(p[21:])
		if doc.Validate() == nil && doc.Lookup("abortTransaction").Type != 0 && c.fault.armed.CompareAndSwap(false, true) {
			c.pending.Store(true)
		}
	}
	return c.Conn.Write(p)
}

func (c *comparisonAbortUnknownConn) Read(p []byte) (int, error) {
	if !c.pending.Swap(false) {
		return c.Conn.Read(p)
	}
	var header [16]byte
	if _, e := io.ReadFull(c.Conn, header[:]); e != nil {
		return 0, e
	}
	n := int(binary.LittleEndian.Uint32(header[:4]))
	if n < 26 || n > 65536 || binary.LittleEndian.Uint32(header[12:]) != 2013 {
		_ = c.Close()
		return 0, io.ErrUnexpectedEOF
	}
	body := make([]byte, n-16)
	if _, e := io.ReadFull(c.Conn, body); e != nil {
		return 0, e
	}
	if len(body) >= 10 && binary.LittleEndian.Uint32(body[:4]) == 0 && body[4] == 0 {
		size := int(binary.LittleEndian.Uint32(body[5:9]))
		if size >= 5 && size == len(body)-5 && lifecycleMongoAbortAcknowledged(bson.Raw(body[5:])) {
			c.fault.acked.Store(true) // Test-only knowledge, never passed to producer.
		}
	}
	_ = c.Close()
	return 0, io.ErrUnexpectedEOF
}

func comparisonAbortOwnedNative(t *testing.T, fault *comparisonAbortUnknownWire) (*mongo.Client, *mongo.Database) {
	t.Helper()
	if os.Getenv("QS_RETIREMENT_REQUIRE_ABORT_NATIVE") != "1" {
		t.Skip("owned original-session abort fixture not requested")
	}
	// The actual owned fixture identity/socket is separately inspected by the
	// runner before this test. Only the existing local and CI owned literals are
	// accepted, each with its exact dial address and observed replica-set name.
	uri := os.Getenv("QS_SERVER_TEST_MONGO_URI")
	address, setName, e := comparisonAbortNativeFixture(uri)
	if e != nil {
		t.Fatal("exact approved owned loopback Mongo fixture required")
	}
	opts := options.Client().ApplyURI(uri).SetConnectTimeout(10 * time.Second).SetServerSelectionTimeout(10 * time.Second).SetSocketTimeout(10 * time.Second)
	if fault != nil {
		opts.SetDialer(comparisonAbortNativeDialer{fault: fault, address: address})
	}
	q, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	client, e := mongo.Connect(q, opts)
	if e != nil {
		t.Fatal("actual owned Mongo connection failed")
	}
	t.Cleanup(func() {
		cleanup, end := context.WithTimeout(context.Background(), 15*time.Second)
		defer end()
		if e := client.Disconnect(cleanup); e != nil {
			t.Error("actual owned client close failed")
		}
	})
	if client.Ping(q, readpref.Primary()) != nil {
		t.Fatal("actual owned Mongo connection failed")
	}
	var hello struct {
		SetName string `bson:"setName"`
	}
	if e = client.Database("admin").RunCommand(q, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); e != nil || hello.SetName != setName {
		t.Fatal("actual owned replica set identity rejected")
	}
	db := client.Database("qs_retirement_abort_test_" + primitive.NewObjectID().Hex())
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if e := db.Drop(cleanup); e != nil {
			t.Error("actual owned namespace cleanup failed")
		}
	})
	return client, db
}

func comparisonAbortNativeFixture(uri string) (address, setName string, err error) {
	switch uri {
	case "mongodb://127.0.0.1:33317/?authSource=admin&replicaSet=qscompat&directConnection=true":
		return "127.0.0.1:33317", "qscompat", nil
	case "mongodb://127.0.0.1:27017/?replicaSet=rs0":
		return "127.0.0.1:27017", "rs0", nil
	default:
		return "", "", lifecycleError("owned_abort_fixture_fixed_route_required")
	}
}

type comparisonAbortNativeDialer struct {
	fault   *comparisonAbortUnknownWire
	address string
}

func (d comparisonAbortNativeDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" || (d.address != "127.0.0.1:33317" && d.address != "127.0.0.1:27017") || address != d.address {
		return nil, lifecycleError("owned_abort_fixture_fixed_route_required")
	}
	c, e := (&net.Dialer{}).DialContext(ctx, network, address)
	if e != nil {
		return nil, e
	}
	return &comparisonAbortUnknownConn{Conn: c, fault: d.fault}, nil
}

func TestComparisonMongoAbortNativeFixtureWhitelist(t *testing.T) {
	for uri, expected := range map[string][2]string{
		"mongodb://127.0.0.1:33317/?authSource=admin&replicaSet=qscompat&directConnection=true": {"127.0.0.1:33317", "qscompat"},
		"mongodb://127.0.0.1:27017/?replicaSet=rs0":                                             {"127.0.0.1:27017", "rs0"},
	} {
		address, setName, e := comparisonAbortNativeFixture(uri)
		if e != nil || address != expected[0] || setName != expected[1] {
			t.Fatal("exact owned fixture binding changed")
		}
	}
	for _, uri := range []string{"", "mongodb://production:27017/?replicaSet=rs0", "mongodb://127.0.0.1:27018/?replicaSet=rs0", "mongodb://127.0.0.1:33317/?replicaSet=rs0", "mongodb://127.0.0.1:27017/?replicaSet=qscompat", "mongodb://user:password@127.0.0.1:27017/?replicaSet=rs0", "mongodb://127.0.0.1:27017/?replicaSet=rs0&directConnection=true"} {
		if _, _, e := comparisonAbortNativeFixture(uri); e == nil {
			t.Fatal("arbitrary URI became an owned native fixture")
		}
	}
}

func comparisonAbortActualSnapshot(t *testing.T, client *mongo.Client, db *mongo.Database) mongo.Session {
	t.Helper()
	if _, e := db.Collection("original_fact").InsertOne(t.Context(), bson.D{{Key: "_id", Value: int32(1)}, {Key: "fact", Value: "owned-native-abort-fixture"}}); e != nil {
		t.Fatal("actual owned fact setup failed")
	}
	s, e := client.StartSession()
	if e != nil {
		t.Fatal("actual owned session setup failed")
	}
	t.Cleanup(func() { s.EndSession(context.Background()) })
	if e = s.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot()).SetReadPreference(readpref.Primary())); e != nil {
		t.Fatal("actual owned snapshot start failed")
	}
	var fact bson.Raw
	if e = db.Collection("original_fact").FindOne(mongo.NewSessionContext(t.Context(), s), bson.D{{Key: "_id", Value: int32(1)}}).Decode(&fact); e != nil {
		t.Fatal("actual original session snapshot read failed")
	}
	return s
}

// These execute only in the actual separately verified owned fixture. They
// prove abort response/refusal, not full fence, DROP, Window or acceptance.
func TestComparisonMongoAbortNativeOriginalSessionResponseAndUnknown(t *testing.T) {
	t.Run("actual-same-session-success", func(t *testing.T) {
		client, db := comparisonAbortOwnedNative(t, nil)
		s := comparisonAbortActualSnapshot(t, client, db)
		id := append(bson.Raw(nil), s.ID()...)
		q, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		if e := lifecycleAbortComparisonMongo(q, client, s); e != nil || !bytes.Equal(id, s.ID()) {
			t.Fatal("original server acknowledgement or session identity was lost")
		}
		if e := s.AbortTransaction(q); e != nil {
			t.Fatal("native driver local state cleanup failed")
		}
		if lifecycleAbortComparisonMongo(q, client, s) == nil {
			t.Fatal("ended original session/transaction was reused as completion")
		}
	})
	t.Run("canceled-rejected-before-fact", func(t *testing.T) {
		client, db := comparisonAbortOwnedNative(t, nil)
		s := comparisonAbortActualSnapshot(t, client, db)
		q, cancel := context.WithCancel(t.Context())
		cancel()
		if lifecycleAbortComparisonMongo(q, client, s) == nil {
			t.Fatal("canceled cleanup minted success")
		}
		cleanup, end := context.WithTimeout(context.Background(), 10*time.Second)
		defer end()
		if lifecycleAbortComparisonMongo(cleanup, client, s) != nil || s.AbortTransaction(cleanup) != nil {
			t.Fatal("original owned session cleanup failed")
		}
	})
	t.Run("actual-server-reply-withheld-is-unknown", func(t *testing.T) {
		fault := &comparisonAbortUnknownWire{}
		client, db := comparisonAbortOwnedNative(t, fault)
		s := comparisonAbortActualSnapshot(t, client, db)
		q, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		if lifecycleAbortComparisonMongo(q, client, s) == nil || !fault.armed.Load() || !fault.acked.Load() {
			t.Fatal("actual withheld reply became producer success or real fault was not exercised")
		}
		if e := s.AbortTransaction(q); e != nil {
			t.Fatal("original driver local state cleanup failed")
		}
	})
}
