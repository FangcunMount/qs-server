package messagingruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"strings"
	"testing"
)

func nativeAuditFixture(t *testing.T) []byte {
	t.Helper()
	return []byte(`{"topic":"TRANS_CHECK_MAX_TIME_TOPIC","key_sha256":"` + strings.Repeat("1", 64) + `","records":[{"broker_message_id":"` + strings.Repeat("A", 32) + `","queue_id":0,"queue_offset":1,"commitlog_offset":2,"index_hour":3}],"raw_admin_sha256":"` + strings.Repeat("2", 64) + `","read_only":true,"body_exported":false,"truncated_possible":false,"alert":"native_transaction_check_limit_reached","severity":"critical"}`)
}
func TestNativeLimitEvidencePreservesExactExport(t *testing.T) {
	raw := nativeAuditFixture(t)
	hash := sha256.Sum256(raw)
	evidence, err := ParseNativeLimitEvidence(raw, hash)
	if err != nil || !evidence.valid() || evidence.Digest() != hash {
		t.Fatalf("exact audit rejected: %v", err)
	}
	if (NativeLimitEvidence{}).valid() {
		t.Fatal("zero evidence accepted")
	}
	changed := append(append([]byte{}, raw...), '\n')
	if _, err = ParseNativeLimitEvidence(changed, hash); !errors.Is(err, ErrNativeLimitEvidence) {
		t.Fatal("changed source accepted under old digest")
	}
	if _, err = ParseNativeLimitEvidence(raw, [32]byte{}); !errors.Is(err, ErrNativeLimitEvidence) {
		t.Fatal("self-authorized digest accepted")
	}
}
func TestNativeLimitEvidenceRejectsUnsafeOrAmbiguousReport(t *testing.T) {
	valid := nativeAuditFixture(t)
	cases := map[string][]byte{
		"duplicate_root":   append([]byte(`{"read_only":false,`), valid[1:]...),
		"duplicate_record": []byte(strings.Replace(string(valid), `"queue_id":0`, `"queue_id":0,"queue_id":0`, 1)),
		"second_document":  append(append([]byte{}, valid...), []byte(` {}`)...),
		"oversize":         []byte(strings.Repeat(" ", 65537)),
	}
	for _, change := range []struct{ old, new string }{
		{`"read_only":true`, `"read_only":false`},
		{`"body_exported":false`, `"body_exported":true`},
		{`"read_only":true,`, ``},
		{`"body_exported":false,`, ``},
		{`"truncated_possible":false`, `"truncated_possible":true`},
		{`"queue_id":0`, `"queue_id":-1`},
		{`"queue_id":0`, `"queue_id":0.0`},
		{`"queue_id":0`, `"queue_id":"0"`},
		{`"queue_id":0`, `"queue_id":9223372036854775808`},
		{`"queue_id":0,`, ``},
		{`TRANS_CHECK_MAX_TIME_TOPIC`, `unrelated-topic`},
		{`"severity":"critical"`, `"severity":"warning"`},
		{`"topic":`, `"owner":"private-owner","topic":`},
	} {
		cases[change.old+" -> "+change.new] = []byte(strings.Replace(string(valid), change.old, change.new, 1))
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseNativeLimitEvidence(raw, sha256.Sum256(raw))
			if !errors.Is(err, ErrNativeLimitEvidence) {
				t.Fatal("unsafe audit accepted")
			}
			if strings.Contains(err.Error(), "private") {
				t.Fatal("private evidence in diagnostic")
			}
		})
	}
}
func TestNativeLimitEvidenceRejectsDuplicateBrokerIndex(t *testing.T) {
	var value map[string]any
	if err := json.Unmarshal(nativeAuditFixture(t), &value); err != nil {
		t.Fatal(err)
	}
	rows := value["records"].([]any)
	value["records"] = append(rows, rows[0])
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ParseNativeLimitEvidence(raw, sha256.Sum256(raw)); !errors.Is(err, ErrNativeLimitEvidence) {
		t.Fatal("duplicate index accepted")
	}
	value["records"] = []any{}
	raw, _ = json.Marshal(value)
	if _, err = ParseNativeLimitEvidence(raw, sha256.Sum256(raw)); !errors.Is(err, ErrNativeLimitEvidence) {
		t.Fatal("empty index interpreted as a native limit")
	}
}

func TestNativeIndexIdentityDoesNotDependOnExportLogsOrFormatting(t *testing.T) {
	raw := nativeAuditFixture(t)
	first, err := ParseNativeLimitEvidence(raw, sha256.Sum256(raw))
	if err != nil {
		t.Fatal(err)
	}
	altered := []byte(strings.Replace(string(raw), strings.Repeat("2", 64), strings.Repeat("3", 64), 1))
	second, err := ParseNativeLimitEvidence(altered, sha256.Sum256(altered))
	if err != nil {
		t.Fatal(err)
	}
	if first.digest == second.digest || len(first.indices) != 1 || len(second.indices) != 1 || first.indices[0] != second.indices[0] {
		t.Fatal("volatile export metadata changed native identity")
	}
	altered = []byte(strings.Replace(string(altered), strings.Repeat("A", 32), strings.Repeat("a", 32), 1))
	third, err := ParseNativeLimitEvidence(altered, sha256.Sum256(altered))
	if err != nil || third.indices[0] != first.indices[0] {
		t.Fatal("hex representation changed index identity", err)
	}
	altered = []byte(strings.Replace(string(altered), `"queue_offset":1`, `"queue_offset":2`, 1))
	moved, err := ParseNativeLimitEvidence(altered, sha256.Sum256(altered))
	if err != nil || moved.indices[0] == first.indices[0] {
		t.Fatal("different native record collapsed", err)
	}
}
func TestNativeNoticeCrossLanguageOriginalByteVector(t *testing.T) {
	index, err := hex.DecodeString("826e83de4c78da591b9fdeb1571d7ad86be6355bb1b360430d576ca262b4a633")
	if err != nil {
		t.Fatal(err)
	}
	var fingerprint [32]byte
	copy(fingerprint[:], index)
	key := nativeNoticeKey(transaction.Binding{Owner: "qs.mysql", StoreID: "原库", WriterEpoch: "epoch-1"}, "m3-1", fingerprint)
	if hex.EncodeToString(key[:]) != "f93835f735b87a3a4af30ceb3b0dae81b08815fb24c8705644f20e7ad5b608ff" {
		t.Fatal("cross-language key drifted")
	}
}
