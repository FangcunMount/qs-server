package authzmatrix

import (
	"testing"

	identityv2 "github.com/FangcunMount/iam/v3/api/grpc/iam/identity/v2"
)

func TestCreatedIsolatedUserIDUsesAuthoritativeCreateResponse(t *testing.T) {
	userID, err := createdIsolatedUserID(&identityv2.User{
		Id:       "12345",
		Nickname: SyntheticEvaluatorNickname,
		Status:   identityv2.UserStatus_USER_STATUS_ACTIVE,
	}, SyntheticEvaluatorNickname)
	if err != nil {
		t.Fatalf("createdIsolatedUserID() error = %v", err)
	}
	if userID != "12345" {
		t.Fatalf("createdIsolatedUserID() = %q, want 12345", userID)
	}
}

func TestCreatedIsolatedUserIDRejectsMismatchedResponse(t *testing.T) {
	_, err := createdIsolatedUserID(&identityv2.User{
		Id:       "12345",
		Nickname: SyntheticPlanManagerNickname,
		Status:   identityv2.UserStatus_USER_STATUS_ACTIVE,
	}, SyntheticEvaluatorNickname)
	if err == nil {
		t.Fatal("createdIsolatedUserID() error = nil, want mismatch error")
	}
}
