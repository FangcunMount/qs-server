// Package interpretation owns the final presentation aggregate after evaluation model execution.
//
// It assembles already-interpreted results into InterpretReport, including
// v2 outcome summary fields (model identity, primary_score, level),
// display dimensions, structured suggestions, and optional model-specific
// extras for personality-style reports. Model-family builders live in
// application/interpretation/reporting and personality typology adapters.
package interpretation
