(function () {
  const t = (k) => (window.I18N ? window.I18N.t(k) : k);
  const path = window.location.pathname;
  const isLogin = path.endsWith("/login.html") || path === "/login";
  const isAdminLogin = path.endsWith("/admin-login.html");
  const isRegister = path.endsWith("/register.html") || path === "/register";
  const isResetPassword = path.endsWith("/reset-password.html") || path === "/reset-password";

  const DEFAULT_CODE_COOLDOWN_SEC = 60;

  function clearAuthStorage() {
    try {
      localStorage.removeItem("accessToken");
      localStorage.removeItem("refreshToken");
    } catch (_) {
      /* ignore */
    }
  }

  function userConsoleBase() {
    const m = document.querySelector('meta[name="traffic-ai-user-port"]');
    if (!m?.content) return "";
    const port = m.content.trim();
    return `${window.location.protocol}//${window.location.hostname}:${port}`;
  }

  function redirectToLogin() {
    window.location.replace("/login.html");
  }

  /** 登录/注册/重置页：401 表示凭据错误等，应展示 message，不应整页跳转。 */
  function isPublicAuthPage() {
    return isLogin || isRegister || isResetPassword || isAdminLogin;
  }

  function isAdminRole(role) {
    return role === "admin" || role === "super_admin";
  }

  async function fetchProfileAfterLogin() {
    const token = localStorage.getItem("accessToken");
    const resp = await fetch("/account/profile", {
      headers: { authorization: `Bearer ${token}` },
    });
    let json;
    try {
      json = await resp.json();
    } catch {
      json = {};
    }
    if (resp.status === 401) {
      clearAuthStorage();
      throw new Error(json.message || t("common.requestFailed"));
    }
    if (typeof json.code !== "number" || json.code !== 0) {
      throw new Error(json.message || t("common.requestFailed"));
    }
    const data = json.data;
    // /account/profile 返回 { profile, dashboard, balanceAlert }，与扁平字段兼容
    if (data && typeof data === "object" && data.profile) {
      return data.profile;
    }
    return data;
  }

  /**
   * POST JSON，解析统一响应 { code, message, data }。
   * HTTP 401：清除 token；若当前不在公开认证页则跳转登录（如 token 过期）。
   */
  async function apiPost(url, body) {
    const resp = await fetch(url, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(body),
    });
    let json;
    try {
      json = await resp.json();
    } catch {
      json = {};
    }
    if (resp.status === 401) {
      clearAuthStorage();
      if (!isPublicAuthPage()) {
        redirectToLogin();
      }
      throw new Error(json.message || t("common.requestFailed"));
    }
    if (typeof json.code !== "number" || json.code !== 0) {
      throw new Error(json.message || t("common.requestFailed"));
    }
    return json;
  }

  function saveTokensFromData(data) {
    if (!data || typeof data !== "object") return;
    if (data.access_token) localStorage.setItem("accessToken", data.access_token);
    if (data.refresh_token) localStorage.setItem("refreshToken", data.refresh_token);
  }

  function cooldownSecondsFromData(data) {
    const raw = data?.data;
    if (!raw || typeof raw !== "object") return DEFAULT_CODE_COOLDOWN_SEC;
    const n = Number(raw.ttl_seconds ?? raw.ttlSeconds);
    if (!Number.isFinite(n)) return DEFAULT_CODE_COOLDOWN_SEC;
    const sec = Math.floor(n);
    if (sec < 1) return DEFAULT_CODE_COOLDOWN_SEC;
    if (sec > 3600) return 3600;
    return sec;
  }

  function setMsg(msg, ok) {
    const el = document.getElementById("msg");
    if (!el) return;
    el.className = `msg ${ok ? "ok" : "err"}`;
    el.textContent = msg;
  }

  function bindSendCodeCooldown(sendCodeBtn) {
    let sendCodeCooldownInterval = null;

    function clearSendCodeCooldown() {
      if (sendCodeCooldownInterval) {
        clearInterval(sendCodeCooldownInterval);
        sendCodeCooldownInterval = null;
      }
    }

    function restoreSendCodeButton() {
      if (!sendCodeBtn) return;
      clearSendCodeCooldown();
      sendCodeBtn.disabled = false;
      sendCodeBtn.textContent = t("auth.sendCode");
    }

    function startSendCodeCooldown(seconds) {
      if (!sendCodeBtn) return;
      clearSendCodeCooldown();
      let left = seconds;
      sendCodeBtn.disabled = true;
      const tick = () => {
        if (left <= 0) {
          restoreSendCodeButton();
          return;
        }
        sendCodeBtn.textContent = t("auth.sendCodeSentCountdown").replace("{n}", String(left));
        left -= 1;
      };
      tick();
      sendCodeCooldownInterval = setInterval(tick, 1000);
    }

    return { restoreSendCodeButton, startSendCodeCooldown };
  }

  if (isLogin) {
    const form = document.getElementById("loginForm");
    form?.addEventListener("submit", async (e) => {
      e.preventDefault();
      const email = document.getElementById("email").value.trim();
      const password = document.getElementById("password").value;
      try {
        const json = await apiPost("/auth/login", { email, password });
        saveTokensFromData(json.data);
        window.location.href = "/app.html";
      } catch (err) {
        setMsg(err.message, false);
      }
    });
  }

  if (isAdminLogin) {
    const form = document.getElementById("loginForm");
    form?.addEventListener("submit", async (e) => {
      e.preventDefault();
      const email = document.getElementById("email").value.trim();
      const password = document.getElementById("password").value;
      try {
        const json = await apiPost("/auth/login", { email, password });
        saveTokensFromData(json.data);
        const profile = await fetchProfileAfterLogin();
        if (!isAdminRole(profile.role)) {
          clearAuthStorage();
          setMsg(t("auth.notAdminRole"), false);
          return;
        }
        window.location.href = "/admin.html";
      } catch (err) {
        setMsg(err.message, false);
      }
    });
  }

  if (isRegister) {
    const form = document.getElementById("registerForm");
    const sendCodeBtn = document.getElementById("sendCodeBtn");
    const { restoreSendCodeButton, startSendCodeCooldown } = bindSendCodeCooldown(sendCodeBtn);

    sendCodeBtn?.addEventListener("click", async () => {
      if (!sendCodeBtn || sendCodeBtn.disabled) return;
      const email = document.getElementById("email").value.trim();
      if (!email) {
        setMsg(t("auth.inputEmailFirst"), false);
        return;
      }
      try {
        sendCodeBtn.disabled = true;
        sendCodeBtn.textContent = t("auth.sendCode");
        const json = await apiPost("/auth/register/send-code", { email });
        setMsg(json.message || t("auth.sendCodeOk"), true);
        startSendCodeCooldown(cooldownSecondsFromData(json));
      } catch (err) {
        setMsg(err.message, false);
        restoreSendCodeButton();
      }
    });

    form?.addEventListener("submit", async (e) => {
      e.preventDefault();
      const nickname = document.getElementById("nickname")?.value?.trim() || "";
      const email = document.getElementById("email").value.trim();
      const code = document.getElementById("verifyCode").value.trim();
      const password = document.getElementById("password").value;
      const payload = { email, password, code };
      if (nickname) payload.nickname = nickname;
      try {
        const json = await apiPost("/auth/register", payload);
        saveTokensFromData(json.data);
        window.location.href = "/app.html";
      } catch (err) {
        setMsg(err.message, false);
      }
    });
  }

  if (isResetPassword) {
    const form = document.getElementById("resetPasswordForm");
    const sendCodeBtn = document.getElementById("sendResetCodeBtn");
    const { restoreSendCodeButton, startSendCodeCooldown } = bindSendCodeCooldown(sendCodeBtn);

    sendCodeBtn?.addEventListener("click", async () => {
      if (!sendCodeBtn || sendCodeBtn.disabled) return;
      const email = document.getElementById("email").value.trim();
      if (!email) {
        setMsg(t("auth.inputEmailFirst"), false);
        return;
      }
      try {
        sendCodeBtn.disabled = true;
        sendCodeBtn.textContent = t("auth.sendCode");
        const json = await apiPost("/auth/reset-password/send-code", { email });
        setMsg(json.message || t("auth.resetSendCodeOk"), true);
        startSendCodeCooldown(cooldownSecondsFromData(json));
      } catch (err) {
        setMsg(err.message, false);
        restoreSendCodeButton();
      }
    });

    form?.addEventListener("submit", async (e) => {
      e.preventDefault();
      const email = document.getElementById("email").value.trim();
      const code = document.getElementById("verifyCode").value.trim();
      const new_password = document.getElementById("newPassword").value;
      try {
        await apiPost("/auth/reset-password", { email, code, new_password });
        setMsg(t("auth.resetOk"), true);
        setTimeout(() => {
          window.location.href = "/login.html";
        }, 800);
      } catch (err) {
        setMsg(err.message, false);
      }
    });
  }

  document.addEventListener("DOMContentLoaded", () => {
    if (!isAdminLogin) return;
    const base = userConsoleBase();
    const loginL = document.getElementById("userConsoleLoginLink");
    const forgot = document.getElementById("userForgotPasswordLink");
    if (loginL && base) loginL.href = `${base}/login.html`;
    if (forgot && base) forgot.href = `${base}/reset-password.html`;
  });
})();
