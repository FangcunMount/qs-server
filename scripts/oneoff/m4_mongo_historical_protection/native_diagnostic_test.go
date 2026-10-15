//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
)

type nativeIndexEvent struct {
	Kind        string           `json:"witness_kind"`
	Observation indexObservation `json:"observation"`
}

func (h *nativeFixture) logIndexEvents(start int) {
	if rows := h.indexEventsFrom(start); len(rows) > 0 {
		raw, _ := json.Marshal(rows)
		h.t.Logf("test-only fixed createIndexes witness: %s", raw)
	}
}

func (h *nativeFixture) recordIndex(kind string, o indexObservation) {
	h.witnessMu.Lock()
	defer h.witnessMu.Unlock()
	h.indexEvents = append(h.indexEvents, nativeIndexEvent{Kind: kind, Observation: o})
}
func (h *nativeFixture) indexEventsFrom(start int) []nativeIndexEvent {
	h.witnessMu.Lock()
	defer h.witnessMu.Unlock()
	return append([]nativeIndexEvent(nil), h.indexEvents[start:]...)
}
func (h *nativeFixture) indexEventCount() int {
	h.witnessMu.Lock()
	defer h.witnessMu.Unlock()
	return len(h.indexEvents)
}
func (h *nativeFixture) hasTypedIndexCode(code int32) bool {
	for _, e := range h.indexEventsFrom(0) {
		if e.Kind == "typed_result" && e.Observation.Class == "server_error" && e.Observation.Code != nil && *e.Observation.Code == code {
			return true
		}
	}
	return false
}
func (h *nativeFixture) observeIndexes() *indexObserver {
	return &indexObserver{monitor: &event.CommandMonitor{
		Succeeded: func(_ context.Context, e *event.CommandSucceededEvent) {
			if e.CommandName == "createIndexes" {
				h.recordIndex("monitor_success", indexObservation{Operation: "createIndexes", Class: "driver_command_success", OK: responseOK(e.Reply)})
			}
		},
		Failed: func(_ context.Context, e *event.CommandFailedEvent) {
			if e.CommandName == "createIndexes" {
				// v1.17.6 exposes Failure as a string. Do not inspect, parse or emit it.
				h.recordIndex("monitor_failed", indexObservation{Operation: "createIndexes", Class: "driver_command_failed"})
			}
		},
	}, completed: func(o indexObservation) { h.recordIndex("typed_result", o) }}
}

type nativeFieldDifference struct {
	Key        string `json:"key"`
	BeforeType string `json:"before_type"`
	AfterType  string `json:"after_type"`
}
type nativeDocumentDifference struct {
	Collection string                  `json:"collection"`
	Kind       string                  `json:"difference_kind"`
	IDHash     string                  `json:"id_sha256,omitempty"`
	BeforeHash string                  `json:"raw_before_sha256,omitempty"`
	AfterHash  string                  `json:"raw_after_sha256,omitempty"`
	Fields     []nativeFieldDifference `json:"field_key_type_differences,omitempty"`
}

func nativeIDHash(raw bson.Raw) string {
	v := raw.Lookup("_id")
	return fingerprint(append([]byte{byte(v.Type)}, v.Value...))
}
func nativeFieldDiff(before, after bson.Raw) []nativeFieldDifference {
	a, b := map[string]bson.RawValue{}, map[string]bson.RawValue{}
	for _, item := range []struct {
		raw    bson.Raw
		target map[string]bson.RawValue
	}{{before, a}, {after, b}} {
		elements, _ := item.raw.Elements()
		for _, e := range elements {
			item.target[e.Key()] = e.Value()
		}
	}
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	ordered := make([]string, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)
	var result []nativeFieldDifference
	for _, k := range ordered {
		av, aok := a[k]
		bv, bok := b[k]
		if aok == bok && av.Type == bv.Type && bytes.Equal(av.Value, bv.Value) {
			continue
		}
		at, bt := "absent", "absent"
		if aok {
			at = av.Type.String()
		}
		if bok {
			bt = bv.Type.String()
		}
		if len(k) > 128 {
			k = "key_sha256:" + fingerprint([]byte(k))
		}
		result = append(result, nativeFieldDifference{Key: k, BeforeType: at, AfterType: bt})
		if len(result) >= 16 {
			break
		}
	}
	return result
}
func nativeDocumentDiff(before, after map[string][]bson.Raw) []nativeDocumentDifference {
	names := map[string]bool{}
	for n := range before {
		names[n] = true
	}
	for n := range after {
		names[n] = true
	}
	ordered := make([]string, 0, len(names))
	for n := range names {
		ordered = append(ordered, n)
	}
	sort.Strings(ordered)
	var result []nativeDocumentDifference
	for _, name := range ordered {
		a, aok := before[name]
		b, bok := after[name]
		if aok != bok {
			kind := "collection_removed"
			if bok {
				kind = "collection_added"
			}
			result = append(result, nativeDocumentDifference{Collection: name, Kind: kind})
			if len(result) >= 8 {
				return result
			}
			continue
		}
		ad, bd := map[string]bson.Raw{}, map[string]bson.Raw{}
		for _, r := range a {
			ad[nativeIDHash(r)] = r
		}
		for _, r := range b {
			bd[nativeIDHash(r)] = r
		}
		ids := map[string]bool{}
		for id := range ad {
			ids[id] = true
		}
		for id := range bd {
			ids[id] = true
		}
		orderedIDs := make([]string, 0, len(ids))
		for id := range ids {
			orderedIDs = append(orderedIDs, id)
		}
		sort.Strings(orderedIDs)
		for _, id := range orderedIDs {
			av, aok := ad[id]
			bv, bok := bd[id]
			if aok == bok && bytes.Equal(av, bv) {
				continue
			}
			o := nativeDocumentDifference{Collection: name, Kind: "document_bytes", IDHash: id, Fields: nativeFieldDiff(av, bv)}
			if aok {
				o.BeforeHash = fingerprint(av)
			}
			if bok {
				o.AfterHash = fingerprint(bv)
			}
			result = append(result, o)
			if len(result) >= 8 {
				return result
			}
		}
	}
	return result
}
func TestNativeDiagnosticWitnessNeverPrintsErrorOrBSONValues(t *testing.T) {
	secret := "synthetic-sensitive-credentials-spec-and-business-values"
	h := &nativeFixture{}
	o := h.observeIndexes()
	o.monitor.Failed(context.Background(), &event.CommandFailedEvent{CommandFinishedEvent: event.CommandFinishedEvent{CommandName: "createIndexes"}, Failure: secret})
	o.completed(observeIndexResult(mongo.CommandError{Code: 11000, Name: "DuplicateKey", Message: secret}))
	raw, _ := json.Marshal(h.indexEventsFrom(0))
	if strings.Contains(string(raw), secret) || !h.hasTypedIndexCode(11000) || h.hasTypedIndexCode(9) {
		t.Fatal("index witness leaked raw failure or lost typed code")
	}
	before := map[string][]bson.Raw{"answersheets": {rawBSON(t, bson.D{{Key: "_id", Value: secret}, {Key: "payload", Value: secret}})}}
	after := map[string][]bson.Raw{"answersheets": {rawBSON(t, bson.D{{Key: "_id", Value: secret}, {Key: "payload", Value: int64(99)}})}, "retired_empty_fixture": nil}
	diff := nativeDocumentDiff(before, after)
	raw, _ = json.Marshal(diff)
	if len(diff) != 2 || strings.Contains(string(raw), secret) || !strings.Contains(string(raw), "id_sha256") || !strings.Contains(string(raw), "collection_added") || !strings.Contains(string(raw), "before_type") {
		t.Fatal("BSON witness leaked values or failed to locate structural delta")
	}
}
