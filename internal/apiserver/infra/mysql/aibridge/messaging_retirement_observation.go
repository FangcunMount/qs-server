package aibridge

// ValidateCommandRetirementObservation validates a stored record's closed
// schema only. It grants no terminal, execution, writer-fence or write proof.
func ValidateCommandRetirementObservation(e CommandRetirementEvidence) error {
	return e.validate(e.Conclusion == "transferred_verified")
}
