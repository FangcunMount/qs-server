package aibridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"strings"
	"time"
)

type ProfileClient struct{ RPC pb.ProfileManagementClient }

func NewProfileClient(conn grpc.ClientConnInterface) *ProfileClient {
	return &ProfileClient{RPC: pb.NewProfileManagementClient(conn)}
}
func profileReference(v app.PromptDraftSource) *pb.PromptDraftSource {
	return &pb.PromptDraftSource{Identity: v.Identity, Version: v.Version, Fingerprint: v.Fingerprint, ContentSha256: v.ContentSHA256}
}
func profileReceipt(raw *pb.ProfileRegistrationReceipt, scope app.DraftScope, id string) (app.ProfileRegistrationReceipt, error) {
	var v app.ProfileRegistrationReceipt
	if raw == nil || raw.SchemaVersion != "qs-ai-profile-registration/v1" || raw.CommandId != id || len(raw.ReceiptJson) > 512*1024 || json.Unmarshal([]byte(raw.ReceiptJson), &v) != nil {
		return v, app.ErrConflict
	}
	_, err := time.Parse(time.RFC3339Nano, v.RegisteredAt)
	if v.Scope != scope || scope.OrganizationID <= 0 || scope.OperatorUserID <= 0 || v.Command.CommandID != id || !v.Command.Valid() || err != nil {
		return app.ProfileRegistrationReceipt{}, app.ErrConflict
	}
	for _, ref := range []app.PromptDraftSource{v.Manifest.Profile, v.Manifest.Prompt, v.Manifest.GenerationRoute, v.Manifest.InputSchema, v.Manifest.OutputSchema} {
		if !ref.Valid() {
			return app.ProfileRegistrationReceipt{}, app.ErrConflict
		}
	}
	var definition map[string]any
	decoder := json.NewDecoder(strings.NewReader(v.Command.DefinitionJSON))
	decoder.UseNumber()
	if decoder.Decode(&definition) != nil || definition == nil {
		return app.ProfileRegistrationReceipt{}, app.ErrConflict
	}
	// Profile fingerprints use the original Go-compatible canonical JSON definition.
	canonical, err := json.Marshal(definition)
	if err != nil {
		return app.ProfileRegistrationReceipt{}, app.ErrConflict
	}
	policy, ok := definition["generation_policy"].(map[string]any)
	if !ok || policy["prompt_template_id"] != v.Manifest.Prompt.Identity || policy["prompt_version"] != v.Manifest.Prompt.Version || policy["provider_route"] != v.Manifest.GenerationRoute.Identity || policy["input_schema_version"] != v.Manifest.InputSchema.Identity+"/"+v.Manifest.InputSchema.Version || policy["output_schema_version"] != v.Manifest.OutputSchema.Identity+"/"+v.Manifest.OutputSchema.Version {
		return app.ProfileRegistrationReceipt{}, app.ErrConflict
	}
	sum := sha256.Sum256(canonical)
	fingerprint := hex.EncodeToString(sum[:])
	if v.Manifest.Profile.Identity != definition["profile_id"] || v.Manifest.Profile.Version != definition["version"] || v.Manifest.Profile.Fingerprint != "sha256:"+fingerprint || v.Manifest.Profile.ContentSHA256 != fingerprint || v.Manifest.Prompt != v.Command.Prompt || v.Manifest.GenerationRoute != v.Command.GenerationRoute {
		return app.ProfileRegistrationReceipt{}, app.ErrConflict
	}
	return v, nil
}
func (c *ProfileClient) RegisterProfile(ctx context.Context, s app.DraftScope, command app.RegisterProfile) (app.ProfileRegistrationReceipt, error) {
	if !command.Valid() {
		return app.ProfileRegistrationReceipt{}, app.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := c.RPC.Register(ctx, &pb.ProfileRegisterCommand{Scope: draftScope(s), CommandId: command.CommandID, Source: profileReference(command.Source), DefinitionJson: command.DefinitionJSON, Prompt: profileReference(command.Prompt), GenerationRoute: profileReference(command.GenerationRoute), Reason: command.Reason}, grpc.MaxCallRecvMsgSize(512*1024))
	if err != nil {
		return app.ProfileRegistrationReceipt{}, err
	}
	receipt, err := profileReceipt(raw, s, command.CommandID)
	if err != nil || receipt.Command != command {
		return app.ProfileRegistrationReceipt{}, app.ErrConflict
	}
	return receipt, nil
}
func (c *ProfileClient) GetProfileReceipt(ctx context.Context, s app.DraftScope, id string) (app.ProfileRegistrationReceipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := c.RPC.GetReceipt(ctx, &pb.ProfileRegistrationQuery{Scope: draftScope(s), CommandId: id}, grpc.MaxCallRecvMsgSize(512*1024))
	if err != nil {
		return app.ProfileRegistrationReceipt{}, err
	}
	return profileReceipt(raw, s, id)
}
