package questionnaire

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/calculation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// TestVersioning_InitializeVersion 测试版本初始化
func TestVersioning_InitializeVersion(t *testing.T) {
	t.Run("初始化空版本为0.0.1", func(t *testing.T) {
		q, _ := NewQuestionnaire(
			meta.NewCode("TEST001"),
			"测试问卷",
		)

		versioning := Versioning{}
		err := versioning.InitializeVersion(q)

		if err != nil {
			t.Errorf("InitializeVersion() error = %v", err)
			return
		}

		if q.GetVersion().Value() != "0.0.1" {
			t.Errorf("InitializeVersion() version = %v, want 0.0.1", q.GetVersion().Value())
		}
	})

	t.Run("不重复初始化已有版本", func(t *testing.T) {
		q, _ := NewQuestionnaire(
			meta.NewCode("TEST001"),
			"测试问卷",
			WithVersion(NewVersion("1.0.5")),
		)

		versioning := Versioning{}
		err := versioning.InitializeVersion(q)

		if err != nil {
			t.Errorf("InitializeVersion() error = %v", err)
			return
		}

		// 版本应该保持不变
		if q.GetVersion().Value() != "1.0.5" {
			t.Errorf("InitializeVersion() version = %v, want 1.0.5", q.GetVersion().Value())
		}
	})
}

// TestVersioning_IncrementMinorVersion 测试小版本递增
func TestVersioning_IncrementMinorVersion(t *testing.T) {
	tests := []struct {
		name            string
		initialVersion  string
		expectedVersion string
		wantErr         bool
	}{
		{
			name:            "从0.0.1递增到0.0.2",
			initialVersion:  "0.0.1",
			expectedVersion: "0.0.2",
			wantErr:         false,
		},
		{
			name:            "从0.0.5递增到0.0.6",
			initialVersion:  "0.0.5",
			expectedVersion: "0.0.6",
			wantErr:         false,
		},
		{
			name:            "从1.0.1递增到1.0.2",
			initialVersion:  "1.0.1",
			expectedVersion: "1.0.2",
			wantErr:         false,
		},
		{
			name:            "从1.0.99递增到1.0.100",
			initialVersion:  "1.0.99",
			expectedVersion: "1.0.100",
			wantErr:         false,
		},
		{
			name:            "从空版本初始化为0.0.1",
			initialVersion:  "",
			expectedVersion: "0.0.1",
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, _ := NewQuestionnaire(
				meta.NewCode("TEST001"),
				"测试问卷",
				WithVersion(NewVersion(tt.initialVersion)),
			)

			versioning := Versioning{}
			err := versioning.IncrementMinorVersion(q)

			if (err != nil) != tt.wantErr {
				t.Errorf("IncrementMinorVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && q.GetVersion().Value() != tt.expectedVersion {
				t.Errorf("IncrementMinorVersion() version = %v, want %v", q.GetVersion().Value(), tt.expectedVersion)
			}
		})
	}
}

// TestVersioning_IncrementMajorVersion 测试大版本递增
func TestVersioning_IncrementMajorVersion(t *testing.T) {
	tests := []struct {
		name            string
		initialVersion  string
		expectedVersion string
		wantErr         bool
	}{
		{
			name:            "从0.0.5递增到1.0.1",
			initialVersion:  "0.0.5",
			expectedVersion: "1.0.1",
			wantErr:         false,
		},
		{
			name:            "从0.0.99递增到1.0.1",
			initialVersion:  "0.0.99",
			expectedVersion: "1.0.1",
			wantErr:         false,
		},
		{
			name:            "从1.0.3递增到2.0.1",
			initialVersion:  "1.0.3",
			expectedVersion: "2.0.1",
			wantErr:         false,
		},
		{
			name:            "从5.2.8递增到6.0.1",
			initialVersion:  "5.2.8",
			expectedVersion: "6.0.1",
			wantErr:         false,
		},
		{
			name:            "从空版本初始化为1.0.1",
			initialVersion:  "",
			expectedVersion: "1.0.1",
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, _ := NewQuestionnaire(
				meta.NewCode("TEST001"),
				"测试问卷",
				WithVersion(NewVersion(tt.initialVersion)),
			)

			versioning := Versioning{}
			err := versioning.IncrementMajorVersion(q)

			if (err != nil) != tt.wantErr {
				t.Errorf("IncrementMajorVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && q.GetVersion().Value() != tt.expectedVersion {
				t.Errorf("IncrementMajorVersion() version = %v, want %v", q.GetVersion().Value(), tt.expectedVersion)
			}
		})
	}
}

// TestLifecycle_Publish_WithMajorVersionIncrement 测试发布时大版本递增
func TestLifecycle_Publish_WithMajorVersionIncrement(t *testing.T) {
	t.Run("首次发布从0.0.5到1.0.1", func(t *testing.T) {
		// 创建问卷并设置版本为0.0.5（经过多次存草稿）
		q, _ := NewQuestionnaire(
			meta.NewCode("TEST001"),
			"测试问卷",
			WithVersion(NewVersion("0.0.5")),
		)

		// 添加一个问题
		question, _ := NewQuestion(
			WithCode(meta.NewCode("Q1")),
			WithStem("测试问题"),
			WithQuestionType(TypeText),
			WithCalculationRule(calculation.FormulaTypeScore),
		)
		q.questions = []Question{question}

		// 发布问卷
		lifecycle := NewLifecycle()
		err := lifecycle.Publish(context.TODO(), q)

		if err != nil {
			t.Errorf("Publish() error = %v", err)
			return
		}

		// 验证状态已变更
		if !q.IsPublished() {
			t.Error("Publish() 问卷应该是已发布状态")
		}

		// 验证版本已递增为1.0.1
		if q.GetVersion().Value() != "1.0.1" {
			t.Errorf("Publish() version = %v, want 1.0.1", q.GetVersion().Value())
		}
	})

	t.Run("发布后再次发布从1.0.3到2.0.1", func(t *testing.T) {
		// 创建已发布的问卷，版本为1.0.3（发布后又编辑并存草稿）
		q, _ := NewQuestionnaire(
			meta.NewCode("TEST002"),
			"测试问卷",
			WithVersion(NewVersion("1.0.3")),
			WithStatus(STATUS_DRAFT), // 编辑后状态回到草稿
		)

		// 添加一个问题
		question, _ := NewQuestion(
			WithCode(meta.NewCode("Q1")),
			WithStem("测试问题"),
			WithQuestionType(TypeText),
			WithCalculationRule(calculation.FormulaTypeScore),
		)
		q.questions = []Question{question}

		// 再次发布问卷
		lifecycle := NewLifecycle()
		err := lifecycle.Publish(context.TODO(), q)

		if err != nil {
			t.Errorf("Publish() error = %v", err)
			return
		}

		// 验证版本已递增为2.0.1
		if q.GetVersion().Value() != "2.0.1" {
			t.Errorf("Publish() version = %v, want 2.0.1", q.GetVersion().Value())
		}
	})
}

// TestVersionWorkflow 测试完整的版本工作流
func TestVersionWorkflow(t *testing.T) {
	t.Run("完整工作流:创建-存草稿-发布-编辑-存草稿-再发布", func(t *testing.T) {
		// 1. 创建问卷，初始版本0.0.1
		q, _ := NewQuestionnaire(
			meta.NewCode("TEST001"),
			"测试问卷",
		)
		versioning := Versioning{}
		versioning.InitializeVersion(q)

		if q.GetVersion().Value() != "0.0.1" {
			t.Errorf("Step 1: 初始化版本 = %v, want 0.0.1", q.GetVersion().Value())
		}

		// 2. 存草稿，版本递增到0.0.2
		versioning.IncrementMinorVersion(q)
		if q.GetVersion().Value() != "0.0.2" {
			t.Errorf("Step 2: 第一次存草稿 = %v, want 0.0.2", q.GetVersion().Value())
		}

		// 3. 再次存草稿，版本递增到0.0.3
		versioning.IncrementMinorVersion(q)
		if q.GetVersion().Value() != "0.0.3" {
			t.Errorf("Step 3: 第二次存草稿 = %v, want 0.0.3", q.GetVersion().Value())
		}

		// 4. 发布，大版本递增到1.0.1
		versioning.IncrementMajorVersion(q)
		if q.GetVersion().Value() != "1.0.1" {
			t.Errorf("Step 4: 首次发布 = %v, want 1.0.1", q.GetVersion().Value())
		}

		// 5. 编辑并存草稿，小版本递增到1.0.2
		versioning.IncrementMinorVersion(q)
		if q.GetVersion().Value() != "1.0.2" {
			t.Errorf("Step 5: 发布后编辑存草稿 = %v, want 1.0.2", q.GetVersion().Value())
		}

		// 6. 再次存草稿，版本递增到1.0.3
		versioning.IncrementMinorVersion(q)
		if q.GetVersion().Value() != "1.0.3" {
			t.Errorf("Step 6: 再次存草稿 = %v, want 1.0.3", q.GetVersion().Value())
		}

		// 7. 再次发布，大版本递增到2.0.1
		versioning.IncrementMajorVersion(q)
		if q.GetVersion().Value() != "2.0.1" {
			t.Errorf("Step 7: 再次发布 = %v, want 2.0.1", q.GetVersion().Value())
		}
	})
}
