#!/usr/bin/env bash
# Reproduce synthetic compatibility vectors with the immutable old module.
set -euo pipefail
repo=$(git rev-parse --show-toplevel)
source_commit=96fc4df42f6af9f6ce22e2184394b1a07df2ffdd
[[ $(git -C "$repo" rev-parse "$source_commit^{commit}") == "$source_commit" ]] || exit 1
snapshot=$(mktemp -d "${TMPDIR:-/tmp}/qs-retired-wire.XXXXXX")
trap 'rm -rf -- "$snapshot"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
git -C "$repo" archive "$source_commit" | tar -x -C "$snapshot"
mkdir -p "$snapshot/cmd/wire-contract-capture"
cat > "$snapshot/cmd/wire-contract-capture/main.go" <<'GO'
package main
import (
 "encoding/json"
 "os"
 "time"
 "github.com/FangcunMount/component-base/pkg/event"
 "github.com/FangcunMount/component-base/pkg/eventcodec"
 "github.com/FangcunMount/component-base/pkg/eventmessaging"
 "github.com/FangcunMount/component-base/pkg/messaging"
)
type wireCase struct { Payload, Wire, RelayWire string }
type decodeCase struct { Input string; Value string; Metadata map[string]string; Error string }
func check(err error) { if err!=nil {panic(err)} }
func main() {
 wires:=map[string]wireCase{}
 for _, item:=range []struct{kind,aggregate string;direct bool}{
  {"answersheet.submitted","AnswerSheet",false},{"evaluation.requested","Evaluation",false},
  {"evaluation.retry.requested","Evaluation",false},{"evaluation.outcome.committed","Evaluation",false},
  {"evaluation.failed","Evaluation",false},{"interpretation.report.generated","Report",false},
  {"interpretation.report.failed","Report",false},{"interpretation.retry.requested","ReportGeneration",false},
  {"task.opened.reminder.requested","AssessmentTask",false},
  {"questionnaire.changed","Questionnaire",true},{"assessment_model.changed","AssessmentModel",true},
  {"task.opened","AssessmentTask",true},{"task.completed","AssessmentTask",true},
  {"task.expired","AssessmentTask",true},{"task.canceled","AssessmentTask",true},
 } {
  id:="stable-event-id"; data:=map[string]any{"org_id":1,"answer_sheet_id":"answer-1"}
  if item.direct {id="best-effort-stable-id";data=map[string]any{"org_id":1,"revision":"v2"}}
  evt:=event.Event[map[string]any]{BaseEvent:event.BaseEvent{ID:id,EventTypeValue:item.kind,
   OccurredAtValue:time.Date(2026,9,23,10,0,0,0,time.FixedZone("UTC+8",8*3600)),
   AggregateTypeValue:item.aggregate,AggregateIDValue:"aggregate-1"},Data:data}
  msg,err:=eventmessaging.BuildMessage(evt,"api-server");check(err)
  wire,err:=messaging.EncodeMessagePayload(msg);check(err)
  stored,err:=eventcodec.DecodeDomainEvent(msg.Payload);check(err)
  relay,err:=eventmessaging.BuildMessage(stored,"api-server");check(err)
  relayWire,err:=messaging.EncodeMessagePayload(relay);check(err)
  wires[item.kind]=wireCase{Payload:string(msg.Payload),Wire:string(wire),RelayWire:string(relayWire)}
 }
 decoders:=map[string]decodeCase{}
 for name,input:=range map[string]string{
  "precise identifiers and UTC+8":`{"id":"original-id","eventType":"evaluation.retry.requested","occurredAt":"2026-10-02T10:01:02.123456789+08:00","aggregateType":"Evaluation","aggregateID":"639678084915671598","data":{"org_id":639678084915671598,"nested":[true,null,"中文"]}}`,
  "unknown fields":`{"id":"original-id","eventType":"future.event","extra":"historically ignored","data":{"new_field":"kept"}}`,
  "null data":`{"id":"original-id","data":null}`,"missing data":`{"id":"original-id"}`,
  "empty envelope":`{}`,"null envelope":`null`,"invalid JSON":`{not-json`,
  "invalid date":`{"occurredAt":"not-a-date"}`,"wrong field type":`{"id":123}`,
  "array envelope":`[]`,"empty payload":``,
 } {
  result:=decodeCase{Input:input}
  value,err:=eventcodec.DecodeDomainEvent([]byte(input))
  if err!=nil {result.Error=err.Error()} else {
   encoded,err:=json.Marshal(value);check(err);result.Value=string(encoded)
   result.Metadata=eventcodec.MetadataFromEvent(value,"api-server")
  }
  decoders[name]=result
 }
 body,err:=json.MarshalIndent(struct{Source string;Wires map[string]wireCase;Decoders map[string]decodeCase}{
  "96fc4df42f6af9f6ce22e2184394b1a07df2ffdd/component-base@v0.6.11",wires,decoders},"","  ");check(err)
 _,err=os.Stdout.Write(body);check(err)
}
GO
(cd "$snapshot" && GOWORK=off go run ./cmd/wire-contract-capture)
