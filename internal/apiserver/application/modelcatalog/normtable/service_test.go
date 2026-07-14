package normtable

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	baseerrors "github.com/FangcunMount/component-base/pkg/errors"
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	modelnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type allowAuthorizer struct{}

func (allowAuthorizer) Authorize(context.Context, modelcatalog.ActorContext, modelcatalog.Action, modelcatalog.Resource) error {
	return nil
}

type memoryNormRepository struct {
	tables map[string]*modelnorm.Norm
}

func newMemoryNormRepository() *memoryNormRepository {
	return &memoryNormRepository{tables: make(map[string]*modelnorm.Norm)}
}

func (r *memoryNormRepository) UpsertNorm(_ context.Context, table *modelnorm.Norm) error {
	existing, ok := r.tables[table.TableVersion]
	if ok && !reflect.DeepEqual(existing, table) {
		return fmt.Errorf("%w: %s", domain.ErrNormVersionConflict, table.TableVersion)
	}
	if !ok {
		copy := *table
		r.tables[table.TableVersion] = &copy
	}
	return nil
}

func (r *memoryNormRepository) FindNorm(_ context.Context, tableVersion string) (*modelnorm.Norm, error) {
	table, ok := r.tables[tableVersion]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return table, nil
}

func (r *memoryNormRepository) ListNorms(_ context.Context, filter port.NormListFilter) ([]*modelnorm.Norm, int64, error) {
	items := make([]*modelnorm.Norm, 0, len(r.tables))
	for _, table := range r.tables {
		if filter.Kind != "" && table.Kind != filter.Kind {
			continue
		}
		if filter.Algorithm != "" && table.Algorithm != filter.Algorithm {
			continue
		}
		if filter.FormVariant != "" && table.FormVariant != filter.FormVariant {
			continue
		}
		items = append(items, table)
	}
	return items, int64(len(items)), nil
}

func TestImportIsIdempotentAndRejectsVersionReuse(t *testing.T) {
	repository := newMemoryNormRepository()
	service := Service{Repository: repository, Authorizer: allowAuthorizer{}}
	table := validNormTable()

	first, err := service.Import(context.Background(), modelcatalog.ActorContext{}, table)
	if err != nil {
		t.Fatalf("first Import() error = %v", err)
	}
	second, err := service.Import(context.Background(), modelcatalog.ActorContext{}, table)
	if err != nil {
		t.Fatalf("second Import() error = %v", err)
	}
	if first.TableVersion != second.TableVersion || len(repository.tables) != 1 {
		t.Fatalf("idempotent import result = (%+v, %+v), stored = %d", first, second, len(repository.tables))
	}

	conflicting := validNormTable()
	conflicting.Factors[0].Lookup[0].TScore = 61
	_, err = service.Import(context.Background(), modelcatalog.ActorContext{}, conflicting)
	if err == nil {
		t.Fatal("conflicting Import() error = nil")
	}
	if got := baseerrors.ParseCoder(err).Code(); got != code.ErrConflict {
		t.Fatalf("conflicting Import() code = %d, want %d", got, code.ErrConflict)
	}
}

func TestImportRejectsInvalidNormTable(t *testing.T) {
	service := Service{Repository: newMemoryNormRepository(), Authorizer: allowAuthorizer{}}
	table := validNormTable()
	table.Factors[0].Lookup[0].Percentile = 101

	_, err := service.Import(context.Background(), modelcatalog.ActorContext{}, table)
	if err == nil {
		t.Fatal("Import() error = nil")
	}
	if got := baseerrors.ParseCoder(err).Code(); got != code.ErrInvalidArgument {
		t.Fatalf("Import() code = %d, want %d", got, code.ErrInvalidArgument)
	}
}

func TestListAndGetNormTables(t *testing.T) {
	repository := newMemoryNormRepository()
	service := Service{Repository: repository, Authorizer: allowAuthorizer{}}
	if _, err := service.Import(context.Background(), modelcatalog.ActorContext{}, validNormTable()); err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	list, err := service.List(context.Background(), modelcatalog.ActorContext{}, modelcatalog.ListNormTablesDTO{Algorithm: string(identity.AlgorithmBrief2)})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Page != 1 || list.PageSize != 20 {
		t.Fatalf("List() = %+v", list)
	}
	detail, err := service.Get(context.Background(), modelcatalog.ActorContext{}, list.Items[0].TableVersion)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if detail.Algorithm != string(identity.AlgorithmBrief2) || len(detail.Factors) != 1 {
		t.Fatalf("Get() = %+v", detail)
	}
}

func validNormTable() *modelnorm.Norm {
	return &modelnorm.Norm{
		TableVersion: "brief2-parent-2026", FormVariant: "parent",
		Kind: identity.KindBehavioralRating, Algorithm: identity.AlgorithmBrief2,
		Factors: []modelnorm.FactorTable{{FactorCode: "gec", Lookup: []modelnorm.LookupEntry{{RawScoreMin: 10, RawScoreMax: 10, TScore: 55, Percentile: 69}}}},
	}
}
