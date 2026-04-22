const t = (k) => (window.I18N ? window.I18N.t(k) : k);

function getAccessToken() {
  return localStorage.getItem("accessToken");
}

if (!getAccessToken()) window.location.href = "/login.html";

let currentUserGroup = "default";
let cachedProfile = null;
let cachedMeTokens = [];

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
  const superAdminLink = document.getElementById("superAdminLinkBtn");
  const role = profile.role || "";
  if (adminLink) {
    adminLink.style.display = role === "admin" ? "inline-flex" : "none";
    adminLink.href = adminConsoleHref();
    adminLink.textContent = t("nav.admin");
  }
  if (superAdminLink) {
    superAdminLink.style.display = role === "super_admin" ? "inline-flex" : "none";
    superAdminLink.href = adminConsoleHref();
    superAdminLink.textContent = t("nav.superAdmin");
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
    const suffix = plain ? "" : "（未保存明文，请先复制）";
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
    tokenGroupInput.value = "default";
    msg.className = "msg console-token-msg";
    msg.textContent = "";
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

async function loadUsage() {
  const stream = document.getElementById("filterStream").value;
  const model = document.getElementById("filterModel").value.trim();
  const params = new URLSearchParams({ limit: "50" });
  if (stream) params.set("stream", stream);
  if (model) params.set("model", model);
  let raw;
  try {
    raw = await api(`/me/usage-logs?${params.toString()}`);
  } catch {
    raw = [];
  }
  const rows = Array.isArray(raw) ? raw : (raw?.list || []);
  const tbody = document.getElementById("usageTable");
  disposeBootstrapTooltips(tbody);
  tbody.innerHTML = "";
  if (!rows.length) {
    renderEmptyTableRow(tbody, 16, t("app.emptyUsage"));
    return;
  }
  rows.forEach((r) => {
    const tr = document.createElement("tr");
    const costDisp = r.costUsdApprox ? `$${r.costUsdApprox}` : usdFromMicroStr(r.costMicroUsd);
    const noteText = String(r.note || "-");
    const noteTooltipAttrs = r.note
      ? ` data-bs-toggle="tooltip" data-bs-placement="top" data-bs-title="${escapeHtml(String(r.note))}" tabindex="0"`
      : "";
    tr.innerHTML = `
      <td>${fmtTime(r.time)}</td>
      <td>${r.type}</td>
      <td>${r.tokenName}</td>
      <td>${r.tokenGroup || "-"}</td>
      <td>${r.model}</td>
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
}

async function loadBalanceLogs() {
  const raw = await api("/me/balance/logs?page=1&page_size=50");
  if (!raw) return;
  const list = raw.list || (Array.isArray(raw) ? raw : []);
  const tbody = document.getElementById("balanceTable");
  tbody.innerHTML = "";
  if (!list.length) {
    renderEmptyTableRow(tbody, 5, t("app.emptyBalanceLogs"));
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
  const modelsDl = document.getElementById("chatPricingModels");
  if (modelsDl) modelsDl.innerHTML = "";
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

    if (modelsDl) {
      const opt = document.createElement("option");
      opt.value = r.model;
      modelsDl.appendChild(opt);
    }
  });
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

  modelEl.setAttribute("data-i18n-placeholder", modelPlaceholderKey);
  promptEl.setAttribute("data-i18n-placeholder", promptPlaceholderKey);
  modelEl.placeholder = t(modelPlaceholderKey);
  promptEl.placeholder = t(promptPlaceholderKey);
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
  document.getElementById("filterBtn").addEventListener("click", () => loadUsage());

  document.getElementById("chatTestApiMode").addEventListener("change", () => {
    syncChatTestApiModeUi();
    window.I18N?.applyI18n?.();
  });
  document.getElementById("chatTestStreamMode")?.addEventListener("change", () => {
    syncChatTestApiModeUi();
    window.I18N?.applyI18n?.();
  });
  syncChatTestApiModeUi();
  document.getElementById("chatTestGeminiImages")?.addEventListener("change", () => {
    syncGeminiImageFileSummary();
  });
  syncGeminiImageFileSummary();

  document.getElementById("chatTestBtn").addEventListener("click", async () => {
    const msgEl = document.getElementById("chatTestMsg");
    const outEl = document.getElementById("chatTestOutput");
    const btn = document.getElementById("chatTestBtn");

    const tokenId = document.getElementById("chatTestTokenSelect").value;
    const plainToken = loadLocalPlainTokens()[tokenId];
    const model = document.getElementById("chatTestModel").value.trim();
    const prompt = document.getElementById("chatTestPrompt").value.trim();
    const rawApi = document.getElementById("chatTestApiMode").value;
    const apiMode = normalizeChatTestApiMode(rawApi);
    const streamModeEl = document.getElementById("chatTestStreamMode");
    const wantSse =
      apiMode !== "gemini-image" && streamModeEl && String(streamModeEl.value) === "sse";

    if (!tokenId) {
      msgEl.className = "msg err";
      msgEl.textContent = "请先选择令牌";
      return;
    }
    if (!plainToken) {
      msgEl.className = "msg err";
      msgEl.textContent = "该令牌明文未保存。请回到令牌列表点击复制。";
      return;
    }
    if (!model) {
      msgEl.className = "msg err";
      msgEl.textContent = "请输入要测试的模型";
      return;
    }
    if (apiMode !== "gemini-image" && !prompt) {
      msgEl.className = "msg err";
      msgEl.textContent = "请输入消息内容";
      return;
    }

    btn.disabled = true;
    outEl.replaceChildren();
    msgEl.className = "msg";
    msgEl.textContent = "发送中...";

    const timeoutMs =
      apiMode === "gemini-image"
        ? 300_000
        : wantSse
          ? CHAT_TEST_STREAM_TIMEOUT_MS
          : 120_000;
    const controller = new AbortController();
    const tmr = setTimeout(() => controller.abort(), timeoutMs);

    let path;
    let payload;
    if (apiMode === "gemini-image") {
      let parts;
      try {
        parts = await buildGeminiImagePartsFromUi();
      } catch (e) {
        msgEl.className = "msg err";
        msgEl.textContent = e.message || "读取参考图失败";
        clearTimeout(tmr);
        btn.disabled = false;
        return;
      }
      if (parts.length === 0) {
        msgEl.className = "msg err";
        msgEl.textContent = t("app.chatTestGeminiNeedTextOrImage");
        clearTimeout(tmr);
        btn.disabled = false;
        return;
      }
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
}

async function refreshAll() {
  await Promise.all([loadProfile(), loadTokens(), loadUsage(), loadBalanceLogs(), loadPricing()]);
}

init().catch(async (e) => {
  await UiDialog.alertError(e.message || t("common.initFailed"));
});
