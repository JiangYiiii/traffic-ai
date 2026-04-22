# cc-connect 接入微信遥控 Claude Code 部署文档

**最后更新**：2026-04-18  
**执行人**：jiangyi（@本机 macOS）  
**目标**：在本地用 cc-connect 把 Claude Code（主）+ Cursor Agent（作为 Claude 的下游 CLI）桥接到**个人微信**，实现"手机发消息 → Mac 上 AI 真实干活 → 结果回微信"。

---

## 0. 目录

- [1. 项目与版本](#1-项目与版本)
- [2. 最终架构](#2-最终架构)
- [3. 前置环境](#3-前置环境)
- [4. 部署步骤（按卡执行）](#4-部署步骤按卡执行)
  - [卡 1：安装 cc-connect@beta](#卡-1安装-cc-connectbeta)
  - [卡 2：写 config.toml 骨架](#卡-2写-configtoml-骨架)
  - [卡 2.5：安装 Cursor Agent CLI](#卡-25安装-cursor-agent-cli可选)
  - [卡 3：扫码绑定微信 ilink](#卡-3扫码绑定微信-ilink)
  - [卡 4：回填 admin_from + 端到端联调](#卡-4回填-admin_from--端到端联调)
  - [卡 4.5：配代理绕 Anthropic 地域屏蔽](#卡-45配代理绕-anthropic-地域屏蔽)
  - [卡 5：launchd daemon 自启](#卡-5launchd-daemon-自启)
- [5. 关键架构决策记录](#5-关键架构决策记录)
- [6. 日常运维](#6-日常运维)
- [7. 故障处理](#7-故障处理)
- [8. 安全注意](#8-安全注意)
- [9. 产出物清单](#9-产出物清单)
- [10. 后续可选增强（P2）](#10-后续可选增强p2)

---

## 1. 项目与版本

| 组件 | 仓库 / 下载地址 | 版本 |
|---|---|---|
| **cc-connect** | [github.com/chenhg5/cc-connect](https://github.com/chenhg5/cc-connect) · MIT · 5.4k★ | `v1.2.2-beta.5` |
| cc-connect npm | [`cc-connect@beta`](https://www.npmjs.com/package/cc-connect) | 同上 |
| **Claude Code CLI** | [docs.anthropic.com/claude-code](https://docs.anthropic.com/en/docs/claude-code) · 官方 Anthropic | 本机已装（`~/.local/bin/claude`） |
| **Cursor Agent CLI** | [cursor.com/install](https://cursor.com/install) · Anysphere 官方 | `2026.04.17-479fd04` |
| **OpenClaw Weixin 插件**（了解用，未部署） | [github.com/Tencent/openclaw-weixin](https://github.com/Tencent/openclaw-weixin) · 腾讯官方 · 217★ | `@tencent-weixin/openclaw-weixin@2.1.8` |
| iLink Bot API（底层协议） | `https://ilinkai.weixin.qq.com` · 腾讯 2026-03-22 正式发布 | — |
| Node.js | brew | v18+ |
| 代理工具 | [Clash Verge](https://github.com/clash-verge-rev/clash-verge-rev) | 用户本机已装，监听 `127.0.0.1:7897` |

**项目定位说明**：
- **cc-connect**：Go + TypeScript 桥接守护，把多家 IM（飞书/钉钉/微信/Telegram/Slack/Discord…）对接到本地 CLI 型 AI agent（Claude Code / Codex / Cursor / Gemini / 等）。
- **openclaw-weixin**：腾讯官方在 OpenClaw 框架下的微信渠道插件，**和 cc-connect 用同一套 iLink 协议**（cc-connect 复刻了其 HTTP API：`getUpdates` 长轮询 + `sendMessage`）。本方案**没用 OpenClaw**，因为单人本地场景 cc-connect 更轻。

---

## 2. 最终架构

```
  [iPhone 微信]
       │
       │  HTTPS 长轮询 (无需公网 IP，直连腾讯国内 IP 140.207.x.x)
       ▼
  ┌──────────────────────────────────────┐
  │  ilinkai.weixin.qq.com (腾讯官方)     │
  │  • getUpdates / sendMessage          │
  └─────────────────┬────────────────────┘
                    │ bot_token: 718504d5537a@im.bot:...
                    ▼
  ┌──────────────────────────────────────┐
  │  cc-connect 守护 (launchd, PID 随机)  │
  │  ~/Library/LaunchAgents/             │
  │     com.cc-connect.service.plist     │
  │  • RunAtLoad=true + KeepAlive        │
  │  • EnvironmentVariables:             │
  │      HTTPS_PROXY=127.0.0.1:7897      │
  │      NO_PROXY=ilinkai.weixin.qq.com  │
  └─────────────────┬────────────────────┘
                    │ stdin/stdout pipe
                    ▼
  ┌──────────────────────────────────────┐
  │  Claude Code (claude)                │
  │  work_dir = traffic-ai/              │
  │  mode = default (每步工具要确认)     │
  └─────────────────┬────────────────────┘
                    │
         ┌──────────┴──────────┐
         │                     │
         ▼                     ▼
  Anthropic API          Bash 工具 -> `agent -p "..."`
  (via Clash:7897)       (Cursor CLI, 本地进程)
```

**单 project 架构**：cc-connect 的硬约束是"每个 `[[projects]]` 必须至少一个 `[[projects.platforms]]`"，所以 Cursor **不作为独立 project 存在**，而是作为 Claude 的下游 CLI 工具存在。Claude 在需要时自己用 Bash 工具调 `agent -p "<task>"`。

---

## 3. 前置环境

| 项 | 要求 | 本次值 |
|---|---|---|
| 操作系统 | macOS（Linux 也可，daemon 走 systemd） | macOS 24.4.0 (Sonoma+) |
| Node.js | >= 18 | 通过 brew |
| 网络 | 访问 `ilinkai.weixin.qq.com` 直连 + `api.anthropic.com` 走代理 | Clash Verge 分流 |
| 微信号 | **务必用小号**，不要用主号 | user's secondary wechat |
| 仓库路径 | 绝对路径（launchd 不认 `~`） | `/Users/jiangyi/Documents/codedev/traffic-ai` |

---

## 4. 部署步骤（按卡执行）

### 卡 1：安装 cc-connect@beta

```bash
npm install -g cc-connect@beta
cc-connect --version   # => v1.2.2-beta.5
which cc-connect       # => /opt/homebrew/bin/cc-connect
```

**为什么是 beta**：稳定版不含 `weixin` 平台，个人微信 ilink 通道只在 pre-release 里。

### 卡 2：写 config.toml 骨架

安装 cc-connect 会在 `~/.cc-connect/config.toml` 留一份 placeholder 示例。先备份再覆盖：

```bash
cp ~/.cc-connect/config.toml ~/.cc-connect/config.toml.bak.$(date +%Y%m%d%H%M%S)
```

最终骨架（**不要包含真实 token，token 由卡 3 自动写入**）：

```toml
# ~/.cc-connect/config.toml
[log]
level = "info"
attachment_send = "on"

[[projects]]
name = "traffic-ai-claude"
reset_on_idle_mins = 60
# admin_from 卡 4 填

[projects.agent]
type = "claudecode"

[projects.agent.options]
work_dir = "/Users/jiangyi/Documents/codedev/traffic-ai"
mode = "default"           # 远程遥控建议 default，别 yolo

# [[projects.platforms]] 由卡 3 `cc-connect weixin setup` 自动写入
```

验证语法：

```bash
python3 -c "import tomllib, pathlib; tomllib.loads(pathlib.Path.home().joinpath('.cc-connect/config.toml').read_text())" && echo OK
```

### 卡 2.5：安装 Cursor Agent CLI（可选）

如需让 Claude 能调用 Cursor，安装官方 CLI：

```bash
curl -fsS https://cursor.com/install | bash
which agent                          # => ~/.local/bin/agent
agent --version                      # => 2026.04.17-479fd04
```

⚠️ 注意：官方文档旧提示 `npm i -g @anthropic-ai/cursor-agent` 是**过时信息**。Cursor 是 Anysphere 产品，不在 Anthropic 下。

### 卡 3：扫码绑定微信 ilink

```bash
cc-connect weixin setup \
  --project traffic-ai-claude \
  --qr-image /tmp/cc-connect-qr.png \
  --timeout 480
```

行为：
1. 终端打印 ASCII 二维码 + 同步保存 PNG 到 `/tmp/cc-connect-qr.png`
2. macOS 用 `open /tmp/cc-connect-qr.png` 在预览里看，用手机微信扫
3. 手机上点"确认"
4. 命令自动退出，并把 platform 块写回 config：

```toml
[[projects.platforms]]
type = "weixin"

[projects.platforms.options]
token = "<BOT_TOKEN>"                                    # 718504d5537a@im.bot:...
base_url = "https://ilinkai.weixin.qq.com"
account_id = "<BOT_ACCOUNT_ID>"                          # 718504d5537a@im.bot
allow_from = "<YOUR_WXID>"                               # o9cq...@im.wechat
```

**扫码后终端回显的关键信息（务必记录）**：
- `bot token`：配置文件里的 `token` 字段（管控 bot 发消息权限，勿外发）
- `allow_from` / 你的 wxid：形如 `o9cq...@im.wechat`，后面回填 `admin_from` 用

### 卡 4：回填 admin_from + 端到端联调

编辑 `~/.cc-connect/config.toml`，在 `[[projects]]` 下加一行：

```toml
[[projects]]
name = "traffic-ai-claude"
reset_on_idle_mins = 60
admin_from = "<YOUR_WXID>"       # ← 回填卡 3 拿到的 wxid
```

首次验证命令：

```bash
cc-connect --config ~/.cc-connect/config.toml > /tmp/cc-connect-run.log 2>&1 &
```

在微信里找刚绑定的那个 bot 对话，**依次发两条**：

1. `/whoami` —— 应该秒回一个结构化的身份卡，显示你的 wxid 与 session key
2. 一个真任务，比如 `读 demo/userClient/app.html 前 30 行，概述它是什么页面`

**如果第二条返回**：
```
API Error: 400 Access to Anthropic models is not allowed from unsupported countries...
```
恭喜，链路完全通了，只差卡 4.5 的代理配置。

### 卡 4.5：配代理绕 Anthropic 地域屏蔽

本机 Clash Verge 监听 `127.0.0.1:7897`，用它给 Anthropic 走代理，微信走直连。

```bash
# 停掉前台 cc-connect
pkill -f 'cc-connect --config'
sleep 2

# 用代理环境变量重启（注意 NO_PROXY 要覆盖 ilinkai.weixin.qq.com）
HTTPS_PROXY=http://127.0.0.1:7897 \
HTTP_PROXY=http://127.0.0.1:7897 \
NO_PROXY="localhost,127.0.0.1,ilinkai.weixin.qq.com,.weixin.qq.com,.qq.com" \
  cc-connect --config ~/.cc-connect/config.toml > /tmp/cc-connect-run.log 2>&1 &
```

**连通性预检**（可选，出问题时用来定位）：

```bash
curl -x http://127.0.0.1:7897 -o /dev/null -w "anthropic: %{http_code}\n" \
  https://api.anthropic.com/v1/messages -X POST
# 期望: anthropic: 401  （401 = 到达 Anthropic 但没带 key，地域没被拦）

curl -o /dev/null -w "ilink: %{http_code}  ip=%{remote_ip}\n" \
  https://ilinkai.weixin.qq.com
# 期望: ilink: 404  ip=140.207.x.x  （直连国内 IP）
```

微信里重新发那条真任务，应在 15 秒内收到 Claude 对 `app.html` 的真实分析。

### 卡 5：launchd daemon 自启

前台模式依赖 shell，不实用。改用 cc-connect 自带的 daemon 安装命令：

```bash
pkill -f 'cc-connect --config'     # 先停前台
cc-connect daemon install --config ~/.cc-connect/config.toml
```

生成 `~/Library/LaunchAgents/com.cc-connect.service.plist`，其中：
- `RunAtLoad = true` —— 登录 Mac 即启动
- `KeepAlive = {SuccessfulExit: true}` —— 退出后自动拉起
- `EnvironmentVariables` —— **原生不含代理三件套**！

**关键补丁**：给 plist 注入代理环境变量，否则 daemon 模式下 Anthropic 仍会被屏蔽。

直接跑维护脚本（本次已落盘）：

```bash
~/.cc-connect/repatch-proxy.sh
```

脚本内容（幂等，可反复跑）：

```bash
#!/usr/bin/env bash
set -euo pipefail
PLIST="$HOME/Library/LaunchAgents/com.cc-connect.service.plist"
PROXY_URL="http://127.0.0.1:7897"
NO_PROXY_LIST="localhost,127.0.0.1,ilinkai.weixin.qq.com,.weixin.qq.com,.qq.com"

[[ -f "$PLIST" ]] || { echo "plist 缺失，先跑 cc-connect daemon install"; exit 1; }

for key in HTTPS_PROXY HTTP_PROXY NO_PROXY; do
  /usr/libexec/PlistBuddy -c "Delete :EnvironmentVariables:$key" "$PLIST" 2>/dev/null || true
done

/usr/libexec/PlistBuddy \
  -c "Add :EnvironmentVariables:HTTPS_PROXY string $PROXY_URL" \
  -c "Add :EnvironmentVariables:HTTP_PROXY  string $PROXY_URL" \
  -c "Add :EnvironmentVariables:NO_PROXY    string $NO_PROXY_LIST" \
  "$PLIST"

launchctl unload "$PLIST" 2>/dev/null || true
launchctl load   "$PLIST"
cc-connect daemon status
```

**验证 env 已生效**：

```bash
ps eww -p "$(pgrep -f 'cc-connect/bin/cc-connect' | head -1)" \
  | tr ' ' '\n' | grep -E '^(HTTPS?_PROXY|NO_PROXY)='
# 应看到三行环境变量
```

### 附加要求：**让 Clash Verge 也开机自启**

cc-connect 依赖 Clash 在 `127.0.0.1:7897`。Clash Verge → 设置 → 通用 → **开机自启**打开。否则 Mac 刚开机的 1-2 分钟里 cc-connect 能起，但 Anthropic 会超时，Claude 暂时无法回答（KeepAlive 会兜底，等 Clash 起来后就恢复）。

---

## 5. 关键架构决策记录

### 5.1 为什么单 project 而不是双 project + relay

**最初方案**：`traffic-ai-claude`（绑 weixin）+ `traffic-ai-cursor`（不绑平台，靠 `cc-connect relay send` 调用）。

**否决原因**：cc-connect 启动时强制校验 `projects[*].platforms` 非空，报错：
```
Error loading config: config: projects[1] needs at least one [[projects.platforms]]
```

**最终方案**：Cursor 降级为 Claude 的下游 CLI（Bash 工具调 `agent -p "..."`），单 project 架构。好处：
- 对话上下文连续（只有一条 Claude 会话）
- Claude 擅长工具编排，可以自主决定何时用 cursor-agent
- 结构更简单，不必多维护一个 project

### 5.2 为什么不用 OpenClaw / openclaw-weixin

三条路线都被评估过：
- **A**：cc-connect + relay（单人本地首选）← **最终选用**
- **B**：cc-connect + OpenClaw ACP bridge（当 agent 后端）
- **C**：OpenClaw Gateway + openclaw-weixin 插件（完全替换 cc-connect）

OpenClaw 适合**多人团队共享 bot + 工作流编排 + 审计限流**的 ToB 场景。单人本地 + 两个 CLI agent 用 cc-connect 的 relay + Bash 调用就够了，多引入 OpenClaw 是增加维护负担。

### 5.3 为什么代理选 env 变量而不是 cc-connect provider 路由

有两种办法绕 Anthropic 地域屏蔽：
- **方案 1（生产级）**：用 DMXAPI / AICodeMirror / claudeapi.com 等中转服务，改 `ANTHROPIC_BASE_URL`
- **方案 2（快速验证）**：走本地代理 Clash Verge，走环境变量

**本次选方案 2**，原因：
- 零第三方依赖，无额外 API key 管理
- 本机已有 Clash，边际成本为 0
- 微信 ilink 走直连，Anthropic 走代理，规则清晰

**未来需要稳定性 SLA 时可切方案 1**，改动点只有：把 `HTTPS_PROXY` 从 plist 移除，在 cc-connect config 里加 `[[projects.agent.providers]]` + 设置 base_url/api_key。

---

## 6. 日常运维

### 6.1 基础命令

```bash
cc-connect daemon status          # 看状态、PID
cc-connect daemon logs -f         # 实时跟踪日志
cc-connect daemon restart         # 改了 config 后必跑
cc-connect daemon stop            # 临停
cc-connect daemon uninstall       # 卸载 LaunchAgent（此后需重装 + 再跑 repatch-proxy.sh）
```

### 6.2 微信里的 slash 命令

| 命令 | 作用 |
|---|---|
| `/whoami` | 查自己的身份（User ID = wxid） |
| `/status` | 查 cc-connect + agent 当前状态 |
| `/list` | 列出当前 project 的会话历史 |
| `/new <name>` | 起一个命名会话 |
| `/switch <id>` | 切会话 |
| `/dir` / `/dir <path>` / `/dir reset` | 看/切/复位 work_dir（特权，需 admin_from） |
| `/mode default` / `/mode acceptEdits` / `/mode plan` / `/mode bypassPermissions` | 切工具权限模式 |
| `/model` / `/model switch <alias>` | 查/切 Claude 模型 |
| `/memory` | 读写 CLAUDE.md / agent 记忆文件 |
| `/provider list` / `/provider switch <name>` | 切 API provider（目前未配） |
| `/cron add` / `/cron list` / `/cron del <id>` | 定时任务 |
| `/shell <cmd>` | 跑 shell（特权，需 admin_from） |

### 6.3 改配置后的标准动作

```bash
vim ~/.cc-connect/config.toml
cc-connect daemon restart
cc-connect daemon logs -n 30        # 确认没启动报错
```

### 6.4 常用 Claude 调用 Cursor 的示例

在微信里发：
```
用 cursor-agent 把 demo/userClient/app.html 里的 <header> 抽成组件，
改完给我 diff
```

Claude 会自动在 Bash 里跑类似：
```bash
agent -p "在当前目录下，把 app.html 的 header 抽成 components/Header.vue"
```
然后把 stdout 汇总回复到微信。

---

## 7. 故障处理

### 7.1 Claude 报 `400 Access to Anthropic models is not allowed...`

代理失效。排查顺序：
```bash
# 1. Clash Verge 还活着吗
curl -x http://127.0.0.1:7897 -o /dev/null -w "%{http_code}\n" https://api.anthropic.com
# 期望 401；如果 connection refused → Clash 挂了或端口变了

# 2. daemon 的 env 里还有 proxy 吗
ps eww -p "$(pgrep -f 'cc-connect/bin/cc-connect' | head -1)" | tr ' ' '\n' | grep PROXY
# 如果空 → 跑 ~/.cc-connect/repatch-proxy.sh

# 3. 如果还是不行，直接把日志贴出来
cc-connect daemon logs -n 100
```

### 7.2 微信发消息 bot 没反应

```bash
# 1. 服务活着吗
cc-connect daemon status

# 2. ilink 连得上吗
curl -o /dev/null -w "%{http_code}\n" https://ilinkai.weixin.qq.com
# 期望 404（根路径不是 API）

# 3. token 还有效吗（看日志里有没有 weixin: gateway error）
cc-connect daemon logs -n 50 | grep -i weixin

# 4. 实在不行，重新扫码换 token
cc-connect weixin setup --project traffic-ai-claude
```

### 7.3 session 串了（Claude 带着上条对话的奇怪记忆）

```bash
# 微信里
/new 新话题
# 或想彻底清干净
rm ~/.cc-connect/sessions/traffic-ai-claude_*.json
cc-connect daemon restart
```

### 7.4 daemon uninstall 后失去 proxy

```bash
cc-connect daemon install --config ~/.cc-connect/config.toml
~/.cc-connect/repatch-proxy.sh                # 必须跑
```

### 7.5 ilink token 被重置 / 风控

- 现象：日志持续 `weixin: session paused after gateway error`，`401` / `403`
- 解法：`cc-connect weixin setup --project traffic-ai-claude` 重新扫码
- 预防：**不要**用主微信，**不要**刷屏，**不要**把同一 token 同时让 cc-connect 和 openclaw-weixin 两边用

---

## 8. 安全注意

| 风险 | 影响 | 缓解 |
|---|---|---|
| **`config.toml` 含活 token** | 谁拿到就能代你收发微信 | 不要 commit；不要整机共享；`~/.cc-connect/` 设 `chmod 700` |
| **`admin_from` 给了主号而主号被盗** | 攻击者可用 `/shell` 远程执行任意命令 | 绑小号；`admin_from` 只放一个 wxid；敏感期改成空字串（禁所有特权命令） |
| **`/mode bypassPermissions` (YOLO)** | 远程一条消息就可改代码/跑命令 | 默认 `default`；真要开 YOLO 时限时临时切 |
| **`attachment_send = "on"`** | agent 可能把敏感文件回传 | 敏感期改 `"off"`；只用绝对路径；不给 agent 访问 `~/.ssh` / `~/.aws` |
| **ilink 协议官方但新** | 仍有封号可能 | 小号 + 限频 + 手机里把 Mac 标为常用设备 |
| **代理走任意中转** | 可能被中间人看到 prompt | Clash Verge 直出，不经可疑节点；真上生产用方案 1 官方中转 |

---

## 9. 产出物清单

| 路径 | 作用 | 是否机密 |
|---|---|---|
| `~/.cc-connect/config.toml` | 主配置 | **是**（含 bot token） |
| `~/.cc-connect/config.toml.bak.*` | 初始 placeholder 备份 | 否 |
| `~/.cc-connect/logs/cc-connect.log` | 守护日志（自动 rotate 10MB） | 中（可能含 prompt 内容） |
| `~/.cc-connect/sessions/traffic-ai-claude_*.json` | Claude 会话历史 | 中 |
| `~/.cc-connect/run/api.sock` | `cc-connect send/relay` 的本地 socket | 否 |
| `~/.cc-connect/data/weixin/default/` | ilink 的 cursor / context_token 缓存 | **是** |
| `~/.cc-connect/repatch-proxy.sh` | 重装后复原 proxy env 的脚本 | 否 |
| `~/Library/LaunchAgents/com.cc-connect.service.plist` | launchd 配置 | 中 |

---

## 10. 后续可选增强（P2）

| 方向 | 做法 | 价值 |
|---|---|---|
| `CLAUDE.md` | 在 traffic-ai 根加一份，教 Claude 何时调 cursor-agent、项目编码风格 | Claude 行为更稳定 |
| Cron 任务 | 微信发：`/cron add 0 9 * * 1 汇总上周 git log 并输出周报` | 定时跑 agent |
| `run_as_user` 隔离 | 新建 `ccagent` unix 用户，`[[projects]] run_as_user = "ccagent"`，`cc-connect doctor user-isolation` 预检 | 文件系统级防护 |
| 附件回传验证 | `生成架构图 /tmp/arch.png 并发回来` → 验证 `cc-connect send --image` | 完成多模态闭环 |
| 从 env-proxy 迁到 cc-connect provider | 改用 DMXAPI / claudeapi.com，`[[projects.agent.providers]]` + `cc-connect provider add` | 生产级 SLA |
| 迁移到 OpenClaw（如果转团队协作） | `type = "acp"` + `command = "openclaw"` | 多人共享 + 工作流编排 |

---

## 附录 A：本次部署的时间线（供回溯）

| 时间 | 动作 | 结果 |
|---|---|---|
| 08:30 | 卡 1：npm 装 cc-connect@beta | `v1.2.2-beta.5` |
| 08:35 | 卡 2：写 config 骨架，Python tomllib 校验 PASS | 单 project，无 platform |
| 08:35 | 卡 2.5：`curl https://cursor.com/install` 装 Cursor Agent CLI | `agent 2026.04.17-479fd04` |
| 08:47 | 卡 3：`weixin setup`，QR PNG 预览打开 | 扫码成功，token / wxid 写回 config |
| 08:52 | 卡 4：填 `admin_from`，前台启动，微信 `/whoami` 成功 | 链路打通，真任务遇 Anthropic 400 |
| 09:01 | 卡 4.5：Clash Verge `127.0.0.1:7897` + `NO_PROXY` 重启 | 真任务 12s 完成，448 output_tokens |
| 09:06 | 卡 5：`daemon install` + plist 手工注入 proxy env | daemon Running PID 38911 |
| 09:09 | `repatch-proxy.sh` 落盘 + 幂等冒烟 | daemon 切 PID 40529，env 依旧完整 |

---

## 附录 B：配置快速 dump（排障用）

```bash
python3 <<'EOF'
import tomllib, json, pathlib
cfg = tomllib.loads(pathlib.Path.home().joinpath('.cc-connect/config.toml').read_text())
# 脱敏后打印
for p in cfg['projects']:
    for pl in p.get('platforms', []):
        o = pl['options']
        if 'token' in o:
            o['token'] = o['token'][:18] + '...(masked)'
print(json.dumps(cfg, indent=2, ensure_ascii=False))
EOF
```

---

**文档结束**。  
后续有任何改动（新增 project / 换 provider / 加 cron / 出故障）建议附加到 `附录 A` 时间线，保持可追溯。
