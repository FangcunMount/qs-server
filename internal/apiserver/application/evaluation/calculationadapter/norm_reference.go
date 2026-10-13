package calculationadapter

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

func normReferenceFromOutcome(reference *domainoutcome.NormReference) *calculation.NormReference {
	if reference == nil {
		return nil
	}
	return &calculation.NormReference{
		ScoreKind: calculation.ScoreKind(reference.ScoreKind), Benchmark: reference.Benchmark,
		TableVersion: reference.TableVersion, FormVariant: reference.FormVariant,
		MinAgeMonths: reference.MinAgeMonths, MaxAgeMonths: reference.MaxAgeMonths,
		Gender: reference.Gender,
	}
}

func normReferenceToOutcome(reference *calculation.NormReference) *domainoutcome.NormReference {
	if reference == nil {
		return nil
	}
	return &domainoutcome.NormReference{
		ScoreKind: domainoutcome.ScoreKind(reference.ScoreKind), Benchmark: reference.Benchmark,
		TableVersion: reference.TableVersion, FormVariant: reference.FormVariant,
		MinAgeMonths: reference.MinAgeMonths, MaxAgeMonths: reference.MaxAgeMonths,
		Gender: reference.Gender,
	}
}
