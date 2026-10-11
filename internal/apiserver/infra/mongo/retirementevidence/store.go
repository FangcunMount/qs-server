// Package retirementevidence supplies a maintenance-only, borrowed-transaction
// CAS. It owns neither connections nor transaction/session lifecycle.
package retirementevidence

import (
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
)

var (
	ErrConflict     = errors.New("historical event evidence baseline or conclusion conflict")
	ErrUnverifiable = errors.New("historical event evidence has no complete trustworthy business binding")
)

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
