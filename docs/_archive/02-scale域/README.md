# 11-05 Scale 子域设计

> **版本**：V2.0  
> **最后更新**：2025-11-29  
> **状态**：✅ 已实现并验证  
> **范围**：问卷&量表 BC 中的 scale 子域  
> **目标**：阐述量表配置子域的核心职责、MedicalScale 聚合设计、领域服务拆分、与 Evaluation 子域的协作
>
> **文档说明**：本文档已根据实际代码实现进行更新，所有示例代码均与实际实现保持一致

---

## 1. Scale 子域的定位与职责

### 1.1 子域边界

**scale 子域关注的核心问题**：

* "量表是什么"：标题、描述、关联问卷
* "测量什么维度"：因子结构定义
* "如何计分"：每个因子的计分策略配置
* "如何解读"：分数区间 → 风险等级 + 结论

**scale 子域不关心的问题**：

* "如何执行计算"：这是 evaluation/calculation 的职责
* "如何执行解读"：这是 evaluation/interpretation 的职责
* "测评流程管理"：这是 evaluation/assessment 的职责
* "问卷结构展示"：这是 survey 子域的职责

### 1.2 核心定位

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                                                                             │
│   Scale = 配置层                    Evaluation = 执行层                    │
│                                                                             │
│   ┌─────────────────────┐          ┌─────────────────────────────────────┐ │
│   │  定义"规则是什么"   │ ────────▶│  执行"规则怎么用"                   │ │
│   │                     │          │                                     │ │
│   │  • 因子结构         │          │  • Calculation: 执行计算           │ │
│   │  • 计分策略配置     │          │  • Interpretation: 执行解读        │ │
│   │  • 解读规则配置     │          │  • Report: 生成报告                 │ │
│   └─────────────────────┘          └─────────────────────────────────────┘ │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 1.3 与其他子域的关系

* **被依赖** 于 evaluation 子域：提供量表配置供计算和解读使用
* **依赖** survey 子域：通过 questionnaireCode 关联问卷

**依赖方向**：

```text
 survey (问卷结构)
    ↑
  scale (量表配置) ←─────── evaluation (测评评估)
```

---

## 文档列表

### 📋 [11-05-01 Scale 子域架构总览](./11-05-01-Scale子域架构总览.md)

**核心内容**：

* Scale 子域的定位与职责边界
* 六边形架构视图
* 状态机设计（Draft → Published → Archived）
* 目录结构设计
* 核心领域概念一览

**关键要点**：

* Scale 是纯配置型领域，定义规则但不执行
* 唯一聚合根 MedicalScale 管理 Factor 和 InterpretationRule
* 通过状态机控制量表生命周期
* 领域服务按职责拆分

---

### 🎯 [11-05-02 MedicalScale 聚合与领域服务](./11-05-02-MedicalScale聚合与领域服务.md)

**核心内容**：

* MedicalScale 聚合根设计
* Factor 实体设计
* InterpretationRule 值对象设计
* Option 模式构造
* 领域服务设计
  * Lifecycle - 生命周期管理
  * BaseInfo - 基础信息更新
  * FactorManager - 因子管理
  * Validator - 发布验证器
* 类型定义（Status、FactorCode、RiskLevel、ScoreRange）

**关键要点**：

* Option 模式解决复杂对象构造问题
* 领域服务分离避免聚合根膨胀
* 值对象保证不可变性
* Validator 在发布前进行完整性校验

---

## 阅读建议

### 📖 全面学习路径

1. 从 **11-05-01** 开始，理解整体架构和职责边界
2. 阅读 **11-05-02**，深入理解聚合根和领域服务设计
3. 对照 **11-06** Evaluation 子域文档，理解 Scale 配置如何被使用

### 🎯 快速上手路径

1. **11-05-01** - 30秒速览核心设计
2. **11-05-02** - 直接看代码索引和类型关系图

### 🔍 问题导向路径

* **如何创建量表？** → 11-05-02（Option 模式构造）
* **如何添加因子？** → 11-05-02（FactorManager）
* **如何发布量表？** → 11-05-02（Lifecycle + Validator）
* **Scale 如何被 Evaluation 使用？** → 11-06-03 + 11-06-04

---

## 代码位置

```text
internal/apiserver/domain/scale/
├── types.go                     # 类型定义（Status, FactorCode, RiskLevel, ScoreRange）
├── medical_scale.go             # 聚合根
├── factor.go                    # 因子实体
├── interpretation_rule.go       # 解读规则值对象
├── lifecycle.go                 # 生命周期领域服务
├── baseinfo.go                  # 基础信息领域服务
├── factor_manager.go            # 因子管理领域服务
├── validator.go                 # 发布验证器
└── repository.go                # 仓储接口（出站端口）

internal/apiserver/application/scale/  # 应用服务
├── interface.go                 # 服务接口定义
├── dto.go                       # 数据传输对象
├── converter.go                 # DTO ↔ Domain 转换
├── lifecycle_service.go         # 生命周期服务
├── factor_service.go            # 因子服务
└── query_service.go             # 查询服务

internal/apiserver/infra/mongo/scale/  # 基础设施层
├── po.go                        # 持久化对象
├── mapper.go                    # PO ↔ Domain 映射
└── repo.go                      # MongoDB 仓储实现
```

---

## 核心流程图

### 量表配置流程

```text
1. 创建量表（Draft 状态）
   ├─ 设置基本信息（标题、描述）
   ├─ 关联问卷（questionnaireCode）
   └─ 初始化为草稿状态

2. 配置因子
   ├─ 添加因子（FactorManager.AddFactor）
   │   ├─ 设置因子基本信息
   │   ├─ 关联题目（questionCodes）
   │   └─ 配置计分策略（scoringStrategy）
   ├─ 配置解读规则（interpretRules）
   │   ├─ 分数区间 [min, max)
   │   ├─ 风险等级（RiskLevel）
   │   └─ 结论和建议文案
   └─ 确保有一个总分因子

3. 发布量表
   ├─ Validator.ValidateForPublish()
   │   ├─ 检查基本信息完整性
   │   ├─ 检查因子完整性
   │   ├─ 检查解读规则有效性
   │   └─ 检查问卷关联
   ├─ Lifecycle.Publish()
   └─ 状态：Draft → Published

4. 被 Evaluation 使用
   ├─ Calculation 读取 scoringStrategy 执行计算
   └─ Interpretation 读取 interpretRules 执行解读
```

---

## 文档特色

✅ **与代码完全一致**：所有示例代码都来自实际实现  
✅ **金字塔结构**：从宏观到微观，层层递进  
✅ **设计模式导向**：重点阐述 Option 模式、聚合模式、领域服务分离  
✅ **问题驱动**：从业务场景出发解释设计决策  
✅ **ASCII 可视化**：大量结构图、流程图、状态机图  
✅ **精简聚焦**：2篇文档覆盖核心设计，避免过度拆分  

---

## 关键设计决策

### 为什么只有一个聚合？

* **领域简单**：Scale 是配置型领域，核心就是量表配置
* **边界清晰**：MedicalScale 包含 Factor，Factor 包含 Rule，天然的聚合结构
* **一致性要求**：因子编码唯一性、总分因子唯一性都需要聚合保证

### 为什么使用 Option 模式？

* **参数多样**：创建量表时必填/可选参数组合多
* **重建需求**：从数据库恢复时需要设置 ID、Status 等
* **扩展友好**：新增可选参数无需修改构造函数签名

### 为什么拆分领域服务？

* **避免膨胀**：所有方法放聚合根会导致代码超过500行
* **职责单一**：Lifecycle、FactorManager、Validator 各司其职
* **易于测试**：领域服务可以独立单元测试

### 为什么 Scale 不执行计算？

* **职责分离**：配置与执行分离，符合单一职责原则
* **策略演进**：计算策略可能需要新增，放在 Evaluation 更灵活
* **依赖方向**：Evaluation 依赖 Scale 读取配置，反过来会循环依赖

---

> **相关文档**：
>
> * 《11-01-问卷&量表BC领域模型总览-v2.md》
> * 《11-06-Evaluation子域设计》
> * 《11-04-Survey子域设计》
