(() => {
  "use strict";
  const root = document.getElementById("verification");
  if (!root) return;
  const button = document.getElementById("verification-start");
  const message = document.getElementById("verification-message");
  const reload = document.getElementById("verification-reload");
  const container = document.getElementById("sumsub-websdk-container");
  const headers = { "X-Verification-Session": root.dataset.session };
  let instance = null;
  let loadingSDK = null;
  let generation = 0;
  let blocked = false;
  let active = false;
  let checking = false;
  const requests = new Set();

  function stop() {
    generation++;
    active = false;
    for (const controller of requests) controller.abort();
    requests.clear();
    const previous = instance;
    instance = null;
    try { previous?.destroy(); } catch { /* Remove the iframe even if SDK cleanup fails. */ }
    container.replaceChildren();
  }
  function sessionLost() {
    blocked = true;
    stop();
    button.disabled = true;
    message.textContent = "Сессия истекла или аккаунт изменился. Откройте кабинет заново.";
    reload.hidden = false;
  }
  function failed() {
    if (blocked) return;
    stop();
    button.disabled = false;
    button.textContent = "Попробовать ещё раз";
    message.textContent = "Не удалось продолжить проверку. Проверьте соединение и попробуйте ещё раз.";
  }
  async function request(path, method = "GET") {
    const controller = new AbortController();
    requests.add(controller);
    const timeout = setTimeout(() => controller.abort(), 12000);
    try {
      const response = await fetch(path, { method, headers, credentials: "same-origin", cache: "no-store", signal: controller.signal });
      if (response.status === 401 || response.status === 409) {
        sessionLost();
        throw new Error("session");
      }
      if (!response.ok) throw new Error("request");
      return method === "POST" ? await response.json() : null;
    } finally {
      clearTimeout(timeout);
      requests.delete(controller);
    }
  }
  function loadSDK() {
    if (window.snsWebSdk) return Promise.resolve();
    if (loadingSDK) return loadingSDK;
    loadingSDK = new Promise((resolve, reject) => {
      const script = document.createElement("script");
      const timeout = setTimeout(() => finish(false), 15000);
      function finish(ok) {
        clearTimeout(timeout);
        script.onload = script.onerror = null;
        if (ok && window.snsWebSdk) resolve();
        else { script.remove(); reject(new Error("sdk")); }
      }
      script.src = "https://static.sumsub.com/idensic/static/sns-websdk-builder.js";
      script.async = true;
      script.onload = () => finish(true);
      script.onerror = () => finish(false);
      document.head.append(script);
    }).catch(error => { loadingSDK = null; throw error; });
    return loadingSDK;
  }
  async function token(run) {
    if (blocked || run !== generation) throw new Error("stale");
    try {
      const data = await request("/account/verification/token", "POST");
      if (blocked || run !== generation) throw new Error("stale");
      if (typeof data.token !== "string" || !data.token) throw new Error("token");
      return data.token;
    } catch (error) {
      if (run === generation) failed();
      throw error;
    }
  }
  button.addEventListener("click", async () => {
    if (active || blocked) return;
    active = true;
    button.disabled = true;
    message.textContent = "Открываем проверку…";
    const run = ++generation;
    try {
      await request("/account/verification/session");
      if (run !== generation) return;
      await loadSDK();
      if (run !== generation) return;
      const initial = await token(run);
      if (run !== generation) return;
      instance = window.snsWebSdk.init(initial, () => token(run))
        .withConf({ lang: "ru", theme: "light" })
        .withOptions({ addViewportTag: false, adaptIframeHeight: true })
        .on("idCheck.onError", () => { if (run === generation) failed(); })
        .build();
      instance.launch("#sumsub-websdk-container");
      if (run === generation) message.textContent = "Следуйте инструкциям в виджете. Это тестовая проверка.";
    } catch {
      if (run === generation) failed();
    }
  });
  async function checkSession() {
    if (blocked || checking) return;
    checking = true;
    const run = generation;
    try { await request("/account/verification/session"); }
    catch { if (active && run === generation) failed(); }
    finally { checking = false; }
  }
  // Detect logout/account changes in another tab without minting more SDK tokens.
  setInterval(() => { if (active) void checkSession(); }, 15000);
  window.addEventListener("focus", () => { void checkSession(); });
  window.addEventListener("pagehide", stop);
  window.addEventListener("pageshow", event => {
    if (event.persisted) { button.disabled = blocked; void checkSession(); }
  });
  document.querySelector('form[action="/logout"]')?.addEventListener("submit", () => { blocked = true; stop(); });
})();
