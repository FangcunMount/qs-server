package binding

// NormalizeKind maps deprecated persisted values to canonical kinds.
func NormalizeKind(kind Kind) Kind {
	switch kind {
	case KindPersonality:
		return KindTypology
	default:
		return kind
	}
}

// KindsEqual reports whether two kind values refer to the same model family.
func KindsEqual(left, right Kind) bool {
	return NormalizeKind(left) == NormalizeKind(right)
}

// IsTypologyKind reports whether kind is typology (canonical or legacy persisted value).
func IsTypologyKind(kind Kind) bool {
	return NormalizeKind(kind) == KindTypology
}

// KindQueryValues returns persisted values that should match a kind filter.
func KindQueryValues(kind Kind) []string {
	normalized := NormalizeKind(kind)
	if normalized == KindTypology {
		return []string{string(KindTypology), string(KindPersonality)}
	}
	return []string{string(normalized)}
}
