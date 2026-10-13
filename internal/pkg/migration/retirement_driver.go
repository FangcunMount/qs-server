package migration

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"io/fs"
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
	ctx           context.Context
	sqlConn       *sql.Conn
	client        *mongo.Client
	database      string
	version       int
	dirty         bool
	upHash        string
	downHash      string
	bodyValid     bool
	stepSucceeded bool
	resources     *migrationResources
	sourceValid   bool
}

func newRetirementDatabaseDriver(driver migratedb.Driver, backend Backend, cfg *Config, conn *sql.Conn, client *mongo.Client, source fs.FS) *retirementDatabaseDriver {
	directory, base := "mysql", "000100_retire_compatibility_message_storage"
	extension := "sql"
	if backend == BackendMongo {
		directory, base, extension = "mongodb", "000039_retire_compatibility_message_storage", "json"
	}
	up, _ := fs.ReadFile(source, "migrations/"+directory+"/"+base+".up."+extension)
	down, _ := fs.ReadFile(source, "migrations/"+directory+"/"+base+".down."+extension)
	canonical, err := migrations.ReadFile("migrations/" + directory + "/" + base + ".up." + extension)
	exact := err == nil && bytes.Equal(up, canonical)
	if backend == BackendMySQL {
		exact = exact && strings.TrimSpace(string(up)) == "DROP TABLE IF EXISTS `domain_event_outbox`;\nDROP TABLE IF EXISTS `ai_bridge_commands`;\nDROP TABLE IF EXISTS `ai_messaging_legacy_commands`;"
	} else {
		exact = exact && strings.TrimSpace(string(up)) == `[{"drop":"domain_event_outbox"}]`
	}
	resources, resourceErr := compiledMigrationResources()
	if cfg.retirementPair != nil {
		resources, resourceErr = readMigrationResources(source, cfg.retirementPair.resources.pairs)
	}
	sourceValid := resourceErr == nil
	if cfg.retirementPair != nil {
		sourceValid = sourceValid && resources.sha256 == cfg.retirementPair.resources.sha256
	}
	return &retirementDatabaseDriver{resources: resources, sourceValid: sourceValid, Driver: driver, backend: backend, pair: cfg.retirementPair, ctx: migrationRunContext(cfg), sqlConn: conn, client: client, database: cfg.Database, upHash: retirementHash(canonical), downHash: retirementHash(down), bodyValid: exact}
}
func (d *retirementDatabaseDriver) tailVersion() int {
	if d.backend == BackendMongo {
		return 39
	}
	return 100
}
func (d *retirementDatabaseDriver) SetVersion(version int, dirty bool) error {
	current, currentDirty, err := d.Driver.Version()
	if err != nil {
		return err
	}
	if version < d.tailVersion() {
		if current >= d.tailVersion() {
			return retirementError("retirement head downgrade or force refused")
		}
	} else {
		if d.pair == nil || !d.bodyValid || !d.sourceValid || version < 0 || !d.resources.backend(d.backend).contains(uint(version)) {
			return retirementError("paired embedded migration required before version write")
		}
		if dirty {
			if currentDirty || version != d.nextVersion(current) {
				return retirementError("non-forward or dirty migration version write refused")
			}
			if version > d.tailVersion() {
				if err := d.verifyPostRetirementStep(); err != nil {
					return err
				}
			}
		} else {
			if !d.dirty || d.version != version || !d.stepSucceeded {
				return retirementError("clean version write without successful bound run refused")
			}
			if version > d.tailVersion() {
				if err := d.verifyPostRetirementStep(); err != nil {
					return err
				}
			}
		}
	}
	if err := d.Driver.SetVersion(version, dirty); err != nil {
		return err
	}
	d.version, d.dirty = version, dirty
	d.stepSucceeded = false
	return nil
}
func (d *retirementDatabaseDriver) nextVersion(current int) int {
	next := -1
	for version := range d.resources.backend(d.backend).up {
		if int(version) > current && (next == -1 || int(version) < next) {
			next = int(version)
		}
	}
	return next
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
	if d.version < d.tailVersion() {
		return d.Driver.Run(bytes.NewReader(raw))
	}
	if d.version > d.tailVersion() {
		expected := ""
		if d.resources != nil {
			expected = d.resources.backend(d.backend).up[uint(d.version)]
		}
		if d.pair == nil || !d.dirty || !d.sourceValid || expected == "" || hash != expected {
			return retirementError("post-retirement migration body or phase rejected")
		}
		if err := d.verifyPostRetirementStep(); err != nil {
			return err
		}
		if err := d.Driver.Run(bytes.NewReader(raw)); err != nil {
			return err
		}
		d.stepSucceeded = true
		return nil
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
		d.stepSucceeded = true
	}
	return e
}
func (d *retirementDatabaseDriver) runSQLTail(raw []byte) error {
	if d.sqlConn == nil || d.database != d.pair.config.MySQLDatabase {
		return retirementError("mysql tail selected connection mismatch")
	}
	ctx, cancel := context.WithTimeout(d.ctx, 15*time.Second)
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
	ctx, cancel := context.WithTimeout(d.ctx, 15*time.Second)
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

func (d *retirementDatabaseDriver) verifyPostRetirementStep() error {
	ctx, cancel := retirementContext(d.ctx)
	defer cancel()
	var sqlReader retirementSQLReader = d.pair.sqlDB
	if d.backend == BackendMySQL {
		sqlReader = d.sqlConn
	}
	a, b, err := pairIdentity(ctx, sqlReader, d.pair.mongo, d.pair.config.MySQLDatabase, d.pair.config.MongoDatabase)
	if err != nil || a != d.pair.sqlHash || b != d.pair.mongoHash {
		return retirementError("post-retirement pair identity changed or unknown")
	}
	if d.backend == BackendMongo {
		collections, err := mongoRetirementNamespace(ctx, d.client.Database(d.database))
		if err != nil || len(collections) != 0 {
			return retirementError("post-retirement mongo target reappeared or unknown")
		}
		return nil
	}
	for _, name := range retirementSQLNames {
		var count int
		if d.sqlConn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?", name).Scan(&count) != nil || count != 0 {
			return retirementError("post-retirement mysql target reappeared or unknown")
		}
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
