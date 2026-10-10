# IaC平台开发工具

.PHONY: help dev-up dev-down dev-down-all db-init db-reset logs test vet check \
	build-server build-agent build-all \
	update-hcl-grammar update-editor-assets \
	docker-build docker-build-frontend docker-build-agent docker-build-db-init docker-build-all \
	docker-push docker-push-frontend docker-push-agent docker-push-db-init docker-push-all \
	docker-push-arm64 docker-push-frontend-arm64 docker-push-frontend-amd64 \
	docker-push-agent-arm64 docker-push-db-init-arm64 docker-push-all-arm64 \
	run-server run-agent local-server local-frontend local-agent \
	generate-secret dev-certs check-private-keys deploy-local export-seed-data clean

# 默认变量（可通过 .env 文件或环境变量覆盖）
DB_PORT ?= 5432
DB_USER ?= postgres
# DB_PASSWORD / JWT_SECRET / DATA_ENCRYPTION_KEY / SIGNING_ROOT_KEY 没有默认值：
# 由 `make generate-secret` 写入 .env（gitignored），本地运行时从 .env 加载。
DB_NAME ?= iac_platform
SERVER_PORT ?= 8080
CC_SERVER_PORT ?= 8090
DB_HOST ?= localhost

# 在 recipe 的 shell 中加载 .env（make generate-secret 生成，含 DB_PASSWORD 与各密钥）。
# 在 backend/ 目录下执行，故路径为 ../.env。
LOAD_ENV = { [ -f ../.env ] || { echo "缺少 .env，请先运行 make generate-secret" >&2; exit 1; }; } && set -a && . ../.env && set +a

# Docker 镜像配置
DOCKER_REPO ?= w564791
IMAGE_SERVER ?= $(DOCKER_REPO)/iac-platform
IMAGE_FRONTEND ?= $(DOCKER_REPO)/iac-frontend
IMAGE_AGENT ?= $(DOCKER_REPO)/iac-agent
IMAGE_DB_INIT ?= $(DOCKER_REPO)/iac-db-init
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT_HASH ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
PLATFORMS ?= linux/arm64,linux/amd64
LDFLAGS_VERSION = -X iac-platform/internal/version.CommitHash=$(COMMIT_HASH) -X iac-platform/internal/version.BuildTime=$(BUILD_TIME)
DOCKER_BUILD_ARGS = --build-arg COMMIT_HASH=$(COMMIT_HASH) --build-arg BUILD_TIME=$(BUILD_TIME)

help: ## 显示帮助信息
	@echo "IaC平台开发命令:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# =============================================================================
# 开发环境
# =============================================================================

dev-up: ## 启动开发环境（仅数据库容器 + 本地后端 + 本地前端）
	@echo "=========================================="
	@echo "IaC Platform 本地开发环境"
	@echo "=========================================="
	@echo ""
	@echo "1. 启动 PostgreSQL..."
	docker compose up -d postgres
	@echo "等待数据库就绪..."
	@until docker exec iac-platform-postgres pg_isready -U postgres -d iac_platform -q 2>/dev/null; do sleep 1; done
	@echo "  [OK] 数据库就绪"
	@echo ""
	@echo "2. 启动后端..."
	@cd backend && $(LOAD_ENV) && DB_HOST=localhost DB_PORT=$(DB_PORT) DB_USER=$(DB_USER) DB_NAME=$(DB_NAME) \
		DB_PASSWORD=$${DB_PASSWORD:?run make generate-secret} \
		DB_SSLMODE=disable SERVER_PORT=$(SERVER_PORT) CC_SERVER_PORT=$(CC_SERVER_PORT) SERVER_HOST=0.0.0.0 \
		ENV=development \
		go run main.go &
	@sleep 2
	@echo "  [OK] 后端启动: http://localhost:$(SERVER_PORT)"
	@echo ""
	@echo "3. 启动前端..."
	@cd frontend && npm run dev &
	@echo "  [OK] 前端启动: http://localhost:5173"
	@echo ""
	@echo "=========================================="
	@echo "本地开发环境已就绪"
	@echo "  前端: http://localhost:5173"
	@echo "  后端: http://localhost:$(SERVER_PORT)"
	@echo "  数据库: localhost:$(DB_PORT)"
	@echo "=========================================="

dev-down: ## 停止本地前后端进程（保留数据库容器）
	@echo "停止本地进程..."
	-@pkill -f "go run main.go" 2>/dev/null || true
	-@pkill -f "vite" 2>/dev/null || true
	@echo "  [OK] 本地进程已停止（数据库容器保留运行）"

dev-down-all: ## 停止所有（本地进程 + 数据库容器）
	@echo "停止所有服务..."
	-@pkill -f "go run main.go" 2>/dev/null || true
	-@pkill -f "vite" 2>/dev/null || true
	docker compose down
	@echo "  [OK] 全部停止"

db-init: ## 初始化数据库
	@echo "初始化数据库..."
	docker-compose exec postgres psql -U postgres -d iac_platform -c "SELECT 'Database initialized successfully';"

db-reset: ## 重置数据库
	@echo "重置数据库..."
	docker-compose down -v
	docker-compose up -d postgres
	@sleep 5
	@echo "数据库已重置"

logs: ## 查看数据库日志
	docker-compose logs -f postgres

test-db: ## 测试数据库连接
	@echo "测试数据库连接..."
	docker-compose exec postgres psql -U postgres -d iac_platform -c "\dt"

# =============================================================================
# 测试与检查
# =============================================================================

vet: ## 运行 go vet 静态检查
	@echo "运行 go vet..."
	cd backend && go vet ./...

test: ## 运行后端单元测试
	@echo "运行后端单元测试..."
	cd backend && CGO_ENABLED=1 go test -count=1 -timeout 120s $$(go list ./... | grep -v /controllers)

check: vet test ## 运行所有检查（vet + test），构建前必须通过

# =============================================================================
# 本地构建（Go 二进制）
# =============================================================================

build-server: ## 构建服务器二进制文件（当前平台）
	@echo "构建服务器..."
	cd backend && CGO_ENABLED=0 go build -ldflags="-s -w $(LDFLAGS_VERSION)" -o iac-platform main.go
	@echo "服务器构建完成: backend/iac-platform"

build-agent: ## 构建Agent二进制文件（当前平台）
	@echo "构建Agent..."
	cd backend && CGO_ENABLED=0 go build -ldflags="-s -w $(LDFLAGS_VERSION)" -o iac-agent cmd/agent/main.go
	@echo "Agent构建完成: backend/iac-agent"

build-all: build-server build-agent ## 构建所有二进制文件

# =============================================================================
# 前端打包前置（HashiCorp grammar 可选更新；provider schema 已改 execute 落库）
# =============================================================================

update-hcl-grammar: ## 从 hashicorp/syntax 拉取最新 HCL/Terraform TextMate grammar
	@echo "更新 HCL grammar (hashicorp/syntax)..."
	cd frontend && npm run update:hcl-grammar
	@echo "  [OK] grammar 已更新"

update-editor-assets: update-hcl-grammar ## 更新 Manifest 编辑器打包资产(目前仅 grammar)

# =============================================================================
# Docker 镜像构建与推送
# =============================================================================

docker-build: ## 构建后端 Docker 镜像（本地，当前架构）
	@echo "构建镜像: $(IMAGE_SERVER):$(VERSION)"
	docker build \
		-t $(IMAGE_SERVER):$(VERSION) \
		-t $(IMAGE_SERVER):latest \
		$(DOCKER_BUILD_ARGS) \
		backend/
	@echo "镜像构建完成: $(IMAGE_SERVER):$(VERSION)"

docker-build-frontend: ## 构建前端 Docker 镜像（本地，当前架构）
	@echo "构建镜像: $(IMAGE_FRONTEND):$(VERSION)"
	docker build  \
		-t $(IMAGE_FRONTEND):$(VERSION) \
		-t $(IMAGE_FRONTEND):latest \
		frontend/
	@echo "镜像构建完成: $(IMAGE_FRONTEND):$(VERSION)"

docker-build-agent: ## 构建 Agent Docker 镜像（本地，当前架构）
	@echo "构建镜像: $(IMAGE_AGENT):$(VERSION)"
	docker build \
		-t $(IMAGE_AGENT):$(VERSION) \
		-t $(IMAGE_AGENT):latest \
		$(DOCKER_BUILD_ARGS) \
		-f backend/cmd/agent/Dockerfile backend/
	@echo "镜像构建完成: $(IMAGE_AGENT):$(VERSION)"

docker-build-db-init: ## 构建 DB 初始化 Docker 镜像（本地，当前架构）
	@echo "构建镜像: $(IMAGE_DB_INIT):$(VERSION)"
	docker build \
		-t $(IMAGE_DB_INIT):$(VERSION) \
		-t $(IMAGE_DB_INIT):latest \
		manifests/db/
	@echo "镜像构建完成: $(IMAGE_DB_INIT):$(VERSION)"

docker-build-all: check docker-build docker-build-frontend docker-build-agent docker-build-db-init ## 构建所有 Docker 镜像（先运行测试）

docker-push: ## 构建多架构后端镜像并推送 (arm64+amd64)
	@echo "构建并推送: $(IMAGE_SERVER):$(VERSION) [$(PLATFORMS)]"
	docker buildx build --platform $(PLATFORMS) \
		-t $(IMAGE_SERVER):$(VERSION) \
		-t $(IMAGE_SERVER):latest \
		$(DOCKER_BUILD_ARGS) \
		--push backend/
	@echo "推送完成"

docker-push-frontend: ## 构建多架构前端镜像并推送 (arm64+amd64)
	@echo "构建并推送: $(IMAGE_FRONTEND):$(VERSION) [$(PLATFORMS)]"
	docker buildx build --platform $(PLATFORMS) \
		-t $(IMAGE_FRONTEND):$(VERSION) \
		-t $(IMAGE_FRONTEND):latest \
		--push frontend/
	@echo "推送完成"

docker-push-agent: ## 构建多架构 Agent 镜像并推送 (arm64+amd64)
	@echo "构建并推送: $(IMAGE_AGENT):$(VERSION) [$(PLATFORMS)]"
	docker buildx build --platform $(PLATFORMS) \
		-t $(IMAGE_AGENT):$(VERSION) \
		-t $(IMAGE_AGENT):latest \
		$(DOCKER_BUILD_ARGS) \
		-f backend/cmd/agent/Dockerfile --push backend/
	@echo "推送完成"

docker-push-db-init: ## 构建多架构 DB 初始化镜像并推送 (arm64+amd64)
	@echo "构建并推送: $(IMAGE_DB_INIT):$(VERSION) [$(PLATFORMS)]"
	docker buildx build --platform $(PLATFORMS) \
		-t $(IMAGE_DB_INIT):$(VERSION) \
		-t $(IMAGE_DB_INIT):latest \
		--push manifests/db/
	@echo "推送完成"

docker-push-all: check docker-push docker-push-frontend docker-push-agent docker-push-db-init ## 构建并推送所有多架构镜像（先运行测试）

docker-push-arm64: ## 构建 arm64 后端镜像并推送
	@echo "构建并推送: $(IMAGE_SERVER):$(VERSION) [linux/arm64]"
	docker buildx build --platform linux/arm64 \
		-t $(IMAGE_SERVER):$(VERSION) \
		-t $(IMAGE_SERVER):latest \
		$(DOCKER_BUILD_ARGS) \
		--push backend/
	@echo "推送完成"
docker-push-amd64: ## 构建 amd64 后端镜像并推送
	@echo "构建并推送: $(IMAGE_SERVER):$(VERSION) [linux/amd64]"
	docker buildx build --platform linux/amd64 \
		-t $(IMAGE_SERVER):$(VERSION) \
		-t $(IMAGE_SERVER):latest \
		$(DOCKER_BUILD_ARGS) \
		--push backend/
	@echo "推送完成"

docker-push-frontend-arm64: ## 构建 arm64 前端镜像并推送
	@echo "构建并推送: $(IMAGE_FRONTEND):$(VERSION) [linux/arm64]"
	docker buildx build --platform linux/arm64 \
		-t $(IMAGE_FRONTEND):$(VERSION) \
		-t $(IMAGE_FRONTEND):latest \
		--push frontend/
	@echo "推送完成"
docker-push-frontend-amd64: ## 构建 amd64 前端镜像并推送
	@echo "构建并推送: $(IMAGE_FRONTEND):$(VERSION) [linux/amd64]"
	docker buildx build --platform linux/amd64 \
		-t $(IMAGE_FRONTEND):$(VERSION) \
		-t $(IMAGE_FRONTEND):latest \
		--push frontend/
	@echo "推送完成"

docker-push-agent-arm64: ## 构建 arm64 Agent 镜像并推送
	@echo "构建并推送: $(IMAGE_AGENT):$(VERSION) [linux/arm64]"
	docker buildx build --platform linux/arm64 \
		-t $(IMAGE_AGENT):$(VERSION) \
		-t $(IMAGE_AGENT):latest \
		$(DOCKER_BUILD_ARGS) \
		-f backend/cmd/agent/Dockerfile --push backend/
	@echo "推送完成"

docker-push-db-init-arm64: ## 构建 arm64 DB 初始化镜像并推送
	@echo "构建并推送: $(IMAGE_DB_INIT):$(VERSION) [linux/arm64]"
	docker buildx build --platform linux/arm64 \
		-t $(IMAGE_DB_INIT):$(VERSION) \
		-t $(IMAGE_DB_INIT):latest \
		--push manifests/db/
	@echo "推送完成"

docker-push-all-arm64: check docker-push-arm64 docker-push-frontend-arm64 docker-push-agent-arm64 docker-push-db-init-arm64 ## 构建并推送所有 arm64 镜像（先运行测试）

# =============================================================================
# Docker 容器运行（编译后运行）
# =============================================================================

run-server: build-server ## 在Docker容器中运行服务器
	@echo "在Docker容器中启动服务器..."
	docker run --rm --platform linux/arm64 -it \
		-p $(SERVER_PORT):$(SERVER_PORT) \
		-p $(CC_SERVER_PORT):$(CC_SERVER_PORT) \
		-e DB_HOST=$(DB_HOST) \
		-e DB_PORT=$(DB_PORT) \
		-e DB_USER=$(DB_USER) \
		--env-file .env \
		-e DB_NAME=$(DB_NAME) \
		-e DB_SSLMODE=disable \
		-e SERVER_PORT=$(SERVER_PORT) \
		-e CC_SERVER_PORT=$(CC_SERVER_PORT) \
		-e SERVER_HOST=0.0.0.0 \
		-v $(PWD)/backend:/app \
		-w /app \
		golang:1.25 \
		./iac-platform

run-agent: build-agent ## 在Docker容器中运行Agent（需要设置环境变量 IAC_AGENT_TOKEN 和 IAC_AGENT_NAME）
	@echo "在Docker容器中启动Agent..."
	@echo "API端点: http://$(DB_HOST):$(SERVER_PORT)"
	@echo "CC端点: ws://$(DB_HOST):$(CC_SERVER_PORT)"
	@if [ -z "$(IAC_AGENT_TOKEN)" ]; then echo "[ERROR] 请设置 IAC_AGENT_TOKEN 环境变量（从平台 Agent Pool 页面获取）"; exit 1; fi
	@if [ -z "$(IAC_AGENT_NAME)" ]; then echo "[ERROR] 请设置 IAC_AGENT_NAME 环境变量"; exit 1; fi
	docker run --rm -it \
		-e IAC_API_ENDPOINT=$(DB_HOST) \
		-e SERVER_PORT=$(SERVER_PORT) \
		-e CC_SERVER_PORT=$(CC_SERVER_PORT) \
		-e IAC_AGENT_TOKEN=$(IAC_AGENT_TOKEN) \
		-e IAC_AGENT_NAME=$(IAC_AGENT_NAME) \
		-v $(PWD)/backend:/app \
		-w /app \
		amazonlinux:unzip \
		./iac-agent

# =============================================================================
# 本地运行
# =============================================================================

local-server: ## 本地运行后端（自动加载数据库配置）
	@echo "启动后端服务器..."
	cd backend && $(LOAD_ENV) && DB_HOST=localhost DB_PORT=$(DB_PORT) DB_USER=$(DB_USER) DB_NAME=$(DB_NAME) \
		DB_PASSWORD=$${DB_PASSWORD:?run make generate-secret} \
		DB_SSLMODE=disable SERVER_PORT=$(SERVER_PORT) CC_SERVER_PORT=$(CC_SERVER_PORT) SERVER_HOST=0.0.0.0 \
		ENV=development \
		go run main.go

local-frontend: ## 本地运行前端（Vite dev server）
	@echo "启动前端..."
	cd frontend && npm run dev

local-agent: ## 本地运行Agent
	@echo "启动Agent..."
	@echo "请确保已设置环境变量: IAC_API_ENDPOINT, IAC_CC_ENDPOINT, IAC_AGENT_TOKEN, IAC_AGENT_NAME"
	cd backend/cmd/agent && go run main.go

# =============================================================================
# 密钥和环境配置
# =============================================================================

generate-secret: ## 生成平台私钥和 .env 配置文件
	@if [ -f .env ] && grep -q "^JWT_SECRET=" .env 2>/dev/null; then \
		echo "[OK] .env 文件已存在，跳过生成"; \
	else \
		JWT_KEY=$$(openssl rand -base64 48 | tr -d '\n'); \
		echo "# IaC Platform 环境变量配置" > .env; \
		echo "# 自动生成，请勿提交到版本控制" >> .env; \
		echo "" >> .env; \
		echo "# 旧版私钥（仅用于验证无 kid 的旧 token / 解密旧数据）" >> .env; \
		echo "JWT_SECRET=$$JWT_KEY" >> .env; \
		echo "# 数据加密密钥 / 签名根密钥（相互独立，生产环境必填）" >> .env; \
		echo "DATA_ENCRYPTION_KEY=$$(openssl rand -base64 32)" >> .env; \
		echo "SIGNING_ROOT_KEY=$$(openssl rand -base64 48 | tr -d '\n')" >> .env; \
		echo "" >> .env; \
		echo "# 数据库配置" >> .env; \
		echo "DB_HOST=localhost" >> .env; \
		echo "DB_PORT=5432" >> .env; \
		echo "DB_NAME=iac_platform" >> .env; \
		echo "DB_USER=postgres" >> .env; \
		echo "DB_PASSWORD=$$(openssl rand -base64 48 | tr -d '\n/+=')" >> .env; \
		echo "" >> .env; \
		echo "# 服务端口" >> .env; \
		echo "SERVER_PORT=8080" >> .env; \
		echo "CC_SERVER_PORT=8090" >> .env; \
		echo "FRONTEND_PORT=5173" >> .env; \
		echo ""; \
		echo "[OK] .env 配置文件已生成"; \
		echo "  JWT_SECRET / SIGNING_ROOT_KEY: openssl rand -base64 48"; \
		echo "  DB_PASSWORD: 随机生成（已有 postgres 数据卷时需与其中密码一致）"; \
		echo "  DB_PORT: 15433"; \
		echo "  SERVER_PORT: 8080"; \
		echo "  [WARN] 请妥善保管 DATA_ENCRYPTION_KEY / SIGNING_ROOT_KEY，不按轮换流程更换将导致："; \
		echo "     - 所有已登录用户的 Token 失效 (SIGNING_ROOT_KEY)"; \
		echo "     - 所有已加密的变量无法解密 (DATA_ENCRYPTION_KEY)"; \
	fi

# 本地开发 TLS 证书（不提交到仓库）：
#   certs/localhost.pem, certs/localhost-key.pem  -> Vite / 本地后端 HTTPS（文件存在即启用）
#   manifests/tls/certs/tls.crt, tls.key          -> kustomize 生成 Secret iac-gateway-tls
# 优先使用 mkcert（本地 CA 受信任）；未安装时回退到 openssl 自签名证书（浏览器会提示不受信任）。
# 已存在则跳过，FORCE=1 重新生成。
DEV_CERT_DIR ?= certs
DEV_CERT_K8S_DIR ?= manifests/tls/certs
DEV_CERT_HOSTS ?= localhost 127.0.0.1 ::1 www.iac-platform.com iac-platform.com api.iac-platform.com
DEV_CERT_DAYS ?= 825

dev-certs: ## 生成本地开发 TLS 证书（mkcert，缺失时回退 openssl 自签名），写入 gitignored 的 certs/ 与 manifests/tls/certs/
	@set -e; \
	crt=$(DEV_CERT_DIR)/localhost.pem; key=$(DEV_CERT_DIR)/localhost-key.pem; \
	if [ -f "$$crt" ] && [ -f "$$key" ] && [ "$(FORCE)" != "1" ]; then \
		echo "[OK] $$crt 已存在，跳过生成（FORCE=1 重新生成）"; \
	else \
		mkdir -p $(DEV_CERT_DIR); rm -f "$$crt" "$$key"; \
		if command -v mkcert >/dev/null 2>&1; then \
			mkcert -cert-file "$$crt" -key-file "$$key" $(DEV_CERT_HOSTS); \
		else \
			echo "[WARN] 未找到 mkcert，使用 openssl 生成自签名证书（浏览器不信任）"; \
			san=""; for h in $(DEV_CERT_HOSTS); do \
				case "$$h" in *:*|[0-9]*.[0-9]*.[0-9]*.[0-9]*) san="$$san,IP:$$h";; *) san="$$san,DNS:$$h";; esac; \
			done; \
			(umask 077; openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days $(DEV_CERT_DAYS) \
				-subj "/CN=localhost/O=Terranova local development" \
				-addext "subjectAltName=$${san#,}" \
				-keyout "$$key" -out "$$crt" 2>/dev/null); \
		fi; \
		chmod 600 "$$key"; chmod 644 "$$crt"; \
		echo "[OK] 已生成 $$crt / $$key"; \
	fi; \
	mkdir -p $(DEV_CERT_K8S_DIR); \
	cp "$$crt" $(DEV_CERT_K8S_DIR)/tls.crt; \
	(umask 077; cp "$$key" $(DEV_CERT_K8S_DIR)/tls.key); \
	echo "[OK] 已复制到 $(DEV_CERT_K8S_DIR)/tls.crt, tls.key（kustomize Secret iac-gateway-tls）"

check-private-keys: ## 检查已跟踪文件中是否包含私钥（CI 同款检查）
	@scripts/check-private-keys.sh

# =============================================================================
# 部署
# =============================================================================

deploy-local: generate-secret dev-up ## 本地部署（生成密钥 + 启动开发环境，首次访问引导创建管理员）
	@echo ""
	@echo "首次使用请访问 http://localhost:5173 完成管理员初始化"

export-seed-data: ## 从当前数据库导出种子数据
	@echo "导出种子数据..."
	bash scripts/export_seed_data.sh

# =============================================================================
# 清理
# =============================================================================

clean: ## 清理构建文件
	@echo "清理构建文件..."
	rm -f backend/iac-platform backend/iac-agent
	@echo "清理完成"
