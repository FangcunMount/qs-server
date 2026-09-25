package iam

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	authnv3 "github.com/FangcunMount/iam/v5/api/grpc/iam/authn/v3"
	identityv2 "github.com/FangcunMount/iam/v5/api/grpc/iam/identity/v2"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type candidateLinksStub struct {
	response *identityv2.ListProfileLinksResponse
	err      error
	called   int
}

func (*candidateLinksStub) IsEnabled() bool { return true }

func (s *candidateLinksStub) ListProfileLinks(_ context.Context, _ string) (*identityv2.ListProfileLinksResponse, error) {
	s.called++
	return s.response, s.err
}

type candidateLookupStub struct {
	response *authnv3.ResolveMiniProgramNotificationRecipientsResponse
	err      error
	request  *authnv3.ResolveMiniProgramNotificationRecipientsRequest
	called   int
}

func (s *candidateLookupStub) ResolveMiniProgramRecipients(
	_ context.Context,
	req *authnv3.ResolveMiniProgramNotificationRecipientsRequest,
) (*authnv3.ResolveMiniProgramNotificationRecipientsResponse, error) {
	s.called++
	s.request = req
	return s.response, s.err
}

func candidateLink(profileID, userID string, relation identityv2.ProfileLinkRelation) *identityv2.ProfileLinkEdge {
	return &identityv2.ProfileLinkEdge{ProfileLink: &identityv2.ProfileLink{
		ProfileId: profileID, UserId: userID, Relation: relation,
	}}
}

func TestRecipientCandidatesUseLinkedUserIDsAndActualAppID(t *testing.T) {
	t.Parallel()
	links := &candidateLinksStub{response: &identityv2.ListProfileLinksResponse{Items: []*identityv2.ProfileLinkEdge{
		candidateLink("42", "99", identityv2.ProfileLinkRelation_PROFILE_LINK_RELATION_PARENT),
		candidateLink("42", "99", identityv2.ProfileLinkRelation_PROFILE_LINK_RELATION_GRANDPARENT),
		candidateLink("42", "100", identityv2.ProfileLinkRelation_PROFILE_LINK_RELATION_SELF),
		{ProfileLink: &identityv2.ProfileLink{ProfileId: "42", UserId: "42", RevokedAt: timestamppb.Now()}},
	}}}
	lookup := &candidateLookupStub{response: &authnv3.ResolveMiniProgramNotificationRecipientsResponse{Items: []*authnv3.MiniProgramNotificationRecipient{
		{UserId: "99", LoginIdentityId: "identity-9", AppId: "app-a", OpenId: "openid-9"},
	}}}
	reader := &miniProgramRecipientCandidateReader{links: links, lookup: lookup}
	linkedUsers, err := reader.ListLinkedUsers(context.Background(), "42")
	if err != nil || len(linkedUsers) != 2 || linkedUsers[0].UserID != "100" || linkedUsers[1].UserID != "99" ||
		!reflect.DeepEqual(linkedUsers[1].Relations, []string{"grandparent", "parent"}) {
		t.Fatalf("active link facts were not preserved: users=%+v err=%v", linkedUsers, err)
	}
	candidates, err := reader.ReadCandidates(context.Background(), "42", "app-a", []string{"99"})
	if err != nil {
		t.Fatalf("read candidates: %v", err)
	}
	if !reflect.DeepEqual(lookup.request.GetUserIds(), []string{"99"}) || lookup.request.GetAppId() != "app-a" {
		t.Fatalf("lookup used a Profile ID or wrong AppID: %+v", lookup.request)
	}
	if len(candidates) != 1 || candidates[0].UserID != "99" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
	if !reflect.DeepEqual(candidates[0].Relations, []string{"grandparent", "parent"}) || candidates[0].LoginIdentityID != "identity-9" {
		t.Fatalf("relationship or stable identity was lost: %+v", candidates[0])
	}
}

func TestRecipientCandidatesRejectOutOfScopeIAMResponse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		item *authnv3.MiniProgramNotificationRecipient
	}{
		{"unlinked user", &authnv3.MiniProgramNotificationRecipient{UserId: "42", LoginIdentityId: "identity", AppId: "app-a", OpenId: "open"}},
		{"other AppID", &authnv3.MiniProgramNotificationRecipient{UserId: "99", LoginIdentityId: "identity", AppId: "app-b", OpenId: "open"}},
		{"missing login identity", &authnv3.MiniProgramNotificationRecipient{UserId: "99", AppId: "app-a", OpenId: "open"}},
		{"missing OpenID", &authnv3.MiniProgramNotificationRecipient{UserId: "99", LoginIdentityId: "identity", AppId: "app-a"}},
		{"linked but not selected", &authnv3.MiniProgramNotificationRecipient{UserId: "100", LoginIdentityId: "identity", AppId: "app-a", OpenId: "open"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			links := &candidateLinksStub{response: &identityv2.ListProfileLinksResponse{Items: []*identityv2.ProfileLinkEdge{
				candidateLink("42", "99", identityv2.ProfileLinkRelation_PROFILE_LINK_RELATION_PARENT),
				candidateLink("42", "100", identityv2.ProfileLinkRelation_PROFILE_LINK_RELATION_OTHER),
			}}}
			lookup := &candidateLookupStub{response: &authnv3.ResolveMiniProgramNotificationRecipientsResponse{Items: []*authnv3.MiniProgramNotificationRecipient{tc.item}}}
			_, err := (&miniProgramRecipientCandidateReader{links: links, lookup: lookup}).ReadCandidates(context.Background(), "42", "app-a", []string{"99"})
			if err == nil || strings.Contains(err.Error(), "open") {
				t.Fatalf("out-of-scope identity must fail without exposing OpenID: %v", err)
			}
		})
	}
}

func TestRecipientCandidatesKeepEmptyAndFailureDistinct(t *testing.T) {
	t.Parallel()
	links := &candidateLinksStub{response: &identityv2.ListProfileLinksResponse{}}
	lookup := &candidateLookupStub{}
	reader := &miniProgramRecipientCandidateReader{links: links, lookup: lookup}
	candidates, err := reader.ReadCandidates(context.Background(), "42", "app-a", nil)
	if err != nil || len(candidates) != 0 || lookup.called != 0 {
		t.Fatalf("empty active links must not call IAM recipient service: candidates=%v err=%v calls=%d", candidates, err, lookup.called)
	}
	links.response = &identityv2.ListProfileLinksResponse{Items: []*identityv2.ProfileLinkEdge{
		candidateLink("42", "99", identityv2.ProfileLinkRelation_PROFILE_LINK_RELATION_SELF),
	}}
	lookup.err = errors.New("IAM temporarily unavailable")
	_, err = reader.ReadCandidates(context.Background(), "42", "app-a", []string{"99"})
	if !errors.Is(err, lookup.err) {
		t.Fatalf("IAM failure must remain distinct from empty results: %v", err)
	}
	if _, err := reader.ReadCandidates(context.Background(), "42", " ", []string{"99"}); err == nil {
		t.Fatal("blank actual AppID must be rejected")
	}
	lookup.err = nil
	lookup.called = 0
	if _, err := reader.ReadCandidates(context.Background(), "42", "app-a", []string{"unlinked"}); err == nil || lookup.called != 0 {
		t.Fatalf("product selection outside active links must fail before identity lookup: err=%v calls=%d", err, lookup.called)
	}
}

func TestRecipientCandidatesDoNotTruncateIAMUserLimit(t *testing.T) {
	t.Parallel()
	items := make([]*identityv2.ProfileLinkEdge, 0, maxMiniProgramRecipientUsers+1)
	selected := make([]string, 0, maxMiniProgramRecipientUsers+1)
	for i := 1; i <= maxMiniProgramRecipientUsers+1; i++ {
		userID := strconv.Itoa(i)
		items = append(items, candidateLink("42", userID, identityv2.ProfileLinkRelation_PROFILE_LINK_RELATION_SELF))
		selected = append(selected, userID)
	}
	links := &candidateLinksStub{response: &identityv2.ListProfileLinksResponse{Items: items}}
	lookup := &candidateLookupStub{}
	_, err := (&miniProgramRecipientCandidateReader{links: links, lookup: lookup}).ReadCandidates(context.Background(), "42", "app-a", selected)
	if err == nil || lookup.called != 0 {
		t.Fatalf("over-limit linked users must fail before IAM lookup, got err=%v calls=%d", err, lookup.called)
	}
}
