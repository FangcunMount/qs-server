package migration

import (
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

func testCurrentResources(t *testing.T) *migrationResources {
	t.Helper()
	r, err := compiledMigrationResources()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func embeddedMigrationSource(t *testing.T) fstest.MapFS {
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
	return source
}

// Build a historical release by running its actual unchanged inventory, even
// after a later release adds resources to this binary.
func migrationResourcesThrough(t *testing.T, sqlHead, mongoHead uint) *migrationResources {
	t.Helper()
	source := embeddedMigrationSource(t)
	for path := range source {
		parts := strings.Split(path, "/")
		if len(parts) != 3 || (!strings.Contains(parts[2], ".up.") && !strings.Contains(parts[2], ".down.")) {
			continue
		}
		version, err := strconv.ParseUint(parts[2][:6], 10, 32)
		if err != nil {
			t.Fatal(err)
		}
		if (parts[1] == "mysql" && uint(version) > sqlHead) || (parts[1] == "mongodb" && uint(version) > mongoHead) {
			delete(source, path)
		}
	}
	var pairs []schemaVersionPair
	for _, pair := range declaredSchemaPairs {
		if pair.SQL <= sqlHead && pair.Mongo <= mongoHead {
			pairs = append(pairs, pair)
		}
	}
	r, err := readMigrationResources(source, pairs)
	if err != nil || r.sql.head != sqlHead || r.mongo.head != mongoHead {
		t.Fatal("historical release inventory unavailable", err)
	}
	return r
}

func futureMigrationPaths(t *testing.T) (string, string) {
	t.Helper()
	current := testCurrentResources(t)
	return fmt.Sprintf("migrations/mysql/%06d_future_test.up.sql", current.sql.head+1),
		fmt.Sprintf("migrations/mongodb/%06d_future_test.up.json", current.mongo.head+1)
}

func futureMigrationPairs(t *testing.T) []schemaVersionPair {
	t.Helper()
	current := testCurrentResources(t)
	pairs := append([]schemaVersionPair(nil), declaredSchemaPairs...)
	return append(pairs, schemaVersionPair{current.sql.head + 1, current.mongo.head}, schemaVersionPair{current.sql.head + 1, current.mongo.head + 1})
}

func futureMigrationSource(t *testing.T) fstest.MapFS {
	t.Helper()
	source := embeddedMigrationSource(t)
	sqlPath, mongoPath := futureMigrationPaths(t)
	source[sqlPath] = &fstest.MapFile{Data: []byte("CREATE TABLE migration_future_probe (id BIGINT PRIMARY KEY);\n")}
	source[mongoPath] = &fstest.MapFile{Data: []byte(`[{"create":"migration_future_probe"}]`)}
	return source
}

func TestMigrationResourcesRequireActualVersionsAndDeclaredPairs(t *testing.T) {
	current := testCurrentResources(t)
	if !current.accepts(99, 38) || !current.accepts(100, 39) || !current.accepts(current.sql.head, current.mongo.head) {
		t.Fatal(current)
	}
	pairs := futureMigrationPairs(t)
	source := futureMigrationSource(t)
	future, err := readMigrationResources(source, pairs)
	if err != nil || future.sql.head != current.sql.head+1 || future.mongo.head != current.mongo.head+1 || future.sha256 == current.sha256 {
		t.Fatal(future, err)
	}
	if future.accepts(current.sql.head, future.mongo.head) || future.accepts(100, 38) || future.sql.contains(future.sql.head+1) {
		t.Fatal("undeclared or unavailable version accepted")
	}
	for _, test := range []struct {
		name  string
		pairs []schemaVersionPair
	}{
		{"target_undeclared", declaredSchemaPairs},
		{"invented_version", append(append([]schemaVersionPair{}, pairs...), schemaVersionPair{future.sql.head + 1, future.mongo.head})},
		{"cross_boundary", append(append([]schemaVersionPair{}, pairs...), schemaVersionPair{100, 38})},
		{"duplicate_pair", append(append([]schemaVersionPair{}, pairs...), schemaVersionPair{future.sql.head, future.mongo.head})},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := readMigrationResources(source, test.pairs); err == nil {
				t.Fatal("invalid declaration accepted")
			}
		})
	}
	for _, name := range []string{fmt.Sprintf("migrations/mysql/%06d_wrong_extension.up.json", future.sql.head+1), fmt.Sprintf("migrations/mysql/%d_malformed.up.sql", future.sql.head+1)} {
		malformed := futureMigrationSource(t)
		malformed[name] = &fstest.MapFile{Data: []byte("SELECT 1;")}
		if _, err := readMigrationResources(malformed, pairs); err == nil {
			t.Fatal("malformed source filename accepted", name)
		}
	}
	sqlPath, _ := futureMigrationPaths(t)
	delete(source, sqlPath)
	source[fmt.Sprintf("migrations/mysql/%06d_future_test.up.sql", future.sql.head+1)] = &fstest.MapFile{Data: []byte("SELECT 1;")}
	if _, err := readMigrationResources(source, pairs); err == nil {
		t.Fatal("missing declared prefix version accepted")
	}
}

func TestMigrationResourceHashBindsFullInventory(t *testing.T) {
	pairs := futureMigrationPairs(t)
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
	source[fmt.Sprintf("migrations/mysql/%06d_duplicate_test.up.sql", first.sql.head)] = &fstest.MapFile{Data: []byte("SELECT 1;")}
	if _, err := readMigrationResources(source, pairs); err == nil {
		t.Fatal("duplicate embedded version accepted")
	}
}
