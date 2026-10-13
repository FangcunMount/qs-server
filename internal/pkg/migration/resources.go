package migration

import (
	"encoding/json"
	"io/fs"
	"regexp"
	"strconv"
	"strings"
)

type schemaVersionPair struct{ SQL, Mongo uint }

// Add a pair only when both schemas are independently safe for this release.
// This is deliberately not the cross product of the backend version inventories.
var declaredSchemaPairs = []schemaVersionPair{{99, 38}, {100, 39}}

type backendResources struct {
	head uint
	up   map[uint]string
}

type migrationResources struct {
	source     fs.FS
	sql, mongo backendResources
	pairs      []schemaVersionPair
	sha256     string
}

func readMigrationResources(source fs.FS, pairs []schemaVersionPair) (*migrationResources, error) {
	r := &migrationResources{source: source, pairs: append([]schemaVersionPair(nil), pairs...)}
	var inventory [][3]string
	for _, item := range []struct {
		directory, extension string
		target               *backendResources
	}{
		{"mysql", "sql", &r.sql}, {"mongodb", "json", &r.mongo},
	} {
		item.target.up = make(map[uint]string)
		entries, err := fs.ReadDir(source, "migrations/"+item.directory)
		if err != nil {
			return nil, retirementError("embedded migration inventory unavailable")
		}
		pattern := regexp.MustCompile(`^([0-9]{6})_[a-z0-9_]+\.(up|down)\.` + item.extension + `$`)
		for _, entry := range entries {
			match := pattern.FindStringSubmatch(entry.Name())
			if entry.IsDir() {
				return nil, retirementError("unexpected embedded migration directory")
			}
			if match == nil {
				if strings.Contains(entry.Name(), ".up.") || strings.Contains(entry.Name(), ".down.") {
					return nil, retirementError("embedded migration filename malformed")
				}
				continue
			}
			v, err := strconv.ParseUint(match[1], 10, 32)
			if err != nil || v == 0 {
				return nil, retirementError("embedded migration version malformed")
			}
			version := uint(v)
			if match[2] == "up" {
				if _, exists := item.target.up[version]; exists {
					return nil, retirementError("duplicate embedded migration version")
				}
			}
			path := "migrations/" + item.directory + "/" + entry.Name()
			raw, err := fs.ReadFile(source, path)
			if err != nil || len(raw) == 0 {
				return nil, retirementError("embedded migration body unavailable")
			}
			hash := retirementHash(raw)
			if match[2] == "up" {
				item.target.up[version] = hash
			}
			inventory = append(inventory, [3]string{item.directory, path, hash})
			if match[2] == "up" && version > item.target.head {
				item.target.head = version
			}
		}
	}
	if r.sql.head < compatibilitySQLVersion || r.mongo.head < compatibilityMongoVersion {
		return nil, retirementError("embedded inventory lacks retirement boundary")
	}
	seen := make(map[schemaVersionPair]bool)
	for _, pair := range r.pairs {
		if seen[pair] || !r.sql.contains(pair.SQL) || !r.mongo.contains(pair.Mongo) ||
			(pair != (schemaVersionPair{99, 38}) && (pair.SQL < compatibilitySQLVersion || pair.Mongo < compatibilityMongoVersion)) {
			return nil, retirementError("declared schema pair malformed or unavailable")
		}
		seen[pair] = true
	}
	if !r.accepts(r.sql.head, r.mongo.head) {
		return nil, retirementError("embedded target pair is not declared safe")
	}
	raw, err := json.Marshal(inventory)
	if err != nil {
		return nil, err
	}
	r.sha256 = retirementHash(raw)
	return r, nil
}

func (r backendResources) contains(version uint) bool { _, ok := r.up[version]; return ok }
func (r *migrationResources) accepts(sql, mongo uint) bool {
	for _, pair := range r.pairs {
		if pair.SQL == sql && pair.Mongo == mongo {
			return true
		}
	}
	return false
}
func (r *migrationResources) backend(backend Backend) backendResources {
	if backend == BackendMongo {
		return r.mongo
	}
	return r.sql
}

func compiledMigrationResources() (*migrationResources, error) {
	return readMigrationResources(migrations, declaredSchemaPairs)
}

// EmbeddedMigrationResources returns the actual compiled target heads and the
// complete up/down resource digest used to bind a pristine bootstrap approval.
func EmbeddedMigrationResources() (mysqlVersion, mongoVersion uint, sha256 string, err error) {
	resources, err := compiledMigrationResources()
	if err != nil {
		return 0, 0, "", err
	}
	return resources.sql.head, resources.mongo.head, resources.sha256, nil
}

func migrationSource(config *Config) fs.FS {
	if config.retirementPair != nil {
		return config.retirementPair.resources.source
	}
	return migrations
}
