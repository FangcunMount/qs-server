package compat

// ReportScaleName maps neutral report model name to legacy scale_name wire field.
func ReportScaleName(modelName string) string {
	return modelName
}

// ReportScaleCode maps neutral report model code to legacy scale_code wire field.
func ReportScaleCode(modelCode string) string {
	return modelCode
}
