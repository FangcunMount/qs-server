package evolution

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// FrozenIdentity is the Algorithm + Questionnaire code locked by the first retained release.
type FrozenIdentity struct {
	Algorithm         domain.Algorithm
	QuestionnaireCode string
}

// Policy protects model identity continuity after the first retained publish (MC-R008).
// It uses retained release history, never head.status.
type Policy struct {
	History modelcatalogport.PublishedReleaseHistoryReader
}

// ResolveFrozenIdentity returns the identity frozen by the oldest retained release.
func (p Policy) ResolveFrozenIdentity(ctx context.Context, modelCode string) (FrozenIdentity, bool, error) {
	if modelCode == "" {
		return FrozenIdentity{}, false, nil
	}
	if p.History == nil {
		// Zero-value Policy allows edits; production wiring always injects History.
		return FrozenIdentity{}, false, nil
	}
	items, err := p.History.ListPublishedReleaseHistory(ctx, modelCode)
	if err != nil {
		if domain.IsNotFound(err) {
			return FrozenIdentity{}, false, nil
		}
		return FrozenIdentity{}, false, err
	}
	if len(items) == 0 {
		return FrozenIdentity{}, false, nil
	}
	// History is published_at DESC; the last item is the first retained publish.
	first := items[len(items)-1]
	if first == nil {
		return FrozenIdentity{}, false, nil
	}
	return FrozenIdentity{
		Algorithm:         first.Algorithm,
		QuestionnaireCode: first.QuestionnaireCode,
	}, true, nil
}

// GuardAlgorithmChange rejects Algorithm edits after the first retained publish.
// Empty proposed keeps the current value and is allowed.
func (p Policy) GuardAlgorithmChange(ctx context.Context, modelCode string, proposed domain.Algorithm) error {
	if proposed == "" {
		return nil
	}
	frozen, ok, err := p.ResolveFrozenIdentity(ctx, modelCode)
	if err != nil || !ok {
		return err
	}
	if frozen.Algorithm != "" && proposed != frozen.Algorithm {
		return errors.WithCode(code.ErrConflict, "algorithm is frozen after first publish: want %s, got %s", frozen.Algorithm, proposed)
	}
	return nil
}

// GuardQuestionnaireCodeChange rejects Questionnaire code changes after first publish.
// Same code with a new version is allowed.
func (p Policy) GuardQuestionnaireCodeChange(ctx context.Context, modelCode, proposedCode string) error {
	if proposedCode == "" {
		return nil
	}
	frozen, ok, err := p.ResolveFrozenIdentity(ctx, modelCode)
	if err != nil || !ok {
		return err
	}
	if frozen.QuestionnaireCode != "" && proposedCode != frozen.QuestionnaireCode {
		return errors.WithCode(code.ErrConflict, "questionnaire code is frozen after first publish: want %s, got %s", frozen.QuestionnaireCode, proposedCode)
	}
	return nil
}

// GuardPublishIdentity is the final publish-time check that draft identity still matches
// the retained freeze (covers paths that bypass edit APIs).
func (p Policy) GuardPublishIdentity(ctx context.Context, model *domain.AssessmentModel) error {
	if model == nil {
		return nil
	}
	frozen, ok, err := p.ResolveFrozenIdentity(ctx, model.Code)
	if err != nil || !ok {
		return err
	}
	if frozen.Algorithm != "" && model.Algorithm != frozen.Algorithm {
		return errors.WithCode(code.ErrConflict, "algorithm is frozen after first publish: want %s, got %s", frozen.Algorithm, model.Algorithm)
	}
	if frozen.QuestionnaireCode != "" && model.Binding.QuestionnaireCode != frozen.QuestionnaireCode {
		return errors.WithCode(code.ErrConflict, "questionnaire code is frozen after first publish: want %s, got %s", frozen.QuestionnaireCode, model.Binding.QuestionnaireCode)
	}
	return nil
}
