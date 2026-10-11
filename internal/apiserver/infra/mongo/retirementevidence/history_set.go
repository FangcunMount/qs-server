package retirementevidence

import (
	"fmt"
	"reflect"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/bson"
)

func splitHistoricalSet(raw bson.Raw, slot string) (bson.Raw, *evidence.HistoricalReferenceSetV1, error) {
	if err := raw.Validate(); err != nil {
		return nil, nil, ErrUnverifiable
	}
	if raw.Lookup("_id").Type == 0 {
		return nil, nil, ErrUnverifiable
	}
	var fields bson.D
	if err := bson.Unmarshal(raw, &fields); err != nil {
		return nil, nil, err
	}
	if err := unambiguous(fields); err != nil {
		return nil, nil, err
	}
	stripped, value, err := strip(fields, []string{slot})
	if err != nil {
		return nil, nil, err
	}
	var set *evidence.HistoricalReferenceSetV1
	if raw.Lookup(slot).Type != 0 {
		if raw.Lookup(slot).Type != bson.TypeEmbeddedDocument {
			return nil, nil, ErrUnverifiable
		}
		body, err := bson.Marshal(value)
		if err != nil {
			return nil, nil, err
		}
		set = new(evidence.HistoricalReferenceSetV1)
		if err := bson.Unmarshal(body, set); err != nil {
			return nil, nil, err
		}
		if err := validateHistoricalSetBSON(set); err != nil {
			return nil, nil, err
		}
		// Unknown fields cannot retain an unbounded body in this compact slot.
		canonical, err := bson.Marshal(set)
		if err != nil {
			return nil, nil, err
		}
		var want, actual bson.M
		if err := bson.Unmarshal(canonical, &want); err != nil {
			return nil, nil, err
		}
		if err := bson.Unmarshal(body, &actual); err != nil {
			return nil, nil, err
		}
		if !reflect.DeepEqual(want, actual) {
			return nil, nil, ErrUnverifiable
		}
	}
	base, err := bson.Marshal(stripped)
	return base, set, err
}

// DecodeHistoricalSetDocument validates a narrow projection without allowing
// ignored BSON fields to hide bodies or weaken the persisted typed contract.
func DecodeHistoricalSetDocument(raw bson.Raw, slot string) (*evidence.HistoricalReferenceSetV1, error) {
	if slot != "legacy_submission_evidence" && slot != "historical_generated_evidence" {
		return nil, ErrUnverifiable
	}
	_, set, err := splitHistoricalSet(raw, slot)
	if err == nil && set == nil {
		return nil, ErrUnverifiable
	}
	return set, err
}

func validateHistoricalSetBSON(set *evidence.HistoricalReferenceSetV1) error {
	if set == nil {
		return nil
	}
	if err := set.Validate(); err != nil {
		return err
	}
	encoded, err := bson.Marshal(set)
	if err != nil {
		return err
	}
	if len(encoded) > 512*1024 {
		return fmt.Errorf("%w: historical BSON set exceeds bounded capacity", ErrConflict)
	}
	return nil
}
