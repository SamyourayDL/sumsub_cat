import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const source = readFileSync(new URL("../internal/web/static/verification.js", import.meta.url), "utf8");
const tick = () => new Promise(resolve => setImmediate(resolve));
function harness() {
  const nodes = new Map();
  function node() {
    return { dataset: { session: "page-binding", action: "sdk", status: "not_started", poll: "false" }, disabled: false, hidden: true, textContent: "", events: {},
      addEventListener(name, fn) { this.events[name] = fn; },
      replaceChildren() { this.cleared = true; }, remove() { this.removed = true; } };
  }
  for (const id of ["verification", "verification-start", "verification-message", "verification-reload", "verification-status", "verification-meme", "sumsub-websdk-container"]) nodes.set(id, node());
  const logout = node();
  const scripts = [], calls = [], instances = [], events = {}, timers = new Map(), intervals = [];
  let timerID = 0;
  const h = { nodes, scripts, calls, instances, events, timers, intervals, logout,
    respond: async path => ({ ok: true, status: path.endsWith("session") ? 204 : 200, json: async () => path.endsWith("status") ? ({ status: "not_started", message: "Проверка не начата", action: "sdk", button: "Подтвердить личность", poll: false }) : ({ token: "opaque-test-token" }) }) };
  const document = { hidden: false, addEventListener(name, fn) { events[name] = fn; }, getElementById: id => nodes.get(id), createElement: node, head: { append: script => scripts.push(script) }, querySelector: () => logout };
  h.document = document;
  const window = { addEventListener(name, fn) { events[name] = fn; } };
  h.installSDK = () => {
    window.snsWebSdk = { init(token, renew) {
      const instance = { token, renew, handlers: {}, destroyed: false,
        withConf() { return this; }, withOptions() { return this; },
        on(name, fn) { this.handlers[name] = fn; return this; }, build() { return this; },
        launch(target) { this.target = target; }, destroy() { this.destroyed = true; } };
      instances.push(instance); return instance;
    } };
  };
  vm.runInNewContext(source, {
    window, document,
    fetch: async (path, options) => { calls.push({ path, options }); return h.respond(path, options); },
    AbortController, setTimeout: fn => { timers.set(++timerID, fn); return timerID; },
    clearTimeout: id => timers.delete(id), setInterval: fn => intervals.push(fn),
  });
  h.click = () => nodes.get("verification-start").events.click();
  h.launch = async () => { h.installSDK(); await h.click(); assert.equal(instances.length, 1); };
  return h;
}

test("lazy CDN, single launch, refresh uses same session and no stored token", async () => {
  const h = harness();
  assert.equal(h.scripts.length, 0);
  const starting = h.click(); await tick();
  assert.equal(h.scripts.length, 1);
  assert.equal(h.scripts[0].src, "https://static.sumsub.com/idensic/static/sns-websdk-builder.js");
  await h.click(); assert.equal(h.scripts.length, 1);
  h.installSDK(); h.scripts[0].onload(); await starting;
  assert.equal(h.instances.length, 1);
  assert.equal(h.instances[0].target, "#sumsub-websdk-container");
  assert.equal(await h.instances[0].renew(), "opaque-test-token");
  const tokens = h.calls.filter(c => c.path.endsWith("token"));
  assert.equal(tokens.length, 2);
  for (const { options } of tokens) {
    assert.equal(options.headers["X-Verification-Session"], "page-binding");
    assert.equal(options.method, "POST"); assert.equal(options.cache, "no-store");
    assert.equal(options.body, undefined);
  }
});

test("CDN error and CDN timeout allow retry", async () => {
  for (const timeout of [false, true]) {
    const h = harness(); const pending = h.click(); await tick();
    if (timeout) { for (const fn of [...h.timers.values()]) fn(); } else h.scripts[0].onerror();
    await pending;
    assert.equal(h.nodes.get("verification-start").disabled, false);
    assert.equal(h.scripts[0].removed, true);
    await h.launch();
  }
});

test("provider error allows retry; refresh failure destroys existing instance", async () => {
  const h = harness(); h.installSDK();
  h.respond = async () => ({ ok: false, status: 502 });
  await h.click(); assert.equal(h.instances.length, 0);
  assert.equal(h.nodes.get("verification-start").disabled, false);
  h.respond = async () => ({ ok: true, status: 200, json: async () => ({ token: "ok" }) });
  await h.click(); assert.equal(h.instances.length, 1);
  h.respond = async () => { throw new Error("network"); };
  await assert.rejects(h.instances[0].renew());
  assert.equal(h.instances[0].destroyed, true);
  assert.equal(h.nodes.get("verification-start").disabled, false);
});

test("account change and expired session block old widget and future renewal", async () => {
  for (const status of [401, 409]) {
    const h = harness(); await h.launch();
    h.respond = async () => ({ ok: false, status });
    h.intervals[0](); await tick();
    assert.equal(h.instances[0].destroyed, true);
    assert.equal(h.nodes.get("verification-start").disabled, true);
    assert.equal(h.nodes.get("verification-reload").hidden, false);
    const calls = h.calls.length;
    await assert.rejects(h.instances[0].renew());
    await h.click(); assert.equal(h.calls.length, calls);
  }
});

test("logout during token exchange cannot mount late response", async () => {
  const h = harness(); h.installSDK(); let deliver;
  h.respond = async path => path.endsWith("session") ? { ok: true, status: 204 } :
    await new Promise(resolve => { deliver = resolve; });
  const starting = h.click(); await tick();
  h.logout.events.submit();
  deliver({ ok: true, status: 200, json: async () => ({ token: "late" }) });
  await starting; assert.equal(h.instances.length, 0);
  assert.equal(h.calls.at(-1).options.signal.aborted, true);
});

test("SDK error destroys frame and can retry; pagehide disposes current widget", async () => {
  const h = harness(); await h.launch();
  h.instances[0].handlers["idCheck.onError"]({ message: "sensitive" });
  assert.equal(h.instances[0].destroyed, true);
  assert.equal(h.nodes.get("verification-message").textContent.includes("sensitive"), false);
  await h.click(); assert.equal(h.instances.length, 2);
  h.events.pagehide(); assert.equal(h.instances[1].destroyed, true);
  h.events.pageshow({ persisted: true }); await tick();
  assert.equal(h.nodes.get("verification-start").disabled, false);
});

test("polling is serial, pauses when hidden, retains status on error, stops at terminal and refreshes on focus", async () => {
 const h=harness(); await h.launch();
 let deliver;
 h.respond=async()=>await new Promise(resolve=>{deliver=resolve;});
 h.intervals[0](); h.intervals[0](); await tick();
 const count=h.calls.length;
 h.document.hidden=true; h.intervals[0](); assert.equal(h.calls.length,count);
 deliver({ok:true,status:200,json:async()=>({status:"pending",message:"Ожидаем",action:"none",button:"",poll:true})}); await tick();
 assert.equal(h.nodes.get("verification-status").textContent,"Ожидаем");
 h.document.hidden=false;
 h.respond=async()=>{throw Error("network");}; h.intervals[0](); await tick();
 assert.equal(h.nodes.get("verification-status").textContent,"Ожидаем");
 assert.equal(h.instances[0].destroyed,false);
 h.respond=async()=>({ok:true,status:200,json:async()=>({status:"approved",message:"Подтверждена",action:"meme",button:"",poll:false})});
 h.events.visibilitychange(); await tick();
 assert.equal(h.nodes.get("verification-meme").hidden,false);
 assert.equal(h.instances[0].destroyed,true);
 const terminalCalls=h.calls.length; h.intervals[0](); await tick(); assert.equal(h.calls.length,terminalCalls);
 h.events.focus(); await tick(); assert.equal(h.calls.length,terminalCalls+1);
});

test("SDK result cannot approve; RETRY reopens and does not destroy the new attempt", async()=>{
 const h=harness(); await h.launch();
 h.instances[0].handlers["idCheck.onApplicantSubmitted"]({reviewAnswer:"GREEN"}); await tick();
 assert.equal(h.nodes.get("verification-meme").hidden,true);
 h.respond=async path=>({ok:true,status:path.endsWith("session")?204:200,json:async()=>path.endsWith("token")?{token:"same-applicant"}:{status:"retry",message:"Исправьте",action:"sdk",button:"Исправить материалы",poll:true}});
 h.intervals[0](); await tick(); assert.equal(h.instances[0].destroyed,true);
 await h.click(); assert.equal(h.instances.length,2);
 h.intervals[0](); await tick(); assert.equal(h.instances[1].destroyed,false);
});
