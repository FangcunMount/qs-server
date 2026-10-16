package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
)

type graphClearanceNamespaceKey struct{}

// Only the readonly clearance entry, after original capture/apply proof and
// known-ACK journal validation, enables this comparison. Archived bytes stay raw.
func graphClearanceNamespaceContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, graphClearanceNamespaceKey{}, true)
}

func graphClearanceSameNamespace(ctx context.Context, store string, a, b []byte) bool {
	if bytes.Equal(a, b) {
		return true
	}
	enabled, _ := ctx.Value(graphClearanceNamespaceKey{}).(bool)
	if !enabled || store != "mysql" {
		return false
	}
	var left, right []string
	if json.Unmarshal(a, &left) != nil || json.Unmarshal(b, &right) != nil || len(left) != 4 || len(right) != 4 || !bytes.Equal(a, encode(left)) || !bytes.Equal(b, encode(right)) || left[1] != right[1] || left[2] != right[2] || left[3] != right[3] {
		return false
	}
	x, ok := graphClearanceTableOptions(left[0], left[3])
	if !ok {
		return false
	}
	y, ok := graphClearanceTableOptions(right[0], right[3])
	return ok && x == y
}

func graphClearanceSameRecord(ctx context.Context, a, b record) bool {
	if sameRecord(a, b) {
		return true
	}
	if !graphClearanceSameNamespace(ctx, a.Object.Store, a.Definition, b.Definition) {
		return false
	}
	// Compare the complete original key/body under the already-compared schema.
	// This local copy does not alter either captured bytes or its raw fingerprint.
	a.Definition = b.Definition
	return sameRecord(a, b)
}

// SHOW CREATE TABLE's body is quote/parenthesis delimited. Only one bare
// AUTO_INCREMENT=<digits> token in its final InnoDB table-options is mutable.
// Quoted column defaults/comments and table comments are never token candidates.
func graphClearanceTableOptions(ddl, mode string) (string, bool) {
	if !strings.HasPrefix(ddl, "CREATE TABLE `") {
		return "", false
	}
	noBackslash := false
	for _, part := range strings.Split(mode, ",") {
		noBackslash = noBackslash || part == "NO_BACKSLASH_ESCAPES"
	}
	quote, depth, bodyEnd := byte(0), 0, -1
	start, removeStart, removeEnd := -1, -1, -1
	finish := func(end int) bool {
		if start < 0 {
			return true
		}
		token := ddl[start:end]
		if strings.HasPrefix(token, "AUTO_INCREMENT=") {
			value := strings.TrimPrefix(token, "AUTO_INCREMENT=")
			if removeStart >= 0 || value == "" {
				return false
			}
			for _, c := range value {
				if c < '0' || c > '9' {
					return false
				}
			}
			removeStart, removeEnd = start-1, end
		}
		start = -1
		return true
	}
	for i := 0; i < len(ddl); i++ {
		c := ddl[i]
		if quote != 0 {
			if c == '\\' && quote != '`' && !noBackslash {
				i++
				if i >= len(ddl) {
					return "", false
				}
			} else if c == quote {
				if i+1 < len(ddl) && ddl[i+1] == quote {
					i++
				} else {
					quote = 0
				}
			}
			continue
		}
		if c == '/' {
			return "", false // no interpretation of version comments or expressions.
		}
		if c == '\'' || c == '"' || c == '`' {
			quote = c
		} else if bodyEnd < 0 {
			switch c {
			case '(':
				depth++
			case ')':
				depth--
				if depth < 0 {
					return "", false
				}
				if depth == 0 {
					bodyEnd = i
					if !strings.HasPrefix(ddl[i:], ") ENGINE=InnoDB ") {
						return "", false
					}
				}
			}
		} else if c == '/' || c == '\n' || c == '\r' || c == '\t' {
			return "", false // unsupported comments/option layout, retain strict comparison.
		}
		if bodyEnd >= 0 && i > bodyEnd {
			if c == ' ' {
				if !finish(i) {
					return "", false
				}
			} else if start < 0 {
				start = i
			}
		}
	}
	if quote != 0 || bodyEnd < 0 || !finish(len(ddl)) {
		return "", false
	}
	if removeStart < 0 {
		return ddl, true
	}
	return ddl[:removeStart] + ddl[removeEnd:], true
}
