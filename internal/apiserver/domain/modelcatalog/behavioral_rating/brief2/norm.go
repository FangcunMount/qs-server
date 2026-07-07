package brief2

import calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"

type (
	Subject             = calcnorm.Subject
	NormTables          = calcnorm.NormTables
	FactorNormTable     = calcnorm.FactorNormTable
	NormBand            = calcnorm.NormBand
	NormLookupEntry     = calcnorm.NormLookupEntry
	TScoreInterpretRule = calcnorm.TScoreInterpretRule
	TScoreRange         = calcnorm.TScoreRange
	NormScore           = calcnorm.NormScore
)

var (
	LookupNormScore = calcnorm.LookupNormScore
	InterpretTScore = calcnorm.InterpretTScore
	ValidateTables  = calcnorm.ValidateTables
)
