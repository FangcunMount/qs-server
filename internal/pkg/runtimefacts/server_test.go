package runtimefacts

import (
	"encoding/json"
	"strings"
	"testing"
)

func queryLine(t *testing.T) []byte {
	t.Helper()
	raw, err := json.Marshal(Query{FormatVersion: QueryVersion, Action: "GET", Challenge: strings.Repeat("b", 64)})
	if err != nil {
		t.Fatal(err)
	}
	return append(raw, '\n')
}

func TestQueryStrictSingleBoundedObject(t *testing.T) {
	valid := queryLine(t)
	query, err := decodeQuery(valid)
	if err != nil || query.Challenge != strings.Repeat("b", 64) {
		t.Fatal("valid query rejected")
	}
	tests := map[string][]byte{
		"missing-newline":     valid[:len(valid)-1],
		"second-line":         append(append([]byte{}, valid...), valid...),
		"extra-key":           []byte(`{"format_version":"qs-runtime-facts-query/v1","action":"GET","challenge":"` + strings.Repeat("b", 64) + `","path":"/anything"}` + "\n"),
		"duplicate-key":       []byte(`{"format_version":"qs-runtime-facts-query/v1","action":"DELETE","action":"GET","challenge":"` + strings.Repeat("b", 64) + `"}` + "\n"),
		"other-action":        []byte(strings.Replace(string(valid), "GET", "DROP", 1)),
		"wrong-version":       []byte(strings.Replace(string(valid), "query/v1", "query/v2", 1)),
		"uppercase-challenge": []byte(strings.Replace(string(valid), strings.Repeat("b", 64), strings.Repeat("B", 64), 1)),
		"short-challenge":     []byte(strings.Replace(string(valid), strings.Repeat("b", 64), "abc", 1)),
		"nonstring":           []byte(`{"format_version":"qs-runtime-facts-query/v1","action":"GET","challenge":null}` + "\n"),
		"array":               []byte("[]\n"),
		"tail":                append(append([]byte{}, valid[:len(valid)-1]...), []byte(" {}\n")...),
		"too-large":           []byte(strings.Repeat(" ", maxQueryBytes) + string(valid)),
		"control":             []byte(strings.Replace(string(valid), "{", "{\t", 1)),
		"non-ascii":           []byte(strings.Replace(string(valid), "{", "{é", 1)),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeQuery(raw); err == nil {
				t.Fatal("invalid query accepted")
			}
		})
	}
}
