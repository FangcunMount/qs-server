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

# IAM API 基础 URL（不包含路径）
IAM_BASE_URL="${IAM_BASE_URL:-https://iam.yangshujie.com/api/v1}"

# IAM 登录接口路径
IAM_LOGIN_ENDPOINT="/authn/login"

# IAM 用户名和密码（从环境变量或配置文件读取）
IAM_USERNAME="${IAM_USERNAME:-}"
IAM_PASSWORD="${IAM_PASSWORD:-}"

# 设备 ID（可选，用于标识登录设备）
DEVICE_ID="${DEVICE_ID:-qs-scheduler-$(hostname)}"

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

# 构建完整的登录 URL
IAM_LOGIN_URL="${IAM_BASE_URL}${IAM_LOGIN_ENDPOINT}"

# 构建请求体
# 根据 API 文档，LoginRequest 格式为：
# {
#   "method": "password",
#   "credentials": [数组],
#   "device_id": "可选"
# }
# 注意：API 文档中 credentials 定义为 array of integer，但实际实现可能接受字符串数组
# 如果实际 API 需要 credential ID（整数），可能需要：
# 1. 先通过其他接口查询 credential ID
# 2. 或者使用不同的请求格式
# 当前实现使用用户名和密码字符串，如果 API 不接受，请根据实际情况调整
REQUEST_BODY=$(cat <<EOF
{
  "method": "password",
  "credentials": ["${IAM_USERNAME}", "${IAM_PASSWORD}"],
  "device_id": "${DEVICE_ID}"
}
EOF
)

# 调用登录接口
HTTP_CODE=$(curl -s -w "%{http_code}" -o "${TMP_RESPONSE}" \
    -X POST \
    -H "Content-Type: application/json" \
    --max-time 30 \
    -d "${REQUEST_BODY}" \
    "${IAM_LOGIN_URL}" || echo "000")

# 检查 HTTP 状态码
if [ "${HTTP_CODE}" != "200" ]; then
    ERROR_RESPONSE=$(cat "${TMP_RESPONSE}" 2>/dev/null || echo "N/A")
    error_exit "IAM login failed with HTTP code: ${HTTP_CODE}, response: ${ERROR_RESPONSE}"
fi

# 解析响应，提取 access_token
# 根据 API 文档，响应格式为 TokenPair：
# {
#   "access_token": "xxx",
#   "refresh_token": "xxx",
#   "token_type": "Bearer",
#   "expires_in": 3600
# }
if command -v jq >/dev/null 2>&1; then
    TOKEN=$(jq -r '.access_token // empty' "${TMP_RESPONSE}" 2>/dev/null || echo "")
else
    # 如果没有 jq，使用 grep 和 cut 提取
    TOKEN=$(grep -o '"access_token"[[:space:]]*:[[:space:]]*"[^"]*"' "${TMP_RESPONSE}" | cut -d'"' -f4 || echo "")
fi

if [ -z "${TOKEN}" ]; then
    ERROR_RESPONSE=$(cat "${TMP_RESPONSE}" 2>/dev/null || echo "N/A")
    error_exit "Failed to extract access_token from response: ${ERROR_RESPONSE}"
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

# 记录 token 信息（如果响应中包含）
if command -v jq >/dev/null 2>&1; then
    EXPIRES_IN=$(jq -r '.expires_in // empty' "${TMP_RESPONSE}" 2>/dev/null || echo "")
    TOKEN_TYPE=$(jq -r '.token_type // empty' "${TMP_RESPONSE}" 2>/dev/null || echo "")
    if [ -n "${EXPIRES_IN}" ]; then
        log "Token type: ${TOKEN_TYPE:-Bearer}, expires in: ${EXPIRES_IN} seconds"
    fi
fi

exit 0

