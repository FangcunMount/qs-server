package aibridge

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
)

var publicationHistoryVersion = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)

func (c *PublicationClient) ListPublicationHistory(ctx context.Context, scope app.PublicationScope, query app.PublicationHistoryQuery) (app.PublicationHistoryPage, error) {
	if !query.Valid() {
		return app.PublicationHistoryPage{}, app.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := c.RPC.ListHistory(ctx, &pb.PublicationHistoryQuery{Scope: publicationScope(scope), Selector: publicationSelector(query.Selector), BeforeVersion: query.BeforeVersion, Limit: query.Limit}, grpc.MaxCallRecvMsgSize(65*1024))
	if err != nil {
		return app.PublicationHistoryPage{}, err
	}
	return historyPage(response, query)
}
func (c *PublicationClient) GetPublicationHistory(ctx context.Context, scope app.PublicationScope, selector app.PublicationSelector, version int64) (app.PublicationReceipt, error) {
	if !selector.Valid() || version < 1 {
		return app.PublicationReceipt{}, app.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := c.RPC.GetHistory(ctx, &pb.PublicationHistoryVersionQuery{Scope: publicationScope(scope), Selector: publicationSelector(selector), Version: version}, grpc.MaxCallRecvMsgSize(1024*1024))
	if err != nil {
		return app.PublicationReceipt{}, err
	}
	if response == nil {
		return app.PublicationReceipt{}, app.ErrConflict
	}
	result, err := decodePublicationReceipt(response, response.CommandId)
	if err != nil || !result.Current.Selector.Equal(selector) || result.Current.Version != version {
		return app.PublicationReceipt{}, app.ErrConflict
	}
	return result, nil
}
func validHistoryEntry(e app.PublicationHistoryEntry) bool {
	if e.Version < 1 || !app.ValidPublicationID(e.CommandID) || !validPublicationAudit(e.Actor, e.Reason, e.ChangedAt) ||
		e.PreviousPublicationID != nil && !app.ValidPublicationID(*e.PreviousPublicationID) {
		return false
	}
	if e.Version == 1 && e.PreviousPublicationID != nil {
		return false
	}
	if e.Action == "disable" {
		return e.PreviousPublicationID != nil && e.PublicationID == nil && e.RunID == nil && e.RunVersion == nil && e.ProfileID == nil && e.ProfileVersion == nil
	}
	if e.Action != "publish" && e.Action != "rollback" || e.Action == "rollback" && e.Version == 1 {
		return false
	}
	return e.PublicationID != nil && app.ValidPublicationID(*e.PublicationID) &&
		(e.PreviousPublicationID == nil || *e.PreviousPublicationID != *e.PublicationID) &&
		e.RunID != nil && app.ValidPublicationID(*e.RunID) && e.RunVersion != nil && *e.RunVersion > 0 &&
		e.ProfileID != nil && strings.TrimSpace(*e.ProfileID) != "" && len(*e.ProfileID) <= 255 && utf8.ValidString(*e.ProfileID) &&
		e.ProfileVersion != nil && publicationHistoryVersion.MatchString(*e.ProfileVersion)
}
func historyPage(response *pb.PublicationHistoryPage, query app.PublicationHistoryQuery) (app.PublicationHistoryPage, error) {
	const schema = "qs-ai-publication-history/v1"
	if !query.Valid() || response == nil || response.SchemaVersion != schema || len(response.PayloadJson) > 64*1024 || !utf8.ValidString(response.PayloadJson) || !json.Valid([]byte(response.PayloadJson)) {
		return app.PublicationHistoryPage{}, app.ErrConflict
	}
	var raw struct {
		SchemaVersion     string                         `json:"schema_version"`
		Selector          app.PublicationSelector        `json:"selector"`
		Entries           *[]app.PublicationHistoryEntry `json:"entries"`
		NextBeforeVersion *int64                         `json:"next_before_version"`
	}
	decoder := json.NewDecoder(bytes.NewBufferString(response.PayloadJson))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&raw) != nil || raw.SchemaVersion != schema || !raw.Selector.Equal(query.Selector) || raw.Entries == nil || raw.NextBeforeVersion == nil ||
		*raw.NextBeforeVersion < 0 || len(*raw.Entries) > int(query.Limit) {
		return app.PublicationHistoryPage{}, app.ErrConflict
	}
	entries := *raw.Entries
	seen := map[string]bool{}
	for i, e := range entries {
		if !validHistoryEntry(e) || query.BeforeVersion > 0 && e.Version >= query.BeforeVersion || seen[e.CommandID] {
			return app.PublicationHistoryPage{}, app.ErrConflict
		}
		if i > 0 {
			later, _ := time.Parse(time.RFC3339Nano, entries[i-1].ChangedAt)
			earlier, _ := time.Parse(time.RFC3339Nano, e.ChangedAt)
			if e.Version >= entries[i-1].Version || earlier.After(later) {
				return app.PublicationHistoryPage{}, app.ErrConflict
			}
		}
		seen[e.CommandID] = true
	}
	next := *raw.NextBeforeVersion
	if next != 0 && (len(entries) != int(query.Limit) || next <= 1 || next != entries[len(entries)-1].Version) {
		return app.PublicationHistoryPage{}, app.ErrConflict
	}
	return app.PublicationHistoryPage{SchemaVersion: schema, Selector: raw.Selector, Entries: entries, NextBeforeVersion: next}, nil
}
