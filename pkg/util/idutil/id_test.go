package idutil

import (
	"encoding/json"
	"testing"
)

func TestUInt64ID(t *testing.T) {
	// 测试创建ID
	id1 := NewUInt64ID(12345)
	if id1.Value() != 12345 {
		t.Errorf("Expected value 12345, got %d", id1.Value())
	}

	// 测试String方法
	if id1.String() != "12345" {
		t.Errorf("Expected string '12345', got '%s'", id1.String())
	}

	// 测试Equals方法
	id2 := NewUInt64ID(12345)
	if !id1.Equals(id2) {
		t.Error("Expected IDs to be equal")
	}

	id3 := NewUInt64ID(54321)
	if id1.Equals(id3) {
		t.Error("Expected IDs to be different")
	}

	// 测试IsZero方法
	zeroID := NewUInt64ID(0)
	if !zeroID.IsZero() {
		t.Error("Expected zero ID")
	}

	if id1.IsZero() {
		t.Error("Expected non-zero ID")
	}
}

func TestStringID(t *testing.T) {
	// 测试创建字符串ID
	id := NewStringID("user-123")
	if id.Value() != "user-123" {
		t.Errorf("Expected value 'user-123', got '%s'", id.Value())
	}

	// 测试String方法
	if id.String() != "user-123" {
		t.Errorf("Expected string 'user-123', got '%s'", id.String())
	}

	// 测试IsZero方法
	emptyID := NewStringID("")
	if !emptyID.IsZero() {
		t.Error("Expected empty string to be zero")
	}
}

func TestInt64ID(t *testing.T) {
	// 测试创建Int64 ID
	id := NewInt64ID(-12345)
	if id.Value() != -12345 {
		t.Errorf("Expected value -12345, got %d", id.Value())
	}
}

func TestIDJSON(t *testing.T) {
	// 测试JSON序列化和反序列化
	tests := []struct {
		name     string
		id       UInt64ID
		expected string
	}{
		{"normal", NewUInt64ID(12345), "12345"},
		{"zero", NewUInt64ID(0), "0"},
		{"large", NewUInt64ID(9223372036854775807), "9223372036854775807"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 测试序列化
			data, err := json.Marshal(tt.id)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			if string(data) != tt.expected {
				t.Errorf("Expected JSON '%s', got '%s'", tt.expected, string(data))
			}

			// 测试反序列化
			var id UInt64ID
			err = json.Unmarshal(data, &id)
			if err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if !id.Equals(tt.id) {
				t.Errorf("Expected ID %d, got %d", tt.id.Value(), id.Value())
			}
		})
	}
}

func TestStringIDJSON(t *testing.T) {
	// 测试字符串ID的JSON序列化
	id := NewStringID("test-id-123")

	data, err := json.Marshal(id)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	expected := `"test-id-123"`
	if string(data) != expected {
		t.Errorf("Expected JSON %s, got %s", expected, string(data))
	}

	// 测试反序列化
	var newID StringID
	err = json.Unmarshal(data, &newID)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !newID.Equals(id) {
		t.Errorf("Expected ID '%s', got '%s'", id.Value(), newID.Value())
	}
}

func TestIDGeneric(t *testing.T) {
	// 测试泛型ID创建
	type CustomID = ID[uint64]

	id := NewID[uint64](999)
	if id.Value() != 999 {
		t.Errorf("Expected value 999, got %d", id.Value())
	}

	// 测试类型别名
	customID := CustomID(id)
	if customID.Value() != 999 {
		t.Errorf("Expected value 999, got %d", customID.Value())
	}
}

func BenchmarkNewUInt64ID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewUInt64ID(uint64(i))
	}
}

func BenchmarkIDEquals(b *testing.B) {
	id1 := NewUInt64ID(12345)
	id2 := NewUInt64ID(12345)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = id1.Equals(id2)
	}
}

func BenchmarkIDJSON(b *testing.B) {
	id := NewUInt64ID(12345)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(id)
	}
}
