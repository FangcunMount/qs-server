package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	mongoQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type questionnaireSeedFile struct {
	Code        string                   `json:"code"`
	Version     string                   `json:"version"`
	Title       string                   `json:"title"`
	Description string                   `json:"description"`
	ImgURL      string                   `json:"img_url"`
	Type        string                   `json:"type"`
	Factors     []traitFactorSeed        `json:"factors,omitempty"`
	Dimensions  map[string]dimensionSeed `json:"dimensions,omitempty"`
	Questions   []questionSeed           `json:"questions"`
}

type questionSeed struct {
	Code        string       `json:"code"`
	Stem        string       `json:"stem"`
	Placeholder string       `json:"placeholder,omitempty"`
	Type        string       `json:"type"`
	Required    bool         `json:"required"`
	Factor      string       `json:"factor,omitempty"`
	Reverse     bool         `json:"reverse,omitempty"`
	Left        string       `json:"left,omitempty"`
	Right       string       `json:"right,omitempty"`
	Options     []optionSeed `json:"options"`
}

type traitFactorSeed struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type dimensionSeed struct {
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	LeftPole  string  `json:"left_pole,omitempty"`
	RightPole string  `json:"right_pole,omitempty"`
	Threshold float64 `json:"threshold,omitempty"`
}

type optionSeed struct {
	Code    string  `json:"code"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

func loadQuestionnaireSeed(relativePath string) (questionnaireSeedFile, error) {
	root, err := repoRoot()
	if err != nil {
		return questionnaireSeedFile{}, err
	}
	raw, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		return questionnaireSeedFile{}, fmt.Errorf("read questionnaire seed %s: %w", relativePath, err)
	}
	var seed questionnaireSeedFile
	if err := json.Unmarshal(raw, &seed); err != nil {
		return questionnaireSeedFile{}, fmt.Errorf("parse questionnaire seed %s: %w", relativePath, err)
	}
	return seed, nil
}

func seedQuestionnaire(ctx context.Context, repo *mongoQuestionnaire.Repository, seed questionnaireSeedFile, force bool) error {
	questions, err := buildDomainQuestions(seed)
	if err != nil {
		return err
	}
	q, err := buildQuestionnaire(seed, questions)
	if err != nil {
		return err
	}
	validator := domainQuestionnaire.Validator{}
	if errs := validator.ValidateForPublish(q); len(errs) > 0 {
		return fmt.Errorf("questionnaire validation failed: %v", errs[0])
	}

	existing, err := repo.FindByCodeVersion(ctx, seed.Code, seed.Version)
	if err != nil {
		return fmt.Errorf("check existing questionnaire: %w", err)
	}
	if existing != nil && !force {
		fmt.Printf("skip questionnaire %s@%s (%d questions already exist)\n", seed.Code, seed.Version, existing.QuestionCount())
		return nil
	}
	if existing != nil && force {
		if err := repo.HardDeleteFamily(ctx, seed.Code); err != nil {
			return fmt.Errorf("delete existing questionnaire %s: %w", seed.Code, err)
		}
	}

	if err := repo.Create(ctx, q); err != nil {
		return fmt.Errorf("create questionnaire head: %w", err)
	}
	if err := repo.Update(ctx, q); err != nil {
		return fmt.Errorf("update questionnaire head: %w", err)
	}
	if err := repo.CreatePublishedSnapshot(ctx, q, true); err != nil {
		return fmt.Errorf("create published snapshot: %w", err)
	}
	if err := repo.SetActivePublishedVersion(ctx, seed.Code, seed.Version); err != nil {
		return fmt.Errorf("activate published version: %w", err)
	}
	fmt.Printf("seeded questionnaire %s@%s with %d questions\n", seed.Code, seed.Version, len(seed.Questions))
	return nil
}

func buildQuestionnaire(seed questionnaireSeedFile, questions []domainQuestionnaire.Question) (*domainQuestionnaire.Questionnaire, error) {
	qType := domainQuestionnaire.NormalizeQuestionnaireType(seed.Type)
	q, err := domainQuestionnaire.NewQuestionnaire(
		meta.NewCode(seed.Code),
		seed.Title,
		domainQuestionnaire.WithDesc(seed.Description),
		domainQuestionnaire.WithImgUrl(seed.ImgURL),
		domainQuestionnaire.WithVersion(domainQuestionnaire.NewVersion(seed.Version)),
		domainQuestionnaire.WithStatus(domainQuestionnaire.STATUS_PUBLISHED),
		domainQuestionnaire.WithType(qType),
		domainQuestionnaire.WithActivePublished(true),
	)
	if err != nil {
		return nil, err
	}
	if err := q.ReplaceQuestions(questions); err != nil {
		return nil, err
	}
	return q, nil
}

func buildDomainQuestions(seed questionnaireSeedFile) ([]domainQuestionnaire.Question, error) {
	questions := make([]domainQuestionnaire.Question, 0, len(seed.Questions))
	for i, item := range seed.Questions {
		options := make([]domainQuestionnaire.Option, 0, len(item.Options))
		for _, opt := range item.Options {
			option, err := domainQuestionnaire.NewOptionWithStringCode(opt.Code, opt.Content, opt.Score)
			if err != nil {
				return nil, fmt.Errorf("question %s option %s: %w", item.Code, opt.Code, err)
			}
			options = append(options, option)
		}
		qType := domainQuestionnaire.QuestionType(item.Type)
		opts := []domainQuestionnaire.QuestionParamsOption{
			domainQuestionnaire.WithCode(meta.NewCode(item.Code)),
			domainQuestionnaire.WithStem(item.Stem),
			domainQuestionnaire.WithQuestionType(qType),
			domainQuestionnaire.WithOptions(options),
		}
		if item.Placeholder != "" {
			opts = append(opts, domainQuestionnaire.WithPlaceholder(item.Placeholder))
		}
		question, err := domainQuestionnaire.NewQuestion(opts...)
		if err != nil {
			return nil, fmt.Errorf("build question #%d (%s): %w", i+1, item.Code, err)
		}
		questions = append(questions, question)
	}
	return questions, nil
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from cwd")
		}
		dir = parent
	}
}
