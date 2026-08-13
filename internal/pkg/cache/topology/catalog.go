// Package topology owns the low-cardinality logical cache topology catalog.
// It is code-owned and deliberately excluded from cache policy YAML/hash input.
package topology

type SourceSpec struct {
	TopologyGroup string `json:"topology_group"`
	ReadModel     string `json:"read_model"`
	SourceKind    string `json:"source_kind"`
}

var sources = []SourceSpec{
	{TopologyGroup: "questionnaire", ReadModel: "questionnaire published Mongo read model", SourceKind: "mongo_read_model"},
	{TopologyGroup: "published-model", ReadModel: "published-model Mongo snapshot", SourceKind: "mongo_snapshot"},
	{TopologyGroup: "assessment-detail", ReadModel: "MySQL evaluation assessment read model", SourceKind: "mysql_read_model"},
	{TopologyGroup: "assessment-access", ReadModel: "MySQL assessment ownership lookup", SourceKind: "mysql_lookup"},
}

func Sources() []SourceSpec { return append([]SourceSpec(nil), sources...) }

func Lookup(group string) (SourceSpec, bool) {
	for _, source := range sources {
		if source.TopologyGroup == group {
			return source, true
		}
	}
	return SourceSpec{}, false
}
