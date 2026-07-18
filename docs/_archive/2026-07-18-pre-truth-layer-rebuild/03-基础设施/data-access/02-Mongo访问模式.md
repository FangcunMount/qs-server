# Mongo 访问模式

## 1. 解决什么问题

Mongo 访问模式解决文档型、结构嵌套、版本快照类数据的持久化问题。

## 2. 所在位置

Mongo 主要承接 AnswerSheet、Questionnaire、InterpretReport、Outbox Mongo store 等文档型数据。

## 3. 设计目标

按聚合保存文档；避免过度 join；保留业务快照；索引服务查询路径；写入和读模型职责分清。

## 4. 正常流程

应用服务通过 repository 写入文档；读取时按业务 ID 或索引字段查询；需要高频读取的数据进入 cache。

## 5. 异常流程

写入失败不得发布业务事件；查询慢要检查索引和读模型；大文档增长要拆分或快照化。

## 6. 观测指标

collection latency、slow query、index miss、document size、write error、read fallback。

## 7. 代码事实源

- [../../../internal/apiserver/infra/mongo](../../../internal/apiserver/infra/mongo)
- [../../../internal/apiserver/infra/mongo/eventoutbox](../../../internal/apiserver/infra/mongo/eventoutbox)
