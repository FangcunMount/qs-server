# qs-ai 历史核验临时适配器退出说明

本页对应 R4 源码退出候选，`doc_status=needs_review`、`runtime_status=unknown`。为 qs-server 四对象清理引入的 qs-ai 历史观察、0040 布局适配和退休核验脚本及其专用测试随本批退出；qs-ai 自己的业务数据库、模型执行和当前消息协议继续由 qs-ai 项目承担。

## 四对象核验边界

本批只核对旧命令的原身份、摘要、业务归属及待处理责任。qs-ai 的 provider unknown、执行租约、首个 receipt 与业务终态必须按其真实所有者解释；重新发送、生成新 ID 或 Broker 投递确认不能替代原责任结论。

既有业务记录保存无旧正文的核验结论和原引用。退休 command ID 持续占用原身份，拒绝复用和迟到回执的错误应用。当前正常 MQ 读取与退休 ID 防护见[现用 MQ 只读清单](../../cmd/qs-ai-bridge/README.md)。本批临时适配器不成为新的长期 qs-ai 运行时依赖。

## 生产验收边界

历史身份、可信源归属及责任闭环须由实际本批扫描与回执证明。全 Broker 历史和其他 qs-ai 功能的完整验收不因本次四对象核验而自动成立。源码退出前仍须满足[五项实际退出条件](compatibility-retirement.md#final-historical-readback-and-temporary-material-cleanup)；本候选没有声称这些生产条件已经满足。

## 无正文历史证明来源

原读取范围、适配设计和当时的测试记录见[固定 B Git 快照](https://github.com/FangcunMount/qs-server/blob/6fdae9a1ebe74490309e88cdee4cec9b0c675715/scripts/database/qs-ai-retirement-readonly-plan.md)，source SHA 为 `6fdae9a1ebe74490309e88cdee4cec9b0c675715`。该链接定位历史证明，保留 needs_review 状态；本页不复制旧 SQL、查询模板或执行器正文。
