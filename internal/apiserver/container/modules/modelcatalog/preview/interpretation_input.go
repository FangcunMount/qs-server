package preview

import (
	"fmt"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact/codec"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelpreview"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func previewInterpretationInput(req modelpreview.Request, outcome *domainoutcome.Execution) (interpinput.InterpretationInput, error) {
	if outcome == nil {
		return interpinput.InterpretationInput{}, fmt.Errorf("preview evaluation outcome is required")
	}
	model := report.ModelIdentity{Kind: string(outcome.ModelRef.Kind()), SubKind: string(outcome.ModelRef.SubKind()), Algorithm: string(outcome.ModelRef.Algorithm()), Code: outcome.ModelRef.Code().String(), Version: outcome.ModelRef.Version(), Title: outcome.ModelRef.Title()}
	model.ProductChannel = string(binding.ProductChannelForIdentity(modelcatalog.Kind(model.Kind), ""))
	model.AlgorithmFamily = binding.AlgorithmFamilyStringFromIdentity(modelcatalog.Kind(model.Kind), modelcatalog.SubKind(model.SubKind), modelcatalog.Algorithm(model.Algorithm))
	in := interpinput.InterpretationInput{Association: report.Association{OrgID: 1, AssessmentID: meta.ID(1), TesteeID: 1}, Model: model, Runtime: interpinput.RuntimeIdentity{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification}, Report: interpinput.ReportSpec{ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersionV1, Algorithm: modelcatalog.Algorithm(model.Algorithm), ProductChannel: modelcatalog.ProductChannel(model.ProductChannel)}}
	if outcome.Primary != nil {
		in.Result.Primary = &report.ScoreValue{Kind: string(outcome.Primary.Kind), Value: outcome.Primary.Value, Label: outcome.Primary.Label, Max: outcome.Primary.Max}
	}
	if outcome.Level != nil {
		in.Result.Level = &report.ResultLevel{Code: outcome.Level.Code, Label: outcome.Level.Label, Severity: outcome.Level.Severity}
	}
	if payload, ok := evaluationinput.TypologyPayload(req.Input); ok && payload != nil {
		if spec, err := payload.ToRuntimeSpec(); err == nil {
			in.Runtime.DecisionKind = spec.Decision.Kind
			in.Report.ReportProfile = policy.ReportProfileForDecisionKind(spec.Decision.Kind)
			in.Report.TemplateID = spec.Report.TemplateID
			in.Report.AdapterKey = string(spec.Report.ResolvedAdapterKey(spec.OutcomeMapping, spec.Decision.Kind))
		}
	}
	if detail, ok := codec.PersonalityTypeDetailFromPayload(outcome.Detail.Payload); ok {
		in.PersonalityType = &interpinput.PersonalityTypeFacts{Detail: personalityDetail(detail)}
		if in.Runtime.DecisionKind == "" {
			in.Runtime.DecisionKind = modelcatalog.DecisionKindPoleComposition
		}
		return in, nil
	}
	if detail, ok := codec.TraitProfileDetailFromPayload(outcome.Detail.Payload); ok {
		traits := make([]reporttypology.TraitProfileFactorReport, 0, len(detail.Traits))
		for _, trait := range detail.Traits {
			traits = append(traits, reporttypology.TraitProfileFactorReport(trait))
		}
		in.TraitProfile = &interpinput.TraitProfileFacts{Detail: reporttypology.TraitProfileReportDetail{Traits: traits, Source: reporttypology.TraitProfileSourceReport{Attribution: detail.Source.Attribution, License: detail.Source.License, NonCommercial: detail.Source.NonCommercial}}}
		if in.Runtime.DecisionKind == "" {
			in.Runtime.DecisionKind = modelcatalog.DecisionKindTraitProfile
		}
		return in, nil
	}
	return interpinput.InterpretationInput{}, fmt.Errorf("unsupported typology preview detail %T", outcome.Detail.Payload)
}

func personalityDetail(d codec.PersonalityTypeDetail) reporttypology.PersonalityTypeReportDetail {
	dims := make([]reporttypology.PersonalityTypeDimensionReport, 0, len(d.Dimensions))
	for _, v := range d.Dimensions {
		dims = append(dims, reporttypology.PersonalityTypeDimensionReport{Code: v.Code, Name: v.Name, LeftPole: v.LeftPole, RightPole: v.RightPole, RawScore: v.RawScore, Preference: v.Preference, Strength: v.Strength, Model: v.Model, Level: v.Level})
	}
	summary := d.Summary
	if summary == "" {
		summary = d.Commentary
	}
	return reporttypology.PersonalityTypeReportDetail{TypeCode: d.TypeCode, TypeName: d.TypeName, OneLiner: d.OneLiner, MatchPercent: d.MatchPercent, ImageURL: d.ImageURL, IsSpecial: d.IsSpecial, SpecialTrigger: d.SpecialTrigger, Commentary: d.Commentary, Profile: reporttypology.PersonalityTypeProfileReport{Summary: summary, Strengths: append([]string(nil), d.Strengths...), Weaknesses: append([]string(nil), d.Weaknesses...), Suggestions: append([]string(nil), d.Suggestions...)}, Rarity: reporttypology.PersonalityTypeRarityReport{Percent: d.Rarity.Percent, Label: d.Rarity.Label, OneInX: d.Rarity.OneInX}, Dimensions: dims, SourceAttribution: d.Source.Attribution, SourceLicense: d.Source.License, SourceNonCommercial: d.Source.NonCommercial}
}
