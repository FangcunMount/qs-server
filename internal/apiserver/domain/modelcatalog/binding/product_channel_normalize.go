package binding

// NormalizeProductChannel maps deprecated persisted values to canonical channels.
func NormalizeProductChannel(channel ProductChannel) ProductChannel {
	switch channel {
	case ProductChannelPersonality:
		return ProductChannelTypology
	default:
		return channel
	}
}

// ProductChannelsEqual reports whether two channels refer to the same taxonomy slot.
func ProductChannelsEqual(left, right ProductChannel) bool {
	return NormalizeProductChannel(left) == NormalizeProductChannel(right)
}

// IsTypologyProductChannel reports typology channel (canonical or legacy persisted value).
func IsTypologyProductChannel(channel ProductChannel) bool {
	return NormalizeProductChannel(channel) == ProductChannelTypology
}

// ProductChannelQueryValues returns persisted values that should match a channel filter.
func ProductChannelQueryValues(channel ProductChannel) []string {
	normalized := NormalizeProductChannel(channel)
	if normalized == ProductChannelTypology {
		return []string{string(ProductChannelTypology), string(ProductChannelPersonality)}
	}
	if normalized == "" {
		return nil
	}
	return []string{string(normalized)}
}
