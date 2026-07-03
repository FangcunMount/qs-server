# ============================================================================
# Makefile for QS-Server
# ============================================================================
# 项目：qs-server - 问卷量表系统
# 架构：六边形架构 + DDD + CQRS
# ============================================================================

.DEFAULT_GOAL := help

# ============================================================================
# 变量定义
# ============================================================================

# 项目信息
PROJECT_NAME := qs-server
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.0.0-dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")

# Go 相关
GO := go
GO_BUILD := $(GO) build
GO_TEST := $(GO) test
GO_LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"
GOLANGCI_LINT_VERSION := v2.1.6
GOVULNCHECK_VERSION := v1.1.4
GOSEC_VERSION := v2.22.3
GO_VERSION := 1.25.9
GO_TOOLCHAIN := go$(GO_VERSION)
GOLANGCI_LINT_BIN = $(CURDIR)/$(BIN_DIR)/golangci-lint

# 目录结构
BIN_DIR := bin
TMP_DIR := tmp
PID_DIR := $(TMP_DIR)/pids
LOG_DIR := logs
COVERAGE_DIR := coverage
SECURITY_DIR := $(TMP_DIR)/security
MAINTAINABILITY_DIR := $(TMP_DIR)/maintainability
QUALITY_DIR := scripts/quality
CD_SCRIPT_DIR := scripts/cd
PERF_DIR := tmp/perf
PERF_SCRIPT_DIR := scripts/perf
PERF_CONFIG_FILE := $(CURDIR)/$(PERF_DIR)/qs-perf.config.json
PERF_K6_SCRIPT := $(PERF_SCRIPT_DIR)/k6/mixed.js
QPS_PROFILE ?= pretest_60

GOSEC_BASE_ARGS := -exclude-generated \
	-exclude-dir=internal/apiserver/docs \
	-exclude-dir=internal/collection-server/docs \
	-exclude-dir=internal/apiserver/interface/grpc/proto \
	-severity=medium \
	-confidence=medium

# 服务配置
APISERVER_BIN := $(BIN_DIR)/qs-apiserver
COLLECTION_BIN := $(BIN_DIR)/collection-server
WORKER_BIN := $(BIN_DIR)/qs-worker

# 根据 ENV 选择配置与端口（默认 dev）
ifeq ($(ENV),prod)
  APISERVER_CONFIG := configs/apiserver.prod.yaml
  COLLECTION_CONFIG := configs/collection-server.prod.yaml
  WORKER_CONFIG := configs/worker.prod.yaml
  # 宿主机端口为避免与已部署的 IAM 冲突，统一后移一位
  APISERVER_PORT := 8081
  COLLECTION_PORT := 8082
else
  APISERVER_CONFIG := configs/apiserver.dev.yaml
  COLLECTION_CONFIG := configs/collection-server.dev.yaml
  WORKER_CONFIG := configs/worker.dev.yaml
  APISERVER_PORT := 18082
  COLLECTION_PORT := 18083
endif

# 环境配置
ENV ?= dev

# 颜色输出
COLOR_RESET := \033[0m
COLOR_BOLD := \033[1m
COLOR_GREEN := \033[32m
COLOR_YELLOW := \033[33m
COLOR_BLUE := \033[34m
COLOR_CYAN := \033[36m
COLOR_RED := \033[31m

# ============================================================================
# .PHONY 声明
# ============================================================================

.PHONY: help version debug
.PHONY: build build-all build-apiserver build-collection build-worker clean
.PHONY: run run-all run-apiserver run-collection run-worker
.PHONY: stop stop-all stop-apiserver stop-collection stop-worker
.PHONY: restart restart-all restart-apiserver restart-collection restart-worker
.PHONY: status status-all status-apiserver status-collection status-worker
.PHONY: logs logs-all logs-apiserver logs-collection logs-worker
.PHONY: health health-check
.PHONY: check-infra check-mysql check-redis check-mongodb check-nsq
.PHONY: dev dev-apiserver dev-collection dev-worker dev-stop dev-status dev-logs
.PHONY: test test-unit test-coverage test-race test-bench test-all
.PHONY: test-submit test-message-queue
.PHONY: lint lint-boundaries fmt fmt-check maintainability-lint maintainability-lint-ci tier1-test-policy ensure-golangci-lint
.PHONY: verify vuln
.PHONY: security security-govulncheck security-govulncheck-ci security-gosec security-gosec-ci
.PHONY: deps deps-download deps-tidy deps-verify deps-check
.PHONY: install-tools install-air install-golangci-lint install-security-tools install-govulncheck install-gosec create-dirs
.PHONY: up down re st log
.PHONY: quick-start
.PHONY: docs-swagger docs-rest docs-hygiene docs-verify
.PHONY: cd-image cd-package cd-remote-deploy cd-validate cd-plan cd-export-image
.PHONY: perf-init perf-ensure-config perf-tokens perf-tokens-collection perf-tokens-apiserver
.PHONY: perf-preflight perf-check-k6 perf-k6 perf-smoke perf-pretest60 perf-pretest120 perf-pretest120-submit-only perf-pretest120-balanced
.PHONY: perf-mixed140 perf-mixed140-submit24 perf-mixed160 perf-mixed180 perf-mixed200 perf-mixed220 perf-mixed240 perf-mixed240-models perf-mixed280 perf-mixed280-models perf-mixed280-models-short-report perf-mixed280-models-ws perf-special-report-short-poll perf-special-report-long-poll perf-mixed300 perf-mixed300-http perf-mixed300-http-query perf-mixed300-http-query-nostats perf-stats-isolate29 perf-stats-warmup perf-mixed300probe
.PHONY: perf-model-smoke perf-outbox120 perf-personality60 perf-mixed300-models perf-mixed300-scanner
.PHONY: perf-diag-report120 perf-diag-query120 perf-diag-submit120 perf-diag-query-submit120 perf-sync-profiles perf-sync-vusers perf-verify

# ============================================================================
# 帮助信息
# ============================================================================

help: ## 显示帮助信息
	@echo "$(COLOR_BOLD)$(COLOR_CYAN)======================================"
	@echo "  问卷量表系统 - 构建和管理工具"
	@echo "======================================$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)项目信息:$(COLOR_RESET)"
	@echo "  版本:     $(COLOR_GREEN)$(VERSION)$(COLOR_RESET)"
	@echo "  分支:     $(COLOR_GREEN)$(GIT_BRANCH)$(COLOR_RESET)"
	@echo "  提交:     $(COLOR_GREEN)$(GIT_COMMIT)$(COLOR_RESET)"
	@echo "  环境:     $(COLOR_GREEN)$(ENV)$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)📦 构建命令:$(COLOR_RESET)"
	@grep -E '^build.*:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(COLOR_CYAN)%-25s$(COLOR_RESET) %s\n", $$1, $$2}'
	@echo ""
	@echo "$(COLOR_BOLD)🚀 服务管理:$(COLOR_RESET)"
	@grep -E '^(run|stop|restart|status|logs|health).*:.*?## .*$$' $(MAKEFILE_LIST) | grep -v "dev" | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(COLOR_CYAN)%-25s$(COLOR_RESET) %s\n", $$1, $$2}'
	@echo ""
	@echo "$(COLOR_BOLD)🔍 环境检查:$(COLOR_RESET)"
	@grep -E '^check.*:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(COLOR_CYAN)%-25s$(COLOR_RESET) %s\n", $$1, $$2}'
	@echo ""
	@echo "$(COLOR_BOLD)🛠️  开发工具:$(COLOR_RESET)"
	@grep -E '^(dev|test|lint|fmt|security).*:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(COLOR_CYAN)%-25s$(COLOR_RESET) %s\n", $$1, $$2}'
	@echo ""
	@echo "$(COLOR_BOLD)📚 其他命令:$(COLOR_RESET)"
	@grep -E '^(deps|install|clean|version|debug|up|down|quick|docs).*:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(COLOR_CYAN)%-25s$(COLOR_RESET) %s\n", $$1, $$2}'
	@echo ""
	@echo "$(COLOR_BOLD)📈 K6 压测:$(COLOR_RESET)"
	@grep -E '^perf-.*:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(COLOR_CYAN)%-25s$(COLOR_RESET) %s\n", $$1, $$2}'
	@echo ""
	@echo "$(COLOR_BOLD)🚢 CD 命令:$(COLOR_RESET)"
	@grep -E '^cd-.*:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(COLOR_CYAN)%-25s$(COLOR_RESET) %s\n", $$1, $$2}'
	@echo ""

docs-swagger: ## 生成 swagger 文档 (apiserver & collection)
	@command -v swag >/dev/null 2>&1 || { echo "swag 未安装，请先执行: go install github.com/swaggo/swag/cmd/swag@v1.16.4"; exit 1; }
	swag init --parseInternal -g apiserver.go -d cmd/qs-apiserver,internal/apiserver,internal/pkg,pkg -o internal/apiserver/docs
	swag init --parseInternal --parseDependency -g main.go -d cmd/collection-server,internal/collection-server,pkg -o internal/collection-server/docs

docs-rest: docs-swagger ## 从 swagger 生成 api/rest 的 OAS 3.1 摘要
	@python -c "import yaml" 2>/dev/null || { echo "缺少 PyYAML，先执行: python -m pip install --quiet pyyaml"; exit 1; }
	python scripts/generate_rest_from_swagger.py --swagger internal/apiserver/docs/swagger.json --output api/rest/apiserver.yaml --server http://localhost:18082 --server https://qs.fangcunmount.cn
	python scripts/generate_rest_from_swagger.py --swagger internal/collection-server/docs/swagger.json --output api/rest/collection.yaml --server http://localhost:18083 --server https://collect.fangcunmount.cn

docs-hygiene: ## 检查现行 docs/ 的链接、锚点与章节编号
	python scripts/check_docs_hygiene.py

docs-verify: docs-rest docs-hygiene ## 对比 api/rest 与 swagger，并检查现行文档卫生
	python scripts/compare_api_docs.py

# ============================================================================
# K6 混合场景压测（详见 docs/04-接口与运维/11-300QPS混合场景压测SOP.md）
# ============================================================================

perf-init: ## 初始化 tmp/perf（不覆盖已有配置与凭据）
	@mkdir -p $(PERF_DIR)
	@if [ ! -f $(PERF_DIR)/qs-perf.config.json ]; then \
		cp $(PERF_SCRIPT_DIR)/qs-perf.config.example.json $(PERF_DIR)/qs-perf.config.json; \
		echo "$(COLOR_GREEN)✅ 已创建 $(PERF_DIR)/qs-perf.config.json$(COLOR_RESET)"; \
	else \
		echo "$(COLOR_YELLOW)ℹ️  保留已有 $(PERF_DIR)/qs-perf.config.json$(COLOR_RESET)"; \
	fi
	@if [ ! -f $(PERF_DIR)/iam-users.json ]; then \
		cp $(PERF_SCRIPT_DIR)/iam-users.example.json $(PERF_DIR)/iam-users.json; \
		echo "$(COLOR_YELLOW)⚠️  请编辑 $(PERF_DIR)/iam-users.json 填入真实凭据后再 make perf-tokens$(COLOR_RESET)"; \
	else \
		echo "$(COLOR_YELLOW)ℹ️  保留已有 $(PERF_DIR)/iam-users.json$(COLOR_RESET)"; \
	fi
	@$(MAKE) perf-ensure-config

perf-ensure-config:
	@command -v jq >/dev/null 2>&1 || { echo "$(COLOR_RED)❌ 需要 jq: brew install jq$(COLOR_RESET)" >&2; exit 1; }
	@test -f $(PERF_DIR)/qs-perf.config.json || { echo "$(COLOR_RED)❌ 先执行: make perf-init$(COLOR_RESET)" >&2; exit 1; }
	@if ! jq -e '.apiserverTokensFile // empty | length > 0' $(PERF_DIR)/qs-perf.config.json >/dev/null; then \
		jq '.apiserverTokensFile = "apiserver-tokens.json"' $(PERF_DIR)/qs-perf.config.json > $(PERF_DIR)/qs-perf.config.json.tmp; \
		mv $(PERF_DIR)/qs-perf.config.json.tmp $(PERF_DIR)/qs-perf.config.json; \
		echo "$(COLOR_GREEN)✅ 已补全 apiserverTokensFile$(COLOR_RESET)"; \
	fi
	@$(PERF_SCRIPT_DIR)/sync-profiles-from-example.sh $(PERF_DIR)/qs-perf.config.json $(PERF_SCRIPT_DIR)/qs-perf.config.example.json 2>/dev/null || true

perf-sync-profiles: ## 从 example 合并缺失的 qpsProfiles/paths（本地已有键保留）
	@command -v jq >/dev/null 2>&1 || { echo "$(COLOR_RED)❌ 需要 jq: brew install jq$(COLOR_RESET)" >&2; exit 1; }
	@test -f $(PERF_DIR)/qs-perf.config.json || { echo "$(COLOR_RED)❌ 先执行: make perf-init$(COLOR_RESET)" >&2; exit 1; }
	@$(PERF_SCRIPT_DIR)/sync-profiles-from-example.sh $(PERF_DIR)/qs-perf.config.json $(PERF_SCRIPT_DIR)/qs-perf.config.example.json

perf-sync-vusers: ## 用 example 覆盖本地各 profile 的 vusers/reportMode（WS 切换后执行）
	@command -v jq >/dev/null 2>&1 || { echo "$(COLOR_RED)❌ 需要 jq: brew install jq$(COLOR_RESET)" >&2; exit 1; }
	@test -f $(PERF_DIR)/qs-perf.config.json || { echo "$(COLOR_RED)❌ 先执行: make perf-init$(COLOR_RESET)" >&2; exit 1; }
	@bash $(PERF_SCRIPT_DIR)/sync-vusers-from-example.sh $(PERF_DIR)/qs-perf.config.json $(PERF_SCRIPT_DIR)/qs-perf.config.example.json

perf-tokens-collection: perf-ensure-config ## 用 collection_users 刷新 tokens.json
	@test -f $(PERF_DIR)/iam-users.json || { echo "$(COLOR_RED)❌ 缺少 $(PERF_DIR)/iam-users.json$(COLOR_RESET)" >&2; exit 1; }
	IAM_USERS_FILE=$(PERF_DIR)/iam-users.json \
	IAM_USERS_GROUP=collection_users \
	TOKENS_OUTPUT_FILE=$(PERF_DIR)/tokens.json \
	$(PERF_SCRIPT_DIR)/fetch-iam-tokens.sh

perf-tokens-apiserver: perf-ensure-config ## 用 apiserver_users 刷新 apiserver-tokens.json
	@test -f $(PERF_DIR)/iam-users.json || { echo "$(COLOR_RED)❌ 缺少 $(PERF_DIR)/iam-users.json$(COLOR_RESET)" >&2; exit 1; }
	IAM_USERS_FILE=$(PERF_DIR)/iam-users.json \
	IAM_USERS_GROUP=apiserver_users \
	TOKENS_OUTPUT_FILE=$(PERF_DIR)/apiserver-tokens.json \
	$(PERF_SCRIPT_DIR)/fetch-iam-tokens.sh

perf-tokens: perf-tokens-collection perf-tokens-apiserver ## 刷新 collection + apiserver 两套 token

perf-preflight: perf-ensure-config ## Token 预检（k6 前必跑）
	PERF_CONFIG_FILE=$(PERF_DIR)/qs-perf.config.json $(PERF_SCRIPT_DIR)/check-token-preflight.sh

perf-check-k6:
	@command -v k6 >/dev/null 2>&1 || { echo "$(COLOR_RED)❌ 需要 k6: brew install k6$(COLOR_RESET)" >&2; exit 1; }

perf-k6: perf-check-k6 ## 运行 k6 混合压测 (QPS_PROFILE=smoke_4|mixed_240_models|mixed_300|mixed_300_models|…)
	$(if $(SUMMARY_EXPORT),@mkdir -p $(dir $(SUMMARY_EXPORT)),)
	k6 run -e PERF_CONFIG_FILE="$(PERF_CONFIG_FILE)" -e PERF_ROOT_DIR="$(CURDIR)" \
		-e QPS_PROFILE="$(QPS_PROFILE)" \
		$(if $(SUMMARY_EXPORT),--summary-export $(SUMMARY_EXPORT),) \
		$(PERF_K6_SCRIPT)

perf-smoke: perf-preflight ## k6 smoke_4 全链路连通 (~30s)
	$(MAKE) perf-k6 QPS_PROFILE=smoke_4

perf-pretest60: perf-preflight ## k6 pretest_60 预压 (3min)
	@mkdir -p $(PERF_DIR)/pretest60
	$(MAKE) perf-k6 QPS_PROFILE=pretest_60 SUMMARY_EXPORT=$(PERF_DIR)/pretest60/k6-summary.json

perf-pretest120: perf-preflight ## k6 pretest_120 中档 (5min)
	@mkdir -p $(PERF_DIR)/pretest120
	$(MAKE) perf-k6 QPS_PROFILE=pretest_120 SUMMARY_EXPORT=$(PERF_DIR)/pretest120/k6-summary.json

perf-pretest120-submit-only: perf-preflight ## k6 pretest_120 隔离：仅 submit=19QPS (5min)
	@mkdir -p $(PERF_DIR)/pretest120-submit-only
	$(MAKE) perf-k6 QPS_PROFILE=pretest_120_submit_only SUMMARY_EXPORT=$(PERF_DIR)/pretest120-submit-only/k6-summary.json

perf-pretest120-balanced: perf-preflight ## k6 pretest_120 混合降读压：34/19/26/13 (5min)
	@mkdir -p $(PERF_DIR)/pretest120-balanced
	$(MAKE) perf-k6 QPS_PROFILE=pretest_120_balanced SUMMARY_EXPORT=$(PERF_DIR)/pretest120-balanced/k6-summary.json

perf-mixed140: perf-preflight ## k6 mixed_140 升档 (5min)
	@mkdir -p $(PERF_DIR)/mixed140
	$(MAKE) perf-k6 QPS_PROFILE=mixed_140 SUMMARY_EXPORT=$(PERF_DIR)/mixed140/k6-summary.json

perf-mixed140-submit24: perf-preflight ## k6 mixed_140 读压升档 + submit=19 (5min)
	@mkdir -p $(PERF_DIR)/mixed140-submit24
	$(MAKE) perf-k6 QPS_PROFILE=mixed_140_submit24 SUMMARY_EXPORT=$(PERF_DIR)/mixed140-submit24/k6-summary.json

perf-mixed160: perf-preflight ## k6 mixed_160 升档 (5min)
	@mkdir -p $(PERF_DIR)/mixed160
	$(MAKE) perf-k6 QPS_PROFILE=mixed_160 SUMMARY_EXPORT=$(PERF_DIR)/mixed160/k6-summary.json

perf-mixed180: perf-preflight ## k6 mixed_180 升档 (5min)
	@mkdir -p $(PERF_DIR)/mixed180
	$(MAKE) perf-k6 QPS_PROFILE=mixed_180 SUMMARY_EXPORT=$(PERF_DIR)/mixed180/k6-summary.json

perf-mixed200: perf-preflight ## k6 mixed_200 升档 (5min)
	@mkdir -p $(PERF_DIR)/mixed200
	$(MAKE) perf-k6 QPS_PROFILE=mixed_200 SUMMARY_EXPORT=$(PERF_DIR)/mixed200/k6-summary.json

perf-mixed220: perf-preflight ## k6 mixed_220 升档 (5min)
	@mkdir -p $(PERF_DIR)/mixed220
	$(MAKE) perf-k6 QPS_PROFILE=mixed_220 SUMMARY_EXPORT=$(PERF_DIR)/mixed220/k6-summary.json

perf-mixed240: perf-preflight ## k6 mixed_240 升档 (8min, legacy 问卷单桶 query)
	@mkdir -p $(PERF_DIR)/mixed240
	$(MAKE) perf-k6 QPS_PROFILE=mixed_240 SUMMARY_EXPORT=$(PERF_DIR)/mixed240/k6-summary.json

perf-mixed240-models: perf-preflight ## k6 mixed_240_models 三域 L1 验收 (8min, 拆分 query)
	@mkdir -p $(PERF_DIR)/mixed240-models
	$(MAKE) perf-k6 QPS_PROFILE=mixed_240_models SUMMARY_EXPORT=$(PERF_DIR)/mixed240-models/k6-summary.json

perf-mixed280: perf-preflight ## k6 mixed_280 升档 (8min, legacy 问卷单桶 query)
	@mkdir -p $(PERF_DIR)/mixed280
	$(MAKE) perf-k6 QPS_PROFILE=mixed_280 SUMMARY_EXPORT=$(PERF_DIR)/mixed280/k6-summary.json

perf-mixed280-models: perf-preflight ## k6 mixed_280_models 三域 L1 升档 (8min, WebSocket report-events)
	@mkdir -p $(PERF_DIR)/mixed280-models
	$(MAKE) perf-k6 QPS_PROFILE=mixed_280_models SUMMARY_EXPORT=$(PERF_DIR)/mixed280-models/k6-summary.json

perf-mixed280-models-long-poll: perf-special-report-long-poll ## 兼容旧 Makefile 名 → special_report_long_poll

perf-special-report-long-poll: perf-preflight ## k6 专项：wait-report 长轮询（非常规升档，生产已弃用）
	@mkdir -p $(PERF_DIR)/special-report-long-poll
	$(MAKE) perf-k6 QPS_PROFILE=special_report_long_poll SUMMARY_EXPORT=$(PERF_DIR)/special-report-long-poll/k6-summary.json

perf-special-report-short-poll: perf-preflight ## k6 专项：HTTP report-status 降级路径（不进升档链）
	@mkdir -p $(PERF_DIR)/special-report-short-poll
	$(MAKE) perf-k6 QPS_PROFILE=special_report_short_poll SUMMARY_EXPORT=$(PERF_DIR)/special-report-short-poll/k6-summary.json

perf-mixed280-models-short-report: perf-special-report-short-poll ## 兼容旧 Makefile 名 → special_report_short_poll

perf-mixed280-models-ws: perf-mixed280-models ## 兼容旧 Makefile 名（现与 mixed_280_models 相同）

perf-mixed300: perf-preflight ## k6 mixed_300 目标档 (10min, 含 chainProbe) + 前后 snapshot
	@mkdir -p $(PERF_DIR)/300qps
	OUT_DIR=$(PERF_DIR)/300qps $(PERF_SCRIPT_DIR)/snapshot-observability.sh before
	$(MAKE) perf-k6 QPS_PROFILE=mixed_300 SUMMARY_EXPORT=$(PERF_DIR)/300qps/k6-summary.json
	OUT_DIR=$(PERF_DIR)/300qps $(PERF_SCRIPT_DIR)/snapshot-observability.sh after

perf-mixed300-http: perf-preflight ## k6 mixed_300_http Step1 (10min, 280 读压+10m, 无 probe)
	@mkdir -p $(PERF_DIR)/300qps-http
	$(MAKE) perf-k6 QPS_PROFILE=mixed_300_http SUMMARY_EXPORT=$(PERF_DIR)/300qps-http/k6-summary.json

perf-mixed300-http-query: perf-preflight ## k6 mixed_300_http_query Step2 (10min, 300 query+report 96)
	@mkdir -p $(PERF_DIR)/300qps-http-query
	$(MAKE) perf-k6 QPS_PROFILE=mixed_300_http_query SUMMARY_EXPORT=$(PERF_DIR)/300qps-http-query/k6-summary.json

perf-mixed300-http-query-nostats: perf-preflight ## k6 线A：Step2 同读压+report，stats=0 (10min)
	@mkdir -p $(PERF_DIR)/300qps-http-query-nostats
	$(MAKE) perf-k6 QPS_PROFILE=mixed_300_http_query_nostats SUMMARY_EXPORT=$(PERF_DIR)/300qps-http-query-nostats/k6-summary.json

perf-stats-isolate29: perf-preflight ## k6 线A：仅 statistics 29/s overview+system (10min)
	@mkdir -p $(PERF_DIR)/stats-isolate29
	$(MAKE) perf-k6 QPS_PROFILE=stats_isolate_29 SUMMARY_EXPORT=$(PERF_DIR)/stats-isolate29/k6-summary.json

perf-stats-warmup: perf-preflight ## k6 线A：Step2 前 stats 预热 1min（overview+system @29/s）
	@mkdir -p $(PERF_DIR)/stats-warmup
	$(MAKE) perf-k6 QPS_PROFILE=stats_warmup_1m SUMMARY_EXPORT=$(PERF_DIR)/stats-warmup/k6-summary.json

perf-mixed300probe: perf-preflight ## k6 mixed_300_probe 目标档 + chainProbe (10min) + 前后 snapshot
	@mkdir -p $(PERF_DIR)/300qps-probe
	OUT_DIR=$(PERF_DIR)/300qps-probe $(PERF_SCRIPT_DIR)/snapshot-observability.sh before
	$(MAKE) perf-k6 QPS_PROFILE=mixed_300_probe SUMMARY_EXPORT=$(PERF_DIR)/300qps-probe/k6-summary.json
	OUT_DIR=$(PERF_DIR)/300qps-probe $(PERF_SCRIPT_DIR)/snapshot-observability.sh after

perf-model-smoke: perf-preflight ## k6 smoke_4 多 model 路径连通 (~30s)
	$(MAKE) perf-k6 QPS_PROFILE=smoke_4

perf-outbox120: perf-preflight ## k6 outbox_120 专测 outbox 排水 (10min) + 前后 snapshot
	@mkdir -p $(PERF_DIR)/outbox120
	OUT_DIR=$(PERF_DIR)/outbox120 $(PERF_SCRIPT_DIR)/snapshot-observability.sh before
	$(MAKE) perf-k6 QPS_PROFILE=outbox_120 SUMMARY_EXPORT=$(PERF_DIR)/outbox120/k6-summary.json
	OUT_DIR=$(PERF_DIR)/outbox120 $(PERF_SCRIPT_DIR)/snapshot-observability.sh after

perf-personality60: perf-preflight ## k6 personality_60 人格 session/submit/wait-report (5min)
	@mkdir -p $(PERF_DIR)/personality60
	$(MAKE) perf-k6 QPS_PROFILE=personality_60 SUMMARY_EXPORT=$(PERF_DIR)/personality60/k6-summary.json

perf-mixed300-models: perf-preflight ## k6 mixed_300_models 医学+人格混合 (~290QPS, 10min) + 前后 snapshot
	@mkdir -p $(PERF_DIR)/300qps-models
	OUT_DIR=$(PERF_DIR)/300qps-models $(PERF_SCRIPT_DIR)/snapshot-observability.sh before
	$(MAKE) perf-k6 QPS_PROFILE=mixed_300_models SUMMARY_EXPORT=$(PERF_DIR)/300qps-models/k6-summary.json
	OUT_DIR=$(PERF_DIR)/300qps-models $(PERF_SCRIPT_DIR)/snapshot-observability.sh after

perf-mixed300-scanner: perf-preflight ## k6 capacity_with_scanner（需先开启 behavior_journey_scan）+ 前后 snapshot
	@mkdir -p $(PERF_DIR)/300qps-scanner
	OUT_DIR=$(PERF_DIR)/300qps-scanner $(PERF_SCRIPT_DIR)/snapshot-observability.sh before
	$(MAKE) perf-k6 QPS_PROFILE=capacity_with_scanner SUMMARY_EXPORT=$(PERF_DIR)/300qps-scanner/k6-summary.json
	OUT_DIR=$(PERF_DIR)/300qps-scanner $(PERF_SCRIPT_DIR)/snapshot-observability.sh after

perf-diag-report120: perf-preflight ## 诊断 pretest_120：仅 report_status_query=36QPS
	QUERY_RPS=0 SUBMIT_RPS=0 REPORT_RPS=36 STATS_RPS=0 \
		$(MAKE) perf-k6 QPS_PROFILE=pretest_120 SUMMARY_EXPORT=$(PERF_DIR)/diag-report-only/k6-summary.json

perf-diag-query120: perf-preflight ## 诊断 pretest_120：仅 questionnaire_query=48QPS
	QUERY_RPS=48 SUBMIT_RPS=0 REPORT_RPS=0 STATS_RPS=0 \
		$(MAKE) perf-k6 QPS_PROFILE=pretest_120 SUMMARY_EXPORT=$(PERF_DIR)/diag-query-only/k6-summary.json

perf-diag-submit120: perf-preflight ## 诊断 pretest_120：仅 answersheet_submit=24QPS（等同 perf-pretest120-submit-only）
	@$(MAKE) perf-pretest120-submit-only SUMMARY_EXPORT=$(PERF_DIR)/diag-submit-only/k6-summary.json

perf-diag-query-submit120: perf-preflight ## 诊断 pretest_120：query=48QPS + submit=24QPS
	QUERY_RPS=48 SUBMIT_RPS=24 REPORT_RPS=0 STATS_RPS=0 \
		$(MAKE) perf-k6 QPS_PROFILE=pretest_120 SUMMARY_EXPORT=$(PERF_DIR)/diag-query-submit/k6-summary.json

perf-verify: perf-check-k6 ## 校验压测脚本与 k6 场景
	bash -n $(PERF_SCRIPT_DIR)/check-token-preflight.sh
	bash -n $(PERF_SCRIPT_DIR)/fetch-iam-tokens.sh
	bash -n $(PERF_SCRIPT_DIR)/snapshot-observability.sh
	bash -n $(PERF_SCRIPT_DIR)/sync-profiles-from-example.sh
	bash -n $(PERF_SCRIPT_DIR)/sync-vusers-from-example.sh
	k6 inspect $(PERF_K6_SCRIPT)
	k6 inspect $(PERF_SCRIPT_DIR)/k6-mixed-300qps.js
	k6 inspect -e PERF_CONFIG_FILE="$(CURDIR)/$(PERF_SCRIPT_DIR)/qs-perf.config.example.json" -e QPS_PROFILE=mixed_300 $(PERF_K6_SCRIPT) | grep -q report_ws_query
	k6 inspect -e PERF_CONFIG_FILE="$(CURDIR)/$(PERF_SCRIPT_DIR)/qs-perf.config.example.json" -e QPS_PROFILE=mixed_240_models $(PERF_K6_SCRIPT) | grep -q medical_model_query
	k6 inspect -e PERF_CONFIG_FILE="$(CURDIR)/$(PERF_SCRIPT_DIR)/qs-perf.config.example.json" -e QPS_PROFILE=mixed_280_models $(PERF_K6_SCRIPT) | grep -q medical_model_query
	k6 inspect -e PERF_CONFIG_FILE="$(CURDIR)/$(PERF_SCRIPT_DIR)/qs-perf.config.example.json" -e QPS_PROFILE=mixed_300_http $(PERF_K6_SCRIPT) | grep -q report_ws_query
	k6 inspect -e PERF_CONFIG_FILE="$(CURDIR)/$(PERF_SCRIPT_DIR)/qs-perf.config.example.json" -e QPS_PROFILE=mixed_300_http_query $(PERF_K6_SCRIPT) | grep -q personality_questionnaire_query
	k6 inspect -e PERF_CONFIG_FILE="$(CURDIR)/$(PERF_SCRIPT_DIR)/qs-perf.config.example.json" -e QPS_PROFILE=personality_60 $(PERF_K6_SCRIPT) | grep -q personality_report_ws_query
	k6 inspect -e PERF_CONFIG_FILE="$(CURDIR)/$(PERF_SCRIPT_DIR)/qs-perf.config.example.json" -e QPS_PROFILE=special_report_short_poll $(PERF_K6_SCRIPT) | grep -q report_status_query

# ============================================================================
# CD 发布入口
# ============================================================================

cd-validate: ## 校验 CD 服务元数据和脚本入口 (SERVICE=apiserver|collection|worker)
	@SERVICE="$(SERVICE)" IMAGE_METADATA_PRINT=1 "$(CD_SCRIPT_DIR)/image-metadata.sh" >/dev/null
	@test -x "$(CD_SCRIPT_DIR)/build-image.sh"
	@test -x "$(CD_SCRIPT_DIR)/export-image.sh"
	@test -x "$(CD_SCRIPT_DIR)/setup-runner-network.sh"
	@test -x "$(CD_SCRIPT_DIR)/setup-runner-ssh.sh"
	@test -x "$(CD_SCRIPT_DIR)/runner-upload-and-deploy.sh"
	@test -x "$(CD_SCRIPT_DIR)/push-dockerhub.sh"
	@test -x "$(CD_SCRIPT_DIR)/push-acr.sh"
	@test -x "$(CD_SCRIPT_DIR)/prepare-package.sh"
	@test -x "$(CD_SCRIPT_DIR)/remote-deploy.sh"
	@test -x "$(CD_SCRIPT_DIR)/plan-services.sh"
	@echo "$(COLOR_GREEN)✅ CD metadata validated for SERVICE=$(SERVICE)$(COLOR_RESET)"

cd-plan: ## 规划本次 CD 需要发布的服务
	@"$(CD_SCRIPT_DIR)/plan-services.sh"

cd-export-image: cd-validate ## 在 CI runner 拉取并导出服务镜像 tarball（供 SCP 到生产机 docker load）
	@SERVICE="$(SERVICE)" DEPLOY_SHA="$(DEPLOY_SHA)" "$(CD_SCRIPT_DIR)/export-image.sh"

cd-push-acr: cd-validate ## 将已发布到 GHCR 的镜像同步到阿里云 ACR
	@SERVICE="$(SERVICE)" DEPLOY_SHA="$(DEPLOY_SHA)" "$(CD_SCRIPT_DIR)/push-acr.sh"

cd-image: cd-validate ## 构建并发布服务镜像到 GHCR、Docker Hub 和阿里云 ACR
	@SERVICE="$(SERVICE)" DEPLOY_REF="$(DEPLOY_REF)" DEPLOY_SHA="$(DEPLOY_SHA)" BUILD_TIME="$(BUILD_TIME)" BUILD_CACHE_REF="$(BUILD_CACHE_REF)" "$(CD_SCRIPT_DIR)/build-image.sh"
	@SERVICE="$(SERVICE)" DEPLOY_SHA="$(DEPLOY_SHA)" "$(CD_SCRIPT_DIR)/push-dockerhub.sh"
	@SERVICE="$(SERVICE)" DEPLOY_SHA="$(DEPLOY_SHA)" "$(CD_SCRIPT_DIR)/push-acr.sh"

cd-package: cd-validate ## 生成服务生产部署包
	@SERVICE="$(SERVICE)" "$(CD_SCRIPT_DIR)/prepare-package.sh"

cd-remote-deploy: cd-validate ## 在目标机执行远端部署脚本
	@SERVICE="$(SERVICE)" IMAGE_TAG="$(IMAGE_TAG)" "$(CD_SCRIPT_DIR)/remote-deploy.sh"

version: ## 显示版本信息
	@echo "$(COLOR_BOLD)版本信息:$(COLOR_RESET)"
	@echo "  版本:     $(COLOR_GREEN)$(VERSION)$(COLOR_RESET)"
	@echo "  构建时间: $(BUILD_TIME)"
	@echo "  Git 提交: $(GIT_COMMIT)"
	@echo "  Git 分支: $(GIT_BRANCH)"
	@echo "  Go 版本:  $(shell $(GO) version)"

# ============================================================================
# 快速启动
# ============================================================================

quick-start: check-infra build-all run-all ## 快速启动 (检查环境 + 构建 + 运行所有服务)
	@echo "$(COLOR_GREEN)✅ 开发环境已就绪!$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BLUE)ℹ️  已启动服务:$(COLOR_RESET)"
	@echo "  $(COLOR_GREEN)•$(COLOR_RESET) API Server        ($(APISERVER_PORT))"
	@echo "  $(COLOR_GREEN)•$(COLOR_RESET) Collection Server ($(COLLECTION_PORT))"
	@echo "  $(COLOR_GREEN)•$(COLOR_RESET) Worker            (后台处理)"
	@echo ""
	@$(MAKE) status-all

# ============================================================================
# 构建命令
# ============================================================================

build: build-all ## 构建所有服务

build-all: ## 构建所有服务
	@echo "$(COLOR_BOLD)$(COLOR_BLUE)🔨 构建所有服务 [环境: $(ENV)]...$(COLOR_RESET)"
	@$(MAKE) create-dirs
	@$(MAKE) build-apiserver
	@$(MAKE) build-collection
	@$(MAKE) build-worker
	@echo "$(COLOR_GREEN)✅ 所有服务构建完成$(COLOR_RESET)"

build-apiserver: ## 构建 API 服务器
	@echo "$(COLOR_BLUE)🔨 构建 API 服务器...$(COLOR_RESET)"
	@$(MAKE) create-dirs
	@$(GO_BUILD) $(GO_LDFLAGS) -o $(APISERVER_BIN) ./cmd/qs-apiserver/
	@echo "$(COLOR_GREEN)✅ API 服务器构建完成: $(APISERVER_BIN)$(COLOR_RESET)"

build-collection: ## 构建 Collection 服务器
	@echo "$(COLOR_BLUE)🔨 构建 Collection 服务器...$(COLOR_RESET)"
	@$(MAKE) create-dirs
	@$(GO_BUILD) $(GO_LDFLAGS) -o $(COLLECTION_BIN) ./cmd/collection-server/
	@echo "$(COLOR_GREEN)✅ Collection 服务器构建完成: $(COLLECTION_BIN)$(COLOR_RESET)"

build-worker: ## 构建 Worker 服务
	@echo "$(COLOR_BLUE)🔨 构建 Worker 服务...$(COLOR_RESET)"
	@$(MAKE) create-dirs
	@$(GO_BUILD) $(GO_LDFLAGS) -o $(WORKER_BIN) ./cmd/qs-worker/
	@echo "$(COLOR_GREEN)✅ Worker 服务构建完成: $(WORKER_BIN)$(COLOR_RESET)"

# ============================================================================
# 服务运行管理
# ============================================================================

run: run-all ## 启动所有服务

run-all: check-infra ## 启动所有服务（先检查基础设施）
	@echo "$(COLOR_BOLD)$(COLOR_BLUE)🚀 启动所有服务 [环境: $(ENV)]...$(COLOR_RESET)"
	@$(MAKE) create-dirs
	@$(MAKE) run-apiserver
	@sleep 2
	@$(MAKE) run-collection
	@sleep 2
	@$(MAKE) run-worker
	@echo "$(COLOR_GREEN)✅ 所有服务已启动$(COLOR_RESET)"
	@echo ""
	@$(MAKE) status-all

run-apiserver: ## 启动 API 服务器
	@echo "🚀 启动 qs-apiserver..."
	@$(MAKE) create-dirs
	@if [ -f $(PID_DIR)/apiserver.pid ]; then \
		echo "⚠️  qs-apiserver 可能已在运行 (PID: $$(cat $(PID_DIR)/apiserver.pid))"; \
		if ! kill -0 $$(cat $(PID_DIR)/apiserver.pid) 2>/dev/null; then \
			echo "🧹 清理无效的 PID 文件"; \
			rm -f $(PID_DIR)/apiserver.pid; \
		else \
			echo "❌ qs-apiserver 已在运行，请先停止"; \
			exit 1; \
		fi; \
	fi
	@nohup ./$(APISERVER_BIN) --config=$(APISERVER_CONFIG) > $(LOG_DIR)/apiserver.log 2>&1 & echo $$! > $(PID_DIR)/apiserver.pid
	@echo "✅ qs-apiserver 已启动 (PID: $$(cat $(PID_DIR)/apiserver.pid))"

run-collection: ## 启动 Collection 服务器
	@echo "🚀 启动 collection-server..."
	@$(MAKE) create-dirs
	@if [ -f $(PID_DIR)/collection.pid ]; then \
		echo "⚠️  collection-server 可能已在运行 (PID: $$(cat $(PID_DIR)/collection.pid))"; \
		if ! kill -0 $$(cat $(PID_DIR)/collection.pid) 2>/dev/null; then \
			echo "🧹 清理无效的 PID 文件"; \
			rm -f $(PID_DIR)/collection.pid; \
		else \
			echo "❌ collection-server 已在运行，请先停止"; \
			exit 1; \
		fi; \
	fi
	@nohup ./$(COLLECTION_BIN) --config=$(COLLECTION_CONFIG) > $(LOG_DIR)/collection-server.log 2>&1 & echo $$! > $(PID_DIR)/collection.pid
	@echo "✅ collection-server 已启动 (PID: $$(cat $(PID_DIR)/collection.pid))"

run-worker: ## 启动 Worker 服务
	@echo "🚀 启动 qs-worker..."
	@$(MAKE) create-dirs
	@if [ -f $(PID_DIR)/worker.pid ]; then \
		echo "⚠️  qs-worker 可能已在运行 (PID: $$(cat $(PID_DIR)/worker.pid))"; \
		if ! kill -0 $$(cat $(PID_DIR)/worker.pid) 2>/dev/null; then \
			echo "🧹 清理无效的 PID 文件"; \
			rm -f $(PID_DIR)/worker.pid; \
		else \
			echo "❌ qs-worker 已在运行，请先停止"; \
			exit 1; \
		fi; \
	fi
	@nohup ./$(WORKER_BIN) --config=$(WORKER_CONFIG) > $(LOG_DIR)/worker.log 2>&1 & echo $$! > $(PID_DIR)/worker.pid
	@echo "✅ qs-worker 已启动 (PID: $$(cat $(PID_DIR)/worker.pid))"

# ============================================================================
# 服务停止管理
# ============================================================================

stop: stop-all ## 停止所有服务

stop-all: ## 停止所有服务
	@echo "⏹️  停止所有服务..."
	@$(MAKE) stop-worker
	@$(MAKE) stop-collection
	@$(MAKE) stop-apiserver
	@echo "✅ 所有服务已停止"

stop-apiserver: ## 停止 API 服务器
	@echo "⏹️  停止 qs-apiserver..."
	@if [ -f $(PID_DIR)/apiserver.pid ]; then \
		PID=$$(cat $(PID_DIR)/apiserver.pid); \
		if kill -0 $$PID 2>/dev/null; then \
			kill $$PID && echo "✅ qs-apiserver 已停止 (PID: $$PID)"; \
			rm -f $(PID_DIR)/apiserver.pid; \
		else \
			echo "⚠️  qs-apiserver 进程不存在，清理 PID 文件"; \
			rm -f $(PID_DIR)/apiserver.pid; \
		fi; \
	else \
		echo "ℹ️  qs-apiserver 未运行"; \
	fi

stop-collection: ## 停止 Collection 服务器
	@echo "⏹️  停止 collection-server..."
	@if [ -f $(PID_DIR)/collection.pid ]; then \
		PID=$$(cat $(PID_DIR)/collection.pid); \
		if kill -0 $$PID 2>/dev/null; then \
			kill $$PID && echo "✅ collection-server 已停止 (PID: $$PID)"; \
			rm -f $(PID_DIR)/collection.pid; \
		else \
			echo "⚠️  collection-server 进程不存在，清理 PID 文件"; \
			rm -f $(PID_DIR)/collection.pid; \
		fi; \
	else \
		echo "ℹ️  collection-server 未运行"; \
	fi

stop-worker: ## 停止 Worker 服务
	@echo "⏹️  停止 qs-worker..."
	@if [ -f $(PID_DIR)/worker.pid ]; then \
		PID=$$(cat $(PID_DIR)/worker.pid); \
		if kill -0 $$PID 2>/dev/null; then \
			kill $$PID && echo "✅ qs-worker 已停止 (PID: $$PID)"; \
			rm -f $(PID_DIR)/worker.pid; \
		else \
			echo "⚠️  qs-worker 进程不存在，清理 PID 文件"; \
			rm -f $(PID_DIR)/worker.pid; \
		fi; \
	else \
		echo "ℹ️  qs-worker 未运行"; \
	fi

# ============================================================================
# 服务重启管理
# ============================================================================

restart: restart-all ## 重启所有服务

restart-all: ## 重启所有服务
	@echo "🔄 重启所有服务..."
	@$(MAKE) stop-all
	@sleep 2
	@$(MAKE) run-all

restart-apiserver: ## 重启 API 服务器
	@echo "🔄 重启 qs-apiserver..."
	@$(MAKE) stop-apiserver
	@sleep 1
	@$(MAKE) run-apiserver

restart-collection: ## 重启 Collection 服务器
	@echo "🔄 重启 collection-server..."
	@$(MAKE) stop-collection
	@sleep 1
	@$(MAKE) run-collection

restart-worker: ## 重启 Worker 服务
	@echo "🔄 重启 qs-worker..."
	@$(MAKE) stop-worker
	@sleep 1
	@$(MAKE) run-worker

# ============================================================================
# 服务状态和日志
# ============================================================================

status: status-all ## 查看所有服务状态

status-all: ## 查看所有服务状态
	@echo "📊 服务状态 [环境: $(ENV)]:"
	@echo "============"
	@$(MAKE) status-apiserver
	@$(MAKE) status-collection
	@$(MAKE) status-worker

status-apiserver: ## 查看 API 服务器状态
	@if [ -f $(PID_DIR)/apiserver.pid ]; then \
		PID=$$(cat $(PID_DIR)/apiserver.pid); \
		if kill -0 $$PID 2>/dev/null; then \
			echo "✅ qs-apiserver      - 运行中 (PID: $$PID, Port: $(APISERVER_PORT))"; \
		else \
			echo "❌ qs-apiserver      - 已停止 (PID 文件存在但进程不存在)"; \
		fi; \
	else \
		echo "⚪ qs-apiserver      - 未运行"; \
	fi

status-collection: ## 查看 Collection 服务器状态
	@if [ -f $(PID_DIR)/collection.pid ]; then \
		PID=$$(cat $(PID_DIR)/collection.pid); \
		if kill -0 $$PID 2>/dev/null; then \
			echo "✅ collection-server - 运行中 (PID: $$PID, Port: $(COLLECTION_PORT))"; \
		else \
			echo "❌ collection-server - 已停止 (PID 文件存在但进程不存在)"; \
		fi; \
	else \
		echo "⚪ collection-server - 未运行"; \
	fi

status-worker: ## 查看 Worker 服务状态
	@if [ -f $(PID_DIR)/worker.pid ]; then \
		PID=$$(cat $(PID_DIR)/worker.pid); \
		if kill -0 $$PID 2>/dev/null; then \
			echo "✅ qs-worker         - 运行中 (PID: $$PID)"; \
		else \
			echo "❌ qs-worker         - 已停止 (PID 文件存在但进程不存在)"; \
		fi; \
	else \
		echo "⚪ qs-worker         - 未运行"; \
	fi

logs: logs-all ## 查看所有日志

logs-all: ## 查看所有服务日志
	@echo "📋 查看所有服务日志..."
	@echo "使用 Ctrl+C 退出"
	@tail -f $(LOG_DIR)/apiserver.log $(LOG_DIR)/collection-server.log $(LOG_DIR)/worker.log

logs-apiserver: ## 查看 API 服务器日志
	@echo "📋 查看 qs-apiserver 日志..."
	@tail -f $(LOG_DIR)/apiserver.log

logs-collection: ## 查看 Collection 服务器日志
	@echo "📋 查看 collection-server 日志..."
	@tail -f $(LOG_DIR)/collection-server.log

logs-worker: ## 查看 Worker 服务日志
	@echo "📋 查看 qs-worker 日志..."
	@tail -f $(LOG_DIR)/worker.log

# ============================================================================
# 健康检查
# ============================================================================

health: health-check ## 健康检查

health-check: ## 检查所有服务健康状态
	@echo "🔍 健康检查:"
	@echo "============"
	@echo -n "qs-apiserver:      "; curl -s http://localhost:$(APISERVER_PORT)/healthz || echo "❌ 无响应"
	@echo -n "collection-server: "; curl -s http://localhost:$(COLLECTION_PORT)/healthz || echo "❌ 无响应"
	@echo -n "qs-worker:         "; \
		if [ -f $(PID_DIR)/worker.pid ] && kill -0 $$(cat $(PID_DIR)/worker.pid) 2>/dev/null; then \
			echo "✅ 运行中 (后台消费者)"; \
		else \
			echo "❌ 未运行"; \
		fi

# ============================================================================
# 基础设施检查
# ============================================================================

check-infra: ## 检查所有基础设施组件是否就绪
	@bash scripts/check-infra.sh all

check-mysql: ## 检查 MySQL 是否就绪
	@bash scripts/check-infra.sh mysql

check-redis: ## 检查 Redis 是否就绪
	@bash scripts/check-infra.sh redis

check-mongodb: ## 检查 MongoDB 是否就绪
	@bash scripts/check-infra.sh mongodb

check-nsq: ## 检查 NSQ 是否就绪
	@bash scripts/check-infra.sh nsq

# ============================================================================
# 开发工具
# ============================================================================

dev: ## 启动开发环境（热更新）
	@echo "🚀 启动开发环境..."
	@mkdir -p $(PID_DIR)
	@echo "启动 qs-apiserver..."
	@air -c .air-apiserver.toml & echo $$! > $(PID_DIR)/air-apiserver.pid
	@sleep 2
	@echo "启动 collection-server..."
	@air -c .air-collection.toml & echo $$! > $(PID_DIR)/air-collection.pid
	@sleep 2
	@echo "启动 qs-worker..."
	@air -c .air-worker.toml & echo $$! > $(PID_DIR)/air-worker.pid
	@sleep 2
	@echo "✅ 所有服务已启动（热更新模式）"
	@echo "提示：使用 Ctrl+C 停止所有服务"
	@echo "      或使用 make dev-stop 停止服务"

dev-apiserver: ## 独立启动 API 服务器（热更新）
	@echo "🚀 启动 qs-apiserver 开发环境..."
	@mkdir -p $(PID_DIR)
	@air -c .air-apiserver.toml

dev-collection: ## 独立启动 Collection 服务器（热更新）
	@echo "🚀 启动 collection-server 开发环境..."
	@mkdir -p $(PID_DIR)
	@air -c .air-collection.toml

dev-worker: ## 独立启动 Worker 服务（热更新）
	@echo "🚀 启动 qs-worker 开发环境..."
	@mkdir -p $(PID_DIR)
	@air -c .air-worker.toml

dev-stop: ## 停止开发环境
	@echo "⏹️  停止开发环境..."
	@if [ -f $(PID_DIR)/air-worker.pid ]; then \
		kill $$(cat $(PID_DIR)/air-worker.pid) 2>/dev/null || true; \
		rm -f $(PID_DIR)/air-worker.pid; \
	fi
	@if [ -f $(PID_DIR)/air-collection.pid ]; then \
		kill $$(cat $(PID_DIR)/air-collection.pid) 2>/dev/null || true; \
		rm -f $(PID_DIR)/air-collection.pid; \
	fi
	@if [ -f $(PID_DIR)/air-apiserver.pid ]; then \
		kill $$(cat $(PID_DIR)/air-apiserver.pid) 2>/dev/null || true; \
		rm -f $(PID_DIR)/air-apiserver.pid; \
	fi
	@echo "✅ 开发环境已停止"

dev-status: ## 查看开发环境状态
	@echo "📊 开发环境状态:"
	@echo "=============="
	@if [ -f $(PID_DIR)/air-apiserver.pid ] && kill -0 $$(cat $(PID_DIR)/air-apiserver.pid) 2>/dev/null; then \
		echo "✅ qs-apiserver      - 运行中 (PID: $$(cat $(PID_DIR)/air-apiserver.pid))"; \
	else \
		echo "⚪ qs-apiserver      - 未运行"; \
	fi
	@if [ -f $(PID_DIR)/air-collection.pid ] && kill -0 $$(cat $(PID_DIR)/air-collection.pid) 2>/dev/null; then \
		echo "✅ collection-server - 运行中 (PID: $$(cat $(PID_DIR)/air-collection.pid))"; \
	else \
		echo "⚪ collection-server - 未运行"; \
	fi
	@if [ -f $(PID_DIR)/air-worker.pid ] && kill -0 $$(cat $(PID_DIR)/air-worker.pid) 2>/dev/null; then \
		echo "✅ qs-worker         - 运行中 (PID: $$(cat $(PID_DIR)/air-worker.pid))"; \
	else \
		echo "⚪ qs-worker         - 未运行"; \
	fi

dev-logs: ## 查看开发环境日志
	@echo "📋 开发环境日志:"
	@echo "=============="
	@tail -f $(TMP_DIR)/build-errors-*.log

# ============================================================================
# 测试
# ============================================================================

test: ## 运行测试
	@echo "$(COLOR_CYAN)🧪 运行测试...$(COLOR_RESET)"
	@$(GO_TEST) ./...

test-unit: ## 运行单元测试
	@echo "$(COLOR_CYAN)🧪 运行单元测试...$(COLOR_RESET)"
	@$(GO_TEST) -v -short ./...

test-coverage: create-dirs ## 生成测试覆盖率报告
	@echo "$(COLOR_CYAN)🧪 生成测试覆盖率报告...$(COLOR_RESET)"
	@mkdir -p $(COVERAGE_DIR)
	@$(GO_TEST) -v -coverprofile=$(COVERAGE_DIR)/coverage.out ./...
	@$(GO) tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	@echo "$(COLOR_GREEN)✅ 覆盖率报告: $(COVERAGE_DIR)/coverage.html$(COLOR_RESET)"
	@$(GO) tool cover -func=$(COVERAGE_DIR)/coverage.out | tail -n 1

test-race: ## 运行竞态检测
	@echo "$(COLOR_CYAN)🧪 运行竞态检测...$(COLOR_RESET)"
	@$(GO_TEST) -v -race ./...

test-bench: ## 运行基准测试
	@echo "$(COLOR_CYAN)🧪 运行基准测试...$(COLOR_RESET)"
	@$(GO_TEST) -v -bench=. -benchmem ./...

test-all: test test-race ## 运行所有测试

test-message-queue: ## 测试消息队列系统
	@echo "📨 测试消息队列系统..."
	@if [ ! -x "./test-message-queue.sh" ]; then \
		echo "❌ 测试脚本不存在或不可执行"; \
		exit 1; \
	fi
	@./test-message-queue.sh

test-submit: ## 测试答卷提交
	@echo "📝 测试答卷提交..."
	@if [ ! -x "./test-answersheet-submit.sh" ]; then \
		echo "❌ 测试脚本不存在或不可执行"; \
		exit 1; \
	fi
	@./test-answersheet-submit.sh

# ============================================================================
# 代码质量
# ============================================================================

ensure-golangci-lint:
	@mkdir -p "$(BIN_DIR)"
	@tool="$(GOLANGCI_LINT_BIN)"; \
	target_go="$(GO_TOOLCHAIN)"; \
	built_go="$$( "$$tool" version 2>/dev/null | sed -n 's/.*built with \(go[^ ]*\).*/\1/p')"; \
	if [ ! -x "$$tool" ] || [ "$$built_go" != "$$target_go" ]; then \
		echo "$(COLOR_CYAN)📦 安装 golangci-lint ($(GOLANGCI_LINT_VERSION)) 到仓库本地 bin/...$(COLOR_RESET)"; \
		env -u GOVERSION GOSUMDB=sum.golang.org GOTOOLCHAIN="$(GO_TOOLCHAIN)" GOBIN="$(CURDIR)/$(BIN_DIR)" $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi

lint: ensure-golangci-lint ## 运行代码检查
	@echo "$(COLOR_CYAN)🔍 运行代码检查...$(COLOR_RESET)"
	@"$(GOLANGCI_LINT_BIN)" run --timeout=5m

lint-boundaries: ensure-golangci-lint ## 运行分层边界检查（depguard: domain/application）
	@echo "$(COLOR_CYAN)🧱 运行分层边界检查...$(COLOR_RESET)"
	@"$(GOLANGCI_LINT_BIN)" run -c .golangci-depguard.yml --timeout=5m

vuln: security-govulncheck ## 运行依赖漏洞扫描（govulncheck）

verify: test lint lint-boundaries vuln ## AI 重构前后质量门禁（行为 + 代码质量 + 分层边界 + 依赖安全）

maintainability-lint: ensure-golangci-lint ## 运行 maintainability advisory 检查
	@echo "$(COLOR_CYAN)🧭 运行 maintainability advisory 检查...$(COLOR_RESET)"
	@mkdir -p "$(MAINTAINABILITY_DIR)"
	@"$(GOLANGCI_LINT_BIN)" run -c .golangci-maintainability.yml --timeout=8m --issues-exit-code=0 \
		--output.text.path "$(MAINTAINABILITY_DIR)/maintainability.txt" \
		--output.json.path "$(MAINTAINABILITY_DIR)/maintainability.json"
	@cat "$(MAINTAINABILITY_DIR)/maintainability.txt"

maintainability-lint-ci: ensure-golangci-lint ## 运行 maintainability advisory 检查并导出报告
	@echo "$(COLOR_CYAN)🧭 运行 maintainability advisory 检查（CI）...$(COLOR_RESET)"
	@mkdir -p "$(MAINTAINABILITY_DIR)"
	@"$(GOLANGCI_LINT_BIN)" run -c .golangci-maintainability.yml --timeout=8m --issues-exit-code=0 \
		--output.text.path "$(MAINTAINABILITY_DIR)/maintainability.txt" \
		--output.json.path "$(MAINTAINABILITY_DIR)/maintainability.json"
	@echo "$(COLOR_GREEN)✅ maintainability 报告已写入 $(MAINTAINABILITY_DIR)$(COLOR_RESET)"

tier1-test-policy: ## 校验 Tier 1 包修改必须伴随同包测试变更
	@echo "$(COLOR_CYAN)🧪 校验 Tier 1 包测试策略...$(COLOR_RESET)"
	@"$(CURDIR)/$(QUALITY_DIR)/check_tier1_tests.sh"

fmt: ## 格式化代码
	@echo "$(COLOR_CYAN)✨ 格式化代码...$(COLOR_RESET)"
	@$(GO) fmt ./...
	@gofmt -s -w .
	@echo "$(COLOR_GREEN)✅ 代码格式化完成$(COLOR_RESET)"

fmt-check: ## 检查代码格式
	@echo "$(COLOR_CYAN)🔍 检查代码格式...$(COLOR_RESET)"
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "$(COLOR_RED)❌ 以下文件需要格式化:$(COLOR_RESET)"; \
		gofmt -l .; \
		exit 1; \
	else \
		echo "$(COLOR_GREEN)✅ 代码格式正确$(COLOR_RESET)"; \
	fi

security: security-govulncheck security-gosec ## 运行安全扫描

security-govulncheck: ## 运行依赖漏洞扫描
	@echo "$(COLOR_CYAN)🔐 运行 govulncheck...$(COLOR_RESET)"
	@mkdir -p "$(SECURITY_DIR)"
	@tool="$$(command -v govulncheck 2>/dev/null || printf '%s\n' '$(CURDIR)/$(BIN_DIR)/govulncheck')"; \
	if [ ! -x "$$tool" ]; then \
		echo "$(COLOR_RED)❌ govulncheck 未安装，请先运行 'make install-security-tools'$(COLOR_RESET)"; \
		exit 1; \
	fi; \
	status=0; \
	(cd "$(CURDIR)/cmd/collection-server" && env -u GOVERSION GOSUMDB=sum.golang.org "$$tool" -scan=module > "$(CURDIR)/$(SECURITY_DIR)/govulncheck.txt" 2>&1) || status=$$?; \
	cat "$(SECURITY_DIR)/govulncheck.txt"; \
	exit $$status

security-govulncheck-ci: ## 运行依赖漏洞扫描并导出 JSON 报告（CI advisory）
	@echo "$(COLOR_CYAN)🔐 运行 govulncheck（CI advisory）...$(COLOR_RESET)"
	@mkdir -p "$(SECURITY_DIR)"
	@tool="$$(command -v govulncheck 2>/dev/null || printf '%s\n' '$(CURDIR)/$(BIN_DIR)/govulncheck')"; \
	if [ ! -x "$$tool" ]; then \
		echo "$(COLOR_RED)❌ govulncheck 未安装，请先运行 'make install-security-tools'$(COLOR_RESET)"; \
		exit 1; \
	fi; \
	status=0; \
	(cd "$(CURDIR)/cmd/collection-server" && env -u GOVERSION GOSUMDB=sum.golang.org "$$tool" -format json -scan=module > "$(CURDIR)/$(SECURITY_DIR)/govulncheck.json") || status=$$?; \
	if [ $$status -ne 0 ] && [ $$status -ne 3 ]; then \
		exit $$status; \
	fi; \
	env -u GOVERSION GOSUMDB=sum.golang.org $(GO) run ./scripts/security/govulncheck_summary.go \
		-input "$(SECURITY_DIR)/govulncheck.json" \
		-output "$(SECURITY_DIR)/govulncheck-summary.md"; \
	if grep -q '"osv": {' "$(SECURITY_DIR)/govulncheck.json"; then \
		echo "$(COLOR_YELLOW)⚠️ govulncheck 发现已知漏洞，报告已写入 $(SECURITY_DIR)/govulncheck.json$(COLOR_RESET)"; \
		echo "$(COLOR_YELLOW)⚠️ 分组摘要已写入 $(SECURITY_DIR)/govulncheck-summary.md$(COLOR_RESET)"; \
	else \
		echo "$(COLOR_GREEN)✅ govulncheck 未发现已知漏洞$(COLOR_RESET)"; \
	fi

security-gosec: ## 运行 gosec 静态安全扫描
	@echo "$(COLOR_CYAN)🔐 运行 gosec...$(COLOR_RESET)"
	@mkdir -p "$(SECURITY_DIR)"
	@tool="$$(command -v gosec 2>/dev/null || printf '%s\n' '$(CURDIR)/$(BIN_DIR)/gosec')"; \
	if [ ! -x "$$tool" ]; then \
		echo "$(COLOR_RED)❌ gosec 未安装，请先运行 'make install-security-tools'$(COLOR_RESET)"; \
		exit 1; \
	fi; \
	status=0; \
	env -u GOVERSION GOSUMDB=sum.golang.org "$$tool" $(GOSEC_BASE_ARGS) ./... > "$(SECURITY_DIR)/gosec.txt" 2>&1 || status=$$?; \
	cat "$(SECURITY_DIR)/gosec.txt"; \
	exit $$status

security-gosec-ci: ## 运行 gosec 并导出 SARIF 报告（CI advisory）
	@echo "$(COLOR_CYAN)🔐 运行 gosec（CI advisory）...$(COLOR_RESET)"
	@mkdir -p "$(SECURITY_DIR)"
	@rm -f "$(SECURITY_DIR)/gosec.sarif"
	@tool="$$(command -v gosec 2>/dev/null || printf '%s\n' '$(CURDIR)/$(BIN_DIR)/gosec')"; \
	if [ ! -x "$$tool" ]; then \
		echo "$(COLOR_RED)❌ gosec 未安装，请先运行 'make install-security-tools'$(COLOR_RESET)"; \
		exit 1; \
	fi; \
	env -u GOVERSION GOSUMDB=sum.golang.org "$$tool" $(GOSEC_BASE_ARGS) -quiet -no-fail -fmt sarif -out "$(SECURITY_DIR)/gosec.sarif" ./...; \
	if [ ! -f "$(SECURITY_DIR)/gosec.sarif" ]; then \
		printf '%s\n' '{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"gosec"}},"results":[]}]}' > "$(SECURITY_DIR)/gosec.sarif"; \
	fi
	@echo "$(COLOR_GREEN)✅ gosec 报告已写入 $(SECURITY_DIR)/gosec.sarif$(COLOR_RESET)"

# ============================================================================
# 依赖管理
# ============================================================================

deps: deps-download ## 下载依赖

deps-download: ## 下载所有依赖
	@echo "$(COLOR_CYAN)📦 下载依赖...$(COLOR_RESET)"
	@$(GO) mod download
	@echo "$(COLOR_GREEN)✅ 依赖下载完成$(COLOR_RESET)"

deps-tidy: ## 整理依赖
	@echo "$(COLOR_CYAN)🧹 整理依赖...$(COLOR_RESET)"
	@$(GO) mod tidy
	@echo "$(COLOR_GREEN)✅ 依赖整理完成$(COLOR_RESET)"

deps-verify: ## 验证依赖
	@echo "$(COLOR_CYAN)🔍 验证依赖...$(COLOR_RESET)"
	@$(GO) mod verify
	@echo "$(COLOR_GREEN)✅ 依赖验证通过$(COLOR_RESET)"

deps-check: ## 检查可更新的依赖
	@echo "$(COLOR_CYAN)🔍 检查依赖状态...$(COLOR_RESET)"
	@$(GO) list -u -m all | grep -v indirect || true
	@echo ""
	@echo "$(COLOR_YELLOW)说明: 后面有方括号 [...] 的表示有更新可用$(COLOR_RESET)"

# ============================================================================
# 工具安装
# ============================================================================

install-tools: ## 安装开发工具
	@echo "$(COLOR_CYAN)📦 安装开发工具...$(COLOR_RESET)"
	@echo "安装 Air (热更新)..."
	@$(GO) install github.com/air-verse/air@latest
	@echo "安装 golangci-lint ($(GOLANGCI_LINT_VERSION))..."
	@$(MAKE) create-dirs
	@env -u GOVERSION GOSUMDB=sum.golang.org GOTOOLCHAIN="$(GO_TOOLCHAIN)" GOBIN="$(CURDIR)/$(BIN_DIR)" $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@echo "$(COLOR_GREEN)✅ 工具安装完成$(COLOR_RESET)"

install-air: ## 安装 Air 热更新工具
	@echo "📦 安装 Air..."
	@$(GO) install github.com/air-verse/air@latest

install-golangci-lint: ## 安装 golangci-lint
	@echo "📦 安装 golangci-lint ($(GOLANGCI_LINT_VERSION))..."
	@$(MAKE) create-dirs
	@env -u GOVERSION GOSUMDB=sum.golang.org GOTOOLCHAIN="$(GO_TOOLCHAIN)" GOBIN="$(CURDIR)/$(BIN_DIR)" $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

install-security-tools: install-govulncheck install-gosec ## 安装安全扫描工具
	@echo "$(COLOR_GREEN)✅ 安全扫描工具安装完成$(COLOR_RESET)"

install-govulncheck: ## 安装 govulncheck
	@echo "📦 安装 govulncheck ($(GOVULNCHECK_VERSION))..."
	@$(MAKE) create-dirs
	@env -u GOVERSION GOSUMDB=sum.golang.org GOBIN="$(CURDIR)/$(BIN_DIR)" $(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

install-gosec: ## 安装 gosec
	@echo "📦 安装 gosec ($(GOSEC_VERSION))..."
	@$(MAKE) create-dirs
	@env -u GOVERSION GOSUMDB=sum.golang.org GOBIN="$(CURDIR)/$(BIN_DIR)" $(GO) install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)

# ============================================================================
# 清理和维护
# ============================================================================

clean: ## 清理构建文件和进程
	@echo "🧹 清理构建文件和进程..."
	@$(MAKE) stop-all 2>/dev/null || true
	@$(MAKE) dev-stop 2>/dev/null || true
	@rm -rf $(TMP_DIR) $(BIN_DIR) $(LOG_DIR)/*.log
	@$(GO) clean
	@echo "✅ 清理完成"

create-dirs: ## 创建必要的目录
	@mkdir -p $(PID_DIR) $(LOG_DIR) $(BIN_DIR)

# ============================================================================
# 调试和诊断
# ============================================================================

debug: ## 显示调试信息
	@echo "$(COLOR_BOLD)$(COLOR_CYAN)🔍 调试信息:$(COLOR_RESET)"
	@echo "$(COLOR_BOLD)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(COLOR_RESET)"
	@echo "项目名称:     $(PROJECT_NAME)"
	@echo "版本:         $(VERSION)"
	@echo "Git 提交:     $(GIT_COMMIT)"
	@echo "Git 分支:     $(GIT_BRANCH)"
	@echo "构建时间:     $(BUILD_TIME)"
	@echo "Go 版本:      $(shell $(GO) version)"
	@echo "GOPATH:       $(shell go env GOPATH)"
	@echo "GOOS:         $(shell go env GOOS)"
	@echo "GOARCH:       $(shell go env GOARCH)"
	@echo "$(COLOR_BOLD)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(COLOR_RESET)"

# ============================================================================
# 快捷命令
# ============================================================================

up: run ## 启动服务（别名）
down: stop ## 停止服务（别名）
re: restart ## 重启服务（别名）
st: status ## 查看状态（别名）
log: logs ## 查看日志（别名）

# ============================================================================
# 结束
# ============================================================================
