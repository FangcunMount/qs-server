package retirement

import (
	"context"
	"encoding/hex"
	"io"
	"reflect"
)

const ErrSourceAuthentication SourceError = "source_facts_authentication_rejected"

// A fixed logical reservation bounds index allocation before any scan. It is
// not a claim about exact Go heap/RSS; the host must also supply process limits.
const sourceIndexReservationPerRow uint64 = 512
const MaxSourceIndexReservation uint64 = 1 << 30

// Expected must come from independently approved inventory, never its header.
// Readers are borrowed: this verifier opens/closes no file or database and
// does not create an approval from an operator-supplied complete flag.
type SourceCopyInput struct {
	Input    io.Reader
	Expected SourceCopyExpectation
}

type verifiedSourceKey struct {
	object uint8
	pk     [32]byte
}
type verifiedSourceRow struct{ facts [32]byte }
type verifiedAIPair struct {
	shared            [32]byte
	delivered, legacy bool
}

// VerifiedSourceCopies is opaque and body-free. It proves complete-copy bytes
// and the exact decoded facts relative to the supplied approved expectations.
// Independent workflow/host/operation provenance remains the caller's gate.
type VerifiedSourceCopies struct {
	rows                 map[verifiedSourceKey]verifiedSourceRow
	eventIDs             map[string]verifiedSourceKey
	pairs                map[string]verifiedAIPair
	receipts             [4]SourceCopyReceipt
	entries, reservation uint64
	complete             bool
}

type VerifiedSourceCopiesSummary struct {
	Complete                             bool                 `json:"complete"`
	Entries                              uint64               `json:"entries"`
	IndexReservationBytes                uint64               `json:"index_reservation_bytes"`
	Copies                               [4]SourceCopyReceipt `json:"copies"`
	ExternalOriginAuthenticationRequired bool                 `json:"external_origin_authentication_required"`
	HostProcessBudgetRequired            bool                 `json:"host_process_budget_required"`
	BusinessClosureVerified              bool                 `json:"business_closure_verified"`
	DropReady                            bool                 `json:"drop_ready"`
}

func (*VerifiedSourceCopies) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*VerifiedSourceCopies) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (v *VerifiedSourceCopies) GoString() string           { return v.String() }
func (*VerifiedSourceCopies) String() string {
	return "private complete-copy fact index; not retirement approval"
}
func (v *VerifiedSourceCopies) Summary() VerifiedSourceCopiesSummary {
	s := VerifiedSourceCopiesSummary{ExternalOriginAuthenticationRequired: true, HostProcessBudgetRequired: true}
	if v != nil {
		s.Complete = v.complete
		s.Entries = v.entries
		s.IndexReservationBytes = v.reservation
		s.Copies = v.receipts
	}
	return s
}

// VerifySourceCopies authenticates all four exact source copies to clean EOF
// before publishing an index or minting a bound event/command. A later failure
// returns nil, even when an earlier copy had already completed successfully.
func VerifySourceCopies(ctx context.Context, copies []SourceCopyInput) (*VerifiedSourceCopies, error) {
	if ctx == nil || len(copies) != 4 {
		return nil, ErrSourceAuthentication
	}
	if ctx.Err() != nil {
		return nil, ErrSourceIncomplete
	}
	expectedObjects := [4][2]string{{"mysql", "domain_event_outbox"}, {"mysql", AIBridgeCommandSource}, {"mysql", AILegacyCommandSource}, {"mongodb", "domain_event_outbox"}}
	var approvedRecords uint64
	for i, input := range copies {
		b := input.Expected.Boundary
		if sourceReaderAbsent(input.Input) || !b.Present || b.Database != expectedObjects[i][0] || b.Name != expectedObjects[i][1] || input.Expected.Records > MaxSourceRecords {
			return nil, ErrSourceAuthentication
		}
		if input.Expected.Records > (MaxSourceIndexReservation/sourceIndexReservationPerRow)-approvedRecords {
			return nil, ErrSourceBounds
		}
		approvedRecords += input.Expected.Records
	}
	v := &VerifiedSourceCopies{rows: make(map[verifiedSourceKey]verifiedSourceRow, int(approvedRecords)), eventIDs: make(map[string]verifiedSourceKey), pairs: make(map[string]verifiedAIPair), reservation: approvedRecords * sourceIndexReservationPerRow}
	for i, input := range copies {
		if err := ctx.Err(); err != nil {
			return nil, ErrSourceIncomplete
		}
		if i == 0 || i == 3 {
			var next func() (*DecodedSourceEvent, error)
			var receipt func() SourceCopyReceipt
			if i == 0 {
				r, err := NewSQLSourceReader(input.Input, input.Expected)
				if err != nil {
					return nil, err
				}
				next = r.Next
				receipt = r.Receipt
			} else {
				r, err := NewMongoSourceReader(input.Input, input.Expected)
				if err != nil {
					return nil, err
				}
				next = r.Next
				receipt = r.Receipt
			}
			for {
				if err := ctx.Err(); err != nil {
					return nil, ErrSourceIncomplete
				}
				event, err := next()
				if err == io.EOF {
					break
				}
				if err != nil {
					return nil, err
				}
				key, err := sourceAuthKey(event.Source.Database, event.Source.Object, event.Source.PrimaryKeySHA256)
				if err != nil {
					return nil, err
				}
				if prior, exists := v.eventIDs[event.EventID]; exists && prior != key {
					return nil, ErrSourceIdentity
				}
				digest, err := privateFactsSHA(event)
				if err != nil {
					return nil, err
				}
				if err = v.add(key, digest); err != nil {
					return nil, err
				}
				v.eventIDs[event.EventID] = key
			}
			v.receipts[i] = receipt()
		} else {
			r, err := NewAISQLSourceReader(input.Input, input.Expected)
			if err != nil {
				return nil, err
			}
			for {
				if err := ctx.Err(); err != nil {
					return nil, ErrSourceIncomplete
				}
				command, err := r.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					return nil, err
				}
				key, err := sourceAuthKey(command.Source.Database, command.Source.Object, command.Source.PrimaryKeySHA256)
				if err != nil {
					return nil, err
				}
				digest, err := privateFactsSHA(command)
				if err != nil {
					return nil, err
				}
				if err = v.add(key, digest); err != nil {
					return nil, err
				}
				shared, err := aiSharedFactsSHA(command)
				if err != nil {
					return nil, err
				}
				if i == 1 {
					if command.Transport.Delivered == nil {
						return nil, ErrAISourceHandoff
					}
					if _, duplicate := v.pairs[command.CommandID]; duplicate {
						return nil, ErrSourceIdentity
					}
					v.pairs[command.CommandID] = verifiedAIPair{shared: shared, delivered: *command.Transport.Delivered}
				} else {
					pair, exists := v.pairs[command.CommandID]
					if !exists || pair.delivered || pair.legacy || pair.shared != shared {
						return nil, ErrAISourceHandoff
					}
					pair.legacy = true
					v.pairs[command.CommandID] = pair
				}
			}
			v.receipts[i] = r.Receipt()
		}
		got := v.receipts[i]
		if !got.Complete || got.Records != input.Expected.Records || got.Bytes != input.Expected.Bytes || got.DataHash != input.Expected.DataHash || got.BusinessClosureVerified || got.DropReady {
			return nil, ErrSourceIncomplete
		}
	}
	if ctx.Err() != nil || v.entries != approvedRecords {
		return nil, ErrSourceIncomplete
	}
	v.complete = true
	// ID uniqueness and paired-command facts have been checked; only the keyed
	// row/fact digests are needed to authenticate future streamed observations.
	v.eventIDs = nil
	v.pairs = nil
	return v, nil
}

func sourceReaderAbsent(reader io.Reader) bool {
	if reader == nil {
		return true
	}
	v := reflect.ValueOf(reader)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func (v *VerifiedSourceCopies) add(key verifiedSourceKey, digest [32]byte) error {
	if _, exists := v.rows[key]; exists {
		return ErrSourceIdentity
	}
	v.rows[key] = verifiedSourceRow{facts: digest}
	v.entries++
	return nil
}

func sourceAuthKey(database, object, pk string) (verifiedSourceKey, error) {
	var key verifiedSourceKey
	switch {
	case database == "mysql" && object == "domain_event_outbox":
		key.object = 0
	case database == "mysql" && object == AIBridgeCommandSource:
		key.object = 1
	case database == "mysql" && object == AILegacyCommandSource:
		key.object = 2
	case database == "mongodb" && object == "domain_event_outbox":
		key.object = 3
	default:
		return key, ErrSourceAuthentication
	}
	raw, err := hex.DecodeString(pk)
	if err != nil || len(raw) != 32 || hex.EncodeToString(raw) != pk {
		return key, ErrSourceAuthentication
	}
	copy(key.pk[:], raw)
	return key, nil
}

// Match the exact original handoff contract without retaining source bodies.
// This digest is private pairing state, not an SDK/body/writer digest.
func aiSharedFactsSHA(v *DecodedAICommand) ([32]byte, error) {
	if !aiDecodedIdentityValid(v) {
		return [32]byte{}, ErrAISourceHandoff
	}
	facts := struct {
		CommandID, RequestID, SourceKind, MessagingKind, OrganizationID, SubjectID, ResourceID string
		PayloadSHA, WriterKind, WriterSHA                                                      string
		Attempts                                                                               uint32
		AvailableAt                                                                            string
		Business                                                                               AICommandBusinessFacts
	}{v.CommandID, v.RequestID, v.SourceKind, v.MessagingKind, v.OrganizationID, v.SubjectID, v.ResourceID, v.PayloadBytesDigest.SHA256, v.WriterPayloadDigest.Kind, v.WriterPayloadDigest.SHA256, v.Transport.SourceAttempts, v.Transport.SourceAvailableAt, v.Business}
	return privateFactsSHA(facts)
}

type VerifiedSourceEvent struct{ facts *DecodedSourceEvent }
type VerifiedSourceAICommand struct{ facts *DecodedAICommand }

func (*VerifiedSourceEvent) MarshalJSON() ([]byte, error)     { return nil, ErrSourceSerialization }
func (*VerifiedSourceAICommand) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*VerifiedSourceEvent) MarshalBSON() ([]byte, error)     { return nil, ErrSourceSerialization }
func (*VerifiedSourceAICommand) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (v *VerifiedSourceEvent) GoString() string               { return v.String() }
func (*VerifiedSourceEvent) String() string {
	return "private verified source event; business closure unproven"
}
func (v *VerifiedSourceAICommand) GoString() string { return v.String() }
func (*VerifiedSourceAICommand) String() string {
	return "private verified source command; business closure unproven"
}

func (v *VerifiedSourceCopies) bind(database, object, pk string, facts any) (reflect.Value, error) {
	if v == nil || !v.complete {
		return reflect.Value{}, ErrSourceAuthentication
	}
	key, err := sourceAuthKey(database, object, pk)
	if err != nil {
		return reflect.Value{}, err
	}
	row, exists := v.rows[key]
	if !exists {
		return reflect.Value{}, ErrSourceAuthentication
	}
	digest, err := privateFactsSHA(facts)
	if err != nil {
		return reflect.Value{}, err
	}
	if digest != row.facts {
		return reflect.Value{}, ErrSourceAuthentication
	}
	nodes := 0
	cloned, err := clonePrivateFacts(reflect.ValueOf(facts), 0, &nodes)
	if err != nil {
		return reflect.Value{}, err
	}
	// Caller must not mutate its DTO concurrently. This second comparison also
	// refuses a changed observation rather than accepting a partly copied DTO.
	digest, err = privateFactsSHA(cloned.Interface())
	if err != nil {
		return reflect.Value{}, err
	}
	if digest != row.facts {
		return reflect.Value{}, ErrSourceAuthentication
	}
	return cloned, nil
}
func (v *VerifiedSourceCopies) BindEvent(facts *DecodedSourceEvent) (*VerifiedSourceEvent, error) {
	if facts == nil {
		return nil, ErrSourceAuthentication
	}
	cloned, err := v.bind(facts.Source.Database, facts.Source.Object, facts.Source.PrimaryKeySHA256, facts)
	if err != nil {
		return nil, err
	}
	return &VerifiedSourceEvent{facts: cloned.Interface().(*DecodedSourceEvent)}, nil
}
func (v *VerifiedSourceCopies) BindAICommand(facts *DecodedAICommand) (*VerifiedSourceAICommand, error) {
	if facts == nil {
		return nil, ErrSourceAuthentication
	}
	cloned, err := v.bind(facts.Source.Database, facts.Source.Object, facts.Source.PrimaryKeySHA256, facts)
	if err != nil {
		return nil, err
	}
	return &VerifiedSourceAICommand{facts: cloned.Interface().(*DecodedAICommand)}, nil
}
func (v *VerifiedSourceEvent) Facts() (*DecodedSourceEvent, error) {
	if v == nil || v.facts == nil {
		return nil, ErrSourceAuthentication
	}
	nodes := 0
	copy, err := clonePrivateFacts(reflect.ValueOf(v.facts), 0, &nodes)
	if err != nil {
		return nil, err
	}
	return copy.Interface().(*DecodedSourceEvent), nil
}
func (v *VerifiedSourceAICommand) Facts() (*DecodedAICommand, error) {
	if v == nil || v.facts == nil {
		return nil, ErrSourceAuthentication
	}
	nodes := 0
	copy, err := clonePrivateFacts(reflect.ValueOf(v.facts), 0, &nodes)
	if err != nil {
		return nil, err
	}
	return copy.Interface().(*DecodedAICommand), nil
}
