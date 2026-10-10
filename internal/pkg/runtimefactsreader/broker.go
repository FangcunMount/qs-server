package runtimefactsreader

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/FangcunMount/qs-server/internal/pkg/runtimefacts"
)

const brokerBodyLimit = 1024 * 1024
const brokerTotalLimit = 8 * 1024 * 1024
const brokerRequestLimit = 1024

type brokerReader struct {
	client          http.Client
	bytes, requests int
	infoSeen        map[string]brokerInfo
}

type peer struct {
	BroadcastAddress string `json:"broadcast_address"`
	TCPPort          int    `json:"tcp_port"`
	HTTPPort         int    `json:"http_port"`
	Tombstones       []bool `json:"tombstones"`
}
type discovery struct {
	Producers []peer   `json:"producers"`
	Channels  []string `json:"channels"`
}
type brokerClient struct {
	Version       string `json:"version"`
	ClientID      string `json:"client_id"`
	Hostname      string `json:"hostname"`
	RemoteAddress string `json:"remote_address"`
	ConnectTime   int64  `json:"connect_ts"`
	State         int    `json:"state"`
}
type brokerChannel struct {
	Name        string         `json:"channel_name"`
	ClientCount int            `json:"client_count"`
	Paused      *bool          `json:"paused"`
	Clients     []brokerClient `json:"clients"`
}
type brokerTopic struct {
	Name     string          `json:"topic_name"`
	Paused   *bool           `json:"paused"`
	Channels []brokerChannel `json:"channels"`
}
type brokerInfo struct {
	BroadcastAddress string `json:"broadcast_address"`
	TCPPort          int    `json:"tcp_port"`
	HTTPPort         int    `json:"http_port"`
	Version          string `json:"version"`
	StartTime        int64  `json:"start_time"`
}

type brokerStats struct {
	Health    string         `json:"health"`
	Version   string         `json:"version"`
	StartTime int64          `json:"start_time"`
	Topics    []brokerTopic  `json:"topics"`
	Producers []brokerClient `json:"producers"`
}

func newBrokerReader(client *http.Client) (*brokerReader, error) {
	// Borrow the actual transport, but never inherit redirects, cookies or a
	// proxy that could make an advertised endpoint escape the registered scope.
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport == nil || transport.Proxy != nil {
		return nil, errors.New("runtime facts original direct HTTP transport unavailable")
	}
	return &brokerReader{client: http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func httpBase(address string) (string, error) {
	if !strings.Contains(address, "://") {
		address = "http://" + address
	}
	u, e := url.Parse(address)
	if e != nil || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("runtime facts HTTP address invalid")
	}
	if !validTCP(u.Host) {
		return "", errors.New("runtime facts HTTP endpoint invalid")
	}
	return u.String(), nil
}
func validTCP(address string) bool {
	host, port, e := net.SplitHostPort(address)
	p, pe := strconv.Atoi(port)
	return e == nil && pe == nil && p > 0 && p <= 65535 && strconv.Itoa(p) == port && host != "" && !strings.ContainsAny(host, "@/?#\\% ")
}

func (r *brokerReader) get(ctx context.Context, base, path string, target any) error {
	r.requests++
	if r.requests > brokerRequestLimit || r.bytes >= brokerTotalLimit {
		return errors.New("runtime facts broker observation bound exceeded")
	}
	q, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, e := http.NewRequestWithContext(q, http.MethodGet, base+path, nil)
	if e != nil {
		return errors.New("runtime facts broker GET unavailable")
	}
	request.Header.Set("Accept", "application/vnd.nsq; version=1.0")
	response, e := r.client.Do(request)
	if e != nil {
		return errors.New("runtime facts broker GET failed")
	}
	raw, e := io.ReadAll(io.LimitReader(response.Body, brokerBodyLimit+1))
	closeError := response.Body.Close()
	r.bytes += len(raw)
	if e != nil || closeError != nil || response.StatusCode != http.StatusOK || len(raw) > brokerBodyLimit || r.bytes > brokerTotalLimit {
		return errors.New("runtime facts broker response incomplete")
	}
	if strings.HasPrefix(path, "/stats?") {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields["topics"] == nil || fields["producers"] == nil {
			return errors.New("runtime facts broker identity inventory omitted")
		}
	}
	if rejectDuplicateJSON(raw) != nil || json.Unmarshal(raw, target) != nil {
		return errors.New("runtime facts broker response ambiguous")
	}
	return nil
}

func peerPairs(peers []peer) (map[string]string, error) {
	if len(peers) == 0 || len(peers) > 64 {
		return nil, errors.New("runtime facts discovery nodes missing or excessive")
	}
	result := make(map[string]string)
	for _, p := range peers {
		if p.BroadcastAddress == "" || p.TCPPort <= 0 || p.TCPPort > 65535 || p.HTTPPort <= 0 || p.HTTPPort > 65535 {
			return nil, errors.New("runtime facts advertised endpoint incomplete")
		}
		tcp := net.JoinHostPort(p.BroadcastAddress, strconv.Itoa(p.TCPPort))
		base, e := httpBase(net.JoinHostPort(p.BroadcastAddress, strconv.Itoa(p.HTTPPort)))
		if e != nil || !validTCP(tcp) {
			return nil, errors.New("runtime facts advertised endpoint invalid")
		}
		for _, tombstone := range p.Tombstones {
			if tombstone {
				return nil, errors.New("runtime facts discovery tombstone ambiguous")
			}
		}
		if _, exists := result[tcp]; exists {
			return nil, errors.New("runtime facts duplicate advertised node")
		}
		result[tcp] = base
	}
	return result, nil
}

func explicitPairs(t runtimefacts.TransportFact) (map[string]string, error) {
	result := make(map[string]string)
	if len(t.NSQDTCPAddresses) != len(t.NSQDPairs) || len(t.NSQDHTTPAddresses) != len(t.NSQDPairs) {
		return nil, errors.New("runtime facts TCP HTTP pairing unproven")
	}
	seenHTTP := make(map[string]bool)
	for _, p := range t.NSQDPairs {
		base, e := httpBase(p.HTTPAddress)
		if e != nil || !validTCP(p.TCPAddress) || result[p.TCPAddress] != "" || seenHTTP[base] {
			return nil, errors.New("runtime facts explicit node pair invalid")
		}
		result[p.TCPAddress], seenHTTP[base] = base, true
	}
	return result, nil
}

func requirements(t runtimefacts.TransportFact) map[string][]string {
	topics := make(map[string][]string)
	for _, s := range t.Subscriptions {
		if !contains(topics[s.Topic], s.Channel) {
			topics[s.Topic] = append(topics[s.Topic], s.Channel)
		}
		if s.FailureTopic != "" {
			if !contains(topics[s.FailureTopic], s.FailureChannel) {
				topics[s.FailureTopic] = append(topics[s.FailureTopic], s.FailureChannel)
			}
		}
	}
	for _, topic := range t.PublishTopics {
		if _, ok := topics[topic]; !ok {
			topics[topic] = []string{}
		}
	}
	return topics
}

// discoveryNodes queries every loaded lookupd and both the complete advertised
// node set and each business/failure topic. It never derives HTTP from a TCP
// port. A loaded explicit pair, when present, must agree with advertisement.
func (r *brokerReader) discoveryNodes(ctx context.Context, t runtimefacts.TransportFact) (map[string]string, map[string]map[string]bool, error) {
	byTopic := make(map[string]map[string]bool)
	required := requirements(t)
	if len(t.LookupdAddresses) == 0 {
		nodes, e := r.loadedPairs(ctx, t)
		if e != nil {
			return nil, nil, e
		}
		if len(nodes) == 0 {
			return nil, nil, errors.New("runtime facts original broker pairing missing")
		}
		for topic := range required {
			byTopic[topic] = make(map[string]bool)
			for tcp := range nodes {
				byTopic[topic][tcp] = true
			}
		}
		return nodes, byTopic, nil
	}
	var advertised map[string]string
	for _, address := range t.LookupdAddresses {
		base, e := httpBase(address)
		if e != nil {
			return nil, nil, e
		}
		var all discovery
		if e = r.get(ctx, base, "/nodes", &all); e != nil {
			return nil, nil, e
		}
		current, e := peerPairs(all.Producers)
		if e != nil {
			return nil, nil, e
		}
		if advertised != nil && !reflect.DeepEqual(advertised, current) {
			return nil, nil, errors.New("runtime facts lookupd node sets disagree")
		}
		advertised = current
		for _, topic := range sortedKeys(required) {
			var lookup discovery
			if e = r.get(ctx, base, "/lookup?topic="+url.QueryEscape(topic), &lookup); e != nil {
				return nil, nil, e
			}
			pairs, e := peerPairs(lookup.Producers)
			if e != nil {
				return nil, nil, e
			}
			currentTopic := make(map[string]bool)
			for tcp, http := range pairs {
				if current[tcp] != http {
					return nil, nil, errors.New("runtime facts topic node not advertised")
				}
				currentTopic[tcp] = true
			}
			for _, channel := range required[topic] {
				if !contains(lookup.Channels, channel) {
					return nil, nil, errors.New("runtime facts lookup channel missing")
				}
			}
			if previous, ok := byTopic[topic]; ok && !reflect.DeepEqual(previous, currentTopic) {
				return nil, nil, errors.New("runtime facts lookupd topic sets disagree")
			}
			byTopic[topic] = currentTopic
		}
	}
	if e := validateLoadedAdvertisements(t, advertised); e != nil {
		return nil, nil, e
	}
	return advertised, byTopic, nil
}

// Loaded HTTP addresses are channel-ensure configuration, not a complete
// dynamic lookupd node inventory. Check each against exact advertised pairs,
// while retaining every advertised node for the subsequent /info and /stats.
func validateLoadedAdvertisements(t runtimefacts.TransportFact, advertised map[string]string) error {
	knownHTTP := make(map[string]bool)
	for _, base := range advertised {
		if knownHTTP[base] {
			return errors.New("runtime facts advertised HTTP node identity reused")
		}
		knownHTTP[base] = true
	}
	for _, tcp := range t.NSQDTCPAddresses {
		if advertised[tcp] == "" {
			return errors.New("runtime facts loaded TCP node not advertised")
		}
	}
	for _, address := range t.NSQDHTTPAddresses {
		base, e := httpBase(address)
		if e != nil || !knownHTTP[base] {
			return errors.New("runtime facts loaded HTTP node not advertised")
		}
	}
	for _, pair := range t.NSQDPairs {
		base, e := httpBase(pair.HTTPAddress)
		if e != nil || advertised[pair.TCPAddress] != base {
			return errors.New("runtime facts loaded pair differs from advertisement")
		}
	}
	return nil
}

func (r *brokerReader) collect(ctx context.Context, s runtimefacts.Snapshot) ([]NodeObservation, []string, error) {
	r.infoSeen = make(map[string]brokerInfo)
	nodes := make(map[string]*NodeObservation)
	httpNodes := make(map[string]string)
	transportNodes := make(map[string]map[string]string)
	transportTopics := make(map[string]map[string]map[string]bool)
	var gaps []string
	for _, t := range s.Transports {
		if t.Direction == "no_local_mq" {
			continue
		}
		pairs, topics, e := r.discoveryNodes(ctx, t)
		if e != nil {
			return nil, nil, e
		}
		transportNodes[t.ID], transportTopics[t.ID] = pairs, topics
		for tcp, base := range pairs {
			if old := httpNodes[base]; old != "" && old != tcp {
				return nil, nil, errors.New("runtime facts HTTP node identity reused")
			}
			if n, ok := nodes[tcp]; ok && n.HTTPAddress != base {
				return nil, nil, errors.New("runtime facts node pairing conflicts")
			}
			httpNodes[base] = tcp
			nodes[tcp] = &NodeObservation{TCPAddress: tcp, HTTPAddress: base, Clients: []ClientObservation{}}
		}
		if t.Direction == "publisher" {
			gaps = append(gaps, "publisher_topic_history_not_exhaustive:"+t.ID)
		}
		if t.Direction == "consumer" {
			for _, sub := range t.Subscriptions {
				if sub.FailureTopic != "" {
					gaps = append(gaps, "shared_handoff_connection_unbound:"+t.ID)
					break
				}
			}
		}
	}
	if len(nodes) > 64 {
		return nil, nil, errors.New("runtime facts broker node bound exceeded")
	}
	for _, tcp := range sortedKeys(nodes) {
		node := nodes[tcp]
		node.Topics = []TopicObservation{}
		for _, t := range s.Transports {
			for _, topic := range sortedKeys(transportTopics[t.ID]) {
				if transportTopics[t.ID][topic][tcp] {
					channels := append([]string{}, requirements(t)[topic]...)
					sort.Strings(channels)
					node.Topics = append(node.Topics, TopicObservation{TransportID: t.ID, Topic: topic, Channels: channels})
				}
			}
		}
		sort.Slice(node.Topics, func(i, j int) bool {
			a, b := node.Topics[i], node.Topics[j]
			return a.TransportID+"\x00"+a.Topic < b.TransportID+"\x00"+b.Topic
		})
		if _, e := r.info(ctx, node.HTTPAddress); e != nil {
			return nil, nil, e
		}
		var stats brokerStats
		if e := r.get(ctx, node.HTTPAddress, "/stats?format=json&include_clients=true", &stats); e != nil {
			return nil, nil, e
		}
		if e := validateStats(stats); e != nil {
			return nil, nil, e
		}
		if e := verifyLoadedClientCoverage(stats, s.Transports, transportNodes, transportTopics, tcp); e != nil {
			return nil, nil, e
		}
		info, e := r.info(ctx, node.HTTPAddress)
		if e != nil {
			return nil, nil, e
		}
		if info.tcp() != node.TCPAddress || !info.sameHTTP(node.HTTPAddress) || stats.Version != info.Version || stats.StartTime != info.StartTime {
			return nil, nil, errors.New("runtime facts broker info stats identity changed")
		}
		node.BrokerVersion, node.BrokerStartTime = info.Version, info.StartTime
		for _, t := range s.Transports {
			if transportNodes[t.ID][tcp] == "" {
				continue
			}
			if t.Direction == "publisher" {
				matches, e := matchingClients(stats.Producers, t, "", "", false)
				if e != nil {
					return nil, nil, e
				}
				if len(matches) == 0 {
					return nil, nil, errors.New("runtime facts publisher connection not visible")
				}
				node.Clients = append(node.Clients, matches...)
			} else {
				// A failure handoff may share the driver's ID. Record actual
				// producer connections, but do not claim they are a distinct
				// independently bound transport or require a send to expose it.
				shared, e := matchingClients(stats.Producers, t, "", "", false)
				if e != nil {
					return nil, nil, e
				}
				if len(shared) > 0 && !hasFailureSubscription(t) {
					return nil, nil, errors.New("runtime facts unregistered shared producer identity")
				}
				for i := range shared {
					shared[i].Direction = "shared_handoff_publisher"
				}
				node.Clients = append(node.Clients, shared...)
				for _, topic := range sortedKeys(requirements(t)) {
					// An unrelated advertised node is still read; it must not silently hold
					// this driver's subscription outside its /lookup topic node set.
					relevant := transportTopics[t.ID][topic][tcp]
					found, e := matchingTopic(stats, t, topic, requirements(t)[topic], relevant)
					if e != nil {
						return nil, nil, e
					}
					node.Clients = append(node.Clients, found...)
				}
			}
		}
		sort.Slice(node.Clients, func(i, j int) bool {
			a, _ := json.Marshal(node.Clients[i])
			b, _ := json.Marshal(node.Clients[j])
			return string(a) < string(b)
		})
	}
	sort.Strings(gaps)
	result := make([]NodeObservation, 0, len(nodes))
	for _, tcp := range sortedKeys(nodes) {
		result = append(result, *nodes[tcp])
	}
	return result, gaps, nil
}

func matchingTopic(stats brokerStats, t runtimefacts.TransportFact, topic string, channels []string, required bool) ([]ClientObservation, error) {
	var result []ClientObservation
	var actual *brokerTopic
	for i := range stats.Topics {
		if stats.Topics[i].Name == topic {
			actual = &stats.Topics[i]
		}
	}
	if actual == nil {
		if required {
			return nil, errors.New("runtime facts subscribed topic missing")
		}
		return result, nil
	}
	if actual.Paused == nil || required && *actual.Paused {
		return nil, errors.New("runtime facts subscribed topic paused")
	}
	for _, channel := range channels {
		var actualChannel *brokerChannel
		for i := range actual.Channels {
			if actual.Channels[i].Name == channel {
				actualChannel = &actual.Channels[i]
			}
		}
		if actualChannel == nil {
			if required {
				return nil, errors.New("runtime facts subscribed channel missing")
			}
			continue
		}
		matches, e := matchingClients(actualChannel.Clients, t, topic, channel, true)
		if e != nil {
			return nil, e
		}
		if actualChannel.Paused == nil || required && (*actualChannel.Paused || len(matches) == 0) {
			return nil, errors.New("runtime facts subscribed consumer unavailable")
		}
		if !required && len(matches) > 0 {
			return nil, errors.New("runtime facts consumer on undiscovered topic node")
		}
		result = append(result, matches...)
	}
	return result, nil
}

func matchingClients(clients []brokerClient, t runtimefacts.TransportFact, topic, channel string, consumer bool) ([]ClientObservation, error) {
	var result []ClientObservation
	seen := make(map[string]bool)
	for _, c := range clients {
		if c.ClientID != t.ClientID {
			continue
		}
		if c.Version != "V2" || c.Hostname != t.Hostname || !validTCP(c.RemoteAddress) || c.ConnectTime <= 0 || consumer && c.State != 3 || !consumer && c.State != 2 {
			return nil, errors.New("runtime facts driver connection identity or state conflicts")
		}
		key := c.RemoteAddress + "\x00" + strconv.FormatInt(c.ConnectTime, 10)
		if seen[key] {
			return nil, errors.New("runtime facts driver duplicate connection")
		}
		seen[key] = true
		result = append(result, ClientObservation{TransportID: t.ID, Direction: t.Direction, ClientID: c.ClientID, Hostname: c.Hostname, Topic: topic, Channel: channel, RemoteAddress: c.RemoteAddress, ConnectTime: c.ConnectTime})
	}
	return result, nil
}

// Scan all topic/channel rows for every loaded ID, including nodes discovered
// by another transport. A matching ID outside its registered subscription and
// lookup topic-node set is an unresolved responsibility, never an omitted row.
func verifyLoadedClientCoverage(stats brokerStats, transports []runtimefacts.TransportFact, nodes map[string]map[string]string, topics map[string]map[string]map[string]bool, tcp string) error {
	for _, t := range transports {
		if t.Direction == "no_local_mq" {
			continue
		}
		for _, producer := range stats.Producers {
			if producer.ClientID == t.ClientID && nodes[t.ID][tcp] == "" {
				return errors.New("runtime facts loaded producer on unregistered node")
			}
		}
		required := requirements(t)
		for _, topic := range stats.Topics {
			for _, channel := range topic.Channels {
				for _, client := range channel.Clients {
					if client.ClientID != t.ClientID {
						continue
					}
					if t.Direction != "consumer" || nodes[t.ID][tcp] == "" || !topics[t.ID][topic.Name][tcp] || !contains(required[topic.Name], channel.Name) {
						return errors.New("runtime facts loaded consumer outside registered subscription scope")
					}
				}
			}
		}
	}
	return nil
}

func validateStats(s brokerStats) error {
	if s.Health != "OK" || s.Topics == nil || len(s.Topics) > 4096 || len(s.Producers) > 4096 {
		return errors.New("runtime facts broker client inventory incomplete")
	}
	topics := make(map[string]bool)
	for _, t := range s.Topics {
		if topics[t.Name] || t.Name == "" || t.Paused == nil || t.Channels == nil || len(t.Channels) > 4096 {
			return errors.New("runtime facts broker topic inventory ambiguous")
		}
		topics[t.Name] = true
		channels := make(map[string]bool)
		for _, c := range t.Channels {
			if c.Name == "" || channels[c.Name] || c.Paused == nil || c.Clients == nil || c.ClientCount != len(c.Clients) || len(c.Clients) > 4096 {
				return errors.New("runtime facts broker channel clients incomplete")
			}
			channels[c.Name] = true
		}
	}
	return nil
}
func sortedKeys[V any](m map[string]V) []string {
	result := make([]string, 0, len(m))
	for key := range m {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
func contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}

func hasFailureSubscription(t runtimefacts.TransportFact) bool {
	for _, sub := range t.Subscriptions {
		if sub.FailureTopic != "" {
			return true
		}
	}
	return false
}

// NSQD v1.3.0 /info returns actual TCP/HTTP listener ports and its configured
// broadcast address. These exact returned values, not array order or port
// arithmetic, may establish a missing loaded TCP/HTTP pairing. Listener-port
// aliases, TLS/reverse proxies, missing fields and duplicates remain unproven.
func (r *brokerReader) loadedPairs(ctx context.Context, t runtimefacts.TransportFact) (map[string]string, error) {
	if len(t.NSQDPairs) > 0 || len(t.NSQDTCPAddresses)+len(t.NSQDHTTPAddresses) == 0 {
		return explicitPairs(t)
	}
	if len(t.NSQDTCPAddresses) == 0 || len(t.NSQDHTTPAddresses) == 0 {
		return nil, errors.New("runtime facts original broker HTTP pairing unavailable")
	}
	result := make(map[string]string)
	for _, address := range t.NSQDHTTPAddresses {
		base, e := httpBase(address)
		if e != nil {
			return nil, e
		}
		info, e := r.info(ctx, base)
		if e != nil {
			return nil, e
		}
		tcp := info.tcp()
		if !contains(t.NSQDTCPAddresses, tcp) || !info.sameHTTP(base) || result[tcp] != "" {
			return nil, errors.New("runtime facts loaded info pairing not unique")
		}
		result[tcp] = base
	}
	if len(result) != len(t.NSQDTCPAddresses) {
		return nil, errors.New("runtime facts loaded TCP pairing incomplete")
	}
	return result, nil
}
func (r *brokerReader) info(ctx context.Context, base string) (brokerInfo, error) {
	var info brokerInfo
	if e := r.get(ctx, base, "/info", &info); e != nil {
		return info, e
	}
	if info.Version != "1.3.0" || info.StartTime <= 0 || info.BroadcastAddress == "" || info.TCPPort <= 0 || info.TCPPort > 65535 || info.HTTPPort <= 0 || info.HTTPPort > 65535 || !validTCP(info.tcp()) {
		return info, errors.New("runtime facts NSQD info schema or identity unsupported")
	}
	if prior, ok := r.infoSeen[base]; ok && prior != info {
		return info, errors.New("runtime facts original broker identity changed")
	}
	if r.infoSeen == nil {
		r.infoSeen = make(map[string]brokerInfo)
	}
	r.infoSeen[base] = info
	return info, nil
}
func (i brokerInfo) tcp() string {
	return net.JoinHostPort(i.BroadcastAddress, strconv.Itoa(i.TCPPort))
}
func (i brokerInfo) sameHTTP(base string) bool {
	u, e := url.Parse(base)
	return e == nil && u.Host == net.JoinHostPort(i.BroadcastAddress, strconv.Itoa(i.HTTPPort))
}
