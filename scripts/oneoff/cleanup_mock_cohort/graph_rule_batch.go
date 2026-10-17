package main

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

type graphRuleBatchContextKey struct{}

type graphRuleBatchObservation struct {
	Started  time.Time `json:"started_at"`
	Released time.Time `json:"released_at"`
}

// Reuse is limited to the exact component selection actually observed and
// released within this one independently frozen maintenance window. It does not
// infer freeze from timestamps; graphStaticAuthority still validates its original
// independent freeze and owner inputs before any live connection is opened.
func graphRuleBatchReuse(index graphIndex, selector cohortSelector, g group, now time.Time) error {
	if g.QS.Graph == nil || index.RuleBatch == nil || index.Source != toolSource || !index.ReadOnly || !index.DiscoveryComplete || !index.Released {
		return errors.New("rule batch requires this tool's released same-window observation")
	}
	window, observed := g.QS.Graph, index.RuleBatch
	if observed.Started.IsZero() || observed.Released.IsZero() || observed.Started.Before(window.WindowStarted) || observed.Released.Before(observed.Started) || !observed.Released.Before(window.WindowExpires) || observed.Released.After(now) || !now.Before(window.WindowExpires) {
		return errors.New("rule batch observation lies outside original frozen window")
	}
	if _, e := graphWindowSelector(selector, window.RootIDs); e != nil {
		return e
	}
	return nil
}

func graphRuleBatchContext(ctx context.Context, enabled bool) context.Context {
	if !enabled {
		return ctx
	}
	return context.WithValue(ctx, graphRuleBatchContextKey{}, true)
}

// This changes transfer/decoding cost, not Mongo's available indexes. Testee has
// no leading index at 101/39, so the server may still scan the collection. The
// original candidate decoder, duplicate holds and protected/shared checks remain.
func graphRuleBatchAnswersheets(ctx context.Context, roots []string, sheetRoots map[string]string) (bson.D, error) {
	if enabled, _ := ctx.Value(graphRuleBatchContextKey{}).(bool); !enabled {
		return bson.D{}, nil
	}
	if e := canonicalIDs(roots, false); e != nil {
		return nil, e
	}
	parents := make([]string, 0, len(sheetRoots))
	for id := range sheetRoots {
		parents = append(parents, id)
	}
	sort.Strings(parents)
	if e := canonicalIDs(parents, true); e != nil {
		return nil, e
	}
	clauses := bson.A{}
	for _, part := range []struct {
		field string
		ids   []string
	}{{"testee_id", roots}, {"domain_id", parents}} {
		if len(part.ids) == 0 {
			continue
		}
		// BSON numeric equality includes the int32/integral-double aliases already
		// recognized by graphRawIDMember. Its string branch also accepts leading
		// '+' and zeroes, so keep these candidates for the original typed hold.
		clauses = append(clauses, bson.D{{Key: part.field, Value: bson.D{{Key: "$in", Value: graphMongoIDs(part.ids)}}}})
		if e := rootBatches(part.ids, 500, func(batch []string) error {
			clauses = append(clauses, bson.D{{Key: part.field, Value: bson.D{{Key: "$regex", Value: "^\\+?0*(" + strings.Join(batch, "|") + ")$"}}}})
			return nil
		}); e != nil {
			return nil, e
		}
	}
	filter := bson.D{{Key: "$or", Value: clauses}}
	raw, e := bson.Marshal(filter)
	if e != nil {
		return nil, e
	}
	if len(raw) > 1024*1024 {
		return nil, errors.New("rule batch AnswerSheet query exceeds fixed 1MiB; choose fewer complete components")
	}
	return filter, nil
}
