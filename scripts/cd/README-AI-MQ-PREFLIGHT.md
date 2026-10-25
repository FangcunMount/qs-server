# QS MQ 发布预检工具

MQ 已建立归属后，正常部署必须先完成离线预检，之后才同步配置或停止旧实例。预检固定原密钥路径、完整配置、运行镜像身份与历史回退资产，不连接数据库、Broker 或模型。

ACR 直拉模式先在服务器拉取本次 SHA 镜像，再执行相同离线预检；拉取失败不停止旧实例。镜像选择方式不改变固定工具、归属标记或回退资产保护。

服务器部署账号不能从可写上传目录以 root 运行任意 Python。发布负责人须在生产执行材料中核对 `scripts/cd/ai-messaging-release.py` 的 SHA-256，将原字节安装为 `/opt/qs-server/qs-apiserver/ai-mq-tools/<sha256>.py`，目录及文件由 root 拥有，目录不可被其他用户写入，文件模式为 0555。

为该固定文件设置有限 sudo 规则，仅允许 QS apiserver 的不可变 40 位源码标签、原上传目录下的 `configs/apiserver.prod.yaml` 和既定 UID/GID。不能授权上传目录中的 Python、任意解释器参数、未知服务镜像或 root UID。安装前后均校验 sudoers，保留规则及工具摘要；先验证错误文件、错误参数被拒绝，再以部署账号执行离线预检。

正常部署根据包内工具摘要选择已安装文件，并拒绝缺失、符号链接、非 root 所有、模式变化及摘要不一致。升级工具时须先准备新的固定文件与受限权限，保留仍被回退版本使用的旧工具；不修改共享 Broker、IAM 或部署暂停开关。

成功预检后，部署使用现有允许的 rsync、chown、chmod 和 test 创建空的永久 `required` 归属标记，之后才停止旧实例。既有标记不覆盖、不更新时间；符号链接拒绝，不新增 root touch 权限。

上传配置保持部署账号所有的 0600 权限。首次预检把原始字节复制到普通镜像 UID/GID 所有的私有暂存文件，以 0400 只读挂载；验证完成后删除副本，再冻结现有四项发布资产。不得通过放宽上传文件权限解决预检失败，也不得把暂存副本留作另一份生产配置。

RocketMQ 普通消息绑定新增显式 provider、publication gate 与三个只读传输文件（credentials、CA、routes）。离线预检校验完整可信路由、CA、权限与内容摘要，不连接 Broker。固定 root 工具升级仍需安装新摘要文件，保留旧工具；不得以本地修改上传脚本绕过生产门禁。

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
