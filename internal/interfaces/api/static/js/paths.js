(function (global) {
  function cfg() {
    return global.__TRAFFIC_CONFIG__ || {};
  }

  function normPath(p) {
    if (!p || p === "/") return "";
    return String(p).replace(/\/$/, "");
  }

  function controlPath() {
    return normPath(cfg().controlPath);
  }

  function gatewayPath() {
    return normPath(cfg().gatewayPath);
  }

  function withLeadingSlash(path) {
    if (!path) return "/";
    return path.startsWith("/") ? path : `/${path}`;
  }

  function gatewayBase() {
    const gp = gatewayPath();
    if (gp) {
      return `${global.location.protocol}//${global.location.hostname}${gp}`;
    }
    const port = cfg().gatewayPort || "8081";
    return `${global.location.protocol}//${global.location.hostname}:${port}`;
  }

  function userConsoleBase() {
    const cp = controlPath();
    if (cp) {
      return `${global.location.protocol}//${global.location.hostname}${cp}`;
    }
    const port = cfg().userPort;
    if (!port) return "";
    return `${global.location.protocol}//${global.location.hostname}:${port}`;
  }

  function adminConsoleBase() {
    const cp = controlPath();
    if (cp) {
      return `${global.location.protocol}//${global.location.hostname}${cp}`;
    }
    const port = cfg().adminPort;
    if (!port) return "";
    return `${global.location.protocol}//${global.location.hostname}:${port}`;
  }

  function api(path) {
    return controlPath() + withLeadingSlash(path);
  }

  function page(path) {
    return controlPath() + withLeadingSlash(path);
  }

  global.trafficPaths = {
    controlPath,
    gatewayPath,
    gatewayBase,
    userConsoleBase,
    adminConsoleBase,
    api,
    page,
  };

  document.addEventListener("DOMContentLoaded", () => {
    const base = controlPath();
    if (!base) return;
    document.querySelectorAll('a[href^="/"]').forEach((a) => {
      const href = a.getAttribute("href");
      if (!href || href.startsWith("//")) return;
      if (/^\/(v1|v1beta)(\/|$)/.test(href)) return;
      a.setAttribute("href", base + href);
    });
  });
})(window);
