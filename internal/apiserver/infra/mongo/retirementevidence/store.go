// Package retirementevidence supplies a maintenance-only, borrowed-transaction
// CAS. It owns neither connections nor transaction/session lifecycle.
package retirementevidence

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	ErrConflict     = errors.New("historical event evidence baseline or conclusion conflict")
	ErrUnverifiable = errors.New("historical event evidence has no complete trustworthy business binding")
)

// Snapshot retains exact selected BSON, including field presence, scalar BSON
// types, unknown fields and stored millisecond clocks. The caller cannot edit it.
type Snapshot struct {
	raw                  bson.Raw
	baseline             bson.Raw
	path                 []string
	client               *mongo.Client
	database, collection string
}

// Read is a point read, never a historical scanner. An empty evidencePath makes
// the complete document an immutable dependency; a nonempty path must be empty.
func Read(ctx context.Context, collection *mongo.Collection, filter any, evidencePath string) (*Snapshot, error) {
	if collection == nil {
		return nil, fmt.Errorf("%w: collection is missing", ErrUnverifiable)
	}
	var raw bson.Raw
	if err := collection.FindOne(ctx, filter).Decode(&raw); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("%w: business record is absent: %w", ErrUnverifiable, err)
		}
		return nil, err
	}
	var path []string
	if evidencePath != "" {
		path = strings.Split(evidencePath, ".")
	}
	baseline, proof, err := withoutEvidence(raw, path)
	if err != nil {
		return nil, err
	}
	if proof != nil {
		return nil, fmt.Errorf("%w: baseline is already classified", ErrConflict)
	}
	return &Snapshot{raw: append(bson.Raw(nil), raw...), baseline: baseline, path: path, client: collection.Database().Client(), database: collection.Database().Name(), collection: collection.Name()}, nil
}

func (s *Snapshot) Decode(target any) error {
	if s == nil {
		return fmt.Errorf("%w: snapshot is missing", ErrUnverifiable)
	}
	if err := bson.Unmarshal(s.raw, target); err != nil {
		return fmt.Errorf("%w: invalid stored business schema: %w", ErrUnverifiable, err)
	}
	return nil
}

// RequireTransaction delegates real active-transaction validation to the pinned
// SDK adapter, then also rejects a session from a different client's pool.
func RequireTransaction(ctx context.Context, collection *mongo.Collection) error {
	if ctx == nil || collection == nil {
		return sdkmongo.ErrTransactionRequired
	}
	tx, ok := ctx.(mongo.SessionContext)
	if !ok || nilInterface(tx) {
		return sdkmongo.ErrTransactionRequired
	}
	session := mongo.SessionFromContext(tx)
	if nilInterface(session) {
		return sdkmongo.ErrTransactionRequired
	}
	if _, err := sdkmongo.Bind(tx, collection); err != nil {
		return err
	}
	if session.Client() != collection.Database().Client() {
		return sdkmongo.ErrTransactionRequired
	}
	return nil
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) && v.IsNil()
}

func (s *Snapshot) checkCollection(collection *mongo.Collection) error {
	if s == nil || collection == nil || s.client != collection.Database().Client() || s.database != collection.Database().Name() || s.collection != collection.Name() {
		return ErrConflict
	}
	return nil
}

func (s *Snapshot) current(ctx context.Context, collection *mongo.Collection) (bson.Raw, *evidence.EventEvidenceV1, error) {
	return s.currentWithOriginalID(ctx, collection, "")
}

func (s *Snapshot) currentWithOriginalID(ctx context.Context, collection *mongo.Collection, originalID string) (bson.Raw, *evidence.EventEvidenceV1, error) {
	if err := s.checkCollection(collection); err != nil {
		return nil, nil, err
	}
	var current bson.Raw
	if err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: s.raw.Lookup("_id")}}).Decode(&current); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil, ErrConflict
		}
		return nil, nil, err
	}
	base, proof, err := withoutEvidence(current, s.path)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: invalid current evidence", ErrConflict)
	}
	if originalID != "" && proof != nil {
		if current.Lookup("generated_event_id").Type != bson.TypeString || current.Lookup("generated_event_id").StringValue() != originalID {
			return nil, nil, ErrConflict
		}
		// Only an atomically copied ID accompanied by a conclusion can normalize
		// to the old missing/empty field. A lone independently changed ID cannot.
		var fields bson.D
		if err := bson.Unmarshal(base, &fields); err != nil {
			return nil, nil, err
		}
		original := s.raw.Lookup("generated_event_id")
		restored := make(bson.D, 0, len(fields))
		for _, field := range fields {
			if field.Key == "generated_event_id" {
				if original.Type == 0 {
					continue
				}
				field.Value = original
			}
			restored = append(restored, field)
		}
		base, err = bson.Marshal(restored)
		if err != nil {
			return nil, nil, err
		}
	}
	if !bytes.Equal(base, s.baseline) {
		return nil, nil, ErrConflict
	}
	return current, proof, nil
}

// CheckDependency rechecks a full immutable dependency in the borrowed snapshot.
// Artifact and committed Outcome immutability remain host contracts; this helper
// does not claim a transaction spanning Mongo and MySQL.
func (s *Snapshot) CheckDependency(ctx context.Context, collection *mongo.Collection) error {
	if err := RequireTransaction(ctx, collection); err != nil {
		return err
	}
	if s == nil || len(s.path) != 0 {
		return ErrConflict
	}
	_, _, err := s.current(ctx, collection)
	return err
}

// ValidateHistorical validates a conclusion against the independently stored ID
// and the standard versioned business projection. It never manufactures an ID.
func ValidateHistorical(proof *evidence.EventEvidenceV1, eventID, binding string) error {
	if err := proof.Validate(); err != nil {
		return err
	}
	if proof.Class != evidence.RetiredVerified && proof.Class != evidence.Unverifiable {
		return fmt.Errorf("maintenance CAS requires a historical conclusion")
	}
	if proof.EventID != eventID || proof.BusinessBindingSHA256 != binding {
		return ErrConflict
	}
	return nil
}

// Apply writes only the existing evidence slot. For identical conclusions it
// obtains a real write lock with a transaction-local evidence fence, then restores
// the exact validated conclusion. Mongo no-op updates do not fence stale reads.
// The host MUST abort on any error; the helper never commits or rolls back.
func (s *Snapshot) Apply(ctx context.Context, collection *mongo.Collection, proof *evidence.EventEvidenceV1) error {
	return s.apply(ctx, collection, proof, "")
}

// ApplyGeneratedOriginal is deliberately specific: only the generation's old
// missing/empty ID may be copied alongside its historical conclusion. The
// business adapter must validate an explicit trusted source reference first.
func (s *Snapshot) ApplyGeneratedOriginal(ctx context.Context, collection *mongo.Collection, proof *evidence.EventEvidenceV1, originalID string) error {
	if s == nil || collection == nil || collection.Name() != "report_generations" || len(s.path) != 1 || s.path[0] != "generated_event_evidence" || originalID == "" || proof == nil || proof.EventID != originalID {
		return ErrConflict
	}
	stored := s.raw.Lookup("generated_event_id")
	if stored.Type != 0 && (stored.Type != bson.TypeString || stored.StringValue() != "") {
		return ErrConflict
	}
	return s.apply(ctx, collection, proof, originalID)
}

func (s *Snapshot) apply(ctx context.Context, collection *mongo.Collection, proof *evidence.EventEvidenceV1, originalID string) error {
	if err := RequireTransaction(ctx, collection); err != nil {
		return err
	}
	if s == nil || len(s.path) == 0 {
		return ErrConflict
	}
	if err := proof.Validate(); err != nil {
		return err
	}
	if proof.Class != evidence.RetiredVerified && proof.Class != evidence.Unverifiable {
		return fmt.Errorf("maintenance CAS requires a historical conclusion")
	}
	current, existing, err := s.currentWithOriginalID(ctx, collection, originalID)
	if err != nil {
		return err
	}
	if existing != nil {
		if err := existing.Validate(); err != nil {
			return ErrConflict
		}
		want, err := bson.Marshal(proof)
		if err != nil {
			return err
		}
		var expectedFields, actualFields bson.M
		if err := bson.Unmarshal(want, &expectedFields); err != nil {
			return err
		}
		if err := bson.Unmarshal(current.Lookup(s.path...).Document(), &actualFields); err != nil {
			return ErrConflict
		}
		if !reflect.DeepEqual(expectedFields, actualFields) {
			return ErrConflict
		}
	}
	filter := bson.D{{Key: "_id", Value: current.Lookup("_id")}, {Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$$ROOT", bson.D{{Key: "$literal", Value: current}}}}}}}
	slot := strings.Join(s.path, ".")
	value := any(proof.Clone())
	var fence string
	if existing != nil {
		body, err := bson.Marshal(proof)
		if err != nil {
			return err
		}
		var fenced bson.D
		if err := bson.Unmarshal(body, &fenced); err != nil {
			return err
		}
		fence = primitive.NewObjectID().Hex()
		fenced = append(fenced, bson.E{Key: "_retirement_cas_fence", Value: fence})
		value = fenced
	}
	set := bson.D{{Key: slot, Value: value}}
	if originalID != "" {
		set = append(set, bson.E{Key: "generated_event_id", Value: originalID})
	}
	result, err := collection.UpdateOne(ctx, filter, bson.D{{Key: "$set", Value: set}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return ErrConflict
	}
	if fence != "" {
		result, err = collection.UpdateOne(ctx, bson.D{{Key: "_id", Value: current.Lookup("_id")}, {Key: slot + "._retirement_cas_fence", Value: fence}}, bson.D{{Key: "$set", Value: bson.D{{Key: slot, Value: proof.Clone()}}}})
		if err != nil {
			return err
		}
		if result.MatchedCount != 1 {
			return ErrConflict
		}
	}
	return nil
}

func withoutEvidence(raw bson.Raw, path []string) (bson.Raw, *evidence.EventEvidenceV1, error) {
	if err := raw.Validate(); err != nil {
		return nil, nil, fmt.Errorf("%w: malformed BSON", ErrUnverifiable)
	}
	if raw.Lookup("_id").Type == 0 {
		return nil, nil, fmt.Errorf("%w: missing BSON identity", ErrUnverifiable)
	}
	var document bson.D
	if err := bson.Unmarshal(raw, &document); err != nil {
		return nil, nil, err
	}
	if err := unambiguous(document); err != nil {
		return nil, nil, err
	}
	stripped, value, err := strip(document, path)
	if err != nil {
		return nil, nil, err
	}
	var proof *evidence.EventEvidenceV1
	if value != nil {
		body, err := bson.Marshal(value)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: malformed evidence slot", ErrUnverifiable)
		}
		proof = new(evidence.EventEvidenceV1)
		if err := bson.Unmarshal(body, proof); err != nil {
			return nil, nil, fmt.Errorf("%w: malformed evidence slot", ErrUnverifiable)
		}
	}
	baseline, err := bson.Marshal(stripped)
	return baseline, proof, err
}

func unambiguous(value any) error {
	switch value := value.(type) {
	case bson.D:
		seen := map[string]bool{}
		for _, field := range value {
			if seen[field.Key] {
				return fmt.Errorf("%w: duplicate BSON field", ErrUnverifiable)
			}
			seen[field.Key] = true
			if err := unambiguous(field.Value); err != nil {
				return err
			}
		}
	case bson.A:
		for _, child := range value {
			if err := unambiguous(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func strip(document bson.D, path []string) (bson.D, any, error) {
	seen := map[string]bool{}
	result := make(bson.D, 0, len(document))
	var proof any
	for _, field := range document {
		if seen[field.Key] {
			return nil, nil, fmt.Errorf("%w: duplicate BSON field", ErrUnverifiable)
		}
		seen[field.Key] = true
		if len(path) > 0 && field.Key == path[0] {
			if len(path) == 1 {
				proof = field.Value
				continue
			}
			child, ok := field.Value.(bson.D)
			if !ok {
				return nil, nil, fmt.Errorf("%w: missing evidence parent", ErrUnverifiable)
			}
			var err error
			field.Value, proof, err = strip(child, path[1:])
			if err != nil {
				return nil, nil, err
			}
		}
		result = append(result, field)
	}
	if len(path) > 1 && !seen[path[0]] {
		return nil, nil, fmt.Errorf("%w: missing evidence parent", ErrUnverifiable)
	}
	return result, proof, nil
}
