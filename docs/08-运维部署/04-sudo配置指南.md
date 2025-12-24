# Sudo 配置指南

## 问题说明

当部署用户需要执行需要 root 权限的操作时（如部署 crontab 配置、脚本文件等），如果 sudo 配置限制了可执行的命令，会出现以下错误：

```text
Sorry, user *** is not allowed to execute '/usr/bin/cp ...' as root
```

## 解决方案

### 推荐配置：Crontab 部署专用 sudoers 配置

在服务器上以 root 用户执行：

```bash
sudo visudo -f /etc/sudoers.d/qs-crontab-deploy
```

**完整配置内容：**

```bash
# ===== Crontab 部署 sudo 白名单：deploy =====
# 只放行必要二进制；全部 NOPASSWD；使用命令别名清晰管理

# 安全默认设置
Defaults:deploy !requiretty
Defaults:deploy env_reset
Defaults:deploy secure_path="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
# 可选：启用 sudo 日志（如果 sudo 版本支持）
# Defaults:deploy logfile="/var/log/sudo-deploy.log"

# 允许保留必要的环境变量（用于脚本参数展开）
Defaults:deploy env_keep += "SCRIPTS_DIR CONFIG_DIR CRON_DIR LOG_DIR LOGROTATE_DIR SUDO_PASSWORD"

# --- 命令分组（清晰可管控） ---
# 文件操作命令
Cmnd_Alias FILE_CMDS = /usr/bin/mkdir, /usr/bin/chmod, /usr/bin/chown, /usr/bin/cp, /usr/bin/tar
# 文件读取/诊断命令
Cmnd_Alias READ_CMDS  = /usr/bin/grep, /usr/bin/test, /usr/bin/ls
# Shell 命令（用于脚本语法检查）
Cmnd_Alias SHELL_CMDS = /bin/bash, /usr/bin/bash
# 系统服务管理命令
Cmnd_Alias SYSTEM_CMDS = /bin/systemctl

# --- 授权 ---
# 替换 deploy 为实际的部署用户名
deploy ALL=(root) NOPASSWD: FILE_CMDS, READ_CMDS, SHELL_CMDS, SYSTEM_CMDS
```

**配置说明：**

- `!requiretty`：允许非交互式执行（SSH 部署必需）
- `env_reset`：重置环境变量，提高安全性
- `secure_path`：限制 PATH，防止路径劫持
- `env_keep`：保留脚本需要的环境变量
- `NOPASSWD:`：执行这些命令时不需要输入密码
- `Cmnd_Alias`：使用命令别名组织命令，便于管理和维护

## 快速配置步骤

1. **创建配置文件**：
   ```bash
   sudo visudo -f /etc/sudoers.d/qs-crontab-deploy
   ```

2. **复制上面的配置内容**（替换 `deploy` 为实际用户名）

3. **保存并验证**：
   ```bash
   # 检查语法
   sudo visudo -c
   
   # 如果显示 "parsed OK"，说明配置正确
   ```

4. **测试配置**（见下方验证部分）

## 部署脚本需要的命令列表

根据 `deploy-crontab.yml` 脚本，需要以下 sudo 权限：

### 文件操作命令（FILE_CMDS）

| 命令 | 用途 | 路径 |
|------|------|------|
| `mkdir` | 创建目录 | `/usr/bin/mkdir` |
| `chmod` | 修改文件权限 | `/usr/bin/chmod` |
| `chown` | 修改文件所有者 | `/usr/bin/chown` |
| `cp` | 复制文件 | `/usr/bin/cp` |
| `tar` | 解压部署包 | `/usr/bin/tar` |

### 文件读取/诊断命令（READ_CMDS）

| 命令 | 用途 | 路径 |
|------|------|------|
| `grep` | 读取文件内容 | `/usr/bin/grep` |
| `test` | 检查文件是否存在 | `/usr/bin/test` |
| `ls` | 列出文件 | `/usr/bin/ls` |

### Shell 命令（SHELL_CMDS）

| 命令 | 用途 | 路径 |
|------|------|------|
| `bash` | 语法检查脚本 | `/bin/bash`, `/usr/bin/bash` |

### 系统服务命令（SYSTEM_CMDS）

| 命令 | 用途 | 路径 |
|------|------|------|
| `systemctl` | 检查服务状态 | `/bin/systemctl` |

**注意：** 不同 Linux 发行版的命令路径可能不同（如 `/bin/` vs `/usr/bin/`），请根据实际情况调整。可以使用 `which mkdir` 等命令查看实际路径。

## 验证配置

配置完成后，可以测试：

```bash
# 切换到部署用户
su - deploy

# 测试文件操作命令（应该不需要密码）
sudo mkdir -p /tmp/test
sudo cp /etc/hosts /tmp/test/
sudo chmod 755 /tmp/test
sudo chown root:root /tmp/test/hosts

# 测试读取命令
sudo grep -q "localhost" /etc/hosts
sudo test -f /etc/hosts
sudo ls -l /etc/hosts

# 测试 shell 命令
sudo bash -n /etc/hosts 2>/dev/null || echo "语法检查完成"

# 测试系统服务命令
sudo systemctl status cron --no-pager || sudo systemctl status crond --no-pager

# 清理测试
sudo rm -rf /tmp/test
```

## 安全建议

1. **最小权限原则**：只授予必要的命令权限，不要使用 `ALL=(ALL) NOPASSWD: ALL`
2. **使用命令别名**：使用 `Cmnd_Alias` 组织命令，便于管理和审查
3. **定期审查**：定期检查 sudoers 配置，移除不再需要的权限
4. **使用 sudoers.d**：使用独立的配置文件，便于管理
5. **启用日志**：如果支持，启用 sudo 日志记录（取消注释 `logfile` 行）
6. **环境变量控制**：使用 `env_reset` 和 `env_keep` 控制环境变量

## 故障排查

如果配置后仍然无法执行：

1. **检查语法**：使用 `sudo visudo -c` 检查配置语法
2. **检查用户组**：确认用户在正确的组中
3. **检查顺序**：sudoers 文件按顺序读取，后面的规则可能覆盖前面的
4. **查看日志**：检查 `/var/log/auth.log` 或 `/var/log/secure` 查看详细错误
5. **检查路径**：确认命令路径正确（使用 `which` 或 `whereis` 查看）

## 相关文档

- [Sudoers Manual](https://www.sudo.ws/man/1.8.15/sudoers.man.html)
- [Linux Sudo 配置指南](https://linux.die.net/man/5/sudoers)
