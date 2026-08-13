package modelcatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const (
	publishedModelDetailKeyPrefix  = "published-model:detail:"
	publishedModelListKeyPrefix    = "published-model:list:"
	publishedModelOptionsKeyPrefix = "published-model:options:"
)

func normalizedModelCode(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

func publishedModelDetailCacheKey(code string) string {
	return publishedModelDetailKeyPrefix + normalizedModelCode(code)
}

func publishedModelOptionsCacheKey(kind string) string {
	scope := strings.TrimSpace(kind)
	if scope == "" {
		scope = "all"
	}
	return publishedModelOptionsKeyPrefix + scope
}

func publishedModelListCacheKey(request *ListRequest) (string, error) {
	normalized, err := normalizedListRequest(request)
	if err != nil {
		return "", err
	}
	identity := struct {
		Kind                 string   `json:"kind"`
		Kinds                []string `json:"kinds"`
		Algorithm            string   `json:"algorithm"`
		Category             string   `json:"category"`
		Keyword              string   `json:"keyword"`
		QuestionnaireCode    string   `json:"questionnaire_code"`
		QuestionnaireVersion string   `json:"questionnaire_version"`
		Page                 int32    `json:"page"`
		PageSize             int32    `json:"page_size"`
	}{
		Kind: normalized.Kind, Kinds: append([]string(nil), normalized.kinds...),
		Algorithm: normalized.Algorithm, Category: normalized.Category, Keyword: normalized.Keyword,
		QuestionnaireCode: normalized.QuestionnaireCode, QuestionnaireVersion: normalized.QuestionnaireVersion,
		Page: normalized.Page, PageSize: normalized.PageSize,
	}
	raw, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(raw)
	return publishedModelListKeyPrefix + hex.EncodeToString(hash[:]), nil
}
