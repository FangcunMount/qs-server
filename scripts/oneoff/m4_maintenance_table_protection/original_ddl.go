package main

import (
	_ "embed"
	"errors"
	"strings"
)

// These are byte-for-byte copies of the immutable release resources. Material
// DDL is evidence only; it is never interpreted or executed by this reader.
//
//go:embed original075.sql
var original075 []byte

//go:embed original078.sql
var original078 []byte

var originalCollations = [...]string{"utf8mb4_0900_ai_ci", "utf8mb4_general_ci", "utf8mb4_unicode_ci"}

func originalResourceBytesValid() bool {
	return fingerprint(original075) == "4e75a3480897c84a8b91141fe5a2da2b6e08919400ce1ea01da6f83d2294b339" && fingerprint(original078) == "75ddef7f5be0b05a19f8da77db4dba81d41493c7d8ac6970a638746ece127720"
}

// MySQL 8.0.36 SHOW CREATE rendering is closed over the observed five schemas
// for these three collations. The two non-default collations explicitly repeat
// the table collation on each fixed text column. Different column overrides,
// extra keys/checks, normalization and material-provided SQL remain rejected.
func originalDDL(i int, collation string) string {
	s := scope[i]
	lines := make([]string, 0, len(s.columns)+3)
	for _, c := range s.columns {
		line := "  `" + c.Name + "` " + c.Type
		if (collation == "utf8mb4_general_ci" || collation == "utf8mb4_unicode_ci") && (c.Type == "longtext" || strings.HasPrefix(c.Type, "char(") || strings.HasPrefix(c.Type, "varchar(")) {
			line += " COLLATE " + collation
		}
		if c.Nullable {
			line += " DEFAULT NULL"
		} else {
			line += " NOT NULL"
		}
		if i == 2 && (c.Name == "after_hash" || c.Name == "rollback_hash") {
			line += " DEFAULT ''"
		}
		lines = append(lines, line)
	}
	var keys []string
	for _, key := range s.keys {
		keys = append(keys, "`"+key+"`")
	}
	lines = append(lines, "  PRIMARY KEY ("+strings.Join(keys, ",")+")")
	tail := ""
	if i == 3 {
		lines = append(lines, "  KEY `idx_clinician_binding_archive_migration` (`migration_id`)", "  CONSTRAINT `clinician_operator_binding_archive_chk_1` CHECK (json_valid(`before_record`))")
		tail = " COMMENT='Restricted historical clinician backend binding archive'"
	}
	if i == 4 {
		lines = append(lines, "  CONSTRAINT `operator_retirement_manifests_chk_1` CHECK (json_valid(`payload`))")
		tail = " COMMENT='Operator retirement immutable baseline and resumable execution record'"
	}
	return "CREATE TABLE `" + s.name + "` (\n" + strings.Join(lines, ",\n") + "\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=" + collation + tail
}

func qualifyOriginalDDL(i int, ddl string) error {
	if i < 0 || i >= len(scope) || !originalResourceBytesValid() {
		return errors.New("original_075_078_resource_rejected")
	}
	for _, collation := range originalCollations {
		if ddl == originalDDL(i, collation) {
			return nil
		}
	}
	return errors.New("original_075_078_table_definition_rejected")
}
