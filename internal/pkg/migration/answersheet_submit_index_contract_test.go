package migration

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnswerSheetSubmitIdentityIndexIsExplicitMigration(t *testing.T) {
	raw, err := migrations.ReadFile("migrations/mongodb/000044_answersheet_submit_intent_index.up.json")
	require.NoError(t, err)
	var commands []map[string]any
	require.NoError(t, json.Unmarshal(raw, &commands))
	require.Len(t, commands, 1)
	require.Equal(t, "answersheets", commands[0]["createIndexes"])
	indexes := commands[0]["indexes"].([]any)
	require.Len(t, indexes, 1)
	index := indexes[0].(map[string]any)
	require.Equal(t, "uk_answersheet_submit_intent", index["name"])
	require.Equal(t, true, index["unique"])
	require.Equal(t, map[string]any{"submit_meta.writer_id": float64(1), "submit_meta.idempotency_key": float64(1)}, index["key"])
	require.Equal(t, map[string]any{"submit_meta.idempotency_key": map[string]any{"$exists": true}}, index["partialFilterExpression"])
}
