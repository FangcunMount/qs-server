package definition

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestCanonicalContentHashV1PreservesNilAndEmptyCollections(t *testing.T) {
	t.Parallel()
	nilHash, err := CanonicalContentHash(&Definition{})
	if err != nil {
		t.Fatal(err)
	}
	for name, def := range map[string]*Definition{
		"outcomes":        {Outcomes: []conclusion.Outcome{}},
		"conclusions":     {Conclusions: []conclusion.Conclusion{}},
		"factors":         {Measure: MeasureSpec{Factors: []factor.Factor{}}},
		"report sections": {ReportMap: ReportMap{Sections: []ReportSection{}}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := CanonicalContentHash(def)
			if err != nil {
				t.Fatal(err)
			}
			if got == nilHash {
				t.Fatal("v1 hash conflated null and empty collection")
			}
		})
	}
	if hash, err := CanonicalContentHash(nil); hash != "" || err != nil {
		t.Fatalf("nil Definition = (%q,%v)", hash, err)
	}
}

func TestCanonicalContentHashV1PreservesEncodingErrorContract(t *testing.T) {
	t.Parallel()
	cases := map[string]*Definition{
		"unsupported pointer conclusion": {Conclusions: []conclusion.Conclusion{&conclusion.RiskConclusion{}}},
		"nonfinite authoring score":      {Outcomes: []conclusion.Outcome{}, Measure: MeasureSpec{Scoring: []factor.Scoring{{Constant: math.NaN()}}}},
	}
	for name, def := range cases {
		t.Run(name, func(t *testing.T) {
			_, legacyErr := json.Marshal(*def)
			hash, err := CanonicalContentHash(def)
			if hash != "" || legacyErr == nil || err == nil || err.Error() != legacyErr.Error() {
				t.Fatalf("encoding error changed: hash=%q got=%v want=%v", hash, err, legacyErr)
			}
			var typed *json.MarshalerError
			if !errors.As(err, &typed) || typed.Type != reflect.TypeOf(Definition{}) {
				t.Fatalf("encoding error lost Definition marshaler identity: %T", err)
			}
		})
	}
}
