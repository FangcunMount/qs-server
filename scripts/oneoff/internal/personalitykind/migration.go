// Package personalitykind audits and normalizes the retired catalog value
// "personality" to the canonical typology identity.
package personalitykind

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

const legacyPersonalityKind = "personality"

type CollectionSpec struct {
	Name                string
	CodeField           string
	KindField           string
	SubKindField        string
	AlgorithmField      string
	ProductChannelField string
}

var (
	Drafts = CollectionSpec{
		Name:                "assessment_models",
		CodeField:           "code",
		KindField:           "kind",
		SubKindField:        "sub_kind",
		AlgorithmField:      "algorithm",
		ProductChannelField: "product_channel",
	}
	Published = CollectionSpec{
		Name:                "published_assessment_models",
		CodeField:           "model_code",
		KindField:           "model_kind",
		SubKindField:        "model_sub_kind",
		AlgorithmField:      "model_algorithm",
		ProductChannelField: "model_product_channel",
	}
)

type Record struct {
	ID             primitive.ObjectID
	Code           string
	Kind           string
	SubKind        string
	Algorithm      string
	ProductChannel string
}

type Finding struct {
	Record
	Collection string
	Eligible   bool
	Reason     string
}

func Findings(ctx context.Context, collection *mongo.Collection, spec CollectionSpec) ([]Finding, error) {
	if collection == nil {
		return nil, fmt.Errorf("%s collection is not configured", spec.Name)
	}
	filter := bson.M{
		"deleted_at": nil,
		"$or": []bson.M{
			{spec.KindField: legacyPersonalityKind},
			{spec.KindField: string(domain.KindTypology), spec.ProductChannelField: legacyPersonalityKind},
		},
	}
	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("find %s legacy kind values: %w", spec.Name, err)
	}
	defer cursor.Close(ctx)

	findings := make([]Finding, 0)
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, fmt.Errorf("decode %s legacy kind value: %w", spec.Name, err)
		}
		findings = append(findings, classify(spec, raw))
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s legacy kind values: %w", spec.Name, err)
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Code == findings[j].Code {
			return findings[i].ID.Hex() < findings[j].ID.Hex()
		}
		return findings[i].Code < findings[j].Code
	})
	return findings, nil
}

func Apply(ctx context.Context, collection *mongo.Collection, spec CollectionSpec, finding Finding) error {
	if !finding.Eligible {
		return fmt.Errorf("%s/%s is not eligible: %s", finding.Collection, finding.Code, finding.Reason)
	}
	filter := bson.M{"_id": finding.ID, "deleted_at": nil, spec.KindField: finding.Kind}
	if finding.ProductChannel == "" {
		filter[spec.ProductChannelField] = bson.M{"$in": bson.A{nil, ""}}
	} else {
		filter[spec.ProductChannelField] = finding.ProductChannel
	}
	if finding.Kind == legacyPersonalityKind {
		filter[spec.SubKindField] = finding.SubKind
		filter[spec.AlgorithmField] = finding.Algorithm
	}
	set := bson.M{
		spec.ProductChannelField: string(domain.ProductChannelTypology),
		"updated_at":             time.Now().UTC(),
	}
	if finding.Kind == legacyPersonalityKind {
		set[spec.KindField] = string(domain.KindTypology)
	}
	result, err := collection.UpdateOne(ctx, filter, bson.M{"$set": set})
	if err != nil {
		return fmt.Errorf("update %s/%s: %w", spec.Name, finding.Code, err)
	}
	if result.MatchedCount != 1 {
		return fmt.Errorf("update %s/%s did not match its audited version", spec.Name, finding.Code)
	}
	return nil
}

func classify(spec CollectionSpec, raw bson.M) Finding {
	record := Record{
		ID:             objectID(raw["_id"]),
		Code:           stringValue(raw[spec.CodeField]),
		Kind:           stringValue(raw[spec.KindField]),
		SubKind:        stringValue(raw[spec.SubKindField]),
		Algorithm:      stringValue(raw[spec.AlgorithmField]),
		ProductChannel: stringValue(raw[spec.ProductChannelField]),
	}
	finding := Finding{Record: record, Collection: spec.Name}
	if record.Kind == string(domain.KindTypology) && record.ProductChannel == legacyPersonalityKind {
		finding.Eligible = true
		finding.Reason = "canonical typology kind has retired personality product channel"
		return finding
	}
	if record.Kind != legacyPersonalityKind {
		finding.Reason = "not a retired personality catalog kind"
		return finding
	}
	if record.SubKind != string(domain.SubKindTypology) {
		finding.Reason = "retired personality kind is missing sub_kind=typology"
		return finding
	}
	if !isTypologyAlgorithm(record.Algorithm) {
		finding.Reason = "retired personality kind has a non-typology algorithm"
		return finding
	}
	if record.ProductChannel != "" && record.ProductChannel != legacyPersonalityKind && record.ProductChannel != string(domain.ProductChannelTypology) {
		finding.Reason = "retired personality kind has an incompatible product channel"
		return finding
	}
	finding.Eligible = true
	finding.Reason = "retired personality typology identity"
	return finding
}

func isTypologyAlgorithm(value string) bool {
	switch domain.Algorithm(value) {
	case domain.AlgorithmPersonalityTypology, domain.AlgorithmBigFive, domain.AlgorithmMBTI, domain.AlgorithmSBTI:
		return true
	default:
		return false
	}
}

func objectID(value any) primitive.ObjectID {
	id, _ := value.(primitive.ObjectID)
	return id
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}
