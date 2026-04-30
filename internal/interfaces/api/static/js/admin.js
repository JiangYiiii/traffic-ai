(function () {
  const t = (k) => (window.I18N ? window.I18N.t(k) : k);

  function userConsoleBase() {
    const m = document.querySelector('meta[name="traffic-ai-user-port"]');
    if (!m?.content) return "";
    return `${window.location.protocol}//${window.location.hostname}:${m.content.trim()}`;
  }

  /** @type {Map<number, object>} */
  const upstreamById = new Map();
  /** @type {Array<object>} */
  let upstreamCatalog = [];
  /** @type {Array<object>} */
  let providerCatalogCache = [];
  let packageModal;
  let userPage = 1;
  const userPageSize = 20;
  let userTotal = 0;
  let redeemPage = 1;
  const redeemPageSize = 20;
  let redeemTotal = 0;
  let adminBlPage = 1;
  const adminBlPageSize = 20;
  let adminBlTotal = 0;
  let usagePage = 1;
  const usagePageSize = 20;
  let usageTotal = 0;
  /** @type {Modal|null} */
  let modelModal;
  let upstreamModal;
  let tokenGroupModal;
  let tgLinksModal;
  let rateLimitModal;
  let bulkPricingModal;
  /** @type {Array<object>} 当前模型表数据，供状态切换等使用 */
  let lastLoadedModels = [];
  /** 串行化模型列表渲染，避免并发 loadModels 造成表格错乱或同一操作被绑定多次 */
  let loadModelsRunChain = Promise.resolve();

  async function api(path, options = {}) {
    const token = localStorage.getItem("accessToken");
    if (!token) {
      window.location.href = "/admin-login.html";
      return null;
    }
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
      window.location.href = "/admin-login.html";
      return null;
    }
    const body = await resp.json().catch(() => ({}));
    if (body.code !== 0) throw new Error(body.message || t("common.requestFailed"));
    return body.data;
  }

  let currentRole = "";

  function isAdminRole(role) {
    return role === "admin" || role === "super_admin";
  }

  function isSuperAdmin() {
    return currentRole === "super_admin";
  }

  function applyRoleTabs() {
    const modelTabItem = document.getElementById("tabItem-model");
    if (!isSuperAdmin() && modelTabItem) {
      modelTabItem.classList.add("d-none");
    }
  }

  function escapeHtml(value) {
    return String(value ?? "").replace(/[&<>"']/g, (ch) => {
      const map = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" };
      return map[ch] || ch;
    });
  }

  /** 微美元 int -> 展示字符串 */
  function fmtMicro(n) {
    const x = Number(n);
    if (!Number.isFinite(x)) return "—";
    return x.toLocaleString();
  }

  function fmtMicroUsd(n) {
    const x = Number(n);
    if (!Number.isFinite(x)) return "—";
    const neg = x < 0;
    const v = Math.abs(Math.trunc(x));
    const intPart = String(Math.floor(v / 1_000_000));
    const frac = String(v % 1_000_000).padStart(6, "0");
    return `${neg ? "-" : ""}$${intPart}.${frac}`;
  }

  function showToast(msg, ok) {
    const el = document.getElementById("adminGlobalMsg");
    if (!el) return;
    el.textContent = msg;
    el.className = `msg mb-3 ${ok ? "ok" : "err"}`;
    el.classList.remove("d-none");
    clearTimeout(showToast._t);
    showToast._t = setTimeout(() => el.classList.add("d-none"), 4000);
  }

  function billingLabel(b) {
    if (b === "per_request") return t("app.pricingModePerRequest");
    return t("app.pricingModePerToken");
  }

  /** 上游表格中展示的接入方式文案（与后端 auth_type 对齐） */
  function authTypeLabelForUpstream(at) {
    if (at === "oauth_authorization_code") return t("admin.upstreams.authOAuth");
    return t("admin.upstreams.authApiKey");
  }

  function fillUpstreamProviderSelect() {
    const sel = document.getElementById("up_provider");
    if (!sel) return;
    const cur = sel.value;
    const tags = new Set();
    providerCatalogCache.forEach((p) => {
      if (p.provider_tag) tags.add(p.provider_tag);
    });
    sel.innerHTML = `<option value="">${escapeHtml(t("admin.upstreams.pickProvider"))}</option>`;
    [...tags]
      .sort()
      .forEach((tag) => {
        const p = providerCatalogCache.find((x) => x.provider_tag === tag);
        const o = document.createElement("option");
        o.value = tag;
        o.textContent = p && p.display_name ? `${p.display_name} (${tag})` : tag;
        sel.appendChild(o);
      });
    if (cur && [...sel.options].some((o) => o.value === cur)) sel.value = cur;
  }

  async function ensureProviderCatalogLoaded() {
    if (providerCatalogCache.length) return;
    const data = await api("/admin/providers");
    if (data && data.providers) providerCatalogCache = data.providers;
  }

  function fillModelProviderFilter() {
    const sel = document.getElementById("modelFilterProvider");
    if (!sel) return;
    const cur = sel.value;
    sel.innerHTML = `<option value="">${escapeHtml(t("admin.models.filterAll"))}</option>`;
    const tags = new Set();
    providerCatalogCache.forEach((p) => {
      if (p.provider_tag) tags.add(p.provider_tag);
    });
    [...tags]
      .sort()
      .forEach((tag) => {
        const o = document.createElement("option");
        o.value = tag;
        o.textContent = tag;
        sel.appendChild(o);
      });
    if (cur && [...sel.options].some((o) => o.value === cur)) sel.value = cur;
  }

  function modelListQueryURL() {
    const prov = document.getElementById("modelFilterProvider")?.value?.trim() || "";
    const q = document.getElementById("modelFilterQ")?.value?.trim() || "";
    const qs = new URLSearchParams();
    if (prov) qs.set("provider", prov);
    if (q) qs.set("q", q);
    const s = qs.toString();
    return s ? `/admin/models?${s}` : "/admin/models";
  }

  function fillUsageLogModelSelect(models) {
    const sel = document.getElementById("usageFilterModel");
    if (!sel || sel.tagName !== "SELECT") return;
    const cur = sel.value;
    sel.innerHTML = `<option value="">${escapeHtml(t("admin.usageLogs.filterModelAll"))}</option>`;
    (models || []).forEach((m) => {
      const o = document.createElement("option");
      o.value = m.model_name;
      o.textContent = `${m.model_name} · ${m.provider}`;
      sel.appendChild(o);
    });
    if (cur && [...sel.options].some((op) => op.value === cur)) sel.value = cur;
  }

  /** 模型列表「连通」列：圆点或未测（模型级汇总，用于无账号占位行） */
  function modelTestStatusHTML(m) {
    const parts = [];
    if (m.last_test_at) parts.push(m.last_test_at);
    if (m.last_test_passed === true && m.last_test_latency_ms != null) {
      parts.push(`${m.last_test_latency_ms}ms`);
    }
    if (m.last_test_passed === false && m.last_test_error) {
      parts.push(m.last_test_error);
    }
    const title = parts.join(" · ");
    const escTitle = escapeHtml(title);
    if (m.last_test_passed == null) {
      return `<span class="text-muted" title="${escTitle}">—</span>`;
    }
    const ok = m.last_test_passed === true;
    const cls = ok ? "model-test-dot model-test-ok" : "model-test-dot model-test-fail";
    const label = ok ? t("admin.models.testStatusOk") : t("admin.models.testStatusFail");
    return `<span class="${cls}" title="${escTitle}" role="img" aria-label="${escapeHtml(label)}"></span>`;
  }

  /** 单条模型账号的连通状态 + 测联通按钮 */
  function accountConnectivityCellHTML(ar) {
    const aid = ar.model_account_id || 0;
    if (!aid) {
      return `<div class="d-flex align-items-center justify-content-center"><span class="text-muted">—</span></div>`;
    }
    const parts = [];
    if (ar.last_test_at) parts.push(ar.last_test_at);
    if (ar.last_test_passed === true && ar.last_test_latency_ms != null) {
      parts.push(`${ar.last_test_latency_ms}ms`);
    }
    if (ar.last_test_passed === false && ar.last_test_error) {
      parts.push(ar.last_test_error);
    }
    const title = parts.join(" · ");
    const escTitle = escapeHtml(title);
    let dot = `<span class="text-muted" title="${escTitle}">—</span>`;
    if (ar.last_test_passed != null) {
      const ok = ar.last_test_passed === true;
      const cls = ok ? "model-test-dot model-test-ok" : "model-test-dot model-test-fail";
      const label = ok ? t("admin.models.testStatusOk") : t("admin.models.testStatusFail");
      dot = `<span class="${cls}" title="${escTitle}" role="img" aria-label="${escapeHtml(label)}"></span>`;
    }
    const btn = `<button type="button" class="btn btn-outline-secondary btn-sm py-0 px-1" data-acc-action="connect-test" data-account-id="${aid}" title="${escapeHtml(t("admin.models.connect"))}">${escapeHtml(t("admin.models.accountConnectTest"))}</button>`;
    return `<div class="d-flex align-items-center justify-content-center flex-wrap gap-1">${dot}${btn}</div>`;
  }

  /** 模型列表「上线状态」列：单条 model_account 是否在线（占位行显示 —） */
  function accountOnlineStatusCellHTML(ar) {
    if (!ar.model_account_id) {
      return `<span class="text-muted">—</span>`;
    }
    const online = ar.status === "online";
    const cls = online ? "pill ok" : "pill off";
    const label = online ? t("admin.account.statusOnline") : t("admin.account.statusOffline");
    return `<span class="${cls}">${escapeHtml(label)}</span>`;
  }

  /** 表格内下拉菜单：避免被 table-responsive 裁剪 */
  function dropdownPopperFixedAttrs() {
    return `data-bs-popper-config='{"strategy":"fixed"}'`;
  }

  /** 模型操作下拉（仅首行 rowspan 展示） */
  function modelActionsDropdownHTML(m) {
    const mid = m.id;
    const pop = dropdownPopperFixedAttrs();
    const parts = [];
    parts.push(`<li><button type="button" class="dropdown-item" data-model-action="test" data-model-id="${mid}">${escapeHtml(t("admin.models.test"))}</button></li>`);
    parts.push(`<li><button type="button" class="dropdown-item" data-model-action="edit" data-model-id="${mid}">${escapeHtml(t("admin.edit"))}</button></li>`);
    parts.push(`<li><button type="button" class="dropdown-item" data-model-action="add-account" data-model-id="${mid}">${escapeHtml(t("admin.account.add"))}</button></li>`);
    parts.push(`<li><button type="button" class="dropdown-item" data-model-action="toggle-active" data-model-id="${mid}">${escapeHtml(m.is_active ? t("action.disable") : t("action.enable"))}</button></li>`);
    parts.push(`<li><button type="button" class="dropdown-item" data-model-action="toggle-listed" data-model-id="${mid}">${escapeHtml(m.is_listed ? t("admin.models.unlistModel") : t("admin.models.listModel"))}</button></li>`);
    parts.push(`<li><hr class="dropdown-divider" /></li>`);
    parts.push(`<li><button type="button" class="dropdown-item text-danger" data-model-action="delete" data-model-id="${mid}">${escapeHtml(t("action.delete"))}</button></li>`);
    return `<div class="dropdown model-table-dropdown">
      <button type="button" class="btn btn-outline-primary btn-sm dropdown-toggle py-0 px-2" ${pop} data-bs-toggle="dropdown" data-bs-display="static" aria-expanded="false">${escapeHtml(t("admin.models.colModelMgmt"))}</button>
      <ul class="dropdown-menu dropdown-menu-end shadow-sm model-table-dropdown-menu">${parts.join("")}</ul>
    </div>`;
  }

  /** 账号操作下拉（每账号一行） */
  function accountActionsDropdownHTML(m, ar) {
    const mid = m.id;
    const aid = ar.model_account_id || 0;
    if (!aid) {
      return `<span class="text-muted small">—</span>`;
    }
    const online = ar.status === "online";
    const pop = dropdownPopperFixedAttrs();
    const parts = [];
    parts.push(`<li><button type="button" class="dropdown-item" data-acc-action="edit" data-model-id="${mid}" data-account-id="${aid}">${escapeHtml(t("admin.edit"))}</button></li>`);
    parts.push(`<li><button type="button" class="dropdown-item" data-acc-action="toggle" data-account-id="${aid}" data-make-active="${online ? "false" : "true"}">${escapeHtml(online ? t("admin.account.offline") : t("admin.account.online"))}</button></li>`);
    parts.push(`<li><button type="button" class="dropdown-item text-danger" data-acc-action="delete" data-model-id="${mid}" data-account-id="${aid}">${escapeHtml(t("admin.account.delShort"))}</button></li>`);
    return `<div class="dropdown model-table-dropdown">
      <button type="button" class="btn btn-outline-primary btn-sm dropdown-toggle py-0 px-2" ${pop} data-bs-toggle="dropdown" data-bs-display="static" aria-expanded="false">${escapeHtml(t("admin.models.colAccountMgmt"))}</button>
      <ul class="dropdown-menu dropdown-menu-end shadow-sm model-table-dropdown-menu">${parts.join("")}</ul>
    </div>`;
  }

  async function toggleModelActive(m) {
    const body = {
      model_name: m.model_name,
      provider: m.provider,
      model_type: m.model_type || "chat",
      billing_type: m.billing_type,
      input_price: m.input_price,
      output_price: m.output_price,
      reasoning_price: m.reasoning_price ?? 0,
      per_request_price: m.per_request_price ?? 0,
      is_active: !m.is_active,
    };
    await api(`/admin/models/${m.id}`, { method: "PUT", body: JSON.stringify(body) });
    showToast(t("admin.models.statusToggled"), true);
    await loadModels();
  }

  async function toggleModelListed(m) {
    const body = {
      model_name: m.model_name,
      provider: m.provider,
      model_type: m.model_type || "chat",
      billing_type: m.billing_type,
      input_price: m.input_price,
      output_price: m.output_price,
      reasoning_price: m.reasoning_price ?? 0,
      per_request_price: m.per_request_price ?? 0,
      is_active: m.is_active,
      is_listed: !m.is_listed,
    };
    await api(`/admin/models/${m.id}`, { method: "PUT", body: JSON.stringify(body) });
    showToast(m.is_listed ? t("admin.models.unlistModel") + " " + t("common.success") : t("admin.models.listModel") + " " + t("common.success"), true);
    await loadModels();
  }

  function fillProviderCatalogSelect() {
    const sel = document.getElementById("providerCatalog");
    if (!sel) return;
    sel.innerHTML = `<option value="">${escapeHtml(t("admin.models.pickVendor"))}</option>`;
    const groups = [];
    const groupMap = {};
    providerCatalogCache.forEach((p) => {
      const g = p.vendor_group || "";
      if (!groupMap[g]) {
        groupMap[g] = [];
        groups.push(g);
      }
      groupMap[g].push(p);
    });
    groups.forEach((g) => {
      const optgroup = document.createElement("optgroup");
      optgroup.label = g || "—";
      groupMap[g].forEach((p) => {
        const opt = document.createElement("option");
        opt.value = p.id;
        const suffix = p.list_supported === false ? t("admin.models.vendorManual") : "";
        opt.textContent = `${p.display_name}${suffix}`;
        optgroup.appendChild(opt);
      });
      sel.appendChild(optgroup);
    });
  }

  function onProviderCatalogChange() {
    const sel = document.getElementById("providerCatalog");
    const authSel = document.getElementById("providerAuthType");
    const baseWrap = document.getElementById("discoverBaseUrlWrap");
    const baseInput = document.getElementById("discoverBaseUrl");
    const id = sel?.value;
    const p = providerCatalogCache.find((x) => x.id === id);
    if (!authSel) return;
    authSel.innerHTML = "";
    if (!p) {
      baseWrap?.classList.add("d-none");
      return;
    }
    (p.auth_types || []).forEach((at) => {
      const o = document.createElement("option");
      o.value = at;
      o.textContent = at === "oauth_bearer" ? "OAuth Bearer" : "API Key";
      authSel.appendChild(o);
    });
    baseWrap?.classList.remove("d-none");
    if (baseInput) {
      baseInput.value = p.default_base_url || "";
      if (p.require_base_url) {
        baseInput.setAttribute("required", "required");
      } else {
        baseInput.removeAttribute("required");
      }
    }
    const baseHelp = document.getElementById("discoverBaseUrlHelp");
    if (baseHelp) {
      baseHelp.textContent = p.require_base_url ? t("admin.models.baseUrlHelpRequired") : t("admin.models.baseUrlHelpOptional");
    }
    const provInput = document.getElementById("provider");
    if (provInput && p.provider_tag) provInput.value = p.provider_tag;
    onAuthTypeChange();
  }

  function onAuthTypeChange() {
    const authSel = document.getElementById("providerAuthType");
    const provSel = document.getElementById("providerCatalog");
    const credWrap = document.getElementById("discoverCredentialWrap");
    const oauthWrap = document.getElementById("oauthAuthorizeWrap");
    const oauthStatus = document.getElementById("oauthStatus");
    const fallbackLink = document.getElementById("oauthFallbackLink");

    const authType = authSel?.value;
    const provId = provSel?.value;
    const p = providerCatalogCache.find((x) => x.id === provId);

    if (authType === "oauth_bearer" && p && p.oauth_config_key) {
      credWrap?.classList.add("d-none");
      oauthWrap?.classList.remove("d-none");
      if (oauthStatus) { oauthStatus.textContent = ""; oauthStatus.className = "small text-muted ms-2"; }
      fallbackLink?.classList.add("d-none");
    } else {
      credWrap?.classList.remove("d-none");
      oauthWrap?.classList.add("d-none");
    }
  }

  /** 列表单选时同步到「模型名」，与保存时权威 id 一致 */
  function syncDiscoverPickToModelName() {
    const pick = document.getElementById("discoverModelPick");
    const mn = document.getElementById("model_name");
    if (!pick || !mn) return;
    const sel = [...pick.selectedOptions];
    if (sel.length === 1) {
      mn.value = sel[0].value;
    }
  }

  async function runDiscoverModels() {
    const provider = document.getElementById("providerCatalog")?.value;
    const auth_type = document.getElementById("providerAuthType")?.value;
    const credential = document.getElementById("discoverCredential")?.value?.trim() || "";
    const wrap = document.getElementById("discoverBaseUrlWrap");
    const baseEl = document.getElementById("discoverBaseUrl");
    const base_url = wrap && !wrap.classList.contains("d-none") && baseEl ? baseEl.value.trim() : "";
    const statusEl = document.getElementById("discoverStatus");
    const pickWrap = document.getElementById("discoverPickWrap");
    const pick = document.getElementById("discoverModelPick");
    if (!provider || !auth_type || !credential) {
      if (statusEl) statusEl.textContent = t("admin.models.discoverNeed");
      return;
    }
    if (statusEl) statusEl.textContent = t("admin.models.discoverLoading");
    pickWrap?.classList.add("d-none");
    if (pick) pick.innerHTML = "";
    try {
      const data = await api("/admin/provider-models/discover", {
        method: "POST",
        body: JSON.stringify({ provider, auth_type, credential, base_url: base_url || undefined }),
      });
      if (!data) return;
      if (data.fetch_failed) {
        if (statusEl) statusEl.textContent = data.message || t("admin.models.discoverFail");
        return;
      }
      const models = data.models || [];
      if (pick) {
        models.forEach((m) => {
          const o = document.createElement("option");
          o.value = m.id;
          o.textContent = m.id;
          pick.appendChild(o);
        });
      }
      if (statusEl) statusEl.textContent = t("admin.models.discoverOk").replace("{n}", String(models.length));
      pickWrap?.classList.toggle("d-none", models.length === 0);
    } catch (e) {
      if (statusEl) statusEl.textContent = e.message || t("common.requestFailed");
    }
  }

  let batchStagedModels = [];

  function discoverBatchAdd() {
    const cred = document.getElementById("discoverCredential")?.value?.trim() || "";
    if (!cred) {
      showToast(t("admin.models.discoverNeedCred"), false);
      return;
    }
    const pick = document.getElementById("discoverModelPick");
    const selected = pick ? [...pick.selectedOptions].map((o) => o.value) : [];
    if (!selected.length) {
      showToast(t("admin.models.discoverPickNone"), false);
      return;
    }
    const provider = document.getElementById("provider")?.value?.trim() || "";
    if (!provider) {
      showToast(t("admin.models.needProviderField"), false);
      return;
    }

    batchStagedModels = selected;

    if (pick) pick.disabled = true;
    const btn = document.getElementById("btnDiscoverBatchAdd");
    if (btn) btn.disabled = true;

    const badge = document.getElementById("batchStagedBadge");
    if (badge) {
      badge.textContent = t("admin.models.batchStaged").replace("{n}", String(selected.length));
      badge.classList.remove("d-none");
    }

    const mn = document.getElementById("model_name");
    if (mn) {
      mn.value = selected.length === 1 ? selected[0] : t("admin.models.batchPlaceholder").replace("{n}", String(selected.length));
      mn.disabled = true;
    }
  }

  async function rebuildUpstreamCatalog() {
    upstreamById.clear();
    upstreamCatalog = [];
    const models = await api("/admin/models");
    if (!models) return;
    for (const m of models) {
      const ups = await api(`/admin/models/${m.id}/model-accounts`);
      if (!ups) return;
      for (const u of ups) {
        const row = { ...u, model_name: m.model_name, model_id: m.id };
        upstreamById.set(u.id, row);
        upstreamCatalog.push(row);
      }
    }
  }

  function syncPlaygroundSelect(models) {
    const sel = document.getElementById("pgModelId");
    if (!sel) return;
    const cur = sel.value;
    sel.innerHTML = `<option value="">${escapeHtml(t("admin.playground.pickModelPh"))}</option>`;
    (models || []).forEach((m) => {
      const opt = document.createElement("option");
      opt.value = String(m.id);
      const mt = (m.model_type || "chat").toLowerCase();
      opt.dataset.modelType = mt;
      opt.textContent = `${m.model_name} · ${m.provider} · ${mt}`;
      sel.appendChild(opt);
    });
    if (cur && [...sel.options].some((o) => o.value === cur)) sel.value = cur;
    syncPlaygroundForm();
  }

  function getPlaygroundModelType() {
    const sel = document.getElementById("pgModelId");
    const opt = sel?.selectedOptions?.[0];
    return String(opt?.dataset?.modelType || "chat").toLowerCase();
  }

  function syncPlaygroundForm() {
    const kind = getPlaygroundModelType();
    const hint = document.getElementById("pgHint");
    const maxWrap = document.getElementById("pgMaxTokensWrap");
    const uLabel = document.getElementById("pgUserLabel");
    const oLabel = document.getElementById("pgOutLabel");
    if (hint) {
      if (kind === "embedding") hint.textContent = t("admin.playground.hintEmbedding");
      else if (kind === "image") hint.textContent = t("admin.playground.hintImage");
      else if (kind === "speech") hint.textContent = t("admin.playground.hintSpeech");
      else hint.textContent = t("admin.playground.hint");
    }
    if (maxWrap) maxWrap.hidden = kind === "embedding" || kind === "image" || kind === "speech";
    if (uLabel) {
      if (kind === "embedding") uLabel.textContent = t("admin.playground.inputEmbed");
      else if (kind === "image") uLabel.textContent = t("admin.playground.inputImage");
      else uLabel.textContent = t("admin.playground.userMsg");
    }
    if (oLabel) {
      if (kind === "embedding") oLabel.textContent = t("admin.playground.outEmbed");
      else if (kind === "image") oLabel.textContent = t("admin.playground.outImage");
      else oLabel.textContent = t("admin.playground.assistant");
    }
  }

  async function runPlayground() {
    const id = document.getElementById("pgModelId")?.value;
    const msg = document.getElementById("pgUserMsg")?.value?.trim() || "";
    const maxTok = Number(document.getElementById("pgMaxTokens")?.value) || 256;
    const out = document.getElementById("pgOut");
    const imgBox = document.getElementById("pgImagePreview");
    if (!id) {
      showToast(t("admin.playground.needModel"), false);
      return;
    }
    if (imgBox) {
      imgBox.hidden = true;
      imgBox.replaceChildren();
    }
    const messages = [{ role: "user", content: msg || t("admin.playground.defaultMsg") }];
    if (out) out.textContent = t("common.loading");
    try {
      const data = await api(`/admin/models/${id}/playground`, {
        method: "POST",
        body: JSON.stringify({ messages, max_tokens: maxTok }),
      });
      if (!data || !out) return;
      if (data.success) {
        const latency = data.latency_ms != null ? `\n\n— ${data.latency_ms} ms` : "";
        if (data.result_kind === "image" && data.image_data_url && imgBox) {
          imgBox.hidden = false;
          const img = document.createElement("img");
          img.src = data.image_data_url;
          img.alt = "preview";
          img.className = "mb-2 rounded border";
          img.style.maxWidth = "100%";
          img.style.height = "auto";
          imgBox.appendChild(img);
        }
        out.textContent = (data.assistant || "—") + latency;
        showToast(t("admin.playground.ok"), true);
      } else {
        let line = data.error || t("common.requestFailed");
        if (data.raw_body_snippet) line += `\n\n${data.raw_body_snippet}`;
        out.textContent = line;
        showToast(t("admin.playground.fail"), false);
      }
    } catch (e) {
      if (out) out.textContent = e.message || "—";
      showToast(e.message, false);
    }
  }

  function syncTgLinksSelect(excludeIds) {
    const sel = document.getElementById("tgLinksSelect");
    if (!sel) return;
    const ex = new Set(excludeIds || []);
    sel.innerHTML = `<option value="">${t("admin.tokenGroups.pickUpstream")}</option>`;
    upstreamCatalog
      .filter((u) => !ex.has(u.id))
      .forEach((u) => {
        const opt = document.createElement("option");
        opt.value = String(u.id);
        opt.textContent = `#${u.id} · ${u.model_name} · ${u.name || u.endpoint}`;
        sel.appendChild(opt);
      });
  }

  async function testAllEnabledModels() {
    const btn = document.getElementById("btnModelConnect");
    const accountIds = [];
    for (const m of lastLoadedModels) {
      if (!m.is_active) continue;
      let accounts = [];
      try {
        accounts = (await api(`/admin/models/${m.id}/model-accounts`)) || [];
      } catch (_) {
        continue;
      }
      for (const a of accounts) {
        if (a.is_active && a.id) accountIds.push(a.id);
      }
    }
    if (!accountIds.length) {
      showToast(t("admin.models.connectNoEnabled"), false);
      return;
    }
    const labelOk = t("admin.models.connect");
    if (btn) {
      btn.disabled = true;
      btn.textContent = t("admin.models.connectRunning");
    }
    let ok = 0;
    let fail = 0;
    try {
      for (const aid of accountIds) {
        try {
          const result = await api(`/admin/model-accounts/${aid}/test`, { method: "POST" });
          if (result && result.success) ok += 1;
          else fail += 1;
        } catch (_) {
          fail += 1;
        }
      }
      await loadModels();
      const summary = t("admin.models.connectSummary")
        .replace("{ok}", String(ok))
        .replace("{fail}", String(fail))
        .replace("{total}", String(accountIds.length));
      showToast(summary, fail === 0);
    } catch (e) {
      showToast(e.message || t("common.requestFailed"), false);
    } finally {
      if (btn) {
        btn.disabled = false;
        btn.textContent = labelOk;
      }
    }
  }

  async function handleModelAccountActionClick(btn) {
    const action = btn.getAttribute("data-acc-action");
    const modelId = Number(btn.getAttribute("data-model-id"));
    const accountId = Number(btn.getAttribute("data-account-id"));
    if (action === "connect-test") {
      const label = btn.textContent;
      btn.disabled = true;
      btn.textContent = t("common.loading");
      try {
        const result = await api(`/admin/model-accounts/${accountId}/test`, { method: "POST" });
        if (result.success) {
          showToast(`${t("admin.models.testOk")} ${result.latency_ms}ms`, true);
        } else {
          showToast(`${t("admin.models.testFail")}: ${result.error || ""}`, false);
        }
        await loadModels();
      } catch (err) {
        showToast(err.message, false);
      } finally {
        btn.disabled = false;
        btn.textContent = label;
      }
      return;
    }
    if (action === "edit") {
      window._editModelAccount(modelId, accountId);
      return;
    }
    if (action === "toggle") {
      const makeActive = btn.getAttribute("data-make-active");
      await window._toggleModelAccount(accountId, makeActive);
      return;
    }
    if (action === "delete") {
      await window._deleteModelAccount(modelId, accountId);
    }
  }

  async function handleModelRowActionClick(btn) {
    const action = btn.getAttribute("data-model-action");
    const id = Number(btn.getAttribute("data-model-id"));
    const mod = lastLoadedModels.find((x) => x.id === id);
    if (!mod) return;
    if (action === "edit") {
      openModelModal(mod);
      return;
    }
    if (action === "add-account") {
      await openUpstreamModal({ modelId: id, modelProvider: mod.provider || "" });
      return;
    }
    if (action === "toggle-active") {
      try {
        await toggleModelActive(mod);
      } catch (err) {
        showToast(err.message, false);
      }
      return;
    }
    if (action === "toggle-listed") {
      try {
        await toggleModelListed(mod);
      } catch (err) {
        showToast(err.message, false);
      }
      return;
    }
    if (action === "delete") {
      const ok = window.confirm(t("admin.deleteModelConfirm").replace("{model}", mod.model_name));
      if (!ok) return;
      try {
        await api(`/admin/models/${id}`, { method: "DELETE" });
        showToast(t("admin.deletedOk"), true);
        await refreshAll();
      } catch (err) {
        showToast(err.message, false);
      }
      return;
    }
    if (action === "test") {
      const label = btn.textContent;
      btn.disabled = true;
      btn.textContent = t("common.loading");
      try {
        const result = await api(`/admin/models/${id}/test`, { method: "POST" });
        if (result.success) {
          showToast(`${t("admin.models.testOk")} ${result.latency_ms}ms`, true);
        } else {
          showToast(`${t("admin.models.testFail")}: ${result.error}`, false);
        }
        await loadModels();
      } catch (err) {
        showToast(err.message, false);
      } finally {
        btn.disabled = false;
        btn.textContent = label;
      }
    }
  }

  function installModelTableRowDelegation() {
    const tbody = document.getElementById("modelTableBody");
    if (!tbody || tbody.dataset.modelTableDelegated === "1") return;
    tbody.dataset.modelTableDelegated = "1";
    tbody.addEventListener("click", (e) => {
      const raw = e.target;
      if (!(raw instanceof Element)) return;

      const expandBtn = raw.closest(".btn-expand");
      if (expandBtn && tbody.contains(expandBtn)) {
        e.preventDefault();
        toggleModelExpand(expandBtn);
        return;
      }

      const upBtn = raw.closest("[data-action='add-upstream']");
      if (upBtn && tbody.contains(upBtn)) {
        e.preventDefault();
        void (async () => {
          const mid = Number(upBtn.getAttribute("data-model-id"));
          const mp = upBtn.getAttribute("data-model-provider") || "";
          await openUpstreamModal({ modelId: mid, modelProvider: mp });
        })();
        return;
      }

      const accBtn = raw.closest("[data-acc-action]");
      if (accBtn && tbody.contains(accBtn)) {
        e.preventDefault();
        void handleModelAccountActionClick(accBtn);
        return;
      }

      const modelBtn = raw.closest("[data-model-action]");
      if (modelBtn && tbody.contains(modelBtn)) {
        e.preventDefault();
        void handleModelRowActionClick(modelBtn);
      }
    });
  }

  async function loadModelsImpl() {
    const tbody = document.getElementById("modelTableBody");
    if (!tbody) return;
    const url = modelListQueryURL();
    let models;
    let usageOptions;
    if (url.includes("?")) {
      const pair = await Promise.all([api(url), api("/admin/models")]);
      models = pair[0];
      usageOptions = pair[1];
    } else {
      models = await api(url);
      usageOptions = models;
    }
    if (!models) return;
    fillUsageLogModelSelect(usageOptions || []);
    lastLoadedModels = models;
    tbody.innerHTML = "";
    if (!models.length) {
      syncPlaygroundSelect([]);
      const tr = document.createElement("tr");
      tr.innerHTML = `<td colspan="18" class="text-muted text-center py-4">${escapeHtml(t("admin.models.empty"))}</td>`;
      tbody.appendChild(tr);
      return;
    }
    syncPlaygroundSelect(models);

    for (const m of models) {
      let accounts = [];
      try { accounts = (await api(`/admin/models/${m.id}/model-accounts`)) || []; } catch(e) {}

      // 展示列：以模型账号自身字段为准。
      const acctRows = [];
      for (const a of accounts) {
        acctRows.push({
          account_id: a.id,
          account_name: a.name || '—',
          provider: a.provider || m.provider || '—',
          auth_type: a.auth_type || 'api_key',
          status: a.status || (a.is_active ? 'online' : 'offline'),
          model_account_id: a.id,
          last_test_passed: a.last_test_passed,
          last_test_at: a.last_test_at,
          last_test_latency_ms: a.last_test_latency_ms,
          last_test_error: a.last_test_error,
        });
      }
      if (acctRows.length === 0) {
        acctRows.push({ account_name: '—', provider: m.provider || '—', auth_type: '—', account_id: 0, status: '—', model_account_id: 0 });
      }

      const rowspan = acctRows.length;
      acctRows.forEach((ar, idx) => {
        const tr = document.createElement("tr");
        tr.dataset.modelId = String(m.id);
        let html = '';
        if (idx === 0) {
          html += `<td class="text-center" rowspan="${rowspan}"><input type="checkbox" class="form-check-input model-table-cb model-row-cb" data-model-id="${m.id}" /></td>`;
          html += `<td rowspan="${rowspan}"><button type="button" class="ghost btn-expand p-0 border-0 bg-transparent" data-action="expand" aria-expanded="false" title="Expand">▸</button></td>`;
          html += `<td rowspan="${rowspan}">${escapeHtml(m.model_name)}</td>`;
          html += `<td rowspan="${rowspan}">${escapeHtml(m.model_type || "—")}</td>`;
          html += `<td rowspan="${rowspan}">${escapeHtml(billingLabel(m.billing_type))}</td>`;
          html += `<td rowspan="${rowspan}">${escapeHtml(fmtMicro(m.input_price))}</td>`;
          html += `<td rowspan="${rowspan}">${escapeHtml(fmtMicro(m.output_price))}</td>`;
          html += `<td rowspan="${rowspan}">${escapeHtml(fmtMicro(m.reasoning_price ?? 0))}</td>`;
          html += `<td rowspan="${rowspan}">${escapeHtml(fmtMicro(m.per_request_price ?? 0))}</td>`;
          html += `<td class="text-center" rowspan="${rowspan}"><span class="pill ${m.is_active ? "ok" : "off"}">${m.is_active ? t("status.enabled") : t("status.disabled")}</span></td>`;
          html += `<td class="text-center" rowspan="${rowspan}"><span class="pill ${m.is_listed ? "ok" : "off"}">${m.is_listed ? t("admin.models.listModel") : t("admin.models.unlistModel")}</span></td>`;
        }
        html += `<td>${escapeHtml(ar.account_name)}</td>`;
        html += `<td>${escapeHtml(ar.provider)}</td>`;
        html += `<td>${escapeHtml(ar.auth_type)}</td>`;
        html += `<td class="text-center text-nowrap">${accountConnectivityCellHTML(ar)}</td>`;
        html += `<td class="text-center text-nowrap">${accountOnlineStatusCellHTML(ar)}</td>`;
        if (idx === 0) {
          html += `<td class="text-end text-nowrap model-row-actions" rowspan="${rowspan}"><div class="d-inline-flex justify-content-end align-items-center">${modelActionsDropdownHTML(m)}</div></td>`;
        }
        html += `<td class="text-end text-nowrap model-row-actions"><div class="d-inline-flex justify-content-end align-items-center">${accountActionsDropdownHTML(m, ar)}</div></td>`;
        tr.innerHTML = html;
        tbody.appendChild(tr);
      });

      const detail = document.createElement("tr");
      detail.className = "d-none upstream-detail-row";
      detail.dataset.modelId = String(m.id);
      detail.innerHTML = `
        <td colspan="18" class="bg-light border-top-0 pt-0 pb-3">
          <div class="p-2 ps-4">
            <div class="d-flex flex-wrap align-items-center justify-content-between gap-2 mb-2">
              <span class="small fw-semibold text-muted" data-i18n="admin.upstreams.title">${escapeHtml(t("admin.upstreams.title"))}</span>
              <button type="button" class="btn btn-primary btn-sm" data-action="add-upstream" data-model-id="${m.id}" data-model-provider="${escapeHtml(m.provider || "")}">${escapeHtml(t("admin.upstreams.add"))}</button>
            </div>
            <div class="table-responsive console-table-shell">
              <table class="table table-sm align-middle mb-0">
                <thead>
                  <tr>
                    <th>${escapeHtml(t("table.name"))}</th>
                    <th>${escapeHtml(t("admin.upstreams.colProvider"))}</th>
                    <th>${escapeHtml(t("admin.upstreams.authType"))}</th>
                    <th>${escapeHtml(t("admin.upstreams.colEndpoint"))}</th>
                    <th>${escapeHtml(t("admin.upstreams.colProtocol"))}</th>
                    <th>${escapeHtml(t("admin.upstreams.colWeight"))}</th>
                    <th>${escapeHtml(t("admin.upstreams.colTimeout"))}</th>
                    <th>${escapeHtml(t("table.status"))}</th>
                    <th>${escapeHtml(t("table.action"))}</th>
                  </tr>
                </thead>
                <tbody class="upstream-tbody" data-model-id="${m.id}">
                  <tr><td colspan="9" class="text-muted small">${escapeHtml(t("common.loading"))}</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </td>`;
      tbody.appendChild(detail);
    }
  }

  function loadModels() {
    const p = loadModelsRunChain.then(() => loadModelsImpl());
    loadModelsRunChain = p.catch(() => {});
    return p;
  }

  async function toggleModelExpand(btn) {
    const tr = btn.closest("tr");
    const id = tr?.dataset?.modelId;
    if (!id) return;
    const detail = document.querySelector(`tr.upstream-detail-row[data-model-id="${id}"]`);
    if (!detail) return;
    const open = detail.classList.contains("d-none");
    detail.classList.toggle("d-none", !open);
    btn.setAttribute("aria-expanded", open ? "true" : "false");
    btn.textContent = open ? "▾" : "▸";
    if (open) await loadUpstreamsForModel(Number(id));
  }

  async function loadUpstreamsForModel(modelId) {
    const ut = document.querySelector(`tbody.upstream-tbody[data-model-id="${modelId}"]`);
    if (!ut) return;
    try {
      const ups = await api(`/admin/models/${modelId}/model-accounts`);
      if (!ups) return;
      ut.innerHTML = "";
      if (!ups.length) {
        ut.innerHTML = `<tr><td colspan="9" class="text-muted small">${escapeHtml(t("admin.upstreams.empty"))}</td></tr>`;
        return;
      }
      ups.forEach((u) => {
        const tr = document.createElement("tr");
        tr.innerHTML = `
          <td>${escapeHtml(u.name || "—")}</td>
          <td class="small">${escapeHtml(u.provider || "—")}</td>
          <td class="small">${escapeHtml(authTypeLabelForUpstream(u.auth_type))}</td>
          <td class="text-break small">${escapeHtml(u.endpoint)}</td>
          <td>${escapeHtml(u.protocol || "—")}</td>
          <td>${u.weight}</td>
          <td>${u.timeout_sec}</td>
          <td><span class="pill ${u.is_active ? "ok" : "off"}">${u.is_active ? t("status.enabled") : t("status.disabled")}</span></td>
          <td>
            <div class="d-flex flex-wrap gap-1">
              <button type="button" class="ghost btn-sm" data-up-edit="${u.id}">${escapeHtml(t("admin.edit"))}</button>
              <button type="button" class="ghost btn-sm text-danger" data-up-del="${u.id}" data-model-id="${modelId}">${escapeHtml(t("action.delete"))}</button>
            </div>
          </td>`;
        ut.appendChild(tr);
      });
      ut.querySelectorAll("[data-up-edit]").forEach((b) => {
        b.addEventListener("click", async () => {
          const uid = Number(b.getAttribute("data-up-edit"));
          const ups = await api(`/admin/models/${modelId}/model-accounts`);
          const u = ups?.find((x) => x.id === uid);
          if (u) await openUpstreamModal({ modelId, edit: u });
        });
      });
      ut.querySelectorAll("[data-up-del]").forEach((b) => {
        b.addEventListener("click", async () => {
          const uid = Number(b.getAttribute("data-up-del"));
          if (!window.confirm(t("admin.upstreams.deleteConfirm"))) return;
          try {
            await api(`/admin/model-accounts/${uid}`, { method: "DELETE" });
            showToast(t("admin.deletedOk"), true);
            await loadUpstreamsForModel(modelId);
            await rebuildUpstreamCatalog();
            await loadTokenGroups();
          } catch (e) {
            showToast(e.message, false);
          }
        });
      });
    } catch (e) {
      ut.innerHTML = `<tr><td colspan="9" class="msg err">${escapeHtml(e.message)}</td></tr>`;
    }
  }

  async function openModelModal(m) {
    document.getElementById("modelModalErr")?.classList.add("d-none");
    const isEdit = !!m;
    document.getElementById("modelModalTitle").textContent = isEdit ? t("admin.models.editTitle") : t("admin.models.add");
    document.getElementById("modelEditId").value = m ? String(m.id) : "";

    const mn = document.getElementById("model_name");
    if (mn) { mn.value = m?.model_name || ""; mn.disabled = false; }

    document.getElementById("provider").value = m?.provider || "";
    document.getElementById("model_type").value = m?.model_type || "chat";
    document.getElementById("billing_type").value = m?.billing_type || "per_token";
    document.getElementById("input_price").value = m ? String(m.input_price) : "1";
    document.getElementById("output_price").value = m ? String(m.output_price) : "1";
    document.getElementById("reasoning_price").value = m ? String(m.reasoning_price ?? 0) : "1";
    document.getElementById("per_request_price").value = m ? String(m.per_request_price ?? 0) : "1";
    document.getElementById("model_is_active").checked = m ? !!m.is_active : true;
    document.getElementById("model_is_listed").checked = m ? !!m.is_listed : false;

    batchStagedModels = [];
    const badge = document.getElementById("batchStagedBadge");
    if (badge) badge.classList.add("d-none");

    const discoverSec = document.getElementById("modelDiscoverSection");
    if (discoverSec) discoverSec.classList.toggle("d-none", isEdit);
    if (!isEdit) {
      const cred = document.getElementById("discoverCredential");
      if (cred) cred.value = "";
      const st = document.getElementById("discoverStatus");
      if (st) st.textContent = "";
      document.getElementById("discoverPickWrap")?.classList.add("d-none");
      const dm = document.getElementById("discoverModelPick");
      if (dm) { dm.innerHTML = ""; dm.disabled = false; }
      const batchBtn = document.getElementById("btnDiscoverBatchAdd");
      if (batchBtn) batchBtn.disabled = false;
      const cat = document.getElementById("providerCatalog");
      if (cat) cat.value = "";
      try {
        await ensureProviderCatalogLoaded();
        fillProviderCatalogSelect();
        onProviderCatalogChange();
      } catch (_) {}
    }
    // 旧的 provider_accounts 勾选面板已废弃：模型账号改为直接在模型行内管理。
    const acctContainer = document.getElementById("modelAccountCheckboxes");
    if (acctContainer) acctContainer.innerHTML = "";

    modelModal?.show();
    window.I18N?.applyI18n?.();
  }

  async function saveModel() {
    const errEl = document.getElementById("modelModalErr");
    errEl?.classList.add("d-none");
    const id = document.getElementById("modelEditId").value;

    const provider = document.getElementById("provider").value.trim();
    const model_type = document.getElementById("model_type").value.trim() || "chat";
    const billing_type = document.getElementById("billing_type").value;
    const input_price = Number(document.getElementById("input_price").value) || 0;
    const output_price = Number(document.getElementById("output_price").value) || 0;
    const reasoning_price = Number(document.getElementById("reasoning_price").value) || 0;
    const per_request_price = Number(document.getElementById("per_request_price").value) || 0;
    const is_active = document.getElementById("model_is_active").checked;
    const is_listed = document.getElementById("model_is_listed").checked;

    if (batchStagedModels.length > 0 && !id) {
      if (!provider) {
        errEl.textContent = t("admin.models.needProviderField");
        errEl.classList.remove("d-none");
        return;
      }
      const cred = document.getElementById("discoverCredential")?.value?.trim() || "";
      const catVal = document.getElementById("providerCatalog")?.value || "";
      const p = providerCatalogCache.find((x) => x.id === catVal);
      const wrap = document.getElementById("discoverBaseUrlWrap");
      const baseEl = document.getElementById("discoverBaseUrl");
      const base_url = wrap && !wrap.classList.contains("d-none") && baseEl ? baseEl.value.trim() : "";
      if (p?.require_base_url && !base_url) {
        errEl.textContent = t("admin.models.baseUrlHelpRequired");
        errEl.classList.remove("d-none");
        return;
      }
      const items = batchStagedModels.map((model_name) => ({
        model_name,
        provider,
        model_type,
        billing_type,
        input_price,
        output_price,
        reasoning_price,
        per_request_price,
        is_active,
        is_listed,
        account_credential: cred,
        ...(base_url ? { account_endpoint: base_url } : {}),
      }));
      try {
        const data = await api("/admin/models/batch", { method: "POST", body: JSON.stringify({ items }) });
        const ok = (data?.created || []).length;
        const bad = (data?.failed || []).length;
        showToast(`${t("admin.models.batchDone")} ${ok} OK, ${bad} fail`, bad === 0);
        batchStagedModels = [];
        modelModal?.hide();
        await refreshAll();
      } catch (e) {
        errEl.textContent = e.message;
        errEl.classList.remove("d-none");
      }
      return;
    }

    const pickWrap = document.getElementById("discoverPickWrap");
    const pick = document.getElementById("discoverModelPick");
    const pickVisible = pickWrap && !pickWrap.classList.contains("d-none");
    const nPick = pick ? pick.selectedOptions.length : 0;
    let modelName = document.getElementById("model_name").value.trim();
    if (!id && pickVisible && nPick === 1 && pick) {
      modelName = pick.selectedOptions[0].value.trim();
    }
    const body = {
      model_name: modelName,
      provider,
      model_type,
      billing_type,
      input_price,
      output_price,
      reasoning_price,
      per_request_price,
      is_active,
      is_listed,
    };
    if (!id) {
      const dc = document.getElementById("discoverCredential")?.value?.trim() || "";
      if (dc) {
        body.account_credential = dc;
        const db = document.getElementById("discoverBaseUrl")?.value?.trim() || "";
        if (db) body.account_endpoint = db;
      }
    }
    try {
      if (id) {
        await api(`/admin/models/${id}`, { method: "PUT", body: JSON.stringify(body) });
      } else {
        await api("/admin/models", { method: "POST", body: JSON.stringify(body) });
      }

      modelModal?.hide();
      showToast(t("admin.savedOk"), true);
      await refreshAll();
    } catch (e) {
      errEl.textContent = e.message;
      errEl.classList.remove("d-none");
    }
  }

  function logger_warn(...args) { if (typeof console !== 'undefined') console.warn(...args); }

  function openBulkPricingModal() {
    const ids = [...document.querySelectorAll("#modelTableBody > tr:not(.upstream-detail-row) .model-row-cb:checked")].map((cb) =>
      Number(cb.dataset.modelId)
    );
    if (!ids.length) {
      showToast(t("admin.models.bulkNoneSelected"), false);
      return;
    }
    document.getElementById("bulkPricingErr")?.classList.add("d-none");
    bulkPricingModal?.show();
  }

  async function saveBulkPricing() {
    const errEl = document.getElementById("bulkPricingErr");
    errEl?.classList.add("d-none");
    const ids = [...document.querySelectorAll("#modelTableBody > tr:not(.upstream-detail-row) .model-row-cb:checked")].map((cb) =>
      Number(cb.dataset.modelId)
    );
    if (!ids.length) {
      showToast(t("admin.models.bulkNoneSelected"), false);
      return;
    }
    const body = {
      ids,
      billing_type: document.getElementById("bulkPricingBilling")?.value || "per_token",
      input_price: Number(document.getElementById("bulkPricingIn")?.value) || 0,
      output_price: Number(document.getElementById("bulkPricingOut")?.value) || 0,
      reasoning_price: Number(document.getElementById("bulkPricingReason")?.value) || 0,
      per_request_price: Number(document.getElementById("bulkPricingPer")?.value) || 0,
    };
    try {
      await api("/admin/models/batch", { method: "PUT", body: JSON.stringify(body) });
      bulkPricingModal?.hide();
      document.getElementById("modelSelectAll").checked = false;
      showToast(t("admin.savedOk"), true);
      await refreshAll();
    } catch (e) {
      if (errEl) {
        errEl.textContent = e.message;
        errEl.classList.remove("d-none");
      }
    }
  }

  async function openUpstreamModal({ modelId, edit, modelProvider }) {
    document.getElementById("upstreamModalErr")?.classList.add("d-none");
    await ensureProviderCatalogLoaded();
    fillUpstreamProviderSelect();
    document.getElementById("upstreamModalTitle").textContent = edit ? t("admin.upstreams.editTitle") : t("admin.upstreams.add");
    document.getElementById("upstreamEditId").value = edit ? String(edit.id) : "";
    document.getElementById("upstreamModelId").value = String(modelId);

    const wantProv = (edit && edit.provider) || modelProvider || "";
    const ps = document.getElementById("up_provider");
    if (ps) {
      if (wantProv && ![...ps.options].some((o) => o.value === wantProv)) {
        const o = document.createElement("option");
        o.value = wantProv;
        o.textContent = wantProv;
        ps.appendChild(o);
      }
      ps.value = wantProv;
    }

    const atSel = document.getElementById("up_auth_type");
    if (atSel) {
      atSel.innerHTML = "";
      const o1 = document.createElement("option");
      o1.value = "api_key";
      o1.textContent = t("admin.upstreams.authApiKey");
      atSel.appendChild(o1);
      const o2 = document.createElement("option");
      o2.value = "oauth_authorization_code";
      o2.textContent = t("admin.upstreams.authOAuth");
      atSel.appendChild(o2);
      atSel.value = edit?.auth_type === "oauth_authorization_code" ? "oauth_authorization_code" : "api_key";
    }

    document.getElementById("up_name").value = edit?.name || "";
    document.getElementById("up_endpoint").value = edit?.endpoint || "";
    document.getElementById("up_credential").value = "";
    document.getElementById("up_protocol").value = edit?.protocol || "chat";
    document.getElementById("up_weight").value = edit ? String(edit.weight || 1) : "1";
    document.getElementById("up_timeout_sec").value = edit ? String(edit.timeout_sec || 60) : "60";
    document.getElementById("up_is_active").checked = edit ? !!edit.is_active : true;
    upstreamModal?.show();
  }

  async function saveUpstream() {
    const errEl = document.getElementById("upstreamModalErr");
    errEl?.classList.add("d-none");
    const eid = document.getElementById("upstreamEditId").value;
    const modelId = Number(document.getElementById("upstreamModelId").value);
    const credRaw = document.getElementById("up_credential").value;
    const cred = credRaw.trim();
    const body = {
      name: document.getElementById("up_name").value.trim(),
      provider: document.getElementById("up_provider")?.value?.trim() || "",
      auth_type: document.getElementById("up_auth_type")?.value || "api_key",
      endpoint: document.getElementById("up_endpoint").value.trim(),
      protocol: document.getElementById("up_protocol").value.trim() || "chat",
      weight: Number(document.getElementById("up_weight").value) || 1,
      timeout_sec: Number(document.getElementById("up_timeout_sec").value) || 60,
      is_active: document.getElementById("up_is_active").checked,
    };
    if (!eid && !cred) {
      errEl.textContent = t("admin.upstreams.needCredential");
      errEl.classList.remove("d-none");
      return;
    }
    if (!eid) body.credential = cred;
    else if (cred) body.credential = cred;
    try {
      if (eid) {
        await api(`/admin/model-accounts/${eid}`, { method: "PUT", body: JSON.stringify(body) });
      } else {
        await api(`/admin/models/${modelId}/model-accounts`, { method: "POST", body: JSON.stringify(body) });
      }
      upstreamModal?.hide();
      showToast(t("admin.savedOk"), true);
      await loadUpstreamsForModel(modelId);
      await rebuildUpstreamCatalog();
      await loadTokenGroups();
    } catch (e) {
      errEl.textContent = e.message;
      errEl.classList.remove("d-none");
    }
  }

  async function loadTokenGroups() {
    const tbody = document.getElementById("tokenGroupTableBody");
    if (!tbody) return;
    const groups = await api("/admin/token-groups");
    if (!groups) return;
    tbody.innerHTML = "";
    if (!groups.length) {
      tbody.innerHTML = `<tr><td colspan="5" class="text-muted text-center py-3">${escapeHtml(t("admin.tokenGroups.empty"))}</td></tr>`;
      return;
    }
    for (const g of groups) {
      let cnt = "—";
      try {
        const linkData = await api(`/admin/token-groups/${g.id}/model-accounts`);
        const arr = linkData && (linkData.model_account_ids || linkData.upstream_ids);
        if (Array.isArray(arr)) cnt = String(arr.length);
      } catch (_) {
        cnt = "—";
      }
      const tr = document.createElement("tr");
      tr.innerHTML = `
        <td>${escapeHtml(g.name)}</td>
        <td class="small">${escapeHtml(g.description || "—")}</td>
        <td><span class="pill ${g.is_active ? "ok" : "off"}">${g.is_active ? t("status.enabled") : t("status.disabled")}</span></td>
        <td>${escapeHtml(cnt)}</td>
        <td>
          <button type="button" class="ghost btn-sm" data-tg-links="${g.id}">${escapeHtml(t("admin.tokenGroups.manageLinks"))}</button>
        </td>`;
      tbody.appendChild(tr);
    }
    tbody.querySelectorAll("[data-tg-links]").forEach((b) => {
      b.addEventListener("click", () => openTgLinksModal(Number(b.getAttribute("data-tg-links"))));
    });
  }

  async function openTgLinksModal(groupId) {
    document.getElementById("tgLinksErr")?.classList.add("d-none");
    document.getElementById("tgLinksGroupId").value = String(groupId);
    await rebuildUpstreamCatalog();
    const linkData = await api(`/admin/token-groups/${groupId}/model-accounts`);
    const ids = (linkData?.model_account_ids || linkData?.upstream_ids) || [];
    syncTgLinksSelect(ids);
    const tb = document.getElementById("tgLinksTableBody");
    tb.innerHTML = "";
    ids.forEach((uid) => {
      const u = upstreamById.get(uid);
      const tr = document.createElement("tr");
      if (!u) {
        tr.innerHTML = `<td>${uid}</td><td colspan="3" class="text-muted">—</td>
          <td><button type="button" class="ghost btn-sm text-danger" data-rm="${uid}">${escapeHtml(t("action.delete"))}</button></td>`;
      } else {
        tr.innerHTML = `
          <td>${u.id}</td>
          <td>${escapeHtml(u.model_name)}</td>
          <td>${escapeHtml(u.name || "—")}</td>
          <td class="text-break small">${escapeHtml(u.endpoint)}</td>
          <td><button type="button" class="ghost btn-sm text-danger" data-rm="${uid}">${escapeHtml(t("action.delete"))}</button></td>`;
      }
      tb.appendChild(tr);
    });
    if (!ids.length) {
      tb.innerHTML = `<tr><td colspan="5" class="text-muted small">${escapeHtml(t("admin.tokenGroups.noLinks"))}</td></tr>`;
    }
    tb.querySelectorAll("[data-rm]").forEach((b) => {
      b.addEventListener("click", async () => {
        const uid = Number(b.getAttribute("data-rm"));
        try {
          await api(`/admin/token-groups/${groupId}/model-accounts/${uid}`, { method: "DELETE" });
          showToast(t("admin.savedOk"), true);
          await openTgLinksModal(groupId);
          await loadTokenGroups();
        } catch (e) {
          document.getElementById("tgLinksErr").textContent = e.message;
          document.getElementById("tgLinksErr").classList.remove("d-none");
        }
      });
    });
    tgLinksModal?.show();
  }

  async function addTgUpstream() {
    const gid = Number(document.getElementById("tgLinksGroupId").value);
    const sel = document.getElementById("tgLinksSelect");
    const model_account_id = Number(sel?.value);
    if (!model_account_id) return;
    try {
      document.getElementById("tgLinksErr")?.classList.add("d-none");
      await api(`/admin/token-groups/${gid}/model-accounts`, {
        method: "POST",
        body: JSON.stringify({ model_account_id }),
      });
      showToast(t("admin.savedOk"), true);
      await openTgLinksModal(gid);
      await loadTokenGroups();
    } catch (e) {
      const el = document.getElementById("tgLinksErr");
      el.textContent = e.message;
      el.classList.remove("d-none");
    }
  }

  async function loadRateLimits() {
    const tbody = document.getElementById("rateLimitTableBody");
    if (!tbody) return;
    const rules = await api("/admin/rate-limits");
    if (!rules) return;
    tbody.innerHTML = "";
    if (!rules.length) {
      tbody.innerHTML = `<tr><td colspan="8" class="text-muted text-center py-3">${escapeHtml(t("admin.rateLimits.empty"))}</td></tr>`;
      return;
    }
    rules.forEach((r) => {
      const tr = document.createElement("tr");
      tr.innerHTML = `
        <td>${escapeHtml(r.name)}</td>
        <td>${escapeHtml(r.scope)}</td>
        <td class="small">${escapeHtml(r.scope_value || "—")}</td>
        <td>${r.max_rpm}</td>
        <td>${r.max_tpm}</td>
        <td>${r.max_concurrent}</td>
        <td><span class="pill ${r.is_active ? "ok" : "off"}">${r.is_active ? t("status.enabled") : t("status.disabled")}</span></td>
        <td>
          <div class="d-flex flex-wrap gap-1">
            <button type="button" class="ghost btn-sm" data-rl-edit="${r.id}">${escapeHtml(t("admin.edit"))}</button>
            <button type="button" class="ghost btn-sm text-danger" data-rl-del="${r.id}">${escapeHtml(t("action.delete"))}</button>
          </div>
        </td>`;
      tbody.appendChild(tr);
    });
    tbody.querySelectorAll("[data-rl-edit]").forEach((b) => {
      b.addEventListener("click", async () => {
        const id = Number(b.getAttribute("data-rl-edit"));
        const rules2 = await api("/admin/rate-limits");
        const r = rules2?.find((x) => x.id === id);
        if (r) openRateLimitModal(r);
      });
    });
    tbody.querySelectorAll("[data-rl-del]").forEach((b) => {
      b.addEventListener("click", async () => {
        const id = Number(b.getAttribute("data-rl-del"));
        if (!window.confirm(t("admin.rateLimits.deleteConfirm"))) return;
        try {
          await api(`/admin/rate-limits/${id}`, { method: "DELETE" });
          showToast(t("admin.deletedOk"), true);
          await loadRateLimits();
        } catch (e) {
          showToast(e.message, false);
        }
      });
    });
  }

  function openRateLimitModal(r) {
    document.getElementById("rateLimitModalErr")?.classList.add("d-none");
    document.getElementById("rl_id").value = r ? String(r.id) : "";
    document.getElementById("rl_name").value = r?.name || "";
    document.getElementById("rl_scope").value = r?.scope || "global";
    document.getElementById("rl_scope_value").value = r?.scope_value || "";
    document.getElementById("rl_max_rpm").value = r ? String(r.max_rpm) : "0";
    document.getElementById("rl_max_tpm").value = r ? String(r.max_tpm) : "0";
    document.getElementById("rl_max_concurrent").value = r ? String(r.max_concurrent) : "0";
    document.getElementById("rl_is_active").checked = r ? !!r.is_active : true;
    document.getElementById("rateLimitModalTitle").textContent = r ? t("admin.rateLimits.editTitle") : t("admin.rateLimits.add");
    rateLimitModal?.show();
  }

  async function saveRateLimit() {
    const errEl = document.getElementById("rateLimitModalErr");
    errEl?.classList.add("d-none");
    const id = document.getElementById("rl_id").value;
    const body = {
      name: document.getElementById("rl_name").value.trim(),
      scope: document.getElementById("rl_scope").value,
      scope_value: document.getElementById("rl_scope_value").value.trim(),
      max_rpm: Number(document.getElementById("rl_max_rpm").value) || 0,
      max_tpm: Number(document.getElementById("rl_max_tpm").value) || 0,
      max_concurrent: Number(document.getElementById("rl_max_concurrent").value) || 0,
      is_active: document.getElementById("rl_is_active").checked,
    };
    try {
      if (id) {
        await api(`/admin/rate-limits/${id}`, { method: "PUT", body: JSON.stringify(body) });
      } else {
        await api("/admin/rate-limits", { method: "POST", body: JSON.stringify(body) });
      }
      rateLimitModal?.hide();
      showToast(t("admin.savedOk"), true);
      await loadRateLimits();
    } catch (e) {
      errEl.textContent = e.message;
      errEl.classList.remove("d-none");
    }
  }

  async function doCharge() {
    const err = document.getElementById("chargeErr");
    const ok = document.getElementById("chargeResult");
    err.classList.add("d-none");
    ok.classList.add("d-none");
    const userId = Number(document.getElementById("chargeUserId").value);
    const amount = Number(document.getElementById("chargeAmount").value);
    const detail = document.getElementById("chargeDetail").value.trim() || t("admin.charge.detailPh");
    if (!userId || userId < 1) {
      err.textContent = t("admin.charge.badUser");
      err.classList.remove("d-none");
      return;
    }
    if (!Number.isFinite(amount) || amount < 1) {
      err.textContent = t("common.needPositive");
      err.classList.remove("d-none");
      return;
    }
    try {
      const data = await api(`/admin/users/${userId}/charge`, {
        method: "POST",
        body: JSON.stringify({ amount, detail }),
      });
      ok.textContent = `${t("admin.topupOk")} ${fmtMicro(data.balance)}（${fmtMicroUsd(data.balance)}）`;
      ok.classList.remove("d-none");
    } catch (e) {
      err.textContent = e.message;
      err.classList.remove("d-none");
    }
  }

  async function loadRedeemList() {
    const tbody = document.getElementById("redeemTableBody");
    if (!tbody) return;
    const data = await api(`/admin/redeem-codes?page=${redeemPage}&page_size=${redeemPageSize}`);
    if (!data) return;
    redeemTotal = Number(data.total) || 0;
    const list = data.list || [];
    tbody.innerHTML = "";
    const info = document.getElementById("redeemPageInfo");
    if (info) {
      info.textContent = t("admin.redeem.pageInfo")
        .replace("{page}", String(data.page || redeemPage))
        .replace("{total}", String(redeemTotal));
    }
    document.getElementById("redeemPrev").disabled = redeemPage <= 1;
    const maxPage = Math.max(1, Math.ceil(redeemTotal / redeemPageSize));
    document.getElementById("redeemNext").disabled = redeemPage >= maxPage;

    if (!list.length) {
      tbody.innerHTML = `<tr><td colspan="6" class="text-muted text-center py-3">${escapeHtml(t("admin.redeem.empty"))}</td></tr>`;
      return;
    }
    list.forEach((row) => {
      const used = row.status === 1;
      const tr = document.createElement("tr");
      tr.innerHTML = `
        <td><code>${escapeHtml(row.code)}</code></td>
        <td>${escapeHtml(fmtMicro(row.amount))}</td>
        <td><span class="pill ${used ? "off" : "ok"}">${used ? t("status.used") : t("status.unused")}</span></td>
        <td>${row.used_by != null ? escapeHtml(String(row.used_by)) : "—"}</td>
        <td class="small">${row.used_at ? escapeHtml(fmtTime(row.used_at)) : "—"}</td>
        <td class="small">${escapeHtml(fmtTime(row.created_at))}</td>`;
      tbody.appendChild(tr);
    });
  }

  async function loadAdminBalanceLogs() {
    const tbody = document.getElementById("adminBalanceLogTableBody");
    if (!tbody) return;
    const reasonType = document.getElementById("balanceLogFilterType")?.value || "";
    let url = `/admin/balance-logs?page=${adminBlPage}&page_size=${adminBlPageSize}`;
    if (reasonType) url += `&reason_type=${encodeURIComponent(reasonType)}`;
    const data = await api(url);
    if (!data) return;
    adminBlTotal = Number(data.total) || 0;
    const list = data.list || [];
    tbody.innerHTML = "";
    if (!list.length) {
      tbody.innerHTML = `<tr><td colspan="7" class="text-muted text-center py-3">${escapeHtml(t("admin.balanceLogs.empty"))}</td></tr>`;
      updateAdminBlPagination();
      return;
    }
    list.forEach((r) => {
      const tr = document.createElement("tr");
      tr.innerHTML = `
        <td>${r.id}</td>
        <td>${escapeHtml(r.reason_type || "—")}</td>
        <td>${fmtMicroUsd(r.amount)}</td>
        <td>${fmtMicroUsd(r.balance_before)}</td>
        <td>${fmtMicroUsd(r.balance_after)}</td>
        <td class="small">${escapeHtml(r.reason_detail || "—")}</td>
        <td class="small">${fmtTime(r.created_at)}</td>`;
      tbody.appendChild(tr);
    });
    updateAdminBlPagination();
  }

  function updateAdminBlPagination() {
    const info = document.getElementById("adminBalanceLogPageInfo");
    const maxPage = Math.max(1, Math.ceil(adminBlTotal / adminBlPageSize));
    if (info) info.textContent = t("admin.pager.pageOfTotal")
      .replace("{page}", String(adminBlPage))
      .replace("{max}", String(maxPage))
      .replace("{total}", String(adminBlTotal));
    const prev = document.getElementById("adminBalanceLogPrev");
    const next = document.getElementById("adminBalanceLogNext");
    if (prev) prev.disabled = adminBlPage <= 1;
    if (next) next.disabled = adminBlPage >= maxPage;
  }

  function fmtTime(val) {
    if (!val) return "—";
    return new Date(val).toLocaleString();
  }

  async function batchRedeem() {
    const out = document.getElementById("batchResult");
    out.classList.add("d-none");
    const count = Number(document.getElementById("batchCount").value);
    const amount = Number(document.getElementById("batchAmount").value);
    if (!Number.isFinite(count) || count < 1) return;
    if (!Number.isFinite(amount) || amount < 1) {
      showToast(t("common.needPositive"), false);
      return;
    }
    try {
      const data = await api("/admin/redeem-codes/batch", {
        method: "POST",
        body: JSON.stringify({ count, amount }),
      });
      const codes = data?.codes || [];
      out.textContent = `${t("admin.createOk")} ${codes.slice(0, 5).join(", ")}${codes.length > 5 ? "…" : ""}`;
      out.classList.remove("d-none");
      redeemPage = 1;
      await loadRedeemList();
    } catch (e) {
      showToast(e.message, false);
    }
  }

  async function loadUsers() {
    const tbody = document.getElementById("userTableBody");
    if (!tbody) return;
    const email = document.getElementById("userSearchEmail")?.value?.trim() || "";
    let url = `/admin/users?page=${userPage}&page_size=${userPageSize}`;
    if (email) url += `&email=${encodeURIComponent(email)}`;
    const data = await api(url);
    if (!data) return;
    userTotal = Number(data.total) || 0;
    const list = data.list || [];
    tbody.innerHTML = "";
    if (!list.length) {
      tbody.innerHTML = `<tr><td colspan="6" class="text-muted text-center py-3">${escapeHtml(t("admin.users.empty"))}</td></tr>`;
      updateUserPagination();
      return;
    }
    list.forEach((u) => {
      const tr = document.createElement("tr");
      tr.innerHTML = `
        <td>${u.id}</td>
        <td>${escapeHtml(u.email)}</td>
        <td>${escapeHtml(u.role)}</td>
        <td><span class="pill ${u.is_active ? "ok" : "off"}">${u.is_active ? t("status.enabled") : t("status.disabled")}</span></td>
        <td class="small">${fmtTime(u.created_at)}</td>
        <td><button type="button" class="ghost btn-sm" data-charge-user="${u.id}">${escapeHtml(t("admin.charge.title"))}</button></td>`;
      tbody.appendChild(tr);
    });
    tbody.querySelectorAll("[data-charge-user]").forEach((btn) => {
      btn.addEventListener("click", () => {
        const uid = btn.getAttribute("data-charge-user");
        document.getElementById("chargeUserId").value = uid;
        document.getElementById("chargeUserId").scrollIntoView({ behavior: "smooth" });
      });
    });
    updateUserPagination();
  }

  function updateUserPagination() {
    const info = document.getElementById("userPageInfo");
    const maxPage = Math.max(1, Math.ceil(userTotal / userPageSize));
    if (info) info.textContent = `${userPage} / ${maxPage}（${t("admin.redeem.pageInfo").replace("{page}", String(userPage)).replace("{total}", String(userTotal))}）`;
    const prev = document.getElementById("userPrev");
    const next = document.getElementById("userNext");
    if (prev) prev.disabled = userPage <= 1;
    if (next) next.disabled = userPage >= maxPage;
  }

  async function loadUsageLogs() {
    const tbody = document.getElementById("usageLogTableBody");
    if (!tbody) return;
    const model = document.getElementById("usageFilterModel")?.value?.trim() || "";
    const status = document.getElementById("usageFilterStatus")?.value || "";
    let url = `/admin/usage-logs?page=${usagePage}&page_size=${usagePageSize}`;
    if (model) url += `&model=${encodeURIComponent(model)}`;
    if (status) url += `&status=${encodeURIComponent(status)}`;
    const data = await api(url);
    if (!data) return;
    usageTotal = Number(data.total) || 0;
    const list = data.list || [];
    tbody.innerHTML = "";
    if (!list.length) {
      tbody.innerHTML = `<tr><td colspan="8" class="text-muted text-center py-3">${escapeHtml(t("admin.usageLogs.empty"))}</td></tr>`;
      updateUsagePagination();
      return;
    }
    list.forEach((r) => {
      const tr = document.createElement("tr");
      tr.innerHTML = `
        <td class="small text-break">${escapeHtml(r.request_id)}</td>
        <td>${r.user_id}</td>
        <td>${escapeHtml(r.model)}</td>
        <td><span class="pill ${r.status === "success" ? "ok" : "off"}">${escapeHtml(r.status)}</span></td>
        <td>${r.total_tokens}</td>
        <td>${fmtMicroUsd(r.cost_micro_usd)}</td>
        <td>${r.latency_ms}ms</td>
        <td class="small">${fmtTime(r.created_at)}</td>`;
      tbody.appendChild(tr);
    });
    updateUsagePagination();
  }

  function updateUsagePagination() {
    const maxPage = Math.max(1, Math.ceil(usageTotal / usagePageSize));
    const info = document.getElementById("usagePageInfo");
    if (info) info.textContent = t("admin.pager.pageOfTotalItems")
      .replace("{page}", String(usagePage))
      .replace("{max}", String(maxPage))
      .replace("{total}", String(usageTotal));
    const prev = document.getElementById("usagePrev");
    const next = document.getElementById("usageNext");
    if (prev) prev.disabled = usagePage <= 1;
    if (next) next.disabled = usagePage >= maxPage;
  }

  async function refreshAll() {
    if (isSuperAdmin()) {
      await rebuildUpstreamCatalog();
      await loadModels();
      await loadTokenGroups();
      await loadRateLimits();
      await loadUsageLogs();
    }
    await loadUsers();
    await loadRedeemList();
    await loadAdminBalanceLogs();
    window.I18N?.applyI18n?.();
  }

  async function gate() {
    const loading = document.getElementById("adminLoading");
    const denied = document.getElementById("adminDenied");
    const app = document.getElementById("adminApp");
    try {
      const raw = await api("/account/profile");
      if (!raw) return;
      const profile = raw.profile || raw;
      if (!isAdminRole(profile.role)) {
        loading.classList.add("d-none");
        denied.classList.remove("d-none");
        setTimeout(() => {
          const base = userConsoleBase();
          window.location.href = base ? `${base}/app.html` : "/app.html";
        }, 2000);
        return;
      }
      currentRole = profile.role;
      applyRoleTabs();
      loading.classList.add("d-none");
      app.classList.remove("d-none");
      if (isSuperAdmin()) {
        try {
          await ensureProviderCatalogLoaded();
          fillModelProviderFilter();
        } catch (_) {}
      }
      await refreshAll();
    } catch (e) {
      loading.classList.add("d-none");
      denied.classList.remove("d-none");
      const p = denied.querySelector(".msg");
      if (p) p.textContent = e.message || t("admin.deniedMsg");
    }
  }

  document.addEventListener("DOMContentLoaded", () => {
    modelModal = new bootstrap.Modal(document.getElementById("modelModal"));
    bulkPricingModal = new bootstrap.Modal(document.getElementById("bulkPricingModal"));
    upstreamModal = new bootstrap.Modal(document.getElementById("upstreamModal"));
    tokenGroupModal = new bootstrap.Modal(document.getElementById("tokenGroupModal"));
    tgLinksModal = new bootstrap.Modal(document.getElementById("tgLinksModal"));
    rateLimitModal = new bootstrap.Modal(document.getElementById("rateLimitModal"));

    installModelTableRowDelegation();

    document.getElementById("adminLogoutBtn")?.addEventListener("click", () => {
      localStorage.removeItem("accessToken");
      window.location.href = "/admin-login.html";
    });

    const ubase = userConsoleBase();
    if (ubase) {
      document.querySelectorAll('a[href="/app.html"]').forEach((a) => {
        a.href = `${ubase}/app.html`;
      });
    }

    document.getElementById("btnModelAdd")?.addEventListener("click", () => openModelModal(null));
    document.getElementById("btnModelBulkPrice")?.addEventListener("click", () => openBulkPricingModal());
    document.getElementById("btnModelConnect")?.addEventListener("click", () => testAllEnabledModels());
    document.getElementById("bulkPricingSave")?.addEventListener("click", () => saveBulkPricing());
    document.getElementById("modelSelectAll")?.addEventListener("change", function () {
      const on = this.checked;
      document.querySelectorAll("#modelTableBody > tr:not(.upstream-detail-row) .model-row-cb").forEach((cb) => {
        cb.checked = on;
      });
    });
    document.getElementById("btnModelFilter")?.addEventListener("click", () => loadModels());
    document.getElementById("btnModelFilterReset")?.addEventListener("click", () => {
      const p = document.getElementById("modelFilterProvider");
      const q = document.getElementById("modelFilterQ");
      if (p) p.value = "";
      if (q) q.value = "";
      loadModels();
    });
    document.getElementById("modelModalSave")?.addEventListener("click", saveModel);

    document.getElementById("providerCatalog")?.addEventListener("change", onProviderCatalogChange);
    document.getElementById("providerAuthType")?.addEventListener("change", onAuthTypeChange);
    document.getElementById("btnOAuthAuthorize")?.addEventListener("click", async function () {
      const providerSel = document.getElementById("providerCatalog");
      const statusEl = document.getElementById("oauthStatus");
      const fallbackLink = document.getElementById("oauthFallbackLink");
      const provider_id = providerSel?.value;
      if (!provider_id) return;

      if (statusEl) statusEl.textContent = t("admin.models.oauthStarting");
      fallbackLink?.classList.add("d-none");

      try {
        const data = await api("/admin/oauth/start", {
          method: "POST",
          body: JSON.stringify({ provider_id }),
        });
        if (!data || !data.auth_url) {
          if (statusEl) statusEl.textContent = t("admin.models.oauthStartFailed");
          return;
        }

        const authUrl = data.auth_url;
        const popup = window.open(authUrl, "oauth_popup", "width=600,height=700");
        if (!popup || popup.closed) {
          if (fallbackLink) {
            fallbackLink.href = authUrl;
            fallbackLink.classList.remove("d-none");
          }
          if (statusEl) statusEl.textContent = t("admin.models.oauthPopupBlocked");
        } else {
          if (statusEl) statusEl.textContent = t("admin.models.oauthWaiting");
        }
      } catch (e) {
        if (statusEl) statusEl.textContent = e.message || t("common.requestFailed");
      }
    });
    window.addEventListener("message", function (event) {
      if (!event.data || event.data.type !== "oauth_callback_success") return;

      const { access_token, refresh_token, expires_in } = event.data;

      const credInput = document.getElementById("discoverCredential");
      if (credInput && access_token) {
        credInput.value = access_token;
      }

      const statusEl = document.getElementById("oauthStatus");
      if (statusEl) {
        statusEl.textContent = t("admin.models.oauthSuccess");
        statusEl.classList.add("text-success");
        statusEl.classList.remove("text-muted");
      }

      const credWrap = document.getElementById("discoverCredentialWrap");
      if (credWrap) credWrap.classList.remove("d-none");

      if (credInput) {
        credInput.dataset.oauthRefreshToken = refresh_token || "";
        credInput.dataset.oauthExpiresIn = String(expires_in || 0);
        credInput.dataset.oauthAuthType = "oauth_authorization_code";
      }

      showToast(t("admin.models.oauthSuccess"), true);
    });
    document.getElementById("btnDiscoverModels")?.addEventListener("click", () => runDiscoverModels());
    document.getElementById("discoverModelPick")?.addEventListener("change", () => syncDiscoverPickToModelName());
    document.getElementById("btnDiscoverBatchAdd")?.addEventListener("click", () => discoverBatchAdd());
    document.getElementById("btnPgSend")?.addEventListener("click", () => runPlayground());
    document.getElementById("pgModelId")?.addEventListener("change", () => syncPlaygroundForm());

    document.getElementById("upstreamModalSave")?.addEventListener("click", saveUpstream);

    document.getElementById("btnTokenGroupAdd")?.addEventListener("click", () => {
      document.getElementById("tg_name").value = "";
      document.getElementById("tg_description").value = "";
      document.getElementById("tg_is_active").checked = true;
      document.getElementById("tokenGroupModalErr")?.classList.add("d-none");
      tokenGroupModal?.show();
    });
    document.getElementById("tokenGroupModalSave")?.addEventListener("click", async () => {
      const errEl = document.getElementById("tokenGroupModalErr");
      errEl?.classList.add("d-none");
      const body = {
        name: document.getElementById("tg_name").value.trim(),
        description: document.getElementById("tg_description").value.trim(),
        is_active: document.getElementById("tg_is_active").checked,
      };
      if (!body.name) {
        errEl.textContent = t("admin.tokenGroups.needName");
        errEl.classList.remove("d-none");
        return;
      }
      try {
        await api("/admin/token-groups", { method: "POST", body: JSON.stringify(body) });
        tokenGroupModal?.hide();
        showToast(t("admin.savedOk"), true);
        await loadTokenGroups();
      } catch (e) {
        errEl.textContent = e.message;
        errEl.classList.remove("d-none");
      }
    });

    document.getElementById("tgLinksAddBtn")?.addEventListener("click", addTgUpstream);

    document.getElementById("btnRateLimitAdd")?.addEventListener("click", () => openRateLimitModal(null));
    document.getElementById("rateLimitModalSave")?.addEventListener("click", saveRateLimit);

    document.getElementById("btnUserSearch")?.addEventListener("click", async () => {
      userPage = 1;
      await loadUsers();
    });
    document.getElementById("userPrev")?.addEventListener("click", async () => {
      if (userPage > 1) { userPage--; await loadUsers(); }
    });
    document.getElementById("userNext")?.addEventListener("click", async () => {
      userPage++;
      await loadUsers();
    });

    document.getElementById("btnCharge")?.addEventListener("click", doCharge);
    document.getElementById("btnBatchRedeem")?.addEventListener("click", batchRedeem);
    document.getElementById("redeemPrev")?.addEventListener("click", async () => {
      if (redeemPage > 1) {
        redeemPage -= 1;
        await loadRedeemList();
      }
    });
    document.getElementById("redeemNext")?.addEventListener("click", async () => {
      redeemPage += 1;
      await loadRedeemList();
    });

    document.getElementById("btnUsageSearch")?.addEventListener("click", async () => {
      usagePage = 1;
      await loadUsageLogs();
    });
    document.getElementById("usagePrev")?.addEventListener("click", async () => {
      if (usagePage > 1) { usagePage--; await loadUsageLogs(); }
    });
    document.getElementById("usageNext")?.addEventListener("click", async () => {
      usagePage++;
      await loadUsageLogs();
    });

    document.getElementById("btnBalanceLogSearch")?.addEventListener("click", async () => {
      adminBlPage = 1;
      await loadAdminBalanceLogs();
    });
    document.getElementById("adminBalanceLogPrev")?.addEventListener("click", async () => {
      if (adminBlPage > 1) { adminBlPage--; await loadAdminBalanceLogs(); }
    });
    document.getElementById("adminBalanceLogNext")?.addEventListener("click", async () => {
      adminBlPage++;
      await loadAdminBalanceLogs();
    });

    // ===================== 流量监控模块 =====================
    let monitorCurrentModelID = null;
    let monitorCurrentModelName = null;
    let monitorRefreshTimer = null;

    function formatNum(n) {
      if (n == null) return '—';
      if (n >= 1e6) return (n / 1e6).toFixed(2) + 'M';
      if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
      return String(n);
    }

    function pct(n) { return n == null ? '—' : (n * 100).toFixed(1) + '%'; }
    function ms(n)  { return n == null ? '—' : Math.round(n) + ' ms'; }
    function usd(n) { return n == null ? '—' : '$' + n.toFixed(4); }

    function statusBadge(status) {
      if (status === 'online') return `<span class="badge bg-success">${escapeHtml(t("admin.account.statusOnline"))}</span>`;
      if (status === 'offline') return `<span class="badge bg-secondary">${escapeHtml(t("admin.account.statusOffline"))}</span>`;
      return `<span class="badge bg-warning text-dark">${escapeHtml(status)}</span>`;
    }

    async function loadMonitorOverview() {
      const hours = document.getElementById('monitorHours').value;
      try {
        const data = await api(`/admin/monitor/overview?hours=${hours}`);
        renderOverviewGrid(data.models || []);
        document.getElementById('monitorLastRefresh').textContent =
          t("admin.monitor.updatedAt").replace("{time}", new Date().toLocaleTimeString());
      } catch(e) { showToast(t("admin.monitor.loadFail") + ': ' + e.message, false); }
    }

    function renderOverviewGrid(models) {
      const grid = document.getElementById('monitorOverviewGrid');
      if (!models.length) {
        grid.innerHTML = `<div class="text-muted">${escapeHtml(t("admin.monitor.empty"))}</div>`;
        return;
      }
      grid.innerHTML = models.map(m => {
        const errRate = m.error_rate || 0;
        const alertClass = errRate > 0.05 || m.avg_latency_ms > 3000 ? 'border-danger' : '';
        const todayTpl = t("admin.monitor.today")
          .replace("{req}", formatNum(m.today_requests))
          .replace("{tok}", formatNum(m.today_tokens));
        return `<div class="col-sm-6 col-xl-4">
          <div class="card h-100 ${alertClass}" style="cursor:pointer" onclick="window._monitorDrillModel(${m.model_id},'${m.model_name}')">
            <div class="card-body p-3">
              <div class="d-flex justify-content-between align-items-start mb-2">
                <span class="fw-bold">${escapeHtml(m.model_name)}</span>
                ${errRate > 0.05 ? `<span class="badge bg-danger">${escapeHtml(t("admin.monitor.highError"))}</span>` : ''}
              </div>
              <div class="row g-1 text-center small">
                <div class="col-4"><div class="text-muted">${escapeHtml(t("admin.monitor.colRequests"))}</div><strong>${formatNum(m.total_requests)}</strong></div>
                <div class="col-4"><div class="text-muted">${escapeHtml(t("admin.monitor.colErrorRate"))}</div><strong class="${errRate>0.05?'text-danger':''}">${pct(errRate)}</strong></div>
                <div class="col-4"><div class="text-muted">${escapeHtml(t("admin.monitor.colAvgLatency"))}</div><strong>${ms(m.avg_latency_ms)}</strong></div>
                <div class="col-4"><div class="text-muted">P95</div><strong>${ms(m.p95_latency_ms)}</strong></div>
                <div class="col-4"><div class="text-muted">Token</div><strong>${formatNum(m.total_tokens)}</strong></div>
                <div class="col-4"><div class="text-muted">${escapeHtml(t("admin.monitor.colCost"))}</div><strong>${usd(m.total_cost_usd)}</strong></div>
              </div>
              ${m.today_requests ? `<div class="mt-2 small text-muted">${escapeHtml(todayTpl)}</div>` : ''}
            </div>
          </div>
        </div>`;
      }).join('');
    }

    window._editModelAccount = async (modelID, accountID) => {
      try {
        const ups = await api(`/admin/models/${modelID}/model-accounts`);
        const u = (ups || []).find(x => x.id === accountID);
        if (u) openUpstreamModal({ modelId: modelID, edit: u });
      } catch (e) { showToast(e.message, false); }
    };

    window._toggleModelAccount = async (accountID, makeActive) => {
      const isActive = (makeActive === true || makeActive === 'true');
      try {
        await api(`/admin/model-accounts/${accountID}/toggle`, { method: 'PATCH', body: JSON.stringify({ is_active: isActive }) });
        showToast(t("admin.savedOk"), true);
        await loadModels();
        await rebuildUpstreamCatalog();
      } catch (e) { showToast(e.message, false); }
    };

    window._deleteModelAccount = async (modelID, accountID) => {
      if (!window.confirm(t("admin.upstreams.deleteConfirm"))) return;
      try {
        await api(`/admin/model-accounts/${accountID}`, { method: 'DELETE' });
        showToast(t("admin.deletedOk"), true);
        await loadModels();
        await rebuildUpstreamCatalog();
        await loadTokenGroups();
      } catch (e) { showToast(e.message, false); }
    };

    window._monitorDrillModel = async (modelID, modelName) => {
      monitorCurrentModelID = modelID;
      monitorCurrentModelName = modelName;
      document.getElementById('monitorModelTitle').textContent = modelName;
      document.getElementById('monitor-overview-view').classList.add('d-none');
      document.getElementById('monitor-model-view').classList.remove('d-none');
      await loadMonitorModelDetail();
    };

    async function loadMonitorModelDetail() {
      if (!monitorCurrentModelID) return;
      const hours = document.getElementById('monitorDetailHours').value;
      const gran  = document.getElementById('monitorDetailGranularity').value;
      try {
        const data = await api(`/admin/monitor/models/${monitorCurrentModelID}?hours=${hours}&granularity=${gran}`);
        renderTrendChart(data.time_series || []);
        renderAccountTable(data.accounts || []);
      } catch(e) { showToast(t("admin.monitor.detailFail") + ': ' + e.message, false); }
    }

    function renderTrendChart(series) {
      const canvas = document.getElementById('monitorTrendChart');
      const ctx = canvas.getContext('2d');
      canvas.width = canvas.parentElement.offsetWidth || 600;
      const w = canvas.width, h = canvas.height;
      ctx.clearRect(0, 0, w, h);
      if (!series.length) {
        ctx.fillStyle = '#888'; ctx.font = '12px sans-serif';
        ctx.fillText(t("admin.monitor.noTrend"), w / 2 - 40, h / 2);
        return;
      }
      const max = Math.max(...series.map(p => p.total_requests), 1);
      const step = w / series.length;
      // 折线
      ctx.strokeStyle = '#2563eb'; ctx.lineWidth = 2;
      ctx.beginPath();
      series.forEach((p, i) => {
        const x = i * step + step / 2;
        const y = h - (p.total_requests / max) * (h - 20) - 5;
        i === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y);
      });
      ctx.stroke();
      // 标签
      ctx.fillStyle = '#666'; ctx.font = '10px sans-serif'; ctx.textAlign = 'center';
      if (series.length <= 24) {
        series.forEach((p, i) => {
          const x = i * step + step / 2;
          ctx.fillText(p.bucket.slice(-5), x, h - 2);
        });
      }
    }

    function renderAccountTable(accounts) {
      const tbody = document.getElementById('monitorAccountTbody');
      if (!accounts.length) {
        tbody.innerHTML = `<tr><td colspan="8" class="text-center text-muted py-3">${escapeHtml(t("admin.monitor.noAccounts"))}</td></tr>`;
        return;
      }
      tbody.innerHTML = accounts.map(a => `
        <tr>
          <td>${escapeHtml(a.account_name || ('#' + a.account_id))}</td>
          <td>${escapeHtml(a.provider || '—')}</td>
          <td>${statusBadge(a.status || 'unknown')}</td>
          <td>${formatNum(a.total_requests)}</td>
          <td class="${(a.error_rate||0)>0.05?'text-danger':''}">${pct(a.error_rate)}</td>
          <td>${ms(a.avg_latency_ms)}</td>
          <td>${formatNum(a.total_tokens)}</td>
          <td>
            <button class="btn btn-outline-secondary btn-xs py-0 px-1" style="font-size:0.75rem"
              onclick="window._monitorPatchWeight(${a.account_id})">${escapeHtml(t("admin.monitor.tuneWeight"))}</button>
            <button class="btn btn-outline-${a.status==='online'?'danger':'success'} btn-xs py-0 px-1 ms-1" style="font-size:0.75rem"
              onclick="window._monitorToggleAccount(${a.account_id},'${a.status==='online'?'offline':'online'}')">
              ${escapeHtml(a.status==='online'?t("admin.account.offline"):t("admin.account.online"))}
            </button>
          </td>
        </tr>`).join('');
    }

    window._monitorPatchWeight = async (accountID) => {
      // accountID 即 model_accounts.id，直接从目录中按 id 查找
      const u = upstreamCatalog.find(x => x.id === accountID);
      if (!u) { showToast(t("admin.monitor.noMatchUpstream"), false); return; }
      const promptMsg = t("admin.monitor.promptWeight")
        .replace("{name}", String(u.name || u.id))
        .replace("{weight}", String(u.weight));
      const w = prompt(promptMsg, u.weight);
      if (w === null) return;
      const weight = parseInt(w, 10);
      if (isNaN(weight) || weight < 0 || weight > 100) { showToast(t("admin.monitor.needWeightRange"), false); return; }
      try {
        await api(`/admin/model-accounts/${u.id}/weight`, { method: 'PATCH', body: JSON.stringify({ weight }) });
        showToast(t("admin.monitor.weightUpdated"), true);
        await loadMonitorModelDetail();
      } catch(e) { showToast(e.message, false); }
    };

    window._monitorToggleAccount = async (accountID, newStatus) => {
      try {
        const isActive = newStatus === 'online' || newStatus === true;
        await api(`/admin/model-accounts/${accountID}/toggle`, { method: 'PATCH', body: JSON.stringify({ is_active: isActive }) });
        showToast(t("admin.monitor.accountStatusUpdated"), true);
        await loadMonitorModelDetail();
      } catch(e) { showToast(e.message, false); }
    };

    document.getElementById('monitorBackBtn').addEventListener('click', () => {
      document.getElementById('monitor-model-view').classList.add('d-none');
      document.getElementById('monitor-overview-view').classList.remove('d-none');
      monitorCurrentModelID = null;
    });

    document.getElementById('monitorRefreshBtn').addEventListener('click', loadMonitorOverview);
    document.getElementById('monitorDetailRefreshBtn').addEventListener('click', loadMonitorModelDetail);
    document.getElementById('monitorHours').addEventListener('change', loadMonitorOverview);
    document.getElementById('monitorDetailHours').addEventListener('change', loadMonitorModelDetail);
    document.getElementById('monitorDetailGranularity').addEventListener('change', loadMonitorModelDetail);

    // 切换到监控 Tab 时自动加载，并启动 30s 自动刷新
    document.getElementById('tab-monitor').addEventListener('shown.bs.tab', () => {
      // 修正 display:none 问题
      document.getElementById('pane-monitor').style.display = '';
      loadMonitorOverview();
      monitorRefreshTimer = setInterval(() => {
        if (monitorCurrentModelID) loadMonitorModelDetail();
        else loadMonitorOverview();
      }, 30000);
    });
    document.getElementById('tab-monitor').addEventListener('hidden.bs.tab', () => {
      clearInterval(monitorRefreshTimer);
    });

    gate();
  });
})();
