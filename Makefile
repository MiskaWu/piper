# 預設目標：顯示所有可用命令
.DEFAULT_GOAL := help

BINARY := piper
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
comma := ,
PRIVATE_IP := $(shell hostname -I 2>/dev/null | awk '{print $$1}')
BIND_HOSTS := $(if $(PRIVATE_IP),127.0.0.1$(comma)$(PRIVATE_IP),127.0.0.1)

.PHONY: help run dev-ui dev-backend build clean

help:
	@echo ""
	@echo "可用命令："
	@echo "-------------------"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""

run: ## 啟動已編譯的執行檔（自動偵測 private IP）
	@test -f ./$(BINARY) || { echo "錯誤：$(BINARY) 不存在，請先執行 make build"; exit 1; }
	./$(BINARY) -host $(BIND_HOSTS)

dev-ui: ## 啟動前端開發伺服器（Vite）
	cd ui && npm run dev

dev-backend: ## 啟動後端開發伺服器（Go + Air hot-reload）
	@test -f $$(go env GOPATH)/bin/air || { \
		echo "air 未安裝，正在安裝 github.com/air-verse/air@latest ..."; \
		go install github.com/air-verse/air@latest; \
	}
	$$(go env GOPATH)/bin/air

build: static/index.html ## 建置（前端 + 後端 → 單一執行檔）
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) .

ui/node_modules: ui/package-lock.json
	cd ui && npm ci
	@touch $@

static/index.html: ui/node_modules $(shell find ui/src -type f 2>/dev/null)
	cd ui && npm run build

clean: ## 清理建置產物
	rm -rf static/* $(BINARY)
