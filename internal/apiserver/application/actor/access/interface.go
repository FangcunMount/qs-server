package access

// TesteeAccessService exposes only action-paired backstage data ranges.
// Doctor and guardian relationship access has independent application entrances.
type TesteeAccessService interface{ StoreScopeAccess }
