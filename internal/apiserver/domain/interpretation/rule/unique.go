package rule

// UniqueSuggestions deduplicates suggestions by category, content, and factor code.
func UniqueSuggestions(suggestions []Suggestion) []Suggestion {
	return uniqueSuggestions(suggestions)
}
