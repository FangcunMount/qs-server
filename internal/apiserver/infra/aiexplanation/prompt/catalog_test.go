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
	_, err := NewCatalog().ResolvePromptPackage(context.Background(), ParticipantScaleTemplateID, "v2")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v", err)
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
