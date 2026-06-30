package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	aminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/assessmentmodel"
	mongoassessmentmodel "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/assessmentmodel"
)

type modelSeedPlan struct {
	Code      string
	Algorithm domain.Algorithm
	Title     string
	Build     func() (*modeltypology.Payload, error)
}

func seedAssessmentModel(
	ctx context.Context,
	draftRepo *mongoassessmentmodel.DraftRepository,
	publishedRepo *mongoassessmentmodel.PublishedModelRepoAdapter,
	plan modelSeedPlan,
	payload *modeltypology.Payload,
	force bool,
	now time.Time,
) error {
	definitionBytes, err := payloadDefinitionBytes(payload)
	if err != nil {
		return err
	}
	binding := domain.QuestionnaireBinding{
		QuestionnaireCode:    payload.QuestionnaireCode,
		QuestionnaireVersion: payload.QuestionnaireVersion,
	}

	existing, findErr := draftRepo.FindByCode(ctx, plan.Code)
	if findErr != nil && !errors.Is(findErr, domain.ErrNotFound) {
		return fmt.Errorf("find draft %s: %w", plan.Code, findErr)
	}
	if existing != nil && !force {
		fmt.Printf("skip model %s (draft exists, pass --force to replace)\n", plan.Code)
		return nil
	}
	if existing != nil && force {
		if err := purgeDraftForSeed(ctx, draftRepo, plan.Code); err != nil {
			return fmt.Errorf("purge draft %s: %w", plan.Code, err)
		}
	}

	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code:      plan.Code,
		Kind:      domain.KindPersonality,
		SubKind:   domain.SubKindTypology,
		Algorithm: plan.Algorithm,
		Title:     firstNonEmpty(plan.Title, payload.Title),
		Now:       now,
	})
	if err != nil {
		return fmt.Errorf("new model %s: %w", plan.Code, err)
	}
	if err := model.BindQuestionnaire(binding, now); err != nil {
		return fmt.Errorf("bind questionnaire %s: %w", plan.Code, err)
	}
	if err := model.UpdateDefinition(domain.DefinitionPayload{
		Format: domain.PayloadFormatPersonalityTypologyV1,
		Data:   definitionBytes,
	}, now); err != nil {
		return fmt.Errorf("update definition %s: %w", plan.Code, err)
	}
	if err := draftRepo.Create(ctx, model); err != nil {
		return fmt.Errorf("create draft %s: %w", plan.Code, err)
	}

	if force {
		if err := publishedRepo.DeletePublished(ctx, domain.KindPersonality, plan.Code); err != nil {
			return fmt.Errorf("delete published %s: %w", plan.Code, err)
		}
	}

	publishPayloadBytes, err := fullPayloadDefinitionBytes(payload)
	if err != nil {
		return err
	}
	publishModel := *model
	publishModel.Definition = domain.DefinitionPayload{
		Format: domain.PayloadFormatPersonalityTypologyV1,
		Data:   publishPayloadBytes,
	}
	snapshot, err := aminfra.BuildPersonalityPublishedSnapshot(&publishModel)
	if err != nil {
		return fmt.Errorf("build published snapshot %s: %w", plan.Code, err)
	}
	if err := publishedRepo.Save(ctx, snapshot); err != nil {
		return fmt.Errorf("save published %s: %w", plan.Code, err)
	}
	if err := model.MarkPublished(now); err != nil {
		return fmt.Errorf("mark published %s: %w", plan.Code, err)
	}
	if err := draftRepo.Update(ctx, model); err != nil {
		return fmt.Errorf("update draft %s: %w", plan.Code, err)
	}

	fmt.Printf("seeded model %s -> questionnaire %s@%s (draft v%d, explicit runtime)\n",
		plan.Code,
		payload.QuestionnaireCode,
		payload.QuestionnaireVersion,
		model.Version,
	)
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func purgeDraftForSeed(ctx context.Context, draftRepo *mongoassessmentmodel.DraftRepository, code string) error {
	if code == "" {
		return domain.ErrNotFound
	}
	if err := draftRepo.Delete(ctx, code); err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	_, err := draftRepo.Collection().DeleteMany(ctx, bson.M{"code": code})
	return err
}
