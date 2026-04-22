.PHONY: all build run-control run-gateway test lint migrate-up migrate-down clean sync-web-console check-web-console-sync

# 前端资源同步约定：
#   - 构建产物唯一来源：internal/interfaces/api/static/（由 Go //go:embed 打包）
#   - web/console/ 是便于本地查阅的镜像副本，构建时不会被读取
#   - 修改必须先改 static/，再通过 make sync-web-console 同步到 web/console/
#   - CI/本地可通过 make check-web-console-sync 校验两份是否一致

GO=go
GOFLAGS=-ldflags="-s -w"
BIN_DIR=bin

all: build

build: build-control build-gateway

build-control:
	$(GO) build $(GOFLAGS) -o $(BIN_DIR)/control ./cmd/control

build-gateway:
	$(GO) build $(GOFLAGS) -o $(BIN_DIR)/gateway ./cmd/gateway

run-control:
	$(GO) run ./cmd/control -config configs/config.yaml

run-gateway:
	$(GO) run ./cmd/gateway -config configs/config.yaml

test:
	$(GO) test ./... -v -count=1

lint:
	golangci-lint run ./...

migrate-up:
	golang-migrate -path migrations -database "mysql://root@tcp(127.0.0.1:3306)/traffic_ai" up

migrate-down:
	golang-migrate -path migrations -database "mysql://root@tcp(127.0.0.1:3306)/traffic_ai" -verbose down 1

clean:
	rm -rf $(BIN_DIR)

# 把 static/ 的全部内容镜像到 web/console/（--delete 会清理 web/console 中不存在于 static 的孤立文件）
# README.md 是镜像目录自身的说明文件，仅存在于 web/console/，不参与同步
sync-web-console:
	rsync -a --delete --exclude='README.md' internal/interfaces/api/static/ web/console/
	@echo "web/console 已从 internal/interfaces/api/static/ 同步"

# CI/本地校验两份镜像是否完全一致；不一致则提示需要运行 sync-web-console
check-web-console-sync:
	@diff -qr --exclude='README.md' internal/interfaces/api/static web/console >/dev/null 2>&1 \
		&& echo "web/console 与 static 已同步" \
		|| (echo "web/console 与 static 不同步，请运行: make sync-web-console" && diff -qr --exclude='README.md' internal/interfaces/api/static web/console; exit 1)
