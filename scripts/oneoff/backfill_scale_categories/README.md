# 医学量表分类回填

此脚本修复历史 `scale` 模型缺失 `category` 的问题。

`assignments.json` 是经审核的 21 项分类清单，使用当前目录和小程序共同采用的值：

| 分类 | 值 |
| --- | --- |
| 注意力/多动 | `adhd` |
| 抽动障碍 | `td` |
| 孤独症谱系 | `asd` |
| 压力 | `pressure` |
| 感觉统合 | `sii` |
| 执行功能 | `efn` |
| 情绪 | `emt` |
| 睡眠 | `slp` |

IPIP Big-Five 与 MBTI 风格偏好均已归档。清单将它们保留为带 `skip: true` 的审计记录；脚本不会尝试编辑或重新发布归档模型。SCL-90 当前存在历史读取错误，也暂以 `skip: true` 保留；因此本轮情绪分类的验收数量为 3，待其发布快照清理后再单独补齐。

## 执行方式

先审阅清单并进行不写入预演：

```bash
bash scripts/oneoff/backfill_scale_categories/apply.sh --dry-run
```

确认后使用有模型管理和发布权限的短期 operator token 执行：

```bash
QS_APISERVER_URL=https://qs.fangcunmount.cn \
QS_OPERATOR_TOKEN="$QS_OPERATOR_TOKEN" \
QS_COLLECTION_URL=https://collect.fangcunmount.cn \
bash scripts/oneoff/backfill_scale_categories/apply.sh --apply
```

脚本对每个分类变更按以下顺序调用受保护 API：

1. 先读取所有非归档模型，确认它们都能作为可编辑草稿读取；这些读取结果会在本次运行内复用。任一读取失败就以状态码 `2` 退出，不进行任何新的写入；
2. `PUT /assessment-models/{code}/basic-info` 更新分类（已发布模型会 fork 为草稿）；
3. `POST /assessment-releases/{code}/publish` 走正常发布事务，在 `assessment_models` 生成新的 `published_snapshot`；
4. 查询 8 个分类接口并校验预期数量。

脚本不会直接更新 `published_snapshot`，因此不会产生工作记录与发布快照分类不一致的问题。
