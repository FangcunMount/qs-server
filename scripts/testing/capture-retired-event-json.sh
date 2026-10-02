#!/usr/bin/env bash
# Reproduce the synthetic business-event fixture with the immutable old module.
set -euo pipefail
repo=$(git rev-parse --show-toplevel)
source_commit=96fc4df42f6af9f6ce22e2184394b1a07df2ffdd
[[ $(git -C "$repo" rev-parse "$source_commit^{commit}") == "$source_commit" ]] || exit 1
snapshot=$(mktemp -d "${TMPDIR:-/tmp}/qs-retired-event-json.XXXXXX")
trap 'rm -rf -- "$snapshot"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
git -C "$repo" archive "$source_commit" | tar -x -C "$snapshot"
mkdir -p "$snapshot/cmd/event-json-capture"
cat > "$snapshot/cmd/event-json-capture/main.go" <<'GO'
package main
import (
 "encoding/json"
 "os"
 "time"
 "github.com/FangcunMount/component-base/pkg/event"
)
func main() {
 value := event.Event[map[string]string]{BaseEvent:event.BaseEvent{
  ID:"original-id",EventTypeValue:"questionnaire.changed",
  OccurredAtValue:time.Date(2026,9,27,20,30,0,123000000,time.FixedZone("UTC+8",8*3600)),
  AggregateTypeValue:"Questionnaire",AggregateIDValue:"问卷-A",
 },Data:map[string]string{"title":"青岛测评"}}
 body,err:=json.Marshal(value);if err!=nil {panic(err)}
 if _,err=os.Stdout.Write(body);err!=nil {panic(err)}
}
GO
(cd "$snapshot" && GOWORK=off go run ./cmd/event-json-capture)
