package binding

// Flat migration-only kinds must not be used when creating new draft models.
const (
	KindMBTIMigration = "mbti"
	KindSBTIMigration = "sbti"
)

// KindMapping resolves deprecated flat kinds to v2 identity triples (scale migration read path only).
func KindMapping(kind string) (mappedKind, subKind, algorithm string, ok bool) {
	switch kind {
	case "scale":
		return "scale", "", "scale_default", true
	default:
		return "", "", "", false
	}
}

// LegacyKindMapping resolves deprecated flat kinds to v2 identity triples.
func LegacyKindMapping(kind Kind) (Kind, SubKind, Algorithm, bool) {
	mappedKind, subKind, algorithm, ok := KindMapping(string(kind))
	if !ok {
		return "", "", "", false
	}
	return Kind(mappedKind), SubKind(subKind), Algorithm(algorithm), true
}

// IsMigrationOnlyKind reports flat kinds that must not be used for new draft models.
func IsMigrationOnlyKind(kind string) bool {
	switch kind {
	case KindMBTIMigration, KindSBTIMigration:
		return true
	default:
		return false
	}
}
