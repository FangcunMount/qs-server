#!/bin/bash
# ============================================================
# QS API 调用脚本（通用模板）
# ============================================================
# 用途：自动获取 Token 并执行 API 调用
# 维护人：DevOps Team
# 最后更新：2025-01-XX
# 
# 使用说明：
# 1. 复制此文件到 /usr/local/bin/qs-api-call.sh
# 2. 设置执行权限：chmod +x /usr/local/bin/qs-api-call.sh
# 3. 在 crontab 中调用：qs-api-call.sh <endpoint> [log_file]
# 
# 示例：
#   qs-api-call.sh /api/v1/statistics/sync/daily
#   qs-api-call.sh /api/v1/statistics/sync/daily /data/logs/crontab/sync-daily.log
# ============================================================

set -euo pipefail

# ============================================================
# 配置变量（从环境变量读取）
# ============================================================

# Token 文件路径
TOKEN_FILE="${TOKEN_FILE:-/etc/qs-server/internal-token}"

# Token 刷新脚本路径
REFRESH_TOKEN_SCRIPT="${REFRESH_TOKEN_SCRIPT:-/usr/local/bin/qs-refresh-token.sh}"

# API 基础 URL
API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"

# 默认日志文件
DEFAULT_LOG_DIR="/data/logs/crontab"

# 超时时间（秒）
TIMEOUT="${TIMEOUT:-300}"

# ============================================================
# 参数解析
# ============================================================

if [ $# -lt 1 ]; then
    echo "Usage: $0 <endpoint> [log_file]" >&2
    echo "Example: $0 /api/v1/statistics/sync/daily" >&2
    exit 1
fi

ENDPOINT="$1"
LOG_FILE="${2:-${DEFAULT_LOG_DIR}/api-call.log}"

# 确保日志目录存在
LOG_DIR=$(dirname "${LOG_FILE}")
mkdir -p "${LOG_DIR}"

# ============================================================
# 函数定义
# ============================================================

# 记录日志
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "${LOG_FILE}"
}

# 错误处理
error_exit() {
    log "ERROR: $*"
    exit 1
}

# ============================================================
# 获取 Token（如果不存在则自动刷新）
# ============================================================

# 检查 Token 文件是否存在或为空
if [ ! -f "${TOKEN_FILE}" ] || [ ! -s "${TOKEN_FILE}" ]; then
    log "Token file not found or empty, refreshing token..."
    
    # 尝试调用刷新脚本
    # 注意：refresh-token.sh 需要从环境变量读取 IAM_USERNAME 和 IAM_PASSWORD
    # 这些环境变量应该在 crontab 配置文件中定义，cron 会自动传递给子进程
    if [ -f "${REFRESH_TOKEN_SCRIPT}" ] && [ -x "${REFRESH_TOKEN_SCRIPT}" ]; then
        # 检查必要的环境变量是否设置
        if [ -z "${IAM_USERNAME:-}" ] || [ -z "${IAM_PASSWORD:-}" ]; then
            error_exit "IAM_USERNAME and IAM_PASSWORD must be set in crontab environment variables"
        fi
        # 调用刷新脚本（环境变量会自动传递）
        if "${REFRESH_TOKEN_SCRIPT}" >> "${LOG_FILE}" 2>&1; then
            log "Token refreshed successfully"
        else
            error_exit "Failed to refresh token. Please check ${REFRESH_TOKEN_SCRIPT} and ensure IAM_USERNAME and IAM_PASSWORD are set in crontab."
        fi
    else
        error_exit "Token file not found and refresh script not available: ${REFRESH_TOKEN_SCRIPT}"
    fi
fi

# 读取 Token
INTERNAL_TOKEN=$(cat "${TOKEN_FILE}" 2>/dev/null || echo "")

if [ -z "${INTERNAL_TOKEN}" ]; then
    error_exit "Token file is empty after refresh: ${TOKEN_FILE}"
fi

# ============================================================
# 执行 API 调用
# ============================================================

log "Calling API: ${API_BASE_URL}${ENDPOINT}"

# 执行 curl 请求
HTTP_CODE=$(curl -s -w "%{http_code}" -o /tmp/qs-api-response-$$.txt \
    -X POST \
    -H "Authorization: Bearer ${INTERNAL_TOKEN}" \
    -H "Content-Type: application/json" \
    --max-time "${TIMEOUT}" \
    "${API_BASE_URL}${ENDPOINT}" || echo "000")

# 检查 HTTP 状态码
if [ "${HTTP_CODE}" = "200" ] || [ "${HTTP_CODE}" = "201" ]; then
    log "API call successful (HTTP ${HTTP_CODE})"
    RESPONSE=$(cat /tmp/qs-api-response-$$.txt 2>/dev/null || echo "")
    if [ -n "${RESPONSE}" ]; then
        log "Response: ${RESPONSE}"
    fi
    rm -f /tmp/qs-api-response-$$.txt
    exit 0
else
    ERROR_RESPONSE=$(cat /tmp/qs-api-response-$$.txt 2>/dev/null || echo "N/A")
    error_exit "API call failed with HTTP code: ${HTTP_CODE}, response: ${ERROR_RESPONSE}"
fi

