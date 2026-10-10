package retirement

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func aiDiscoveryFixture(t *testing.T) ([]byte, aiExternalDiscoveryFacts) {
	t.Helper()
	facts := aiExternalDiscoveryFacts{Protocol: "qs-ai-readonly-bounds-discovery-facts/v1", SourceSHA: strings.Repeat("a", 40), OperationID: "123-1", RunID: "123", RuntimeSourceSHA: strings.Repeat("b", 40), RuntimeBindingSHA: strings.Repeat("c", 64), ImageID: "sha256:" + strings.Repeat("d", 64), ContainerID: strings.Repeat("e", 64), Bounds: map[string]aiExternalDiscoveredPacket{}, Sections: map[string]aiExternalSection{AIBridgeCommandSource: {1, 128, strings.Repeat("a", 64)}, AILegacyCommandSource: {0, 0, strings.Repeat("b", 64)}}, LogicalObjects: 53, Epochs: 2, Scope: "diagnostic-unapproved-bounds-only"}
	for _, side := range []string{"ai", "peer"} {
		count, head, source, identity := 44, "0040_module_table_names", "82ffa1b43308f23fbb1ebe669c3071e0486e105a", strings.Repeat("f", 64)
		if side == "peer" {
			count, head, source, identity = 14, "99", facts.SourceSHA, strings.Repeat("0", 64)
		}
		tables, after := map[string]any{}, map[string]bool{}
		for i := 0; i < count; i++ {
			name := fmt.Sprintf("synthetic_%02d", i)
			tables[name] = map[string]any{"upper": nil}
			after[name] = false
		}
		b := map[string]any{"protocol": "qs-ai-full-ledger-bounds/v1", "side": side, "source_sha": source, "identity_hash": identity, "head": head, "catalog_sha256": strings.Repeat("1", 64), "tables": tables, "profile": []int64{1000, 1000000, 2 << 30, 256 << 20, 32 << 20, 30, 1500}}
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		facts.Bounds[side] = aiExternalDiscoveredPacket{base64.StdEncoding.EncodeToString(raw), sourceSHA(raw), count, side == "peer", after}
	}
	raw, err := json.Marshal(facts)
	if err != nil {
		t.Fatal(err)
	}
	return raw, facts
}

func TestAIExternalBoundsObservationDoesNotAcquireExecutionOrCASAuthority(t *testing.T) {
	directory, err := filepath.Abs("../../../../scripts/database")
	if err != nil {
		t.Fatal(err)
	}
	assets, err := aiExternalAssets(directory)
	if err != nil || sourceSHA(assets.host) != aiExternalHostSHA || !strings.Contains(aiStoppedCarrierHost, aiExternalHostSHA) {
		t.Fatal("actual host asset and stopped carrier pins differ")
	}
	raw, _ := aiDiscoveryFixture(t)
	facts, ai, peer, err := aiExternalDecodeDiscovery(raw)
	if err != nil {
		t.Fatal(err)
	}
	o := &AIExternalBoundsObservation{facts: facts, ai: ai, peer: peer}
	o.self = o
	o.seal = aiJSONHash(facts)
	s := o.Summary()
	if s.Scope != "diagnostic-unapproved-bounds-only" || s.AIPhysicalObjects != 44 || s.AILogicalObjects != 53 || s.PeerObjects != 14 || s.IndependentEpochs != 2 || s.PriorAIBindingMatched || s.IndependentApproval || s.BusinessClosure || s.WriterFence || s.BrokerCoverage || s.CASAuthority || s.RetirementWritten || s.RecoveryAuthority || s.DropReady {
		t.Fatal("discovery scope/authority changed")
	}
	if _, err = json.Marshal(o); !errors.Is(err, ErrSourceSerialization) {
		t.Fatal("private PK packet serialized")
	}
	first, second, err := o.PrivatePackets()
	if err != nil {
		t.Fatal(err)
	}
	first[0] = '!'
	second[0] = '!'
	if !o.valid() || o.ai[0] != '{' || o.peer[0] != '{' {
		t.Fatal("returned packet mutation corrupted original")
	}
	clone := *o
	if _, _, err = clone.PrivatePackets(); err != ErrAIExternalBounds {
		t.Fatal("copied diagnostic object accepted")
	}
}

func TestAIExternalBoundsRejectsCapabilityFieldsAndUnboundPacketChanges(t *testing.T) {
	_, original := aiDiscoveryFixture(t)
	for _, which := range []string{"approval", "business", "fence", "cas", "drop", "packet_bytes", "packet_hash", "after_upper_set", "identity", "head", "catalog", "profile", "physical_missing_head", "physical_extra_object", "logical", "epochs", "unknown_field", "duplicate", "missing_flag", "null_flag"} {
		t.Run(which, func(t *testing.T) {
			_, facts := aiDiscoveryFixture(t)
			p := facts.Bounds["ai"]
			switch which {
			case "approval":
				facts.IndependentApproval = true
			case "business":
				facts.BusinessClosure = true
			case "fence":
				facts.Fence = true
			case "cas":
				facts.CASAuthority = true
			case "drop":
				facts.DropReady = true
			case "packet_bytes":
				p.Bytes = base64.StdEncoding.EncodeToString([]byte("{}"))
				facts.Bounds["ai"] = p
			case "packet_hash":
				p.SHA = strings.Repeat("0", 64)
				facts.Bounds["ai"] = p
			case "after_upper_set":
				delete(p.AfterUpper, "synthetic_00")
				p.AfterUpper["different"] = false
				facts.Bounds["ai"] = p
			case "logical":
				facts.LogicalObjects = 52
			case "epochs":
				facts.Epochs = 1
			default:
				if which == "identity" || which == "head" || which == "catalog" || which == "profile" || which == "physical_missing_head" || which == "physical_extra_object" {
					raw, e := base64.StdEncoding.DecodeString(p.Bytes)
					if e != nil {
						t.Fatal(e)
					}
					var b map[string]any
					if e = json.Unmarshal(raw, &b); e != nil {
						t.Fatal(e)
					}
					switch which {
					case "identity":
						b["identity_hash"] = ""
					case "head":
						b["head"] = "0041_unknown"
					case "catalog":
						b["catalog_sha256"] = ""
					case "profile":
						b["profile"] = []int{1000}
					case "physical_missing_head":
						delete(b["tables"].(map[string]any), "synthetic_43")
						delete(p.AfterUpper, "synthetic_43")
						p.Objects = 43
					case "physical_extra_object":
						b["tables"].(map[string]any)["synthetic_44"] = map[string]any{"upper": nil}
						p.AfterUpper["synthetic_44"] = false
						p.Objects = 45
					}
					raw, e = json.Marshal(b)
					if e != nil {
						t.Fatal(e)
					}
					p.Bytes = base64.StdEncoding.EncodeToString(raw)
					p.SHA = sourceSHA(raw)
					facts.Bounds["ai"] = p
				}
			}
			raw, e := json.Marshal(facts)
			if e != nil {
				t.Fatal(e)
			}
			if which == "missing_flag" || which == "null_flag" {
				var changed map[string]any
				if e = json.Unmarshal(raw, &changed); e != nil {
					t.Fatal(e)
				}
				if which == "missing_flag" {
					delete(changed, "independent_approval")
				} else {
					changed["independent_approval"] = nil
				}
				raw, e = json.Marshal(changed)
				if e != nil {
					t.Fatal(e)
				}
			}
			if which == "unknown_field" {
				raw = append([]byte(`{"complete":true,`), raw[1:]...)
			}
			if which == "duplicate" {
				raw = append([]byte(`{"drop_ready":false,`), raw[1:]...)
			}
			if _, _, _, e = aiExternalDecodeDiscovery(raw); e != ErrAIExternalBounds {
				t.Fatal("altered producer output accepted")
			}
		})
	}
	if original.Bounds["ai"].PriorMatched {
		t.Fatal("fixture declared prior approval")
	}
}

func TestAIExternalDiscoveryPriorIdentityPairAndRuntimeBindingRequired(t *testing.T) {
	in := AIExternalBoundsInput{RuntimeSourceSHA: strings.Repeat("a", 40), ImageID: "sha256:" + strings.Repeat("b", 64), ContainerID: strings.Repeat("c", 64), RunID: "123", ApprovedAIRuntimeBindingSHA256: strings.Repeat("d", 64)}
	if !aiExternalDiscoveryInputValid(in) {
		t.Fatal("explicit unknown prior rejected")
	}
	in.ExpectedAIIdentityHash = strings.Repeat("e", 64)
	if aiExternalDiscoveryInputValid(in) {
		t.Fatal("half prior binding accepted")
	}
	in.ExpectedAIHead = "0040_module_table_names"
	if !aiExternalDiscoveryInputValid(in) {
		t.Fatal("complete prior binding rejected")
	}
	in.ApprovedAIRuntimeBindingSHA256 = ""
	if aiExternalDiscoveryInputValid(in) {
		t.Fatal("missing runtime binding accepted")
	}
	var c *HistoricalCoordinator
	if q, err := c.ObserveAIExternalBounds(context.Background(), nil, in); q != nil || err != ErrAIExternalBounds {
		t.Fatal("missing actual owner produced observation")
	}
}

func TestAIExternalDiscoveryNextCycleHintDoesNotChangeAuthority(t *testing.T) {
	raw, _ := aiDiscoveryFixture(t)
	facts, ai, peer, err := aiExternalDecodeDiscovery(raw)
	if err != nil {
		t.Fatal(err)
	}
	facts.Bounds["ai"].AfterUpper["synthetic_00"] = true
	o := &AIExternalBoundsObservation{facts: facts, ai: ai, peer: peer}
	o.self = o
	o.seal = aiJSONHash(facts)
	s := o.Summary()
	if !s.NextCycleRequired || s.IndependentApproval || s.DropReady || s.CASAuthority || s.BusinessClosure {
		t.Fatal("new rows became approval")
	}
}
