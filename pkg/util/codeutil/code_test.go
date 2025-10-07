package codeutil

import (
	"encoding/json"
	"testing"
)

func TestCode(t *testing.T) {
	// 测试创建Code
	code1 := NewCode("TEST001")
	if code1.Value() != "TEST001" {
		t.Errorf("Expected value 'TEST001', got '%s'", code1.Value())
	}

	// 测试String方法
	if code1.String() != "TEST001" {
		t.Errorf("Expected string 'TEST001', got '%s'", code1.String())
	}

	// 测试Equals方法
	code2 := NewCode("TEST001")
	if !code1.Equals(code2) {
		t.Error("Expected codes to be equal")
	}

	code3 := NewCode("TEST002")
	if code1.Equals(code3) {
		t.Error("Expected codes to be different")
	}

	// 测试IsZero方法
	emptyCode := NewCode("")
	if !emptyCode.IsZero() {
		t.Error("Expected empty code to be zero")
	}

	if code1.IsZero() {
		t.Error("Expected non-empty code")
	}

	// 测试IsEmpty方法
	if !emptyCode.IsEmpty() {
		t.Error("Expected empty code")
	}

	if code1.IsEmpty() {
		t.Error("Expected non-empty code")
	}
}

func TestGenerateNewCode(t *testing.T) {
	// 测试生成唯一编码
	code1, err := GenerateNewCode()
	if err != nil {
		t.Fatalf("Failed to generate code: %v", err)
	}

	if code1.IsEmpty() {
		t.Error("Generated code should not be empty")
	}

	// 生成多个编码,确保唯一性
	code2, err := GenerateNewCode()
	if err != nil {
		t.Fatalf("Failed to generate second code: %v", err)
	}

	if code1.Equals(code2) {
		t.Error("Generated codes should be unique")
	}

	t.Logf("Generated code1: %s", code1.Value())
	t.Logf("Generated code2: %s", code2.Value())
}

func TestCodeJSON(t *testing.T) {
	// 测试JSON序列化
	code := NewCode("JSON001")

	data, err := json.Marshal(code)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	expected := `"JSON001"`
	if string(data) != expected {
		t.Errorf("Expected JSON '%s', got '%s'", expected, string(data))
	}

	// 测试JSON反序列化
	var newCode Code
	err = json.Unmarshal(data, &newCode)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !newCode.Equals(code) {
		t.Errorf("Expected code '%s', got '%s'", code.Value(), newCode.Value())
	}
}

func TestCodeUniqueness(t *testing.T) {
	// 测试批量生成编码的唯一性
	const count = 100
	codes := make(map[string]bool, count)

	for i := 0; i < count; i++ {
		code, err := GenerateNewCode()
		if err != nil {
			t.Fatalf("Failed to generate code at iteration %d: %v", i, err)
		}

		codeValue := code.Value()
		if codes[codeValue] {
			t.Errorf("Duplicate code generated: %s", codeValue)
		}
		codes[codeValue] = true
	}

	if len(codes) != count {
		t.Errorf("Expected %d unique codes, got %d", count, len(codes))
	}
}

func BenchmarkNewCode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewCode("BENCH001")
	}
}

func BenchmarkGenerateNewCode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = GenerateNewCode()
	}
}

func BenchmarkCodeEquals(b *testing.B) {
	code1 := NewCode("BENCH001")
	code2 := NewCode("BENCH001")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = code1.Equals(code2)
	}
}

func BenchmarkCodeJSON(b *testing.B) {
	code := NewCode("BENCH001")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(code)
	}
}
