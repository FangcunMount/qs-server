package legacyadapter

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// AssessmentSnapshotPublisher publishes legacy scales through the target
// AssessmentModel -> AssessmentSnapshot path while old scale writes still exist.
type AssessmentSnapshotPublisher struct {
	ModelRepo     port.ModelRepository
	PublishedRepo port.PublishedModelRepository
	Now           func() time.Time
}

func NewAssessmentSnapshotPublisher(modelRepo port.ModelRepository, publishedRepo port.PublishedModelRepository) *AssessmentSnapshotPublisher {
	return &AssessmentSnapshotPublisher{
		ModelRepo:     modelRepo,
		PublishedRepo: publishedRepo,
	}
}

func (p *AssessmentSnapshotPublisher) PublishAssessmentSnapshot(ctx context.Context, scale *scaledefinition.MedicalScale) error {
	if p == nil {
		return fmt.Errorf("assessment snapshot publisher is nil")
	}
	if p.PublishedRepo == nil {
		return fmt.Errorf("published model repository is nil")
	}
	model, err := AssessmentModelFromMedicalScale(scale, p.now())
	if err != nil {
		return err
	}
	if err := p.upsertAssessmentModel(ctx, model); err != nil {
		return err
	}
	publisher := publication.Publisher{Repo: p.PublishedRepo}
	snapshot, err := publisher.BuildSnapshot(ctx, model)
	if err != nil {
		return err
	}
	if err := p.PublishedRepo.DeletePublished(ctx, domain.KindScale, model.Code); err != nil {
		return err
	}
	return publisher.Save(ctx, snapshot)
}

func (p *AssessmentSnapshotPublisher) upsertAssessmentModel(ctx context.Context, model *domain.AssessmentModel) error {
	if p.ModelRepo == nil {
		return nil
	}
	existing, err := p.ModelRepo.FindByCode(ctx, model.Code)
	if err != nil {
		if stderrors.Is(err, domain.ErrNotFound) {
			return p.ModelRepo.Create(ctx, model)
		}
		return err
	}
	if existing != nil {
		model.ID = existing.ID
		model.CreatedAt = existing.CreatedAt
		model.Version = existing.Version + 1
	}
	return p.ModelRepo.Update(ctx, model)
}

func (p *AssessmentSnapshotPublisher) now() time.Time {
	if p.Now != nil {
		return p.Now().UTC()
	}
	return time.Now().UTC()
}
