const t = (k) => (window.I18N ? window.I18N.t(k) : k);

function getAccessToken() {
  return localStorage.getItem("accessToken");
}

if (!getAccessToken()) window.location.href = "/login.html";

let currentUserGroup = "default";
let cachedProfile = null;
let cachedMeTokens = [];
let cachedPricingModels = [];
let cachedModelHealth = {};
const RECENT_CHAT_TEST_CFG_KEY = "traffic_ai_recent_chat_test_cfg_v1";
const CHAT_TEST_HISTORY_KEY = "traffic_ai_chat_test_history_v1";

const DEFAULT_PAGE_SIZE = 10;
const usageState = { page: 1, total: 0, pageSize: DEFAULT_PAGE_SIZE };
const balanceState = { page: 1, total: 0, pageSize: DEFAULT_PAGE_SIZE };

function renderPager({ infoId, prevId, nextId, state, pageSize }) {
  const info = document.getElementById(infoId);
  const prev = document.getElementById(prevId);
  const next = document.getElementById(nextId);
  if (!info || !prev || !next) return;
  const total = Number(state.total) || 0;
  const page = Number(state.page) || 1;
  const pages = total > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
  info.textContent = t("app.pageInfo")
    .replace("{page}", String(page))
    .replace("{pages}", String(pages))
    .replace("{total}", String(total));
  prev.disabled = page <= 1;
  next.disabled = page >= pages || total === 0;
}

function gatewayBase() {
  return `${window.location.protocol}//${window.location.hostname}:8081`;
}

function adminConsoleHref() {
  const m = document.querySelector('meta[name="traffic-ai-admin-port"]');
  if (!m?.content) return "/admin.html";
  const port = m.content.trim();
  return `${window.location.protocol}//${window.location.hostname}:${port}/admin.html`;
}

function pickTokenField(row, camel, snake) {
  if (row && Object.prototype.hasOwnProperty.call(row, camel)) return row[camel];
  if (row && Object.prototype.hasOwnProperty.call(row, snake)) return row[snake];
  return undefined;
}

const TOKEN_EYE_SVG =
  '<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>';

async function api(path, options = {}) {
  const token = getAccessToken();
  const resp = await fetch(path, {
    ...options,
    headers: {
      "content-type": "application/json",
      authorization: `Bearer ${token}`,
      ...(options.headers || {}),
    },
  });
  if (resp.status === 401) {
    localStorage.removeItem("accessToken");
    window.location.href = "/login.html";
    return null;
  }
  const body = await resp.json().catch(() => ({}));
  if (typeof body.code !== "number" || body.code !== 0) {
    throw new Error(body.message || t("common.requestFailed"));
  }
  return body.data;
}

/** 微美元整数字符串 -> $x.xxxxxx */
function usdFromMicroStr(str) {
  if (str === undefined || str === null || str === "") return "$0.000000";
  const raw = String(str);
  const neg = raw.startsWith("-");
  const s = neg ? raw.slice(1) : raw;
  if (!/^\d+$/.test(s)) return "$?";
  const intPart = s.length <= 6 ? "0" : s.slice(0, -6);
  const frac = s.length <= 6 ? s.padStart(6, "0") : s.slice(-6);
  return `${neg ? "-" : ""}$${intPart}.${frac}`;
}

function fmtTime(t) {
  if (!t) return "-";
  return new Date(t).toLocaleString();
}

function toDatetimeLocalInputValue(date) {
  const d = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return d.toISOString().slice(0, 16);
}

function applyUsageQuickRange(hoursBack) {
  const endEl = document.getElementById("filterEnd");
  const startEl = document.getElementById("filterStart");
  if (!startEl || !endEl) return;
  const end = new Date();
  const start = new Date(end.getTime() - hoursBack * 60 * 60 * 1000);
  startEl.value = toDatetimeLocalInputValue(start);
  endEl.value = toDatetimeLocalInputValue(end);
}

function ensureChatTestModelOption(value) {
  const modelEl = document.getElementById("chatTestModel");
  if (!modelEl || !value) return;
  if ([...modelEl.options].some((opt) => opt.value === value)) return;
  if (!cachedPricingModels.some((r) => String(r?.model || "").trim() === value)) {
    cachedPricingModels.push({ model: value, provider: "openai" });
  }
  const opt = document.createElement("option");
  opt.value = value;
  opt.textContent = value;
  modelEl.appendChild(opt);
}

function inferModelCapabilities(row) {
  const model = String(row?.model || "").toLowerCase();
  const provider = String(row?.provider || row?.provider_name || "").toLowerCase();
  if (provider.includes("anthropic") || model.includes("claude")) return ["anthropic"];
  if (provider.includes("gemini") || provider.includes("google") || model.includes("gemini")) {
    if (model.includes("image") || model.includes("imagen")) return ["gemini-image"];
    return ["gemini-chat"];
  }
  return ["openai", "responses"];
}

function modeToCapability(mode) {
  if (mode === "anthropic") return "anthropic";
  if (mode === "gemini-image") return "gemini-image";
  if (mode === "gemini-chat") return "gemini-chat";
  if (mode === "responses") return "responses";
  return "openai";
}

function renderChatTestModelOptions(preferredValue = "") {
  const modelSel = document.getElementById("chatTestModel");
  const modeEl = document.getElementById("chatTestApiMode");
  if (!modelSel || !modeEl) return;
  const mode = normalizeChatTestApiMode(modeEl.value);
  const cap = modeToCapability(mode);
  const rows = Array.isArray(cachedPricingModels) ? cachedPricingModels : [];
  const allowed = rows.filter((r) => inferModelCapabilities(r).includes(cap));
  const names = Array.from(new Set(allowed.map((r) => String(r.model || "").trim()).filter(Boolean)));

  const prev = preferredValue || String(modelSel.value || "").trim();
  modelSel.innerHTML = "";
  const ph = document.createElement("option");
  ph.value = "";
  ph.textContent = t("app.chatTestModelPlaceholder");
  modelSel.appendChild(ph);
  names.forEach((name) => {
    const opt = document.createElement("option");
    opt.value = name;
    const h = cachedModelHealth[name];
    const s = h ? h.success : 0;
    const e = h ? h.error : 0;
    opt.textContent = `${name} [${healthLabelForModel(name)} S${s}/E${e}]`;
    modelSel.appendChild(opt);
  });
  if (prev && names.includes(prev)) {
    modelSel.value = prev;
  } else if (names.length > 0) {
    modelSel.value = names[0];
  } else {
    modelSel.value = "";
  }
  syncChatTestModelHealthHint();
}

function saveRecentChatTestConfig(cfg) {
  try {
    localStorage.setItem(RECENT_CHAT_TEST_CFG_KEY, JSON.stringify(cfg || {}));
  } catch {}
}

function loadRecentChatTestConfig() {
  try {
    const raw = localStorage.getItem(RECENT_CHAT_TEST_CFG_KEY);
    if (!raw) return null;
    const x = JSON.parse(raw);
    return x && typeof x === "object" ? x : null;
  } catch {
    return null;
  }
}

function loadChatTestHistory() {
  try {
    const raw = localStorage.getItem(CHAT_TEST_HISTORY_KEY);
    const arr = raw ? JSON.parse(raw) : [];
    return Array.isArray(arr) ? arr : [];
  } catch {
    return [];
  }
}

function saveChatTestHistoryItem(item) {
  const next = [item, ...loadChatTestHistory()]
    .filter((x) => x && x.model && x.apiMode)
    .slice(0, 10);
  localStorage.setItem(CHAT_TEST_HISTORY_KEY, JSON.stringify(next));
}

function renderChatTestHistoryOptions() {
  const sel = document.getElementById("chatTestHistorySelect");
  if (!sel) return;
  const history = loadChatTestHistory();
  sel.innerHTML = "";
  const ph = document.createElement("option");
  ph.value = "";
  ph.textContent = "最近请求历史（可回填）";
  sel.appendChild(ph);
  history.forEach((h, idx) => {
    const opt = document.createElement("option");
    opt.value = String(idx);
    const when = h.at ? fmtTime(h.at) : "-";
    opt.textContent = `${h.apiMode} / ${h.model} / ${when}`;
    sel.appendChild(opt);
  });
}

function healthLabelForModel(modelName) {
  const h = cachedModelHealth[String(modelName || "")];
  if (!h) return "未知";
  if (h.error > 0 && h.success === 0) return "异常";
  if (h.error > 0) return "告警";
  if (h.success > 0) return "正常";
  return "未知";
}

function modelHealthSummary(modelName) {
  const name = String(modelName || "");
  const h = cachedModelHealth[name];
  if (!h) return "最近200条调用中暂无该模型记录";
  const lastErr = h.lastErrorAt ? fmtTime(h.lastErrorAt) : "无";
  return `状态：${healthLabelForModel(name)}，成功 ${h.success} 次，失败 ${h.error} 次，最近失败：${lastErr}`;
}

function syncChatTestModelHealthHint() {
  const modelEl = document.getElementById("chatTestModel");
  const hintEl = document.getElementById("chatTestModelHealthHint");
  if (!modelEl || !hintEl) return;
  const model = String(modelEl.value || "").trim();
  hintEl.textContent = model ? modelHealthSummary(model) : "选择模型后显示健康状态";
}

function applyRecentChatTestConfig() {
  const cfg = loadRecentChatTestConfig();
  if (!cfg) return;
  const modeEl = document.getElementById("chatTestApiMode");
  const streamEl = document.getElementById("chatTestStreamMode");
  const tokenEl = document.getElementById("chatTestTokenSelect");
  const modelEl = document.getElementById("chatTestModel");
  const promptEl = document.getElementById("chatTestPrompt");

  if (modeEl && cfg.apiMode) modeEl.value = String(cfg.apiMode);
  syncChatTestApiModeUi();
  if (streamEl && cfg.streamMode) streamEl.value = String(cfg.streamMode);
  if (tokenEl && cfg.tokenId && [...tokenEl.options].some((o) => o.value === String(cfg.tokenId))) {
    tokenEl.value = String(cfg.tokenId);
  }
  if (modelEl && cfg.model) {
    ensureChatTestModelOption(String(cfg.model));
    modelEl.value = String(cfg.model);
  }
  if (promptEl && cfg.prompt) promptEl.value = String(cfg.prompt);
  syncChatTestModelHealthHint();
}

function setText(id, value) {
  const el = document.getElementById(id);
  if (el) el.textContent = value;
}

function escapeHtml(value) {
  return String(value ?? "").replace(/[&<>"']/g, (char) => {
    switch (char) {
      case "&":
        return "&amp;";
      case "<":
        return "&lt;";
      case ">":
        return "&gt;";
      case '"':
        return "&quot;";
      case "'":
        return "&#39;";
      default:
        return char;
    }
  });
}

function disposeBootstrapTooltips(root = document) {
  const Tooltip = window.bootstrap?.Tooltip;
  if (!Tooltip || !root?.querySelectorAll) return;
  root.querySelectorAll('[data-bs-toggle="tooltip"]').forEach((el) => {
    Tooltip.getInstance(el)?.dispose();
  });
}

function initBootstrapTooltips(root = document) {
  const Tooltip = window.bootstrap?.Tooltip;
  if (!Tooltip || !root?.querySelectorAll) return;
  root.querySelectorAll('[data-bs-toggle="tooltip"]').forEach((el) => {
    Tooltip.getOrCreateInstance(el, {
      container: "body",
      trigger: "hover focus",
    });
  });
}

function syncProfileMeta(profile = cachedProfile) {
  if (!profile) return;
  const group = profile.group || "default";
  const metaParts = [profile.email];
  if (group !== "default") {
    metaParts.push(`${t("common.group")}: ${group}`);
  }
  metaParts.push(`${t("common.inviteCode")}: ${profile.inviteCode || profile.invite_code || "-"}`);
  setText("userMeta", metaParts.join(" | "));
  setText("heroEmail", profile.email || "-");
  setText("heroGroup", `${t("common.group")}: ${group}`);
  setText("heroInviteCode", `${t("common.inviteCode")}: ${profile.inviteCode || profile.invite_code || "-"}`);

  const adminLink = document.getElementById("adminLinkBtn");
  const role = profile.role || "";
  if (adminLink) {
    adminLink.style.display = role === "admin" || role === "super_admin" ? "inline-flex" : "none";
    adminLink.href = adminConsoleHref();
    adminLink.textContent = role === "super_admin" ? t("nav.superAdmin") : t("nav.admin");
  }
}

function maskTokenPreview(prefix) {
  const safePrefix = String(prefix || "").trim() || "sk_user_";
  return `${safePrefix}••••••••••••`;
}

function renderEmptyTableRow(tbody, colSpan, text) {
  const tr = document.createElement("tr");
  tr.className = "table-empty-row";
  const td = document.createElement("td");
  td.colSpan = colSpan;
  td.textContent = text;
  tr.appendChild(td);
  tbody.appendChild(tr);
}

/** 折叠条上展示当前开关与阈值（随语言切换更新） */
function syncBalanceAlertSummary() {
  const statusEl = document.getElementById("balanceAlertSummaryText");
  if (!statusEl) return;
  const elEn = document.getElementById("balanceAlertEnabled");
  const elUsd = document.getElementById("balanceAlertUsd");
  if (!elEn || !elUsd) return;
  const raw = elUsd.value.trim();
  statusEl.textContent = elEn.checked
    ? t("app.balanceAlertSummaryOn").replace("{usd}", raw || "—")
    : t("app.balanceAlertSummaryOff");
}

async function loadProfile() {
  const data = await api("/account/profile");
  if (!data) return;

  const p = data.profile || data;
  const d = data.dashboard;
  currentUserGroup = p.group || "default";
  cachedProfile = p;
  syncProfileMeta(p);

  if (d) {
    setText("kpiBalance", usdFromMicroStr(d.balanceMicroUsd));
    setText("kpiConsumed", usdFromMicroStr(d.totalConsumedMicroUsd));
    setText("kpiCalls", String(d.totalCalls ?? "-"));
    setText("kpiTokens", String(d.activeTokenCount ?? "-"));
  } else {
    setText("kpiBalance", usdFromMicroStr(String(p.balance || 0)));
    setText("kpiConsumed", "-");
    setText("kpiCalls", "-");
    setText("kpiTokens", "-");
  }

  const ba = data.balanceAlert;
  const elEn = document.getElementById("balanceAlertEnabled");
  const elUsd = document.getElementById("balanceAlertUsd");
  if (ba) {
    if (elEn) elEn.checked = ba.enabled !== false;
    if (elUsd) {
      const micro = BigInt(ba.thresholdMicroUsd || "0");
      const usd = Number(micro) / 1_000_000;
      elUsd.value = Number.isFinite(usd) && usd > 0 ? String(usd) : "10";
    }
  } else {
    if (elEn) elEn.checked = p.alert_enabled !== false;
    if (elUsd) {
      const usd = Number(p.alert_threshold);
      elUsd.value = Number.isFinite(usd) && usd > 0 ? String(usd) : "10";
    }
  }
  syncBalanceAlertSummary();
}

async function loadTokens() {
  const rows = await api("/me/tokens");
  if (rows === undefined || rows === null) return;
  const list = Array.isArray(rows) ? rows : [];
  cachedMeTokens = list;
  const tbody = document.getElementById("tokenTable");
  tbody.innerHTML = "";

  const localTokenMap = loadLocalPlainTokens();
  renderChatTestTokenSelect(list, localTokenMap);
  if (!list.length) {
    renderEmptyTableRow(tbody, 6, t("app.emptyTokens"));
    return;
  }

  list.forEach((r) => {
    const id = pickTokenField(r, "id", "id");
    const name = pickTokenField(r, "name", "name");
    const tokenGroup = pickTokenField(r, "tokenGroup", "token_group") ?? "-";
    const keyDisplay = pickTokenField(r, "keyDisplay", "key_display");
    const keyPrefix = pickTokenField(r, "keyPrefix", "key_prefix") || pickTokenField(r, "tokenPrefix", "token_prefix");
    const isActive = pickTokenField(r, "isActive", "is_active");
    const lastUsed = pickTokenField(r, "lastUsedAt", "last_used_at");

    const plain = localTokenMap[id];
    const maskedText = keyDisplay || maskTokenPreview(keyPrefix || (plain ? plain.slice(0, 10) : ""));

    const tr = document.createElement("tr");
    const secretCell = `<td class="token-full-cell${plain ? "" : " token-full-cell--readonly"}">
        <div class="token-full-wrap">
          <span class="token-plain-masked muted">${escapeHtml(maskedText)}</span>
          <code class="token-plain-full"></code>
        </div>
        ${plain ? `<button type="button" class="ghost token-eye-btn" data-token-id="${escapeHtml(id)}" title="${escapeHtml(t("app.tokenShowPlain"))}" aria-label="${escapeHtml(t("app.tokenShowPlain"))}" aria-pressed="false">${TOKEN_EYE_SVG}</button>` : ""}
      </td>`;
    tr.innerHTML = `
      <td>${escapeHtml(name)}</td>
      <td>${escapeHtml(String(tokenGroup))}</td>
      ${secretCell}
      <td><span class="pill ${isActive ? "ok" : "off"}">${isActive ? t("status.enabled") : t("status.disabled")}</span></td>
      <td>${fmtTime(lastUsed)}</td>
      <td>
        <div class="table-action-group table-action-group--token">
          ${plain ? `<button type="button" data-copy="${escapeHtml(id)}" class="ghost">${t("common.copy")}</button>` : `<span class="muted">${t("app.tokenMissing")}</span>`}
          ${isActive ? `<button type="button" data-disable="${escapeHtml(id)}" class="ghost">${t("action.disable")}</button>` : `<button type="button" data-enable="${escapeHtml(id)}" class="ghost">${t("action.enable")}</button>`}
          <button type="button" data-delete="${escapeHtml(id)}" class="ghost">${t("action.delete")}</button>
        </div>
      </td>
    `;
    tbody.appendChild(tr);
  });

  if (window.I18N && typeof window.I18N.applyI18n === "function") {
    window.I18N.applyI18n();
  }

  if (!tbody.dataset.tokenEyeBound) {
    tbody.dataset.tokenEyeBound = "1";
    tbody.addEventListener("click", handleTokenEyeClick);
  }

  tbody.querySelectorAll("button[data-copy]").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const id = btn.getAttribute("data-copy");
      const plain = loadLocalPlainTokens()[id];
      if (!plain) {
        await UiDialog.alert(t("app.tokenMissing"));
        return;
      }
      await copyToClipboard(plain);
      btn.textContent = t("common.copied");
      setTimeout(() => (btn.textContent = t("common.copy")), 1200);
    });
  });

  tbody.querySelectorAll("button[data-disable]").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const id = btn.getAttribute("data-disable");
      try {
        await api(`/me/tokens/${encodeURIComponent(id)}/disable`, { method: "PATCH" });
        await Promise.all([loadTokens(), loadProfile()]);
      } catch (err) {
        await UiDialog.alertError(err.message);
      }
    });
  });

  tbody.querySelectorAll("button[data-enable]").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const id = btn.getAttribute("data-enable");
      try {
        await api(`/me/tokens/${encodeURIComponent(id)}/enable`, { method: "PATCH" });
        await Promise.all([loadTokens(), loadProfile()]);
      } catch (err) {
        await UiDialog.alertError(err.message);
      }
    });
  });

  tbody.querySelectorAll("button[data-delete]").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const id = btn.getAttribute("data-delete");
      const row = cachedMeTokens.find((x) => String(pickTokenField(x, "id", "id")) === String(id));
      const name = (row && pickTokenField(row, "name", "name")) || id;
      const msg = t("app.deleteTokenConfirm").replace("{name}", name);
      if (!(await UiDialog.confirmTwoStep(msg, t("common.deleteConfirmSecond")))) return;
      try {
        await api(`/me/tokens/${encodeURIComponent(id)}`, { method: "DELETE" });
        removeLocalPlainToken(id);
        await Promise.all([loadTokens(), loadProfile()]);
      } catch (err) {
        await UiDialog.alertError(err.message);
      }
    });
  });
}

function renderChatTestTokenSelect(tokens, localTokenMap) {
  const sel = document.getElementById("chatTestTokenSelect");
  if (!sel) return;
  const previousValue = sel.value;
  sel.innerHTML = "";
  const activeTokens = (tokens || []).filter((tok) => pickTokenField(tok, "isActive", "is_active"));
  const ordered = activeTokens.length ? activeTokens : tokens || [];

  if (!ordered.length) {
    const opt = document.createElement("option");
    opt.value = "";
    opt.disabled = true;
    opt.selected = true;
    opt.textContent = t("app.chatNoTokens");
    sel.appendChild(opt);
    return;
  }

  ordered.forEach((tok) => {
    const id = pickTokenField(tok, "id", "id");
    const name = pickTokenField(tok, "name", "name");
    const group = pickTokenField(tok, "tokenGroup", "token_group") || "default";
    const plain = localTokenMap[id];
    const opt = document.createElement("option");
    opt.value = id;
    const suffix = plain ? "" : t("app.chatTestTokenNoLocalPlain");
    opt.textContent = `${name} [${group}]${suffix}`;
    sel.appendChild(opt);
  });

  if (previousValue && [...sel.options].some((opt) => opt.value === previousValue)) {
    sel.value = previousValue;
  } else {
    sel.selectedIndex = 0;
  }
}

const LOCAL_PLAIN_TOKEN_KEY = "plainTokensById";

function loadLocalPlainTokens() {
  try {
    const raw = localStorage.getItem(LOCAL_PLAIN_TOKEN_KEY);
    if (!raw) return {};
    const obj = JSON.parse(raw);
    if (!obj || typeof obj !== "object") return {};
    return obj;
  } catch {
    return {};
  }
}

function saveLocalPlainToken(id, plainToken) {
  const map = loadLocalPlainTokens();
  map[id] = plainToken;
  localStorage.setItem(LOCAL_PLAIN_TOKEN_KEY, JSON.stringify(map));
}

function removeLocalPlainToken(id) {
  const map = loadLocalPlainTokens();
  delete map[id];
  localStorage.setItem(LOCAL_PLAIN_TOKEN_KEY, JSON.stringify(map));
}

async function handleTokenEyeClick(ev) {
  const btn = ev.target.closest(".token-eye-btn");
  if (!btn) return;
  ev.preventDefault();
  const id = btn.getAttribute("data-token-id");
  const cell = btn.closest(".token-full-cell");
  if (!cell || !id) return;
  const fullEl = cell.querySelector(".token-plain-full");
  const willReveal = !cell.classList.contains("revealed");
  if (willReveal) {
    const plain = loadLocalPlainTokens()[id];
    if (!plain) {
      await UiDialog.alert(t("app.tokenMissing"));
      return;
    }
    cell.classList.add("revealed");
    if (fullEl) fullEl.textContent = plain;
    btn.setAttribute("aria-pressed", "true");
    btn.setAttribute("title", t("app.tokenHidePlain"));
    btn.setAttribute("aria-label", t("app.tokenHidePlain"));
  } else {
    cell.classList.remove("revealed");
    if (fullEl) fullEl.textContent = "";
    btn.setAttribute("aria-pressed", "false");
    btn.setAttribute("title", t("app.tokenShowPlain"));
    btn.setAttribute("aria-label", t("app.tokenShowPlain"));
  }
}

async function copyToClipboard(text) {
  try {
    await navigator.clipboard.writeText(text);
    return;
  } catch (_e) {
    /* 避免 Chrome 等非安全上下文下直接弹出难看的 prompt */
  }
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.left = "-9999px";
    ta.style.top = "0";
    document.body.appendChild(ta);
    ta.select();
    ta.setSelectionRange(0, text.length);
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    if (ok) return;
  } catch (_e2) {
    /* ignore */
  }
  window.prompt(t("common.copy"), text);
}

let openNewTokenModal = () => {};
let openCreateTokenModal = () => {};

function setupCreateTokenModal() {
  const backdrop = document.getElementById("createTokenModal");
  const openBtn = document.getElementById("openCreateTokenBtn");
  const cancelBtn = document.getElementById("createTokenModalCancel");
  const form = document.getElementById("tokenForm");
  const nameInput = document.getElementById("tokenName");
  const tokenGroupInput = document.getElementById("tokenGroup");
  const msg = document.getElementById("tokenMsg");
  if (!backdrop || !openBtn || !cancelBtn || !form || !nameInput || !tokenGroupInput || !msg) return;

  function modalScrollLock(on) {
    if (on) {
      const sb = window.innerWidth - document.documentElement.clientWidth;
      document.body.dataset.modalScrollY = String(window.scrollY || 0);
      document.body.style.overflow = "hidden";
      if (sb > 0) document.body.style.paddingRight = `${sb}px`;
    } else {
      const y = Number(document.body.dataset.modalScrollY || 0);
      delete document.body.dataset.modalScrollY;
      document.body.style.overflow = "";
      document.body.style.paddingRight = "";
      window.scrollTo(0, y);
    }
  }

  function resetCreateTokenForm() {
    form.reset();
    selectTokenGroupIfPresent("default");
    msg.className = "msg console-token-msg";
    msg.textContent = "";
  }

  // 若目标值已在 options 内则选中；否则保留当前值（避免写入一个不存在的值导致表单态失真）。
  function selectTokenGroupIfPresent(value) {
    const exists = Array.from(tokenGroupInput.options).some((o) => o.value === value);
    if (exists) tokenGroupInput.value = value;
  }

  // 打开弹窗时拉取可选分组填充下拉；失败或返回空时回退为只有 default 的兜底。
  async function populateTokenGroupOptions() {
    const prev = tokenGroupInput.value || "default";
    let items = null;
    try {
      const data = await api("/me/token-groups");
      if (data && Array.isArray(data.items)) items = data.items;
    } catch (_err) {
      items = null;
    }

    if (!items || items.length === 0) {
      tokenGroupInput.innerHTML = '<option value="default">default</option>';
      tokenGroupInput.value = "default";
      return;
    }

    const frag = document.createDocumentFragment();
    for (const it of items) {
      const name = String(it?.name ?? "").trim();
      if (!name) continue;
      const opt = document.createElement("option");
      opt.value = name;
      const desc = String(it?.description ?? "").trim();
      opt.textContent = desc ? `${name} - ${desc}` : name;
      frag.appendChild(opt);
    }
    tokenGroupInput.innerHTML = "";
    tokenGroupInput.appendChild(frag);

    if (tokenGroupInput.options.length === 0) {
      tokenGroupInput.innerHTML = '<option value="default">default</option>';
      tokenGroupInput.value = "default";
      return;
    }

    const prevExists = Array.from(tokenGroupInput.options).some((o) => o.value === prev);
    if (prevExists) {
      tokenGroupInput.value = prev;
    } else {
      const defaultExists = Array.from(tokenGroupInput.options).some((o) => o.value === "default");
      tokenGroupInput.value = defaultExists ? "default" : tokenGroupInput.options[0].value;
    }
  }

  function closeCreateTokenModal() {
    backdrop.classList.add("modal-hidden");
    backdrop.setAttribute("aria-hidden", "true");
    modalScrollLock(false);
    resetCreateTokenForm();
  }

  openCreateTokenModal = () => {
    resetCreateTokenForm();
    backdrop.classList.remove("modal-hidden");
    backdrop.setAttribute("aria-hidden", "false");
    modalScrollLock(true);
    requestAnimationFrame(() => nameInput.focus());
    populateTokenGroupOptions();
  };

  openBtn.addEventListener("click", () => openCreateTokenModal());
  cancelBtn.addEventListener("click", () => closeCreateTokenModal());
  backdrop.addEventListener("click", (ev) => {
    if (ev.target === backdrop) closeCreateTokenModal();
  });

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const name = nameInput.value.trim();
    const token_group = tokenGroupInput.value.trim() || "default";
    try {
      const data = await api("/me/tokens", {
        method: "POST",
        body: JSON.stringify({ name, token_group }),
      });
      const plainKey = data.key || data.token;
      const newId = data.id;
      if (plainKey && newId) saveLocalPlainToken(String(newId), plainKey);
      closeCreateTokenModal();
      if (plainKey) openNewTokenModal(plainKey);
      await loadTokens();
      await loadProfile();
    } catch (err) {
      msg.className = "msg err console-token-msg";
      msg.textContent = err.message;
    }
  });

  document.addEventListener("keydown", (ev) => {
    if (ev.key !== "Escape") return;
    if (backdrop.classList.contains("modal-hidden")) return;
    closeCreateTokenModal();
  });
}

function setupNewTokenModal() {
  const backdrop = document.getElementById("newTokenModal");
  const input = document.getElementById("newTokenModalValue");
  const copyBtn = document.getElementById("newTokenModalCopy");
  const closeBtn = document.getElementById("newTokenModalClose");
  if (!backdrop || !input || !copyBtn || !closeBtn) return;

  function modalScrollLock(on) {
    if (on) {
      const sb = window.innerWidth - document.documentElement.clientWidth;
      document.body.dataset.modalScrollY = String(window.scrollY || 0);
      document.body.style.overflow = "hidden";
      if (sb > 0) document.body.style.paddingRight = `${sb}px`;
    } else {
      const y = Number(document.body.dataset.modalScrollY || 0);
      delete document.body.dataset.modalScrollY;
      document.body.style.overflow = "";
      document.body.style.paddingRight = "";
      window.scrollTo(0, y);
    }
  }

  function closeNewTokenModal() {
    backdrop.classList.add("modal-hidden");
    backdrop.setAttribute("aria-hidden", "true");
    input.value = "";
    modalScrollLock(false);
  }

  openNewTokenModal = (plain) => {
    input.value = plain;
    copyBtn.textContent = t("common.copy");
    backdrop.classList.remove("modal-hidden");
    backdrop.setAttribute("aria-hidden", "false");
    modalScrollLock(true);
    requestAnimationFrame(() => {
      input.focus();
      input.select();
    });
  };

  closeBtn.addEventListener("click", () => closeNewTokenModal());
  backdrop.addEventListener("click", (ev) => {
    if (ev.target === backdrop) closeNewTokenModal();
  });
  copyBtn.addEventListener("click", async (ev) => {
    ev.preventDefault();
    const plain = input.value.trim();
    if (!plain) return;
    await copyToClipboard(plain);
    copyBtn.textContent = t("common.copied");
    setTimeout(() => {
      copyBtn.textContent = t("common.copy");
    }, 1200);
  });

  document.addEventListener("keydown", (ev) => {
    if (ev.key !== "Escape") return;
    if (backdrop.classList.contains("modal-hidden")) return;
    closeNewTokenModal();
  });
}

async function loadUsage(page) {
  if (typeof page === "number" && page >= 1) usageState.page = page;
  const stream = document.getElementById("filterStream").value;
  const model = document.getElementById("filterModel").value.trim();
  const startRaw = document.getElementById("filterStart")?.value?.trim() || "";
  const endRaw = document.getElementById("filterEnd")?.value?.trim() || "";
  const params = new URLSearchParams({
    page: String(usageState.page),
    page_size: String(usageState.pageSize),
  });
  if (stream) params.set("stream", stream);
  if (model) params.set("model", model);
  if (startRaw) params.set("start_time", startRaw);
  if (endRaw) params.set("end_time", endRaw);
  let raw;
  try {
    raw = await api(`/me/usage-logs?${params.toString()}`);
  } catch {
    raw = {};
  }
  const rows = Array.isArray(raw) ? raw : (raw?.list || []);
  usageState.total = typeof raw?.total === "number" ? raw.total : rows.length;
  const tbody = document.getElementById("usageTable");
  disposeBootstrapTooltips(tbody);
  tbody.innerHTML = "";
  if (!rows.length) {
    renderEmptyTableRow(tbody, 17, t("app.emptyUsage"));
    renderPager({
      infoId: "usagePageInfo",
      prevId: "usagePrevBtn",
      nextId: "usageNextBtn",
      state: usageState,
      pageSize: usageState.pageSize,
    });
    return;
  }
  rows.forEach((r) => {
    const tr = document.createElement("tr");
    const costDisp = r.costUsdApprox ? `$${r.costUsdApprox}` : usdFromMicroStr(r.costMicroUsd);
    const noteText = String(r.note || "-");
    const autoRoute = r.requestedModel && r.resolvedModel && r.requestedModel !== r.resolvedModel
      ? `${r.requestedModel} -> ${r.resolvedModel}`
      : "-";
    const noteTooltipAttrs = r.note
      ? ` data-bs-toggle="tooltip" data-bs-placement="top" data-bs-title="${escapeHtml(String(r.note))}" tabindex="0"`
      : "";
    tr.innerHTML = `
      <td>${fmtTime(r.time)}</td>
      <td>${r.type}</td>
      <td>${r.tokenName}</td>
      <td>${r.tokenGroup || "-"}</td>
      <td>${r.model}</td>
      <td>${escapeHtml(autoRoute)}</td>
      <td>${r.reasoningEffort || "-"}</td>
      <td>${r.latencyMs}ms</td>
      <td>${r.stream ? t("app.streamTrue") : t("app.streamFalse")}</td>
      <td>${r.promptTokens}</td>
      <td>${r.completionTokens}</td>
      <td>${r.totalTokens ?? ((Number(r.promptTokens) || 0) + (Number(r.completionTokens) || 0))}</td>
      <td>${r.cacheCreationTokens ?? 0}</td>
      <td>${r.cacheReadTokens ?? 0}</td>
      <td title="micro: ${r.costMicroUsd}">${costDisp}</td>
      <td>${r.ip || "-"}</td>
      <td class="usage-note-cell"><span class="usage-note-text"${noteTooltipAttrs}>${escapeHtml(noteText)}</span></td>
    `;
    tbody.appendChild(tr);
  });
  initBootstrapTooltips(tbody);
  renderPager({
    infoId: "usagePageInfo",
    prevId: "usagePrevBtn",
    nextId: "usageNextBtn",
    state: usageState,
    pageSize: usageState.pageSize,
  });
}

async function loadBalanceLogs(page) {
  if (typeof page === "number" && page >= 1) balanceState.page = page;
  const params = new URLSearchParams({
    page: String(balanceState.page),
    page_size: String(balanceState.pageSize),
  });
  const raw = await api(`/me/balance/logs?${params.toString()}`);
  if (!raw) return;
  const list = raw.list || (Array.isArray(raw) ? raw : []);
  balanceState.total = typeof raw.total === "number" ? raw.total : list.length;
  const tbody = document.getElementById("balanceTable");
  tbody.innerHTML = "";
  if (!list.length) {
    renderEmptyTableRow(tbody, 5, t("app.emptyBalanceLogs"));
    renderPager({
      infoId: "balancePageInfo",
      prevId: "balancePrevBtn",
      nextId: "balanceNextBtn",
      state: balanceState,
      pageSize: balanceState.pageSize,
    });
    return;
  }
  list.forEach((r) => {
    const amount = pickTokenField(r, "amount", "amount") ?? pickTokenField(r, "changeMicroUsd", "change_micro_usd") ?? "0";
    const before = pickTokenField(r, "balanceBefore", "balance_before") ?? pickTokenField(r, "balanceBeforeMicroUsd", "balance_before_micro_usd") ?? "0";
    const after = pickTokenField(r, "balanceAfter", "balance_after") ?? pickTokenField(r, "balanceAfterMicroUsd", "balance_after_micro_usd") ?? "0";
    const created = pickTokenField(r, "createdAt", "created_at");
    const reasonType = pickTokenField(r, "reasonType", "reason_type");

    const amountStr = String(amount);
    let ch = 0n;
    try { ch = BigInt(amountStr); } catch { ch = 0n; }
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td>${fmtTime(created)}</td>
      <td>${ch > 0n ? "+" : ""}${usdFromMicroStr(amountStr)}</td>
      <td>${usdFromMicroStr(String(before))}</td>
      <td>${usdFromMicroStr(String(after))}</td>
      <td>${reasonType || "-"}</td>
    `;
    tbody.appendChild(tr);
  });
  renderPager({
    infoId: "balancePageInfo",
    prevId: "balancePrevBtn",
    nextId: "balanceNextBtn",
    state: balanceState,
    pageSize: balanceState.pageSize,
  });
}

async function loadPricing() {
  let raw;
  try {
    raw = await api("/me/model-pricing");
  } catch {
    raw = [];
  }
  const rows = Array.isArray(raw) ? raw : (raw?.list || []);
  const tbody = document.getElementById("pricingTable");
  tbody.innerHTML = "";
  cachedPricingModels = rows;
  renderChatTestModelOptions();
  if (!rows.length) {
    renderEmptyTableRow(tbody, 2, t("app.emptyPricing"));
    return;
  }

  function buildPricingText(r) {
    if (!r) return "—";
    if (r.pricingMode === "per_request") {
      if (r.perRequestUsd === undefined || r.perRequestUsd === null || r.perRequestUsd === "") return "—";
      return `$${r.perRequestUsd}${t("app.pricingUnitRequestInline")}`;
    }
    const inputPrice =
      r.inputUsdPer1M === undefined || r.inputUsdPer1M === null || r.inputUsdPer1M === ""
        ? "—"
        : `$${r.inputUsdPer1M}${t("app.pricingUnitTokenInline")}`;
    const outputPrice =
      r.outputUsdPer1M === undefined || r.outputUsdPer1M === null || r.outputUsdPer1M === ""
        ? "—"
        : `$${r.outputUsdPer1M}${t("app.pricingUnitTokenInline")}`;
    return `${t("app.pricingLabelInput")}${inputPrice} ${t("app.pricingLabelOutput")}${outputPrice}`;
  }

  rows.forEach((r) => {
    const tr = document.createElement("tr");

    const tdModel = document.createElement("td");
    const nameBtn = document.createElement("button");
    nameBtn.type = "button";
    nameBtn.className = "ghost pricing-model-copy";
    nameBtn.textContent = r.model;
    nameBtn.title = t("app.pricingClickToCopy");
    nameBtn.addEventListener("click", async () => {
      await copyToClipboard(r.model);
      const label = r.model;
      nameBtn.textContent = t("common.copied");
      setTimeout(() => {
        nameBtn.textContent = label;
      }, 1200);
    });
    tdModel.appendChild(nameBtn);
    tr.appendChild(tdModel);

    const tdPrice = document.createElement("td");
    tdPrice.textContent = buildPricingText(r);
    tdPrice.title = buildPricingText(r);
    tr.appendChild(tdPrice);

    tbody.appendChild(tr);

  });
}

async function loadModelHealthSnapshot() {
  let raw;
  try {
    raw = await api(`/me/usage-logs?page=1&page_size=200`);
  } catch {
    raw = {};
  }
  const rows = Array.isArray(raw) ? raw : (raw?.list || []);
  const next = {};
  rows.forEach((r) => {
    const model = String(r?.model || "").trim();
    if (!model) return;
    const type = String(r?.type || "").toLowerCase();
    if (!next[model]) next[model] = { success: 0, error: 0, lastErrorAt: "" };
    if (type.includes("error") || type.includes("fail")) {
      next[model].error += 1;
      const tt = String(r?.time || "");
      if (tt && (!next[model].lastErrorAt || new Date(tt) > new Date(next[model].lastErrorAt))) {
        next[model].lastErrorAt = tt;
      }
    } else {
      next[model].success += 1;
    }
  });
  cachedModelHealth = next;
  renderChatTestModelOptions();
  syncChatTestModelHealthHint();
}

function usdInputToMicroStr(usdNumber) {
  const n = Number(usdNumber);
  if (!Number.isFinite(n) || n <= 0) throw new Error(t("common.needPositive"));
  const micro = Math.round(n * 1_000_000);
  if (micro < 1) throw new Error(t("common.tooSmall"));
  return String(micro);
}

function normalizeChatTestApiMode(raw) {
  if (raw === "gemini-image") return "gemini-image";
  if (raw === "gemini-chat") return "gemini-chat";
  if (raw === "anthropic") return "anthropic";
  if (raw === "responses") return "responses";
  return "openai";
}

function syncChatTestApiModeUi() {
  const modeEl = document.getElementById("chatTestApiMode");
  const gemEl = document.getElementById("chatTestGeminiOpts");
  const streamWrap = document.getElementById("chatTestStreamModeWrap");
  const modelEl = document.getElementById("chatTestModel");
  const promptEl = document.getElementById("chatTestPrompt");
  if (!modeEl || !gemEl || !modelEl || !promptEl) return;
  const mode = normalizeChatTestApiMode(modeEl.value);
  gemEl.hidden = mode !== "gemini-image";
  if (streamWrap) streamWrap.hidden = mode === "gemini-image";

  let modelPlaceholderKey = "app.chatTestModelPlaceholder";
  let promptPlaceholderKey = "app.chatTestPromptPlaceholder";
  if (mode === "gemini-image") {
    modelPlaceholderKey = "app.chatTestModelPlaceholderGemini";
    promptPlaceholderKey = "app.chatTestPromptPlaceholderGemini";
  } else if (mode === "gemini-chat") {
    modelPlaceholderKey = "app.chatTestModelPlaceholderGeminiChat";
    promptPlaceholderKey = "app.chatTestPromptPlaceholderGeminiChat";
  } else if (mode === "anthropic") {
    modelPlaceholderKey = "app.chatTestModelPlaceholderAnthropic";
  } else if (mode === "responses") {
    modelPlaceholderKey = "app.chatTestModelPlaceholderResponses";
    promptPlaceholderKey = "app.chatTestPromptPlaceholderResponses";
  }

  modelEl.title = t(modelPlaceholderKey);
  promptEl.setAttribute("data-i18n-placeholder", promptPlaceholderKey);
  promptEl.placeholder = t(promptPlaceholderKey);
  renderChatTestModelOptions();
}

function syncGeminiImageFileSummary() {
  const input = document.getElementById("chatTestGeminiImages");
  const sumEl = document.getElementById("chatTestGeminiImageSummary");
  if (!input || !sumEl) return;
  const n = input.files?.length ?? 0;
  if (n <= 0) {
    sumEl.hidden = true;
    sumEl.textContent = "";
    return;
  }
  sumEl.hidden = false;
  sumEl.textContent = t("app.chatTestGeminiImageSummary").replace("{n}", String(n));
}

function dataUrlToInlineData(dataUrl) {
  const s = String(dataUrl);
  const comma = s.indexOf(",");
  if (comma < 0) throw new Error("invalid image data");
  const head = s.slice(0, comma);
  const data = s.slice(comma + 1).replace(/\s/g, "");
  const mimeMatch = /^data:([^;,]+)/i.exec(head);
  const mimeType = (mimeMatch ? mimeMatch[1] : "image/png").trim() || "image/png";
  return { mimeType, data };
}

function fileToInlineDataPart(file) {
  return new Promise((resolve, reject) => {
    const fr = new FileReader();
    fr.onload = () => {
      try {
        const { mimeType, data } = dataUrlToInlineData(fr.result);
        resolve({ inlineData: { mimeType, data } });
      } catch (e) {
        reject(e);
      }
    };
    fr.onerror = () => reject(fr.error || new Error("read failed"));
    fr.readAsDataURL(file);
  });
}

async function buildGeminiImagePartsFromUi() {
  const input = document.getElementById("chatTestGeminiImages");
  const prompt = document.getElementById("chatTestPrompt").value.trim();
  const files = input?.files?.length ? [...input.files] : [];
  const parts = [];
  for (const f of files) {
    parts.push(await fileToInlineDataPart(f));
  }
  if (prompt) parts.push({ text: prompt });
  return parts;
}

function findFirstGeminiInlineImage(data) {
  const parts = data?.candidates?.[0]?.content?.parts;
  if (!Array.isArray(parts)) return null;
  for (const p of parts) {
    if (p?.inlineData?.data && p?.inlineData?.mimeType) {
      return { mimeType: p.inlineData.mimeType, data: p.inlineData.data };
    }
  }
  return null;
}

async function buildChatTestRequestContext() {
  const tokenId = document.getElementById("chatTestTokenSelect").value;
  const manualEl = document.getElementById("chatTestTokenManual");
  const manualPlain =
    manualEl && typeof manualEl.value === "string" ? manualEl.value.trim() : "";
  const plainToken = manualPlain || loadLocalPlainTokens()[tokenId];
  const model = document.getElementById("chatTestModel").value.trim();
  const prompt = document.getElementById("chatTestPrompt").value.trim();
  const rawApi = document.getElementById("chatTestApiMode").value;
  const apiMode = normalizeChatTestApiMode(rawApi);
  const streamModeEl = document.getElementById("chatTestStreamMode");
  const wantSse =
    apiMode !== "gemini-image" && streamModeEl && String(streamModeEl.value) === "sse";

  if (!tokenId) throw new Error("请先选择令牌");
  if (!plainToken) throw new Error(t("app.chatTestNeedPlainOrPaste"));
  if (!model) throw new Error("请输入要测试的模型");
  if (apiMode !== "gemini-image" && !prompt) throw new Error("请输入消息内容");

  let path;
  let payload;
  if (apiMode === "gemini-image") {
    const parts = await buildGeminiImagePartsFromUi();
    if (parts.length === 0) throw new Error(t("app.chatTestGeminiNeedTextOrImage"));
    const aspect = document.getElementById("chatTestGeminiAspect").value;
    const imageSize = document.getElementById("chatTestGeminiSize").value;
    path = `/v1beta/models/${encodeURIComponent(model)}:generateContent`;
    payload = {
      contents: [{ role: "user", parts }],
      generationConfig: {
        imageConfig: { aspectRatio: aspect, imageSize },
        responseModalities: ["IMAGE"],
      },
    };
  } else if (apiMode === "gemini-chat") {
    path = wantSse
      ? `/v1beta/models/${encodeURIComponent(model)}:streamGenerateContent`
      : `/v1beta/models/${encodeURIComponent(model)}:generateContent`;
    payload = {
      contents: [{ role: "user", parts: [{ text: prompt }] }],
    };
  } else if (apiMode === "anthropic") {
    path = "/v1/messages";
    payload = {
      model,
      max_tokens: 1024,
      messages: [{ role: "user", content: prompt }],
      stream: wantSse,
    };
  } else if (apiMode === "responses") {
    path = "/v1/responses";
    payload = {
      model,
      input: prompt,
      stream: wantSse,
    };
  } else {
    path = "/v1/chat/completions";
    payload = {
      model,
      messages: [{ role: "user", content: prompt }],
      stream: wantSse,
    };
  }

  const fetchHeaders =
    apiMode === "anthropic"
      ? {
          "content-type": "application/json",
          "x-api-key": plainToken,
          "anthropic-version": "2023-06-01",
        }
      : {
          "content-type": "application/json",
          authorization: `Bearer ${plainToken}`,
        };

  return { apiMode, wantSse, path, payload, fetchHeaders };
}

function redactedHeaders(headers) {
  const x = { ...headers };
  if (x.authorization) x.authorization = "Bearer sk-***";
  if (x["x-api-key"]) x["x-api-key"] = "sk-***";
  return x;
}

function buildCurlPreview(path, headers, payload) {
  const hdrs = redactedHeaders(headers);
  const headerFlags = Object.entries(hdrs)
    .map(([k, v]) => `-H ${JSON.stringify(`${k}: ${v}`)}`)
    .join(" ");
  const body = JSON.stringify(payload);
  return `curl -X POST ${JSON.stringify(`${gatewayBase()}${path}`)} ${headerFlags} --data-raw ${JSON.stringify(body)}`;
}

function geminiResponseJsonForPre(data) {
  try {
    const clone = JSON.parse(JSON.stringify(data));
    const walk = (o) => {
      if (!o || typeof o !== "object") return;
      if (Array.isArray(o)) {
        o.forEach(walk);
        return;
      }
      if (o.inlineData && typeof o.inlineData.data === "string" && o.inlineData.data.length > 64) {
        o.inlineData.data = `[base64 ${o.inlineData.data.length} chars omitted]`;
      }
      Object.values(o).forEach(walk);
    };
    walk(clone);
    return JSON.stringify(clone, null, 2);
  } catch {
    return JSON.stringify(data, null, 2);
  }
}

function renderChatTestOutputOpenAI(outEl, text) {
  outEl.replaceChildren();
  const pre = document.createElement("pre");
  pre.className = "chat-test-json-pre";
  pre.textContent = text;
  outEl.appendChild(pre);
}

/** Anthropic Messages 非流式：从 content 块提取 assistant 文本 */
function anthropicAssistantPlainText(data) {
  const blocks = data?.content;
  if (!Array.isArray(blocks)) return "";
  const parts = [];
  for (const b of blocks) {
    if (b && b.type === "text" && typeof b.text === "string") parts.push(b.text);
  }
  return parts.join("\n\n");
}

function geminiAssistantPlainText(data) {
  const candidates = Array.isArray(data?.candidates) ? data.candidates : [];
  for (const candidate of candidates) {
    const parts = candidate?.content?.parts;
    if (!Array.isArray(parts)) continue;
    const texts = [];
    for (const part of parts) {
      if (part && typeof part.text === "string" && part.text) texts.push(part.text);
    }
    if (texts.length > 0) return texts.join("\n\n");
  }
  return "";
}

/** OpenAI Responses API 非流式：从 output 中的 message / 文本块提取可见回复 */
function responsesAssistantPlainText(data) {
  const root = data?.response && typeof data.response === "object" ? data.response : data;
  const out = root?.output;
  if (!Array.isArray(out)) return "";
  const texts = [];
  for (const item of out) {
    if (!item || typeof item !== "object") continue;
    const content = item.content;
    if (!Array.isArray(content)) continue;
    for (const c of content) {
      if (!c || typeof c !== "object") continue;
      const typ = String(c.type || "");
      if (typeof c.text === "string" && c.text) {
        if (typ.includes("text") || typ === "" || typ === "output_text") texts.push(c.text);
      }
    }
  }
  return texts.join("\n\n");
}

function chatTestErrorMessage(data, rawText, status) {
  let errPiece = data?.error;
  if (errPiece && typeof errPiece === "object") {
    errPiece = errPiece.message || errPiece.type || "";
  }
  if (typeof errPiece !== "string") errPiece = "";
  return errPiece || data?.code || rawText.slice(0, 200) || `HTTP ${status}`;
}

const CHAT_TEST_STREAM_TIMEOUT_MS = 300_000;

function sseJsonFromChatTestLine(line) {
  const trimmed = String(line).trimEnd();
  if (!trimmed.startsWith("data:")) return null;
  const payload = trimmed.slice(5).trim();
  if (!payload || payload === "[DONE]") return null;
  try {
    return JSON.parse(payload);
  } catch {
    return null;
  }
}

function geminiSseChunkPlainText(obj) {
  if (!obj || typeof obj !== "object") return "";
  const parts = obj?.candidates?.[0]?.content?.parts;
  if (!Array.isArray(parts)) return "";
  return parts.map((p) => (p && typeof p.text === "string" ? p.text : "")).join("");
}

function chatTestStreamDeltaFromLine(apiMode, line) {
  const obj = sseJsonFromChatTestLine(line);
  if (!obj) return "";
  if (apiMode === "openai") {
    const c = obj?.choices?.[0]?.delta?.content;
    return typeof c === "string" ? c : "";
  }
  if (apiMode === "anthropic") {
    if (obj.type === "content_block_delta" && obj.delta && obj.delta.type === "text_delta") {
      return typeof obj.delta.text === "string" ? obj.delta.text : "";
    }
    return "";
  }
  if (apiMode === "responses") {
    const typ = String(obj.type || "");
    if (typeof obj.delta === "string" && (typ.endsWith(".delta") || typ.includes("output_text"))) {
      return obj.delta;
    }
    if (typ === "response.output_text.delta" && typeof obj.delta === "string") return obj.delta;
    return "";
  }
  if (apiMode === "gemini-chat") {
    return geminiSseChunkPlainText(obj);
  }
  return "";
}

async function pumpChatTestSse(reader, apiMode, outPre, signal) {
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { done, value } = await reader.read();
    if (signal.aborted) throw new DOMException("Aborted", "AbortError");
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";
    for (const ln of lines) {
      const piece = chatTestStreamDeltaFromLine(apiMode, ln);
      if (piece) outPre.textContent += piece;
    }
  }
  if (buffer.trim()) {
    const piece = chatTestStreamDeltaFromLine(apiMode, buffer);
    if (piece) outPre.textContent += piece;
  }
}

function renderChatTestOutputGeminiImage(outEl, data) {
  outEl.replaceChildren();
  const imgData = findFirstGeminiInlineImage(data);
  if (imgData) {
    const img = document.createElement("img");
    img.className = "chat-test-gen-img";
    img.alt = "generated";
    img.src = `data:${imgData.mimeType};base64,${imgData.data}`;
    outEl.appendChild(img);
  }
  const pre = document.createElement("pre");
  pre.className = "chat-test-json-pre";
  pre.textContent = geminiResponseJsonForPre(data);
  outEl.appendChild(pre);
}

async function init() {
  setupCreateTokenModal();
  setupNewTokenModal();

  document.getElementById("logoutBtn").addEventListener("click", () => {
    localStorage.removeItem("accessToken");
    window.location.href = "/login.html";
  });

  document.getElementById("refreshBtn").addEventListener("click", () => refreshAll());
  document.getElementById("filterBtn").addEventListener("click", () => loadUsage(1));
  document.getElementById("usageRange1h")?.addEventListener("click", () => {
    applyUsageQuickRange(1);
    loadUsage(1);
  });
  document.getElementById("usageRange24h")?.addEventListener("click", () => {
    applyUsageQuickRange(24);
    loadUsage(1);
  });
  document.getElementById("usageRange7d")?.addEventListener("click", () => {
    applyUsageQuickRange(24 * 7);
    loadUsage(1);
  });
  document.getElementById("filterResetBtn")?.addEventListener("click", () => {
    const streamEl = document.getElementById("filterStream");
    const modelEl = document.getElementById("filterModel");
    const startEl = document.getElementById("filterStart");
    const endEl = document.getElementById("filterEnd");
    if (streamEl) streamEl.value = "";
    if (modelEl) modelEl.value = "";
    if (startEl) startEl.value = "";
    if (endEl) endEl.value = "";
    loadUsage(1);
  });
  document.getElementById("usagePrevBtn")?.addEventListener("click", () => {
    if (usageState.page > 1) loadUsage(usageState.page - 1);
  });
  document.getElementById("usageNextBtn")?.addEventListener("click", () => {
    const pages = usageState.total > 0 ? Math.max(1, Math.ceil(usageState.total / usageState.pageSize)) : 1;
    if (usageState.page < pages) loadUsage(usageState.page + 1);
  });
  document.getElementById("usagePageSize")?.addEventListener("change", () => {
    const v = Number(document.getElementById("usagePageSize").value);
    usageState.pageSize = Number.isFinite(v) && v > 0 ? v : DEFAULT_PAGE_SIZE;
    loadUsage(1);
  });
  document.getElementById("usagePageJumpBtn")?.addEventListener("click", () => {
    const v = Number(document.getElementById("usagePageJump").value);
    const pages = usageState.total > 0 ? Math.max(1, Math.ceil(usageState.total / usageState.pageSize)) : 1;
    if (Number.isFinite(v) && v >= 1) loadUsage(Math.min(v, pages));
  });
  document.getElementById("usagePageJump")?.addEventListener("keydown", (ev) => {
    if (ev.key !== "Enter") return;
    ev.preventDefault();
    document.getElementById("usagePageJumpBtn")?.click();
  });
  document.getElementById("balancePrevBtn")?.addEventListener("click", () => {
    if (balanceState.page > 1) loadBalanceLogs(balanceState.page - 1);
  });
  document.getElementById("balanceNextBtn")?.addEventListener("click", () => {
    const pages = balanceState.total > 0 ? Math.max(1, Math.ceil(balanceState.total / balanceState.pageSize)) : 1;
    if (balanceState.page < pages) loadBalanceLogs(balanceState.page + 1);
  });
  document.getElementById("balancePageSize")?.addEventListener("change", () => {
    const v = Number(document.getElementById("balancePageSize").value);
    balanceState.pageSize = Number.isFinite(v) && v > 0 ? v : DEFAULT_PAGE_SIZE;
    loadBalanceLogs(1);
  });
  document.getElementById("balancePageJumpBtn")?.addEventListener("click", () => {
    const v = Number(document.getElementById("balancePageJump").value);
    const pages = balanceState.total > 0 ? Math.max(1, Math.ceil(balanceState.total / balanceState.pageSize)) : 1;
    if (Number.isFinite(v) && v >= 1) loadBalanceLogs(Math.min(v, pages));
  });
  document.getElementById("balancePageJump")?.addEventListener("keydown", (ev) => {
    if (ev.key !== "Enter") return;
    ev.preventDefault();
    document.getElementById("balancePageJumpBtn")?.click();
  });
  const usageSizeEl = document.getElementById("usagePageSize");
  const balanceSizeEl = document.getElementById("balancePageSize");
  if (usageSizeEl) usageSizeEl.value = String(usageState.pageSize);
  if (balanceSizeEl) balanceSizeEl.value = String(balanceState.pageSize);

  document.getElementById("chatTestApiMode").addEventListener("change", () => {
    syncChatTestApiModeUi();
    window.I18N?.applyI18n?.();
  });
  document.getElementById("chatTestStreamMode")?.addEventListener("change", () => {
    syncChatTestApiModeUi();
    window.I18N?.applyI18n?.();
  });
  document.getElementById("chatTestModel")?.addEventListener("change", () => {
    syncChatTestModelHealthHint();
  });
  syncChatTestApiModeUi();
  document.getElementById("chatTestGeminiImages")?.addEventListener("change", () => {
    syncGeminiImageFileSummary();
  });
  syncGeminiImageFileSummary();
  renderChatTestHistoryOptions();
  document.getElementById("chatTestPresetHealth")?.addEventListener("click", () => {
    const modeEl = document.getElementById("chatTestApiMode");
    const modelEl = document.getElementById("chatTestModel");
    const promptEl = document.getElementById("chatTestPrompt");
    if (modeEl && !modeEl.value) modeEl.value = "openai";
    if (modelEl && !modelEl.value.trim()) {
      ensureChatTestModelOption("gpt-4o-mini");
      modelEl.value = "gpt-4o-mini";
    }
    if (promptEl) {
      promptEl.value = "请输出一句健康检查文案，并返回 3 条可观测性检查项。";
      promptEl.focus();
    }
    syncChatTestApiModeUi();
  });
  document.getElementById("chatTestPresetSummary")?.addEventListener("click", () => {
    const modeEl = document.getElementById("chatTestApiMode");
    const modelEl = document.getElementById("chatTestModel");
    const promptEl = document.getElementById("chatTestPrompt");
    if (modeEl && modeEl.value !== "responses") modeEl.value = "responses";
    if (modelEl && !modelEl.value.trim()) {
      ensureChatTestModelOption("gpt-4.1-mini");
      modelEl.value = "gpt-4.1-mini";
    }
    if (promptEl) {
      promptEl.value = "请把以下信息整理成 JSON：目标、风险、下一步。";
      promptEl.focus();
    }
    syncChatTestApiModeUi();
  });
  document.getElementById("chatTestClearOutput")?.addEventListener("click", () => {
    const outEl = document.getElementById("chatTestOutput");
    const msgEl = document.getElementById("chatTestMsg");
    const previewEl = document.getElementById("chatTestRequestPreview");
    outEl?.replaceChildren();
    if (previewEl) previewEl.textContent = "";
    if (msgEl) {
      msgEl.className = "msg";
      msgEl.textContent = "";
    }
  });
  document.getElementById("chatTestPreviewBtn")?.addEventListener("click", async () => {
    const msgEl = document.getElementById("chatTestMsg");
    const previewEl = document.getElementById("chatTestRequestPreview");
    if (!previewEl || !msgEl) return;
    try {
      const req = await buildChatTestRequestContext();
      previewEl.textContent = JSON.stringify(
        {
          path: req.path,
          headers: redactedHeaders(req.fetchHeaders),
          payload: req.payload,
        },
        null,
        2,
      );
      msgEl.className = "msg ok";
      msgEl.textContent = "已生成请求预览";
    } catch (err) {
      msgEl.className = "msg err";
      msgEl.textContent = err.message || "生成预览失败";
    }
  });
  document.getElementById("chatTestCopyCurlBtn")?.addEventListener("click", async () => {
    const msgEl = document.getElementById("chatTestMsg");
    if (!msgEl) return;
    try {
      const req = await buildChatTestRequestContext();
      await copyToClipboard(buildCurlPreview(req.path, req.fetchHeaders, req.payload));
      msgEl.className = "msg ok";
      msgEl.textContent = "cURL 已复制（令牌已脱敏）";
    } catch (err) {
      msgEl.className = "msg err";
      msgEl.textContent = err.message || "复制 cURL 失败";
    }
  });
  document.getElementById("chatTestHistoryApply")?.addEventListener("click", () => {
    const sel = document.getElementById("chatTestHistorySelect");
    const modeEl = document.getElementById("chatTestApiMode");
    const streamEl = document.getElementById("chatTestStreamMode");
    const tokenEl = document.getElementById("chatTestTokenSelect");
    const modelEl = document.getElementById("chatTestModel");
    const promptEl = document.getElementById("chatTestPrompt");
    const idx = Number(sel?.value || -1);
    const item = loadChatTestHistory()[idx];
    if (!item) return;
    if (modeEl) modeEl.value = item.apiMode || "openai";
    syncChatTestApiModeUi();
    if (streamEl) streamEl.value = item.streamMode || "sse";
    if (tokenEl && item.tokenId && [...tokenEl.options].some((o) => o.value === String(item.tokenId))) {
      tokenEl.value = String(item.tokenId);
    }
    if (modelEl && item.model) {
      ensureChatTestModelOption(String(item.model));
      modelEl.value = String(item.model);
    }
    if (promptEl) promptEl.value = String(item.prompt || "");
  });
  document.getElementById("chatTestHistoryClear")?.addEventListener("click", () => {
    localStorage.removeItem(CHAT_TEST_HISTORY_KEY);
    renderChatTestHistoryOptions();
  });

  document.getElementById("chatTestBtn").addEventListener("click", async () => {
    const msgEl = document.getElementById("chatTestMsg");
    const outEl = document.getElementById("chatTestOutput");
    const previewEl = document.getElementById("chatTestRequestPreview");
    const btn = document.getElementById("chatTestBtn");
    const tokenId = String(document.getElementById("chatTestTokenSelect")?.value || "");
    const streamMode = String(document.getElementById("chatTestStreamMode")?.value || "sse");
    let req;
    try {
      req = await buildChatTestRequestContext();
    } catch (err) {
      msgEl.className = "msg err";
      msgEl.textContent = err.message || "参数检查失败";
      return;
    }
    const { apiMode, wantSse, path, payload, fetchHeaders } = req;

    btn.disabled = true;
    outEl.replaceChildren();
    msgEl.className = "msg";
    msgEl.textContent = "发送中...";
    if (previewEl) {
      previewEl.textContent = JSON.stringify(
        { path, headers: redactedHeaders(fetchHeaders), payload },
        null,
        2,
      );
    }

    const timeoutMs =
      apiMode === "gemini-image"
        ? 300_000
        : wantSse
          ? CHAT_TEST_STREAM_TIMEOUT_MS
          : 120_000;
    const controller = new AbortController();
    const tmr = setTimeout(() => controller.abort(), timeoutMs);

    async function doFetch(url) {
      return await fetch(url, {
        method: "POST",
        headers: fetchHeaders,
        signal: controller.signal,
        body: JSON.stringify(payload),
      });
    }

    try {
      let resp;
      const gwBase = gatewayBase();
      try {
        resp = await doFetch(`${gwBase}${path}`);
      } catch (e) {
        resp = await doFetch(`${window.location.origin}${path}`);
        msgEl.className = "msg";
        msgEl.textContent = `提示：网关请求失败，已回退到同源发送测试。`;
      }

      if (!resp.ok) {
        const rawText = await resp.text().catch(() => "");
        let data = {};
        if (rawText) {
          try {
            data = JSON.parse(rawText);
          } catch {
            throw new Error(rawText.slice(0, 240) || `HTTP ${resp.status}`);
          }
        }
        throw new Error(chatTestErrorMessage(data, rawText, resp.status));
      }

      const ct = (resp.headers.get("content-type") || "").toLowerCase();
      const sseLike = ct.includes("text/event-stream") || ct.includes("event-stream");

      if (wantSse && sseLike && resp.body) {
        msgEl.className = "msg";
        msgEl.textContent = t("app.chatTestStreaming");
        const pre = document.createElement("pre");
        pre.className = "chat-test-json-pre";
        pre.textContent = "";
        outEl.replaceChildren();
        outEl.appendChild(pre);
        const sr = resp.body.getReader();
        await pumpChatTestSse(sr, apiMode, pre, controller.signal);
        msgEl.className = "msg ok";
        msgEl.textContent = t("app.chatTestStreamDone");
        saveRecentChatTestConfig({
          tokenId,
          apiMode,
          streamMode,
          model: payload?.model || document.getElementById("chatTestModel")?.value || "",
          prompt: document.getElementById("chatTestPrompt")?.value || "",
        });
        saveChatTestHistoryItem({
          at: new Date().toISOString(),
          tokenId,
          apiMode,
          streamMode,
          model: payload?.model || document.getElementById("chatTestModel")?.value || "",
          prompt: document.getElementById("chatTestPrompt")?.value || "",
        });
        renderChatTestHistoryOptions();
      } else {
        const rawText = await resp.text().catch(() => "");
        let data = {};
        if (rawText) {
          try {
            data = JSON.parse(rawText);
          } catch {
            throw new Error(rawText.slice(0, 240) || `HTTP ${resp.status}`);
          }
        }

        msgEl.className = "msg ok";
        msgEl.textContent = "发送成功";
        saveRecentChatTestConfig({
          tokenId,
          apiMode,
          streamMode,
          model: payload?.model || document.getElementById("chatTestModel")?.value || "",
          prompt: document.getElementById("chatTestPrompt")?.value || "",
        });
        saveChatTestHistoryItem({
          at: new Date().toISOString(),
          tokenId,
          apiMode,
          streamMode,
          model: payload?.model || document.getElementById("chatTestModel")?.value || "",
          prompt: document.getElementById("chatTestPrompt")?.value || "",
        });
        renderChatTestHistoryOptions();

        if (apiMode === "gemini-image") {
          renderChatTestOutputGeminiImage(outEl, data);
        } else if (apiMode === "gemini-chat") {
          const content = geminiAssistantPlainText(data) || geminiResponseJsonForPre(data);
          renderChatTestOutputOpenAI(outEl, content);
        } else if (apiMode === "anthropic") {
          const extracted = anthropicAssistantPlainText(data);
          const content = extracted || JSON.stringify(data, null, 2);
          renderChatTestOutputOpenAI(outEl, content);
        } else if (apiMode === "responses") {
          const extracted = responsesAssistantPlainText(data);
          const content = extracted || JSON.stringify(data, null, 2);
          renderChatTestOutputOpenAI(outEl, content);
        } else {
          const content =
            data?.choices?.[0]?.message?.content ??
            data?.choices?.[0]?.text ??
            JSON.stringify(data, null, 2);
          renderChatTestOutputOpenAI(outEl, content);
        }
      }
    } catch (err) {
      msgEl.className = "msg err";
      if (err && (err.name === "AbortError" || /aborted/i.test(String(err.message || "")))) {
        const sec = String(timeoutMs / 1000);
        if (apiMode === "gemini-image") {
          msgEl.textContent = t("app.chatTestTimeoutGemini").replace(/\{seconds\}/g, sec);
        } else if (wantSse) {
          msgEl.textContent = t("app.chatTestTimeoutStream").replace(/\{seconds\}/g, sec);
        } else {
          msgEl.textContent = t("app.chatTestTimeoutUnary").replace(/\{seconds\}/g, sec);
        }
      } else {
        msgEl.textContent = err.message || "发送失败";
      }
    } finally {
      clearTimeout(tmr);
      btn.disabled = false;
    }
  });

  const balanceAlertEnabledEl = document.getElementById("balanceAlertEnabled");
  const balanceAlertUsdEl = document.getElementById("balanceAlertUsd");
  if (balanceAlertEnabledEl) balanceAlertEnabledEl.addEventListener("change", syncBalanceAlertSummary);
  if (balanceAlertUsdEl) balanceAlertUsdEl.addEventListener("input", syncBalanceAlertSummary);

  document.getElementById("balanceAlertForm").addEventListener("submit", async (e) => {
    e.preventDefault();
    const enabled = document.getElementById("balanceAlertEnabled").checked;
    const balanceAlertUsd = Number(document.getElementById("balanceAlertUsd").value);
    const msg = document.getElementById("balanceAlertMsg");
    if (!Number.isFinite(balanceAlertUsd) || balanceAlertUsd < 0.01) {
      msg.className = "msg err";
      msg.textContent = t("common.needPositive");
      return;
    }
    try {
      const threshold = usdInputToMicroStr(balanceAlertUsd);
      await api("/me/balance-alert", {
        method: "PATCH",
        body: JSON.stringify({ enabled, threshold: Number(threshold) }),
      });
      msg.className = "msg ok";
      msg.textContent = t("app.balanceAlertSaved");
      syncBalanceAlertSummary();
      await loadProfile();
    } catch (err) {
      msg.className = "msg err";
      msg.textContent = err.message;
    }
  });

  document.querySelectorAll("[data-lang-switch]").forEach((el) => {
    el.addEventListener("change", () => {
      queueMicrotask(() => {
        syncBalanceAlertSummary();
        syncProfileMeta(cachedProfile);
        syncChatTestApiModeUi();
        syncGeminiImageFileSummary();
      });
    });
  });

  document.getElementById("redeemForm").addEventListener("submit", async (e) => {
    e.preventDefault();
    const code = document.getElementById("redeemCode").value.trim();
    try {
      await api("/me/balance/redeem", {
        method: "POST",
        body: JSON.stringify({ code }),
      });
      document.getElementById("redeemMsg").className = "msg ok";
      document.getElementById("redeemMsg").textContent = t("app.redeemSuccess");
      await Promise.all([loadProfile(), loadBalanceLogs()]);
    } catch (err) {
      document.getElementById("redeemMsg").className = "msg err";
      document.getElementById("redeemMsg").textContent = err.message;
    }
  });

  await refreshAll();
  applyRecentChatTestConfig();
}

async function refreshAll() {
  await Promise.all([loadProfile(), loadTokens(), loadUsage(), loadBalanceLogs(), loadPricing(), loadModelHealthSnapshot()]);
}

init().catch(async (e) => {
  await UiDialog.alertError(e.message || t("common.initFailed"));
});
