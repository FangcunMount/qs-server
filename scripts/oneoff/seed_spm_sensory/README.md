# SPM 感觉统合量表初始化脚本

此脚本初始化 **Sensory Processing Measure（感觉统合量表）**，不是 Raven 标准推理测验。模型默认编码和问卷编码均为 `bJFKi3`。

模型身份：

```text
kind=behavioral_rating
algorithm=spm_sensory
product_channel=behavior_ability
payload_format=assessmentmodel.behavioral_rating.default.v1
```

脚本写入 `assessment_norms`，以及 `assessment_models` 中的 head/published_snapshot。它只需要 MongoDB，不使用 MySQL。常模已经作为版本化、gzip+base64 编码的 JSON 资产内嵌在 `data/spm-sensory-cn-legacy-bJFKi3-v1.json.gz.b64`，服务器不需要 PHP，也不需要本地附件。Base64 不是加密，不应把未经授权的常模资产提交到公开仓库。

内嵌资产解压后 JSON 的 SHA-256 为 `280841d9893b72ece1170417eeb0f1ead05e0aad5c8d605928576d59c0db473b`，可用于部署前完整性核验。

## 题目映射

仓库已经根据已发布问卷 `bJFKi3@4.0.1` 准备好 [data/bJFKi3_4.0.1_factor_map.json](./data/bJFKi3_4.0.1_factor_map.json)。映射文件显式绑定问卷 code/version。

`TOT` 汇总 VIS、HEA、TOU、BOD、BAL 五个标准感觉系统维度以及 5 个“味觉与嗅觉”题，共 56 题；SOC 和 PLA 单独报告，不计入 TOT。味觉与嗅觉使用历史辅助因子编码 `wcgKM7uV`，只参与 TOT。

脚本校验四级选项计分方向：SOC 的 10 题和“拥有良好的平衡感”（`jenu1Rox`）必须反向计分，其余题目必须正向计分。

## 服务器执行

先验证内嵌常模和映射，不连接数据库：

```bash
go run ./scripts/oneoff/seed_spm_sensory/ \
  --questionnaire-code bJFKi3 \
  --questionnaire-version 4.0.1 \
  --factor-map ./scripts/oneoff/seed_spm_sensory/data/bJFKi3_4.0.1_factor_map.json
```

确认 dry-run 输出后连接 MongoDB 写入：

```bash
go run ./scripts/oneoff/seed_spm_sensory/ \
  --mongo-uri "$MONGO_URL" --mongo-db "${MONGO_DB:-qs}" \
  --questionnaire-code bJFKi3 \
  --questionnaire-version 4.0.1 \
  --factor-map ./scripts/oneoff/seed_spm_sensory/data/bJFKi3_4.0.1_factor_map.json \
  --apply
```

若服务器使用 `MONGO_URI`，可以省略 `--mongo-uri`。`--norm-source` 仅用于传入同结构的规范化 JSON 覆盖文件，通常不要使用。

常模、草稿和发布快照在一个 MongoDB 多文档事务中写入，目标 MongoDB 必须是副本集或分片集群。已有同编码模型时脚本会拒绝普通写入。只有在完成数据库备份、审阅当前草稿和发布快照并确认需要整体替换后，才允许增加 `--force`。强制迁移按 model code 和问卷 code/version 识别历史快照，不依赖旧模型的 kind/algorithm，因此支持历史 `scale` 迁移到 `behavioral_rating/spm_sensory`；无关 model code 或其他问卷版本冲突会被拒绝。常模版本不可覆盖，相同版本仅允许内容完全相同。

## 验收

确认：

- 常模版本为 `spm-sensory-cn-legacy-bJFKi3-v1`；
- 模型算法为 `spm_sensory`，而不是 `spm`；
- 问卷绑定为 `bJFKi3@4.0.1`；
- 常模因子为 SOC、VIS、HEA、TOU、BOD、BAL、PLA、TOT；
- 模型状态和发布快照均存在。
