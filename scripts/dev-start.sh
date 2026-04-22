#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
CONFIG="$PROJECT_DIR/configs/config.yaml"
# 与 configs/config.yaml 中 server.*_port 对齐，供探活与提示使用（勿写死端口以免改配置后脚本误导）
CONTROL_PORT="$(awk '/^server:/{s=1} s&&/^[^[:space:]#]/{if($0!~/^server:/)exit} s&&/^[[:space:]]*control_port:/{print $2;exit}' "$CONFIG")"
ADMIN_CONTROL_PORT="$(awk '/^server:/{s=1} s&&/^[^[:space:]#]/{if($0!~/^server:/)exit} s&&/^[[:space:]]*admin_control_port:/{print $2;exit}' "$CONFIG")"
GATEWAY_PORT="$(awk '/^server:/{s=1} s&&/^[^[:space:]#]/{if($0!~/^server:/)exit} s&&/^[[:space:]]*gateway_port:/{print $2;exit}' "$CONFIG")"
CONTROL_PORT="${CONTROL_PORT:-8080}"
ADMIN_CONTROL_PORT="${ADMIN_CONTROL_PORT:-8083}"
GATEWAY_PORT="${GATEWAY_PORT:-8081}"
PID_DIR="$PROJECT_DIR/.run"
BIN_DIR="$PROJECT_DIR/bin"
mkdir -p "$PID_DIR"
mkdir -p "$BIN_DIR"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[✓]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
fail() { echo -e "${RED}[✗]${NC} $1"; exit 1; }

# ========== 检查依赖 ==========
check_deps() {
    command -v go    >/dev/null 2>&1 || fail "Go 未安装"
    command -v mysql >/dev/null 2>&1 || fail "mysql client 未安装"
    command -v redis-cli >/dev/null 2>&1 || fail "redis-cli 未安装"
    log "依赖检查通过 (go/mysql/redis-cli)"
}

# ========== 检查服务连通性 ==========
check_services() {
    mysql -u root -h 127.0.0.1 -e "SELECT 1" >/dev/null 2>&1 || fail "MySQL 连接失败 (root@127.0.0.1:3306)"
    redis-cli ping >/dev/null 2>&1 || fail "Redis 连接失败 (127.0.0.1:6379)"
    log "MySQL + Redis 连通正常"
}

# ========== 初始化数据库 ==========
init_db() {
    mysql -u root -h 127.0.0.1 -e "CREATE DATABASE IF NOT EXISTS traffic_ai DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;" 2>/dev/null
    mysql -u root -h 127.0.0.1 traffic_ai < "$PROJECT_DIR/migrations/000001_init_schema.up.sql" 2>/dev/null || true
    for f in "$PROJECT_DIR"/migrations/000[0-9]*_*.up.sql; do
        [ "$f" = "$PROJECT_DIR/migrations/000001_init_schema.up.sql" ] && continue
        [ -f "$f" ] && mysql -u root -h 127.0.0.1 traffic_ai < "$f" 2>/dev/null || true
    done
    TABLE_COUNT=$(mysql -u root -h 127.0.0.1 -N -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='traffic_ai';" 2>/dev/null)
    log "数据库 traffic_ai 就绪 (${TABLE_COUNT} 张表)"
}

# ========== 创建超级管理员 ==========
init_admin() {
    ADMIN_EXISTS=$(mysql -u root -h 127.0.0.1 -N -e "SELECT COUNT(*) FROM traffic_ai.users WHERE role='super_admin';" 2>/dev/null)
    if [ "$ADMIN_EXISTS" = "0" ]; then
        warn "尚未创建管理员账户，启动后可通过以下方式创建:"
        echo "    POST /auth/register/send-code  {\"email\":\"admin@4tk.ai\"}"
        echo "    POST /auth/register            {\"email\":\"admin@4tk.ai\",\"password\":\"...\",\"code\":\"...\"}"
        echo "    然后手动提权: mysql -u root -h 127.0.0.1 -e \"UPDATE traffic_ai.users SET role='super_admin' WHERE email='admin@4tk.ai';\""
    else
        log "管理员账户已存在"
    fi
}

# ========== 编译 ==========
build_all() {
    cd "$PROJECT_DIR"
    export GOPROXY=https://goproxy.cn,direct
    log "编译 control …"
    go build -o "$BIN_DIR/control" ./cmd/control
    log "编译 gateway …"
    go build -o "$BIN_DIR/gateway" ./cmd/gateway
    log "编译完成 → $BIN_DIR/control, $BIN_DIR/gateway"
}

# ========== 启动控制面 ==========
start_control() {
    if [ -f "$PID_DIR/control.pid" ] && kill -0 "$(cat "$PID_DIR/control.pid")" 2>/dev/null; then
        warn "控制面已在运行 (PID $(cat "$PID_DIR/control.pid"))"
        return
    fi
    cd "$PROJECT_DIR"
    nohup "$BIN_DIR/control" -config "$CONFIG" > "$PID_DIR/control.log" 2>&1 &
    echo $! > "$PID_DIR/control.pid"
    sleep 2
    if curl -s "http://127.0.0.1:${CONTROL_PORT}/healthz" >/dev/null 2>&1; then
        log "用户控制面启动成功 → http://127.0.0.1:${CONTROL_PORT}"
    else
        warn "用户控制面 (${CONTROL_PORT}) 探活失败，请检查 $PID_DIR/control.log"
    fi
    if curl -s "http://127.0.0.1:${ADMIN_CONTROL_PORT}/healthz" >/dev/null 2>&1; then
        log "管理后台启动成功 → http://127.0.0.1:${ADMIN_CONTROL_PORT}"
    else
        warn "管理后台 (${ADMIN_CONTROL_PORT}) 探活失败，请检查 $PID_DIR/control.log"
    fi
}

# ========== 启动数据面 ==========
start_gateway() {
    if [ -f "$PID_DIR/gateway.pid" ] && kill -0 "$(cat "$PID_DIR/gateway.pid")" 2>/dev/null; then
        warn "数据面已在运行 (PID $(cat "$PID_DIR/gateway.pid"))"
        return
    fi
    cd "$PROJECT_DIR"
    nohup "$BIN_DIR/gateway" -config "$CONFIG" > "$PID_DIR/gateway.log" 2>&1 &
    echo $! > "$PID_DIR/gateway.pid"
    sleep 2
    if curl -s "http://127.0.0.1:${GATEWAY_PORT}/healthz" >/dev/null 2>&1; then
        log "数据面启动成功 → http://127.0.0.1:${GATEWAY_PORT}"
    else
        warn "数据面 (${GATEWAY_PORT}) 探活失败，请检查 $PID_DIR/gateway.log"
    fi
}

# ========== 停止服务 ==========
stop_all() {
    for svc in control gateway; do
        if [ -f "$PID_DIR/$svc.pid" ]; then
            PID=$(cat "$PID_DIR/$svc.pid")
            if kill -0 "$PID" 2>/dev/null; then
                kill "$PID" 2>/dev/null
                log "$svc 已停止 (PID $PID)"
            fi
            rm -f "$PID_DIR/$svc.pid"
        fi
    done
}

# ========== 查看状态 ==========
status() {
    for svc in control gateway; do
        if [ -f "$PID_DIR/$svc.pid" ] && kill -0 "$(cat "$PID_DIR/$svc.pid")" 2>/dev/null; then
            log "$svc 运行中 (PID $(cat "$PID_DIR/$svc.pid"))"
        else
            warn "$svc 未运行"
        fi
    done
}

# ========== 主入口 ==========
case "${1:-start}" in
    start)
        echo "========== traffic-ai 开发环境启动 =========="
        check_deps
        check_services
        init_db
        init_admin
        build_all
        start_control
        start_gateway
        echo "========== 启动完毕 =========="
        echo "  用户控制面: http://127.0.0.1:${CONTROL_PORT}"
        echo "  管理后台:   http://127.0.0.1:${ADMIN_CONTROL_PORT}"
        echo "  数据面网关: http://127.0.0.1:${GATEWAY_PORT}"
        echo "  日志:   $PID_DIR/control.log | gateway.log"
        echo "  停止:   $0 stop"
        ;;
    stop)
        stop_all
        ;;
    restart)
        stop_all
        sleep 1
        exec "$0" start
        ;;
    status)
        status
        ;;
    *)
        echo "用法: $0 {start|stop|restart|status}"
        exit 1
        ;;
esac
