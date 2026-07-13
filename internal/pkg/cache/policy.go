package cache

import (
	"bytes"
	"compress/gzip"
	"io"
	"math/rand"
	"time"
)

// PolicySwitch is a tri-state switch: inherit, explicitly enabled or disabled.
type PolicySwitch uint8

const (
	PolicySwitchInherit PolicySwitch = iota
	PolicySwitchEnabled
	PolicySwitchDisabled
)

func PolicySwitchFromBool(value bool) PolicySwitch {
	if value {
		return PolicySwitchEnabled
	}
	return PolicySwitchDisabled
}

func PolicySwitchFromBoolPtr(value *bool) PolicySwitch {
	if value == nil {
		return PolicySwitchInherit
	}
	return PolicySwitchFromBool(*value)
}

func (s PolicySwitch) Enabled(defaultValue bool) bool {
	switch s {
	case PolicySwitchEnabled:
		return true
	case PolicySwitchDisabled:
		return false
	default:
		return defaultValue
	}
}

// Policy describes the shared TTL, negative-cache, compression and
// miss-coalescing behavior for one cache capability.
type Policy struct {
	TTL          time.Duration
	NegativeTTL  time.Duration
	Negative     PolicySwitch
	Compress     PolicySwitch
	Singleflight PolicySwitch
	JitterRatio  float64
}

func (p Policy) MergeWith(parent Policy) Policy {
	merged := parent
	if p.TTL > 0 {
		merged.TTL = p.TTL
	}
	if p.NegativeTTL > 0 {
		merged.NegativeTTL = p.NegativeTTL
	}
	if p.Negative != PolicySwitchInherit {
		merged.Negative = p.Negative
	}
	if p.Compress != PolicySwitchInherit {
		merged.Compress = p.Compress
	}
	if p.Singleflight != PolicySwitchInherit {
		merged.Singleflight = p.Singleflight
	}
	if p.JitterRatio > 0 {
		merged.JitterRatio = p.JitterRatio
	}
	return merged
}

func (p Policy) TTLOr(defaultTTL time.Duration) time.Duration {
	if p.TTL > 0 {
		return p.TTL
	}
	return defaultTTL
}

func (p Policy) NegativeTTLOr(defaultTTL time.Duration) time.Duration {
	if p.NegativeTTL > 0 {
		return p.NegativeTTL
	}
	return defaultTTL
}

func (p Policy) NegativeEnabled(defaultValue bool) bool {
	return p.Negative.Enabled(defaultValue)
}

func (p Policy) SingleflightEnabled(defaultValue bool) bool {
	return p.Singleflight.Enabled(defaultValue)
}

func (p Policy) JitterTTL(ttl time.Duration) time.Duration {
	return JitterTTL(ttl, p.JitterRatio)
}

func (p Policy) CompressValue(data []byte) []byte {
	return CompressData(data, p.Compress.Enabled(false))
}

func (p Policy) DecompressValue(data []byte) []byte {
	return DecompressData(data)
}

// JitterTTL adds a random [0, ttl*ratio] extension to spread expirations.
func JitterTTL(ttl time.Duration, ratio float64) time.Duration {
	if ttl <= 0 {
		return ttl
	}
	if ratio <= 0 {
		return ttl
	}
	if ratio > 1 {
		ratio = 1
	}
	extra := time.Duration(rand.Float64() * float64(ttl) * ratio)
	return ttl + extra
}

func CompressData(data []byte, enable bool) []byte {
	if !enable || len(data) == 0 {
		return data
	}
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return data
	}
	_ = w.Close()
	return buf.Bytes()
}

func DecompressData(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return data
	}
	defer func() { _ = r.Close() }()
	out, err := io.ReadAll(r)
	if err != nil {
		return data
	}
	return out
}
