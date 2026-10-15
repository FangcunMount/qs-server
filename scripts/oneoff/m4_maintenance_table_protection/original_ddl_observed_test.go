package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Independent complete SHOW CREATE bytes from the owned MySQL8.0.36 diagnostic:
// compiled source731af233e15461c11f29c46e7d0fb13ecdf7f629, JSONL SHA256
// 94c6add8b90538541591169685db133a936f988ea714a129930ce97cd2f84922.
// Its Go reader passed; the wrapper failed parsing a split log line. These are
// synthetic observed shapes, never production observations or restore proof.
var observedOriginalShapeHashes = [3][5]string{
	{"eead8303607db8ba5fd9347161d17a25b4e0c6137a0ae57cf5b358d6d5e3a3ff", "74c7188a27cff71a7b45993321d9fd04af7d408a46fc5db8516c87292a857e62", "2e1a8cf2e887941c92125174f53500a213e55a193bcfcbcf6f8e24480d8049e1", "19bc7c83f117b1ad5773c6e725cae05b3900a680ae8a0d996811bbeff9ad2f61", "d9fce461343eb1204918620fbc05a8c87ef235e70d644c3c7f8df047326243c2"},
	{"15d5ba72a852b81fbe5a997844e9561eea73139e3d199902e613a4e5af7a091c", "61688811fed976a6f7b0c2a9f516d05c9e00640f6e0bda9091bcfdc071838eec", "ad595b86bf1b85b53fbc2e913c70ffe72de0909bc9f63e16eaa978ca545ec141", "d63b6e782ef22fdc95fb04d811080378c9e6222d2311ac133f5aab8954be53db", "c56b9778675b40d1d080cad72f9de7fe0557a9f9801817c9c31f45422fbab4f3"},
	{"df42bfdbd3232cb0a707322e6e4e69d98024d8414b317b8dfaeb7f9e4059b4c9", "6b62d8a7e6d56ea1e936bc5a97e2ff5d33ea76e15949cc53956c5a0c15576e15", "07d68ee6fa25f7bc4c721640853ef7e3add4f5d28b56253966406bb655482020", "f54f1a5fd8da60a724a28eba8f405fdcc0068de0e2dd8dd6adbcb409ed712d10", "a38c929b73de17a420dac26add0acb6ff8a325d5137e796b1ad35bcae942a2ba"},
}

func observedOriginalShape(t *testing.T, collationIndex, tableIndex int) string {
	t.Helper()
	name := originalCollations[collationIndex] + "-0" + strconv.Itoa(tableIndex) + ".sql"
	raw, e := os.ReadFile(filepath.Join("testdata", "original_shapes", name))
	if e != nil || fingerprint(raw) != observedOriginalShapeHashes[collationIndex][tableIndex] {
		t.Fatal("independent observed original shape bytes changed")
	}
	return string(raw)
}

func TestOriginalDDLMatchesIndependentObservedMySQL8036Shapes(t *testing.T) {
	for c, collation := range originalCollations {
		for i, s := range scope {
			t.Run(collation+"/"+s.name, func(t *testing.T) {
				actual := observedOriginalShape(t, c, i)
				if originalDDL(i, collation) != actual || qualifyOriginalDDL(i, actual) != nil {
					t.Fatal("closed template differs from independent full observed SHOW CREATE")
				}
			})
		}
	}
}

func TestOriginalDDLRejectsUnobservedColumnCollationShapes(t *testing.T) {
	for c, collation := range originalCollations {
		for i, s := range scope {
			t.Run(collation+"/"+s.name, func(t *testing.T) {
				original := observedOriginalShape(t, c, i)
				mutations := map[string]string{
					"unknown_table_collation":   strings.ReplaceAll(original, "COLLATE="+collation, "COLLATE=utf8mb4_bin"),
					"unknown_column_collation":  strings.Replace(original, "varchar(64)", "varchar(64) COLLATE utf8mb4_bin", 1),
					"explicit_charset_override": strings.Replace(original, "varchar(64)", "varchar(64) CHARACTER SET utf8mb4", 1),
				}
				if c == 0 {
					mutations["unobserved_default_column_clause"] = strings.Replace(original, "varchar(64)", "varchar(64) COLLATE "+collation, 1)
				} else {
					mutations["unknown_column_collation"] = strings.Replace(original, " COLLATE "+collation, " COLLATE utf8mb4_bin", 1)
					mutations["missing_required_column_clause"] = strings.Replace(original, " COLLATE "+collation, "", 1)
					mutations["mixed_known_column_collation"] = strings.Replace(original, " COLLATE "+collation, " COLLATE "+originalCollations[0], 1)
				}
				for name, changed := range mutations {
					if changed == original || qualifyOriginalDDL(i, changed) == nil {
						t.Fatal("unobserved column shape accepted", name)
					}
				}
			})
		}
	}
}
