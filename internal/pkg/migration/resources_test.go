package migration

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

func futureMigrationSource(t *testing.T) fstest.MapFS {
	t.Helper()
	source := make(fstest.MapFS)
	if err := fs.WalkDir(migrations, "migrations", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, err := migrations.ReadFile(path)
		if err != nil {
			return err
		}
		source[path] = &fstest.MapFile{Data: raw}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	source["migrations/mysql/000101_future_test.up.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE migration_future_probe (id BIGINT PRIMARY KEY);\n")}
	source["migrations/mongodb/000040_future_test.up.json"] = &fstest.MapFile{Data: []byte(`[{"create":"migration_future_probe"}]`)}
	return source
}

func TestMigrationResourcesRequireActualVersionsAndDeclaredPairs(t *testing.T) {
	current, err := compiledMigrationResources()
	if err != nil || current.sql.head != 100 || current.mongo.head != 39 || !current.accepts(99, 38) || !current.accepts(100, 39) {
		t.Fatal(current, err)
	}
	pairs := []schemaVersionPair{{99, 38}, {100, 39}, {101, 39}, {101, 40}}
	source := futureMigrationSource(t)
	future, err := readMigrationResources(source, pairs)
	if err != nil || future.sql.head != 101 || future.mongo.head != 40 || future.sha256 == current.sha256 {
		t.Fatal(future, err)
	}
	if future.accepts(100, 40) || future.accepts(100, 38) || future.sql.contains(102) {
		t.Fatal("undeclared or unavailable version accepted")
	}
	for _, test := range []struct {
		name  string
		pairs []schemaVersionPair
	}{
		{"target_undeclared", []schemaVersionPair{{100, 39}}},
		{"invented_version", append(append([]schemaVersionPair{}, pairs...), schemaVersionPair{102, 40})},
		{"cross_boundary", append(append([]schemaVersionPair{}, pairs...), schemaVersionPair{100, 38})},
		{"duplicate_pair", append(append([]schemaVersionPair{}, pairs...), schemaVersionPair{101, 40})},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := readMigrationResources(source, test.pairs); err == nil {
				t.Fatal("invalid declaration accepted")
			}
		})
	}
	for _, name := range []string{"migrations/mysql/000102_wrong_extension.up.json", "migrations/mysql/102_malformed.up.sql"} {
		malformed := futureMigrationSource(t)
		malformed[name] = &fstest.MapFile{Data: []byte("SELECT 1;")}
		if _, err := readMigrationResources(malformed, pairs); err == nil {
			t.Fatal("malformed source filename accepted", name)
		}
	}
	delete(source, "migrations/mysql/000101_future_test.up.sql")
	source["migrations/mysql/000102_future_test.up.sql"] = &fstest.MapFile{Data: []byte("SELECT 1;")}
	if _, err := readMigrationResources(source, pairs); err == nil {
		t.Fatal("missing declared prefix version accepted")
	}
}

func TestMigrationResourceHashBindsFullInventory(t *testing.T) {
	pairs := []schemaVersionPair{{100, 39}, {101, 40}}
	source := futureMigrationSource(t)
	first, err := readMigrationResources(source, pairs)
	if err != nil {
		t.Fatal(err)
	}
	source["migrations/mysql/000001_init_actor_schema.up.sql"].Data = []byte("SELECT 1;\n")
	changed, err := readMigrationResources(source, pairs)
	if err != nil || first.sha256 == changed.sha256 {
		t.Fatal("historical body mutation not bound", err)
	}
	source["migrations/mysql/000100_retire_compatibility_message_storage.down.sql"].Data = []byte("SELECT 1;")
	downChanged, err := readMigrationResources(source, pairs)
	if err != nil || downChanged.sha256 == changed.sha256 {
		t.Fatal("down resource mutation not bound", err)
	}
	source["migrations/mysql/000101_duplicate_test.up.sql"] = &fstest.MapFile{Data: []byte("SELECT 1;")}
	if _, err := readMigrationResources(source, pairs); err == nil {
		t.Fatal("duplicate embedded version accepted")
	}
}
