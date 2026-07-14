# BRIEF-2 初始化脚本

此脚本将既有 BRIEF-2 家长版常模导入 canonical `behavioral_rating/brief2` 模型。默认模型编码和问卷编码均为 `gXkk9W`。

它写入：

- `assessment_norms`：`brief2-parent-cn-legacy-gXkk9W-v1`，以年龄（月）和性别分层的 T 分/百分位查表项；
- `assessment_models`：已发布的 `behavioral_rating + brief2 + behavior_ability` 草稿；
- `published_assessment_models`：冻结的 BRIEF-2 运行快照。

## 题目映射

仓库已经根据已发布问卷 `gXkk9W@4.0.1` 和 BRIEF-2 家长版标准题号准备好 [data/gXkk9W_4.0.1_factor_map.json](./data/gXkk9W_4.0.1_factor_map.json)。映射文件显式绑定问卷 code/version，避免将题目编码误用于其他版本。

键必须保持以下历史因子编码，值为当前已发布问卷 `gXkk9W` 中对应的题目编码数组：

| 因子 | 中文名 |
| --- | --- |
| `p3O50jXO` | 抑制 |
| `Aa7IbYHN` | 自我监控 |
| `nJkTU8bM` | 情景转换 |
| `AyvItzpm` | 情绪控制 |
| `Tox3nsdt` | 任务启动 |
| `CI01dlwX` | 工作记忆 |
| `N279wV33` | 计划/组织 |
| `WJI5vCPX` | 任务监控 |
| `C5T60lQa` | 材料组织 |

标准63题中有60题进入九个临床分量表；Infrequency 的3个专用题不进入临床分，问卷末尾另有7个非 BRIEF-2 补充题。映射文件通过 `excluded_question_codes` 明确记录这10题。执行时脚本会校验：九个临床量表都存在、每一道可作答题恰好被计分或显式排除、所有题目和选项编码均来自已发布问卷。映射不完整、重复或版本不匹配时不会写入模型。

当前脚本不计算 Infrequency、Negativity 和 Inconsistency 效度指标。尤其 Inconsistency 需要题目对差值策略，不能用普通求和冒充。

## 执行

先只校验输入：

```bash
go run ./scripts/oneoff/seed_brief2/ \
  --questionnaire-code gXkk9W --questionnaire-version 4.0.1 \
  --norm-source /path/to/BRIEF2_Norms.php \
  --factor-map ./scripts/oneoff/seed_brief2/data/gXkk9W_4.0.1_factor_map.json
```

再写入（脚本会读取 `MONGO_URI`、`MONGO_DB`，也可显式传参数）：

```bash
go run ./scripts/oneoff/seed_brief2/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs \
  --questionnaire-code gXkk9W --questionnaire-version 4.0.1 \
  --norm-source /path/to/BRIEF2_Norms.php \
  --factor-map ./scripts/oneoff/seed_brief2/data/gXkk9W_4.0.1_factor_map.json \
  --apply
```

若同编码模型已存在，先审阅已发布快照；确认需要整体替换时才追加 `--force`。常模表版本不可覆盖：相同版本仅允许内容完全相同。

当前 qs-server 尚未注册 `/norm-tables` 写入路由，因此 operating API token 只能用于只读核验问卷，不能替代 Mongo 完成常模导入。本脚本的 `--apply` 仍需要 `MONGO_URI`。

`BRIEF2_Texts.php` 中的维度解释、通常表现和家庭建议可作为后续报告资产迁移来源，但当前 `DefinitionV2.ReportMap` 不承载该富文本结构，本脚本不会静默写入运行时无法消费的数据。

## 验收

```javascript
db.assessment_norms.find(
  { table_version: "brief2-parent-cn-legacy-gXkk9W-v1", deleted_at: null },
  { table_version: 1, kind: 1, algorithm: 1, form_variant: 1, factors: 1 }
)
db.assessment_models.find(
  { code: "gXkk9W", deleted_at: null },
  { code: 1, kind: 1, algorithm: 1, product_channel: 1, questionnaire_code: 1, questionnaire_version: 1, status: 1 }
)
db.published_assessment_models.find(
  { model_code: "gXkk9W", deleted_at: null },
  { model_code: 1, model_kind: 1, model_algorithm: 1, model_product_channel: 1, questionnaire_code: 1, questionnaire_version: 1 }
)
```
