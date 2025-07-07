#!/bin/bash

# 演示 Makefile 功能的脚本
# 这个脚本将展示如何使用 Makefile 管理问卷量表系统的所有服务

set -e

echo "🎯 问卷量表系统 Makefile 功能演示"
echo "========================================"
echo ""

# 函数：打印分隔线
print_separator() {
    echo ""
    echo "----------------------------------------"
    echo "📋 $1"
    echo "----------------------------------------"
}

# 函数：等待用户按键
wait_for_key() {
    echo ""
    echo "按任意键继续..."
    read -n 1 -s
    echo ""
}

# 1. 显示帮助信息
print_separator "1. 显示帮助信息"
echo "运行: make help"
echo ""
make help
wait_for_key

# 2. 检查当前服务状态
print_separator "2. 检查当前服务状态"
echo "运行: make status-all"
echo ""
make status-all
wait_for_key

# 3. 构建所有服务
print_separator "3. 构建所有服务"
echo "运行: make build-all"
echo ""
make build-all
wait_for_key

# 4. 创建必要目录
print_separator "4. 创建必要目录"
echo "运行: make create-dirs"
echo ""
make create-dirs
echo "✅ 目录创建完成"
echo ""
echo "检查创建的目录："
ls -la tmp/pids/ logs/
wait_for_key

# 5. 启动单个服务（apiserver）
print_separator "5. 启动 API 服务器"
echo "运行: make run-apiserver"
echo ""
make run-apiserver
echo ""
echo "等待服务启动..."
sleep 3
make status-apiserver
wait_for_key

# 6. 健康检查
print_separator "6. 健康检查"
echo "运行: make health-check"
echo ""
make health-check
wait_for_key

# 7. 查看日志（前几行）
print_separator "7. 查看 API 服务器日志"
echo "运行: head -20 logs/apiserver.log"
echo ""
if [ -f logs/apiserver.log ]; then
    head -20 logs/apiserver.log
else
    echo "日志文件不存在"
fi
wait_for_key

# 8. 停止服务
print_separator "8. 停止 API 服务器"
echo "运行: make stop-apiserver"
echo ""
make stop-apiserver
echo ""
make status-apiserver
wait_for_key

# 9. 演示完整的服务管理流程
print_separator "9. 完整的服务管理流程演示"
echo "这将演示启动所有服务、查看状态、然后停止所有服务"
echo ""
echo "步骤 1: 启动所有服务"
echo "运行: make run-all"
echo ""
make run-all
wait_for_key

echo "步骤 2: 查看所有服务状态"
echo "运行: make status-all"
echo ""
make status-all
wait_for_key

echo "步骤 3: 进行健康检查"
echo "运行: make health-check"
echo ""
make health-check
wait_for_key

echo "步骤 4: 停止所有服务"
echo "运行: make stop-all"
echo ""
make stop-all
wait_for_key

# 10. 清理演示
print_separator "10. 清理演示"
echo "运行: make clean"
echo ""
make clean
wait_for_key

# 总结
print_separator "演示完成"
echo "🎉 Makefile 功能演示完成！"
echo ""
echo "主要功能总结："
echo "✅ 构建管理 - 可以构建单个或所有服务"
echo "✅ 服务管理 - 启动、停止、重启服务"
echo "✅ 状态监控 - 查看服务状态和健康检查"
echo "✅ 日志管理 - 查看实时日志"
echo "✅ 进程管理 - 使用 PID 文件管理进程"
echo "✅ 清理功能 - 自动清理构建文件和进程"
echo ""
echo "更多详细信息请查看: docs/Makefile使用指南.md"
echo ""
echo "常用命令："
echo "  make help           - 查看所有可用命令"
echo "  make build-all      - 构建所有服务"
echo "  make run-all        - 启动所有服务"
echo "  make status-all     - 查看服务状态"
echo "  make health-check   - 健康检查"
echo "  make logs-all       - 查看所有日志"
echo "  make stop-all       - 停止所有服务"
echo "  make clean          - 清理所有文件"
echo ""
echo "🚀 开始使用 Makefile 管理你的问卷量表系统吧！" 