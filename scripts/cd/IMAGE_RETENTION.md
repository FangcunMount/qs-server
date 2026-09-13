# 发布镜像保留

发布脚本在目标服务器取得共享锁，然后执行镜像导入/拉取、容器更新和现有验证。
验证成功后调用 `image-retention.py --apply`；清理失败输出 warning，不将健康发布回滚。
工作流会先执行 `python3 scripts/cd/test_image_retention.py`。

## 保留规则

- 每个服务保留当前成功版本及最近两个不同的成功镜像 ID；重复部署和回滚按成功执行顺序更新。
- 保护所有现有容器（包含停止的容器）的镜像引用。
- 首次接入保护现存最新三个镜像及替换前容器使用的镜像；这些只记录为 bootstrap，不能当作成功发布证据。
  积累三个成功版本后取消 bootstrap 保护。因此过渡期/容器引用可能使保留数量超过三个。
- 同一镜像的 ACR/GHCR 标签合并计算；只删除当前服务允许仓库中的旧标签。
  带未知仓库别名的镜像、无标签镜像、工具镜像和数据卷均不参与本脚本的删除。
- 使用 `docker image rm --no-prune`，不强制删除，不运行全局 system/image prune。

## 锁与记录

所有部署入口共用 `/var/lib/fangcun-image-retention/deploy.lock`，覆盖导入到清理全程。
目录 root 所有，0755；锁文件 0666 允许不同部署账号锁定同一个 inode，不能删除或替换锁文件。
版本记录和最近清理记录 root 所有，0600：

- `<service>.json`：成功版本与过渡期保护列表，原子更新。
- `<service>-last-cleanup.json`：候选、已删标签、结果和磁盘可用字节数。

独立调用默认只预览，不更新记录也不删除；省略 `--deployment-locked`，让脚本自己取得同一把锁。
该内部参数仅供已经持锁的发布入口使用。独立调用的 image-ref 必须是已完成验证且正在运行的版本。
例如在首次新版发布初始化目录后：

```bash
sudo python3 /path/to/image-retention.py --service SERVICE --image-ref REPOSITORY:TAG
```

需要应用预览时增加 `--apply`。成功记录写入失败时不删除；删除遇到引用冲突或容器状态变化时停止并记录失败。
脚本依赖服务器 Python 3、Docker CLI，Shell 发布还依赖 flock。缺少部署锁前置条件会阻止开始发布。

## 交付与维护

同一份标准库脚本随四个仓库的发布包交付，避免生产发布时动态下载可变脚本。
修改策略时同步 qs-server、iam、qs-operating-system、qs-ai 中的脚本和测试。
本改动不安装定时任务，不调整远程镜像仓库策略，不清理发布目录或备份。
合并并成功执行新版 CI/CD 后才在生产生效；本地测试通过不代表生产已接入。

## 部署锁初始化兼容性

Shell 入口使用已获准的 mkdir/chown/chmod/ln 初始化锁，不要求 sudo touch。
候选文件在发布包目录内准备，通过不覆盖目标的硬链接发布；包目录与锁目录必须位于同一文件系统。
已有锁保持 inode 和内容，竞争初始化复用获胜者；初始化失败则停止部署。临时文件自动清理。
`python3 scripts/cd/test_image_deploy_lock.py` 验证命令权限边界、竞争与失败路径。
镜像清理仍需单独的执行权限；清理助手被拒绝时只记录警告，不视为清理成功，也不因此回滚健康服务。
