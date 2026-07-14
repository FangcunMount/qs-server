# SPM 感觉统合量表初始化脚本

此脚本初始化的是 **Sensory Processing Measure（感觉统合量表）**，不是 Raven 标准推理测验。模型默认编码和问卷编码均为 `bJFKi3`。

写入模型身份为：

```text
kind=behavioral_rating
algorithm=spm_sensory
product_channel=behavior_ability
payload_format=assessmentmodel.behavioral_rating.default.v1
```

它写入 `assessment_norms`、`assessment_models` 和 `published_assessment_models`，以 SOC、VIS、HEA、TOU、BOD、BAL、PLA 和 TOT 的原始分范围映射 T 分和百分位。源表中空白的顶端百分位按其已有顶码规则保存为 `99`；脚本会输出使用该回退的条数。

## 题目映射

仓库已经根据已发布问卷 `bJFKi3@4.0.1` 准备好 [data/bJFKi3_4.0.1_factor_map.json](./data/bJFKi3_4.0.1_factor_map.json)。映射文件显式绑定问卷 code/version。

`TOT` 不可填入映射。按现有常模口径，它汇总 VIS、HEA、TOU、BOD、BAL 五个标准感觉系统维度以及5个“味觉与嗅觉”题，共56题；SOC（社会参与）和 PLA（规划与想法）单独报告，不计入 TOT。味觉与嗅觉使用历史辅助因子编码 `wcgKM7uV`，只参与 TOT，不单独查常模或展示为标准维度。

脚本会拒绝：缺失维度、未在问卷中发布的题目、同一题归属多个维度，以及未分配的可作答题目。

脚本还会校验四级选项的计分方向：SOC 的10题和“拥有良好的平衡感”（`jenu1Rox`）必须反向计分，其余题目必须正向计分。这样即使后续有人改了问卷选项分值，也不会带着错误方向发布。

## 执行

```bash
# 只校验常模源和映射文件
go run ./scripts/oneoff/seed_spm_sensory/ \
  --questionnaire-code bJFKi3 --questionnaire-version 4.0.1 \
  --norm-source /path/to/SPM_Norms.php \
  --factor-map ./scripts/oneoff/seed_spm_sensory/data/bJFKi3_4.0.1_factor_map.json

# 实际写入
go run ./scripts/oneoff/seed_spm_sensory/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs \
  --questionnaire-code bJFKi3 --questionnaire-version 4.0.1 \
  --norm-source /path/to/SPM_Norms.php \
  --factor-map ./scripts/oneoff/seed_spm_sensory/data/bJFKi3_4.0.1_factor_map.json \
  --apply
```

已有同编码模型时，必须先审阅快照；仅在确认需要整体替换时使用 `--force`。常模版本不可覆盖，只有内容完全相同的重复导入才会幂等通过。

当前 qs-server 尚未注册 `/norm-tables` 写入路由，因此 operating API token 只能用于只读核验问卷，不能替代 Mongo 完成常模导入。本脚本的 `--apply` 仍需要 `MONGO_URI`；在常模表 REST 契约上线前不要尝试用后台 token 绕过此限制。

`SPM_Texts.php` 的详细报告依赖敏感、低反应、感觉寻求等子因子。当前问卷映射只足以建立八个顶层常模因子，因此本脚本不会伪造这些子因子或写入无法被报告运行时消费的富文本。
