(function (global) {
  const BRAND = "#2563eb";
  const CANCEL = "#6b7280";
  const DANGER = "#dc2626";

  function uiT(key, fallback) {
    if (global.I18N && typeof global.I18N.t === "function") {
      return global.I18N.t(key);
    }
    return fallback || key;
  }

  function hasSwal() {
    return typeof global.Swal !== "undefined" && typeof global.Swal.fire === "function";
  }

  async function alert(text, opts) {
    const msg = String(text);
    if (!hasSwal()) {
      global.alert(msg);
      return;
    }
    await global.Swal.fire({
      icon: "info",
      text: msg,
      confirmButtonColor: BRAND,
      confirmButtonText: uiT("common.ok", "OK"),
      ...(opts || {}),
    });
  }

  async function alertError(text, opts) {
    const msg = String(text);
    if (!hasSwal()) {
      global.alert(msg);
      return;
    }
    await global.Swal.fire({
      icon: "error",
      text: msg,
      confirmButtonColor: BRAND,
      confirmButtonText: uiT("common.ok", "OK"),
      ...(opts || {}),
    });
  }

  async function confirm(text, opts) {
    const msg = String(text);
    if (!hasSwal()) {
      return global.confirm(msg);
    }
    const r = await global.Swal.fire({
      icon: "question",
      text: msg,
      showCancelButton: true,
      confirmButtonColor: BRAND,
      cancelButtonColor: CANCEL,
      confirmButtonText: uiT("common.confirm", "Confirm"),
      cancelButtonText: uiT("common.cancel", "Cancel"),
      focusCancel: true,
      ...(opts || {}),
    });
    return !!r.isConfirmed;
  }

  /**
   * 两次确认（如删除）：第一次警告，第二次危险色确认。
   */
  async function confirmTwoStep(firstText, secondText, opts) {
    const o = opts || {};
    const ok1 = await confirm(firstText, { icon: "warning", ...(o.first || {}) });
    if (!ok1) return false;
    return confirm(secondText, {
      icon: "error",
      confirmButtonColor: DANGER,
      ...(o.second || {}),
    });
  }

  global.UiDialog = {
    alert,
    alertError,
    confirm,
    confirmTwoStep,
  };
})(typeof window !== "undefined" ? window : globalThis);
