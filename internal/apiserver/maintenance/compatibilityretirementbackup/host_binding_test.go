package compatibilityretirementbackup

import (
	"context"
	"strings"
	"testing"
)

func TestHostArchiveBindingsKeepInventoryAndRecoveryProjectionsSeparate(t *testing.T) {
	inventory, recovery, mongo := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	a := &Archive{data: manifest{SQLRecoveryNonTargetHash: recovery, Inventory: inventoryFixtureForHost(inventory, mongo)}}
	var expected [4]HostObjectBinding
	for i, s := range a.data.Inventory.Targets {
		expected[i] = HostObjectBinding{Database: s.Database, Name: s.Name, Kind: s.Kind, IdentityHash: s.IdentityHash, SchemaHash: s.SchemaHash, DataHash: s.DataHash, Records: s.Records}
	}
	if VerifyHostArchiveObjects(context.Background(), a, expected, 99, 38, inventory, recovery, mongo) != nil {
		t.Fatal("separate approved projections rejected")
	}
	if VerifyHostArchiveObjects(context.Background(), a, expected, 99, 38, inventory, inventory, mongo) == nil {
		t.Fatal("inventory projection adopted for recovery")
	}
	a.data.SQLRecoveryNonTargetHash = ""
	if VerifyHostArchiveObjects(context.Background(), a, expected, 99, 38, inventory, recovery, mongo) == nil {
		t.Fatal("old archive without projection adopted")
	}
	if VerifyHostArchiveObjects(context.Background(), new(Archive), expected, 99, 38, inventory, recovery, mongo) == nil {
		t.Fatal("zero archive accepted")
	}
}
func inventoryFixtureForHost(sqlHash, mongoHash string) inventory {
	r := inventory{Bindings: map[string]Binding{"mysql": {Version: 99, NonTargetHash: sqlHash}, "mongodb": {Version: 38, NonTargetHash: mongoHash}}}
	for i, name := range targetNames {
		database, kind := "mysql", "base_table"
		if i == 3 {
			database, kind = "mongodb", "collection"
		}
		r.Targets = append(r.Targets, SourceSnapshot{Database: database, Name: name, Kind: kind, IdentityHash: strings.Repeat("d", 64), SchemaHash: strings.Repeat("e", 64), DataHash: strings.Repeat("f", 64)})
	}
	return r
}
