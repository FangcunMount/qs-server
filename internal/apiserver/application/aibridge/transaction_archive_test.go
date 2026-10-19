package aibridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

func archiveFixture(t *testing.T) (map[string]any, FrozenMessagingArchive) {
	t.Helper()
	raw, err := os.ReadFile("testdata/messaging-archive-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err = json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	decode := func(k string) []byte {
		b, e := hex.DecodeString(v[k].(string))
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	a, err := DecodeMessagingArchive(decode("payload_hex"), decode("wire_hex"))
	if err != nil {
		t.Fatal(err)
	}
	return v, a
}

func TestMessagingArchiveCrossLanguageGolden(t *testing.T) {
	v, a := archiveFixture(t)
	if a.OrganizationID != ^uint64(0) || a.Sequence != ^uint64(0) || !a.Ordered || !a.RequiresReceipt {
		t.Fatal("unsigned identity or flags changed")
	}
	raw, err := a.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(raw) != v["payload_hex"] || hex.EncodeToString(a.Header) != v["header_hex"] || hex.EncodeToString(a.Body) != v["body_hex"] {
		t.Fatal("original byte fields changed")
	}
	if !bytes.HasSuffix(a.Header, []byte{0xa0, 0x06, 0x07}) {
		t.Fatal("unknown protobuf field lost")
	}
	b, err := FreezeMessagingBranch(&PreparedMessaging{Envelope: a.Envelope, Body: a.Body, Wire: a.Wire, Topic: CommandsTopic}, "18446744073709551615", a.Sequence, true, true, "command", "m3.archive.test", "m3-1")
	if err != nil {
		t.Fatal(err)
	}
	original, err := a.Message()
	if err != nil {
		t.Fatal(err)
	}
	b.Message = original // Decode the original winner without re-marshalling its header.
	fingerprint := original.Fingerprint()
	if hex.EncodeToString(fingerprint[:]) != v["message_fingerprint"] {
		t.Fatal("Go/Python fingerprints differ")
	}
	sum := sha256.Sum256(b.Wire)
	if hex.EncodeToString(sum[:]) != v["wire_sha256"] {
		t.Fatal("first wire changed")
	}
	plan, err := transaction.NewPlan(transaction.PlanInput{Owner: "qs.mysql", Scope: b.Message.Input().Scope, OperationID: "operation", IdempotencyKey: "same-request", InputHash: sha256.Sum256([]byte("input")), Branches: []transaction.BranchInput{b}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := transaction.EncodePlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := transaction.DecodePlan(encoded)
	if err != nil {
		t.Fatal(err)
	}
	found, err := DecodeMessagingBranch(restored.Input().Branches[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(found.Header, a.Header) || !bytes.Equal(found.Wire, a.Wire) {
		t.Fatal("winner recovery needs local candidate")
	}
	a.Envelope.MessageId = "mutated-after-freeze"
	if _, err = DecodeMessagingBranch(b); err != nil {
		t.Fatal("frozen plan borrowed mutable protobuf", err)
	}
}

func TestMessagingArchiveRefusesCorruption(t *testing.T) {
	_, a := archiveFixture(t)
	raw, _ := a.Encode()
	for _, name := range []string{"version", "flag", "length", "body", "truncated", "trailing"} {
		t.Run(name, func(t *testing.T) {
			b := bytes.Clone(raw)
			switch name {
			case "version":
				b[0] ^= 1
			case "flag":
				b[len(archiveMagic)+16] = 4
			case "length":
				copy(b[len(archiveMagic)+17:], []byte{255, 255, 255, 255})
			case "body":
				b[len(b)-1] ^= 1
			case "truncated":
				b = b[:len(b)-1]
			case "trailing":
				b = append(b, 0)
			}
			if _, err := DecodeMessagingArchive(b, a.Wire); err == nil {
				t.Fatal("accepted damaged archive")
			}
		})
	}
	b, err := FreezeMessagingBranch(&PreparedMessaging{Envelope: a.Envelope, Body: a.Body, Wire: a.Wire, Topic: CommandsTopic}, "18446744073709551615", a.Sequence, true, true, "command", "physical", "m3-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"scope", "route", "wire"} {
		t.Run(name, func(t *testing.T) {
			changed := b
			switch name {
			case "scope":
				in := b.Message.Input()
				in.Scope = "org:7"
				changed.Message, _ = message.New(in)
			case "route":
				changed.RouteRevision = "wrong"
			case "wire":
				changed.Wire = []byte("wrong")
			}
			if _, err := DecodeMessagingBranch(changed); err == nil {
				t.Fatal("accepted changed identity")
			}
		})
	}
}
