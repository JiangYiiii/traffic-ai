# 4tk.ai 控制台（app.html#tokenSection）信息摘录与接口推理

来源页面：<https://4tk.ai/app.html#tokenSection>  
抓取时间：2026-04-14  
说明：控制台数据接口为**同源**相对路径（页面所在域，如 `https://4tk.ai`）；对话/模型调用走 **`https://api.4tk.ai`**。依据站点公开脚本 `/js/app.js` 整理。

---

## 一、页面功能与文案结构（信息复制）

### 导航与全局

- 品牌：4tk.ai 控制台
- 语言：中文 / English
- 链接：接入文档 → `/docs`
- 按钮：刷新数据（触发重新拉取仪表盘与表格）
- 管理员后台：`/admin.html`（仅当用户 `group === "admin"` 时显示）
- 超级管理员后台：`/super-admin.html`（仅当 `group === "super_admin"` 时显示）
- 退出登录：清除 `localStorage.accessToken` 并跳转 `/login.html`（**无服务端 logout API 调用**）

### 工作台概览（#overviewSection）

- 标题：统一管理令牌、余额和模型调用
- 副标题：从一个页面完成充值、令牌创建、余额提醒、模型测试和消费追踪。
- 展示芯片：`heroEmail`、`heroGroup`、`heroInviteCode`（来自个人资料）

### KPI 四格

| 区域 ID        | 文案     | 含义（脚本字段）                    |
|----------------|----------|-------------------------------------|
| kpiBalance     | 账户余额 | `dashboard.balanceMicroUsd`（微美元） |
| kpiConsumed    | 累计消费 | `dashboard.totalConsumedMicroUsd`   |
| kpiCalls       | 调用次数 | `dashboard.totalCalls`              |
| kpiTokens      | 活跃令牌 | `dashboard.activeTokenCount`        |

### 余额邮件提醒（#balanceAlertSection）

- 说明：当账户余额低于下方金额时，向注册邮箱发提醒（每个「低于阈值」周期至多一封；充值回到阈值以上后会再次提醒）。
- 控件：启用余额提醒（checkbox）、提醒阈值 USD（数字）、保存设置。
- 折叠条摘要：`balanceAlertSummaryText`（随语言与开关变化）。

### 兑换码充值（#redeemSection）

- 说明：使用兑换码即时充值，结果同步写入余额流水。
- 输入：兑换码（占位示例 `RC-XXXXXXXXXXXXXXX`）、立即兑换。

### 令牌列表（#tokenSection）

- 说明：按环境与分组创建子令牌，复制后在客户端、脚本或 IDE 中使用。
- 提示：请保管好您的令牌，避免泄露给任何人。
- 表格列：名称、分组、令牌、状态、最后使用、操作（复制 / 启用 / 停用 / 删除等，以页面为准）。
- 添加令牌：弹窗填写令牌名称、令牌分组（默认 `default`）；创建成功后弹窗展示**完整明文**供复制。

### 模型定价（#pricingSection）

- 说明：复制模型名并核对当前单价；按 token 展示输入/输出单价，按次展示单次价格。

### 调用日志（#usageSection）

- 说明：筛选最近调用，定位模型、推理强度、分组、流式状态和耗时。
- 筛选：流式（全部 / 流式 / 非流式）、模型关键字、「筛选日志」。
- 表格列（脚本字段示例）：时间、类型、令牌名、分组、模型、推理强度、耗时、流式、提示 token、补全 token、缓存创建、缓存读取、消费 USD、IP、说明。

### 余额流水（#balanceSection）

- 说明：查看充值、扣费和余额变化。
- 列：时间、变动、变动前、变动后、类型（`reasonType`）。

### 对话测试（#chatTestSection）

- 说明：用子令牌直接测当前账户可用模型。
- 令牌：下拉（来自 `/me/tokens` 与本地缓存的明文）。
- 接口模式：`openai`（OpenAI 对话）、`responses`（OpenAI Responses）、`anthropic`（Anthropic Messages）、`gemini-chat`、`gemini-image`。
- 响应格式：流式 SSE / 非流式 JSON（Gemini 生图等模式另有宽高比、图像尺寸、参考图多选）。

---

## 二、前端鉴权

- 登录后 `localStorage` 键名：`accessToken`。
- 所有 `api()` 请求：`Authorization: Bearer <accessToken>`，`Content-Type: application/json`。
- `401`：清 token 并跳转登录页。

---

## 三、控制台 REST 接口（同源，路径以 `/` 开头）

以下均由 `app.js` 中 `api(path)` 调用，**基址为网站源站**（例如 `https://4tk.ai`）。

| 方法   | 路径 | 作用（推理） |
|--------|------|----------------|
| GET    | `/account/profile` | 拉取用户资料 `profile`、仪表盘 KPI `dashboard`、余额提醒配置 `balanceAlert`；用于首屏与「刷新数据」。 |
| GET    | `/me/tokens` | 列出当前账户子令牌（含 id、name、tokenGroup、前缀、是否活跃、最后使用时间等）；创建后仅一次返回完整 `token`。 |
| POST   | `/me/tokens` | 创建子令牌，body：`{ name, tokenGroup }`；响应含新令牌明文。 |
| PATCH  | `/me/tokens/:id/disable` | 停用指定子令牌。 |
| PATCH  | `/me/tokens/:id/enable` | 启用指定子令牌。 |
| DELETE | `/me/tokens/:id` | 删除子令牌。 |
| GET    | `/me/usage-logs?limit=50&stream=&model=` | 分页/筛选调用日志；`stream` 过滤是否流式，`model` 过滤模型名。 |
| GET    | `/me/balance/logs?limit=50` | 最近余额流水（充值、扣费等）。 |
| GET    | `/me/model-pricing` | 各模型定价（按 token 或按次）。 |
| PATCH  | `/me/balance-alert` | 更新余额邮件提醒，body：`{ balanceAlertEnabled, balanceAlertUsd }`。 |
| POST   | `/me/balance/redeem` | 兑换码充值，body：`{ code }`。 |

---

## 四、模型网关（api.4tk.ai）

聊天测试使用常量 `CHAT_API_BASE = "https://api.4tk.ai"`，对子令牌使用 **Bearer**（Anthropic 模式使用 `x-api-key` + `anthropic-version`）。

| 场景 | 方法 | 路径 | 作用（推理） |
|------|------|------|----------------|
| OpenAI 兼容对话 | POST | `/v1/chat/completions` | 兼容 OpenAI Chat Completions；`stream` 控制 SSE。 |
| OpenAI Responses | POST | `/v1/responses` | OpenAI Responses API 形态。 |
| Anthropic | POST | `/v1/messages` | Claude Messages；请求头模拟官方。 |
| Gemini 文本 | POST | `/v1beta/models/{model}:generateContent` 或 `:streamGenerateContent` | Gemini 生成/流式。 |
| Gemini 生图 | POST | `/v1beta/models/{model}:generateContent` | `generationConfig` 含 `imageConfig`、`responseModalities: ["IMAGE"]` 等。 |

若对 `api.4tk.ai` 的跨域请求失败，脚本会**回退**到 `window.location.origin` 同路径再试。

---

## 五、汇总成「一个问题」（便于你写文档或评审）

**问题：**  
4tk.ai 控制台在登录后通过同源 API（如 `GET /account/profile`、`GET /me/tokens`、`GET /me/usage-logs`、`GET /me/balance/logs`、`GET /me/model-pricing`，以及 `POST/PATCH/DELETE /me/tokens*`、`PATCH /me/balance-alert`、`POST /me/balance/redeem`）完成账户概览、子令牌生命周期、调用与账务流水、模型定价与余额提醒配置；实际模型调用则统一走 `https://api.4tk.ai` 下的 OpenAI/Responses/Anthropic/Gemini 兼容路径（如 `/v1/chat/completions`、`/v1/responses`、`/v1/messages`、`/v1beta/models/...`），并用子令牌作为 Bearer（或 Anthropic 的 x-api-key）——请确认上述路由与字段是否与官方 OpenAI/Anthropic/Gemini 文档一致、哪些能力由网关扩展（如推理强度、缓存 token 计费），以及同源 API 与 `api.4tk.ai` 在鉴权、限流和错误码上如何对应？

---

## 六、本地脚本依据

- 页面：`https://4tk.ai/app.html`
- 逻辑：`https://4tk.ai/js/app.js?v=20260406-chat-test-anthropic`（已用 curl 拉取并解析）

若你需要**带真实响应 JSON 的样例**（含余额数值、令牌列表），请在已登录状态下由你在浏览器里打开开发者工具 Network 导出 HAR，或告知我再尝试其他抓取方式。
