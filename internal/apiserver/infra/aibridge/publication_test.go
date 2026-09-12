package aibridge

import (
	"context"
	"encoding/json"
	"errors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"strings"
	"testing"
)

const publicationTestID = "00000000-0000-4000-8000-000000000001"
const publicationTestAt = "2026-09-12T01:00:00Z"

func publicationTestState() *pb.PublicationState {
	selector := app.PublicationSelector{Audience: "participant", ModelKind: "scale", DecisionKind: "score_range"}
	definition, _ := json.Marshal(map[string]any{"profile_id": "profile-a", "version": "v1", "selector": selector})
	proof := publicationProof{PublicationID: publicationTestID, Audit: publicationAudit{Actor: "user:42", Reason: "发布", At: publicationTestAt}}
	e := &proof.Evidence
	e.RunID = publicationTestID
	e.RunVersion = 9
	e.Profile.ProfileID = "profile-a"
	e.Profile.Version = "v1"
	e.Profile.DefinitionJSON = string(definition)
	e.Profile.Fingerprint = digest(definition)
	e.FinalReview.Actor = "user:42"
	e.FinalReview.Reason = "已批准"
	e.FinalReview.FinalizedAt = publicationTestAt
	passed := true
	e.FinalReview.Passed = &passed
	e.Release = map[string]app.FrozenEvaluationRef{}
	e.Manifest = map[string]publicationAssetRef{}
	for _, name := range []string{"suite", "profile", "prompt", "input_schema", "output_schema", "generation_route", "semantic_prompt", "semantic_output_schema", "semantic_route", "execution_policy", "gate_policy"} {
		e.Release[name] = app.FrozenEvaluationRef{ID: name, Version: "v1", Fingerprint: digest([]byte(name))}
	}
	e.Release["profile"] = app.FrozenEvaluationRef{ID: "profile-a", Version: "v1", Fingerprint: e.Profile.Fingerprint}
	canonical := map[string]any{"schema_version": "qs-ai-generation-manifest/v1"}
	for _, name := range []string{"profile", "prompt", "generation_route", "input_schema", "output_schema"} {
		r := e.Release[name]
		e.Manifest[name] = publicationAssetRef{Identity: r.ID, Version: r.Version, Fingerprint: r.Fingerprint, ContentSHA256: strings.TrimPrefix(r.Fingerprint, "sha256:")}
		if strings.HasSuffix(name, "schema") {
			r.Version = r.ID + "/" + r.Version
			e.Release[name] = r
		}
		m := e.Manifest[name]
		canonical[name] = map[string]string{"identity": m.Identity, "version": m.Version, "fingerprint": m.Fingerprint, "content_sha256": m.ContentSHA256}
	}
	raw, _ := json.Marshal(canonical)
	e.EvaluatedManifestFingerprint = digest(raw)
	raw, _ = json.Marshal(map[string]any{"schema_version": "qs-ai-publication/v1", "publication": proof})
	return &pb.PublicationState{Selector: publicationSelector(selector), Version: 1, ActivePublicationId: publicationTestID, PublicationJson: string(raw), ChangedAt: publicationTestAt}
}
func publicationTestReceipt() *pb.PublicationReceipt {
	current := publicationTestState()
	return &pb.PublicationReceipt{CommandId: publicationTestID, Previous: &pb.PublicationState{Selector: proto.Clone(current.Selector).(*pb.PublicationSelector)}, Current: current, Action: "publish", Actor: "user:42", Reason: "发布", ChangedAt: publicationTestAt}
}
func TestPublicationWireRejectsUnboundEvidence(t *testing.T) {
	good := publicationTestReceipt()
	scope := app.PublicationScope{OrganizationID: 1, OperatorUserID: 42}
	if _, err := publicationReceipt(good, scope, publicationTestID); err != nil {
		t.Fatal("valid wire rejected", err)
	}
	for name, change := range map[string]func(*pb.PublicationReceipt){
		"command":     func(r *pb.PublicationReceipt) { r.CommandId = "00000000-0000-4000-8000-000000000002" },
		"actor":       func(r *pb.PublicationReceipt) { r.Actor = "user:43" },
		"version":     func(r *pb.PublicationReceipt) { r.Current.Version = 4 },
		"selector":    func(r *pb.PublicationReceipt) { code := "other"; r.Current.Selector.ModelCode = &code },
		"publication": func(r *pb.PublicationReceipt) { r.Current.ActivePublicationId = "00000000-0000-4000-8000-000000000002" },
		"content": func(r *pb.PublicationReceipt) {
			r.Current.PublicationJson = strings.Replace(r.Current.PublicationJson, "profile-a", "profile-b", 1)
		},
		"failed-review": func(r *pb.PublicationReceipt) {
			r.Current.PublicationJson = strings.Replace(r.Current.PublicationJson, `"passed":true`, `"passed":false`, 1)
		},
		"audit":        func(r *pb.PublicationReceipt) { r.Reason = "another reason" },
		"oversized":    func(r *pb.PublicationReceipt) { r.Current.PublicationJson = strings.Repeat(" ", 1024*1024) },
		"initial-time": func(r *pb.PublicationReceipt) { r.Previous.ChangedAt = publicationTestAt },
	} {
		t.Run(name, func(t *testing.T) {
			r := proto.Clone(good).(*pb.PublicationReceipt)
			change(r)
			if _, err := publicationReceipt(r, scope, publicationTestID); !errors.Is(err, app.ErrConflict) {
				t.Fatal("corrupt evidence accepted", err)
			}
		})
	}
}

type publicationRPCStub struct {
	pb.PublicationManagementClient
	response *pb.PublicationReceipt
	err      error
	calls    int
	request  *pb.PublicationPublishCommand
	deadline bool
}

func (s *publicationRPCStub) Publish(ctx context.Context, c *pb.PublicationPublishCommand, _ ...grpc.CallOption) (*pb.PublicationReceipt, error) {
	s.calls++
	s.request = c
	_, s.deadline = ctx.Deadline()
	return s.response, s.err
}
func TestPublicationClientBindsConfirmationAndNeverRetries(t *testing.T) {
	response := publicationTestReceipt()
	state, _ := publicationState(response.Current)
	proof, _ := publicationDocument(state)
	v := int64(0)
	command := app.PublishConfiguration{PublicationCommand: app.PublicationCommand{CommandID: publicationTestID, Expected: &app.PublicationExpectation{Selector: state.Selector, Version: &v}, Reason: "发布", Confirm: true}, RunID: publicationTestID, RunVersion: 9, ReleaseFingerprint: releaseDigest(proof.Evidence.Release)}
	scope := app.PublicationScope{OrganizationID: 7, OperatorUserID: 42}
	for _, bad := range []string{"", "run", "run-version", "release", "confirmation", "timeout"} {
		t.Run(bad, func(t *testing.T) {
			s := &publicationRPCStub{response: response}
			c := command
			switch bad {
			case "run":
				c.RunID = "00000000-0000-4000-8000-000000000002"
			case "run-version":
				c.RunVersion = 10
			case "release":
				c.ReleaseFingerprint = "sha256:" + strings.Repeat("a", 64)
			case "confirmation":
				c.Reason = "changed"
			case "timeout":
				s.err = status.Error(codes.DeadlineExceeded, "unknown")
			}
			_, err := (&PublicationClient{RPC: s}).PublishConfiguration(context.Background(), scope, c)
			if bad == "" && err != nil {
				t.Fatal(err)
			}
			if bad != "" && err == nil {
				t.Fatal("bad result accepted")
			}
			if s.calls != 1 || !s.deadline || s.request.Scope.OrganizationId != 7 || s.request.Scope.OperatorUserId != 42 {
				t.Fatal("lost timeout/scope or retried")
			}
		})
	}
}
