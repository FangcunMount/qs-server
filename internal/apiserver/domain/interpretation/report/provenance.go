package report

const (
	BuilderIdentityFactorScoring   = "factor-scoring"
	BuilderIdentityNormProfile     = "norm-profile"
	BuilderIdentityTypology        = "typology"
	BuilderIdentityTaskPerformance = "task-performance"

	ContentSchemaVersionV1 = "report-content/v1"

	UnknownBuilderIdentity       = "unknown"
	LegacyContentSchemaVersion   = "legacy"
)

func normalizeLegacyProvenance(builderIdentity, contentSchemaVersion string) (string, string) {
	if builderIdentity == "" {
		builderIdentity = UnknownBuilderIdentity
	}
	if contentSchemaVersion == "" {
		contentSchemaVersion = LegacyContentSchemaVersion
	}
	return builderIdentity, contentSchemaVersion
}
