# 受试者管理 API 接口文档

## 概述

本文档定义了受试者管理相关的所有后端 API 接口及其返回值结构，包括列表页面和详情页面。

---

## 1. GET /testees - 查询受试者列表

### 接口说明

分页查询受试者列表，支持按机构、姓名、是否重点关注等条件筛选。用于受试者列表页面展示。

### 请求参数

- **Query 参数**
  - `org_id` (integer, required): 机构ID
  - `name` (string, optional): 姓名，支持模糊匹配
  - `is_key_focus` (boolean, optional): 是否重点关注
  - `page` (integer, optional): 页码，默认1
  - `page_size` (integer, optional): 每页数量，默认20

### 响应结构

```typescript
{
  code: 0,
  message: "success",
  data: {
    items: [                        // 受试者列表
      {
        // ===== 基本信息 =====
        id: number                  // 受试者ID
        name: string                // 姓名
        gender: string              // 性别：male/female
        birthday?: string           // 出生日期，格式：YYYY-MM-DD
        org_id: number              // 机构ID
        profile_id?: number         // 用户档案ID
        iam_child_id?: number       // IAM儿童ID（已废弃，向后兼容）
        
        // ===== 扩展信息 =====
        is_key_focus: boolean       // 是否重点关注
        tags?: string[]             // 标签列表
        source?: string             // 来源
        
        // ===== 统计信息 =====
        assessment_stats?: {
          total_count: number           // 总测评次数
          completed_count: number       // 已完成次数
          pending_count: number         // 待完成次数
          last_assessment_at?: string   // 最后测评时间，格式：YYYY-MM-DD HH:mm:ss
        }
        
        // ===== 时间戳 =====
        created_at: string          // 创建时间，格式：YYYY-MM-DD HH:mm:ss
        updated_at: string          // 更新时间，格式：YYYY-MM-DD HH:mm:ss
      }
    ]
    page: number                    // 当前页码
    page_size: number               // 每页数量
    total: number                   // 总记录数
    total_pages: number             // 总页数
  }
}
```

### 示例请求

```
GET /testees?org_id=1&name=张&is_key_focus=true&page=1&page_size=20
```

### 示例响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "items": [
      {
        "id": 123,
        "name": "张小明",
        "gender": "male",
        "birthday": "2015-06-15",
        "org_id": 1,
        "profile_id": 456,
        "is_key_focus": true,
        "tags": ["注意力问题", "多动倾向"],
        "source": "入校筛查",
        "assessment_stats": {
          "total_count": 5,
          "completed_count": 5,
          "pending_count": 0,
          "last_assessment_at": "2024-12-10 14:30:00"
        },
        "created_at": "2024-01-15 10:00:00",
        "updated_at": "2024-12-10 14:30:00"
      },
      {
        "id": 124,
        "name": "张小红",
        "gender": "female",
        "birthday": "2016-03-20",
        "org_id": 1,
        "profile_id": 457,
        "is_key_focus": true,
        "tags": ["焦虑倾向"],
        "source": "主动报名",
        "assessment_stats": {
          "total_count": 3,
          "completed_count": 2,
          "pending_count": 1,
          "last_assessment_at": "2024-12-08 10:00:00"
        },
        "created_at": "2024-02-10 09:00:00",
        "updated_at": "2024-12-08 10:00:00"
      }
    ],
    "page": 1,
    "page_size": 20,
    "total": 2,
    "total_pages": 1
  }
}
```

### 使用场景

- 受试者列表页面主数据源
- 支持搜索、筛选功能
- 显示基本信息和测评统计
- 点击列表项进入详情页

---

## 2. GET /testees/{id} - 获取受试者详情

### 请求参数

- **Path 参数**
  - `id` (integer, required): 受试者ID

### 响应结构

```typescript
{
  code: 0,
  message: "success",
  data: {
    // ===== 基本信息 =====
    id: number                      // 受试者ID
    name: string                    // 姓名
    gender: string                  // 性别：male/female
    birthday?: string               // 出生日期，格式：YYYY-MM-DD
    org_id: number                  // 机构ID
    profile_id?: number             // 用户档案ID
    iam_child_id?: number           // IAM儿童ID（已废弃，向后兼容）
    
    // ===== 扩展信息 =====
    is_key_focus: boolean           // 是否重点关注
    tags?: string[]                 // 标签列表，如：["注意力问题", "焦虑倾向"]
    source?: string                 // 来源，如："入校筛查"
    
    // ===== 监护人信息（新增字段） =====
    guardians?: [                   // 监护人列表
      {
        name: string                // 监护人姓名
        relation: string            // 关系：父亲/母亲/爷爷/奶奶等
        phone: string               // 联系电话
      }
    ]
    
    // ===== 统计信息 =====
    assessment_stats?: {
      total_count: number           // 总测评次数
      completed_count: number       // 已完成次数
      pending_count: number         // 待完成次数
      last_assessment_at?: string   // 最后测评时间
    }
    
    // ===== 时间戳 =====
    created_at: string              // 创建时间，格式：YYYY-MM-DD HH:mm:ss
    updated_at: string              // 更新时间，格式：YYYY-MM-DD HH:mm:ss
  }
}
```

### 示例响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 123,
    "name": "张小明",
    "gender": "male",
    "birthday": "2015-06-15",
    "org_id": 1,
    "profile_id": 456,
    "is_key_focus": true,
    "tags": ["注意力问题", "多动倾向"],
    "source": "入校筛查",
    "guardians": [
      {
        "name": "张大明",
        "relation": "父亲",
        "phone": "13800138000"
      },
      {
        "name": "李红",
        "relation": "母亲",
        "phone": "13900139000"
      }
    ],
    "assessment_stats": {
      "total_count": 5,
      "completed_count": 5,
      "pending_count": 0,
      "last_assessment_at": "2024-12-10 14:30:00"
    },
    "created_at": "2024-01-15 10:00:00",
    "updated_at": "2024-12-10 14:30:00"
  }
}
```

---

## 3. GET /testees/{id}/scale-analysis - 获取量表趋势分析

### 接口说明

返回该受试者在各个量表上的历史测评数据，用于绘制趋势图表。前端会根据时间轴展示总分和各因子得分的变化曲线。

### 请求参数

- **Path 参数**
  - `id` (integer, required): 受试者ID

### 响应结构

```typescript
{
  code: 0,
  message: "success",
  data: {
    scales: [                       // 量表趋势列表
      {
        scale_id: number            // 量表ID
        scale_code: string          // 量表编码
        scale_name: string          // 量表名称
        tests: [                    // 测评历史记录（按时间升序排列）
          {
            assessment_id: number   // 测评ID
            test_date: string       // 测评日期，格式：YYYY-MM-DD HH:mm:ss
            total_score: number     // 总分
            risk_level: string      // 风险等级：normal/medium/high
            result?: string         // 结果描述，如："轻度焦虑"
            factors: [              // 各因子得分
              {
                factor_code: string    // 因子编码
                factor_name: string    // 因子名称
                raw_score: number      // 原始分
                t_score?: number       // T分
                percentile?: number    // 百分位
                risk_level?: string    // 风险等级：normal/medium/high
              }
            ]
          }
        ]
      }
    ]
  }
}
```

### 示例响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "scales": [
      {
        "scale_id": 1,
        "scale_code": "SAS",
        "scale_name": "焦虑自评量表",
        "tests": [
          {
            "assessment_id": 101,
            "test_date": "2024-09-01 10:00:00",
            "total_score": 45,
            "risk_level": "medium",
            "result": "轻度焦虑",
            "factors": [
              {
                "factor_code": "F1",
                "factor_name": "躯体焦虑",
                "raw_score": 20,
                "t_score": 55,
                "percentile": 70,
                "risk_level": "medium"
              },
              {
                "factor_code": "F2",
                "factor_name": "精神焦虑",
                "raw_score": 25,
                "t_score": 52,
                "percentile": 65,
                "risk_level": "normal"
              }
            ]
          },
          {
            "assessment_id": 102,
            "test_date": "2024-10-01 10:00:00",
            "total_score": 42,
            "risk_level": "normal",
            "result": "正常范围",
            "factors": [
              {
                "factor_code": "F1",
                "factor_name": "躯体焦虑",
                "raw_score": 18,
                "t_score": 52,
                "percentile": 65,
                "risk_level": "normal"
              },
              {
                "factor_code": "F2",
                "factor_name": "精神焦虑",
                "raw_score": 24,
                "t_score": 50,
                "percentile": 60,
                "risk_level": "normal"
              }
            ]
          }
        ]
      }
    ]
  }
}
```

### 使用场景

- 在受试者详情页的"量表分析"Tab中展示
- 绘制折线图：X轴为测评日期，Y轴为得分
- 支持按量表筛选、按因子筛选
- 可以看到得分的变化趋势，判断干预效果

---

## 4. GET /testees/{id}/periodic-stats - 获取周期性测评统计

### 接口说明

返回该受试者参与的周期性测评项目的完成进度。例如：某个为期8周的心理干预项目，每周需要完成一次测评，该接口返回每周的完成情况。

### 请求参数

- **Path 参数**
  - `id` (integer, required): 受试者ID

### 响应结构

```typescript
{
  code: 0,
  message: "success",
  data: {
    projects: [                     // 周期性项目列表
      {
        project_id: number          // 项目ID
        project_name: string        // 项目名称
        scale_name: string          // 关联的量表名称
        total_weeks: number         // 总周数
        completed_weeks: number     // 已完成周数
        completion_rate: number     // 完成率（0-100）
        current_week: number        // 当前应该完成的周次
        tasks: [                    // 各周任务状态（按周次升序排列）
          {
            week: number            // 第几周（从1开始）
            status: string          // 状态：completed/pending/overdue
            completed_at?: string   // 完成时间，格式：YYYY-MM-DD HH:mm:ss
            due_date?: string       // 截止时间，格式：YYYY-MM-DD
            assessment_id?: number  // 关联的测评ID（如已完成）
          }
        ]
        start_date?: string         // 项目开始日期，格式：YYYY-MM-DD
        end_date?: string           // 项目结束日期，格式：YYYY-MM-DD
      }
    ]
    total_projects: number          // 项目总数
    active_projects: number         // 进行中的项目数
  }
}
```

### 示例响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "projects": [
      {
        "project_id": 1,
        "project_name": "注意力训练项目",
        "scale_name": "注意力测评量表",
        "total_weeks": 8,
        "completed_weeks": 5,
        "completion_rate": 62.5,
        "current_week": 6,
        "tasks": [
          {
            "week": 1,
            "status": "completed",
            "completed_at": "2024-09-01 10:00:00",
            "due_date": "2024-09-07",
            "assessment_id": 101
          },
          {
            "week": 2,
            "status": "completed",
            "completed_at": "2024-09-08 10:00:00",
            "due_date": "2024-09-14",
            "assessment_id": 102
          },
          {
            "week": 3,
            "status": "completed",
            "completed_at": "2024-09-15 10:00:00",
            "due_date": "2024-09-21",
            "assessment_id": 103
          },
          {
            "week": 4,
            "status": "completed",
            "completed_at": "2024-09-22 10:00:00",
            "due_date": "2024-09-28",
            "assessment_id": 104
          },
          {
            "week": 5,
            "status": "completed",
            "completed_at": "2024-09-29 10:00:00",
            "due_date": "2024-10-05",
            "assessment_id": 105
          },
          {
            "week": 6,
            "status": "pending",
            "due_date": "2024-10-12"
          },
          {
            "week": 7,
            "status": "pending",
            "due_date": "2024-10-19"
          },
          {
            "week": 8,
            "status": "pending",
            "due_date": "2024-10-26"
          }
        ],
        "start_date": "2024-09-01",
        "end_date": "2024-10-26"
      }
    ],
    "total_projects": 1,
    "active_projects": 1
  }
}
```

### 使用场景

- 在受试者详情页的"仪表盘"Tab中展示
- 显示项目进度条、完成率
- 展示日历视图，标记已完成/待完成/逾期的周次
- 提醒即将到期的任务

---

## 5. 已有接口（参考）

### GET /evaluations/assessments?testee_id={id}

获取该受试者的所有测评记录列表。

### GET /admin/answersheets?filler_id={id}

获取该受试者填写的所有答卷记录列表。

### GET /evaluations/assessments/{assessment_id}

获取单次测评的详细信息。

### GET /admin/answersheets/{answersheet_id}

获取单份答卷的详细内容。

---

## 数据字段说明

### 性别 (gender)

- `male`: 男
- `female`: 女

### 风险等级 (risk_level)

- `normal`: 正常
- `medium`: 中等风险
- `high`: 高风险

### 任务状态 (status)

- `completed`: 已完成
- `pending`: 待完成
- `overdue`: 已逾期

### 日期时间格式

- 日期：`YYYY-MM-DD`，如 `2024-12-15`
- 日期时间：`YYYY-MM-DD HH:mm:ss`，如 `2024-12-15 14:30:00`

---

## 前端实现位置

相关代码位置：

- API 定义：`src/api/path/subject.ts`
- 类型定义：`src/api/path/subject.ts`
- Store：`src/store/subject.ts`
- 列表页面组件：`src/pages/subject/list/index.tsx`
- 详情页面组件：`src/pages/subject/detail/index.tsx`

---

## 接口实现优先级

**P0（高优先级）**

1. ✅ GET /testees - 受试者列表（已实现）
2. ✅ GET /testees/{id} - 受试者详情（需补充 guardians 字段）
3. ✅ GET /evaluations/assessments?testee_id={id} - 测评记录列表（已有）
4. ✅ GET /admin/answersheets?filler_id={id} - 答卷记录列表（已有）

**P1（中优先级）**

5. 🔄 GET /testees/{id}/scale-analysis - 量表趋势分析（新增）

**P2（低优先级）**

6. 🔄 GET /testees/{id}/periodic-stats - 周期性测评统计（新增）

---

## 备注

1. **监护人信息**：请在 GET /testees/{id} 接口中补充 `guardians` 字段
2. **列表接口**：GET /testees 已实现，支持分页和筛选
3. **量表趋势分析**：用于统计分析和数据可视化，建议实现
4. **周期性测评统计**：用于跟踪长期干预项目，可根据业务需求决定是否实现
