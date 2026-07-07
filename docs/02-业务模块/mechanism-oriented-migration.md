# 机制导向目录终局与迁移路线

## 终局原则

**代码按机制组织，数据按测评组织。**

| 机制（代码） | 测评（配置） |
|-------------|-------------|
| factor_scoring | PHQ-9、GAD-7、通用量表 |
| factor_classification / typology | MBTI、SBTI、BigFive |
| factor_norm | Brief-2、Conners（规划） |
| task_performance | SPM、工作记忆任务（规划） |

## 终局目录（目标态）

```
domain/modelcatalog/
├── factor
├── factor_norm          # 常模/综合指数 metadata
├── task_performance     # 任务表现 metadata
├── classification
└── legacy

domain/calculation/
├── scoring
├── projection
└── norm                 # 常模查表 + norm projection

application/evaluation/
├── calculationadapter
├── factor_scoring
├── factor_classification
├── factor_norm
└── task_performance
```

## 三阶段迁移

### 阶段一：过渡（当前）

- 允许 `behavioral_rating/brief2`、`cognitive/spm`、`adapter/{mbti,sbti,bigfive}` 作为 algorithm extension。
- 架构守卫测试禁止**新增**以测评 code 命名的 package。
- 所有过渡包须标注 `transitional` 注释。

### 阶段二：抽象（第二个同类模型出现时）

| 触发 | 动作 |
|------|------|
| Brief-2 + Conners | 抽 `calculation/norm`、`modelcatalog/factor_norm` |
| SPM + 第二任务 | 抽 `calculation/task`、`modelcatalog/task_performance` 执行层 |
| MBTI/SBTI/BigFive | 收敛 report/detail 到 `personality_type` / `trait_profile` 机制 |

### 阶段三：退化为配置

测评 code（brief2、mbti、sbti、spm 等）仅存在于：

- `Algorithm` 枚举
- ModelCatalog payload / seed
- 测试 fixture / migration

不再存在于主干 package 名称中。
