package management

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestUpdateBasicInfoForScaleAdvancesRevisionOnce(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code: "SNAP-IV", Kind: domain.KindScale, Algorithm: domain.AlgorithmScaleDefault, Title: "Before", Now: now,
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel() error = %v", err)
	}
	initialRevision := model.Revision()
	repo := &revisionCheckingModelRepo{model: model, persistedRevision: initialRevision}
	service := Service{
		ModelRepo:  repo,
		Authorizer: allowManagementAuthorizer{},
		Now:        func() time.Time { return now.Add(time.Minute) },
	}

	_, err = service.UpdateBasicInfo(context.Background(), modelcatalog.ActorContext{}, modelcatalog.UpdateBasicInfoDTO{
		Code: "SNAP-IV", Title: "SNAP-IV量表（26项）", Description: "请您根据孩子最近一段时间的情况作答",
		Category: "adhd", Stages: []string{"follow_up"}, ApplicableAges: []string{"school_child", "adolescent"},
		Reporters: []string{"parent", "teacher"}, Tags: []string{"注意缺陷", "多动冲动", "对立违抗"},
	})
	if err != nil {
		t.Fatalf("UpdateBasicInfo() error = %v", err)
	}
	if got, want := model.Revision(), initialRevision+1; got != want {
		t.Fatalf("revision = %d, want %d", got, want)
	}
	if got, want := model.Stages, []string{"follow_up"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("stages = %#v, want %#v", got, want)
	}
	if got, want := model.ApplicableAges, []string{"school_child", "adolescent"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("applicable ages = %#v, want %#v", got, want)
	}
	if got, want := model.Reporters, []string{"parent", "teacher"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("reporters = %#v, want %#v", got, want)
	}
}

type allowManagementAuthorizer struct{}

func (allowManagementAuthorizer) Authorize(context.Context, modelcatalog.ActorContext, modelcatalog.Action, modelcatalog.Resource) error {
	return nil
}

type revisionCheckingModelRepo struct {
	model             *domain.AssessmentModel
	persistedRevision int64
}

func (*revisionCheckingModelRepo) Create(context.Context, *domain.AssessmentModel) error { return nil }

func (r *revisionCheckingModelRepo) Update(_ context.Context, model *domain.AssessmentModel) error {
	if got, want := model.Revision(), r.persistedRevision+1; got != want {
		return fmt.Errorf("optimistic-lock revision = %d, want %d", got, want)
	}
	r.model = model
	r.persistedRevision = model.Revision()
	return nil
}

func (r *revisionCheckingModelRepo) FindByCode(_ context.Context, code string) (*domain.AssessmentModel, error) {
	if r.model == nil || r.model.Code != code {
		return nil, domain.ErrNotFound
	}
	return r.model, nil
}

func (*revisionCheckingModelRepo) FindByQuestionnaireCode(context.Context, domain.Kind, string) (*domain.AssessmentModel, error) {
	return nil, domain.ErrNotFound
}

func (*revisionCheckingModelRepo) List(context.Context, modelcatalogport.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	return nil, 0, nil
}

func (*revisionCheckingModelRepo) Delete(context.Context, string) error { return nil }
