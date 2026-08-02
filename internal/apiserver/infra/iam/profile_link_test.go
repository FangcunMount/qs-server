package iam

import "testing"

func TestNilProfileLinkServiceIsDisabled(t *testing.T) {
	t.Parallel()
	var service *ProfileLinkService
	if service.IsEnabled() {
		t.Fatal("nil ProfileLinkService must be disabled")
	}
}
