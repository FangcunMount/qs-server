#!/bin/bash

# 数据库基础设施部署脚本 - 问卷收集&量表测评系统
# 该脚本用于部署和管理MySQL、Redis、MongoDB服务

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 脚本目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

# 日志函数
log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] $1${NC}"
}

warn() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] WARNING: $1${NC}"
}

error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: $1${NC}"
}

info() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')] INFO: $1${NC}"
}

# 加载环境变量配置
load_config() {
    local config_file="$PROJECT_ROOT/configs/env/config.env"
    if [ -f "$config_file" ]; then
        log "加载环境变量配置文件: $config_file"
        set -a  # 自动导出所有变量
        source "$config_file"
        set +a
        log "配置加载完成"
    else
        warn "配置文件 $config_file 不存在，使用默认配置"
        warn "您可以从模板文件创建配置："
        warn "  cp $PROJECT_ROOT/configs/env/config.prod.env $config_file"
    fi
}

# 检查Docker和Docker Compose
check_docker() {
    if ! command -v docker &> /dev/null; then
        error "Docker未安装，请先安装Docker"
        exit 1
    fi

    if ! docker compose version &> /dev/null; then
        error "docker compose 未安装或版本过低，请升级到 Docker Desktop 3.6+ 或安装 Docker Compose v2"
        exit 1
    fi

    log "Docker和Docker Compose检查通过"
}

# 创建必要的目录
create_directories() {
    log "创建必要的目录..."
    
    # 创建数据目录
    mkdir -p "${MYSQL_DATA_PATH:-/data/mysql/qs/data}"
    mkdir -p "${MYSQL_LOGS_PATH:-/data/logs/qs/mysql}"
    mkdir -p "${REDIS_DATA_PATH:-/data/redis/qs/data}"
    mkdir -p "${REDIS_LOGS_PATH:-/data/logs/qs/redis}"
    mkdir -p "${MONGODB_DATA_PATH:-/data/mongodb/qs/data}"
    mkdir -p "${MONGODB_CONFIG_PATH:-/data/mongodb/qs/config}"
    mkdir -p "${MONGODB_LOGS_PATH:-/data/logs/qs/mongodb}"
    mkdir -p "${BACKUP_PATH:-$PROJECT_ROOT/backups}"
    
    # 设置权限
    chmod -R 755 "${MYSQL_DATA_PATH:-/data/mysql/qs/data}" 2>/dev/null || true
    chmod -R 755 "${MYSQL_LOGS_PATH:-/data/logs/qs/mysql}" 2>/dev/null || true
    chmod -R 755 "${REDIS_DATA_PATH:-/data/redis/qs/data}" 2>/dev/null || true
    chmod -R 755 "${REDIS_LOGS_PATH:-/data/logs/qs/redis}" 2>/dev/null || true
    chmod -R 755 "${MONGODB_DATA_PATH:-/data/mongodb/qs/data}" 2>/dev/null || true
    chmod -R 755 "${MONGODB_CONFIG_PATH:-/data/mongodb/qs/config}" 2>/dev/null || true
    chmod -R 755 "${MONGODB_LOGS_PATH:-/data/logs/qs/mongodb}" 2>/dev/null || true
    
    log "目录创建完成"
}

# 检查配置文件
check_config_files() {
    log "检查配置文件..."
    
    cd "$PROJECT_ROOT"
    
    # MySQL配置文件
    if [ ! -f "configs/mysql/my.cnf" ]; then
        error "MySQL配置文件不存在: configs/mysql/my.cnf"
        exit 1
    fi
    
    if [ ! -f "configs/mysql/questionnaire.sql" ]; then
        error "MySQL初始化脚本不存在: configs/mysql/questionnaire.sql"
        exit 1
    fi
    
    # Redis配置文件
    if [ ! -f "configs/redis/redis.conf" ]; then
        error "Redis配置文件不存在: configs/redis/redis.conf"
        exit 1
    fi
    
    # MongoDB配置文件
    if [ ! -f "configs/mongodb/mongod.conf" ]; then
        error "MongoDB配置文件不存在: configs/mongodb/mongod.conf"
        exit 1
    fi
    
    # MongoDB初始化脚本
    if [ ! -f "scripts/mongodb/init-mongo.js" ]; then
        error "MongoDB初始化脚本不存在: scripts/mongodb/init-mongo.js"
        exit 1
    fi
    
    if [ ! -f "scripts/mongodb/create-indexes.js" ]; then
        error "MongoDB索引脚本不存在: scripts/mongodb/create-indexes.js"
        exit 1
    fi
    
    log "配置文件检查通过"
}

# 构建镜像
build_images() {
    log "构建自定义镜像..."
    
    cd "$SCRIPT_DIR"
    
    docker compose build --no-cache
    
    log "镜像构建完成"
}

# 启动所有服务
start_all() {
    log "启动所有数据库服务..."
    
    cd "$SCRIPT_DIR"
    
    docker compose up -d
    
    log "所有服务启动完成"
}

# 启动单个服务
start_service() {
    local service="$1"
    if [ -z "$service" ]; then
        error "请指定要启动的服务: mysql, redis, mongodb"
        exit 1
    fi
    
    log "启动 $service 服务..."
    
    cd "$SCRIPT_DIR"
    
    case "$service" in
        mysql|redis|mongodb)
            docker compose up -d "$service"
            ;;
        *)
            error "未知服务: $service"
            exit 1
            ;;
    esac
    
    log "$service 服务启动完成"
}

# 停止所有服务
stop_all() {
    log "停止所有数据库服务..."
    
    cd "$SCRIPT_DIR"
    
    docker compose down
    
    log "所有服务已停止"
}

# 停止单个服务
stop_service() {
    local service="$1"
    if [ -z "$service" ]; then
        error "请指定要停止的服务: mysql, redis, mongodb"
        exit 1
    fi
    
    log "停止 $service 服务..."
    
    cd "$SCRIPT_DIR"
    
    case "$service" in
        mysql|redis|mongodb)
            docker compose stop "$service"
            ;;
        *)
            error "未知服务: $service"
            exit 1
            ;;
    esac
    
    log "$service 服务已停止"
}

# 重启服务
restart_all() {
    log "重启所有数据库服务..."
    stop_all
    start_all
    log "所有服务重启完成"
}

# 重启单个服务
restart_service() {
    local service="$1"
    if [ -z "$service" ]; then
        error "请指定要重启的服务: mysql, redis, mongodb"
        exit 1
    fi
    
    log "重启 $service 服务..."
    stop_service "$service"
    start_service "$service"
    log "$service 服务重启完成"
}

# 查看服务状态
status() {
    info "数据库服务状态:"
    
    cd "$SCRIPT_DIR"
    
    docker compose ps
}

# 查看所有日志
logs_all() {
    info "查看所有服务日志:"
    
    cd "$SCRIPT_DIR"
    
    docker compose logs -f
}

# 查看单个服务日志
logs_service() {
    local service="$1"
    if [ -z "$service" ]; then
        error "请指定要查看日志的服务: mysql, redis, mongodb"
        exit 1
    fi
    
    info "查看 $service 服务日志:"
    
    cd "$SCRIPT_DIR"
    
    docker compose logs -f "$service"
}

# 进入容器
enter_container() {
    local service="$1"
    if [ -z "$service" ]; then
        error "请指定要进入的容器: mysql, redis, mongodb"
        exit 1
    fi
    
    info "进入 $service 容器..."
    
    case "$service" in
        mysql)
            docker exec -it ${MYSQL_CONTAINER_NAME:-questionnaire-mysql} bash
            ;;
        redis)
            docker exec -it ${REDIS_CONTAINER_NAME:-questionnaire-redis} sh
            ;;
        mongodb)
            docker exec -it ${MONGODB_CONTAINER_NAME:-questionnaire-mongodb} bash
            ;;
        *)
            error "未知服务: $service"
            exit 1
            ;;
    esac
}

# 连接数据库
connect_db() {
    local service="$1"
    if [ -z "$service" ]; then
        error "请指定要连接的数据库: mysql, redis, mongodb"
        exit 1
    fi
    
    info "连接 $service 数据库..."
    
    case "$service" in
        mysql)
            docker exec -it ${MYSQL_CONTAINER_NAME:-questionnaire-mysql} mysql -u ${MYSQL_USER:-qs_app_user} -p${MYSQL_PASSWORD:-qs_app_password_2024} ${MYSQL_DATABASE:-questionnaire_scale}
            ;;
        redis)
            docker exec -it ${REDIS_CONTAINER_NAME:-questionnaire-redis} redis-cli -a ${REDIS_PASSWORD:-questionnaire_redis_2024}
            ;;
        mongodb)
            docker exec -it ${MONGODB_CONTAINER_NAME:-questionnaire-mongodb} mongo -u ${MONGODB_USER:-qs_app_user} -p ${MONGODB_PASSWORD:-qs_app_password_2024} --authenticationDatabase ${MONGODB_DATABASE:-questionnaire_scale} ${MONGODB_DATABASE:-questionnaire_scale}
            ;;
        *)
            error "未知服务: $service"
            exit 1
            ;;
    esac
}

# 备份所有数据库
backup_all() {
    local backup_dir="${BACKUP_PATH:-$PROJECT_ROOT/backups}/$(date +%Y%m%d_%H%M%S)"
    mkdir -p "$backup_dir"
    
    log "备份所有数据库到: $backup_dir"
    
    # 备份MySQL
    if docker ps --format "table {{.Names}}" | grep -q ${MYSQL_CONTAINER_NAME:-questionnaire-mysql}; then
        log "备份MySQL数据库..."
        docker exec ${MYSQL_CONTAINER_NAME:-questionnaire-mysql} mysqldump -u root -p${MYSQL_ROOT_PASSWORD:-questionnaire_root_2024} --all-databases > "$backup_dir/mysql_backup.sql"
    fi
    
    # 备份Redis
    if docker ps --format "table {{.Names}}" | grep -q ${REDIS_CONTAINER_NAME:-questionnaire-redis}; then
        log "备份Redis数据库..."
        docker exec ${REDIS_CONTAINER_NAME:-questionnaire-redis} redis-cli -a ${REDIS_PASSWORD:-questionnaire_redis_2024} --rdb /tmp/redis_backup.rdb
        docker cp ${REDIS_CONTAINER_NAME:-questionnaire-redis}:/tmp/redis_backup.rdb "$backup_dir/redis_backup.rdb"
        docker exec ${REDIS_CONTAINER_NAME:-questionnaire-redis} rm /tmp/redis_backup.rdb
    fi
    
    # 备份MongoDB
    if docker ps --format "table {{.Names}}" | grep -q ${MONGODB_CONTAINER_NAME:-questionnaire-mongodb}; then
        log "备份MongoDB数据库..."
        docker exec ${MONGODB_CONTAINER_NAME:-questionnaire-mongodb} mongodump --host localhost --port ${MONGODB_PORT:-27017} \
            --username ${MONGODB_ROOT_USERNAME:-admin} --password ${MONGODB_ROOT_PASSWORD:-questionnaire_admin_2024} --authenticationDatabase admin \
            --db ${MONGODB_DATABASE:-questionnaire_scale} --out /tmp/mongodb_backup
        docker cp ${MONGODB_CONTAINER_NAME:-questionnaire-mongodb}:/tmp/mongodb_backup "$backup_dir/mongodb_backup"
        docker exec ${MONGODB_CONTAINER_NAME:-questionnaire-mongodb} rm -rf /tmp/mongodb_backup
    fi
    
    log "所有数据库备份完成: $backup_dir"
}

# 清理所有数据
clean_all() {
    warn "这将删除所有数据库数据，请确认操作！"
    read -p "输入 'yes' 确认删除: " confirm
    
    if [ "$confirm" = "yes" ]; then
        log "清理所有数据库数据..."
        
        cd "$SCRIPT_DIR"
        
        docker compose down -v --remove-orphans
        
        # 清理数据目录
        rm -rf "${MYSQL_DATA_PATH:-/data/mysql/qs/data}"/*
        rm -rf "${REDIS_DATA_PATH:-/data/redis/qs/data}"/*
        rm -rf "${MONGODB_DATA_PATH:-/data/mongodb/qs/data}"/*
        rm -rf "${MONGODB_CONFIG_PATH:-/data/mongodb/qs/config}"/*
        rm -rf "${MYSQL_LOGS_PATH:-/data/logs/qs/mysql}"/*
        rm -rf "${REDIS_LOGS_PATH:-/data/logs/qs/redis}"/*
        rm -rf "${MONGODB_LOGS_PATH:-/data/logs/qs/mongodb}"/*
        
        log "所有数据清理完成"
    else
        info "操作已取消"
    fi
}

# 显示访问信息
show_access_info() {
    info "数据库访问信息:"
    echo ""
    echo "MySQL:"
    echo "  - 连接: mysql://${MYSQL_USER:-qs_app_user}:${MYSQL_PASSWORD:-qs_app_password_2024}@${MYSQL_HOST:-localhost}:${MYSQL_PORT:-3306}/${MYSQL_DATABASE:-questionnaire_scale}"
    echo "  - 根用户: root / ${MYSQL_ROOT_PASSWORD:-questionnaire_root_2024}"
    echo ""
    echo "Redis:"
    echo "  - 连接: redis://${REDIS_HOST:-localhost}:${REDIS_PORT:-6379}"
    echo "  - 密码: ${REDIS_PASSWORD:-questionnaire_redis_2024}"
    echo ""
    echo "MongoDB:"
    echo "  - 连接: mongodb://${MONGODB_USER:-qs_app_user}:${MONGODB_PASSWORD:-qs_app_password_2024}@${MONGODB_HOST:-localhost}:${MONGODB_PORT:-27017}/${MONGODB_DATABASE:-questionnaire_scale}"
    echo "  - 管理员: ${MONGODB_ROOT_USERNAME:-admin} / ${MONGODB_ROOT_PASSWORD:-questionnaire_admin_2024}"
    echo ""
}

# 显示帮助信息
show_help() {
    echo "数据库基础设施部署脚本 - 问卷收集&量表测评系统"
    echo ""
    echo "用法: $0 [命令] [选项]"
    echo ""
    echo "全局命令:"
    echo "  deploy          - 完整部署所有数据库服务"
    echo "  build           - 构建自定义镜像"
    echo "  start           - 启动所有服务"
    echo "  stop            - 停止所有服务"
    echo "  restart         - 重启所有服务"
    echo "  status          - 查看所有服务状态"
    echo "  logs            - 查看所有服务日志"
    echo "  backup          - 备份所有数据库"
    echo "  clean           - 清理所有数据"
    echo "  info            - 显示访问信息"
    echo ""
    echo "单服务命令:"
    echo "  start <service>     - 启动指定服务 (mysql|redis|mongodb)"
    echo "  stop <service>      - 停止指定服务"
    echo "  restart <service>   - 重启指定服务"
    echo "  logs <service>      - 查看指定服务日志"
    echo "  shell <service>     - 进入指定容器"
    echo "  connect <service>   - 连接指定数据库"
    echo ""
    echo "示例:"
    echo "  $0 deploy              # 完整部署"
    echo "  $0 start mysql         # 只启动MySQL"
    echo "  $0 connect redis       # 连接Redis"
    echo "  $0 logs mongodb        # 查看MongoDB日志"
}

# 完整部署
deploy() {
    log "开始部署数据库基础设施..."
    load_config
    check_docker
    create_directories
    check_config_files
    build_images
    start_all
    
    sleep 15  # 等待服务启动
    
    log "验证服务状态..."
    status
    
    log "数据库基础设施部署完成！"
    show_access_info
}

# 主函数
main() {
    case "${1:-help}" in
        deploy)
            deploy
            ;;
        build)
            load_config
            check_docker
            check_config_files
            build_images
            ;;
        start)
            load_config
            if [ -n "$2" ]; then
                start_service "$2"
            else
                create_directories
                start_all
            fi
            ;;
        stop)
            load_config
            if [ -n "$2" ]; then
                stop_service "$2"
            else
                stop_all
            fi
            ;;
        restart)
            load_config
            if [ -n "$2" ]; then
                restart_service "$2"
            else
                restart_all
            fi
            ;;
        status)
            status
            ;;
        logs)
            if [ -n "$2" ]; then
                logs_service "$2"
            else
                logs_all
            fi
            ;;
        shell)
            load_config
            enter_container "$2"
            ;;
        connect)
            load_config
            connect_db "$2"
            ;;
        backup)
            load_config
            backup_all
            ;;
        clean)
            load_config
            clean_all
            ;;
        info)
            load_config
            show_access_info
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            error "未知命令: $1"
            show_help
            exit 1
            ;;
    esac
}

# 执行主函数
main "$@" 