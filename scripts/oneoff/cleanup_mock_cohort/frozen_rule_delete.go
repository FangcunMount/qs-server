package main

import (
	"bytes"
	"context"
	"errors"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
)

type graphFrozenRuleDeleteKey struct{}

// Only the explicitly selected rule-batch action installs this context. The
// private dispatcher must prove this same raw archive is durable on Mac before
// dispatching apply; this context is not a custody or startup authorization.
func graphFrozenRuleDeleteContext(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, graphFrozenRuleDeleteKey{}, enabled)
}

func graphIsFrozenRuleDelete(ctx context.Context) bool {
	enabled, _ := ctx.Value(graphFrozenRuleDeleteKey{}).(bool)
	return enabled
}

// Split already captured originals rather than fetching their bodies again.
// All complete raw originals and restore shape/packet checks remain; only the
// target secondary-unique collision reads are omitted by the explicit mode.
func graphFrozenCapturePreflight(ctx context.Context, s graphBatchProvider, records []record, emit func([]record) error) error {
	err := s.preflightExactBatch(ctx, records, true)
	if errors.Is(err, errGraphBatchBudget) && len(records) > 1 {
		n := len(records) / 2
		if err = graphFrozenCapturePreflight(ctx, s, records[:n], emit); err != nil {
			return err
		}
		return graphFrozenCapturePreflight(ctx, s, records[n:], emit)
	}
	if err != nil {
		return err
	}
	return emit(records)
}

func graphSQLKeyColumns(shape graphSQLShape) string {
	columns := make([]string, len(shape.keys))
	for i, key := range shape.keys {
		columns[i] = "x.`" + key.Name + "`"
	}
	return strings.Join(columns, ",")
}

func graphMutationSQLRead(ctx context.Context, q sqlReader, objects []object, shape graphSQLShape) ([]graphReadResult, error) {
	if graphIsFrozenRuleDelete(ctx) {
		return graphReadSQLBatchColumns(ctx, q, objects, shape, true, graphSQLKeyColumns(shape))
	}
	return graphReadSQLBatch(ctx, q, objects, shape, true)
}

func (s *graphStore) graphMutationMongoRead(ctx context.Context, objects []object, definition []byte) ([]graphReadResult, error) {
	if graphIsFrozenRuleDelete(ctx) {
		return s.readGraphMongoBatchProjection(ctx, objects, definition, bson.D{{Key: "_id", Value: 1}})
	}
	return s.readGraphMongoBatch(ctx, objects, definition)
}

func graphMutationPreimages(ctx context.Context, records []record, current []graphReadResult, restore bool) ([]record, error) {
	if !graphIsFrozenRuleDelete(ctx) {
		return graphBatchPreimages(records, current, restore)
	}
	if restore {
		return nil, errors.New("frozen rule delete cannot restore originals")
	}
	if len(records) != len(current) {
		return nil, errors.New("frozen rule key cardinality changed")
	}
	for i, r := range records {
		if !current[i].Exists || objectID(current[i].Record.Object) != objectID(r.Object) || !bytes.Equal(current[i].Record.Definition, r.Definition) {
			return nil, errors.New("frozen rule physical key missing or namespace changed")
		}
	}
	return records, nil
}

func graphFrozenReadKeys(ctx context.Context, s graphBatchProvider, objects []object) ([]graphReadResult, error) {
	real, ok := s.(*graphStore)
	if !ok {
		return s.readExactBatch(ctx, objects)
	}
	if err := graphBatchObjects(objects, false); err != nil {
		return nil, err
	}
	if objects[0].Store == "mysql" {
		shape, err := graphBatchSQLShape(ctx, real.live.readSQL(), objects[0].Name)
		if err != nil {
			return nil, err
		}
		return graphReadSQLBatchColumns(ctx, real.live.readSQL(), objects, shape, false, graphSQLKeyColumns(shape))
	}
	definition, err := newGraphStore(real.live).mongoDefinition(ctx, objects[0].Name)
	if err != nil {
		return nil, err
	}
	return real.readGraphMongoBatchProjection(ctx, objects, definition, bson.D{{Key: "_id", Value: 1}})
}

func graphVerificationState(ctx context.Context, s graphBatchProvider, records []record, original, uncertain bool) (bool, bool, error) {
	if !graphIsFrozenRuleDelete(ctx) || original || uncertain {
		// Unknown commits still require the original complete read-only
		// reconciliation. They are never converted into another write attempt.
		return graphChunkState(ctx, s, records)
	}
	objects := make([]object, len(records))
	for i, r := range records {
		objects[i] = r.Object
	}
	current, err := graphFrozenReadKeys(ctx, s, objects)
	if err != nil {
		return false, false, err
	}
	if len(current) != len(records) {
		return false, false, errors.New("frozen rule absence cardinality changed")
	}
	absent := true
	for i, v := range current {
		if objectID(v.Record.Object) != objectID(objects[i]) || !graphClearanceSameNamespace(ctx, objects[i].Store, v.Record.Definition, records[i].Definition) {
			return false, false, errors.New("frozen rule absence namespace or physical key changed")
		}
		absent = absent && !v.Exists
	}
	return absent, false, nil
}
