package request

import "strings"

// CanonicalProfileID 返回当前请求使用的 canonical profile_id。
func (r *GetTesteeByProfileIDRequest) CanonicalProfileID() string {
	if r == nil {
		return ""
	}
	if profileID := strings.TrimSpace(r.ProfileID); profileID != "" {
		return profileID
	}
	return strings.TrimSpace(r.IAMChildID)
}
