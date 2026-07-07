package legacy

// Flat migration-only kinds must not be used when creating new draft models.
const (
	KindMBTIMigration = "mbti"
	KindSBTIMigration = "sbti"
)

// KindMapping resolves deprecated flat kinds to v2 identity triples for read/migration paths.
func KindMapping(kind string) (mappedKind, subKind, algorithm string, ok bool) {
	switch kind {
	case "scale":
		return "scale", "", "scale_default", true
	case KindMBTIMigration:
		return "personality", "typology", "mbti", true
	case KindSBTIMigration:
		return "personality", "typology", "sbti", true
	default:
		return "", "", "", false
	}
}

// IsMigrationOnlyKind reports legacy flat kinds that must not be used for new draft models.
func IsMigrationOnlyKind(kind string) bool {
	switch kind {
	case KindMBTIMigration, KindSBTIMigration:
		return true
	default:
		return false
	}
}
