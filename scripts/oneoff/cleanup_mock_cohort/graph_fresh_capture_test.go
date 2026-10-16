package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFreshFrozenCaptureKeepsCompleteScopeAndRestorableOriginals(t *testing.T) {
	s, g, p, index, m, scope, dir := graphEngineFixture(t)
	if err := graphArchiveCoverage(m, g, p, index, scope, dir, true); err != nil {
		t.Fatal(err)
	}
	// Consumers which need original values still expand and verify every value.
	if err := validateGraphCoverage(m, g, p, index, scope, dir); err != nil {
		t.Fatal(err)
	}
	for _, ref := range m.Targets {
		rows, err := readGraphArchivePage(dir, m, ref)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			delete(s.rows, objectID(row.Object))
		}
		if err = s.mutateExactBatch(t.Context(), rows, true); err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if !sameRecord(row, s.rows[objectID(row.Object)]) || row.Object.Fingerprint != fingerprint(row) {
				t.Fatal("fresh key coverage changed a restorable full original")
			}
		}
	}
}

func TestFreshFrozenCaptureCoverageRejectsOmissionDuplicateAndProtectionIntersection(t *testing.T) {
	for _, variant := range []string{"target_omitted", "target_duplicated", "guard_omitted", "rank", "definition", "protection_intersection"} {
		t.Run(variant, func(t *testing.T) {
			_, g, p, index, m, scope, dir := graphEngineFixture(t)
			m.Targets = append([]graphShard{}, m.Targets...)
			switch variant {
			case "target_omitted":
				m.Targets = m.Targets[:len(m.Targets)-1]
			case "target_duplicated":
				m.Targets[1] = m.Targets[0]
			case "guard_omitted":
				m.Guards = nil
			case "rank":
				m.Targets[0].RestoreRank++
			case "definition":
				index.Definitions[m.Targets[0].Store+"/"+m.Targets[0].Name] = strings.Repeat("f", 64)
			case "protection_intersection":
				rows, err := readGraphArchivePage(dir, m, m.Targets[0])
				if err != nil {
					t.Fatal(err)
				}
				p.QS.Other = append(p.QS.Other, rows[0].Object)
			}
			if err := graphArchiveCoverage(m, g, p, index, scope, dir, true); err == nil {
				t.Fatal("fresh explicit coverage admitted changed complete scope")
			}
			if err := validateGraphCoverage(m, g, p, index, scope, dir); err == nil {
				t.Fatal("default complete coverage was weakened")
			}
		})
	}
}

// This local allocation comparison measures the same immutable original bytes
// through both readers. It is not a production capture-duration benchmark.
func BenchmarkFreshCaptureOriginalPageRead(b *testing.B) {
	home, err := os.UserHomeDir()
	if err != nil {
		b.Fatal(err)
	}
	dir, err := os.MkdirTemp(home, ".qs-fresh-capture-bench-")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = os.RemoveAll(dir) })
	r := graphBatchRow("1", strings.Repeat("same original SQL value ", 32768))
	m := graphArchive{Namespaces: map[string][]byte{}, Counts: map[string]int64{}}
	if err = archiveGraphPage(dir, &m, []record{r}, graphSQLTables["assessment_score"], false); err != nil {
		b.Fatal(err)
	}
	ref := m.Targets[0]
	if _, err = os.Stat(filepath.Join(dir, ref.File)); err != nil {
		b.Fatal(err)
	}
	for _, full := range []bool{true, false} {
		name := "fresh_physical_keys"
		if full {
			name = "complete_original_values"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(ref.Bytes))
			for range b.N {
				var err error
				if full {
					_, err = readGraphArchivePage(dir, m, ref)
				} else {
					_, err = readGraphFrozenKeys(dir, m, ref)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
