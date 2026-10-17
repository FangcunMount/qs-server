package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestClearanceOnlyIgnoresBareTableAutoIncrementCounter(t *testing.T) {
	ddl := "CREATE TABLE `t` (\n  `id` bigint NOT NULL AUTO_INCREMENT,\n  `note` varchar(80) DEFAULT 'AUTO_INCREMENT=41' COMMENT 'AUTO_INCREMENT=42',\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB AUTO_INCREMENT=43 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='AUTO_INCREMENT=44'"
	base := encode([]string{ddl, "+00:00", "utf8mb4", "STRICT_TRANS_TABLES"})
	counter := encode([]string{strings.Replace(ddl, "InnoDB AUTO_INCREMENT=43", "InnoDB AUTO_INCREMENT=999", 1), "+00:00", "utf8mb4", "STRICT_TRANS_TABLES"})
	ctx := graphClearanceNamespaceContext(t.Context())
	if !graphClearanceSameNamespace(ctx, "mysql", base, counter) || graphClearanceSameNamespace(t.Context(), "mysql", base, counter) || graphClearanceSameNamespace(ctx, "mongo", base, counter) {
		t.Fatal("counter comparison escaped known-ACK SQL clearance")
	}
	withoutCounter := encode([]string{strings.Replace(ddl, " AUTO_INCREMENT=43", "", 1), "+00:00", "utf8mb4", "STRICT_TRANS_TABLES"})
	if !graphClearanceSameNamespace(ctx, "mysql", base, withoutCounter) {
		t.Fatal("absent initial table counter is a schema identity")
	}
	for name, changed := range map[string]string{
		"column_attribute": strings.Replace(ddl, "NOT NULL AUTO_INCREMENT,", "NOT NULL,", 1),
		"column_default":   strings.Replace(ddl, "=41'", "=51'", 1),
		"column_comment":   strings.Replace(ddl, "=42'", "=52'", 1),
		"table_comment":    strings.Replace(ddl, "=44'", "=54'", 1),
		"collation":        strings.Replace(ddl, "utf8mb4_unicode_ci", "utf8mb4_bin", 1),
		"engine":           strings.Replace(ddl, "ENGINE=InnoDB", "ENGINE=MyISAM", 1),
		"index":            strings.Replace(ddl, "PRIMARY KEY", "UNIQUE KEY", 1),
		"unknown_option":   ddl + " /* AUTO_INCREMENT=55 */",
		"duplicate":        ddl + " AUTO_INCREMENT=55",
	} {
		t.Run(name, func(t *testing.T) {
			if graphClearanceSameNamespace(ctx, "mysql", base, encode([]string{changed, "+00:00", "utf8mb4", "STRICT_TRANS_TABLES"})) {
				t.Fatal("non-counter schema change was ignored")
			}
		})
	}
	for _, session := range [][]string{{ddl, "SYSTEM", "utf8mb4", "STRICT_TRANS_TABLES"}, {ddl, "+00:00", "latin1", "STRICT_TRANS_TABLES"}, {ddl, "+00:00", "utf8mb4", ""}} {
		if graphClearanceSameNamespace(ctx, "mysql", base, encode(session)) {
			t.Fatal("SQL session semantics changed")
		}
	}
	a := record{Object: object{Store: "mysql", Name: "t", Key: []cell{{Name: "id", Type: "uint64", Raw: []byte("1")}}}, Columns: []cell{{Name: "note", Raw: []byte("original")}}, Definition: base}
	b := a
	b.Definition = counter
	beforeA, beforeB := encode(a), encode(b)
	if !graphClearanceSameRecord(ctx, a, b) || sameRecord(a, b) {
		t.Fatal("readonly protection comparison did not preserve raw fingerprint semantics")
	}
	b.Columns = []cell{{Name: "note", Raw: []byte("changed")}}
	if graphClearanceSameRecord(ctx, a, b) {
		t.Fatal("protected row body was not compared")
	}
	b.Columns = a.Columns
	if !bytes.Equal(beforeA, encode(a)) || !bytes.Equal(beforeB, encode(b)) {
		t.Fatal("normalization rewrote original bytes")
	}
}
