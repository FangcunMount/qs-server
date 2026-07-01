package ws

import "testing"

func TestDecodeSubscribeFrame(t *testing.T) {
	frame, err := decodeFrame([]byte(`{"op":"subscribe","assessment_id":"123","kind":"personality","testee_id":"456"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if frame.Op != OpSubscribe || frame.AssessmentID != "123" || frame.Kind != "personality" || frame.TesteeID != "456" {
		t.Fatalf("unexpected frame: %+v", frame)
	}
}

func TestEncodeStatusFrame(t *testing.T) {
	payload, err := encodeFrame(outboundFrame{
		Op:   OpStatus,
		Data: map[string]any{"status": "interpreted"},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("expected payload")
	}
}

func TestConnectionManagerLimits(t *testing.T) {
	mgr := newConnectionManager(1, 1)
	if !mgr.TryAcquire("1") {
		t.Fatal("expected first acquire to succeed")
	}
	if mgr.TryAcquire("1") {
		t.Fatal("expected per-testee limit to reject")
	}
	mgr.Release("1")
	if !mgr.TryAcquire("2") {
		t.Fatal("expected acquire after release")
	}
}
