package interpretationreadmodel

import (
	"errors"
	"strings"
	"testing"
)

func TestCatalogDanglingSourceErrorCarriesConsistencyIdentity(t *testing.T) {
	err := error(&CatalogDanglingSourceError{AssessmentID: 7, SourceKind: "archive", SourceID: 9})
	var target *CatalogDanglingSourceError
	if !errors.As(err, &target) {
		t.Fatal("expected typed dangling source error")
	}
	if target.AssessmentID != 7 || target.SourceKind != "archive" || target.SourceID != 9 {
		t.Fatalf("unexpected identity: %#v", target)
	}
}

func TestCatalogSourceAssociationMismatchErrorCarriesFieldsWithoutBody(t *testing.T) {
	err := error(&CatalogSourceAssociationMismatchError{
		AssessmentID:     11,
		SourceKind:       "artifact",
		SourceID:         13,
		MismatchedFields: []string{"assessment_id", "org_id", "testee_id"},
	})
	var target *CatalogSourceAssociationMismatchError
	if !errors.As(err, &target) {
		t.Fatal("expected typed association mismatch error")
	}
	if target.AssessmentID != 11 || target.SourceKind != "artifact" || target.SourceID != 13 {
		t.Fatalf("unexpected identity: %#v", target)
	}
	if len(target.MismatchedFields) != 3 {
		t.Fatalf("unexpected fields: %#v", target.MismatchedFields)
	}
	msg := strings.ToLower(target.Error())
	for _, banned := range []string{"conclusion", "dimension", "suggestion"} {
		if strings.Contains(msg, banned) {
			t.Fatalf("error message leaked body-ish token %q: %s", banned, target.Error())
		}
	}
}
