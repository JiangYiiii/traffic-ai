(function () {
  const t = (k) => (window.I18N ? window.I18N.t(k) : k);
  const accessToken = localStorage.getItem("accessToken");

  function gatewayBase() {
    return `${window.location.protocol}//${window.location.hostname}:8081`;
  }

  const GW = gatewayBase();
  const BASE_V1 = `${GW}/v1`;
  const MESSAGES_URL = `${GW}/v1/messages`;
  const RESPONSES_URL = `${GW}/v1/responses`;

  function setText(id, v) {
    const el = document.getElementById(id);
    if (el) el.textContent = v;
  }

  setText("docsBaseUrl", BASE_V1);
  setText("docsMessagesUrl", MESSAGES_URL);
  setText("docsResponsesUrl", RESPONSES_URL);

  // ---- sidebar nav ----
  const sidebar = document.getElementById("docsSidebar");
  if (sidebar) {
    sidebar.addEventListener("click", (e) => {
      const a = e.target.closest("a[data-section]");
      if (!a) return;
      e.preventDefault();
      const sec = a.dataset.section;
      document.querySelectorAll(".docs-section").forEach((s) => s.classList.remove("active"));
      const target = document.getElementById("sec-" + sec);
      if (target) target.classList.add("active");
      sidebar.querySelectorAll("a").forEach((x) => x.classList.remove("active"));
      a.classList.add("active");
    });
  }

  // ---- auth status + token select ----
  const statusEl = document.getElementById("docsAuthStatus");
  const tokenSelect = document.getElementById("docsTokenSelect");
  let selectedPlainToken = "YOUR_SUB_TOKEN";

  const localTokenMap = (function () {
    try {
      return JSON.parse(localStorage.getItem("plainTokensById") || "{}");
    } catch {
      return {};
    }
  })();

  async function loadTokensForDocs() {
    if (!accessToken) {
      if (statusEl) statusEl.textContent = t("docs.auth.loggedOut");
      tokenSelect.innerHTML = '<option value="">—</option>';
      return;
    }
    try {
      const resp = await fetch("/me/tokens", {
        headers: { "content-type": "application/json", authorization: `Bearer ${accessToken}` },
      });
      const body = await resp.json().catch(() => ({}));
      if (body.code !== 0) throw new Error();
      const rows = body.data || [];
      if (statusEl) statusEl.textContent = t("docs.auth.loggedIn");
      tokenSelect.innerHTML = "";
      rows.forEach((r) => {
        const id = r.id;
        const plain = localTokenMap[id];
        if (!plain) return;
        const opt = document.createElement("option");
        opt.value = plain;
        opt.textContent = `${r.name || r.id} [${r.token_group || r.tokenGroup || "default"}]`;
        tokenSelect.appendChild(opt);
      });
      if (tokenSelect.options.length === 0) {
        tokenSelect.innerHTML = '<option value="">—</option>';
      } else {
        selectedPlainToken = tokenSelect.value;
      }
    } catch {
      if (statusEl) statusEl.textContent = t("docs.auth.loggedOut");
      tokenSelect.innerHTML = '<option value="">—</option>';
    }
  }

  if (tokenSelect) {
    tokenSelect.addEventListener("change", () => {
      selectedPlainToken = tokenSelect.value || "YOUR_SUB_TOKEN";
      renderAllCodeBlocks();
    });
  }

  // ---- model table ----
  async function loadModelsForDocs() {
    const tbody = document.getElementById("docsModelTableBody");
    if (!tbody) return;
    try {
      let rows = [];
      if (accessToken) {
        const resp = await fetch("/me/model-pricing", {
          headers: { "content-type": "application/json", authorization: `Bearer ${accessToken}` },
        });
        const body = await resp.json().catch(() => ({}));
        if (body.code === 0 && body.data) rows = Array.isArray(body.data) ? body.data : [];
      }
      if (!rows.length) {
        const resp2 = await fetch(`${GW}/v1/models`);
        const d2 = await resp2.json().catch(() => ({}));
        if (d2.data) rows = d2.data.map((m) => ({ model: m.id }));
      }
      tbody.innerHTML = "";
      if (!rows.length) {
        tbody.innerHTML = `<tr><td colspan="3">${t("docs.models.empty")}</td></tr>`;
        return;
      }
      rows.forEach((r) => {
        const tr = document.createElement("tr");
        const model = r.model || r.model_name || r.id || "-";
        const inp = r.inputUsdPer1M || (r.input_price != null ? (r.input_price / 1e6).toFixed(4) : "-");
        const out = r.outputUsdPer1M || (r.output_price != null ? (r.output_price / 1e6).toFixed(4) : "-");
        tr.innerHTML = `<td>${esc(model)}</td><td>${esc(String(inp))}</td><td>${esc(String(out))}</td>`;
        tbody.appendChild(tr);
      });
    } catch {
      const tbody2 = document.getElementById("docsModelTableBody");
      if (tbody2) tbody2.innerHTML = `<tr><td colspan="3">${t("docs.models.error")}</td></tr>`;
    }
  }

  function esc(s) {
    return String(s).replace(/[&<>"']/g, (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c] || c
    );
  }

  // ---- code blocks ----
  function tok() {
    return selectedPlainToken || "YOUR_SUB_TOKEN";
  }

  function renderAllCodeBlocks() {
    const T = tok();

    setText("codeOpenai",
`curl ${BASE_V1}/chat/completions \\
  -H "Authorization: Bearer ${T}" \\
  -H "Content-Type: application/json" \\
  -d '{
  "model": "gpt-4o-mini",
  "messages": [{"role":"user","content":"Hello"}],
  "stream": false
}'`);

    setText("codeEmbeddings",
`curl ${BASE_V1}/embeddings \\
  -H "Authorization: Bearer ${T}" \\
  -H "Content-Type: application/json" \\
  -d '{
  "model": "text-embedding-3-large",
  "input": "Hello world"
}'`);

    setText("codeSpeech",
`curl ${BASE_V1}/audio/speech \\
  -H "Authorization: Bearer ${T}" \\
  -H "Content-Type: application/json" \\
  --output speech.mp3 \\
  -d '{
  "model": "tts-1-hd",
  "input": "你好，这是一段语音测试。",
  "voice": "alloy"
}'`);

    setText("codeResponses",
`curl ${BASE_V1}/responses \\
  -H "Authorization: Bearer ${T}" \\
  -H "Content-Type: application/json" \\
  -d '{
  "model": "gpt-4o-mini",
  "input": "Hello, what can you do?",
  "stream": false
}'`);

    setText("codeGemini",
`# 非流式
curl ${GW}/v1beta/models/gemini-2.5-flash:generateContent \\
  -H "Authorization: Bearer ${T}" \\
  -H "Content-Type: application/json" \\
  -d '{
  "contents": [{"role":"user","parts":[{"text":"Hello"}]}]
}'

# 流式
curl "${GW}/v1beta/models/gemini-2.5-flash:streamGenerateContent?alt=sse" \\
  -H "Authorization: Bearer ${T}" \\
  -H "Content-Type: application/json" \\
  -d '{
  "contents": [{"role":"user","parts":[{"text":"Hello"}]}]
}'`);

    setText("codeAnthropic",
`curl ${GW}/v1/messages \\
  -H "x-api-key: ${T}" \\
  -H "anthropic-version: 2023-06-01" \\
  -H "Content-Type: application/json" \\
  -d '{
  "model": "claude-sonnet-4-20250514",
  "max_tokens": 1024,
  "messages": [{"role":"user","content":"Hello"}]
}'`);

    setText("codeOpenclawEnv",
`export TRAFFIC_AI_API_KEY="${T}"
export TRAFFIC_AI_BASE_URL="${BASE_V1}"`);

    setText("codeOpenclawJson",
`{
  "name": "traffic-ai",
  "type": "openai-compatible",
  "baseUrl": "${BASE_V1}",
  "apiKey": "\${TRAFFIC_AI_API_KEY}",
  "models": [
    { "name": "gpt-5.4-mini", "role": "main" },
    { "name": "gpt-5.4",      "role": "coding", "reasoning_effort": "xhigh" },
    { "name": "text-embedding-3-large", "role": "embedding" },
    { "name": "tts-1-hd",     "role": "tts" }
  ]
}`);

    setText("codeCodexRecommend",
`# ~/.codex/config.toml
model = "gpt-5.4"
provider = "openai"

[providers.openai]
api_key = "${T}"
base_url = "${BASE_V1}"

[history]
persistence = "none"
max_conversations = 1

[reasoning]
effort = "xhigh"
summary = "concise"`);

    setText("codeCodexAlt",
`# 快速 / 低成本
# ~/.codex/config.toml
model = "gpt-5.4-mini"
provider = "openai"

[providers.openai]
api_key = "${T}"
base_url = "${BASE_V1}"

[reasoning]
effort = "medium"
summary = "auto"

# -----

# 平衡（默认 high）
model = "gpt-5.4"
provider = "openai"

[providers.openai]
api_key = "${T}"
base_url = "${BASE_V1}"

[reasoning]
effort = "high"
summary = "auto"`);

    setText("codeLangchain",
`from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    base_url="${BASE_V1}",
    api_key="${T}",
    model="gpt-4o-mini",
)
print(llm.invoke("Hello").content)`);
  }

  // ---- copy buttons ----
  document.addEventListener("click", async (e) => {
    const btn = e.target.closest(".copy-btn");
    if (!btn) return;
    const pre = btn.parentElement?.querySelector("pre");
    if (!pre) return;
    try {
      await navigator.clipboard.writeText(pre.textContent);
    } catch {
      const ta = document.createElement("textarea");
      ta.value = pre.textContent;
      ta.style.cssText = "position:fixed;left:-9999px";
      document.body.appendChild(ta);
      ta.select();
      document.execCommand("copy");
      document.body.removeChild(ta);
    }
    const orig = btn.textContent;
    btn.textContent = t("docs.copied");
    setTimeout(() => (btn.textContent = orig), 1200);
  });

  // ---- lang switch ----
  document.querySelectorAll("[data-lang-switch]").forEach((el) => {
    el.addEventListener("change", () => {
      renderAllCodeBlocks();
    });
  });

  // ---- init ----
  renderAllCodeBlocks();
  loadTokensForDocs().then(() => {
    renderAllCodeBlocks();
  });
  loadModelsForDocs();
})();
