# qs-server 文档中心

这套文档只保留当前仍值得维护的事实层。旧目录、历史设计和重建前材料已经整体隔离到 [`_archive`](./_archive/README.md)，不会参与现行阅读路径。

## 1. 从哪里开始

| 读者问题 | 入口 |
| --- | --- |
| 系统是什么、边界在哪里 | [00-总览](./00-总览/README.md) |
| 三个进程如何启动和协作 | [01-运行时](./01-运行时/README.md) |
| 领域模型、服务和关键链路在哪里 | [02-业务模块](./02-业务模块/README.md) |
| 可靠事件、缓存、并发与韧性如何实现 | [03-基础设施](./03-基础设施/README.md) |
| REST、gRPC、配置、部署和前端接入看哪里 | [04-接口与运维](./04-接口与运维/README.md) |
| 为什么作出关键架构选择 | [05-决策记录](./05-决策记录/README.md) |
| 如何对外讲解项目 | [06-宣讲](./06-宣讲/README.md) |

第一次阅读建议按 `00 -> 01 -> 02`；排障或接入可直接从 `04` 进入。

## 2. 文档层次

```text
00-05：现行事实、操作说明和已确认的设计决策
06：从现行事实派生的讲解材料，不承担实现真值
_archive：历史快照，只供追溯
```

源码和机器契约始终高于 prose 文档。具体优先级与维护规则见 [文档写作约定](./CONTRIBUTING-DOCS.md)。

## 3. 当前模块地图

模块名以 [`internal/apiserver/container/modules/registry.go`](../internal/apiserver/container/modules/registry.go) 为准：

```text
survey -> modelcatalog -> evaluation -> interpretation
   |            |              |              |
actor -------- plan -------- statistics ------+
```

这张图表达阅读关系，不表示所有代码依赖。真实依赖和组合顺序应回到模块 `wire.go`、`install.go`、容器注册表和架构测试核对。

## 4. 机器契约入口

| 契约 | 事实源 |
| --- | --- |
| apiserver REST | [`api/rest/apiserver.yaml`](../api/rest/apiserver.yaml) |
| collection REST | [`api/rest/collection.yaml`](../api/rest/collection.yaml) |
| gRPC | [`api/grpc/proto`](../api/grpc/proto/) 与生成代码 |
| 领域事件 | [`configs/events.yaml`](../configs/events.yaml) |
| 一次性信令 | [`configs/signals.yaml`](../configs/signals.yaml) |
| 运行配置 | [`configs`](../configs/) |

## 5. 重建状态

本轮把 181 篇、约 4.7 万行的旧现行树收缩为一个更小的维护集合。已复核和待补证据范围见 [重建状态](./MIGRATION-STATUS.md)。

## 6. 提交前验证

```bash
make docs-hygiene
make docs-facts
git diff --check
```

`docs-hygiene` 检查链接、锚点和章节编号；`docs-facts` 检查现行树边界、模块入口和关键契约名称。两者都通过，仍不等于正文事实永久正确，涉及行为变更时必须重新沿代码链核对。
