package messagingruntime

import (
	"errors"
	"math"
	"strings"
	"time"
)

// Host policy: this state neither confirms messages nor authorizes business retry.
// The sampler owns synchronization. A protection episode stays latched until a
// named review of the same generation observes fresh healthy metadata/capacity.
const (
	protocolWarningAge    = 120 * time.Second
	protocolProtectionAge = 300 * time.Second
	protocolSaturationAge = 120 * time.Second
)

var ErrProtocolProtection = errors.New("transaction protocol protection requires fresh evidence and named review")

type protocolSample struct {
	reserved, preparing, gaps, unaged, untrackedAborted, unobservedCommitted, conflicts int64
	expiryAge, confirmationAge                                                          float64
}

func (s protocolSample) valid() bool {
	return s.reserved >= 0 && s.preparing >= 0 && s.gaps >= 0 && s.unaged >= 0 && s.untrackedAborted >= 0 && s.unobservedCommitted >= 0 && s.conflicts >= 0 &&
		s.expiryAge >= 0 && s.confirmationAge >= 0 && !math.IsNaN(s.expiryAge) && !math.IsNaN(s.confirmationAge) && !math.IsInf(s.expiryAge, 0) && !math.IsInf(s.confirmationAge, 0)
}

type protocolProtection struct {
	sample                                            protocolSample
	available                                         bool
	lastSuccess, lastPoll, errorSince, saturatedSince time.Time
	errors                                            int64
	reason                                            string
	generation                                        uint64
}

func (p *protocolProtection) latch(reason string) {
	if p.reason == "" {
		p.reason = reason
		p.generation++
	}
}

func (p *protocolProtection) observe(now time.Time, sample protocolSample, readErr error, bindingConflict bool, full bool, freshness time.Duration) {
	if !p.lastPoll.IsZero() && now.Sub(p.lastPoll) > freshness {
		p.saturatedSince = time.Time{}
	}
	p.lastPoll = now
	if full {
		if p.saturatedSince.IsZero() {
			p.saturatedSince = now
		}
		if now.Sub(p.saturatedSince) >= protocolSaturationAge {
			p.latch("checker_saturated")
		}
	} else {
		p.saturatedSince = time.Time{}
	}
	if readErr != nil {
		p.available = false
		p.errors++
		if p.errorSince.IsZero() {
			p.errorSince = now
		}
		if bindingConflict {
			p.latch("binding_conflict")
		} else if now.Sub(p.errorSince) >= protocolWarningAge {
			p.latch("observation_unavailable")
		}
		return
	}
	if !sample.valid() {
		p.available = false
		p.errors++
		p.latch("invalid_observation")
		return
	}
	p.available = true
	p.lastSuccess = now
	p.errorSince = time.Time{}
	p.sample = sample
	switch {
	case sample.conflicts > 0:
		p.latch("metadata_conflict")
	case sample.unaged > 0:
		p.latch("confirmation_age_unavailable")
	case sample.unobservedCommitted > 0:
		p.latch("committed_half_unobserved")
	case sample.gaps > 0 && sample.confirmationAge >= protocolProtectionAge.Seconds():
		p.latch("confirmation_gap")
	case sample.preparing > 0 && sample.expiryAge >= protocolProtectionAge.Seconds():
		p.latch("preparation_expired")
	}
}

func (p *protocolProtection) fresh(now time.Time, freshness time.Duration) bool {
	return p.available && !p.lastSuccess.IsZero() && now.Sub(p.lastSuccess) >= 0 && now.Sub(p.lastSuccess) <= freshness
}
func (p *protocolProtection) allowed(now time.Time, freshness time.Duration) bool {
	return p.reason == "" && p.fresh(now, freshness)
}
func (p *protocolProtection) review(now time.Time, freshness time.Duration, generation uint64, review string, ready, full bool) error {
	if strings.TrimSpace(review) == "" || len(review) > 128 || generation != p.generation || p.reason == "" || !p.fresh(now, freshness) || !ready || full ||
		p.sample.conflicts > 0 || p.sample.unaged > 0 || p.sample.unobservedCommitted > 0 ||
		(p.sample.gaps > 0 && p.sample.confirmationAge >= protocolWarningAge.Seconds()) ||
		(p.sample.preparing > 0 && p.sample.expiryAge >= protocolWarningAge.Seconds()) {
		return ErrProtocolProtection
	}
	p.reason = ""
	return nil
}
