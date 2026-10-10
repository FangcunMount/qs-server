// Package runtimefactsreader observes loaded local transports. Neither a
// snapshot nor its broker observations authorizes a writer fence or acceptance.
package runtimefactsreader

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/runtimefacts"
)

const queryLimit = 256 * 1024
const requestTimeout = 5 * time.Second

// BorrowedProcess must be supplied by the original native Docker/session owner.
// Proc and Root are borrowed original descriptors; this package never closes
// them. Expected values are checked against real Linux state, not accepted as
// proof of Docker, image, Window, lease or source provenance.
type BorrowedProcess struct {
	Proc, Root                   *os.File
	HostPID, UID                 int
	StartTimeTicks               uint64
	BootID, Component, SourceSHA string
}

type NativeIdentity struct {
	HostPID, UID                                     int
	StartTimeTicks                                   uint64
	BootID                                           string
	RootDevice, RootInode, SocketDevice, SocketInode uint64
}

type ClientObservation struct {
	TransportID, Direction, ClientID, Hostname, Topic, Channel, RemoteAddress string
	ConnectTime                                                               int64
}

type TopicObservation struct {
	TransportID, Topic string
	Channels           []string
}

type NodeObservation struct {
	TCPAddress, HTTPAddress string
	BrokerVersion           string
	BrokerStartTime         int64
	Topics                  []TopicObservation
	Clients                 []ClientObservation
}

// Observation is data, deliberately not an acceptance token. Gaps remain hard
// failures of completeness even when the recorded local consumer reads succeed.
type Observation struct {
	Snapshot runtimefacts.Snapshot
	Native   NativeIdentity
	Nodes    []NodeObservation
	Gaps     []string
}

var ErrScopeUnproven = errors.New("runtime facts broker scope unproven")

// Observe uses only the original process handles and the host's borrowed HTTP
// transport. It creates no MQ connection, sends no message and owns no host
// transaction. Redirects are refused; authentication is never synthesized.
func Observe(ctx context.Context, process BorrowedProcess, client *http.Client) (Observation, error) {
	return observe(ctx, func(ctx context.Context) (runtimefacts.Snapshot, NativeIdentity, error) {
		return QuerySnapshot(ctx, process)
	}, client)
}

func observe(ctx context.Context, query func(context.Context) (runtimefacts.Snapshot, NativeIdentity, error), client *http.Client) (Observation, error) {
	var result Observation
	if ctx == nil || client == nil || query == nil {
		return result, errors.New("runtime facts original reader unavailable")
	}
	q, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	first, native, err := query(q)
	if err != nil {
		return result, err
	}
	if err = validateSnapshot(first); err != nil {
		return result, err
	}
	result.Snapshot, result.Native = first, native
	reader, err := newBrokerReader(client)
	if err != nil {
		return result, err
	}
	before, gaps, err := reader.collect(q, first)
	if err != nil {
		return result, err
	}
	after, afterGaps, err := reader.collect(q, first)
	if err != nil {
		return result, err
	}
	last, lastNative, err := query(q)
	if err != nil {
		return result, err
	}
	if err = validateSnapshot(last); err != nil {
		return result, err
	}
	if !sameSnapshot(first, last) || native != lastNative || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(gaps, afterGaps) {
		return result, errors.New("runtime facts process topology or connections changed")
	}
	result.Nodes = after
	result.Gaps = append(gaps, "external_qs_ai_not_locally_proven")
	return result, ErrScopeUnproven
}

func sameSnapshot(a, b runtimefacts.Snapshot) bool {
	a.ObservedAt, b.ObservedAt = "", ""
	return reflect.DeepEqual(a, b)
}

func validHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return s != strings.Repeat("0", n)
}

func validComponent(s string) bool {
	return s == "apiserver" || s == "worker" || s == "collection-server"
}

func validateSnapshot(s runtimefacts.Snapshot) error {
	if s.FormatVersion != runtimefacts.SnapshotVersion || !validComponent(s.Component) || !validHex(s.SourceSHA, 40) || !validHex(s.ProcessNonce, 64) || !validText(s.Hostname, 253) || s.Process.PID != 1 || s.Process.UID < 0 || s.Process.StartTimeTicks == 0 || s.Process.BootID == "" || s.Lifecycle != "started" || !s.ObservationComplete || len(s.IncompleteReasons) != 0 || s.BrokerConnectionsVerified || len(s.Transports) == 0 || len(s.Transports) > 64 {
		return errors.New("runtime facts complete original snapshot unavailable")
	}
	if _, e := time.Parse(time.RFC3339Nano, s.ObservedAt); e != nil {
		return errors.New("runtime facts observation time invalid")
	}
	// Reuse the producer's exact declaration validator and identity rules rather
	// than maintain a second configuration parser. No native owner is started.
	validator, e := runtimefacts.New(s.Component, s.SourceSHA)
	if e != nil {
		return errors.New("runtime facts declaration validator unavailable")
	}
	seen := make(map[string]bool)
	clientIDs := make(map[string]bool)
	for _, f := range s.Transports {
		if seen[f.ID] || (f.State != "started" && f.State != "no_local_mq") || len(f.Subscriptions) > 128 {
			return errors.New("runtime facts transport completeness invalid")
		}
		seen[f.ID] = true
		if f.Direction == "no_local_mq" {
			if s.Component != "collection-server" || len(s.Transports) != 1 || f.State != "no_local_mq" || len(f.Subscriptions) != 0 || f.Provider != "none" || f.ClientID != "" || f.Hostname != "" || len(f.LookupdAddresses)+len(f.NSQDTCPAddresses)+len(f.NSQDHTTPAddresses)+len(f.NSQDPairs)+len(f.PublishTopics) != 0 {
				return errors.New("runtime facts no-local-MQ role invalid")
			}
			if e = validator.NoLocalMQ(); e != nil {
				return errors.New("runtime facts no-local-MQ declaration invalid")
			}
			continue
		}
		// The live nonce determines every declared driver ID, but v1 does not carry
		// its scope. Validate declaration shape without pretending to know that
		// scope, by issuing the validator's own identity for this copied declaration.
		t := f.Transport
		if f.State != "started" {
			return errors.New("runtime facts loaded transport not started")
		}
		if clientIDs[t.ClientID] || t.Hostname != s.Hostname || len(t.ClientID) != 67 || !strings.HasPrefix(t.ClientID, "qs-") || !validHex(t.ClientID[3:], 64) {
			return errors.New("runtime facts driver identity invalid")
		}
		clientIDs[t.ClientID] = true
		t.Hostname, t.ClientID = validator.Hostname(), validator.ClientID(f.ID)
		if e = validator.Declare(t); e != nil {
			return errors.New("runtime facts loaded declaration invalid")
		}
		if f.Direction == "consumer" && len(f.Subscriptions) == 0 || f.Direction != "consumer" && len(f.Subscriptions) != 0 {
			return errors.New("runtime facts subscription role invalid")
		}
		for _, sub := range f.Subscriptions {
			if e = validator.MarkSubscribed(f.ID, sub); e != nil {
				return errors.New("runtime facts loaded subscription invalid")
			}
		}
	}
	return nil
}

func decodeResponse(raw []byte, challenge string) (runtimefacts.Snapshot, error) {
	var r runtimefacts.Response
	if len(raw) == 0 || len(raw) > queryLimit || raw[len(raw)-1] != '\n' || bytes.Count(raw, []byte{'\n'}) != 1 {
		return r.Snapshot, errors.New("runtime facts response boundary invalid")
	}
	if err := rejectDuplicateJSON(raw); err != nil {
		return r.Snapshot, err
	}
	if err := requiredResponseFields(raw); err != nil {
		return r.Snapshot, err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil {
		return r.Snapshot, errors.New("runtime facts response shape invalid")
	}
	if _, err := d.Token(); err != io.EOF || r.FormatVersion != runtimefacts.QueryVersion || r.Challenge != challenge || !validHex(challenge, 64) {
		return r.Snapshot, errors.New("runtime facts response challenge invalid")
	}
	if err := validateSnapshot(r.Snapshot); err != nil {
		return r.Snapshot, err
	}
	return r.Snapshot, nil
}

func newChallenge() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", errors.New("runtime facts challenge unavailable")
	}
	return hex.EncodeToString(b[:]), nil
}

// The native and broker readers reject duplicate keys at every depth. Broker
// extensions are permitted, but ambiguity in an identity field is never valid.
func rejectDuplicateJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := walkJSON(d, 0); err != nil {
		return err
	}
	if _, e := d.Token(); e != io.EOF {
		return errors.New("runtime facts JSON trailing data")
	}
	return nil
}

func walkJSON(d *json.Decoder, depth int) error {
	if depth > 32 {
		return errors.New("runtime facts JSON depth exceeded")
	}
	t, e := d.Token()
	if e != nil {
		return errors.New("runtime facts JSON invalid")
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys := make(map[string]bool)
		for d.More() {
			t, e = d.Token()
			key, ok := t.(string)
			if e != nil || !ok || keys[key] {
				return errors.New("runtime facts JSON duplicate or invalid key")
			}
			keys[key] = true
			if e = walkJSON(d, depth+1); e != nil {
				return e
			}
		}
		t, e = d.Token()
		if e != nil || t != json.Delim('}') {
			return errors.New("runtime facts JSON object invalid")
		}
	case '[':
		for d.More() {
			if e = walkJSON(d, depth+1); e != nil {
				return e
			}
		}
		t, e = d.Token()
		if e != nil || t != json.Delim(']') {
			return errors.New("runtime facts JSON array invalid")
		}
	default:
		return errors.New("runtime facts JSON delimiter invalid")
	}
	return nil
}

func validText(s string, max int) bool {
	if len(s) == 0 || len(s) > max {
		return false
	}
	for _, c := range s {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}

// v1 emits every field, including empty arrays and false observations. Missing
// fields cannot be treated as an affirmative empty transport or broker scope.
func requiredResponseFields(raw []byte) error {
	root, e := requiredObject(raw, "format_version", "challenge", "snapshot")
	if e != nil {
		return e
	}
	snapshot, e := requiredObject(root["snapshot"], "format_version", "component", "source_sha", "process_nonce", "hostname", "process", "observed_at", "lifecycle", "observation_complete", "incomplete_reasons", "broker_connections_verified", "transports")
	if e != nil {
		return e
	}
	if _, e = requiredObject(snapshot["process"], "pid", "uid", "start_time_ticks", "boot_id"); e != nil {
		return e
	}
	var transports []json.RawMessage
	if json.Unmarshal(snapshot["transports"], &transports) != nil || transports == nil {
		return errors.New("runtime facts transport inventory omitted")
	}
	for _, raw := range transports {
		transport, e := requiredObject(raw, "id", "provider", "direction", "lookupd_addresses", "nsqd_tcp_addresses", "nsqd_http_addresses", "nsqd_pairs", "publish_topics", "client_id", "hostname", "state", "subscriptions")
		if e != nil {
			return e
		}
		var subscriptions []json.RawMessage
		if json.Unmarshal(transport["subscriptions"], &subscriptions) != nil || subscriptions == nil {
			return errors.New("runtime facts subscription inventory omitted")
		}
		for _, raw := range subscriptions {
			if _, e = requiredObject(raw, "topic", "channel", "failure_topic", "failure_channel"); e != nil {
				return e
			}
		}
	}
	return nil
}
func requiredObject(raw []byte, keys ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, errors.New("runtime facts required object missing")
	}
	for _, key := range keys {
		if fields[key] == nil || string(fields[key]) == "null" {
			return nil, errors.New("runtime facts required field missing")
		}
	}
	return fields, nil
}
