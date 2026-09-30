package interpretation

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	interpretationexecution "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	mongoEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	"go.mongodb.org/mongo-driver/mongo"
)

const acceptanceFailureProbeEnv = "QS_M5_ACCEPTANCE_REPORT_FAILURE"

func acceptanceFailureGateFromEnv(db *mongo.Database) (interpretationexecution.BuildFailureGate, error) {
	raw, configured := os.LookupEnv(acceptanceFailureProbeEnv)
	if !configured {
		return nil, nil
	}
	if db == nil {
		return nil, fmt.Errorf("Mongo database is required")
	}
	scope, err := parseAcceptanceFailureScope(raw)
	if err != nil {
		return nil, err
	}
	return mongoEval.NewAcceptanceFailureGate(db.Collection("interpretation_acceptance_failure_claims"), scope)
}

func parseAcceptanceFailureScope(raw string) (mongoEval.AcceptanceFailureScope, error) {
	var scope mongoEval.AcceptanceFailureScope
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&scope); err != nil {
		return scope, fmt.Errorf("decode %s: %w", acceptanceFailureProbeEnv, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return scope, fmt.Errorf("%s must contain one JSON object", acceptanceFailureProbeEnv)
	}
	return scope, nil
}
