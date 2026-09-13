package aibridge

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
)

var creationActor = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:._/-]{0,127}$`)

func evaluationCreation(response *pb.EvaluationState) (*app.EvaluationCreationReceipt, error) {
	raw := response.CreationJson
	if raw == "" { // Older AI versions do not provide creation recovery evidence.
		return nil, nil
	}
	var receipt app.EvaluationCreationReceipt
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if len(raw) > 16*1024 || !utf8.ValidString(raw) || !json.Valid([]byte(raw)) || decoder.Decode(&receipt) != nil ||
		receipt.SchemaVersion != "qs-ai-evaluation-creation-receipt/v1" || receipt.RunID != response.RunId || !app.ValidPublicationID(receipt.RunID) || !receipt.Release.Valid() ||
		!creationActor.MatchString(receipt.RequestedBy) || strings.TrimSpace(receipt.RequestReason) == "" || len(receipt.RequestReason) > 1000 || strings.ContainsAny(receipt.RequestReason, "<>") {
		return nil, app.ErrConflict
	}
	if _, err := time.Parse(time.RFC3339Nano, receipt.CreatedAt); err != nil {
		return nil, app.ErrConflict
	}
	encoded, _ := json.Marshal(receipt.Release)
	var refs map[string]app.FrozenEvaluationRef
	if json.Unmarshal(encoded, &refs) != nil || receipt.ReleaseFingerprint != releaseDigest(refs) {
		return nil, app.ErrConflict
	}
	return &receipt, nil
}
