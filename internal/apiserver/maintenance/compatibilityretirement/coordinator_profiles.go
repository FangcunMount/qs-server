package retirement

import (
	"crypto/sha256"
	"slices"
	"strconv"
)

// These bound an optional optimization, not the set of legal source facts.
// A saturated pool leaves the candidate's complete private list unchanged.
const (
	maxCandidateProfiles     = 4096
	maxCandidateProfileBytes = uint64(8 << 20)
	candidateProfileOverhead = uint64(256)
)

type candidateProfileRole uint8

const (
	candidateOriginalRunMissing candidateProfileRole = iota + 1
	candidateHistoricalGaps
	candidateBlockingReasons
	candidateRequiredAdapters
)

type candidateProfileKey struct {
	role   candidateProfileRole
	digest [32]byte
}

type candidateProfileEntry struct {
	values []string
	charge uint64
}

// The pool belongs to one coordinator. Its lists are immutable after insertion;
// public candidate exports still use the existing defensive clone path.
type candidateProfilePool struct {
	entries map[candidateProfileKey]candidateProfileEntry
	bytes   uint64
}

func candidateProfileDigest(role candidateProfileRole, values []string) candidateProfileKey {
	h := sha256.New()
	sourceFrame(h, []byte("historical-candidate-profile/v1"), false)
	sourceFrame(h, []byte(strconv.Itoa(int(role))), false)
	sourceFrame(h, []byte(strconv.Itoa(len(values))), false)
	for _, value := range values {
		sourceFrame(h, []byte(value), false)
	}
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return candidateProfileKey{role: role, digest: digest}
}

// The conservative accounting covers list headers and text plus per-entry
// overhead. It is not a measurement or guarantee of Go heap/process RSS.
func candidateProfileCharge(values []string) (uint64, bool) {
	charge := candidateProfileOverhead
	for _, value := range values {
		n := uint64(len(value))
		if n > maxCandidateProfileBytes || charge > maxCandidateProfileBytes-16 || n > maxCandidateProfileBytes-charge-16 {
			return 0, false
		}
		charge += 16 + n
	}
	return charge, true
}

func (p *candidateProfilePool) intern(role candidateProfileRole, values []string) ([]string, error) {
	if p == nil || role < candidateOriginalRunMissing || role > candidateRequiredAdapters || len(p.entries) > maxCandidateProfiles || p.bytes > maxCandidateProfileBytes || p.bytes < uint64(len(p.entries))*candidateProfileOverhead || (len(p.entries) == 0) != (p.bytes == 0) {
		return nil, ErrCoordinatorPage
	}
	// Do not normalize nil or a non-nil empty list. The complete candidate hash
	// distinguishes them, and neither requires cache storage.
	if len(values) == 0 {
		return values, nil
	}
	charge, cacheable := candidateProfileCharge(values)
	if !cacheable {
		return values, nil
	}
	key := candidateProfileDigest(role, values)
	if cached, exists := p.entries[key]; exists {
		// A hash collision or changed private cache never silently shares values.
		if cached.charge != charge || cached.charge > p.bytes || cached.values == nil || len(cached.values) == 0 || len(cached.values) != cap(cached.values) || !slices.Equal(cached.values, values) {
			return nil, ErrCoordinatorPage
		}
		return cached.values, nil
	}
	if len(p.entries) == maxCandidateProfiles || charge > maxCandidateProfileBytes-p.bytes {
		return values, nil
	}
	copyValues := make([]string, len(values))
	copy(copyValues, values)
	if p.entries == nil {
		p.entries = make(map[candidateProfileKey]candidateProfileEntry)
	}
	p.entries[key] = candidateProfileEntry{values: copyValues, charge: charge}
	p.bytes += charge
	return copyValues, nil
}

// All four qualification routes enter consume only after final classification.
// Preserve original order, every unknown/gap label and every other field; this
// does not rerun classification, change a hash protocol or create authority.
func (c *HistoricalCoordinator) shareCandidateProfiles(rows []HistoricalCandidate) error {
	for i := range rows {
		value := &rows[i]
		for _, profile := range []struct {
			role   candidateProfileRole
			values *[]string
		}{
			{candidateOriginalRunMissing, &value.OriginalRunMissing},
			{candidateHistoricalGaps, &value.HistoricalGaps},
			{candidateBlockingReasons, &value.BlockingReasons},
			{candidateRequiredAdapters, &value.RequiredAdapters},
		} {
			shared, err := c.profiles.intern(profile.role, *profile.values)
			if err != nil {
				return err
			}
			*profile.values = shared
		}
	}
	return nil
}
