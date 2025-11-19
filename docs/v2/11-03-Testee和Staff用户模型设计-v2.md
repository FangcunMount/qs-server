# 11-03 Testee 和 Staff 用户模型设计（V2）

> 版本：V2.0
> 范围：问卷&量表 BC 内部 user 子域
> 目标：澄清 Testee（受试者）与 Staff（后台人员）在本 BC 内的职责、结构与与 IAM 的关系。

---

## 1. 设计动机

在 V1 中，用户相关概念混乱：

* IAM 中有 User / Account / Child；
* 问卷系统内部又有 writer / viewer / 受试者 / 管理员 等术语。

问题：

* “谁是被测人、谁在填表、谁负责管理项目”混在一起；
* 很难在领域层表达“以谁为主档做统计”。

V2 目标：

1. 在问卷&量表 BC 内收敛成两个核心用户模型：

   * **Testee**：受试者（被测评对象）
   * **Staff**：后台工作人员（量表后台用户）
2. 通过 ID 与 IAM 集成，而不是重造认证/账号体系。

---

## 2. Testee（受试者）

### 2.1 定义

> Testee 表示“被测评的人”在问卷&量表 BC 内的视图，是统计和趋势分析的核心主体。

典型场景：

* 互联网医院：患者本人或儿童患者；
* 行为训练中心：学员；
* 入校筛查：学生。

### 2.2 字段设计

```go
type TesteeID string

type Testee struct {
    id         TesteeID
    orgID      int64          // 所属机构（医院、训练中心、学校）
    iamUserID  *int64         // 可选：绑定 IAM.User
    iamChildID *int64         // 可选：绑定 IAM.Child
    name       string
    gender     int8
    birthday   *time.Time
    grade      *string        // 年级信息（筛查场景）
    createdAt  time.Time
    updatedAt  time.Time
}
```

说明：

* `TesteeID`：本 BC 内部主键；
* `orgID`：多机构场景中区分机构；
* `iamUserID` / `iamChildID`：与 IAM 的关联，可空。

### 2.3 与 Assessment 的关系

* `Assessment.testeeID` 引用 `TesteeID`；
* 与 Testee 相关的典型查询：

  * 查看某 Testee 的全部测评历史；
  * 查看某个量表在该 Testee 身上的多次测评折线图；
  * 对高风险 Testee 做持续追踪。

---

## 3. Staff（后台工作人员）

### 3.1 定义

> Staff 表示在问卷&量表系统中执行配置、管理、审核等操作的内部用户。

典型角色：

* ScaleAdmin：量表管理员；
* Evaluator：评估人员；
* ScreeningOwner：筛查项目负责人。

### 3.2 字段设计

```go
type StaffID string

type StaffRole string

const (
    StaffRoleScaleAdmin     StaffRole = "scale_admin"
    StaffRoleEvaluator      StaffRole = "evaluator"
    StaffRoleScreeningOwner StaffRole = "screening_owner"
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

## 8. 总结

V2 中 user 子域的设计要点：

1. 将“人”在本 BC 中收敛为两类：

   * Testee：被测评主体；
   * Staff：系统中的工作人员。
2. 与 IAM 保持松耦合，通过 ID 映射即可；
3. 清晰定义 Testee 的创建时机，为计划、筛查与趋势分析打好基础。

所有与“谁是被测对象”“谁在操作系统”相关的逻辑，都应围绕 Testee / Staff 展开，而不是直接操作 IAM.User/Child。
