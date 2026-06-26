package container

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongoassessmentmodel "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/assessmentmodel"
	mongoruleset "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/ruleset"
	aminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/assessmentmodel"
)

func (c *Container) buildPersonalityModelModuleDeps() assembler.PersonalityModelModuleDeps {
	if c == nil || c.mongoDB == nil {
		return assembler.PersonalityModelModuleDeps{}
	}
	mongoOpts := mongoBase.BaseRepositoryOptions{Limiter: c.backpressure.Mongo}
	v2Repo := mongoassessmentmodel.NewRepository(c.mongoDB, mongoOpts)
	legacyRepo := mongoruleset.NewRepository(c.mongoDB, mongoOpts)
	return assembler.PersonalityModelModuleDeps{
		PublishedLister: aminfra.NewDualStore(v2Repo, legacyRepo),
	}
}

func (c *Container) buildPersonalityModelModule() (*assembler.PersonalityModelModule, error) {
	return assembler.NewPersonalityModelModule(c.buildPersonalityModelModuleDeps())
}

func (c *Container) initPersonalityModelModule() error {
	module, err := c.buildPersonalityModelModule()
	if err != nil {
		return fmt.Errorf("failed to initialize personality model module: %w", err)
	}
	c.PersonalityModelModule = module
	c.registerModule("personalitymodel", module)
	c.printf("📦 Personality model module initialized\n")
	return nil
}
