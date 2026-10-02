package eventruntime

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/FangcunMount/component-base/pkg/eventcodec"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
)

func TestSDKDomainDecoderPreservesHistoricalReplayContract(t *testing.T) {
	for name, payload := range map[string]string{
		"precise identifiers and UTC+8": `{"id":"original-id","eventType":"evaluation.retry.requested","occurredAt":"2026-10-02T10:01:02.123456789+08:00","aggregateType":"Evaluation","aggregateID":"639678084915671598","data":{"org_id":639678084915671598,"nested":[true,null,"中文"]}}`,
		"unknown fields":                `{"id":"original-id","eventType":"future.event","extra":"historically ignored","data":{"new_field":"kept"}}`,
		"null data":                     `{"id":"original-id","data":null}`,
		"missing data":                  `{"id":"original-id"}`,
		"empty envelope":                `{}`,
		"null envelope":                 `null`,
		"invalid JSON":                  `{not-json`,
		"invalid date":                  `{"occurredAt":"not-a-date"}`,
		"wrong field type":              `{"id":123}`,
		"array envelope":                `[]`,
		"empty payload":                 ``,
	} {
		t.Run(name, func(t *testing.T) {
			previous, previousErr := eventcodec.DecodeDomainEvent([]byte(payload))
			current, currentErr := DecodeDomainEvent([]byte(payload))
			if previousErr != nil {
				// encoding/json reports the Go parser type for a non-object
				// envelope. Only its package name changes with SDK ownership.
				want := strings.ReplaceAll(previousErr.Error(), "eventcodec.Envelope", "domain.Envelope")
				if currentErr == nil || currentErr.Error() != want {
					t.Fatalf("decode error changed: previous=%v current=%v", previousErr, currentErr)
				}
				return
			}
			if currentErr != nil {
				t.Fatal(currentErr)
			}
			before, err := json.Marshal(previous)
			if err != nil {
				t.Fatal(err)
			}
			after, err := json.Marshal(current)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatalf("replay payload changed: previous=%s current=%s", before, after)
			}
			if !reflect.DeepEqual(eventcodec.MetadataFromEvent(previous, SourceAPIServer), domainwire.MetadataFromEvent(current, SourceAPIServer)) {
				t.Fatal("original identity or metadata changed")
			}
		})
	}
}
