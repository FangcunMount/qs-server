#!/usr/bin/env bash

# ===================================
# 基础设施检查脚本
# ===================================
# 用途：检查 MySQL、Redis、MongoDB、NSQ 等基础组件是否就绪
# 使用：./scripts/check-infra.sh [component]
#       component: mysql | redis | mongodb | nsq | all (默认)

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 默认配置（优先使用环境变量）
MYSQL_HOST="${MYSQL_HOST:-127.0.0.1}"
MYSQL_PORT="${MYSQL_PORT:-3306}"
MYSQL_USER="${MYSQL_USER:-root}"
MYSQL_PASSWORD="${MYSQL_PASSWORD:-dev_root_123}"

# Redis Cache 实例（端口 6379）
REDIS_CACHE_HOST="${REDIS_CACHE_HOST:-127.0.0.1}"
REDIS_CACHE_PORT="${REDIS_CACHE_PORT:-6379}"
REDIS_CACHE_USER="${REDIS_CACHE_USERNAME:-${REDIS_CACHE_USER:-app}}"
REDIS_CACHE_PASSWORD="${REDIS_CACHE_PASSWORD:-dev_app_123}"

# Redis Store 实例（端口 6380）
REDIS_STORE_HOST="${REDIS_STORE_HOST:-127.0.0.1}"
REDIS_STORE_PORT="${REDIS_STORE_PORT:-6380}"
REDIS_STORE_USER="${REDIS_STORE_USERNAME:-${REDIS_STORE_USER:-app}}"
REDIS_STORE_PASSWORD="${REDIS_STORE_PASSWORD:-dev_app_123}"

# Redis 旧版兼容（用于 check-redis 命令）
REDIS_HOST="${REDIS_HOST:-${REDIS_CACHE_HOST}}"
REDIS_PORT="${REDIS_PORT:-${REDIS_CACHE_PORT}}"
REDIS_USER="${REDIS_USERNAME:-${REDIS_USER:-${REDIS_CACHE_USER}}}"
REDIS_PASSWORD="${REDIS_PASSWORD:-${REDIS_CACHE_PASSWORD}}"

# MongoDB
MONGODB_HOST="${MONGO_HOST:-${MONGODB_HOST:-127.0.0.1}}"
MONGODB_PORT="${MONGO_PORT:-${MONGODB_PORT:-27017}}"
MONGODB_USER="${MONGO_USER:-${MONGODB_USER:-}}"
MONGODB_PASSWORD="${MONGO_PASSWORD:-${MONGODB_PASSWORD:-}}"

# NSQ
NSQ_LOOKUP_HOST="${NSQ_HOST:-${NSQ_LOOKUP_HOST:-127.0.0.1}}"
NSQ_LOOKUP_PORT="${NSQLOOKUPD_HTTP_PORT:-${NSQ_LOOKUP_PORT:-4161}}"
NSQ_D_HOST="${NSQ_HOST:-${NSQ_D_HOST:-127.0.0.1}}"
NSQ_D_PORT="${NSQD_HTTP_PORT:-${NSQ_D_PORT:-4151}}"

# 超时时间（秒）
TIMEOUT="${CHECK_TIMEOUT:-5}"

# 检查 timeout 命令是否存在（Linux 有，macOS 需要用 gtimeout）
if command -v timeout &> /dev/null; then
    TIMEOUT_CMD="timeout $TIMEOUT"
elif command -v gtimeout &> /dev/null; then
    TIMEOUT_CMD="gtimeout $TIMEOUT"
else
    # macOS 没有 timeout 命令，使用 perl 实现简单的超时
    TIMEOUT_CMD=""
fi

# ===================================
# 辅助函数
# ===================================

print_status() {
    local component="$1"
    local status="$2"
    local message="$3"
    
    printf "%-15s " "$component"
    if [ "$status" = "ok" ]; then
        printf "[${GREEN}✓${NC}] "
    elif [ "$status" = "fail" ]; then
        printf "[${RED}✗${NC}] "
    elif [ "$status" = "warn" ]; then
        printf "[${YELLOW}!${NC}] "
    else
        printf "[${BLUE}?${NC}] "
    fi
    echo "$message"
}

# ===================================
# 检查函数
# ===================================

check_mysql() {
    printf "%-15s " "MySQL"
    
    # 检查 mysql 命令是否存在
    if ! command -v mysql &> /dev/null; then
        printf "[${YELLOW}!${NC}] mysql 命令未安装，跳过连接测试\n"
        return 1
    fi
    
    # 尝试连接 MySQL（--connect-timeout 已经提供超时功能）
    if mysql -h"$MYSQL_HOST" -P"$MYSQL_PORT" -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" \
        --connect-timeout="$TIMEOUT" -e "SELECT 1;" &> /dev/null; then
        printf "[${GREEN}✓${NC}] 运行正常 (${MYSQL_HOST}:${MYSQL_PORT})\n"
        return 0
    else
        printf "[${RED}✗${NC}] 连接失败 (${MYSQL_HOST}:${MYSQL_PORT})\n"
        return 1
    fi
}

check_redis() {
    # 检查 redis-cli 命令是否存在
    if ! command -v redis-cli &> /dev/null; then
        printf "%-15s " "Redis Cache"
        printf "[${YELLOW}!${NC}] redis-cli 命令未安装，跳过连接测试\n"
        printf "%-15s " "Redis Store"
        printf "[${YELLOW}!${NC}] redis-cli 命令未安装，跳过连接测试\n"
        return 1
    fi
    
    local failed=0
    
    # 检查 Redis Cache 实例（端口 6379）
    printf "%-15s " "Redis Cache"
    local cache_cmd="redis-cli -h $REDIS_CACHE_HOST -p $REDIS_CACHE_PORT"
    if [ -n "$REDIS_CACHE_PASSWORD" ]; then
        if [ -n "$REDIS_CACHE_USER" ]; then
            cache_cmd="$cache_cmd --user $REDIS_CACHE_USER --pass $REDIS_CACHE_PASSWORD --no-auth-warning"
        else
            cache_cmd="$cache_cmd -a $REDIS_CACHE_PASSWORD --no-auth-warning"
        fi
    fi
    
    # redis-cli 默认有 5 秒超时，不需要额外的 timeout 命令
    if $cache_cmd PING &> /dev/null; then
        printf "[${GREEN}✓${NC}] 运行正常 (${REDIS_CACHE_HOST}:${REDIS_CACHE_PORT})\n"
    else
        printf "[${RED}✗${NC}] 连接失败 (${REDIS_CACHE_HOST}:${REDIS_CACHE_PORT})\n"
        failed=1
    fi
    
    # 检查 Redis Store 实例（端口 6380）
    printf "%-15s " "Redis Store"
    local store_cmd="redis-cli -h $REDIS_STORE_HOST -p $REDIS_STORE_PORT"
    if [ -n "$REDIS_STORE_PASSWORD" ]; then
        if [ -n "$REDIS_STORE_USER" ]; then
            store_cmd="$store_cmd --user $REDIS_STORE_USER --pass $REDIS_STORE_PASSWORD --no-auth-warning"
        else
            store_cmd="$store_cmd -a $REDIS_STORE_PASSWORD --no-auth-warning"
        fi
    fi
    
    # redis-cli 默认有 5 秒超时，不需要额外的 timeout 命令
    if $store_cmd PING &> /dev/null; then
        printf "[${GREEN}✓${NC}] 运行正常 (${REDIS_STORE_HOST}:${REDIS_STORE_PORT})\n"
    else
        printf "[${RED}✗${NC}] 连接失败 (${REDIS_STORE_HOST}:${REDIS_STORE_PORT})\n"
        failed=1
    fi
    
    return $failed
}

check_mongodb() {
    printf "%-15s " "MongoDB"
    
    # 检查 mongosh 或 mongo 命令是否存在
    local mongo_cmd=""
    if command -v mongosh &> /dev/null; then
        mongo_cmd="mongosh"
    elif command -v mongo &> /dev/null; then
        mongo_cmd="mongo"
    else
        printf "[${YELLOW}!${NC}] mongosh/mongo 命令未安装，跳过连接测试\n"
        return 1
    fi
    
    # 构建连接字符串
    local mongo_uri="mongodb://"
    if [ -n "$MONGODB_USER" ] && [ -n "$MONGODB_PASSWORD" ]; then
        mongo_uri="${mongo_uri}${MONGODB_USER}:${MONGODB_PASSWORD}@"
    fi
    mongo_uri="${mongo_uri}${MONGODB_HOST}:${MONGODB_PORT}"
    
    # 尝试连接 MongoDB（mongosh/mongo 自带超时机制）
    if $mongo_cmd "$mongo_uri" --eval "db.adminCommand('ping')" &> /dev/null; then
        printf "[${GREEN}✓${NC}] 运行正常 (${MONGODB_HOST}:${MONGODB_PORT})\n"
        return 0
    else
        printf "[${RED}✗${NC}] 连接失败 (${MONGODB_HOST}:${MONGODB_PORT})\n"
        return 1
    fi
}

check_nsq() {
    printf "%-15s " "NSQ"
    
    # 检查 nsqlookupd
    if ! curl -sf --connect-timeout "$TIMEOUT" "http://${NSQ_LOOKUP_HOST}:${NSQ_LOOKUP_PORT}/ping" &> /dev/null; then
        printf "[${RED}✗${NC}] nsqlookupd 连接失败 (${NSQ_LOOKUP_HOST}:${NSQ_LOOKUP_PORT})\n"
        return 1
    fi
    
    # 检查 nsqd
    if ! curl -sf --connect-timeout "$TIMEOUT" "http://${NSQ_D_HOST}:${NSQ_D_PORT}/ping" &> /dev/null; then
        printf "[${RED}✗${NC}] nsqd 连接失败 (${NSQ_D_HOST}:${NSQ_D_PORT})\n"
        return 1
    fi
    
    printf "[${GREEN}✓${NC}] 运行正常 (lookupd:${NSQ_LOOKUP_PORT}, nsqd:${NSQ_D_PORT})\n"
    return 0
}

# ===================================
# 主函数
# ===================================

show_usage() {
    cat << EOF
用法: $0 [COMPONENT]

检查基础设施组件是否就绪

COMPONENT:
    mysql       检查 MySQL 数据库
    redis       检查 Redis 双实例（Cache + Store）
    mongodb     检查 MongoDB 数据库
    nsq         检查 NSQ 消息队列
    all         检查所有组件 (默认)

环境变量:
    MySQL:
        MYSQL_HOST, MYSQL_PORT, MYSQL_USER, MYSQL_PASSWORD
    
    Redis (双实例架构，支持 ACL 认证):
        REDIS_CACHE_HOST, REDIS_CACHE_PORT, REDIS_CACHE_USERNAME, REDIS_CACHE_PASSWORD  # Cache 实例
        REDIS_STORE_HOST, REDIS_STORE_PORT, REDIS_STORE_USERNAME, REDIS_STORE_PASSWORD  # Store 实例
    
    MongoDB:
        MONGODB_HOST, MONGODB_PORT, MONGODB_USER, MONGODB_PASSWORD
    
    NSQ:
        NSQ_LOOKUP_HOST, NSQ_LOOKUP_PORT, NSQ_D_HOST, NSQ_D_PORT
    
    其他:
        CHECK_TIMEOUT (默认: 5 秒)

示例:
    $0              # 检查所有组件
    $0 mysql        # 只检查 MySQL
    $0 redis        # 检查 Redis 双实例
    
    # 使用自定义配置
    MYSQL_HOST=192.168.1.100 $0 mysql
    
    # 检查自定义 Redis Store 实例
    REDIS_STORE_HOST=192.168.1.101 REDIS_STORE_PORT=6380 $0 redis

退出码:
    0 - 所有检查通过
    1 - 至少有一个检查失败
EOF
}

main() {
    local component="${1:-all}"
    local failed=0
    
    # 显示帮助
    if [ "$component" = "-h" ] || [ "$component" = "--help" ]; then
        show_usage
        exit 0
    fi
    
    echo -e "${BLUE}===========================================\n检查基础设施组件\n===========================================${NC}\n"
    
    case "$component" in
        mysql)
            check_mysql || failed=1
            ;;
        redis)
            check_redis || failed=1
            ;;
        mongodb)
            check_mongodb || failed=1
            ;;
        nsq)
            check_nsq || failed=1
            ;;
        all)
            check_mysql || failed=1
            check_redis || failed=1
            check_mongodb || failed=1
            check_nsq || failed=1
            ;;
        *)
            echo -e "${RED}错误: 未知的组件 '$component'${NC}\n"
            show_usage
            exit 1
            ;;
    esac
    
    echo ""
    if [ $failed -eq 0 ]; then
        echo -e "${GREEN}✓ 所有检查通过${NC}"
        return 0
    else
        echo -e "${RED}✗ 部分检查失败，请确保基础设施已正确启动${NC}"
        echo -e "${YELLOW}提示: 开发环境请确保 infra 项目已启动${NC}"
        return 1
    fi
}

# 执行主函数
main "$@"
