# qs-server 设计文档

> 问卷量表系统（Questionnaire & Scale）核心服务文档

## 🎯 30秒了解系统

**qs-server** 是基于 DDD 和六边形架构的问卷量表测评系统，包含：

- **qs-apiserver**: 核心 API 服务（问卷、量表、评估、用户管理）
- **qs-worker**: 后台事件处理服务（异步评估、报告生成）
- **collection-server**: 轻量级数据收集服务

## 📚 文档导航

| 目录 | 内容 | 阅读时间 |
|-----|------|---------|
| [00-概览](./00-概览/) | 系统架构、代码结构 | 5分钟 |
| [01-survey域](./01-survey域/) | 问卷子域（Questionnaire、AnswerSheet） | 10分钟 |
| [02-scale域](./02-scale域/) | 量表子域（MedicalScale、Factor） | 5分钟 |
| [03-evaluation域](./03-evaluation域/) | 评估子域（Assessment、Report） | 10分钟 |
| [04-actor域](./04-actor域/) | 用户子域（Testee、Staff） | 5分钟 |
| [05-plan域](./05-plan域/) | 测评计划子域 | 5分钟 |
| [06-screening域](./06-screening域/) | 入校筛查子域 | 5分钟 |
| [07-基础设施](./07-基础设施/) | 高并发、事件驱动架构 | 10分钟 |
| [08-运维部署](./08-运维部署/) | 端口配置、部署指南 | 2分钟 |

## 🏗️ 核心架构

```
┌─────────────────────────────────────────────────────────┐
│                    qs-server BC                         │
├─────────────────────────────────────────────────────────┤
│  survey 子域          │  scale 子域                     │
│  ├─ Questionnaire     │  ├─ MedicalScale               │
│  ├─ Question          │  ├─ Factor                     │
│  └─ AnswerSheet       │  └─ InterpretationRule         │
├─────────────────────────────────────────────────────────┤
│  evaluation 子域（桥接核心）                            │
│  ├─ Assessment ←── 统计分析锚点                        │
│  ├─ Calculation       │  └─ Report                     │
│  └─ Interpretation                                     │
├─────────────────────────────────────────────────────────┤
│  actor 子域           │  plan/screening 子域           │
│  ├─ Testee            │  ├─ AssessmentPlan             │
│  └─ Staff             │  └─ ScreeningProject           │
└─────────────────────────────────────────────────────────┘
```

## 🔥 V2 核心设计原则

1. **内容与业务分离**: AnswerSheet(Mongo) 存内容，Assessment(MySQL) 存业务
2. **Assessment 中心化**: 所有统计分析基于 Assessment 表
3. **事件驱动**: AssessmentSubmittedEvent 驱动异步评估工作流
4. **幂等创建**: EnsureTestee 支持多上游安全调用

## 📖 阅读建议

- **新成员**: 00-概览 → 01-survey域 → 03-evaluation域
- **后端开发**: 按需查阅各子域设计文档
- **运维人员**: 08-运维部署
