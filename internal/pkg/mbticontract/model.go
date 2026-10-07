// Package mbticontract binds report facts to the finite immutable MBTI models
// supported by AI. It does not calculate preferences or preference strength.
package mbticontract

type Axis struct {
	Code, Left, Right   string
	Min, Max, Threshold float64
}

type Model struct{ Axes [4]Axis }

func Lookup(code, version string) (Model, bool) {
	switch {
	case code == "MBTI_OEJTS" && version == "v64-report-202608-v1":
		return Model{Axes: [4]Axis{{"EI", "I", "E", 8, 40, 24}, {"SN", "S", "N", 8, 40, 24}, {"TF", "F", "T", 8, 40, 24}, {"JP", "J", "P", 8, 40, 24}}}, true
	case code == "MBTI_FC_93" && version == "v55-report-202608-v1":
		return Model{Axes: [4]Axis{{"EI", "E", "I", 0, 23, 11.5}, {"SN", "S", "N", 0, 23, 11.5}, {"TF", "T", "F", 0, 23, 11.5}, {"JP", "J", "P", 0, 24, 11.5}}}, true
	default:
		return Model{}, false
	}
}

func IsModelCode(code string) bool { return code == "MBTI_OEJTS" || code == "MBTI_FC_93" }
