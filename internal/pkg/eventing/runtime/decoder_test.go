package eventruntime

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
)

func TestSDKDomainDecoderPreservesHistoricalReplayContract(t *testing.T) {
	_, decoders := retiredWireContracts(t)
	for name, previous := range decoders {
		t.Run(name, func(t *testing.T) {
			current, currentErr := DecodeDomainEvent([]byte(previous.Input))
			if previous.Error != "" {
				// encoding/json reports the Go parser type for a non-object
				// envelope. Only its package name changes with SDK ownership.
				want := strings.ReplaceAll(previous.Error, "eventcodec.Envelope", "domain.Envelope")
				if currentErr == nil || currentErr.Error() != want {
					t.Fatalf("decode error changed: previous=%v current=%v", previous.Error, currentErr)
				}
				return
			}
			if currentErr != nil {
				t.Fatal(currentErr)
			}
			before := []byte(previous.Value)
			after, err := json.Marshal(current)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatalf("replay payload changed: previous=%s current=%s", before, after)
			}
			if !reflect.DeepEqual(previous.Metadata, domainwire.MetadataFromEvent(current, SourceAPIServer)) {
				t.Fatal("original identity or metadata changed")
			}
		})
	}
}
