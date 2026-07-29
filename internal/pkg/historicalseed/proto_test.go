package historicalseed

import (
	"testing"
	"time"
)

func TestProtoRoundTrip(t *testing.T) {
	t0 := time.Date(2025, 1, 1, 8, 0, 0, 123, time.FixedZone("CST", 8*60*60))
	t1 := t0.Add(time.Minute)
	in := Context{BatchID: "batch", ScenarioID: "scenario", OrgID: 9, Version: Version1, Timeline: Timeline{TesteeCreatedAt: &t0, AnswerSheetFilledAt: &t0, EvaluatedAt: &t1}}
	out, err := FromProto(ToProto(in))
	if err != nil {
		t.Fatalf("FromProto() error = %v", err)
	}
	if out.BatchID != in.BatchID || out.ScenarioID != in.ScenarioID || out.OrgID != in.OrgID || out.Timeline.TesteeCreatedAt == nil || !out.Timeline.TesteeCreatedAt.Equal(t0) || out.Timeline.AnswerSheetFilledAt == nil || !out.Timeline.AnswerSheetFilledAt.Equal(t0) {
		t.Fatalf("round trip = %#v", out)
	}
}

func TestFromProtoNilIsOrdinaryExecution(t *testing.T) {
	out, err := FromProto(nil)
	if err != nil || out != (Context{}) {
		t.Fatalf("FromProto(nil) = %#v, %v", out, err)
	}
}
