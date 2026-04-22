package cachepolicy

import (
	"bytes"
	"compress/gzip"
	"io"
	"time"

	rediskit "github.com/FangcunMount/component-base/pkg/redis"
)

// EnableCompression 控制默认压缩开关。
var EnableCompression bool

// PolicySwitch 表示缓存策略开关的三态值：
// inherit 继承父策略，enabled 显式开启，disabled 显式关闭。
type PolicySwitch uint8

const (
	PolicySwitchInherit PolicySwitch = iota
	PolicySwitchEnabled
	PolicySwitchDisabled
)

// PolicySwitchFromBool 将普通布尔值转换为显式策略开关。
func PolicySwitchFromBool(value bool) PolicySwitch {
	if value {
		return PolicySwitchEnabled
	}
	return PolicySwitchDisabled
}

// PolicySwitchFromBoolPtr 将可选布尔值转换为三态策略开关。
func PolicySwitchFromBoolPtr(value *bool) PolicySwitch {
	if value == nil {
		return PolicySwitchInherit
	}
	return PolicySwitchFromBool(*value)
}

// Enabled 返回最终布尔值；inherit 时回退到 defaultValue。
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

// CachePolicyKey 定义缓存对象级策略键。
type CachePolicyKey string

const (
	PolicyScale            CachePolicyKey = "scale"
	PolicyScaleList        CachePolicyKey = "scale_list"
	PolicyQuestionnaire    CachePolicyKey = "questionnaire"
	PolicyAssessmentDetail CachePolicyKey = "assessment_detail"
	PolicyAssessmentList   CachePolicyKey = "assessment_list"
	PolicyTestee           CachePolicyKey = "testee"
	PolicyPlan             CachePolicyKey = "plan"
	PolicyStatsQuery       CachePolicyKey = "stats_query"
)

// CachePolicy 描述单类缓存对象的 TTL / negative cache / compression 策略。
type CachePolicy struct {
	TTL          time.Duration
	NegativeTTL  time.Duration
	Negative     PolicySwitch
	Compress     PolicySwitch
	Singleflight PolicySwitch
	JitterRatio  float64
}

// MergeWith 以 parent 为默认值叠加当前策略。
func (p CachePolicy) MergeWith(parent CachePolicy) CachePolicy {
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

// TTLOr 返回显式 TTL 或给定默认值。
func (p CachePolicy) TTLOr(defaultTTL time.Duration) time.Duration {
	if p.TTL > 0 {
		return p.TTL
	}
	return defaultTTL
}

// NegativeTTLOr 返回显式 negative TTL 或给定默认值。
func (p CachePolicy) NegativeTTLOr(defaultTTL time.Duration) time.Duration {
	if p.NegativeTTL > 0 {
		return p.NegativeTTL
	}
	return defaultTTL
}

// NegativeEnabled 返回 negative cache 是否生效。
func (p CachePolicy) NegativeEnabled(defaultValue bool) bool {
	return p.Negative.Enabled(defaultValue)
}

// JitterTTL 根据策略内 jitter 比例为 TTL 增加抖动。
func (p CachePolicy) JitterTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return ttl
	}
	ratio := p.JitterRatio
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	return rediskit.JitterTTL(ttl, ratio)
}

// CompressValue 根据策略决定是否压缩 payload。
func (p CachePolicy) CompressValue(data []byte) []byte {
	return CompressData(data, p.Compress.Enabled(EnableCompression))
}

// DecompressValue 对缓存值做向后兼容解压。
func (p CachePolicy) DecompressValue(data []byte) []byte {
	return DecompressData(data)
}

// SingleflightEnabled 返回 singleflight 是否生效。
func (p CachePolicy) SingleflightEnabled(defaultValue bool) bool {
	return p.Singleflight.Enabled(defaultValue)
}

// CompressData gzip 压缩（可选）。
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

// DecompressData 尝试解压 gzip，不是 gzip 时返回原数据。
func DecompressData(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return data
	}
	defer func() {
		_ = r.Close()
	}()
	out, err := io.ReadAll(r)
	if err != nil {
		return data
	}
	return out
}
