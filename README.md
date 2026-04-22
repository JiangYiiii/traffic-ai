# traffic-ai

面向大模型调用的**流量控制系统**：提供用户控制台（嵌入 Web 前端）、管理后台与 OpenAI 兼容的数据面网关；基于 **MySQL** 持久化与 **Redis** 限流/扣费，双进程部署（控制面 + 网关）。

## 架构速览

| 进程 | 默认端口 | 说明 |
|------|-----------|------|
| `control` | **8080**（用户控制台 + API）、**8083**（管理后台 + API） | 认证、API Key、模型与路由配置、计费与审计；静态页面内嵌于二进制 |
| `gateway` | **8081** | API Key 鉴权、限流、上游转发（如 `/v1/chat/completions`） |

配置见 `configs/config.yaml`（默认连接本机 `127.0.0.1:3306`、`127.0.0.1:6379`，数据库名 `traffic_ai`）。

## 前置条件

- Go（与 `go.mod` 一致）
- 本机 **MySQL**、**Redis** 已启动，且与 `configs/config.yaml` 中账号一致（脚本默认使用 `mysql -u root -h 127.0.0.1` 与 `redis-cli`）
- 可选：`mysql`、`redis-cli` 命令行客户端（开发脚本用于检查连通与初始化库表）

## 启动（推荐）

在项目根目录执行：

```bash
./scripts/dev-start.sh start
```

脚本会：检查依赖与 MySQL/Redis 连通性 → 创建库并导入 `migrations/000001_init_schema.up.sql`（若尚未存在）→ 编译 `bin/control` 与 `bin/gateway` → 后台启动并探测 `http://localhost:8080/healthz` 与 `http://localhost:8081/healthz`。

## 编译与重启

日常**重新编译并重启**控制面与网关：

```bash
./scripts/dev-start.sh restart
```

## 停止与状态

```bash
./scripts/dev-start.sh stop      # 停止 control / gateway
./scripts/dev-start.sh status    # 查看是否在跑
```

日志：`.run/control.log`、`.run/gateway.log`。

## 仅编译（不启动）

```bash
make build
# 产出 bin/control、bin/gateway；手动运行示例：
# ./bin/control -config configs/config.yaml
# ./bin/gateway -config configs/config.yaml
```

## 访问地址（默认）

- 用户控制台：http://localhost:8080  
- 管理后台：http://localhost:8083  
- 数据面网关：http://localhost:8081  

（请勿在仓库中提交 API Key；使用环境变量或本地未跟踪的配置文件。）
