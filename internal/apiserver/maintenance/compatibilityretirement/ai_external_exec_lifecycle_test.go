package retirement

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func aiExecUnitHost(t *testing.T) []byte {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("source location unavailable")
	}
	raw, e := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../../scripts/database/qs-ai-retirement-readonly-host.py"))
	if e != nil || sourceSHA(raw) != aiExternalHostSHA {
		t.Fatal("fixed public producer source unavailable or changed")
	}
	return raw
}
func aiExecUnitBinding(input []byte) aiExecBinding {
	return aiExecBinding{SourceSHA: strings.Repeat("a", 40), OperationID: "900-1", RunID: "901", RuntimeSourceSHA: strings.Repeat("b", 40), ImageID: "sha256:" + strings.Repeat("c", 64), ContainerID: strings.Repeat("d", 64), PythonSHA256: aiExternalHostSHA, InputSHA256: sourceSHA(input), DeadlineUnixNano: time.Now().Add(time.Minute).UnixNano()}
}
func aiExecUnitJournal(t *testing.T, b aiExecBinding) *aiExecJournal {
	t.Helper()
	directory := t.TempDir()
	var canonicalErr error
	directory, canonicalErr = filepath.EvalSymlinks(directory)
	if canonicalErr != nil {
		t.Fatal(canonicalErr)
	}
	if e := os.Chmod(directory, 0700); e != nil {
		t.Fatal(e)
	}
	j, e := aiExecOpenJournal(filepath.Join(directory, "original-exec.ndjson"), b, true)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := j.Close(); e != nil {
			t.Error(e)
		}
	})
	return j
}

// The fake tests protocol ordering/fail-closed handling only; it creates no
// Engine execution, origin/closure proof or usable production qualification.
type aiExecUnitProtocol struct {
	t                                *testing.T
	journal                          *aiExecJournal
	calls                            []string
	createErr, attachErr, inspectErr error
	id                               string
	output                           []byte
	before, after                    aiExecInspect
}

func (p *aiExecUnitProtocol) version(context.Context) (string, error) {
	p.calls = append(p.calls, "version")
	return strings.Repeat("e", 64), nil
}
func (p *aiExecUnitProtocol) create(context.Context, string, []byte) (string, error) {
	p.calls = append(p.calls, "create")
	raw, e := os.ReadFile(p.journal.path)
	if e != nil {
		p.t.Fatal(e)
	}
	rs, e := aiExecDecodeJournal(raw, p.journal.binding)
	if e != nil || len(rs) != 1 || rs[0].Stage != "create_intent" {
		p.t.Fatal("create preceded durable intent")
	}
	return p.id, p.createErr
}
func (p *aiExecUnitProtocol) attach(context.Context, string, []byte) ([]byte, error) {
	p.calls = append(p.calls, "attach")
	raw, e := os.ReadFile(p.journal.path)
	if e != nil {
		p.t.Fatal(e)
	}
	rs, e := aiExecDecodeJournal(raw, p.journal.binding)
	if e != nil || len(rs) != 3 || rs[1].ExecID != p.id || rs[2].Stage != "start_intent" {
		p.t.Fatal("attach preceded actual ID and durable start intent")
	}
	return p.output, p.attachErr
}
func (p *aiExecUnitProtocol) inspect(context.Context, string, string, string) (aiExecInspect, error) {
	p.calls = append(p.calls, "inspect")
	if len(p.calls) >= 2 && p.calls[len(p.calls)-2] == "create" {
		return p.before, p.inspectErr
	}
	return p.after, p.inspectErr
}
func aiExecUnitProtocolFor(t *testing.T, j *aiExecJournal) *aiExecUnitProtocol {
	zero := 0
	return &aiExecUnitProtocol{t: t, journal: j, id: strings.Repeat("f", 64), output: []byte(`{"protocol":"synthetic_only"}`), after: aiExecInspect{ExitCode: &zero}}
}
func TestAIExternalExecDurableOrderingAndSameHandleOutput(t *testing.T) {
	host := aiExecUnitHost(t)
	input := []byte(`{"private":"synthetic-do-not-record"}`)
	b := aiExecUnitBinding(input)
	j := aiExecUnitJournal(t, b)
	p := aiExecUnitProtocolFor(t, j)
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, b.DeadlineUnixNano))
	defer cancel()
	observed, e := aiExecProduce(ctx, p, j, host, input)
	if e != nil {
		t.Fatal(e)
	}
	actual, e := observed.actualOutput(ctx)
	if e != nil || !bytes.Equal(actual, p.output) {
		t.Fatal("actual complete attach and inspected exit lost")
	}
	if !reflect.DeepEqual(p.calls, []string{"version", "create", "inspect", "attach", "inspect"}) {
		t.Fatal("unexpected exec lifecycle sequence")
	}
	raw, e := os.ReadFile(j.path)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(raw, input) || bytes.Contains(raw, host) || bytes.Contains(raw, p.output) {
		t.Fatal("private input/code/output leaked into journal")
	}
	var first map[string]any
	if json.Unmarshal(bytes.Split(raw, []byte{'\n'})[0], &first) != nil {
		t.Fatal("journal fixture decode failed")
	}
	for _, key := range []string{"attach_complete", "output_bytes", "previous_sha256", "running"} {
		changed := map[string]any{}
		for k, v := range first {
			changed[k] = v
		}
		delete(changed, key)
		line, encodeErr := json.Marshal(changed)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		if _, e := aiExecDecodeJournal(append(line, '\n'), b); e == nil {
			t.Fatal("missing canonical journal field adopted")
		}
	}
	if _, e = aiExecProduce(ctx, p, j, host, input); e == nil || len(p.calls) != 5 {
		t.Fatal("same journal was reused to execute again")
	}
	jCopy := reflect.New(reflect.TypeOf(j).Elem()).Elem()
	jCopy.Set(reflect.ValueOf(j).Elem())
	copied := jCopy.Addr().Interface().(*aiExecJournal)
	if copied.Close() == nil || j.checkLocked() != nil {
		t.Fatal("struct copy touched original owned fd")
	}
}
func TestAIExternalExecLostCreateResponseNeverStartsOrResends(t *testing.T) {
	for _, known := range []bool{false, true} {
		t.Run(map[bool]string{false: "unknown_id", true: "id_received_client_unknown"}[known], func(t *testing.T) {
			host := aiExecUnitHost(t)
			input := []byte(`{}`)
			b := aiExecUnitBinding(input)
			j := aiExecUnitJournal(t, b)
			p := aiExecUnitProtocolFor(t, j)
			p.createErr = ErrAIExternalExecUnknown
			if !known {
				p.id = ""
			}
			ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, b.DeadlineUnixNano))
			defer cancel()
			if _, e := aiExecProduce(ctx, p, j, host, input); e == nil {
				t.Fatal("unknown create admitted")
			}
			if !reflect.DeepEqual(p.calls, []string{"version", "create"}) || j.last.Stage != "unknown" || known && j.last.ExecID != p.id {
				t.Fatal("create unknown retried, started or lost known handle")
			}
			if _, e := aiExecProduce(ctx, p, j, host, input); e == nil || len(p.calls) != 2 {
				t.Fatal("unknown create was resent")
			}
		})
	}
}
func TestAIExternalExecLostAttachCannotRecoverQualificationFromZeroExit(t *testing.T) {
	host := aiExecUnitHost(t)
	input := []byte(`{}`)
	b := aiExecUnitBinding(input)
	j := aiExecUnitJournal(t, b)
	p := aiExecUnitProtocolFor(t, j)
	p.attachErr = io.ErrUnexpectedEOF
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, b.DeadlineUnixNano))
	defer cancel()
	observed, e := aiExecProduce(ctx, p, j, host, input)
	if e == nil || observed == nil || observed.complete || observed.exitCode == nil || *observed.exitCode != 0 {
		t.Fatal("lost output was promoted or actual inspect was discarded")
	}
	if _, e = observed.actualOutput(ctx); e == nil {
		t.Fatal("zero exit recreated missing stdout qualification")
	}
	// Reopen is always inspect-only. Reconcile never invokes create or attach.
	if e = j.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := aiExecOpenJournal(j.path, b, false)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := reopened.Close(); e != nil {
			t.Error(e)
		}
	}()
	p.calls = nil
	p.journal = reopened
	reopened.mu.Lock()
	result, e := aiExecReconcileProtocolLocked(ctx, p, reopened)
	reopened.mu.Unlock()
	if e != nil || result.complete || len(result.output) != 0 || !reflect.DeepEqual(p.calls, []string{"version", "inspect"}) {
		t.Fatal("reconcile reran or restored output")
	}
	if _, e = aiExecProduce(ctx, p, reopened, host, input); e == nil || len(p.calls) != 2 {
		t.Fatal("reopened journal authorized execution")
	}
}
func TestAIExternalExecRunningOrMissingExitRemainUnknown(t *testing.T) {
	for _, state := range []aiExecInspect{{Running: true}, {Running: false}, {ExitCode: func() *int { v := 7; return &v }()}} {
		t.Run(func() string {
			if state.Running {
				return "running"
			}
			if state.ExitCode == nil {
				return "not_started_or_unknown"
			}
			return "nonzero"
		}(), func(t *testing.T) {
			host := aiExecUnitHost(t)
			input := []byte(`{}`)
			b := aiExecUnitBinding(input)
			j := aiExecUnitJournal(t, b)
			p := aiExecUnitProtocolFor(t, j)
			p.after = state
			ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, b.DeadlineUnixNano))
			defer cancel()
			result, e := aiExecProduce(ctx, p, j, host, input)
			if e == nil || result == nil {
				t.Fatal("nonterminal/nonzero admitted")
			}
			if _, e = result.actualOutput(ctx); e == nil {
				t.Fatal("unknown output admitted")
			}
		})
	}
}
func TestAIExternalExecOriginalDeadlineAndPrivateInputNeverReset(t *testing.T) {
	host := aiExecUnitHost(t)
	input := []byte(`{}`)
	b := aiExecUnitBinding(input)
	j := aiExecUnitJournal(t, b)
	p := aiExecUnitProtocolFor(t, j)
	late, cancel := context.WithDeadline(context.Background(), time.Unix(0, b.DeadlineUnixNano).Add(time.Second))
	defer cancel()
	if _, e := aiExecProduce(late, p, j, host, input); e == nil || len(p.calls) != 0 || len(j.raw) != 0 {
		t.Fatal("original deadline extended")
	}
	bounded, cancel2 := context.WithDeadline(context.Background(), time.Unix(0, b.DeadlineUnixNano))
	defer cancel2()
	if _, e := aiExecProduce(bounded, p, j, host, []byte(`{"changed":true}`)); e == nil || len(p.calls) != 0 {
		t.Fatal("private packet hash not bound")
	}
	b.DeadlineUnixNano = time.Now().Add(-time.Second).UnixNano()
	if _, _, e := b.scope(bounded); e == nil {
		t.Fatal("expired original deadline reset")
	}
}
func TestAIExternalExecJournalStrictMetadataAndNoAdoption(t *testing.T) {
	input := []byte(`{}`)
	b := aiExecUnitBinding(input)
	j := aiExecUnitJournal(t, b)
	if _, e := aiExecOpenJournal(j.path, b, true); e == nil {
		t.Fatal("existing journal adopted")
	}
	if e := os.WriteFile(j.path, []byte("torn"), 0600); e != nil {
		t.Fatal(e)
	}
	if j.checkLocked() == nil {
		t.Fatal("changed raw journal not rejected")
	}
	if e := j.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e := aiExecOpenJournal(j.path, b, false); e == nil {
		t.Fatal("torn journal adopted")
	}
	path := filepath.Join(filepath.Dir(j.path), "fifo")
	if e := syscall.Mkfifo(path, 0600); e != nil {
		t.Fatal(e)
	}
	started := time.Now()
	if _, e := aiExecOpenJournal(path, b, false); e == nil {
		t.Fatal("FIFO adopted")
	}
	if time.Since(started) > time.Second {
		t.Fatal("FIFO read blocked")
	}
	sym := filepath.Join(filepath.Dir(j.path), "link")
	if e := os.Symlink(j.path, sym); e != nil {
		t.Fatal(e)
	}
	if _, e := aiExecOpenJournal(sym, b, false); e == nil {
		t.Fatal("symlink adopted")
	}
	var zero aiExecJournal
	if e := zero.Close(); e != nil {
		t.Fatal("zero value closed a host descriptor")
	}
	if _, e := aiExternalExecTracked(context.Background(), nil, nil, nil, nil); e == nil {
		t.Fatal("missing actual Docker executor admitted")
	}
	if _, e := aiExternalExecReconcile(context.Background(), nil, nil); e == nil {
		t.Fatal("missing actual original exec handle admitted")
	}
}
func TestAIExternalExecInspectStrictActualIDAndNullableExit(t *testing.T) {
	host := aiExecUnitHost(t)
	id, cid := strings.Repeat("a", 64), strings.Repeat("b", 64)
	base := map[string]any{"ID": id, "ContainerID": cid, "Running": false, "ExitCode": nil, "OpenStdin": true, "OpenStdout": true, "OpenStderr": true, "ProcessConfig": map[string]any{"tty": false, "entrypoint": "/app/.venv/bin/python", "arguments": []string{"-I", "-B", "-c", string(host)}, "privileged": false}}
	encode := func(v map[string]any) []byte {
		raw, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		return raw
	}
	if state, e := aiExecDecodeInspect(encode(base), id, cid, aiExternalHostSHA); e != nil || state.ExitCode != nil || state.Running {
		t.Fatal("created null-exit state misrepresented")
	}
	for _, change := range []func(map[string]any){func(v map[string]any) { v["ID"] = cid }, func(v map[string]any) { v["ContainerID"] = id }, func(v map[string]any) { delete(v, "Running") }, func(v map[string]any) { v["Running"] = true; v["ExitCode"] = 0 }, func(v map[string]any) { v["OpenStdin"] = false }, func(v map[string]any) { v["ExitCode"] = -1 }, func(v map[string]any) { v["ProcessConfig"] = nil }} {
		altered := map[string]any{}
		for k, v := range base {
			altered[k] = v
		}
		change(altered)
		if _, e := aiExecDecodeInspect(encode(altered), id, cid, aiExternalHostSHA); e == nil {
			t.Fatal("incorrect actual exec inspection accepted")
		}
	}
}
func aiExecUnitFrame(stream byte, payload []byte) []byte {
	h := make([]byte, 8)
	h[0] = stream
	binary.BigEndian.PutUint32(h[4:], uint32(len(payload)))
	return append(h, payload...)
}
func TestAIExternalExecMultiplexBoundsTruncationAndStderrPrivacy(t *testing.T) {
	good := append(aiExecUnitFrame(2, []byte("synthetic-sensitive-stderr")), aiExecUnitFrame(1, []byte(`{"ok":true}`))...)
	output, e := aiExecMultiplex(bytes.NewReader(good))
	if e != nil || string(output) != `{"ok":true}` {
		t.Fatal("stdout/stderr separation failed")
	}
	cases := [][]byte{nil, good[:len(good)-1], {1, 0, 0, 0}, aiExecUnitFrame(3, []byte("engine error")), aiExecUnitFrame(2, bytes.Repeat([]byte{'x'}, aiExecStderrLimit+1))}
	for _, raw := range cases {
		if _, e := aiExecMultiplex(bytes.NewReader(raw)); e == nil {
			t.Fatal("incomplete or unknown stream accepted")
		}
	}
	reader := &aiExecHeaderReader{reader: strings.NewReader(strings.Repeat("X", aiExecHeaderLimit+1))}
	if _, e := io.ReadAll(reader); !errors.Is(e, ErrAIExternalExecUnknown) {
		t.Fatal("unbounded HTTP header accepted")
	}
	for _, pair := range [][2]string{{"1.25", "1.51"}, {"1.44", "1.44"}} {
		if !aiExecVersionSupports(pair[0], pair[1]) {
			t.Fatal("supported actual API range rejected")
		}
	}
	for _, pair := range [][2]string{{"", "1.51"}, {"1.45", "1.54"}, {"1.25", "1.43"}, {"1.044", "1.51"}, {"2.0", "2.1"}} {
		if aiExecVersionSupports(pair[0], pair[1]) {
			t.Fatal("unsupported or malformed API range accepted")
		}
	}
}

func TestAIExternalExecObservationFormattingAndImportsCannotRecoverOutput(t *testing.T) {
	v := &aiExternalExecObservation{output: []byte("synthetic-private-result-do-not-publish")}
	v.self = v
	if strings.Contains(fmt.Sprintf("%v %#v", v, v), "synthetic-private") {
		t.Fatal("private attach output was formatted")
	}
	if _, e := json.Marshal(v); !errors.Is(e, ErrSourceSerialization) {
		t.Fatal("opaque lifecycle observation serialized")
	}
	var imported aiExternalExecObservation
	if e := json.Unmarshal([]byte(`{"complete":true,"exitCode":0,"output":"synthetic-private"}`), &imported); !errors.Is(e, ErrSourceSerialization) {
		t.Fatal("opaque lifecycle JSON import accepted")
	}
	if _, e := imported.actualOutput(context.Background()); e == nil {
		t.Fatal("JSON created actual output capability")
	}
}
