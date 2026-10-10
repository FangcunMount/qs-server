package runtimefacts

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"time"
)

const maxQueryBytes = 1024
const maxResponseBytes = 256 * 1024

type Query struct {
	FormatVersion string `json:"format_version"`
	Action        string `json:"action"`
	Challenge     string `json:"challenge"`
}

type Response struct {
	FormatVersion string   `json:"format_version"`
	Challenge     string   `json:"challenge"`
	Snapshot      Snapshot `json:"snapshot"`
}

func (o *Owner) serve(listener *net.UnixListener) {
	defer o.wg.Done()
	for {
		conn, err := listener.AcceptUnix()
		if err != nil {
			o.mu.Lock()
			if !o.closed {
				o.started = false
				o.qualification = append(o.qualification, "private_channel_accept_failed")
			}
			o.mu.Unlock()
			return
		}
		o.mu.Lock()
		if o.closed || len(o.connections) >= 8 {
			o.mu.Unlock()
			_ = conn.Close()
			continue
		}
		o.connections[conn] = struct{}{}
		o.wg.Add(1)
		o.mu.Unlock()
		go o.serveQuery(conn)
	}
}

func (o *Owner) serveQuery(conn *net.UnixConn) {
	defer o.wg.Done()
	defer func() { _ = conn.Close(); o.mu.Lock(); delete(o.connections, conn); o.mu.Unlock() }()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return
	}
	if err := verifyNativePeer(conn, o.identity.UID); err != nil {
		return
	}
	// The caller half-closes its write side. Requiring EOF makes a second query
	// or any trailing bytes invalid, without an unbounded read or parser buffer.
	raw, err := io.ReadAll(io.LimitReader(conn, maxQueryBytes+1))
	if err != nil || len(raw) > maxQueryBytes {
		return
	}
	query, err := decodeQuery(raw)
	if err != nil {
		return
	}
	response := Response{FormatVersion: QueryVersion, Challenge: query.Challenge, Snapshot: o.Snapshot()}
	encoded, err := json.Marshal(response)
	if err != nil || len(encoded)+1 > maxResponseBytes {
		return
	}
	encoded = append(encoded, '\n')
	for len(encoded) != 0 {
		n, err := conn.Write(encoded)
		if err != nil || n <= 0 {
			return
		}
		encoded = encoded[n:]
	}
}

func decodeQuery(raw []byte) (Query, error) {
	var query Query
	if len(raw) == 0 || len(raw) > maxQueryBytes || raw[len(raw)-1] != '\n' || bytes.Count(raw, []byte{'\n'}) != 1 {
		return query, errors.New("invalid runtime facts query boundary")
	}
	for _, c := range raw {
		if c > 126 || (c < 32 && c != '\n') {
			return query, errors.New("invalid runtime facts query encoding")
		}
	}
	// Duplicate keys are invalid, even if encoding/json would keep the last.
	decoder := json.NewDecoder(bytes.NewReader(raw[:len(raw)-1]))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return query, errors.New("invalid runtime facts query object")
	}
	values := make(map[string]string, 3)
	for decoder.More() {
		token, err = decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || (key != "format_version" && key != "action" && key != "challenge") {
			return query, errors.New("invalid runtime facts query field")
		}
		if _, duplicate := values[key]; duplicate {
			return query, errors.New("duplicate runtime facts query field")
		}
		var value string
		if decoder.Decode(&value) != nil {
			return query, errors.New("invalid runtime facts query value")
		}
		values[key] = value
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') || len(values) != 3 {
		return query, errors.New("invalid runtime facts query shape")
	}
	if _, err = decoder.Token(); err != io.EOF {
		return query, errors.New("trailing runtime facts query")
	}
	query = Query{FormatVersion: values["format_version"], Action: values["action"], Challenge: values["challenge"]}
	if query.FormatVersion != QueryVersion || query.Action != "GET" || !validHex(query.Challenge, 64) {
		return query, errors.New("invalid runtime facts query binding")
	}
	return query, nil
}
