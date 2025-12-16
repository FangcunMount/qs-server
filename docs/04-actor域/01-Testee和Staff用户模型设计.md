# 11-03 Testee 和 Staff 用户模型设计（V2）

> **版本**：V2.0  
> **范围**：问卷&量表 BC 内部 user 子域  
> **目标**：澄清 Testee（受试者）与 Staff（后台人员）在本 BC 内的职责、结构及与 IAM 的关系

---

## 1. 设计动机

### 1.1 V1 存在的问题

在 V1 版本中，用户相关概念混杂：

* 既有 IAM 中的 `User` / `Account` / `Child`
* 又有问卷系统内部提到的 "writer / viewer / 受试者 / 管理员" 等概念
* 还混用了 "填写人" 与 "被测人" 的概念

**结果**：

* "谁是被测人、谁在填表、谁负责管理项目" 边界不清
* 很难在领域层清晰表达 "以谁为主体进行统计"
* 权限和数据归属逻辑混乱

### 1.2 V2 的核心改进

**在问卷&量表 BC 内部收敛为两个核心用户模型**：

1. **Testee（受试者）**：被测评的对象
   * 是测评记录的核心主体
   * 是趋势分析、历史查询的归档维度

2. **Staff（后台工作人员）**：量表系统内部员工
   * 配置量表、管理项目、审核报告
   * 与具体测评行为无关，只负责系统管理

**关键原则**：

* 通过 ID 映射与 IAM 集成，而非在本 BC 重造认证/账号体系
* 区分 "谁被测（Testee）" 与 "谁在填（Writer）"
* 明确 Testee 是长期存在的业务实体，Writer 只是操作上下文

---

## 2. Testee（受试者）模型

### 2.1 定义

> **Testee** 表示 "被测评的人" 在问卷&量表 BC 内的领域视图，是统计和趋势分析的核心主体。

**典型场景**：

* **互联网医院业务**：患者本人 / 儿童患者
* **行为训练中心业务**：学员（儿童）
* **入校筛查业务**：学生

**核心职责**：

* 作为 `Assessment` 的 "人" 这一侧的绑定实体
* 支持查询某个 Testee 的测评历史、趋势统计
* 为长期追踪、风险预警提供稳定的主体标识

### 2.2 字段设计（领域层视角）

```go
package user

import "time"

type TesteeID string

type Testee struct {
    // === 核心标识 ===
    id         TesteeID
    orgID      int64          // 所属机构（医院、训练中心、学校等）
    
    // === 与 IAM 的映射 ===
    iamUserID  *int64         // 可选：绑定 IAM.User（成人患者）
    iamChildID *int64         // 可选：绑定 IAM.Child（儿童档案）
    
    // === 基本属性 ===
    name       string         // 姓名（可脱敏）
    gender     int8           // 0=未知, 1=男, 2=女
    birthday   *time.Time     // 出生日期
    
    // === 业务归属与标签 ===
    grade      *string        // 年级信息（筛查场景）
    className  *string        // 班级信息
    schoolName *string        // 学校名称（筛查场景）
    tags       []string       // 业务标签：["high_risk", "adhd_suspect", "vip"]
    source     string         // 数据来源：online_form / clinic_import / screening_campaign
    isKeyFocus bool           // 是否重点关注对象
    
    // === 统计快照（可选，减轻查询压力）===
    lastAssessmentAt *time.Time // 最近一次测评完成时间
    totalAssessments int        // 总测评次数
    lastRiskLevel    *string    // 最近一次测评的风险等级
    
    // === 审计字段 ===
    createdAt  time.Time
    createdBy  int64           // 操作员工的 IAM UserID
    updatedAt  time.Time
    updatedBy  int64
    deletedAt  *time.Time      // 软删除标记
    version    int64           // 乐观锁版本号
}

// === 构造函数 ===
func NewTestee(
    orgID int64,
    name string,
    gender int8,
    birthday *time.Time,
) *Testee {
    now := time.Now()
    return &Testee{
        id:        TesteeID(generateID()),
        orgID:     orgID,
        name:      name,
        gender:    gender,
        birthday:  birthday,
        source:    "unknown",
        createdAt: now,
        updatedAt: now,
    }
}

// === 核心行为 ===
func (t *Testee) ID() TesteeID { return t.id }

func (t *Testee) BindIAMUser(userID int64) {
    t.iamUserID = &userID
}

func (t *Testee) BindIAMChild(childID int64) {
    t.iamChildID = &childID
}

func (t *Testee) MarkAsKeyFocus() {
    t.isKeyFocus = true
}

func (t *Testee) AddTag(tag string) {
    // 防重复
    for _, existing := range t.tags {
        if existing == tag {
            return
        }
    }
    t.tags = append(t.tags, tag)
}
```

**字段说明**：

* `TesteeID`：本 BC 内部的受试者主键，不直接使用 IAM 的 UserID/ChildID
* `orgID`：多机构托管时，用于区分不同医院/门店/学校
* `iamUserID` / `iamChildID`：
  * 对接 IAM 系统时，用于做账号/档案的映射
  * 在部分场景（纯线下、仅手机号）可以为空
* `tags`：业务标签，用于快速筛查和报表统计
* `source`：数据来源标识，对运营分析和数据质量追踪重要
* 统计快照字段：通过异步任务/事件更新，避免每次查询实时计算

### 2.3 与 Assessment 的关系

* `Assessment.testeeID` 字段引用 `TesteeID`
* 所有与 "某个被测人" 的历史记录、趋势统计，都通过 `TesteeID` 聚合

**典型查询**：

1. **查看某个 Testee 的测评历史列表**

   ```sql
   SELECT * FROM assessments WHERE testee_id = ? ORDER BY created_at DESC
   ```

2. **查看某个量表在该 Testee 身上的多次测评折线图**

   ```sql
   SELECT total_score, risk_level, interpreted_at 
   FROM assessments 
   WHERE testee_id = ? AND medical_scale_id = ?
   ORDER BY interpreted_at
   ```

3. **查看某个 Testee 在某段时间内不同量表的风险变化对比**

### 2.4 Testee 与 Writer 的区分

**核心原则**：Testee（谁被测） ≠ Writer（谁在填）

**场景示例**：

| 场景 | Testee | Writer | 说明 |
|------|--------|--------|------|
| 成人自己填 | 患者本人 | 患者本人 | Testee 和 Writer 是同一人 |
| 家长代填 | 儿童 | 家长 | Testee 是孩子，Writer 是家长 |
| 医生代填 | 患者 | 医生 | Testee 是患者，Writer 是医生（Staff） |
| 筛查扫码 | 学生 | 家长 | Testee 是学生，Writer 是扫码的家长 |

**领域建模中的体现**：

* `AnswerSheet` 可以记录 `WriterInfo`（谁填的、关系、填写方式）
* `Assessment` 绑定 `TesteeID`（统计主体）
* `InterpretReport` 面向 Testee，而非 Writer

```go
// AnswerSheet 中记录填写人信息（可选）
type WriterType string

const (
    WriterTypeSelf     WriterType = "self"      // 本人填写
    WriterTypeGuardian WriterType = "guardian"  // 监护人代填
    WriterTypeStaff    WriterType = "staff"     // 员工代填
)

type WriterInfo struct {
    WriterType WriterType
    IAMUserID  *int64     // 填写人的 IAM UserID
    IP         string     // 填写 IP
    UserAgent  string     // 填写设备
    // 可扩展：RelationshipToTestee, StaffID 等
}

type AnswerSheet struct {
    // ... 其他字段
    writerInfo *WriterInfo // 可选：记录填写上下文
}
```

**权限判断示例**：

* **用户查看自己的答卷**：`sheet.Testee.UserID == 当前登录 UserID`
* **家长查看孩子的答卷**：`isGuardianOf(当前用户, sheet.Testee.TesteeID)`
* **医生查看患者答卷**：通过 IAM/AuthZ 做资源级权限判断

---

### 3. Staff（后台工作人员）

### 3.1 定义

> Staff 表示在问卷&量表系统中执行配置、管理、审核等操作的后台人员。

典型角色（迁移到统一权限中心标识）：

- `qs:admin`：QS 管理员
- `qs:content_manager`：内容管理员（问卷/量表管理）
- `qs:evaluator`：评估员

### 3.2 字段设计

```go
type StaffID string

type StaffRole string

const (
    StaffRoleQSAdmin        StaffRole = "qs:admin"
    StaffRoleContentManager StaffRole = "qs:content_manager"
    StaffRoleEvaluator      StaffRole = "qs:evaluator"
)

type Staff struct {
    id        StaffID
    orgID     int64
    iamUserID int64
    roles     []StaffRole
}
```

说明：

* Staff 是 IAM.User 在本 BC 内的投影；
* 具体权限依然由 IAM/AuthZ 负责，本 BC 不重建 RBAC。

---

## 4. 与 IAM BC 的关系

### 4.1 职责边界

* **IAM BC**：

  * 负责用户注册、登录、账号管理；
  * 提供 User / Child / Org 等全局身份；
  * 实现统一认证与授权。

* **问卷&量表 BC**：

  * 只关心业务视角下的人：

    * Testee：被测人；
    * Staff：工作人员。
  * 通过 ID 字段与 IAM 做松耦合关联。

### 4.2 映射方式

* 从 IAM 到本 BC：

  * 创建 Testee / Staff 时绑定 `iamUserID` / `iamChildID`；
* 从本 BC 到 IAM：

  * 当需要更多账号信息时，由应用层调用 IAM API 查询。

---

## 5. Testee 的创建时机

### 5.1 一次性测评（门诊/门店扫码）

* 若已有 IAM.User/Child：

  * 可基于 IAM ID 查找/创建 Testee；
* 若没有 IAM 档案：

  * 可创建仅在本 BC 存在的 Testee（可记录手机号等）；
  * 后续如需要可通过绑定 IAM ID 补链。

### 5.2 周期性测评（测评计划）

* 在“生成测评计划”的业务节点统一创建 Testee：

  * 输入：IAM.User/Child + Org 信息；
  * 输出：TesteeID；
* AssessmentPlan 绑定 TesteeID；
* 计划下所有 Assessment 共用同一 Testee，便于趋势分析。

### 5.3 入校筛查项目

* 可选模式：

  1. 临时 Testee：基于班级/姓名创建，只用于项目内分析；
  2. 标准 Testee：预导入学生主档，显式创建 Testee 并与学生档案打通。

---

## 6. Staff 的使用

使用场景：

1. 量表后台配置：管理 MedicalScale / Questionnaire 模板；
2. 筛查项目管理：创建 ScreeningProject、配置时间窗口、查看统计；
3. 解读结果审核：对高风险案例做人工标注或备注。

实现方式：

* Handler 层从认证上下文拿到当前 IAM.User；
* 应用层查 StaffRepository 获取 Staff 信息；
* 领域层用 StaffID / OrgID 决定能不能执行某些操作。

---

## 7. 仓储与领域服务接口

仓储接口示例：

```go
type TesteeRepository interface {
    FindByID(ctx context.Context, id TesteeID) (*Testee, error)
    FindByIAMChild(ctx context.Context, orgID int64, iamChildID int64) (*Testee, error)
    Save(ctx context.Context, t *Testee) error
}

type StaffRepository interface {
    FindByID(ctx context.Context, id StaffID) (*Staff, error)
    FindByIAMUser(ctx context.Context, orgID int64, iamUserID int64) (*Staff, error)
    Save(ctx context.Context, s *Staff) error
}
```

领域服务示例：

```go
type TesteeFactory interface {
    GetOrCreateByIAMChild(
        ctx context.Context,
        orgID int64,
        iamChildID int64,
        name string,
        gender int8,
        birthday *time.Time,
    ) (*Testee, error)
}
```

---

## 8. FillerRef：填写人的行为角色

### 8.1 设计动机

在答卷场景中，"谁被测"（Testee）和"谁在填写"（Filler）可能不是同一个人：

* 儿童测评：家长/老师代填
* 认知障碍：护理人员代填
* 自测场景：受试者本人填写

因此需要在 AnswerSheet 上明确记录填写动作的执行者。

### 8.2 FillerRef 设计

```go
// 填写动作的角色类型
type FillerType string

const (
    FillerTypeSelf     FillerType = "self"      // 受试者本人填写
    FillerTypeGuardian FillerType = "guardian"  // 监护人/家长/老师代填
    FillerTypeStaff    FillerType = "staff"     // 内部员工代填
)

// 填写人引用（值对象）
type FillerRef struct {
    UserID     int64      // IAM.UserID
    FillerType FillerType
}
```

### 8.3 在 AnswerSheet 中的使用

```go
type AnswerSheet struct {
    ID                 AnswerSheetID
    QuestionnaireCode  Code

    Testee             TesteeRef  // 被测者
    FilledBy           FillerRef  // 谁操作填写

    // ... 其他字段
}
```

**关键点：**

* FillerRef 是值对象，不是领域实体
* 系统中只有 Testee + Staff 两种"人"的实体
* FilledBy 只是记录"这次填写动作"的执行者元数据

### 8.4 访问控制策略

**谁可以查看某份答卷？** 通过策略函数判断：

```go
func CanViewAnswerSheet(userID int64, sheet *AnswerSheet) bool {
    // 1. 受试者本人
    if sheet.Testee.IamUserID != nil && *sheet.Testee.IamUserID == userID {
        return true
    }

    // 2. 填写人（家长/老师）
    if sheet.FilledBy.UserID == userID {
        return true
    }

    // 3. 监护关系（通过 IAM 查询）
    if isGuardianOf(userID, sheet.Testee.TesteeID) {
        return true
    }

    // 4. 有权限的 Staff
    if hasPermission(userID, "qs.answersheet.read") {
        return true
    }

    return false
}
```

**说明：**

* "查看人（Viewer）"不是实体，是访问控制的概念
* 通过 Testee、FilledBy、Guardian、Staff 关系计算权限

---

## 9. 与 IAM BC 的用户关系图

```text
IAM BC (统一身份认证)           问卷&量表 BC (业务领域视图)
┌──────────────────┐            ┌──────────────────────┐
│ User             │ ◄──────────┤ Testee               │
│ - UserID         │  ID 映射   │ - TesteeID           │
│ - Phone          │            │ - IamUserID (FK)     │
│ - Password       │            │ - Grade, Tags        │
└──────────────────┘            └──────────────────────┘
        ↓                                  ↓
┌──────────────────┐            ┌──────────────────────┐
│ Child            │ ◄──────────┤ Testee               │
│ - ChildID        │  ID 映射   │ - TesteeID           │
│ - ParentUserID   │            │ - IamChildID (FK)    │
│ - Birthdate      │            │ - Grade, Tags        │
└──────────────────┘            └──────────────────────┘

┌──────────────────┐            ┌──────────────────────┐
│ User (员工)      │ ◄──────────┤ Staff                │
│ - UserID         │  ID 映射   │ - StaffID            │
│ - OrgID          │            │ - IamUserID (FK)     │
│ - Roles (IAM)    │            │ - Roles (业务)       │
└──────────────────┘            └──────────────────────┘
```

**关键设计点：**

* IAM BC：负责全局身份、认证、授权
* 问卷&量表 BC：维护业务相关的人的视图
* 通过 ID 映射松耦合，而非聚合引用

---

## 10. Principal（当前请求用户）的处理

### 10.1 Principal 不是领域对象

从 token 解析的当前用户（Principal）是技术对象，不属于领域层：

```go
type Principal struct {
    UserID      int64    // IAM.UserID
    TenantID    int64
    Roles       []string // IAM 角色
    Permissions []string // 权限 claims
}

// 中间件从 token 解析 Principal
func AuthnMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := extractToken(r)
        p, err := ParseAndVerifyToken(token)
        if err != nil {
            // 401
            return
        }
        ctx := context.WithValue(r.Context(), ctxKeyPrincipal, p)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### 10.2 应用层使用 Principal

```go
func (s *AnswerAppService) ListMyAnswerSheets(ctx context.Context) ([]AnswerDTO, error) {
    p := CurrentPrincipal(ctx)
    userID := p.UserID  // 拿到 IAM.UserID

    // 根据 UserID 查询该用户关联的 Testee 的答卷
    sheets, err := s.answerRepo.ListByTesteeUserID(ctx, userID)
    return sheets, err
}
```

**说明：**

* Principal 只在应用层/接口层使用
* 需要业务语义时，将 UserID 映射为 Testee 或 Staff
* 领域层不直接依赖 Principal

---

## 11. 总结

### 11.1 核心设计要点

V2 user 子域的设计要点：

1. **领域实体收敛**：将人在本 BC 中收敛为两类
   * **Testee**：被测评主体，长期存在的业务实体
   * **Staff**：系统工作人员，后台管理角色

2. **行为角色设计**：FillerRef 记录填写动作的执行者
   * 不是用户类型，是值对象（VO）
   * 区分"谁被测（Testee）"和"谁在填（Filler）"

3. **与 IAM 松耦合**：通过 ID 映射关联
   * IAM BC 负责认证授权
   * 问卷&量表 BC 维护业务视图
   * 应用层负责两者之间的映射

4. **创建策略**：TesteeFactory 模式
   * 在第一次参与测评业务时创建
   * GetOrCreateByIAMChild 保证幂等性
   * 通过唯一索引避免重复创建

5. **访问控制**：通过策略函数计算权限
   * 不建立 Viewer 实体
   * 基于 Testee、FilledBy、Guardian、Staff 关系判断
   * 粗粒度权限由 IAM/AuthZ，细粒度由领域规则

### 11.2 实施建议

1. **先建立基础设施**
   * TesteeRepository / StaffRepository
   * TesteeFactory 领域服务
   * 与 IAM 的集成 API 封装

2. **明确创建时机**
   * 一次性测评：可临时创建或绑定已有 Testee
   * 测评计划：在计划创建时统一创建 Testee
   * 入校筛查：可选择临时或标准模式

3. **权限控制分层**
   * 认证：IAM BC 统一处理
   * 粗粒度授权：IAM/AuthZ（"能否访问量表模块"）
   * 细粒度授权：领域规则（"能否查看这份具体答卷"）

4. **统计维度统一**
   * 所有测评相关统计以 Testee 为主维度
   * 避免直接依赖 IAM.User 做业务统计
   * Testee 提供稳定的历史查询锚点

5. **扩展性设计**
   * Testee.tags 支持业务标签体系
   * FillerRef 可扩展关系类型
   * Staff.roles 可灵活定义业务角色

---

本文档定义了问卷&量表 BC 内 user 子域的核心模型，为后续测评计划、入校筛查、历史趋势分析等功能提供了清晰的"人"的视角。所有涉及"谁被测""谁在操作"的逻辑，都应围绕 **Testee** / **Staff** / **FillerRef** 这套模型展开，而非直接使用 IAM.User/Child 等上游身份概念。
