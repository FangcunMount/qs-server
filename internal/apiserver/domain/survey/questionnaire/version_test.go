package questionnaire

import (
	"testing"
)

// TestVersion_IncrementMinor 测试小版本递增
func TestVersion_IncrementMinor(t *testing.T) {
	tests := []struct {
		name     string
		version  Version
		expected Version
	}{
		{
			name:     "0.0.1 -> 0.0.2",
			version:  Version("0.0.1"),
			expected: Version("0.0.2"),
		},
		{
			name:     "0.0.9 -> 0.0.10",
			version:  Version("0.0.9"),
			expected: Version("0.0.10"),
		},
		{
			name:     "1.0.1 -> 1.0.2",
			version:  Version("1.0.1"),
			expected: Version("1.0.2"),
		},
		{
			name:     "1.0.99 -> 1.0.100",
			version:  Version("1.0.99"),
			expected: Version("1.0.100"),
		},
		{
			name:     "5.2.8 -> 5.2.9",
			version:  Version("5.2.8"),
			expected: Version("5.2.9"),
		},
		{
			name:     "两位版本 1.0 -> 1.0.1",
			version:  Version("1.0"),
			expected: Version("1.0.1"),
		},
		{
			name:     "单位版本 1 -> 1.0.1",
			version:  Version("1"),
			expected: Version("1.0.1"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.version.IncrementMinor()
			if result != tt.expected {
				t.Errorf("IncrementMinor() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestVersion_IncrementMajor 测试大版本递增
func TestVersion_IncrementMajor(t *testing.T) {
	tests := []struct {
		name     string
		version  Version
		expected Version
	}{
		{
			name:     "0.0.1 -> 1.0.1",
			version:  Version("0.0.1"),
			expected: Version("1.0.1"),
		},
		{
			name:     "0.0.99 -> 1.0.1",
			version:  Version("0.0.99"),
			expected: Version("1.0.1"),
		},
		{
			name:     "1.0.1 -> 2.0.1",
			version:  Version("1.0.1"),
			expected: Version("2.0.1"),
		},
		{
			name:     "1.5.8 -> 2.0.1",
			version:  Version("1.5.8"),
			expected: Version("2.0.1"),
		},
		{
			name:     "5.2.8 -> 6.0.1",
			version:  Version("5.2.8"),
			expected: Version("6.0.1"),
		},
		{
			name:     "9.99.99 -> 10.0.1",
			version:  Version("9.99.99"),
			expected: Version("10.0.1"),
		},
		{
			name:     "两位版本 1.5 -> 2.0.1",
			version:  Version("1.5"),
			expected: Version("2.0.1"),
		},
		{
			name:     "单位版本 5 -> 6.0.1",
			version:  Version("5"),
			expected: Version("6.0.1"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.version.IncrementMajor()
			if result != tt.expected {
				t.Errorf("IncrementMajor() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestVersion_Validate 测试版本验证
func TestVersion_Validate(t *testing.T) {
	tests := []struct {
		name    string
		version Version
		wantErr bool
	}{
		{
			name:    "有效版本 0.0.1",
			version: Version("0.0.1"),
			wantErr: false,
		},
		{
			name:    "有效版本 1.0.1",
			version: Version("1.0.1"),
			wantErr: false,
		},
		{
			name:    "有效版本 10.20.30",
			version: Version("10.20.30"),
			wantErr: false,
		},
		{
			name:    "有效版本 1.0",
			version: Version("1.0"),
			wantErr: false,
		},
		{
			name:    "有效版本 1",
			version: Version("1"),
			wantErr: false,
		},
		{
			name:    "有效版本 v1.0.1",
			version: Version("v1.0.1"),
			wantErr: false,
		},
		{
			name:    "空版本",
			version: Version(""),
			wantErr: true,
		},
		{
			name:    "只有v",
			version: Version("v"),
			wantErr: true,
		},
		{
			name:    "无效格式 v1.0.a",
			version: Version("v1.0.a"),
			wantErr: true,
		},
		{
			name:    "无效格式 abc",
			version: Version("abc"),
			wantErr: true,
		},
		{
			name:    "无效格式 1..0",
			version: Version("1..0"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.version.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestVersion_Equals 测试版本相等性
func TestVersion_Equals(t *testing.T) {
	tests := []struct {
		name     string
		v1       Version
		v2       Version
		expected bool
	}{
		{
			name:     "相同版本",
			v1:       Version("1.0.1"),
			v2:       Version("1.0.1"),
			expected: true,
		},
		{
			name:     "不同版本",
			v1:       Version("1.0.1"),
			v2:       Version("1.0.2"),
			expected: false,
		},
		{
			name:     "空版本",
			v1:       Version(""),
			v2:       Version(""),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v1.Equals(tt.v2)
			if result != tt.expected {
				t.Errorf("Equals() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestVersion_IsEmpty 测试版本是否为空
func TestVersion_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		version  Version
		expected bool
	}{
		{
			name:     "空版本",
			version:  Version(""),
			expected: true,
		},
		{
			name:     "非空版本",
			version:  Version("1.0.1"),
			expected: false,
		},
		{
			name:     "零值",
			version:  Version("0.0.0"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.version.IsEmpty()
			if result != tt.expected {
				t.Errorf("IsEmpty() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestVersion_Value 测试获取版本值
func TestVersion_Value(t *testing.T) {
	tests := []struct {
		name     string
		version  Version
		expected string
	}{
		{
			name:     "标准版本",
			version:  Version("1.0.1"),
			expected: "1.0.1",
		},
		{
			name:     "带v前缀",
			version:  Version("v2.0.0"),
			expected: "v2.0.0",
		},
		{
			name:     "空版本",
			version:  Version(""),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.version.Value()
			if result != tt.expected {
				t.Errorf("Value() = %v, want %v", result, tt.expected)
			}
		})
	}
}
