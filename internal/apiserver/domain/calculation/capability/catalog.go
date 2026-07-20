package capability

// Path identifies an Evaluation execution path. Values match modelcatalog
// binding.ExecutionPath strings so Calculation stays free of modelcatalog imports.
type Path string

const (
	PathScaleDescriptor            Path = "scale_descriptor"
	PathTypologyDescriptor         Path = "typology_descriptor"
	PathBehavioralRatingDescriptor Path = "behavioral_rating_descriptor"
	PathCognitiveDescriptor        Path = "cognitive_descriptor"
)

// Usage identifies how a strategy code is consumed inside a path.
type Usage string

const (
	UsageQuestionAggregation Usage = "question_aggregation"
	UsageCompositeProjection Usage = "composite_projection"
	UsageTypologyLeaf        Usage = "typology_leaf"
	UsageTypologyComposite   Usage = "typology_composite"
)

// Entry is one supported strategy code and its accepted aliases.
type Entry struct {
	Code    string
	Aliases []string
}

type matrixKey struct {
	path  Path
	usage Usage
}

// catalog is the single source of truth for runtime-supported strategies.
// It mirrors what execution paths actually accept today (not the authoring enum).
var catalog = map[matrixKey][]Entry{
	{PathScaleDescriptor, UsageQuestionAggregation}: {
		{Code: "sum"},
		{Code: "avg", Aliases: []string{"average"}},
		{Code: "cnt", Aliases: []string{"count"}},
	},
	{PathScaleDescriptor, UsageCompositeProjection}: {
		{Code: "sum"},
		{Code: "avg", Aliases: []string{"average"}},
		{Code: "weighted_sum"},
		{Code: "none"},
		{Code: "lookup"},
		{Code: "custom"},
	},
	{PathTypologyDescriptor, UsageTypologyLeaf}: {
		{Code: "sum"},
	},
	{PathTypologyDescriptor, UsageTypologyComposite}: {
		{Code: "sum"},
		{Code: "avg", Aliases: []string{"average"}},
		{Code: "weighted_avg"},
	},
	// Behavioral/cognitive question aggregation currently shares the scale
	// collect subset (sum/avg/cnt) on the public ScaleFactorScorer path.
	{PathBehavioralRatingDescriptor, UsageQuestionAggregation}: {
		{Code: "sum"},
		{Code: "avg", Aliases: []string{"average"}},
		{Code: "cnt", Aliases: []string{"count"}},
	},
	{PathBehavioralRatingDescriptor, UsageCompositeProjection}: {
		{Code: "sum"},
		{Code: "avg", Aliases: []string{"average"}},
		{Code: "weighted_sum"},
		{Code: "none"},
		{Code: "lookup"},
		{Code: "custom"},
	},
	{PathCognitiveDescriptor, UsageQuestionAggregation}: {
		{Code: "sum"},
		{Code: "avg", Aliases: []string{"average"}},
		{Code: "cnt", Aliases: []string{"count"}},
	},
	{PathCognitiveDescriptor, UsageCompositeProjection}: {
		{Code: "sum"},
		{Code: "avg", Aliases: []string{"average"}},
		{Code: "weighted_sum"},
		{Code: "none"},
		{Code: "lookup"},
		{Code: "custom"},
	},
}

// Supports reports whether strategy is accepted for path+usage (including aliases).
func Supports(path Path, usage Usage, strategy string) bool {
	_, ok := Canonical(path, usage, strategy)
	return ok
}

// Canonical returns the catalog code for a strategy or alias.
func Canonical(path Path, usage Usage, strategy string) (string, bool) {
	if strategy == "" {
		return "", false
	}
	for _, entry := range catalog[matrixKey{path, usage}] {
		if entry.Code == strategy {
			return entry.Code, true
		}
		for _, alias := range entry.Aliases {
			if alias == strategy {
				return entry.Code, true
			}
		}
	}
	return "", false
}

// SupportedCodes returns canonical strategy codes for path+usage (stable order).
func SupportedCodes(path Path, usage Usage) []string {
	entries := catalog[matrixKey{path, usage}]
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Code)
	}
	return out
}

// AllPaths returns execution paths that have at least one capability entry.
func AllPaths() []Path {
	return []Path{
		PathScaleDescriptor,
		PathTypologyDescriptor,
		PathBehavioralRatingDescriptor,
		PathCognitiveDescriptor,
	}
}
