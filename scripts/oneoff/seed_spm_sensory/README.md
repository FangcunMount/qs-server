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

## 准备题目映射

输入 PHP 文件不含题目到七个非总分维度的归属。先从历史 `assessment_mode.code=bJFKi3` 导出对应关系，再按 [factor_map.example.json](./factor_map.example.json) 填写。`TOT` 不可填入映射：它由七个分维度原始分求和。

脚本会拒绝：缺失维度、未在问卷中发布的题目、同一题归属多个维度，以及未分配的可作答题目。

## 执行

```bash
# 只校验常模源和映射文件
go run ./scripts/oneoff/seed_spm_sensory/ \
  --norm-source /path/to/SPM_Norms.php \
  --factor-map /path/to/spm_sensory_factor_map.json

# 实际写入
go run ./scripts/oneoff/seed_spm_sensory/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs \
  --norm-source /path/to/SPM_Norms.php \
  --factor-map /path/to/spm_sensory_factor_map.json \
  --apply
```

已有同编码模型时，必须先审阅快照；仅在确认需要整体替换时使用 `--force`。常模版本不可覆盖，只有内容完全相同的重复导入才会幂等通过。
