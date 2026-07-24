package interpretation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	appreporttemplate "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporttemplate"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreporttemplate "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/reporttemplate"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

const reportTemplateCollection = "interpretation_report_templates"

// ReportTemplatePO is the Mongo document for one template release asset.
type ReportTemplatePO struct {
	DomainID        uint64     `bson:"domain_id"`
	TemplateID      string     `bson:"template_id"`
	TemplateVersion string     `bson:"template_version"`
	BuilderIdentity string     `bson:"builder_identity"`
	AdapterKey      string     `bson:"adapter_key,omitempty"`
	Status          string     `bson:"status"`
	CreatedAt       time.Time  `bson:"created_at"`
	UpdatedAt       time.Time  `bson:"updated_at"`
	PublishedAt     *time.Time `bson:"published_at,omitempty"`
	PublishedBy     string     `bson:"published_by,omitempty"`
	DisabledAt      *time.Time `bson:"disabled_at,omitempty"`
	DisabledBy      string     `bson:"disabled_by,omitempty"`
}

func (ReportTemplatePO) CollectionName() string { return reportTemplateCollection }

// ReportTemplateRepository persists ReportTemplate release assets.
type ReportTemplateRepository struct {
	base.BaseRepository
}

func NewReportTemplateRepository(db *mongo.Database, opts ...base.BaseRepositoryOptions) (*ReportTemplateRepository, error) {
	repo := &ReportTemplateRepository{BaseRepository: base.NewBaseRepository(db, reportTemplateCollection, opts...)}
	if _, err := repo.Collection().Indexes().CreateMany(context.Background(), reportTemplateIndexModels()); err != nil {
		return nil, fmt.Errorf("create interpretation report template indexes: %w", err)
	}
	if err := repo.ensureBootstrap(context.Background()); err != nil {
		return nil, err
	}
	return repo, nil
}

func reportTemplateIndexModels() []mongo.IndexModel {
	return []mongo.IndexModel{
		{Keys: bson.D{{Key: "domain_id", Value: 1}}, Options: options.Index().SetName("uk_report_template_domain_id").SetUnique(true)},
		{Keys: bson.D{{Key: "template_id", Value: 1}, {Key: "template_version", Value: 1}}, Options: options.Index().SetName("uk_report_template_release").SetUnique(true)},
		{Keys: bson.D{{Key: "template_id", Value: 1}, {Key: "status", Value: 1}}, Options: options.Index().SetName("idx_report_template_status")},
	}
}

var _ domainreporttemplate.Repository = (*ReportTemplateRepository)(nil)

func (r *ReportTemplateRepository) Save(ctx context.Context, template *domainreporttemplate.ReportTemplate) error {
	if template == nil {
		return fmt.Errorf("report template is required")
	}
	po := reportTemplateToPO(template)
	filter := bson.M{"template_id": po.TemplateID, "template_version": po.TemplateVersion}
	upsert := false
	switch template.Status() {
	case domainreporttemplate.StatusDraft:
		filter["domain_id"] = po.DomainID
		filter["status"] = string(domainreporttemplate.StatusDraft)
		upsert = true
	case domainreporttemplate.StatusPublished:
		filter["status"] = string(domainreporttemplate.StatusDraft)
	case domainreporttemplate.StatusDisabled:
		filter["status"] = string(domainreporttemplate.StatusPublished)
	default:
		return fmt.Errorf("save report template: invalid status")
	}
	result, err := r.Collection().ReplaceOne(ctx, filter, po, options.Replace().SetUpsert(upsert))
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return domainreporttemplate.ErrConflict
		}
		return fmt.Errorf("save report template: %w", err)
	}
	if !upsert && result.MatchedCount == 0 {
		return domainreporttemplate.ErrConflict
	}
	return nil
}

func (r *ReportTemplateRepository) FindByKey(ctx context.Context, templateID string, version policy.TemplateVersion) (*domainreporttemplate.ReportTemplate, error) {
	var po ReportTemplatePO
	if err := r.FindOne(ctx, bson.M{"template_id": templateID, "template_version": version.String()}, &po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domainreporttemplate.ErrNotFound
		}
		return nil, fmt.Errorf("find report template: %w", err)
	}
	return reportTemplateToDomain(&po)
}

func (r *ReportTemplateRepository) FindPublished(ctx context.Context, templateID string, version policy.TemplateVersion) (*domainreporttemplate.ReportTemplate, error) {
	var po ReportTemplatePO
	if err := r.FindOne(ctx, bson.M{
		"template_id": templateID, "template_version": version.String(), "status": string(domainreporttemplate.StatusPublished),
	}, &po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domainreporttemplate.ErrNotFound
		}
		return nil, fmt.Errorf("find published report template: %w", err)
	}
	return reportTemplateToDomain(&po)
}

func (r *ReportTemplateRepository) ListByTemplateID(ctx context.Context, templateID string, limit int) ([]*domainreporttemplate.ReportTemplate, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	cur, err := r.Collection().Find(ctx,
		bson.M{"template_id": templateID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "domain_id", Value: -1}}).SetLimit(int64(limit)),
	)
	if err != nil {
		return nil, fmt.Errorf("list report template versions: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()
	items := make([]*domainreporttemplate.ReportTemplate, 0)
	for cur.Next(ctx) {
		var po ReportTemplatePO
		if err := cur.Decode(&po); err != nil {
			return nil, err
		}
		item, err := reportTemplateToDomain(&po)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, cur.Err()
}

func (r *ReportTemplateRepository) IsPublished(templateID string, version string) bool {
	_, err := r.FindPublished(context.Background(), templateID, policy.TemplateVersion(version))
	return err == nil
}

func (r *ReportTemplateRepository) ensureBootstrap(ctx context.Context) error {
	svc := appreporttemplate.NewService(r)
	now := time.Now().UTC()
	for _, seed := range appreporttemplate.BootstrapDrafts {
		if _, err := r.FindByKey(ctx, seed.TemplateID, seed.TemplateVersion); err == nil {
			continue
		} else if !errors.Is(err, domainreporttemplate.ErrNotFound) {
			return err
		}
		draft, err := svc.CreateDraft(ctx, appreporttemplate.CreateDraftCommand{
			Actor:      appreporttemplate.Actor{OperatorUserID: 1},
			TemplateID: seed.TemplateID, TemplateVersion: seed.TemplateVersion,
			BuilderIdentity: seed.BuilderIdentity, AdapterKey: seed.AdapterKey,
		})
		if err != nil {
			return err
		}
		if _, err := svc.Publish(ctx, appreporttemplate.PublishCommand{
			Actor:      appreporttemplate.Actor{OperatorUserID: 1},
			TemplateID: draft.TemplateID(), TemplateVersion: draft.TemplateVersion(),
		}); err != nil {
			return err
		}
		_ = now
	}
	return nil
}

func reportTemplateToPO(template *domainreporttemplate.ReportTemplate) *ReportTemplatePO {
	return &ReportTemplatePO{
		DomainID: template.ID().Uint64(), TemplateID: template.TemplateID(), TemplateVersion: template.TemplateVersion().String(),
		BuilderIdentity: template.BuilderIdentity(), AdapterKey: template.AdapterKey(), Status: string(template.Status()),
		CreatedAt: template.CreatedAt(), UpdatedAt: template.UpdatedAt(), PublishedAt: template.PublishedAt(),
		PublishedBy: template.PublishedBy(), DisabledAt: template.DisabledAt(), DisabledBy: template.DisabledBy(),
	}
}

func reportTemplateToDomain(po *ReportTemplatePO) (*domainreporttemplate.ReportTemplate, error) {
	return domainreporttemplate.Rehydrate(domainreporttemplate.PersistedInput{
		ID: meta.FromUint64(po.DomainID), TemplateID: po.TemplateID, TemplateVersion: policy.TemplateVersion(po.TemplateVersion),
		BuilderIdentity: po.BuilderIdentity, AdapterKey: po.AdapterKey, Status: domainreporttemplate.Status(po.Status),
		CreatedAt: po.CreatedAt, UpdatedAt: po.UpdatedAt, PublishedAt: po.PublishedAt, PublishedBy: po.PublishedBy,
		DisabledAt: po.DisabledAt, DisabledBy: po.DisabledBy,
	})
}
