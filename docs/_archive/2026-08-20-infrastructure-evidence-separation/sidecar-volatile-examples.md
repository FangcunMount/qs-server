# Sidecar 易失示例快照（归档）

> 归档原因：下列内容曾混入 maintained sidecar，但没有绑定 exact deployed SHA、
> effective config、执行记录和有效期。它们只用于追溯原文，不是当前配置、生产状态
> 或执行授权。现行 sidecar 只保留稳定契约、前置条件与验证入口；真实执行结果
> 应进入 `docs/00-总览/10-基础设施生产证据台账.md` 及其机器台账。

## 移出的历史内容

| 原 sidecar | 移出内容 | 归档判定 |
| --- | --- | --- |
| `scripts/oneoff/cleanup_perf_testee_data/README.md` | 备份后缀 `temp_assessment_20260819` | 某次日期化输入；现改为本次变更显式输入 |
| `scripts/oneoff/select_seeddata_duplicate_testees/README.md` | clinician ID `614995509882401326`、日期窗口 `2026-08-05` 至 `2026-08-12`、日期化输出路径/备份后缀 | 未绑定证据的历史运行示例 |
| `scripts/oneoff/smoke_modelcatalog_cutover/README.md` | `https://collect.fangcunmount.cn` 与 `ISI7,MBTI_OEJTS,gXkk9W,bJFKi3` 模型列表 | 环境 endpoint 与发布目录输入；不是稳定工具契约 |
| `scripts/oneoff/govern_interpretation_template_releases/README.md` | “生产缺失 `enneagram@legacy-v1`” | 未绑定时间、SHA 或查询的环境断言；现改为条件化工具行为 |
| `scripts/perf/README.md` | “当前硬件 120 QPS 上限” | 无 exact SHA/负载环境/原始 run 的容量断言；`ceiling-120` 只是版本化计划 |

本快照不补写不存在的证据，也不将上述值转换为新的生产结论。
