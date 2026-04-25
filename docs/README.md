# qs-server 文档中心

**本文回答**：`docs/README.md` 只负责三件事：说明 `docs/` 的目录边界、给出稳定阅读入口、明确现行真值层与 archive 的关系；它不替代各组正文，也不重复维护第二套细节目录。

---

## 30 秒结论

| 维度 | 结论 |
| ---- | ---- |
| 文档目标 | 让读者先知道“该从哪里进入”，再进入具体目录 |
| 真值层 | `00-05` 是现行真值层；`06-宣讲` 是讲解层；`_archive` 是历史层 |
| 事实优先级 | 源码与机器契约优先于 prose 文档 |
| 阅读原则 | 先总览，再按问题进入运行时 / 业务模块 / 基础设施 / 接口运维 / 专题分析 |
| 事件入口 | 事件系统统一从 [03-基础设施/01-事件系统.md](./03-基础设施/01-事件系统.md) 和 [03-基础设施/event/README.md](./03-基础设施/event/README.md) 进入 |
| Redis 入口 | Redis 相关文档统一从 [03-基础设施/12-Redis文档中心.md](./03-基础设施/12-Redis文档中心.md) 进入 |
| Resilience 入口 | 限流、队列、背压、锁、幂等和降级统一从 [03-基础设施/resilience/README.md](./03-基础设施/resilience/README.md) 进入 |
| Security 入口 | JWT、IAM、authz snapshot、capability、service auth、mTLS/ACL 统一从 [03-基础设施/security/README.md](./03-基础设施/security/README.md) 进入 |

---

## 事实来源优先级

阅读或维护文档时，默认按下面的优先级判断真值：

1. **源码**：`internal/`、`cmd/`、`pkg/`
2. **机器契约与配置**：`api/rest/`、`proto/`、`configs/events.yaml`、`configs/*.yaml`
3. **`docs/00-05` 现行正文**
4. **`docs/06-宣讲`**
5. **`docs/_archive`**

如果 prose 与代码、契约冲突，以代码和契约为准。

---

## 目录地图

| 目录 | 解决什么问题 | 什么时候进入 |
| ---- | ------------ | ------------ |
| [00-总览](./00-总览/) | 系统地图、代码边界、主链路、本地开发入口 | 第一次读仓库时 |
| [01-运行时](./01-运行时/) | 三进程职责、调用方向、运行时协作 | 需要确认谁调谁、怎么跑时 |
| [02-业务模块](./02-业务模块/) | `survey / scale / evaluation / actor / plan / statistics` 六个模块的深讲 truth layer | 需要看模块职责、对象模型、状态机和 SOP 时 |
| [03-基础设施](./03-基础设施/) | 事件、存储、Redis、缓存限流、IAM、配置等横切能力 | 需要看机制、配置和代码挂载点时 |
| [04-接口与运维](./04-接口与运维/) | REST / gRPC 契约、端口部署、调度任务、事故复盘 | 需要看机器契约和运维入口时 |
| [05-专题分析](./05-专题分析/) | 为什么这样拆界、为什么同步提交但异步评估、为什么要保护层 | 需要看系统级设计判断时 |
| [06-宣讲](./06-宣讲/) | 对外讲解、技术分享、答辩材料 | 需要“把项目讲清楚”时 |
| [_archive](./_archive/) | 历史设计稿、阶段记录、旧文档 | 只在需要历史背景时参考 |

---

## 推荐阅读入口

### 第一次读仓库

1. [00-总览/README.md](./00-总览/README.md)
2. [00-总览/01-系统地图.md](./00-总览/01-系统地图.md)
3. [00-总览/02-代码组织与边界.md](./00-总览/02-代码组织与边界.md)
4. [00-总览/03-核心业务链路.md](./00-总览/03-核心业务链路.md)
5. 再按问题进入 `01-05`

### 需要改模块或看领域设计

- 从 [02-业务模块/README.md](./02-业务模块/README.md) 进入
- 需要深入模块时，优先进入对应子目录：`survey/`、`scale/`、`evaluation/`、`actor/`、`plan/`、`statistics/`

### 需要排障、看运行时或跨进程链路

- 从 [01-运行时/README.md](./01-运行时/README.md) 进入
- 如涉及保护层与异步链，再看 [05-专题分析](./05-专题分析/)

### 需要看事件系统

1. [03-基础设施/01-事件系统.md](./03-基础设施/01-事件系统.md)
2. [03-基础设施/event/README.md](./03-基础设施/event/README.md)
3. [03-基础设施/event/02-Publish与Outbox.md](./03-基础设施/event/02-Publish与Outbox.md)
4. [03-基础设施/event/03-Worker消费与AckNack.md](./03-基础设施/event/03-Worker消费与AckNack.md)

### 需要看 Redis

1. [03-基础设施/12-Redis文档中心.md](./03-基础设施/12-Redis文档中心.md)
2. [03-基础设施/redis/README.md](./03-基础设施/redis/README.md)
3. [03-基础设施/06-Redis使用情况.md](./03-基础设施/06-Redis使用情况.md)
4. [03-基础设施/13-Redis缓存业务清单.md](./03-基础设施/13-Redis缓存业务清单.md)

### 需要看高并发治理 / Resilience Plane

1. [03-基础设施/resilience/README.md](./03-基础设施/resilience/README.md)
2. [03-基础设施/resilience/00-整体架构.md](./03-基础设施/resilience/00-整体架构.md)
3. [03-基础设施/resilience/05-观测降级与排障.md](./03-基础设施/resilience/05-观测降级与排障.md)
4. [03-基础设施/resilience/06-新增高并发治理能力SOP.md](./03-基础设施/resilience/06-新增高并发治理能力SOP.md)
5. [03-基础设施/resilience/07-能力矩阵.md](./03-基础设施/resilience/07-能力矩阵.md)

### 需要看身份与安全控制面

1. [03-基础设施/security/README.md](./03-基础设施/security/README.md)
2. [03-基础设施/security/00-整体架构.md](./03-基础设施/security/00-整体架构.md)
3. [03-基础设施/security/01-Principal与TenantScope.md](./03-基础设施/security/01-Principal与TenantScope.md)
4. [03-基础设施/security/02-AuthzSnapshot与CapabilityDecision.md](./03-基础设施/security/02-AuthzSnapshot与CapabilityDecision.md)
5. [03-基础设施/security/03-ServiceIdentity与mTLS-ACL.md](./03-基础设施/security/03-ServiceIdentity与mTLS-ACL.md)

### 需要准备宣讲或对外介绍

- 从 [06-宣讲/README.md](./06-宣讲/README.md) 进入
- 需要完整分享脚本、图谱素材和追问证据时，继续看 [06-宣讲/09-30分钟技术分享脚本.md](./06-宣讲/09-30分钟技术分享脚本.md)、[06-宣讲/10-架构图素材索引.md](./06-宣讲/10-架构图素材索引.md)、[06-宣讲/11-面试追问证据索引.md](./06-宣讲/11-面试追问证据索引.md)

---

## 真值层与宣讲层

### 现行真值层

- [00-总览](./00-总览/)
- [01-运行时](./01-运行时/)
- [02-业务模块](./02-业务模块/)
- [03-基础设施](./03-基础设施/)
- [04-接口与运维](./04-接口与运维/)
- [05-专题分析](./05-专题分析/)

这几层回答“代码现在是什么”。

### 宣讲层

- [06-宣讲](./06-宣讲/)

这一层回答“怎么把项目讲清楚”，不承担机器契约和实现真值。

### 历史层

- [_archive](./_archive/)

这一层只保留历史背景，不进入现行阅读主路径，也不作为现行事实来源。

---

## archive 政策

`docs/_archive` 的定位是**长期保留的历史层**，不是临时垃圾桶，也不是第二真值层。

使用规则：

- 现行文档默认不依赖 `_archive`
- 从 `_archive` 回迁内容前，必须重新核对源码和契约
- `make docs-hygiene` 默认不检查 `_archive`

archive 的具体规则见 [docs/_archive/README.md](./_archive/README.md)。

---

## 维护入口

- 文档写作规则： [CONTRIBUTING-DOCS.md](./CONTRIBUTING-DOCS.md)
- 提交前校验： `make docs-hygiene`
- 根仓快速开始： [../README.md](../README.md)

---

## 代码与契约锚点

- REST： [../api/rest/apiserver.yaml](../api/rest/apiserver.yaml)、[../api/rest/collection.yaml](../api/rest/collection.yaml)
- gRPC Proto： [../internal/apiserver/interface/grpc/proto](../internal/apiserver/interface/grpc/proto)
- 事件： [../configs/events.yaml](../configs/events.yaml)
- 三进程入口：
  - [../cmd/qs-apiserver/apiserver.go](../cmd/qs-apiserver/apiserver.go)
  - [../cmd/collection-server/main.go](../cmd/collection-server/main.go)
  - [../cmd/qs-worker/main.go](../cmd/qs-worker/main.go)
