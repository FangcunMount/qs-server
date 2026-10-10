// Package runtimefacts records the message transports actually registered by a
// process. Its private query channel is an observation, never a writer fence.
package runtimefacts

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	SnapshotVersion  = "qs-runtime-facts/v1"
	QueryVersion     = "qs-runtime-facts-query/v1"
	maxTransports    = 64
	maxAddresses     = 64
	maxSubscriptions = 128
)

// NSQDPair must come from the loaded configuration. No port is inferred.
type NSQDPair struct {
	TCPAddress  string `json:"tcp_address"`
	HTTPAddress string `json:"http_address"`
}

type Transport struct {
	ID                string     `json:"id"`
	Provider          string     `json:"provider"`
	Direction         string     `json:"direction"`
	LookupdAddresses  []string   `json:"lookupd_addresses"`
	NSQDTCPAddresses  []string   `json:"nsqd_tcp_addresses"`
	NSQDHTTPAddresses []string   `json:"nsqd_http_addresses"`
	NSQDPairs         []NSQDPair `json:"nsqd_pairs"`
	PublishTopics     []string   `json:"publish_topics"`
	ClientID          string     `json:"client_id"`
	Hostname          string     `json:"hostname"`
}

// MarkSubscribed is called only after the corresponding driver subscription
// succeeds. Failure fields describe the actual subscription configuration.
type Subscription struct {
	Topic          string `json:"topic"`
	Channel        string `json:"channel"`
	FailureTopic   string `json:"failure_topic"`
	FailureChannel string `json:"failure_channel"`
}

type TransportFact struct {
	Transport
	State         string         `json:"state"`
	Subscriptions []Subscription `json:"subscriptions"`
}

type ProcessIdentity struct {
	PID            int    `json:"pid"`
	UID            int    `json:"uid"`
	StartTimeTicks uint64 `json:"start_time_ticks"`
	BootID         string `json:"boot_id"`
}

type Snapshot struct {
	FormatVersion             string          `json:"format_version"`
	Component                 string          `json:"component"`
	SourceSHA                 string          `json:"source_sha"`
	ProcessNonce              string          `json:"process_nonce"`
	Hostname                  string          `json:"hostname"`
	Process                   ProcessIdentity `json:"process"`
	ObservedAt                string          `json:"observed_at"`
	Lifecycle                 string          `json:"lifecycle"`
	ObservationComplete       bool            `json:"observation_complete"`
	IncompleteReasons         []string        `json:"incomplete_reasons"`
	BrokerConnectionsVerified bool            `json:"broker_connections_verified"`
	Transports                []TransportFact `json:"transports"`
}

type nativeEndpoint interface {
	Listener() *net.UnixListener
	Path() string
	Verify() error
	Close() error
}

// Owner does not own any borrowed MQ client, connection, pool or transaction.
type Owner struct {
	mu                                    sync.Mutex
	component, sourceSHA, nonce, hostname string
	identity                              ProcessIdentity
	qualification                         []string
	transports                            map[string]*TransportFact
	startAttempted, started, closed       bool
	endpoint                              nativeEndpoint
	connections                           map[*net.UnixConn]struct{}
	clientIDs                             map[string]bool
	wg                                    sync.WaitGroup
}

// New still provides a process nonce when development builds have no source
// SHA or native Linux identity. Such observations can never be complete.
func New(component, sourceSHA string) (*Owner, error) {
	if !validName(component, 40) {
		return nil, errors.New("invalid runtime facts component")
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, errors.New("runtime facts nonce unavailable")
	}
	o := &Owner{component: component, nonce: hex.EncodeToString(nonce[:]), transports: make(map[string]*TransportFact), connections: make(map[*net.UnixConn]struct{}), clientIDs: make(map[string]bool)}
	if validHex(sourceSHA, 40) && sourceSHA != strings.Repeat("0", 40) {
		o.sourceSHA = sourceSHA
	} else {
		o.qualification = append(o.qualification, "source_unbound")
	}
	hostname, err := os.Hostname()
	if err == nil && validText(hostname, 253) {
		o.hostname = hostname
	} else {
		o.qualification = append(o.qualification, "hostname_unavailable")
	}
	identity, err := nativeIdentity()
	if err != nil {
		o.qualification = append(o.qualification, "native_identity_unavailable")
	} else {
		o.identity = identity
	}
	return o, nil
}

func (o *Owner) Hostname() string { return o.hostname }

// ClientID is stable for a scope within this process and changes on restart.
func (o *Owner) ClientID(scope string) string {
	h := sha256.Sum256([]byte(o.nonce + "\x00" + scope))
	id := "qs-" + hex.EncodeToString(h[:])
	o.mu.Lock()
	o.clientIDs[id] = true
	o.mu.Unlock()
	return id
}

func (o *Owner) Declare(t Transport) error {
	if t.Direction == "no_local_mq" && o.component != "collection-server" {
		o.mu.Lock()
		o.qualification = append(o.qualification, "no_local_mq_role_invalid")
		o.mu.Unlock()
		return errors.New("runtime facts no-local-MQ role requires collection-server")
	}
	if err := validateTransport(t, o.hostname); err != nil {
		o.mu.Lock()
		o.qualification = append(o.qualification, "transport_registration_failed")
		o.mu.Unlock()
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.startAttempted || o.closed {
		return errors.New("runtime facts declarations sealed")
	}
	if t.Direction != "no_local_mq" && !o.clientIDs[t.ClientID] {
		return errors.New("runtime facts client identity not process-bound")
	}
	if len(o.transports) >= maxTransports {
		return errors.New("runtime facts transport limit")
	}
	if _, exists := o.transports[t.ID]; exists {
		return errors.New("runtime facts transport already declared")
	}
	for _, previous := range o.transports {
		if t.Direction == "no_local_mq" || previous.Direction == "no_local_mq" {
			return errors.New("runtime facts no-local-MQ role conflicts")
		}
		if previous.ClientID == t.ClientID {
			return errors.New("runtime facts client identity reused")
		}
	}
	f := &TransportFact{Transport: copyTransport(t), State: "declared", Subscriptions: []Subscription{}}
	if t.Direction == "no_local_mq" {
		f.State = "no_local_mq"
	}
	o.transports[t.ID] = f
	return nil
}

// NoLocalMQ is the collection-server composition's explicit absence of a local
// message driver. Other components cannot use it to qualify an empty inventory.
func (o *Owner) NoLocalMQ() error {
	return o.Declare(Transport{ID: "no-local-mq", Provider: "none", Direction: "no_local_mq"})
}

func (o *Owner) MarkSubscribed(id string, s Subscription) error {
	if !validNSQName(s.Topic) || !validNSQName(s.Channel) || (s.FailureTopic != "" && !validNSQName(s.FailureTopic)) || (s.FailureChannel != "" && !validNSQName(s.FailureChannel)) || ((s.FailureTopic == "") != (s.FailureChannel == "")) {
		o.MarkIncomplete(id)
		return errors.New("invalid runtime facts subscription")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	f, err := o.mutableTransport(id)
	if err != nil {
		return err
	}
	if f.Direction != "consumer" || len(f.Subscriptions) >= maxSubscriptions {
		return errors.New("invalid runtime facts subscription role or limit")
	}
	for _, existing := range f.Subscriptions {
		if existing.Topic == s.Topic && existing.Channel == s.Channel {
			return errors.New("runtime facts subscription already recorded")
		}
	}
	f.Subscriptions = append(f.Subscriptions, s)
	return nil
}

func (o *Owner) MarkStarted(id string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	f, err := o.mutableTransport(id)
	if err != nil {
		return err
	}
	if f.Direction == "no_local_mq" {
		return nil
	}
	if f.Direction == "consumer" && len(f.Subscriptions) == 0 {
		return errors.New("runtime facts consumer has no successful subscription")
	}
	f.State = "started"
	return nil
}

func (o *Owner) MarkIncomplete(id string) { o.markTerminal(id, "incomplete") }
func (o *Owner) MarkStopped(id string)    { o.markTerminal(id, "stopped") }

func (o *Owner) markTerminal(id, state string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if f, ok := o.transports[id]; ok {
		if f.State != "stopped" {
			f.State = state
		}
	} else {
		o.qualification = append(o.qualification, "unregistered_transport_transition")
	}
}

func (o *Owner) mutableTransport(id string) (*TransportFact, error) {
	f, exists := o.transports[id]
	if !exists || o.closed || f.State == "incomplete" || f.State == "stopped" {
		return nil, errors.New("runtime facts transport unavailable")
	}
	return f, nil
}

func (o *Owner) Start() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.startAttempted || o.closed {
		return errors.New("runtime facts owner cannot restart")
	}
	o.startAttempted = true
	endpoint, err := createNativeEndpoint(o.component)
	if err != nil {
		o.qualification = append(o.qualification, "private_channel_unavailable")
		return err
	}
	o.endpoint, o.started = endpoint, true
	o.wg.Add(1)
	go o.serve(endpoint.Listener())
	return nil
}

func (o *Owner) Path() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.endpoint == nil {
		return ""
	}
	return o.endpoint.Path()
}

func (o *Owner) Snapshot() Snapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	s := Snapshot{FormatVersion: SnapshotVersion, Component: o.component, SourceSHA: o.sourceSHA, ProcessNonce: o.nonce, Hostname: o.hostname, Process: o.identity, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Lifecycle: "declared", IncompleteReasons: append([]string{}, o.qualification...), Transports: []TransportFact{}}
	if o.closed {
		s.Lifecycle = "stopped"
	} else if o.started {
		s.Lifecycle = "started"
	} else if o.startAttempted {
		s.Lifecycle = "incomplete"
	}
	if !o.started || o.closed {
		s.IncompleteReasons = append(s.IncompleteReasons, "owner_not_started")
	}
	if len(o.transports) == 0 {
		s.IncompleteReasons = append(s.IncompleteReasons, "message_role_unregistered")
	}
	if o.started && !o.closed {
		identity, err := nativeIdentity()
		if err != nil || identity != o.identity {
			s.IncompleteReasons = append(s.IncompleteReasons, "native_identity_changed")
		}
		if o.endpoint == nil || o.endpoint.Verify() != nil {
			s.IncompleteReasons = append(s.IncompleteReasons, "private_channel_identity_changed")
		}
	}
	for _, original := range o.transports {
		f := *original
		f.Transport = copyTransport(original.Transport)
		f.Subscriptions = append([]Subscription{}, original.Subscriptions...)
		sort.Slice(f.Subscriptions, func(i, j int) bool {
			return f.Subscriptions[i].Topic+"\x00"+f.Subscriptions[i].Channel < f.Subscriptions[j].Topic+"\x00"+f.Subscriptions[j].Channel
		})
		if f.State != "started" && f.State != "no_local_mq" {
			s.IncompleteReasons = append(s.IncompleteReasons, "transport_not_started")
		}
		s.Transports = append(s.Transports, f)
	}
	sort.Slice(s.Transports, func(i, j int) bool { return s.Transports[i].ID < s.Transports[j].ID })
	sort.Strings(s.IncompleteReasons)
	s.IncompleteReasons = compactStrings(s.IncompleteReasons)
	s.ObservationComplete = len(s.IncompleteReasons) == 0
	return s
}

func (o *Owner) Close() error {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return nil
	}
	o.closed, o.started = true, false
	for _, f := range o.transports {
		f.State = "stopped"
	}
	endpoint := o.endpoint
	for conn := range o.connections {
		_ = conn.Close()
	}
	o.mu.Unlock()
	var err error
	if endpoint != nil {
		err = endpoint.Close()
	}
	o.wg.Wait()
	return err
}

func copyTransport(t Transport) Transport {
	t.LookupdAddresses = append([]string{}, t.LookupdAddresses...)
	t.NSQDTCPAddresses = append([]string{}, t.NSQDTCPAddresses...)
	t.NSQDHTTPAddresses = append([]string{}, t.NSQDHTTPAddresses...)
	t.NSQDPairs = append([]NSQDPair{}, t.NSQDPairs...)
	t.PublishTopics = append([]string{}, t.PublishTopics...)
	sort.Strings(t.PublishTopics)
	return t
}

func validateTransport(t Transport, hostname string) error {
	if !validName(t.ID, 128) {
		return errors.New("invalid runtime facts transport ID")
	}
	if t.Direction == "no_local_mq" {
		if t.Provider != "none" || len(t.LookupdAddresses)+len(t.NSQDTCPAddresses)+len(t.NSQDHTTPAddresses)+len(t.NSQDPairs)+len(t.PublishTopics) != 0 || t.ClientID != "" || t.Hostname != "" {
			return errors.New("invalid runtime facts no-local-MQ role")
		}
		return nil
	}
	if t.Provider != "nsq" || (t.Direction != "consumer" && t.Direction != "publisher") || !validText(t.ClientID, 128) || t.Hostname != hostname || !validText(hostname, 253) {
		return errors.New("invalid runtime facts transport identity")
	}
	if len(t.LookupdAddresses)+len(t.NSQDTCPAddresses) == 0 {
		return errors.New("runtime facts transport addresses missing")
	}
	for _, group := range []struct {
		values []string
		http   bool
	}{{t.LookupdAddresses, true}, {t.NSQDTCPAddresses, false}, {t.NSQDHTTPAddresses, true}} {
		if len(group.values) > maxAddresses {
			return errors.New("runtime facts address limit")
		}
		seen := make(map[string]bool)
		for _, address := range group.values {
			if !validAddress(address, group.http) || seen[address] {
				return errors.New("invalid or duplicate runtime facts address")
			}
			seen[address] = true
		}
	}
	if len(t.NSQDPairs) > maxAddresses {
		return errors.New("runtime facts address pair limit")
	}
	if len(t.PublishTopics) > maxSubscriptions || (t.Direction != "publisher" && len(t.PublishTopics) != 0) {
		return errors.New("invalid runtime facts publish topic role or limit")
	}
	topics := make(map[string]bool)
	for _, topic := range t.PublishTopics {
		if !validNSQName(topic) || topics[topic] {
			return errors.New("invalid or duplicate runtime facts publish topic")
		}
		topics[topic] = true
	}
	seen := make(map[string]bool)
	for _, pair := range t.NSQDPairs {
		if !validAddress(pair.TCPAddress, false) || !validAddress(pair.HTTPAddress, true) || !contains(t.NSQDTCPAddresses, pair.TCPAddress) || !contains(t.NSQDHTTPAddresses, pair.HTTPAddress) || seen[pair.TCPAddress] {
			return errors.New("invalid runtime facts address pair")
		}
		seen[pair.TCPAddress] = true
	}
	return nil
}

func validAddress(value string, allowHTTP bool) bool {
	if !validText(value, 512) {
		return false
	}
	address := value
	if strings.Contains(value, "://") {
		if !allowHTTP {
			return false
		}
		u, err := url.Parse(value)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
			return false
		}
		address = u.Host
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || !validText(host, 253) || strings.ContainsAny(host, "@/?#\\%") {
		return false
	}
	if net.ParseIP(host) == nil {
		for _, c := range host {
			if allowed := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_'; !allowed {
				return false
			}
		}
	}
	p, err := strconv.ParseUint(port, 10, 16)
	return err == nil && p > 0 && fmt.Sprint(p) == port
}

func validText(value string, limit int) bool {
	if len(value) == 0 || len(value) > limit {
		return false
	}
	for _, c := range value {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}

func validName(value string, limit int) bool {
	if len(value) == 0 || len(value) > limit {
		return false
	}
	for _, c := range value {
		if allowed := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_'; !allowed {
			return false
		}
	}
	return true
}

func validHex(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for _, c := range value {
		if allowed := (c >= 'a' && c <= 'f') || (c >= '0' && c <= '9'); !allowed {
			return false
		}
	}
	return true
}

func validNSQName(value string) bool {
	base := strings.TrimSuffix(value, "#ephemeral")
	if len(value) == 0 || len(value) > 64 || base == "" {
		return false
	}
	for _, c := range base {
		if allowed := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.ContainsRune("._-", c); !allowed {
			return false
		}
	}
	return true
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func compactStrings(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
