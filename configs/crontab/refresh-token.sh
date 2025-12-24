#!/bin/bash
# ============================================================
# IAM Token 自动刷新脚本
# ============================================================
# 用途：调用 IAM 登录接口获取 Token 并存储到本地
# 维护人：DevOps Team
# 最后更新：2025-01-XX
# 
# 使用说明：
# 1. 复制此文件到 /usr/local/bin/qs-refresh-token.sh
# 2. 设置执行权限：chmod +x /usr/local/bin/qs-refresh-token.sh
# 3. 由 api-call.sh 按需调用（当 Token 不存在时）
# 4. 确保在 crontab 配置文件中设置了 IAM_USERNAME 和 IAM_PASSWORD 环境变量
# ============================================================

set -euo pipefail

# ============================================================
# 配置变量（从环境变量或配置文件读取）
# ============================================================

# IAM 登录接口地址
IAM_LOGIN_URL="${IAM_LOGIN_URL:-https://iam.example.com/api/v1/auth/login}"

# IAM 用户名和密码（从环境变量或配置文件读取）
IAM_USERNAME="${IAM_USERNAME:-}"
IAM_PASSWORD="${IAM_PASSWORD:-}"

# Token 存储路径
TOKEN_FILE="${TOKEN_FILE:-/etc/qs-server/internal-token}"

# 日志文件
LOG_FILE="${LOG_FILE:-/data/logs/crontab/refresh-token.log}"

# 确保日志目录存在
LOG_DIR=$(dirname "${LOG_FILE}")
mkdir -p "${LOG_DIR}" 2>/dev/null || true

# 临时响应文件
TMP_RESPONSE="/tmp/iam-login-response-$$.json"

# ============================================================
# 函数定义
# ============================================================

# 记录日志
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" >> "${LOG_FILE}"
}

# 错误处理
error_exit() {
    log "ERROR: $*"
    rm -f "${TMP_RESPONSE}"
    exit 1
}

# 清理临时文件
cleanup() {
    rm -f "${TMP_RESPONSE}"
}
trap cleanup EXIT

# ============================================================
# 参数验证
# ============================================================

if [ -z "${IAM_USERNAME}" ]; then
    error_exit "IAM_USERNAME is not set"
fi

if [ -z "${IAM_PASSWORD}" ]; then
    error_exit "IAM_PASSWORD is not set"
fi

# ============================================================
# 调用 IAM 登录接口
# ============================================================

log "Starting token refresh for user: ${IAM_USERNAME}"

# 调用登录接口
HTTP_CODE=$(curl -s -w "%{http_code}" -o "${TMP_RESPONSE}" \
    -X POST \
    -H "Content-Type: application/json" \
    --max-time 30 \
    -d "{
        \"username\": \"${IAM_USERNAME}\",
        \"password\": \"${IAM_PASSWORD}\"
    }" \
    "${IAM_LOGIN_URL}" || echo "000")

# 检查 HTTP 状态码
if [ "${HTTP_CODE}" != "200" ]; then
    error_exit "IAM login failed with HTTP code: ${HTTP_CODE}, response: $(cat ${TMP_RESPONSE} 2>/dev/null || echo 'N/A')"
fi

# 解析响应，提取 token
# 假设响应格式为：{"token": "xxx", "expires_at": "xxx"} 或 {"access_token": "xxx"}
TOKEN=$(cat "${TMP_RESPONSE}" | grep -o '"token"[[:space:]]*:[[:space:]]*"[^"]*"' | cut -d'"' -f4 || \
        cat "${TMP_RESPONSE}" | grep -o '"access_token"[[:space:]]*:[[:space:]]*"[^"]*"' | cut -d'"' -f4 || \
        jq -r '.token // .access_token // empty' "${TMP_RESPONSE}" 2>/dev/null || echo "")

if [ -z "${TOKEN}" ]; then
    error_exit "Failed to extract token from response: $(cat ${TMP_RESPONSE})"
fi

# ============================================================
# 保存 Token 到文件
# ============================================================

# 确保目录存在
TOKEN_DIR=$(dirname "${TOKEN_FILE}")
mkdir -p "${TOKEN_DIR}"
chmod 755 "${TOKEN_DIR}"

# 保存 Token
echo "${TOKEN}" > "${TOKEN_FILE}"
chmod 600 "${TOKEN_FILE}"
chown root:root "${TOKEN_FILE}"

log "Token refreshed successfully, saved to ${TOKEN_FILE}"

# 可选：记录 token 过期时间（如果响应中包含）
if command -v jq >/dev/null 2>&1; then
    EXPIRES_AT=$(jq -r '.expires_at // .expires_in // empty' "${TMP_RESPONSE}" 2>/dev/null || echo "")
    if [ -n "${EXPIRES_AT}" ]; then
        log "Token expires at: ${EXPIRES_AT}"
    fi
fi

exit 0

