package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func finalHistoryDescriptor() (lifecycleRequest, *lifecycleFinalHistoryInput) {
	r := lifecycleRequest{OperationID: "123-1", ActualRunID: "456-2"}
	d := &lifecycleFinalHistoryInput{AssetsDirectory: "/opt/qs-server/retirement-assets", RuntimeSourceSHA: strings.Repeat("a", 40), ImageID: "sha256:" + strings.Repeat("b", 64), ContainerID: strings.Repeat("c", 64), RuntimeBindingSHA256: strings.Repeat("d", 64)}
	root := "/opt/backups/qs-server/compatibility-retirement/123-1"
	d.AIBounds = lifecycleFinalFileBinding{filepath.Join(root, "ai-bounds.json"), strings.Repeat("e", 64)}
	d.PeerBounds = lifecycleFinalFileBinding{filepath.Join(root, "peer-bounds.json"), strings.Repeat("f", 64)}
	d.Protection = lifecycleFinalFileBinding{filepath.Join(root, "protection.json"), strings.Repeat("1", 64)}
	return r, d
}

func TestLifecycleFinalHistoryRequiresExactCurrentOriginalAndFileConstraints(t *testing.T) {
	r, d := finalHistoryDescriptor()
	if !d.valid(r) {
		t.Fatal("exact constraints rejected")
	}
	for _, tc := range []struct {
		name   string
		change func(*lifecycleFinalHistoryInput)
	}{
		{"image_tag", func(v *lifecycleFinalHistoryInput) { v.ImageID = "qs-ai:latest" }},
		{"source", func(v *lifecycleFinalHistoryInput) { v.RuntimeSourceSHA = "main" }},
		{"hash", func(v *lifecycleFinalHistoryInput) { v.RuntimeBindingSHA256 = "" }},
		{"relative_asset", func(v *lifecycleFinalHistoryInput) { v.AssetsDirectory = "assets" }},
		{"other_operation", func(v *lifecycleFinalHistoryInput) {
			v.AIBounds.Path = strings.ReplaceAll(v.AIBounds.Path, "123-1", "124-1")
		}},
		{"aliased_input", func(v *lifecycleFinalHistoryInput) { v.Protection = v.AIBounds }},
		{"body_path", func(v *lifecycleFinalHistoryInput) { v.AIBounds.Path = "/tmp/body" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := *d
			tc.change(&v)
			if v.valid(r) {
				t.Fatal("unbound fact accepted")
			}
		})
	}
	if _, e := lifecycleFinalExternalInput(lifecycleRequest{ActualRunID: "0456-2", FinalHistory: d, OperationID: r.OperationID}); e == nil {
		t.Fatal("noncanonical actual run accepted")
	}
}

func TestLifecycleFinalHistoryDoesNotBypassMissingNativeServiceScope(t *testing.T) {
	r, d := finalHistoryDescriptor()
	r.FinalHistory = d
	for _, h := range []*lifecycleFixedHost{nil, {}} {
		if h.FinalDifferenceAndEOF(t.Context(), r, nil) == nil {
			t.Fatal("missing native Window/leases/DB adopted")
		}
	}
	if lifecycleEffectsPreflight(context.Background()) == nil {
		t.Fatal("final history enabled production effects")
	}
}

func TestLifecyclePrivateFinalInputPreservesExactRawBytesAndRejectsLinkAndHash(t *testing.T) {
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "bounds.json")
	raw := []byte("{\"bound\":1}\n")
	if e = os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	var value json.RawMessage
	if readLifecyclePrivateLimit(path, digestRaw(raw), &value, 4<<20) != nil || strings.TrimSpace(string(value)) != strings.TrimSpace(string(raw)) {
		t.Fatal("approved raw facts changed")
	}
	if readLifecyclePrivateLimit(path, strings.Repeat("f", 64), &value, 4<<20) == nil {
		t.Fatal("different input hash accepted")
	}
	link := filepath.Join(dir, "link")
	if e = os.Link(path, link); e != nil {
		t.Fatal(e)
	}
	if readLifecyclePrivateLimit(path, digestRaw(raw), &value, 4<<20) == nil {
		t.Fatal("multiple file identities accepted")
	}
}
