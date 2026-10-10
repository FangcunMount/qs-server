package runtimefactsreader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/runtimefacts"
)

func fixtureSnapshot() runtimefacts.Snapshot {
	return runtimefacts.Snapshot{FormatVersion: runtimefacts.SnapshotVersion, Component: "worker", SourceSHA: strings.Repeat("a", 40), ProcessNonce: strings.Repeat("b", 64), Hostname: "owned-worker", Process: runtimefacts.ProcessIdentity{PID: 1, UID: 501, StartTimeTicks: 12, BootID: "12345678-1234-1234-1234-123456789abc"}, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Lifecycle: "started", ObservationComplete: true, IncompleteReasons: []string{}, Transports: []runtimefacts.TransportFact{{Transport: runtimefacts.Transport{ID: "standard-consumer", Provider: "nsq", Direction: "consumer", LookupdAddresses: []string{}, NSQDTCPAddresses: []string{}, NSQDHTTPAddresses: []string{}, NSQDPairs: []runtimefacts.NSQDPair{}, PublishTopics: []string{}, ClientID: "qs-" + strings.Repeat("c", 64), Hostname: "owned-worker"}, State: "started", Subscriptions: []runtimefacts.Subscription{{Topic: "business", Channel: "worker", FailureTopic: "failed", FailureChannel: "failure"}}}}}
}
func responseBytes(t *testing.T, s runtimefacts.Snapshot) []byte {
	t.Helper()
	b, e := json.Marshal(runtimefacts.Response{FormatVersion: runtimefacts.QueryVersion, Challenge: strings.Repeat("e", 64), Snapshot: s})
	if e != nil {
		t.Fatal(e)
	}
	return append(b, '\n')
}

func TestResponseCompletenessAndClosedProtocol(t *testing.T) {
	s := fixtureSnapshot()
	s.Transports[0].LookupdAddresses = []string{"lookupd.internal:4161"}
	raw := responseBytes(t, s)
	if _, e := decodeResponse(raw, strings.Repeat("e", 64)); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name      string
		raw       []byte
		challenge string
	}{
		{"wrong-challenge", raw, strings.Repeat("f", 64)},
		{"no-eof-line", raw[:len(raw)-1], strings.Repeat("e", 64)},
		{"trailing-json", append(append([]byte{}, raw...), []byte("{}")...), strings.Repeat("e", 64)},
		{"duplicate-nested", []byte(strings.Replace(string(raw), `"uid":501`, `"uid":501,"uid":501`, 1)), strings.Repeat("e", 64)},
		{"omitted-false-field", []byte(strings.Replace(string(raw), `"broker_connections_verified":false,`, "", 1)), strings.Repeat("e", 64)},
		{"unknown-field", []byte(strings.Replace(string(raw), `"snapshot":{`, `"snapshot":{"authority":true,`, 1)), strings.Repeat("e", 64)},
		{"oversized", append(make([]byte, queryLimit), '\n'), strings.Repeat("e", 64)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, e := decodeResponse(tc.raw, tc.challenge); e == nil {
				t.Fatal("ambiguous response accepted")
			}
		})
	}
	for _, tc := range []struct {
		name   string
		change func(*runtimefacts.Snapshot)
	}{
		{"incomplete", func(s *runtimefacts.Snapshot) { s.ObservationComplete = false }},
		{"broker-self-proof", func(s *runtimefacts.Snapshot) { s.BrokerConnectionsVerified = true }},
		{"not-pid1", func(s *runtimefacts.Snapshot) { s.Process.PID = 8 }},
		{"stopped", func(s *runtimefacts.Snapshot) { s.Transports[0].State = "stopped" }},
		{"empty-consumer", func(s *runtimefacts.Snapshot) { s.Transports[0].Subscriptions = nil }},
		{"bad-failure-pair", func(s *runtimefacts.Snapshot) { s.Transports[0].Subscriptions[0].FailureChannel = "" }},
		{"reused-identity", func(s *runtimefacts.Snapshot) {
			other := s.Transports[0]
			other.ID = "other"
			s.Transports = append(s.Transports, other)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := s
			copy.Transports = append([]runtimefacts.TransportFact(nil), s.Transports...)
			copy.Transports[0].Subscriptions = append([]runtimefacts.Subscription(nil), s.Transports[0].Subscriptions...)
			tc.change(&copy)
			if _, e := decodeResponse(responseBytes(t, copy), strings.Repeat("e", 64)); e == nil {
				t.Fatal("incomplete snapshot accepted")
			}
		})
	}
}

type brokerFixture struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []string
	calls    map[string]int
	mutation string
	id       string
}

func newBrokerFixture(t *testing.T, mutation string) *brokerFixture {
	t.Helper()
	f := &brokerFixture{calls: map[string]int{}, mutation: mutation, id: "qs-" + strings.Repeat("c", 64)}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests = append(f.requests, r.Method+" "+r.URL.RequestURI())
		f.calls[r.URL.Path]++
		if r.Method != "GET" {
			t.Error("non-read method")
			w.WriteHeader(405)
			return
		}
		if f.mutation == "redirect" {
			w.Header().Set("Location", "http://127.0.0.1:1/stats")
			w.WriteHeader(302)
			return
		}
		if f.mutation == "status-error" {
			w.WriteHeader(403)
			return
		}
		if f.mutation == "oversized" {
			_, _ = w.Write([]byte(strings.Repeat("x", brokerBodyLimit+1)))
			return
		}
		peer := map[string]any{"broadcast_address": "127.0.0.1", "tcp_port": 14150, "http_port": mustPort(f.server.URL), "hostname": "broker", "version": "1.3.0"}
		switch r.URL.Path {
		case "/info":
			info := map[string]any{"broadcast_address": "127.0.0.1", "tcp_port": 14150, "http_port": mustPort(f.server.URL), "version": "1.3.0", "start_time": 100}
			if f.mutation == "missing-info-port" {
				delete(info, "http_port")
			}
			if f.mutation == "info-wrong-port" {
				info["tcp_port"] = 14149
			}
			if f.mutation == "info-unsupported" {
				info["version"] = "2.0.0"
			}
			if f.mutation == "info-restart" && f.calls["/info"] > 1 {
				info["start_time"] = 101
			}
			_ = json.NewEncoder(w).Encode(info)
		case "/nodes":
			_ = json.NewEncoder(w).Encode(map[string]any{"producers": []any{peer}})
		case "/lookup":
			if f.mutation == "unknown-node" {
				peer["http_port"] = 14152
			}
			channels := []string{"worker"}
			if r.URL.Query().Get("topic") == "failed" {
				channels = []string{"failure"}
			}
			if f.mutation == "missing-channel" {
				channels = []string{}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"channels": channels, "producers": []any{peer}, "timestamp": 1})
		case "/stats":
			client := map[string]any{"version": "V2", "client_id": f.id, "hostname": "owned-worker", "remote_address": "127.0.0.1:20123", "connect_ts": 123, "state": 3, "message_count": f.calls["/stats"]}
			if f.mutation == "wrong-hostname" {
				client["hostname"] = "foreign"
			}
			if f.mutation == "connection-churn" && f.calls["/stats"] > 1 {
				client["connect_ts"] = 124
			}
			clients := []any{client}
			if f.mutation == "missing-client" {
				clients = []any{}
			}
			count := len(clients)
			if f.mutation == "incomplete-client-count" {
				count++
			}
			channel := func(name string) any {
				row := map[string]any{"channel_name": name, "clients": clients, "client_count": count, "paused": f.mutation == "paused"}
				switch f.mutation {
				case "missing-channel-paused":
					delete(row, "paused")
				case "null-channel-paused":
					row["paused"] = nil
				case "invalid-channel-paused":
					row["paused"] = "false"
				}
				return row
			}
			topics := []any{map[string]any{"topic_name": "business", "channels": []any{channel("worker")}, "paused": false}, map[string]any{"topic_name": "failed", "channels": []any{channel("failure")}, "paused": false}}
			if f.mutation == "missing-failure-topic" {
				topics = topics[:1]
			}
			switch f.mutation {
			case "missing-topic-paused":
				delete(topics[0].(map[string]any), "paused")
			case "null-topic-paused":
				topics[0].(map[string]any)["paused"] = nil
			case "invalid-topic-paused":
				topics[0].(map[string]any)["paused"] = "false"
			case "loaded-id-extra-topic":
				topics = append(topics, map[string]any{"topic_name": "unregistered", "channels": []any{channel("foreign")}, "paused": false})
			case "loaded-id-extra-channel":
				topics[0].(map[string]any)["channels"] = []any{channel("worker"), channel("unregistered")}
			case "unrelated-topic-unknown-paused":
				topics = append(topics, map[string]any{"topic_name": "unrelated", "channels": []any{}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"topics": topics, "producers": []any{}, "version": "1.3.0", "health": "OK", "start_time": 100})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}
func mustPort(base string) int {
	var port int
	_, _ = fmt.Sscanf(base, "http://127.0.0.1:%d", &port)
	return port
}
func fixtureQuery(s runtimefacts.Snapshot) func(context.Context) (runtimefacts.Snapshot, NativeIdentity, error) {
	return func(context.Context) (runtimefacts.Snapshot, NativeIdentity, error) {
		copy := s
		copy.ObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return copy, NativeIdentity{HostPID: 91, UID: 501, StartTimeTicks: 12}, nil
	}
}
func directClient(f *brokerFixture) *http.Client {
	return &http.Client{Transport: &http.Transport{Proxy: nil}}
}

func TestWholeAdvertisedLocalReadAndHardScopeGaps(t *testing.T) {
	f := newBrokerFixture(t, "")
	s := fixtureSnapshot()
	s.Transports[0].LookupdAddresses = []string{f.server.URL}
	result, e := observe(context.Background(), fixtureQuery(s), directClient(f))
	if !errors.Is(e, ErrScopeUnproven) || len(result.Nodes) != 1 || len(result.Nodes[0].Clients) != 2 || !reflect.DeepEqual(result.Gaps, []string{"shared_handoff_connection_unbound:standard-consumer", "external_qs_ai_not_locally_proven"}) {
		t.Fatalf("bounded local observation incomplete: %+v %v", result, e)
	}
	for _, path := range []string{"/nodes", "/lookup", "/stats"} {
		if f.calls[path] != map[string]int{"/nodes": 2, "/lookup": 4, "/stats": 2}[path] {
			t.Fatalf("two complete passes missing %s: %+v", path, f.calls)
		}
	}
	if result.Snapshot.BrokerConnectionsVerified {
		t.Fatal("source bool escalated")
	}
	for _, r := range f.requests {
		if strings.Contains(r, "create") || strings.Contains(r, "pub?") {
			t.Fatal("writer endpoint used")
		}
	}
}
func TestBrokerFailuresCannotQualify(t *testing.T) {
	for _, name := range []string{"wrong-hostname", "missing-client", "missing-failure-topic", "missing-channel", "incomplete-client-count", "unknown-node", "connection-churn", "paused", "redirect", "status-error", "oversized", "missing-info-port", "info-wrong-port", "info-unsupported", "info-restart", "missing-topic-paused", "null-topic-paused", "invalid-topic-paused", "missing-channel-paused", "null-channel-paused", "invalid-channel-paused", "unrelated-topic-unknown-paused", "loaded-id-extra-topic", "loaded-id-extra-channel"} {
		t.Run(name, func(t *testing.T) {
			f := newBrokerFixture(t, name)
			s := fixtureSnapshot()
			s.Transports[0].LookupdAddresses = []string{f.server.URL}
			result, e := observe(context.Background(), fixtureQuery(s), directClient(f))
			if e == nil || errors.Is(e, ErrScopeUnproven) || len(result.Nodes) != 0 {
				t.Fatalf("failed broker observation qualified: %+v %v", result, e)
			}
		})
	}
}
func TestActualSnapshotChangedAfterBrokerRead(t *testing.T) {
	f := newBrokerFixture(t, "")
	s := fixtureSnapshot()
	s.Transports[0].LookupdAddresses = []string{f.server.URL}
	calls := 0
	query := func(context.Context) (runtimefacts.Snapshot, NativeIdentity, error) {
		calls++
		next := s
		if calls > 1 {
			next.ProcessNonce = strings.Repeat("d", 64)
		}
		return next, NativeIdentity{}, nil
	}
	if _, e := observe(context.Background(), query, directClient(f)); e == nil || errors.Is(e, ErrScopeUnproven) {
		t.Fatal("changed process accepted")
	}
}
func TestExplicitAddressPairAndMultipleConnections(t *testing.T) {
	fact := fixtureSnapshot().Transports[0]
	fact.NSQDTCPAddresses = []string{"127.0.0.1:14150"}
	fact.NSQDHTTPAddresses = []string{"http://127.0.0.1:14151"}
	if _, e := explicitPairs(fact); e == nil {
		t.Fatal("unregistered pairing guessed")
	}
	fact.NSQDPairs = []runtimefacts.NSQDPair{{TCPAddress: fact.NSQDTCPAddresses[0], HTTPAddress: fact.NSQDHTTPAddresses[0]}}
	if _, e := explicitPairs(fact); e != nil {
		t.Fatal(e)
	}
	clients := []brokerClient{{Version: "V2", ClientID: fact.ClientID, Hostname: fact.Hostname, RemoteAddress: "127.0.0.1:30001", ConnectTime: 12, State: 3}, {Version: "V2", ClientID: fact.ClientID, Hostname: fact.Hostname, RemoteAddress: "127.0.0.1:30002", ConnectTime: 13, State: 3}}
	if c, e := matchingClients(clients, fact, "topic", "channel", true); e != nil || len(c) != 2 {
		t.Fatal("actual shared identity multiplicity suppressed", e)
	}
	clients[1] = clients[0]
	if _, e := matchingClients(clients, fact, "topic", "channel", true); e == nil {
		t.Fatal("duplicate broker connection accepted")
	}
}
func TestCancellationAndOriginalHTTPTransport(t *testing.T) {
	f := newBrokerFixture(t, "")
	s := fixtureSnapshot()
	s.Transports[0].LookupdAddresses = []string{f.server.URL}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := observe(ctx, fixtureQuery(s), directClient(f)); e == nil || errors.Is(e, ErrScopeUnproven) {
		t.Fatal("cancelled observation accepted")
	}
	for _, c := range []*http.Client{{}, {Transport: http.DefaultTransport}, {Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}}} {
		if _, e := newBrokerReader(c); e == nil {
			t.Fatal("unbound or proxy transport accepted")
		}
	}
}

func TestEveryLookupdAndAdvertisedNSQDRead(t *testing.T) {
	for _, tc := range []struct {
		name string
		fail bool
	}{
		{"lookup-only", false},
		{"http-only-subset", false},
		{"http-only-second-node-missing-client", true},
		{"http-only-missing-advertisement", true},
		{"loaded-pair-conflict", true},
		{"advertised-http-conflict", true},
		{"http-only-second-node-extra-topic", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var servers [2]*httptest.Server
			var mu sync.Mutex
			counts := map[string]int{}
			for i := range servers {
				index := i
				servers[i] = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					defer mu.Unlock()
					counts[fmt.Sprintf("%d%s", index, r.URL.Path)]++
					if r.Method != http.MethodGet {
						t.Error("broker mutation")
						w.WriteHeader(405)
						return
					}
					peers := []any{}
					for node := range servers {
						port := mustPort(servers[node].URL)
						if tc.name == "advertised-http-conflict" {
							port = mustPort(servers[0].URL)
						}
						peers = append(peers, map[string]any{"broadcast_address": "127.0.0.1", "tcp_port": 15000 + node, "http_port": port})
					}
					switch r.URL.Path {
					case "/info":
						_ = json.NewEncoder(w).Encode(map[string]any{"broadcast_address": "127.0.0.1", "tcp_port": 15000 + index, "http_port": mustPort(servers[index].URL), "version": "1.3.0", "start_time": 100})
					case "/nodes":
						_ = json.NewEncoder(w).Encode(map[string]any{"producers": peers})
					case "/lookup":
						_ = json.NewEncoder(w).Encode(map[string]any{"channels": []string{"worker", "failure"}, "producers": peers})
					case "/stats":
						clients := []any{map[string]any{"version": "V2", "client_id": "qs-" + strings.Repeat("c", 64), "hostname": "owned-worker", "remote_address": fmt.Sprintf("127.0.0.1:%d", 16000+index), "connect_ts": 123, "state": 3}}
						if tc.name == "http-only-second-node-missing-client" && index == 1 {
							clients = []any{}
						}
						topic := func(name, channel string) any {
							return map[string]any{"topic_name": name, "paused": false, "channels": []any{map[string]any{"channel_name": channel, "paused": false, "clients": clients, "client_count": len(clients)}}}
						}
						topics := []any{topic("business", "worker"), topic("failed", "failure")}
						if tc.name == "http-only-second-node-extra-topic" && index == 1 {
							topics = append(topics, topic("unregistered", "foreign"))
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"topics": topics, "producers": nil, "version": "1.3.0", "start_time": 100, "health": "OK"})
					default:
						w.WriteHeader(404)
					}
				}))
			}
			for _, server := range servers {
				server.Start()
				t.Cleanup(server.Close)
			}
			s := fixtureSnapshot()
			s.Transports[0].LookupdAddresses = []string{servers[0].URL, servers[1].URL}
			if tc.name != "lookup-only" {
				// The actual SDK consumer loads lookupd and HTTP channel-ensure
				// endpoints, without a TCP list. A subset does not hide node 2.
				s.Transports[0].NSQDHTTPAddresses = []string{servers[0].URL}
			}
			if tc.name == "http-only-missing-advertisement" {
				s.Transports[0].NSQDHTTPAddresses = []string{"http://127.0.0.1:1"}
			}
			if tc.name == "loaded-pair-conflict" {
				s.Transports[0].NSQDTCPAddresses = []string{"127.0.0.1:15001"}
				s.Transports[0].NSQDPairs = []runtimefacts.NSQDPair{{TCPAddress: "127.0.0.1:15001", HTTPAddress: servers[0].URL}}
			}
			client := &http.Client{Transport: &http.Transport{Proxy: nil}}
			actual, e := observe(context.Background(), fixtureQuery(s), client)
			if tc.fail {
				if e == nil || errors.Is(e, ErrScopeUnproven) || len(actual.Nodes) != 0 {
					t.Fatal("failed second advertised node hidden", e)
				}
				return
			}
			if !errors.Is(e, ErrScopeUnproven) || len(actual.Nodes) != 2 {
				t.Fatal("two-node scope missing", e)
			}
			for i := range servers {
				for _, path := range []string{"/nodes", "/lookup", "/stats"} {
					want := 2
					if path == "/lookup" {
						want = 4
					}
					if counts[fmt.Sprintf("%d%s", i, path)] != want {
						t.Fatal("one endpoint omitted", counts)
					}
				}
			}
		})
	}
}

func TestIdlePublisherCannotBeExposedBySyntheticSend(t *testing.T) {
	f := newBrokerFixture(t, "")
	s := fixtureSnapshot()
	fact := &s.Transports[0]
	fact.Direction = "publisher"
	fact.Subscriptions = []runtimefacts.Subscription{}
	fact.NSQDTCPAddresses = []string{"127.0.0.1:14150"}
	fact.NSQDHTTPAddresses = []string{f.server.URL}
	fact.NSQDPairs = []runtimefacts.NSQDPair{{TCPAddress: fact.NSQDTCPAddresses[0], HTTPAddress: f.server.URL}}
	if _, e := observe(context.Background(), fixtureQuery(s), directClient(f)); e == nil || errors.Is(e, ErrScopeUnproven) {
		t.Fatal("idle publisher accepted despite missing broker connection")
	}
	if f.calls["/stats"] != 1 {
		t.Fatal("observation tried to manufacture a publisher", f.requests)
	}
	for _, r := range f.requests {
		if !strings.HasPrefix(r, "GET /stats?") && r != "GET /info" {
			t.Fatal("non-observation request", r)
		}
	}
}

func TestChallengeIsFreshBoundedHex(t *testing.T) {
	a, e := newChallenge()
	if e != nil {
		t.Fatal(e)
	}
	b, e := newChallenge()
	if e != nil || !validHex(a, 64) || !validHex(b, 64) || a == b {
		t.Fatal("challenge not fresh bounded hex")
	}
}
func TestInfoBindsLoadedPairsWithoutPortGuess(t *testing.T) {
	f := newBrokerFixture(t, "")
	s := fixtureSnapshot()
	fact := s.Transports[0]
	fact.NSQDTCPAddresses = []string{"127.0.0.1:14150"}
	fact.NSQDHTTPAddresses = []string{f.server.URL}
	r, e := newBrokerReader(directClient(f))
	if e != nil {
		t.Fatal(e)
	}
	actual, e := r.loadedPairs(context.Background(), fact)
	if e != nil || actual[fact.NSQDTCPAddresses[0]] != f.server.URL {
		t.Fatal("actual info pair not bound", e)
	}
	wrong := fact
	wrong.NSQDTCPAddresses = []string{"127.0.0.1:14149"}
	if _, e = r.loadedPairs(context.Background(), wrong); e == nil {
		t.Fatal("adjacent TCP port guessed")
	}
	duplicate := fact
	duplicate.NSQDHTTPAddresses = append(append([]string{}, fact.NSQDHTTPAddresses...), f.server.URL)
	if _, e = r.loadedPairs(context.Background(), duplicate); e == nil {
		t.Fatal("duplicate endpoint pair accepted")
	}
}

func TestLoadedIDCoverageIncludesOtherTransportNodes(t *testing.T) {
	fact := fixtureSnapshot().Transports[0]
	paused := false
	client := brokerClient{ClientID: fact.ClientID}
	for _, tc := range []struct {
		name  string
		stats brokerStats
	}{
		{"consumer-other-node", brokerStats{Topics: []brokerTopic{{Name: "business", Paused: &paused, Channels: []brokerChannel{{Name: "worker", Paused: &paused, Clients: []brokerClient{client}}}}}}},
		{"producer-other-node", brokerStats{Producers: []brokerClient{client}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nodes := map[string]map[string]string{fact.ID: {"127.0.0.1:14150": "http://127.0.0.1:14151"}}
			topics := map[string]map[string]map[string]bool{fact.ID: {"business": {"127.0.0.1:14150": true}}}
			if e := verifyLoadedClientCoverage(tc.stats, []runtimefacts.TransportFact{fact}, nodes, topics, "127.0.0.1:14160"); e == nil {
				t.Fatal("loaded ID on another transport's node was hidden")
			}
		})
	}
}
