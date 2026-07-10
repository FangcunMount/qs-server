package grpcclient

// CategoryOutput is shared by published catalogue projections that expose a
// typed category option. It is not a scale-specific DTO.
type CategoryOutput struct {
	Value string
	Label string
}
