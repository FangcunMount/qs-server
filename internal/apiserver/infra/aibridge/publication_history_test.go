package aibridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func historyTestPage() (app.PublicationHistoryPage, app.PublicationHistoryQuery) {
	selector := app.PublicationSelector{Audience: "participant", ModelKind: "scale", DecisionKind: "score_range"}
	pub, run, profile, version := publicationTestID, publicationTestID, "profile-a", "v1"
	runVersion := int64(9)
	entry := app.PublicationHistoryEntry{Version: 3, CommandID: publicationTestID, Action: "publish", Actor: "user:42", Reason: "发布", ChangedAt: publicationTestAt, PublicationID: &pub, RunID: &run, RunVersion: &runVersion, ProfileID: &profile, ProfileVersion: &version}
	return app.PublicationHistoryPage{SchemaVersion: "qs-ai-publication-history/v1", Selector: selector, Entries: []app.PublicationHistoryEntry{entry}, NextBeforeVersion: 3}, app.PublicationHistoryQuery{Selector: selector, BeforeVersion: 4, Limit: 1}
}
func historyTestWire(p app.PublicationHistoryPage) *pb.PublicationHistoryPage {
	raw, _ := json.Marshal(p)
	return &pb.PublicationHistoryPage{SchemaVersion: p.SchemaVersion, PayloadJson: string(raw)}
}

func TestPublicationHistoryPageChecksBindingsAndPagination(t *testing.T) {
	p, q := historyTestPage()
	if r, err := historyPage(historyTestWire(p), q); err != nil || r.NextBeforeVersion != 3 || r.Entries[0].Actor != "user:42" {
		t.Fatal(r, err)
	}
	for name, alter := range map[string]func(*app.PublicationHistoryPage){
		"schema":            func(p *app.PublicationHistoryPage) { p.SchemaVersion = "other" },
		"selector":          func(p *app.PublicationHistoryPage) { p.Selector.Audience = "other" },
		"cursor-binding":    func(p *app.PublicationHistoryPage) { p.NextBeforeVersion = 2 },
		"negative-cursor":   func(p *app.PublicationHistoryPage) { p.NextBeforeVersion = -1 },
		"empty-with-cursor": func(p *app.PublicationHistoryPage) { p.Entries = []app.PublicationHistoryEntry{} },
		"null-entries":      func(p *app.PublicationHistoryPage) { p.Entries = nil },
		"exclusive-bound":   func(p *app.PublicationHistoryPage) { p.Entries[0].Version = 4 },
		"zero-version":      func(p *app.PublicationHistoryPage) { p.Entries[0].Version = 0 },
		"command":           func(p *app.PublicationHistoryPage) { p.Entries[0].CommandID = "invalid" },
		"actor":             func(p *app.PublicationHistoryPage) { p.Entries[0].Actor = "" },
		"audit":             func(p *app.PublicationHistoryPage) { p.Entries[0].ChangedAt = "yesterday" },
		"action":            func(p *app.PublicationHistoryPage) { p.Entries[0].Action = "unknown" },
		"false-disable":     func(p *app.PublicationHistoryPage) { p.Entries[0].Action = "disable" },
		"missing-run":       func(p *app.PublicationHistoryPage) { p.Entries[0].RunID = nil },
		"missing-profile":   func(p *app.PublicationHistoryPage) { p.Entries[0].ProfileVersion = nil },
		"initial-rollback": func(p *app.PublicationHistoryPage) {
			p.Entries[0].Version = 1
			p.Entries[0].Action = "rollback"
			p.NextBeforeVersion = 0
		},
	} {
		t.Run(name, func(t *testing.T) {
			page, query := historyTestPage()
			alter(&page)
			if _, err := historyPage(historyTestWire(page), query); !errors.Is(err, app.ErrConflict) {
				t.Fatal("bad history accepted", err)
			}
		})
	}
	for _, payload := range []string{"{}", "null", strings.Repeat(" ", 65537), strings.Replace(historyTestWire(p).PayloadJson, `"next_before_version":3`, `"next_before_version":null`, 1), strings.Replace(historyTestWire(p).PayloadJson, `"next_before_version":3`, `"unexpected":true,"next_before_version":3`, 1)} {
		if _, err := historyPage(&pb.PublicationHistoryPage{SchemaVersion: p.SchemaVersion, PayloadJson: payload}, q); !errors.Is(err, app.ErrConflict) {
			t.Fatal("invalid payload accepted", err)
		}
	}
	p.Entries = []app.PublicationHistoryEntry{}
	p.NextBeforeVersion = 0
	if _, err := historyPage(historyTestWire(p), q); err != nil {
		t.Fatal("empty history rejected", err)
	}
}

func TestPublicationHistoryDescendingOrderAndActionEvidence(t *testing.T) {
	p, q := historyTestPage()
	q.Limit = 2
	second := p.Entries[0]
	second.Version = 2
	second.CommandID = "00000000-0000-4000-8000-000000000002"
	p.Entries = append(p.Entries, second)
	p.NextBeforeVersion = 2
	if _, err := historyPage(historyTestWire(p), q); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"duplicate-command", "ascending-version", "ascending-time"} {
		bad := p
		bad.Entries = append([]app.PublicationHistoryEntry{}, p.Entries...)
		switch kind {
		case "duplicate-command":
			bad.Entries[1].CommandID = bad.Entries[0].CommandID
		case "ascending-version":
			bad.Entries[1].Version = 3
		case "ascending-time":
			bad.Entries[1].ChangedAt = "2026-09-13T01:00:00Z"
		}
		if _, err := historyPage(historyTestWire(bad), q); !errors.Is(err, app.ErrConflict) {
			t.Fatal(kind, err)
		}
	}
	disabled := app.PublicationHistoryEntry{Version: 4, CommandID: publicationTestID, Action: "disable", Actor: "user:9", Reason: "停用", ChangedAt: publicationTestAt, PreviousPublicationID: &second.CommandID}
	if !validHistoryEntry(disabled) {
		t.Fatal("valid disable rejected")
	}
	disabled.RunID = &second.CommandID
	if validHistoryEntry(disabled) {
		t.Fatal("disabled entry retained active evidence")
	}
}

type publicationHistoryRPC struct {
	pb.PublicationManagementClient
	page     *pb.PublicationHistoryPage
	receipt  *pb.PublicationReceipt
	err      error
	calls    int
	list     *pb.PublicationHistoryQuery
	get      *pb.PublicationHistoryVersionQuery
	deadline bool
}

func (s *publicationHistoryRPC) ListHistory(ctx context.Context, q *pb.PublicationHistoryQuery, _ ...grpc.CallOption) (*pb.PublicationHistoryPage, error) {
	s.calls++
	s.list = q
	d, ok := ctx.Deadline()
	s.deadline = ok && time.Until(d) <= 5*time.Second
	return s.page, s.err
}
func (s *publicationHistoryRPC) GetHistory(ctx context.Context, q *pb.PublicationHistoryVersionQuery, _ ...grpc.CallOption) (*pb.PublicationReceipt, error) {
	s.calls++
	s.get = q
	d, ok := ctx.Deadline()
	s.deadline = ok && time.Until(d) <= 5*time.Second
	return s.receipt, s.err
}

func TestPublicationHistoryClientPreservesOriginalActor(t *testing.T) {
	p, q := historyTestPage()
	s := &publicationHistoryRPC{page: historyTestWire(p), receipt: publicationTestReceipt()}
	c := &PublicationClient{RPC: s}
	scope := app.PublicationScope{OrganizationID: 7, OperatorUserID: 88}
	if _, err := c.ListPublicationHistory(context.Background(), scope, q); err != nil || s.list.Scope.OperatorUserId != 88 || s.list.BeforeVersion != 4 || s.list.Limit != 1 || !s.deadline {
		t.Fatal(err, s)
	}
	r, err := c.GetPublicationHistory(context.Background(), scope, q.Selector, 1)
	if err != nil || r.Actor != "user:42" || s.get.Scope.OrganizationId != 7 || s.get.Version != 1 || !s.deadline {
		t.Fatal(r, err, s)
	}
	if _, err := publicationReceipt(s.receipt, scope, publicationTestID); !errors.Is(err, app.ErrConflict) {
		t.Fatal("original command operator restriction lost", err)
	}
	if _, err := c.GetPublicationHistory(context.Background(), scope, q.Selector, 2); !errors.Is(err, app.ErrConflict) {
		t.Fatal("wrong version accepted", err)
	}
	code := "another-model"
	selector := q.Selector
	selector.ModelCode = &code
	if _, err := c.GetPublicationHistory(context.Background(), scope, selector, 1); !errors.Is(err, app.ErrConflict) {
		t.Fatal("wrong selector accepted", err)
	}
	s.err = status.Error(codes.Unavailable, "down")
	before := s.calls
	if _, err := c.ListPublicationHistory(context.Background(), scope, q); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
	if _, err := c.GetPublicationHistory(context.Background(), scope, q.Selector, 1); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
	if s.calls != before+2 {
		t.Fatal("read retried")
	}
	before = s.calls
	q.Limit = 0
	if _, err := c.ListPublicationHistory(context.Background(), scope, q); !errors.Is(err, app.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := c.GetPublicationHistory(context.Background(), scope, q.Selector, 0); !errors.Is(err, app.ErrInvalid) {
		t.Fatal(err)
	}
	if s.calls != before {
		t.Fatal("invalid query forwarded")
	}
}
