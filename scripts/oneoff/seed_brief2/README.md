# BRIEF-2 初始化脚本

此脚本将 BRIEF-2 家长版常模导入 canonical `behavioral_rating/brief2` 模型。默认模型编码和问卷编码均为 `gXkk9W`。

它写入：

- `assessment_norms`：`brief2-parent-cn-legacy-gXkk9W-v1`，以年龄（月）和性别分层的 T 分/百分位查表项；
- `assessment_models`：`behavioral_rating + brief2 + behavior_ability` 草稿；
- `assessment_models` 中的 `published_snapshot`：冻结的 BRIEF-2 运行快照。

脚本只需要 MongoDB，不使用 MySQL。常模已经作为版本化、gzip+base64 编码的 JSON 资产内嵌在 `data/brief2-parent-cn-legacy-gXkk9W-v1.json.gz.b64`，服务器不需要 PHP，也不需要本地附件。Base64 不是加密，不应把未经授权的常模资产提交到公开仓库。

内嵌资产解压后 JSON 的 SHA-256 为 `daaf5d9dc87d42b0db9f289066adfda35bc85355189e976994edeea46dbb3b12`，可用于部署前完整性核验。

## 题目映射

仓库已经根据已发布问卷 `gXkk9W@4.0.1` 和 BRIEF-2 家长版标准题号准备好 [data/gXkk9W_4.0.1_factor_map.json](./data/gXkk9W_4.0.1_factor_map.json)。映射文件显式绑定问卷 code/version。

标准 63 题中有 60 题进入九个临床分量表；Infrequency 的 3 个专用题不进入临床分，问卷末尾另有 7 个非 BRIEF-2 补充题。当前脚本不计算 Infrequency、Negativity 和 Inconsistency 效度指标。

## 服务器执行

从仓库根目录运行。先验证内嵌常模和映射，不连接数据库：

```bash
go run ./scripts/oneoff/seed_brief2/ \
  --questionnaire-code gXkk9W \
  --questionnaire-version 4.0.1 \
  --factor-map ./scripts/oneoff/seed_brief2/data/gXkk9W_4.0.1_factor_map.json
```

确认 dry-run 输出后连接 MongoDB 写入：

```bash
go run ./scripts/oneoff/seed_brief2/ \
  --mongo-uri "$MONGO_URL" --mongo-db "${MONGO_DB:-qs}" \
  --questionnaire-code gXkk9W \
  --questionnaire-version 4.0.1 \
  --factor-map ./scripts/oneoff/seed_brief2/data/gXkk9W_4.0.1_factor_map.json \
  --apply
```

若服务器使用 `MONGO_URI`，可以省略 `--mongo-uri`。`--norm-source` 仅用于传入同结构的规范化 JSON 覆盖文件，通常不要使用。

常模、草稿和发布快照在一个 MongoDB 多文档事务中写入，目标 MongoDB 必须是副本集或分片集群。已有同编码模型时脚本会拒绝普通写入。只有在完成数据库备份、审阅当前草稿和发布快照并确认需要整体替换后，才允许增加 `--force`。强制迁移按 model code 和问卷 code/version 识别历史快照，不依赖旧模型的 kind/algorithm，因此支持历史 `scale` 迁移到 `behavioral_rating/brief2`；无关 model code 或其他问卷版本冲突会被拒绝。常模版本不可覆盖，相同版本仅允许内容完全相同。

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
db.assessment_models.find(
  { model_code: "gXkk9W", deleted_at: null },
  { model_code: 1, model_kind: 1, model_algorithm: 1, model_product_channel: 1, questionnaire_code: 1, questionnaire_version: 1 }
)
```
