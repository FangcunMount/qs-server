package migration

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"io"
	"reflect"
	"strings"
	"time"

	migratedb "github.com/golang-migrate/migrate/v4/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type retirementDatabaseDriver struct {
	migratedb.Driver
	backend       Backend
	pair          *PairPreflight
	sqlConn       *sql.Conn
	client        *mongo.Client
	database      string
	version       int
	dirty         bool
	upHash        string
	downHash      string
	bodyValid     bool
	tailSucceeded bool
}

func newRetirementDatabaseDriver(driver migratedb.Driver, backend Backend, cfg *Config, conn *sql.Conn, client *mongo.Client, fs embed.FS) *retirementDatabaseDriver {
	directory, base := "mysql", "000100_retire_compatibility_message_storage"
	extension := "sql"
	if backend == BackendMongo {
		directory, base, extension = "mongodb", "000039_retire_compatibility_message_storage", "json"
	}
	up, _ := fs.ReadFile("migrations/" + directory + "/" + base + ".up." + extension)
	down, _ := fs.ReadFile("migrations/" + directory + "/" + base + ".down." + extension)
	canonical, err := migrations.ReadFile("migrations/" + directory + "/" + base + ".up." + extension)
	exact := err == nil && bytes.Equal(up, canonical)
	if backend == BackendMySQL {
		exact = exact && strings.TrimSpace(string(up)) == "DROP TABLE IF EXISTS `domain_event_outbox`;\nDROP TABLE IF EXISTS `ai_bridge_commands`;\nDROP TABLE IF EXISTS `ai_messaging_legacy_commands`;"
	} else {
		exact = exact && strings.TrimSpace(string(up)) == `[{"drop":"domain_event_outbox"}]`
	}
	return &retirementDatabaseDriver{Driver: driver, backend: backend, pair: cfg.retirementPair, sqlConn: conn, client: client, database: cfg.Database, upHash: retirementHash(canonical), downHash: retirementHash(down), bodyValid: exact}
}
func (d *retirementDatabaseDriver) tailVersion() int {
	if d.backend == BackendMongo {
		return 39
	}
	return 100
}
func (d *retirementDatabaseDriver) SetVersion(version int, dirty bool) error {
	if version > d.tailVersion() {
		return retirementError("unsupported post-retirement version write")
	}
	if version >= d.tailVersion() && (d.pair == nil || !d.bodyValid) {
		return retirementError("paired preflight required before retirement version write")
	}
	if version < d.tailVersion() {
		current, _, err := d.Version()
		if err != nil {
			return err
		}
		if current >= d.tailVersion() {
			return retirementError("retirement head downgrade or force refused")
		}
	}
	if version == d.tailVersion() {
		if !dirty && !d.tailSucceeded {
			return retirementError("retirement clean write without successful exact run refused")
		}
		d.tailSucceeded = false
	}
	if e := d.Driver.SetVersion(version, dirty); e != nil {
		return e
	}
	d.version, d.dirty = version, dirty
	return nil
}
func (d *retirementDatabaseDriver) Run(reader io.Reader) error {
	raw, e := io.ReadAll(reader)
	if e != nil {
		return e
	}
	hash := retirementHash(raw)
	if hash == d.downHash {
		return retirementError("irreversible retirement down refused")
	}
	if hash == d.upHash && d.version != d.tailVersion() {
		return retirementError("retirement exact body at wrong version rejected")
	}
	if d.version != d.tailVersion() {
		return d.Driver.Run(bytes.NewReader(raw))
	}
	if d.pair == nil || !d.dirty || hash != d.upHash || !d.bodyValid {
		return retirementError("retirement version or exact body binding rejected")
	}
	if d.backend == BackendMySQL {
		e = d.runSQLTail(raw)
	} else {
		e = d.runMongoTail(raw)
	}
	if e == nil {
		d.tailSucceeded = true
	}
	return e
}
func (d *retirementDatabaseDriver) runSQLTail(raw []byte) error {
	if d.sqlConn == nil || d.database != d.pair.config.MySQLDatabase {
		return retirementError("mysql tail selected connection mismatch")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var selected, uuid string
	if d.sqlConn.QueryRowContext(ctx, "SELECT DATABASE(),@@server_uuid").Scan(&selected, &uuid) != nil || selected != d.database {
		return retirementError("mysql tail identity unknown")
	}
	encoded, _ := marshalSQLIdentity(uuid, selected)
	if retirementHash(encoded) != d.pair.sqlHash {
		return retirementError("mysql tail identity changed")
	}
	if actual, e := retirementMongoIdentity(ctx, d.pair.mongo, d.pair.config.MongoDatabase); e != nil || actual != d.pair.mongoHash {
		return retirementError("mysql tail paired mongo identity changed")
	}
	for _, name := range retirementSQLNames {
		var kind string
		e := d.sqlConn.QueryRowContext(ctx, "SELECT TABLE_TYPE FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?", name).Scan(&kind)
		if e == sql.ErrNoRows {
			continue
		}
		if e != nil || kind != "BASE TABLE" {
			return retirementError("mysql tail namespace unknown")
		}
		if !d.pair.pristine {
			return retirementError("mysql installed target reappeared")
		}
		var count int
		if d.sqlConn.QueryRowContext(ctx, "SELECT /*+ MAX_EXECUTION_TIME(15000) */ COUNT(*) FROM `"+name+"`").Scan(&count) != nil || count != 0 {
			return retirementError("mysql pristine tail nonempty or unknown")
		}
	}
	if e := d.Driver.Run(bytes.NewReader(raw)); e != nil {
		return e
	}
	for _, name := range retirementSQLNames {
		var count int
		if d.sqlConn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?", name).Scan(&count) != nil || count != 0 {
			return retirementError("mysql tail absence unknown")
		}
	}
	return nil
}
func (d *retirementDatabaseDriver) runMongoTail(raw []byte) error {
	if d.client == nil || d.database != d.pair.config.MongoDatabase {
		return retirementError("mongo tail selected connection mismatch")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db := d.client.Database(d.database)
	a, b, e := pairIdentity(ctx, d.pair.sqlDB, d.client, d.pair.config.MySQLDatabase, d.database)
	if e != nil || a != d.pair.sqlHash || b != d.pair.mongoHash {
		return retirementError("mongo tail pair identity changed")
	}
	collections, e := mongoRetirementNamespace(ctx, db)
	if e != nil {
		return e
	}
	if len(collections) == 0 {
		return nil
	}
	if len(collections) != 1 || collections[0]["type"] != "collection" || !d.pair.pristine {
		return retirementError("mongo installed target reappeared or kind rejected")
	}
	count, e := db.Collection("domain_event_outbox").CountDocuments(ctx, bson.D{}, options.Count().SetLimit(1).SetMaxTime(15*time.Second))
	if e != nil || count != 0 {
		return retirementError("mongo pristine tail nonempty or unknown")
	}
	beforeUUID, e := mongoRetirementUUID(collections[0])
	if e != nil {
		return e
	}
	confirmed, e := mongoRetirementNamespace(ctx, db)
	if e != nil || len(confirmed) != 1 {
		return retirementError("mongo pristine tail namespace changed")
	}
	afterUUID, e := mongoRetirementUUID(confirmed[0])
	if e != nil || !reflect.DeepEqual(beforeUUID, afterUUID) {
		return retirementError("mongo pristine tail namespace identity changed")
	}
	if e = d.Driver.Run(bytes.NewReader(raw)); e != nil {
		return e
	}
	collections, e = mongoRetirementNamespace(ctx, db)
	if e != nil || len(collections) != 0 {
		return retirementError("mongo tail absence unknown")
	}
	return nil
}

func mongoRetirementUUID(collection bson.M) (any, error) {
	info, ok := collection["info"].(bson.M)
	if !ok {
		return nil, retirementError("mongo pristine tail UUID unknown")
	}
	uuid, ok := info["uuid"].(primitive.Binary)
	if !ok || uuid.Subtype != 4 || len(uuid.Data) != 16 {
		return nil, retirementError("mongo pristine tail UUID unknown")
	}
	return uuid, nil
}
func mongoRetirementNamespace(ctx context.Context, db *mongo.Database) ([]bson.M, error) {
	cursor, e := db.ListCollections(ctx, bson.D{{Key: "name", Value: "domain_event_outbox"}}, options.ListCollections().SetNameOnly(false).SetAuthorizedCollections(false))
	if e != nil {
		return nil, retirementError("mongo tail visibility unknown")
	}
	var result []bson.M
	e = cursor.All(ctx, &result)
	closeErr := cursor.Close(ctx)
	if e != nil || closeErr != nil {
		return nil, retirementError("mongo tail namespace unknown")
	}
	return result, nil
}
