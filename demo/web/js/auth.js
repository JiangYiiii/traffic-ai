(function () {
  const t = (k) => (window.I18N ? window.I18N.t(k) : k);
  const path = window.location.pathname;
  const isLogin = path.endsWith("/login.html") || path === "/login";
  const isRegister = path.endsWith("/register.html") || path === "/register";
  const isResetPassword = path.endsWith("/reset-password.html") || path === "/reset-password";

  async function send(url, body) {
    const resp = await fetch(url, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(body),
    });
    const data = await resp.json().catch(() => ({}));
    if (!resp.ok) throw new Error(data.error || t("common.requestFailed"));
    return data;
  }

  function setMsg(msg, ok) {
    const el = document.getElementById("msg");
    if (!el) return;
    el.className = `msg ${ok ? "ok" : "err"}`;
    el.textContent = msg;
  }

  if (isLogin) {
    const form = document.getElementById("loginForm");
    form?.addEventListener("submit", async (e) => {
      e.preventDefault();
      const email = document.getElementById("email").value.trim();
      const password = document.getElementById("password").value;
      try {
        const data = await send("/auth/login", { email, password });
        localStorage.setItem("accessToken", data.accessToken);
        window.location.href = "/app.html";
      } catch (err) {
        setMsg(err.message, false);
      }
    });
  }

  if (isRegister) {
    const form = document.getElementById("registerForm");
    const sendCodeBtn = document.getElementById("sendCodeBtn");
    let sendCodeCooldownInterval = null;

    function cooldownSecondsFromResponse(data) {
      const n = Number(data?.ttlSeconds);
      if (!Number.isFinite(n)) return 60;
      const sec = Math.floor(n);
      if (sec < 1) return 60;
      if (sec > 3600) return 3600;
      return sec;
    }

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
        const data = await send("/auth/register/send-code", { email });
        if (data.sent === false && !data.warning) {
          setMsg(t("auth.sendCodeAlreadyRegistered"), false);
          restoreSendCodeButton();
          return;
        }
        const msg = data.warning || t("auth.sendCodeOk");
        setMsg(msg, data.sent !== false);
        startSendCodeCooldown(cooldownSecondsFromResponse(data));
      } catch (err) {
        setMsg(err.message, false);
        restoreSendCodeButton();
      }
    });

    form?.addEventListener("submit", async (e) => {
      e.preventDefault();
      const name = document.getElementById("name").value.trim();
      const email = document.getElementById("email").value.trim();
      const verifyCode = document.getElementById("verifyCode").value.trim();
      const password = document.getElementById("password").value;
      const inviteCode = document.getElementById("inviteCode")?.value?.trim() || "";
      try {
        const payload = { name, email, password, verifyCode };
        if (inviteCode) payload.inviteCode = inviteCode;
        const data = await send("/auth/register", payload);
        localStorage.setItem("accessToken", data.accessToken);
        window.location.href = "/app.html";
      } catch (err) {
        setMsg(err.message, false);
      }
    });
  }

  if (isResetPassword) {
    const form = document.getElementById("resetPasswordForm");
    const sendCodeBtn = document.getElementById("sendResetCodeBtn");
    let sendCodeCooldownInterval = null;

    function cooldownSecondsFromResponse(data) {
      const n = Number(data?.ttlSeconds);
      if (!Number.isFinite(n)) return 60;
      const sec = Math.floor(n);
      if (sec < 1) return 60;
      if (sec > 3600) return 3600;
      return sec;
    }

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
        const data = await send("/auth/reset-password/send-code", { email });
        const msg = data.warning || t("auth.resetSendCodeOk");
        setMsg(msg, true);
        startSendCodeCooldown(cooldownSecondsFromResponse(data));
      } catch (err) {
        setMsg(err.message, false);
        restoreSendCodeButton();
      }
    });

    form?.addEventListener("submit", async (e) => {
      e.preventDefault();
      const email = document.getElementById("email").value.trim();
      const verifyCode = document.getElementById("verifyCode").value.trim();
      const newPassword = document.getElementById("newPassword").value;
      try {
        await send("/auth/reset-password", { email, verifyCode, newPassword });
        setMsg(t("auth.resetOk"), true);
        setTimeout(() => {
          window.location.href = "/login.html";
        }, 800);
      } catch (err) {
        setMsg(err.message, false);
      }
    });
  }
})();
