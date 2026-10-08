package retirement

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestPrivateFactFramePreservesOriginalByteVector(t *testing.T) {
	h := &privateFactHasher{h: sha256.New()}
	if err := h.text(""); err != nil {
		t.Fatal(err)
	}
	if err := h.textParts("a", "", "bc"); err != nil {
		t.Fatal(err)
	}
	if err := h.text(strings.Repeat("汉字🙂", 150)); err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, 768)
	for i := range raw {
		raw[i] = byte(i)
	}
	if err := h.frame(raw); err != nil {
		t.Fatal(err)
	}
	if err := h.textParts("", ":", "named.Type"); err != nil {
		t.Fatal(err)
	}
	// Computed independently from original non-NULL, big-endian nine-byte
	// framing of five complete frames. Includes scratch reuse, multi-chunk
	// UTF-8 and binary data, empty content and a segmented type label.
	const want = "008d728fcf78d99efcf2f3c8706fb4f0ca0ccb26f1c12e2cf09d094130c6a4d2"
	if actual := hex.EncodeToString(h.h.Sum(nil)); actual != want {
		t.Fatalf("original frame vector changed: %s", actual)
	}
}

func TestPrivateFactFrameBudgetRejectsBeforeHashMutation(t *testing.T) {
	for _, path := range []string{"text", "parts", "bytes"} {
		t.Run(path, func(t *testing.T) {
			h := &privateFactHasher{h: sha256.New(), bytes: maxPrivateFactBytes - 10}
			before := hex.EncodeToString(h.h.Sum(nil))
			var err error
			switch path {
			case "text":
				err = h.text("ab")
			case "parts":
				err = h.textParts("a", "b")
			case "bytes":
				err = h.frame([]byte{0, 1})
			}
			if err != ErrSourceBounds || h.bytes != maxPrivateFactBytes-10 || hex.EncodeToString(h.h.Sum(nil)) != before {
				t.Fatal("rejected frame changed original byte budget or hash")
			}
			if err = h.textParts("", "a", ""); err != nil || h.bytes != maxPrivateFactBytes {
				t.Fatal("exact byte-budget boundary rejected", err)
			}
			before = hex.EncodeToString(h.h.Sum(nil))
			if h.text("") != ErrSourceBounds || h.bytes != maxPrivateFactBytes || hex.EncodeToString(h.h.Sum(nil)) != before {
				t.Fatal("empty frame bypassed original nine-byte budget")
			}
		})
	}
}
