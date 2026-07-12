# BRIEF-2 初始化脚本

此脚本将既有 BRIEF-2 家长版常模导入 canonical `behavioral_rating/brief2` 模型。默认模型编码和问卷编码均为 `gXkk9W`。

它写入：

- `assessment_norms`：`brief2-parent-cn-legacy-gXkk9W-v1`，以年龄（月）和性别分层的 T 分/百分位查表项；
- `assessment_models`：已发布的 `behavioral_rating + brief2 + behavior_ability` 草稿；
- `published_assessment_models`：冻结的 BRIEF-2 运行快照。

## 先准备题目映射

两份输入 PHP 文件只含常模、因子编码和报告文案，不含 63 道题目到 9 个临床分量表的归属。因此必须从历史 `assessment_mode.code=gXkk9W` 导出该归属，填入一份 JSON 映射文件。可从 [factor_map.example.json](./factor_map.example.json) 复制。

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

执行时脚本会校验：九个临床量表都存在、每一道可作答题只归属一个量表、所有题目和选项编码均来自已发布问卷。映射不完整或重复时不会写入模型。

## 执行

先只校验输入：

```bash
go run ./scripts/oneoff/seed_brief2/ \
  --norm-source /path/to/BRIEF2_Norms.php \
  --factor-map /path/to/brief2_factor_map.json
```

再写入（脚本会读取 `MONGO_URI`、`MONGO_DB`，也可显式传参数）：

```bash
go run ./scripts/oneoff/seed_brief2/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs \
  --norm-source /path/to/BRIEF2_Norms.php \
  --factor-map /path/to/brief2_factor_map.json \
  --apply
```

若同编码模型已存在，先审阅已发布快照；确认需要整体替换时才追加 `--force`。常模表版本不可覆盖：相同版本仅允许内容完全相同。

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
