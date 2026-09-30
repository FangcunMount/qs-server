package evaluation

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	mongoEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/evaluation"
	"go.mongodb.org/mongo-driver/mongo"
)

const evaluationAcceptanceFailureEnv = "QS_M5_ACCEPTANCE_EVALUATION_FAILURE"

func evaluationFailureGateFromEnv(db *mongo.Database) (execute.PreExecutionFailureGate, error) {
	raw, configured := os.LookupEnv(evaluationAcceptanceFailureEnv)
	if !configured {
		return nil, nil
	}
	if db == nil {
		return nil, fmt.Errorf("mongo database is required")
	}
	scope, err := parseEvaluationAcceptanceFailureScope(raw)
	if err != nil {
		return nil, err
	}
	return mongoEval.NewAcceptanceFailureGate(db.Collection("evaluation_acceptance_failure_claims"), scope)
}

func parseEvaluationAcceptanceFailureScope(raw string) (mongoEval.AcceptanceFailureScope, error) {
	var scope mongoEval.AcceptanceFailureScope
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&scope); err != nil {
		return scope, fmt.Errorf("decode %s: %w", evaluationAcceptanceFailureEnv, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return scope, fmt.Errorf("%s must contain one JSON object", evaluationAcceptanceFailureEnv)
	}
	return scope, nil
}
