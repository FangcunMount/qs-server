# QS MQ 发布预检工具

所有 API 正常部署（包括首次安装）必须先完成最终事务绑定离线预检，之后才同步配置或停止旧实例。预检固定原密钥路径、完整配置、运行镜像身份与历史回退资产，不连接数据库、Broker 或模型。

ACR 直拉模式先在服务器拉取本次 SHA 镜像，再执行相同离线预检；拉取失败不停止旧实例。镜像选择方式不改变固定工具、归属标记或回退资产保护。

服务器部署账号不能从可写上传目录以 root 运行任意 Python。发布负责人须在生产执行材料中核对 `scripts/cd/ai-messaging-release.py` 的 SHA-256，将原字节安装为 `/opt/qs-server/qs-apiserver/ai-mq-tools/<sha256>.py`，目录及文件由 root 拥有，目录不可被其他用户写入，文件模式为 0555。

为该固定文件设置有限 sudo 规则，仅允许 QS apiserver 的不可变 40 位源码标签、原上传目录下的 `configs/apiserver.prod.yaml` 和既定 UID/GID。不能授权上传目录中的 Python、任意解释器参数、未知服务镜像或 root UID。安装前后均校验 sudoers，保留规则及工具摘要；先验证错误文件、错误参数被拒绝，再以部署账号执行离线预检。

正常部署根据包内工具摘要选择已安装文件，并拒绝缺失、符号链接、非 root 所有、模式变化及摘要不一致。升级工具时须先准备新的固定文件与受限权限，保留仍被回退版本使用的旧工具；不修改共享 Broker、IAM 或部署暂停开关。

成功预检后，部署使用现有允许的 rsync、chown、chmod 和 test 创建空的永久 `required` 归属标记，之后才停止旧实例。既有标记不覆盖、不更新时间；符号链接拒绝，不新增 root touch 权限。

上传配置保持部署账号所有的 0600 权限。首次预检把原始字节复制到普通镜像 UID/GID 所有的私有暂存文件，以 0400 只读挂载；验证完成后删除副本，再冻结现有四项发布资产。不得通过放宽上传文件权限解决预检失败，也不得把暂存副本留作另一份生产配置。

最终事务绑定要求显式 RocketMQ、publication gate、原 qs.mysql 身份和完整预算。四个只读传输文件为 SQL credentials、Mongo credentials、CA、routes；校验完整可信路由、不同 Broker 主体、CA、权限与内容摘要，不连接 Broker。固定 root 工具升级仍需安装新摘要文件，保留旧工具；不得以本地修改上传脚本绕过生产门禁。

## Public MySQL TLS

Production Actions use organization `MYSQL_PUBLIC_HOST`, `MYSQL_PUBLIC_PORT`,
`MYSQL_TLS_MODE=VERIFY_IDENTITY` and immutable `MYSQL_TLS_CA_REF`. The host verifies
the pinned infra CA before deployment or maintenance. Containers receive a read-only
CA mount; Go clients share component-base v0.8.2 verification. Credentials, query
budgets, timezone, transactions, schema and existing approval boundaries are preserved.
This connection change does not authorize a migration or business write. Local and
disposable test databases retain explicit `DISABLED`. Production cutover and business
acceptance require separate runtime evidence.

## M4 worker配置边界

worker发布现在只接受消费专用RocketMQ配置，冻结文件版本在同步配置前校验并加入Compose环境；发布门关闭、无事务owner。三个文件逐一只读挂载，缺失来源不自动创建。该变化不修改共享镜像保留策略、部署锁inode、API不可变AI预检或原保护列表。当前CD继续暂停，实际文件目录、账号、配置和八实例重建仍须通过M4门禁；源码验证不表示生产已安装。详见 [部署正文](../../docs/04-接口与运维/06-部署与端口.md#7-m4-worker生产配置收敛)。

## M4：覆盖AI配置时保留领域事务绑定

最终API主配置包含 `messaging.rocketmq.transactions` 中的字面owner键 `qs.mysql`、`qs.mongo`。离线AI绑定渲染保留原领域messaging子树，只覆盖具名的AI messaging配置；不使用 `AllSettings` 重建这两个点号键。原store/epoch的大小写、UTF-8和美元字节、两个owner的凭据文件及既有预算保持，关闭的发布准入也不被打开。

真实渲染输出再次通过宿主 `app.DecodeOptions` 验证两种存储绑定和AI的同一SQL身份；既有不读环境秘密、不加载业务事实的测试继续运行。此修复只解决配置覆盖中的键形状，不完成全部最终CD参数/私有文件目录重建，不安装生产资源或启用CD。M4进度见 [infra总表](https://github.com/FangcunMount/infra/blob/main/docs/server/rocketmq-m4-execution.md)。

## M4：API生产包的双owner与固定授权订阅

API打包要求显式RocketMQ和原有 `qs-apiserver-1` 实例身份；从四个非秘密部署变量
`ROCKETMQ_MYSQL_STORE_ID/ROCKETMQ_MYSQL_WRITER_EPOCH` 与
`ROCKETMQ_MONGO_STORE_ID/ROCKETMQ_MONGO_WRITER_EPOCH` 冻结原已安装身份。
不得为新镜像生成或规范化这些值。缺失、超过128 UTF-8字节或无效provider/准入在替换原包之前拒绝。
包中的 `qs.mysql` 与 `qs.mongo` 分别使用 `credentials.json` 和 `mongo-credentials.json`；
两者继续使用m3-1及4发送/2回查worker/16队列/每owner每轮50条维护预算。
IAM订阅保留 `qs-authz-sync` 前缀与原实例身份，使用原SQL账号，仅订阅、不取得事务owner。
生产启动DDL和旧发布型Outbox关闭，默认新写准入关闭；事件catalog仍沿用当前业务分类。

API 的领域消息、AI 桥接和 IAM 订阅在同一冻结文件内显式配置，不再依赖消息环境变量补齐。
AI 桥接必须复用领域 SQL 的原 store/epoch、路由版本、发布准入和预算；IAM 始终仅订阅。
实际宿主 decoder 拒绝身份、凭据路径、预算、实例或准入不一致，以及启动 DDL/旧 Outbox。

原生镜像预检回执必须包含 `final_transaction_binding=true`，拒绝只有源码身份的旧工具。
主机工具挂载四份 MQ 材料和四份 JOSE 材料，连同冻结主配置共九个只读 bind，
全部禁止自动创建缺失来源。同一 SHA 的冻结发布再次使用时，也必须离线重跑原生预检，
校验原配置、密钥指纹、四份传输摘要及已冻结镜像，不能借当前指针重写旧发布。

required CI 使用标准非 root API 镜像、真实密钥/CA/路由和主机冻结工具执行十项离线验证，
包括缺失/权限错误/符号链接 Mongo 凭据、重复主体、错误事件 owner、缺失 IAM 路由和密钥。
容器断网、只读根文件系统且不启动应用、数据库、Broker 或模型。
生产 CD 保持暂停；生产工具安装、材料目录和八角色重建仍由 infra C06-D 单独记账。

M4 D03：新部署材料使用root管理的 `/var/lib/fangcun-m4-materials`，容器内文件路径和原密钥保持。
原目录、冻结镜像/发布资产及共享锁保留；生产CD继续暂停，八实例重建与最终发布单独验收。
