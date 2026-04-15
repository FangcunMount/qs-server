package response

// LegacyIAMChildIDAlias 在保留兼容字段时，用 canonical profile_id 填充旧 iam_child_id。
func LegacyIAMChildIDAlias(profileID *string) *string {
	return profileID
}
