package interpretationreadmodel

import (
	"errors"
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
