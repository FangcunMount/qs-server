package messagingruntime

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime/testsupport"
	"github.com/FangcunMount/qs-server/internal/pkg/runtimefacts"
)

func TestWirePublisherFactsLoadedAddressesAndStop(t *testing.T) {
	owner, err := runtimefacts.New("apiserver", strings.Repeat("a", 40))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close() }()
	httpEndpoints := []string{"https://resolved-nsqd.internal:14451"}
	address, identities := testsupport.NSQPublisherFixture(t)
	p, err := NewSDKNSQWirePublisherWithFacts(address, owner, "api-wire-publisher", httpEndpoints)
	if err != nil {
		t.Fatal(err)
	}
	httpEndpoints[0] = "http://mutated.invalid:1"
	snapshot := owner.Snapshot()
	if len(snapshot.Transports) != 1 {
		t.Fatal("actual publisher unregistered")
	}
	fact := snapshot.Transports[0]
	actual := testsupport.ReceiveNSQIdentify(t, identities)
	if actual.ClientID != fact.ClientID || actual.Hostname != fact.Hostname || fact.State != "started" || fact.ClientID != owner.ClientID("api-wire-publisher") || fact.Hostname != owner.Hostname() || !reflect.DeepEqual(fact.NSQDTCPAddresses, []string{address}) || !reflect.DeepEqual(fact.NSQDHTTPAddresses, []string{"https://resolved-nsqd.internal:14451"}) || len(fact.NSQDPairs) != 0 {
		t.Fatal("actual constructor projection changed or guessed TCP/HTTP pairing")
	}
	if snapshot.BrokerConnectionsVerified || snapshot.ObservationComplete {
		t.Fatal("unused publisher claimed live broker connection")
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if owner.Snapshot().Transports[0].State != "stopped" {
		t.Fatal("publisher close did not invalidate observation")
	}
}

func TestWirePublisherFactsDoNotExposeAddressCredentials(t *testing.T) {
	owner, err := runtimefacts.New("apiserver", strings.Repeat("a", 40))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close() }()
	address, _ := testsupport.NSQPublisherFixture(t)
	p, err := NewSDKNSQWirePublisherWithFacts(address, owner, "api-wire-publisher", []string{"https://user:private-secret@nsqd.internal:4151"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	raw, err := json.Marshal(owner.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private-secret") || strings.Contains(string(raw), "user:") || owner.Snapshot().ObservationComplete || len(owner.Snapshot().Transports) != 0 {
		t.Fatal("unsafe projection retained credentials or qualified")
	}
}
