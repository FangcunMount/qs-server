package retirement

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func originTestReaders(f authFixture) []io.Reader {
	out := make([]io.Reader, 4)
	for i := range out {
		out[i] = bytes.NewReader(f.raw[i])
	}
	return out
}
func TestSourceOriginCopyBindingFullEOFAndNoAuthority(t *testing.T) {
	f := sourceAuthFixture(t, true, true)
	copies, err := VerifySourceCopies(t.Context(), f.inputs())
	if err != nil {
		t.Fatal(err)
	}
	inputs := f.inputs()
	observed := make([]*authObservedReader, 4)
	for i := range inputs {
		observed[i] = &authObservedReader{Reader: bytes.NewReader(f.raw[i])}
		inputs[i].Input = observed[i]
	}
	binding, err := BindOriginCopies(t.Context(), copies, inputs, DefaultSourceOriginLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, reader := range observed {
		if reader.reads == 0 || reader.closes != 0 {
			t.Fatal("borrowed stream not read or closed")
		}
	}
	if _, _, err = binding.verifyFiles(t.Context(), originTestReaders(f), true); err != nil {
		t.Fatal(err)
	}
	if _, err = json.Marshal(binding); !errors.Is(err, ErrSourceSerialization) {
		t.Fatal("opaque binding serialized")
	}
	var proof *SourceOriginRecheckProof
	report := proof.Report()
	if report.ActualOriginMatched || report.IndependentEpochRechecked || report.CASAuthorized || report.BusinessClosureVerified || report.DropReady || !report.IndependentApprovalRequired || !report.FirstAuthMetadataContinuityUnproven {
		t.Fatal("unearned authority")
	}
	// Legacy Auth deliberately did not retain Boundary. Binding fixes the supplied
	// new header only from this construction, while reporting the earlier gap.
	changed := f
	changed.expected[0].Boundary.SchemaHash = strings.Repeat("c", 64)
	changed.raw[0] = bytes.Replace(f.raw[0], []byte(strings.Repeat("a", 64)), []byte(strings.Repeat("c", 64)), 1)
	other, err := BindOriginCopies(t.Context(), copies, changed.inputs(), DefaultSourceOriginLimits())
	if err != nil {
		t.Fatal("new header should bind without inventing prior continuity", err)
	}
	if other.hash == binding.hash {
		t.Fatal("header identity omitted")
	}
	if _, _, err = binding.verifyFiles(t.Context(), originTestReaders(changed), true); err == nil {
		t.Fatal("bound header tampering accepted")
	}
}
func TestSourceOriginCopyRejectsIncompleteScopeTamperAndLimits(t *testing.T) {
	f := sourceAuthFixture(t, true, true)
	copies, err := VerifySourceCopies(t.Context(), f.inputs())
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"typed_nil", "truncated", "trailing", "different_body", "swapped", "missing", "count", "rows_cap", "bytes_cap", "cancelled", "expiry"} {
		t.Run(kind, func(t *testing.T) {
			inputs := f.inputs()
			limits := DefaultSourceOriginLimits()
			ctx := t.Context()
			switch kind {
			case "typed_nil":
				var reader *bytes.Reader
				inputs[0].Input = reader
			case "truncated":
				inputs[3].Input = bytes.NewReader(f.raw[3][:len(f.raw[3])-1])
			case "trailing":
				inputs[0].Input = bytes.NewReader(append(append([]byte(nil), f.raw[0]...), []byte("{}\n")...))
			case "different_body":
				inputs[0].Input = bytes.NewReader(bytes.Replace(f.raw[0], []byte("cHVibGlzaGVk"), []byte("cGVuZGluZw=="), 1))
			case "swapped":
				inputs[1], inputs[2] = inputs[2], inputs[1]
			case "missing":
				inputs = inputs[:3]
			case "count":
				inputs[0].Expected.Records++
			case "rows_cap":
				limits.MaxRows = 1
			case "bytes_cap":
				limits.MaxBytes = 1
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			case "expiry":
				limits.MaxDuration = time.Nanosecond
			}
			if _, e := BindOriginCopies(ctx, copies, inputs, limits); e == nil {
				t.Fatal("unsafe copy accepted")
			}
		})
	}
}
func TestSourceOriginRequiresActualHostScopes(t *testing.T) {
	f := sourceAuthFixture(t, false, true)
	copies, err := VerifySourceCopies(t.Context(), f.inputs())
	if err != nil {
		t.Fatal(err)
	}
	binding, err := BindOriginCopies(t.Context(), copies, f.inputs(), DefaultSourceOriginLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = PrepareSourceOriginEpoch(t.Context(), binding, nil, nil, originTestReaders(f)); err == nil {
		t.Fatal("editable report without actual database scopes accepted")
	}
	var epoch *SourceOriginEpoch
	if _, err = epoch.Recheck(t.Context(), nil, nil, nil); err == nil {
		t.Fatal("nil epoch accepted")
	}
	if originMongoNumber(123) != "123" {
		t.Fatal("transaction token numeric encoding changed")
	}
}
