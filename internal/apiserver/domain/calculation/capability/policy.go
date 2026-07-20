package capability

// MissingAnswerPolicy describes how a path+usage treats absent answers/children.
type MissingAnswerPolicy string

const (
	// MissingAnswerSkip omits missing question answers from aggregation (scale collect).
	MissingAnswerSkip MissingAnswerPolicy = "skip"
	// MissingAnswerFail hard-fails when a required answer or child score is absent.
	MissingAnswerFail MissingAnswerPolicy = "fail"
)

// MissingAnswerPolicyFor returns the runtime missing-answer contract for path+usage.
func MissingAnswerPolicyFor(path Path, usage Usage) MissingAnswerPolicy {
	switch {
	case path == PathTypologyDescriptor && usage == UsageTypologyLeaf:
		return MissingAnswerFail
	case usage == UsageQuestionAggregation:
		return MissingAnswerSkip
	default:
		// Composite / typology composite require child scores to be present.
		return MissingAnswerFail
	}
}

// RequiresExecutableScoring reports whether a FactorRole must carry executable
// Measure Scoring (non-empty sources) on the given execution path.
// Role strings match modelcatalog factor.FactorRole values.
func RequiresExecutableScoring(path Path, role string) bool {
	if role == "" {
		role = "dimension"
	}
	if role == "report_group" {
		return false
	}
	switch path {
	case PathScaleDescriptor, PathBehavioralRatingDescriptor:
		switch role {
		case "dimension", "total", "validity", "subtest", "task_set", "index", "ability_domain":
			return true
		default:
			return false
		}
	case PathTypologyDescriptor:
		switch role {
		case "dimension", "index":
			return true
		default:
			return false
		}
	case PathCognitiveDescriptor:
		// Raven SPM total/ability_domain are produced by ExecutionSpec, not Measure Scoring.
		switch role {
		case "dimension", "subtest", "task_set", "index", "validity":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

// PathForKind maps canonical model Kind strings to execution paths.
func PathForKind(kind string) (Path, bool) {
	switch kind {
	case "scale":
		return PathScaleDescriptor, true
	case "typology":
		return PathTypologyDescriptor, true
	case "behavioral_rating":
		return PathBehavioralRatingDescriptor, true
	case "cognitive":
		return PathCognitiveDescriptor, true
	default:
		return "", false
	}
}

// AuthoringStrategyCodes returns strategy codes suitable for Definition editing
// options on a path (leaf + composite usages, de-duplicated, stable order).
func AuthoringStrategyCodes(path Path) []string {
	usages := []Usage{UsageQuestionAggregation, UsageCompositeProjection}
	switch path {
	case PathTypologyDescriptor:
		usages = []Usage{UsageTypologyLeaf, UsageTypologyComposite}
	}
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, usage := range usages {
		for _, code := range SupportedCodes(path, usage) {
			if _, ok := seen[code]; ok {
				continue
			}
			seen[code] = struct{}{}
			out = append(out, code)
		}
	}
	return out
}

// DeclaredAuthoringStrategyCodes is the union of AuthoringStrategyCodes across
// all paths (stable order). OpenAPI ScoringStrategy enums and Go declaration
// constants must match this set — not the historical max/min/first/last extras.
func DeclaredAuthoringStrategyCodes() []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, path := range AllPaths() {
		for _, code := range AuthoringStrategyCodes(path) {
			if _, ok := seen[code]; ok {
				continue
			}
			seen[code] = struct{}{}
			out = append(out, code)
		}
	}
	return out
}
