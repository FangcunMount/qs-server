package retirement

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// These are private graph algorithm fixtures, not native qualifications. The
// exported freeze factory remains uncallable without real live owner inputs.
func componentFixture(t *testing.T, n int) (*WholeSourceJointIndex, []*HistoricalCASComponentInput) {
	t.Helper()
	index := &WholeSourceJointIndex{indexSHA: "actual-index-fixture", complete: true, owner: &HistoricalCoordinator{}, auth: &VerifiedSourceCopies{complete: true}, entries: map[string]wholeSourceJointEntry{}}
	frames := make([]*HistoricalCASComponentInput, n)
	for i := range frames {
		key := verifiedSourceKey{object: 0}
		key.pk[0] = byte(i + 1)
		id := uint64(i + 1)
		index.entries[string(rune('a'+i))] = wholeSourceJointEntry{Key: key}
		f := &HistoricalCASComponentInput{index: index, sequence: id, sources: []verifiedSourceKey{key}, owners: []historicalCASOwnerKey{{"assessment", id, 7}}, rows: []historicalCASRowInput{{historicalCASRowKey{"mysql", "assessment", id}, "physical-row", 100, true}}}
		f.self, f.seal = f, f.digest()
		frames[i] = f
	}
	return index, frames
}
func resealComponentFixture(f *HistoricalCASComponentInput) { f.seal = f.digest() }
func componentSequences(c *HistoricalCASComponents) [][]uint64 {
	var result [][]uint64
	for _, component := range c.components {
		var ids []uint64
		for _, frame := range component.inputs {
			ids = append(ids, frame.sequence)
		}
		result = append(result, ids)
	}
	return result
}

func TestHistoricalCASComponentsWriteReadClosure(t *testing.T) {
	index, frames := componentFixture(t, 3)
	// Page3 writes a physical BSON owner that page1 really read. Transitive
	// SQL write/read then closes page2, even though all have distinct owners.
	raw, err := bson.Marshal(bson.D{{Key: "domain_id", Value: int64(42)}, {Key: "name", Value: "original"}})
	if err != nil {
		t.Fatal(err)
	}
	id, err := historicalSpoolMongoID(raw)
	if err != nil {
		t.Fatal(err)
	}
	row := historicalCASRowInput{historicalCASRowKey{"mongodb", "answersheets", id}, historicalSpoolSHA(raw), uint64(len(raw)), false}
	frames[0].rows = append(frames[0].rows, row)
	row.write = true
	frames[2].rows = append(frames[2].rows, row)
	dep := frames[0].rows[0]
	dep.write = false
	frames[1].rows = append(frames[1].rows, dep)
	for _, f := range frames {
		resealComponentFixture(f)
	}
	result, err := PrepareHistoricalCASComponents(context.Background(), index, []*HistoricalCASComponentInput{frames[2], frames[0], frames[1]}, DefaultHistoricalCASComponentLimits())
	if err != nil || !reflect.DeepEqual(componentSequences(result), [][]uint64{{1, 2, 3}}) {
		t.Fatalf("physical closure: %v %v", result, err)
	}
}

func TestHistoricalCASComponentsSameOwnerAcrossPagesAndReadonlySharedRow(t *testing.T) {
	index, frames := componentFixture(t, 3)
	frames[1].owners = append(frames[1].owners, frames[0].owners[0])
	// The same unmodified row is read by every frame; it must not link page3.
	readonly := historicalCASRowInput{historicalCASRowKey{"mongodb", "report_generations", 99}, "unchanged", 20, false}
	for _, f := range frames {
		f.rows = append(f.rows, readonly)
		resealComponentFixture(f)
	}
	result, err := PrepareHistoricalCASComponents(context.Background(), index, frames, DefaultHistoricalCASComponentLimits())
	if err != nil || !reflect.DeepEqual(componentSequences(result), [][]uint64{{1, 2}, {3}}) {
		t.Fatalf("owner/readonly closure: %v %v", result, err)
	}
	copy := result.Components()
	copy[0] = nil
	if result.components[0] == nil {
		t.Fatal("mutable component list exposed")
	}
	inputs := result.components[0].Inputs()
	inputs[0] = nil
	if result.components[0].inputs[0] == nil {
		t.Fatal("mutable input list exposed")
	}
}

func TestHistoricalCASComponentsAllBudgetsRejectBeforeBatches(t *testing.T) {
	for _, field := range []string{"frames", "sources", "owners", "rows", "bytes"} {
		t.Run(field, func(t *testing.T) {
			index, frames := componentFixture(t, 3)
			frames[2].owners = append(frames[2].owners, frames[1].owners[0])
			resealComponentFixture(frames[2])
			limits := DefaultHistoricalCASComponentLimits()
			switch field {
			case "frames":
				limits.MaxFrames = 1
			case "sources":
				limits.MaxSources = 1
			case "owners":
				limits.MaxOwners = 1
			case "rows":
				limits.MaxRows = 1
			case "bytes":
				limits.MaxBytes = 100
			}
			result, err := PrepareHistoricalCASComponents(context.Background(), index, frames, limits)
			if result != nil || !errors.Is(err, ErrWholeSourceJointBounds) {
				t.Fatalf("%s returned partial batches: %v %v", field, result, err)
			}
		})
	}
}

func TestHistoricalCASComponentsRejectUnclosedOrChangedInputs(t *testing.T) {
	for _, name := range []string{"coverage", "duplicate-source", "duplicate-page", "changed-row", "changed-seal", "unknown-namespace", "cross-org"} {
		t.Run(name, func(t *testing.T) {
			index, frames := componentFixture(t, 2)
			switch name {
			case "coverage":
				frames = frames[:1]
			case "duplicate-source":
				frames[1].sources = frames[0].sources
			case "duplicate-page":
				frames[1].sequence = frames[0].sequence
			case "changed-row":
				row := frames[0].rows[0]
				row.sha = "changed"
				frames[1].rows = append(frames[1].rows, row)
			case "changed-seal":
				frames[0].rows[0].bytes++
			case "unknown-namespace":
				frames[0].rows[0].key.name = "unregistered_table"
			case "cross-org":
				frames[1].owners = []historicalCASOwnerKey{{"assessment", 1, 8}}
			}
			if name != "changed-seal" {
				for _, f := range frames {
					resealComponentFixture(f)
				}
			}
			result, err := PrepareHistoricalCASComponents(context.Background(), index, frames, DefaultHistoricalCASComponentLimits())
			if result != nil || err == nil {
				t.Fatalf("%s admitted: %v", name, result)
			}
		})
	}
}

func TestHistoricalCASComponentInputCannotImportOrFreezeDetachedDTO(t *testing.T) {
	var f HistoricalCASComponentInput
	if json.Unmarshal([]byte(`{}`), &f) == nil {
		t.Fatal("serialized input admitted")
	}
	if _, err := FreezeHistoricalCASComponentInput(context.Background(), nil, nil, nil, nil); err == nil {
		t.Fatal("detached capture admitted")
	}
	index, frames := componentFixture(t, 1)
	copy := *frames[0]
	if result, err := PrepareHistoricalCASComponents(context.Background(), index, []*HistoricalCASComponentInput{&copy}, DefaultHistoricalCASComponentLimits()); err == nil || result != nil {
		t.Fatal("copied input admitted")
	}
}
