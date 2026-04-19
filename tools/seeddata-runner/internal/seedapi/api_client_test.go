package seedapi

import "testing"

func TestDecodeResponseDataAssessmentEntryListSupportsFormattedTime(t *testing.T) {
	resp := &Response{
		Data: map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{
					"id":             "entry-1",
					"org_id":         "1",
					"clinician_id":   "2",
					"token":          "token-1",
					"target_type":    "scale",
					"target_code":    "SNAP",
					"is_active":      true,
					"expires_at":     "2026-10-14 12:45:08",
					"qrcode_url":     "https://example.com/qrcode",
					"target_version": "v1",
				},
			},
			"total":       1,
			"page":        1,
			"page_size":   20,
			"total_pages": 1,
		},
	}

	var result AssessmentEntryListResponse
	if err := decodeResponseData(resp, &result); err != nil {
		t.Fatalf("decodeResponseData returned error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].ExpiresAt == nil {
		t.Fatal("expected expires_at to be parsed")
	}
	if got, want := result.Items[0].ExpiresAt.Format("2006-01-02 15:04:05"), "2026-10-14 12:45:08"; got != want {
		t.Fatalf("unexpected expires_at: got=%s want=%s", got, want)
	}
}

func TestDecodeResponseDataRejectsInvalidFlexibleTime(t *testing.T) {
	resp := &Response{
		Data: map[string]interface{}{
			"id":         "testee-1",
			"name":       "Alice",
			"created_at": "not-a-time",
			"updated_at": "2026-04-01 10:00:00",
		},
	}

	var result ApiserverTesteeResponse
	if err := decodeResponseData(resp, &result); err == nil {
		t.Fatal("expected decodeResponseData to fail for invalid created_at")
	}
}
