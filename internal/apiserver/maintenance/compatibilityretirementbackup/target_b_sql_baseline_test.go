package compatibilityretirementbackup

import (
	"strings"
	"testing"
)

func TestBRecoverySQLBaselineRetainsCapturedProjection(t *testing.T) {
	str := func(value string) *string { return &value }
	empty := [][]*string{}
	defs := map[string]any{
		"table:ai_bridge_commands": "original target",
		"table:ai_bridge_requests": "kept parent",
		"table:rm_outbox":          "kept message ledger",
		"constraints": [][]*string{
			{str("ai_bridge_commands"), str("fk_request"), str("owner"), str("ai_bridge_requests")},
			{str("kept_child"), str("fk_kept"), str("owner"), str("kept_parent")},
		},
		"inbound_constraints": empty,
		"triggers":            empty,
		"routines":            empty,
		"events":              empty,
	}
	recoveryHash, e := targetSQLNonTarget(defs, "owner")
	if e != nil {
		t.Fatal(e)
	}
	inventorySchema := map[string]any{}
	for key, value := range defs {
		if !strings.HasPrefix(key, "table:") || !targetSQLName(strings.TrimPrefix(key, "table:")) {
			inventorySchema[key] = value
		}
	}
	inventoryHash := jsonSHA(inventorySchema)
	if recoveryHash == inventoryHash {
		t.Fatal("owned foreign key must distinguish the two projections")
	}
	a := &Archive{data: manifest{SQLRecoveryNonTargetHash: recoveryHash, Inventory: inventory{Bindings: map[string]Binding{"mysql": {NonTargetHash: inventoryHash}}}}}
	r := TargetRecoveryRequest{SQLNonTargetSHA256: recoveryHash}
	if e = targetBArchiveSQLRecoveryBaseline(a, r); e != nil {
		t.Fatal("captured recovery projection rejected", e)
	}
	for _, value := range []string{"", "not-a-digest", strings.Repeat("f", 64), inventoryHash} {
		changed := &Archive{data: a.data}
		changed.data.SQLRecoveryNonTargetHash = value
		if e = targetBArchiveSQLRecoveryBaseline(changed, r); e != ErrRecoveryBinding {
			t.Fatal("missing, malformed or changed captured projection accepted")
		}
	}
	r.SQLNonTargetSHA256 = inventoryHash
	if e = targetBArchiveSQLRecoveryBaseline(a, r); e != ErrRecoveryBinding {
		t.Fatal("caller replaced captured recovery projection")
	}
	if e = targetBArchiveSQLRecoveryBaseline(nil, TargetRecoveryRequest{}); e != ErrRecoveryBinding {
		t.Fatal("no captured archive accepted")
	}
}
