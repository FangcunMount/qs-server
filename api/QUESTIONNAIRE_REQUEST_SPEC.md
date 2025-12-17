# 问卷问题接口请求参数规范

## 概述

本文档定义了问卷问题相关接口的请求参数规范，前端需要严格按照此规范传递参数。

## 接口列表

### 1. 批量更新问题

**接口路径：** `PUT /api/v1/questionnaires/{code}/questions/batch`

**请求体结构：**

```json
{
  "questions": [
    {
      "code": "string",
      "question_type": "string",
      "stem": "string",
      "tips": "string",
      "options": [],
      "validation_rules": []
    }
  ]
}
```

## 参数详细说明

### QuestionDTO（问题对象）

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `code` | string | 是 | 问题编码，用于标识问题 |
| `question_type` | string | 是 | 问题类型，见下方类型说明 |
| `stem` | string | 是 | 问题题干 |
| `tips` | string | 否 | 问题提示/描述 |
| `placeholder` | string | 否 | 占位符（仅文本类题型使用） |
| `options` | array | 否 | 选项列表（仅选择题使用） |
| `validation_rules` | array | 否 | 校验规则列表 |
| `calculation_rule` | object | 否 | 算分规则（可选，见下方说明） |
| `show_controller` | object | 否 | 显示控制器（路由规则），用于控制问题的显示条件 |

### question_type（问题类型）

**必须使用以下值（区分大小写）：**

| 值 | 说明 | 是否需要 options |
|----|------|-----------------|
| `Radio` | 单选题 | 是，至少 2 个选项 |
| `Checkbox` | 多选题 | 是，至少 2 个选项 |
| `Text` | 单行文本 | 否 |
| `Textarea` | 多行文本 | 否 |
| `Number` | 数字 | 否 |
| `Section` | 段落（纯展示） | 否 |

**错误示例（不要使用）：**

- ❌ `single_choice` → 应使用 `Radio`
- ❌ `multi_choice` → 应使用 `Checkbox`
- ❌ `text` → 应使用 `Text`
- ❌ `textarea` → 应使用 `Textarea`

### OptionDTO（选项对象）

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `code` | string | 是 | 选项编码 |
| `content` | string | 是 | 选项内容 |
| `score` | number | 否 | 选项分数（支持小数） |

### ValidationRuleDTO（校验规则对象）

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `rule_type` | string | 是 | 规则类型，见下方规则类型说明 |
| `target_value` | string | 是 | 目标值（字符串格式） |

### rule_type（校验规则类型）

**必须使用以下值（小写，下划线分隔）：**

| 值 | 说明 | 适用题型 | target_value 格式 |
|----|------|---------|------------------|
| `required` | 必填 | 所有 | `"0"` 或 `"1"`（通常为 `"0"`） |
| `min_length` | 最小字符数 | Text, Textarea | 数字字符串，如 `"10"` |
| `max_length` | 最大字符数 | Text, Textarea | 数字字符串，如 `"100"` |
| `min_value` | 最小值 | Number | 数字字符串，如 `"0"` |
| `max_value` | 最大值 | Number | 数字字符串，如 `"100"` |
| `min_selections` | 最少选择数 | Checkbox | 数字字符串，如 `"1"` |
| `max_selections` | 最多选择数 | Checkbox | 数字字符串，如 `"3"` |
| `pattern` | 正则表达式 | Text, Textarea | 正则表达式字符串 |

**已废弃的规则类型（不要使用）：**

- ❌ `min_words` → 应使用 `min_length`
- ❌ `max_words` → 应使用 `max_length`

### ShowControllerDTO（显示控制器对象）

显示控制器用于控制问题的显示条件，基于其他问题的答案来决定当前问题是否显示。

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `rule` | string | 是 | 逻辑规则：`"and"`（所有条件满足）或 `"or"`（任一条件满足） |
| `questions` | array | 是 | 条件问题列表，见下方说明 |

### ShowControllerConditionDTO（显示控制条件对象）

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `code` | string | 是 | 条件问题编码（引用其他问题的编码） |
| `select_option_codes` | array | 是 | 选中的选项编码列表。对于单选题，数组长度为 1；对于多选题，数组长度 >= 1 |

### CalculationRuleDTO（算分规则对象）

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `formula_type` | string | 是 | 公式类型，可选值：`score`（选项分值）、`sum`（求和）、`avg`（平均值）、`max`（最大值）、`min`（最小值） |

**注意：** 算分规则主要用于量表类问卷，普通调查问卷通常不需要配置此字段。

**显示控制器逻辑说明：**

- `rule: "and"`：所有条件问题都必须满足其选项条件，当前问题才显示
- `rule: "or"`：任一条件问题满足其选项条件，当前问题就显示
- `select_option_codes`：当条件问题的答案包含这些选项编码时，条件满足

## 完整请求示例

### 示例 1：单选题

```json
{
  "questions": [
    {
      "code": "nkd3zfBF",
      "question_type": "Radio",
      "stem": "食欲下降",
      "tips": "",
      "options": [
        {
          "code": "7c5Vb4AS",
          "content": "正常",
          "score": 0
        },
        {
          "code": "k2lyHvK8",
          "content": "轻",
          "score": 1
        },
        {
          "code": "RaVCO2xx",
          "content": "中",
          "score": 2
        }
      ],
      "validation_rules": [
        {
          "rule_type": "required",
          "target_value": "0"
        }
      ]
    }
  ]
}
```

### 示例 2：多行文本题

```json
{
  "questions": [
    {
      "code": "qiLUbiCk",
      "question_type": "Textarea",
      "stem": "其他疑似药物引起的不良反应",
      "tips": "",
      "validation_rules": [
        {
          "rule_type": "required",
          "target_value": "0"
        },
        {
          "rule_type": "min_length",
          "target_value": "0"
        },
        {
          "rule_type": "max_length",
          "target_value": "3000"
        }
      ]
    }
  ]
}
```

### 示例 3：数字题

```json
{
  "questions": [
    {
      "code": "num001",
      "question_type": "Number",
      "stem": "请输入年龄",
      "tips": "请输入 0-150 之间的数字",
      "validation_rules": [
        {
          "rule_type": "required",
          "target_value": "0"
        },
        {
          "rule_type": "min_value",
          "target_value": "0"
        },
        {
          "rule_type": "max_value",
          "target_value": "150"
        }
      ]
    }
  ]
}
```

### 示例 4：带路由规则的问题（条件显示）

```json
{
  "questions": [
    {
      "code": "nkd3zfBF",
      "question_type": "Radio",
      "stem": "食欲下降",
      "tips": "",
      "options": [
        {
          "code": "7c5Vb4AS",
          "content": "正常",
          "score": 0
        },
        {
          "code": "k2lyHvK8",
          "content": "轻",
          "score": 1
        }
      ],
      "validation_rules": [
        {
          "rule_type": "required",
          "target_value": "0"
        }
      ]
    },
    {
      "code": "uer3kD9R",
      "question_type": "Radio",
      "stem": "便秘",
      "tips": "",
      "options": [
        {
          "code": "GRLqhYtD",
          "content": "正常",
          "score": 0
        },
        {
          "code": "TM3C7JVh",
          "content": "轻",
          "score": 1
        }
      ],
      "show_controller": {
        "rule": "and",
        "questions": [
          {
            "code": "nkd3zfBF",
            "select_option_codes": ["7c5Vb4AS"]
          }
        ]
      },
      "validation_rules": [
        {
          "rule_type": "required",
          "target_value": "0"
        }
      ]
    }
  ]
}
```

**说明：** 上面的示例中，`uer3kD9R`（便秘）问题只有在 `nkd3zfBF`（食欲下降）问题选择了 `"7c5Vb4AS"`（正常）选项时才会显示。

### 示例 5：多条件路由规则（OR 逻辑）

```json
{
  "questions": [
    {
      "code": "question_with_or_rule",
      "question_type": "Textarea",
      "stem": "请详细说明",
      "tips": "",
      "show_controller": {
        "rule": "or",
        "questions": [
          {
            "code": "q1",
            "select_option_codes": ["opt1"]
          },
          {
            "code": "q2",
            "select_option_codes": ["opt2", "opt3"]
          }
        ]
      },
      "validation_rules": [
        {
          "rule_type": "required",
          "target_value": "0"
        }
      ]
    }
  ]
}
```

**说明：** 当问题 `q1` 选择了 `opt1`，**或者**问题 `q2` 选择了 `opt2` 或 `opt3` 时，当前问题才会显示。

## 常见错误

### 错误 1：问题类型使用小写或下划线格式

```json
// ❌ 错误
{
  "question_type": "single_choice"
}

// ✅ 正确
{
  "question_type": "Radio"
}
```

### 错误 2：校验规则类型使用废弃格式

```json
// ❌ 错误
{
  "rule_type": "min_words",
  "target_value": "10"
}

// ✅ 正确
{
  "rule_type": "min_length",
  "target_value": "10"
}
```

### 错误 3：选择题缺少选项

```json
// ❌ 错误 - Radio 类型必须提供至少 2 个选项
{
  "question_type": "Radio",
  "options": []
}

// ✅ 正确
{
  "question_type": "Radio",
  "options": [
    { "code": "opt1", "content": "选项1", "score": 0 },
    { "code": "opt2", "content": "选项2", "score": 1 }
  ]
}
```

### 错误 4：路由规则格式错误

```json
// ❌ 错误 - rule 必须是 "and" 或 "or"
{
  "show_controller": {
    "rule": "AND",  // 大小写错误
    "questions": [...]
  }
}

// ❌ 错误 - select_option_codes 不能为空数组
{
  "show_controller": {
    "rule": "and",
    "questions": [
      {
        "code": "q1",
        "select_option_codes": []  // 空数组
      }
    ]
  }
}

// ✅ 正确
{
  "show_controller": {
    "rule": "and",
    "questions": [
      {
        "code": "q1",
        "select_option_codes": ["opt1"]
      }
    ]
  }
}
```

## 注意事项

1. **问题类型必须使用首字母大写的格式**（Radio, Checkbox, Text, Textarea, Number, Section）
2. **校验规则类型使用小写下划线格式**（required, min_length, max_length 等）
3. **target_value 始终为字符串格式**，即使是数字也要用字符串表示
4. **选择题（Radio/Checkbox）必须提供至少 2 个选项**
5. **文本类题型（Text/Textarea）不需要 options 字段**
6. **required 规则的 target_value 通常为 `"0"`**（表示必填）
7. **路由规则（show_controller）说明：**
   - `rule` 字段必须是小写的 `"and"` 或 `"or"`，区分大小写
   - `questions` 数组不能为空，至少包含一个条件
   - `select_option_codes` 数组不能为空，至少包含一个选项编码
   - 条件问题编码（`code`）必须引用问卷中已存在的其他问题
   - 对于单选题，`select_option_codes` 通常只包含一个选项编码
   - 对于多选题，`select_option_codes` 可以包含多个选项编码（表示选择了这些选项中的任意一个或多个）
   - 路由规则是可选字段，不提供时问题默认显示

## 相关接口

- 添加问题：`POST /api/v1/questionnaires/{code}/questions`
- 更新问题：`PUT /api/v1/questionnaires/{code}/questions/{questionCode}`
- 重排问题：`POST /api/v1/questionnaires/{code}/questions/reorder`

以上接口的参数格式与批量更新接口保持一致。
