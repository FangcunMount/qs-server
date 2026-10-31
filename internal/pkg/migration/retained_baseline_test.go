package migration

import (
	"database/sql"
	"testing"
)

func TestRetainedDigestPreservesNullEmptyBoundariesAndMultiplicity(t *testing.T) {
	values := [][]sql.RawBytes{{nil}, {sql.RawBytes{}}, {sql.RawBytes("a"), sql.RawBytes("bc")}, {sql.RawBytes("ab"), sql.RawBytes("c")}}
	seen := map[[32]byte]bool{}
	for _, v := range values {
		h := digestSQLRow(v)
		if seen[h] {
			t.Fatal("typed value boundary lost")
		}
		seen[h] = true
	}
	a, b := digestSQLRow(values[0]), digestSQLRow(values[1])
	if sortedDigestRows([][32]byte{a, b}) != sortedDigestRows([][32]byte{b, a}) {
		t.Fatal("server row ordering affected full content hash")
	}
	if sortedDigestRows([][32]byte{a, b}) == sortedDigestRows([][32]byte{a, b, b}) {
		t.Fatal("duplicate row multiplicity lost")
	}
	if quotedSQLIdentifier("a`b") != "`a``b`" {
		t.Fatal("identifier not escaped")
	}
}
