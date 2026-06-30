package typology

import "testing"

func TestPayloadIsPublished(t *testing.T) {
	t.Run("legacy empty status is published", func(t *testing.T) {
		if !(&Payload{Status: ""}).IsPublished() {
			t.Fatal("empty status should be treated as published for legacy payloads")
		}
	})

	t.Run("explicit published", func(t *testing.T) {
		if !(&Payload{Status: "published"}).IsPublished() {
			t.Fatal("published status should pass")
		}
	})

	t.Run("draft is rejected", func(t *testing.T) {
		if (&Payload{Status: "draft"}).IsPublished() {
			t.Fatal("draft status should be rejected")
		}
	})

	t.Run("archived is rejected", func(t *testing.T) {
		if (&Payload{Status: "archived"}).IsPublished() {
			t.Fatal("archived status should be rejected")
		}
	})
}
