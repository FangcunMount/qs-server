package prompt

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
)

func TestCatalogReturnsValidatedImmutablePackage(t *testing.T) {
	pkg, err := NewCatalog().ResolvePromptPackage(context.Background(), ParticipantScaleTemplateID, ParticipantScaleVersion)
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.Validate(); err != nil {
		t.Fatal(err)
	}
	if pkg.Ref.Fingerprint != ParticipantScaleFingerprint || pkg.Ref.GitBlobSHA != ParticipantScaleGitBlobSHA {
		t.Fatalf("Prompt ref = %#v", pkg.Ref)
	}
	if pkg.Ref.Fingerprint != aiexplanation.Fingerprint("sha256:0b3259bea414a1e8d2c8ee77c68af4a3ecc7b909c59232609d7acc782a426c50") {
		t.Fatalf("Prompt fingerprint = %s", pkg.Ref.Fingerprint)
	}

	pkg.AllowedPlaceholders[0] = "{{mutated}}"
	again, err := NewCatalog().ResolvePromptPackage(context.Background(), ParticipantScaleTemplateID, ParticipantScaleVersion)
	if err != nil {
		t.Fatal(err)
	}
	if again.AllowedPlaceholders[0] == "{{mutated}}" {
		t.Fatal("catalog leaked mutable Prompt package state")
	}
}

func TestCatalogRejectsUnknownPackage(t *testing.T) {
	_, err := NewCatalog().ResolvePromptPackage(context.Background(), ParticipantScaleTemplateID, "v3")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v", err)
	}
}

func TestCatalogReturnsValidatedV2PackageWithoutMutatingV1(t *testing.T) {
	v1, err := NewCatalog().ResolvePromptPackage(context.Background(), ParticipantScaleTemplateID, ParticipantScaleVersion)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := NewCatalog().ResolvePromptPackage(context.Background(), ParticipantScaleTemplateID, ParticipantScaleVersionV2)
	if err != nil {
		t.Fatal(err)
	}
	if err := v2.Validate(); err != nil {
		t.Fatal(err)
	}
	if v2.Ref.Fingerprint != ParticipantScaleFingerprintV2 || v2.Ref.GitBlobSHA != ParticipantScaleGitBlobSHAV2 {
		t.Fatalf("v2 Prompt ref = %#v", v2.Ref)
	}
	if v1.Ref.Version != ParticipantScaleVersion || v1.Ref.Fingerprint != ParticipantScaleFingerprint {
		t.Fatalf("v1 Prompt identity changed = %#v", v1.Ref)
	}
	for _, required := range []string{
		"弱化词不能把因果内容变成允许",
		"不得根据原始分数",
		"focus areas 只是本次请求的组织重点",
		"不得同时包含父维度与其任何子孙维度",
	} {
		if !strings.Contains(v2.SystemMessage, required) && !strings.Contains(v2.TaskTemplate, required) {
			t.Errorf("v2 Prompt is missing guardrail %q", required)
		}
	}
}

func TestExecutableMessagesMatchNormativeMarkdownCodeBlocks(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "..", "api", "schema", "interpretation", "ai-explanation-prompt-template-v1.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```text\\n(.*?)\\n```").FindAllStringSubmatch(strings.ReplaceAll(string(raw), "\r\n", "\n"), -1)
	if len(blocks) < 3 {
		t.Fatalf("normative text blocks = %d", len(blocks))
	}
	if blocks[0][1] != participantScaleSystemMessage || blocks[1][1] != participantScaleTaskTemplate {
		t.Fatal("executable system/task messages drifted from normative Markdown")
	}
	dataBlock := strings.TrimSuffix(blocks[2][1], "\n\n{{provider_payload_json}}")
	if dataBlock != participantScaleDataPreamble {
		t.Fatal("executable data preamble drifted from normative Markdown")
	}
}

func TestExecutableV2MessagesMatchNormativeMarkdownCodeBlocks(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "..", "api", "schema", "interpretation", "ai-explanation-prompt-template-v2.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```text\\n(.*?)\\n```").FindAllStringSubmatch(strings.ReplaceAll(string(raw), "\r\n", "\n"), -1)
	if len(blocks) < 3 {
		t.Fatalf("normative text blocks = %d", len(blocks))
	}
	if blocks[0][1] != participantScaleSystemMessageV2 || blocks[1][1] != participantScaleTaskTemplateV2 {
		t.Fatal("executable v2 system/task messages drifted from normative Markdown")
	}
	dataBlock := strings.TrimSuffix(blocks[2][1], "\n\n{{provider_payload_json}}")
	if dataBlock != participantScaleDataPreamble {
		t.Fatal("executable v2 data preamble drifted from normative Markdown")
	}
}
