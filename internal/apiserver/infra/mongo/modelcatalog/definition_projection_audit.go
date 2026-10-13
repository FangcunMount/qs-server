package modelcatalog

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"go.mongodb.org/mongo-driver/bson"
)

const projectionAuditPathLimit = 32

// DefinitionProjectionAudit contains only hashes and field paths. It never
// returns presentation copy or a replacement Definition that could be saved.
type DefinitionProjectionAudit struct {
	CanonicalHash   string   `json:"canonical_hash,omitempty"`
	InvalidCount    int      `json:"invalid_count"`
	InvalidPaths    []string `json:"invalid_paths,omitempty"`
	DifferenceCount int      `json:"difference_count"`
	DifferencePaths []string `json:"difference_paths,omitempty"`
	HashError       bool     `json:"hash_error"`
}

// AuditDefinitionProjections compares the stored layers with the pure canonical
// projections. In particular, it never uses ResolvedInterpretationAssets or
// rematerializes through the storage encoder, which could hide lost fields.
func AuditDefinitionProjections(raw bson.Raw) DefinitionProjectionAudit {
	var result DefinitionProjectionAudit
	if err := raw.Validate(); err != nil {
		result.invalid("definition_v2")
		return result
	}
	auditBSONShape(raw, reflect.TypeOf(DefinitionPO{}), "definition_v2", &result)
	var po DefinitionPO
	if err := bson.Unmarshal(raw, &po); err != nil {
		result.invalid("definition_v2")
		return result
	}
	for i, item := range po.Conclusions {
		switch conclusion.Kind(item.Kind) {
		case conclusion.KindRisk, conclusion.KindNorm, conclusion.KindAbility, conclusion.KindType:
		default:
			result.invalid(indexPath("definition_v2.conclusions", i) + ".kind")
		}
	}
	def := definitionFromPO(&po)
	for _, issue := range modeldefinition.Validate(*def) {
		result.invalid("definition_v2." + issue.Field)
	}
	auditDuplicateCodes(def, &result)
	if result.InvalidCount > 0 {
		return result
	}
	hash, err := modeldefinition.CanonicalContentHash(def)
	if err != nil {
		result.HashError = true
		return result
	}
	result.CanonicalHash = hash
	auditProjectionValues(reflect.ValueOf(def.DecisionSpec), reflect.ValueOf(modeldefinition.DecisionSpecFrom(def)), "definition_v2.decision_spec", &result)
	auditProjectionValues(reflect.ValueOf(def.InterpretationAssets), reflect.ValueOf(modeldefinition.InterpretationAssetsFrom(def)), "definition_v2.interpretation_assets", &result)
	return result
}

func (r *DefinitionProjectionAudit) invalid(path string) {
	r.InvalidCount++
	if len(r.InvalidPaths) < projectionAuditPathLimit {
		r.InvalidPaths = append(r.InvalidPaths, path)
	}
}

func (r *DefinitionProjectionAudit) different(path string) {
	r.DifferenceCount++
	if len(r.DifferencePaths) < projectionAuditPathLimit {
		r.DifferencePaths = append(r.DifferencePaths, path)
	}
}

// Collection nil/empty representations are equivalent. Pointer presence,
// scalar content, array order and duplicates remain observable differences.
func auditProjectionValues(stored, expected reflect.Value, path string, result *DefinitionProjectionAudit) {
	switch stored.Kind() {
	case reflect.Struct:
		for i := range stored.NumField() {
			auditProjectionValues(stored.Field(i), expected.Field(i), path+"."+stored.Type().Field(i).Name, result)
		}
	case reflect.Slice, reflect.Array:
		if stored.Len() != expected.Len() {
			result.different(path + ".length")
		}
		for i := range min(stored.Len(), expected.Len()) {
			auditProjectionValues(stored.Index(i), expected.Index(i), indexPath(path, i), result)
		}
	case reflect.Pointer:
		if stored.IsNil() != expected.IsNil() {
			result.different(path)
		} else if !stored.IsNil() {
			auditProjectionValues(stored.Elem(), expected.Elem(), path, result)
		}
	default:
		if !reflect.DeepEqual(stored.Interface(), expected.Interface()) {
			result.different(path)
		}
	}
}

// BSON decoding normally discards unknown fields and last-wins duplicate keys.
// Refuse both rather than letting the mapper turn unexamined content into zero.
func auditBSONShape(raw bson.Raw, typ reflect.Type, path string, result *DefinitionProjectionAudit) {
	fields := map[string]reflect.Type{}
	for i := range typ.NumField() {
		field := typ.Field(i)
		name := strings.Split(field.Tag.Get("bson"), ",")[0]
		if name != "" && name != "-" {
			fields[name] = field.Type
		}
	}
	seen := map[string]bool{}
	elements, err := raw.Elements()
	if err != nil {
		result.invalid(path)
		return
	}
	for _, element := range elements {
		key := element.Key()
		fieldType, known := fields[key]
		if !known || seen[key] {
			result.invalid(path + "." + key)
			continue
		}
		seen[key] = true
		auditBSONValue(element.Value(), fieldType, path+"."+key, result)
	}
}

func auditBSONValue(value bson.RawValue, typ reflect.Type, path string, result *DefinitionProjectionAudit) {
	if value.Type == bson.TypeNull {
		if typ.Kind() != reflect.Pointer && typ.Kind() != reflect.Slice && typ.Kind() != reflect.Map {
			result.invalid(path)
		}
		return
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Struct:
		if value.Type != bson.TypeEmbeddedDocument {
			result.invalid(path)
			return
		}
		auditBSONShape(value.Document(), typ, path, result)
	case reflect.Slice:
		if value.Type != bson.TypeArray {
			result.invalid(path)
			return
		}
		values, err := value.Array().Values()
		if err != nil {
			result.invalid(path)
			return
		}
		for i, item := range values {
			auditBSONValue(item, typ.Elem(), indexPath(path, i), result)
		}
	}
}

func auditDuplicateCodes(def *modeldefinition.Definition, result *DefinitionProjectionAudit) {
	check := func(codes []string, path string) {
		seen := map[string]bool{}
		for i, code := range codes {
			if code == "" || seen[code] {
				result.invalid(indexPath(path, i))
			}
			seen[code] = true
		}
	}
	var codes []string
	for _, item := range def.InterpretationAssets.Outcomes {
		codes = append(codes, item.OutcomeCode)
	}
	check(codes, "definition_v2.interpretation_assets.Outcomes")
	codes = nil
	for _, item := range def.InterpretationAssets.Profiles {
		codes = append(codes, item.OutcomeCode)
	}
	check(codes, "definition_v2.interpretation_assets.Profiles")
	for i, item := range def.Conclusions {
		if typed, ok := item.(conclusion.TypeConclusion); ok {
			codes = nil
			for _, profile := range typed.Profiles {
				codes = append(codes, profile.OutcomeCode)
			}
			check(codes, indexPath("definition_v2.conclusions", i)+".profiles")
			codes = nil
			for _, outcome := range typed.Outcomes {
				codes = append(codes, outcome.Code)
			}
			check(codes, indexPath("definition_v2.conclusions", i)+".outcomes")
		}
	}
}

func indexPath(path string, index int) string {
	return path + "[" + strconv.Itoa(index) + "]"
}
