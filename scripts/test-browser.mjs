// Real Chrome + native messaging + SQLite, using only private fixtures/profiles.
// The fixture host permission is pre-granted in a private copy of the extension;
// the shipped manifest keeps all website access optional.
import { spawn, spawnSync } from "node:child_process";
import { createServer } from "node:http";
import { mkdtemp, readFile, writeFile, mkdir, cp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { resolve, join } from "node:path";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const project = resolve(import.meta.dirname, "..");
const work = await mkdtemp(join(tmpdir(), "cliprag-browser-test-"));
const environment = {
  ...process.env,
  XDG_CONFIG_HOME: join(work, "config"),
  XDG_DATA_HOME: join(work, "data"),
};
let chrome, client;
const require = createRequire(join(project, "frontend/package.json"));
const reactFixture = await require("esbuild").build({
  stdin: {
    contents: `import React from 'react';import {createRoot} from 'react-dom/client';function App(){const [text,setText]=React.useState('');const [n,setN]=React.useState(0);return React.createElement('div',null,React.createElement('label',null,'Controlled React input',React.createElement('input',{id:'controlled',value:text,onChange:e=>{setText(e.target.value);window.controlledText=e.target.value}})),React.createElement('button',{id:'rerender',type:'button',onClick:()=>setN(n+1)},'Rerender '+n))}createRoot(document.getElementById('react-root')).render(React.createElement(App));`,
    resolveDir: join(project, "frontend"),
  },
  bundle: true,
  write: false,
  format: "iife",
  minify: true,
});
const server = createServer((request, response) => {
  if (request.url === "/react.js") {
    response.setHeader("Content-Type", "application/javascript");
    response.end(reactFixture.outputFiles[0].text);
    return;
  }
  response.setHeader("Content-Type", "text/html; charset=utf-8");
  response.end(
    `<!doctype html><html><head><title>ClipRAG browser check</title><style>body{font:17px system-ui;background:#121019;color:#eee;margin:50px;max-width:800px}h1{color:#c3a4ec}label{display:block;margin-top:25px}input,textarea,[contenteditable]{display:block;width:600px;padding:14px;margin-top:8px;background:#26202e;color:#fff;border:1px solid #715887;border-radius:8px;font:17px system-ui}textarea{height:120px}</style></head><body><h1>Clipboard suggestions, as you type.</h1><p>A private ClipRAG integration fixture.</p><form onsubmit="event.preventDefault();window.submits++"><label>Text or saved URL<input id="text"></label><label>Multiline snippets<textarea id="area"></textarea></label><label>Rich editor<div contenteditable="true" id="rich"><b>Existing</b></div></label><label>Password<input type="password" id="password"></label><label>Payment<input id="payment" autocomplete="cc-number"></label><input id="limited" maxlength="5"><button>Submit</button></form><div contenteditable="false"><div contenteditable="true" role="textbox" aria-label="Message Body" g_editable="true" id="gmail"><div>Earlier paragraph</div><div id="gmail-line"><br></div><div class="gmail_signature" contenteditable="false">Signature stays unchanged</div><blockquote>Quoted mail stays unchanged</blockquote></div></div><div contenteditable="true" role="textbox" aria-label="Message Body" id="gmail-br">Earlier paragraph<br><span id="gmail-caret"></span><br><div class="gmail_signature">Signature stays unchanged</div></div><div id="react-root"></div><script src="/react.js"></script><script>window.submits=0;window.events=0;window.xss=0;document.addEventListener('input',()=>window.events++);</script></body></html>`,
  );
});
await new Promise((done) => server.listen(0, "127.0.0.1", done));
const origin = `http://127.0.0.1:${server.address().port}`;
const urlClip = origin + "/saved-url?x=clipboard";
const records = [
  ["Hello from my clipboard", "Text", 7],
  ["Hello second choice", "Text", 2],
  [urlClip, "URLs", 2],
  ["SELECT id FROM users;", "SQL", 1],
  ['{"name":"Anmol"}', "JSON", 1],
  ["Snippet first line\n  second line\tindented", "Code", 1],
  ['Markup <img src=x onerror="window.xss=1">', "Code", 1],
  ["Hello password=supersecret", "Text", 100],
  ["Hello escape\u001b[2J", "Text", 100],
  ["Hello 👩‍💻 developer", "Text", 1],
  ["git checkout main", "Git", 1],
];
const schema = await readFile(
  join(project, "internal/storage/migrations.go"),
  "utf8",
);
const sql = ["schemaV1", "schemaV2"]
  .map(
    (name) =>
      schema.match(new RegExp("const " + name + " = `([\\s\\S]*?)`"))[1],
  )
  .join("\n");
const seeded = spawnSync(
  "python3",
  [
    "-c",
    `import sys,json,sqlite3,hashlib,pathlib,time
data=json.load(sys.stdin);p=pathlib.Path(sys.argv[1])/'cliprag';p.mkdir(parents=True,mode=0o700)
with sqlite3.connect(p/'history.db') as db:
 db.executescript(data['schema'])
 for text,category,count in data['records']:
  db.execute('INSERT INTO entries(content,hash,category,first_copied,last_copied,copy_count) VALUES(?,?,?,?,?,?)',(text,hashlib.sha256(text.encode()).hexdigest(),category,int(time.time()*1000),int(time.time()*1000),count))
(p/'history.db').chmod(0o600)`,
    environment.XDG_DATA_HOME,
  ],
  { input: JSON.stringify({ schema: sql, records }), encoding: "utf8" },
);
assert.equal(seeded.status, 0, seeded.stderr);

class CDP {
  constructor(process) {
    this.process = process;
    this.next = 0;
    this.pending = new Map();
    this.buffer = "";
    process.stdio[4].on("data", (chunk) => {
      this.buffer += chunk.toString();
      let end;
      while ((end = this.buffer.indexOf("\0")) >= 0) {
        const raw = this.buffer.slice(0, end);
        this.buffer = this.buffer.slice(end + 1);
        if (!raw) continue;
        const message = JSON.parse(raw),
          entry = this.pending.get(message.id);
        if (!entry) continue;
        clearTimeout(entry.timer);
        this.pending.delete(message.id);
        if (message.error)
          entry.reject(new Error(JSON.stringify(message.error)));
        else entry.resolve(message.result);
      }
    });
    process.on("exit", () => {
      for (const entry of this.pending.values()) {
        clearTimeout(entry.timer);
        entry.reject(new Error("Chrome exited"));
      }
      this.pending.clear();
    });
  }
  send(method, params = {}, sessionId) {
    return new Promise((resolve, reject) => {
      const id = ++this.next;
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error("CDP timeout: " + method));
      }, 15000);
      this.pending.set(id, { resolve, reject, timer });
      this.process.stdio[3].write(
        JSON.stringify({
          id,
          method,
          params,
          ...(sessionId ? { sessionId } : {}),
        }) + "\0",
      );
    });
  }
  async evaluate(session, expression) {
    const result = await this.send(
      "Runtime.evaluate",
      {
        expression,
        awaitPromise: true,
        returnByValue: true,
        userGesture: true,
      },
      session,
    );
    if (result.exceptionDetails)
      throw new Error(
        result.exceptionDetails.exception?.description ||
          JSON.stringify(result.exceptionDetails),
      );
    return result.result.value;
  }
}
async function waitFor(check, label, timeout = 7000) {
  const until = Date.now() + timeout;
  while (Date.now() < until) {
    const value = await check();
    if (value) return value;
    await new Promise((r) => setTimeout(r, 70));
  }
  throw new Error("Timeout: " + label);
}
async function sessionFor(targetId) {
  return (
    await client.send("Target.attachToTarget", { targetId, flatten: true })
  ).sessionId;
}
async function key(session, name) {
  await client.send(
    "Input.dispatchKeyEvent",
    {
      type: "keyDown",
      key: name,
      code: name,
      windowsVirtualKeyCode:
        {
          Tab: 9,
          Enter: 13,
          Escape: 27,
          ArrowRight: 39,
          ArrowDown: 40,
          ArrowUp: 38,
        }[name] || 0,
    },
    session,
  );
  await client.send(
    "Input.dispatchKeyEvent",
    { type: "keyUp", key: name, code: name },
    session,
  );
}
function uses(text) {
  const result = spawnSync(
    "python3",
    [
      "-c",
      `import sqlite3,sys
with sqlite3.connect(sys.argv[1]) as db: print(db.execute('SELECT autocomplete_count FROM entries WHERE content=?',(sys.argv[2],)).fetchone()[0])`,
      join(environment.XDG_DATA_HOME, "cliprag/history.db"),
      text,
    ],
    { encoding: "utf8" },
  );
  assert.equal(result.status, 0, result.stderr);
  return Number(result.stdout.trim());
}
try {
  const profile = join(work, "profile"),
    extension = join(work, "extension");
  await mkdir(profile, { recursive: true });
  await cp(join(project, "bin/cliprag-browser-extension"), extension, {
    recursive: true,
  });
  const manifest = JSON.parse(
    await readFile(join(extension, "manifest.json"), "utf8"),
  );
  assert(
    !manifest.host_permissions && !manifest.content_scripts,
    "shipping extension grants website access without opt-in",
  );
  manifest.host_permissions = ["http://127.0.0.1/*"]; // Private test pre-grant only.
  await writeFile(join(extension, "manifest.json"), JSON.stringify(manifest));
  const installed = spawnSync(
    join(project, "bin/cliprag-browser"),
    [
      "install",
      "chrome",
      "--manifest-dir",
      join(profile, "NativeMessagingHosts"),
    ],
    { env: environment, encoding: "utf8" },
  );
  assert.equal(installed.status, 0, installed.stderr);
  let stderr = "";
  chrome = spawn(
    process.env.CLIPRAG_CHROME || "/usr/bin/google-chrome",
    [
      "--headless=new",
      "--remote-debugging-pipe",
      "--enable-unsafe-extension-debugging",
      "--enable-automation",
      "--user-data-dir=" + profile,
      "--no-first-run",
      "--no-default-browser-check",
      "--disable-background-networking",
      "--disable-sync",
      "--disable-component-update",
      "--disable-gpu",
      "--window-size=1100,900",
    ],
    { env: environment, stdio: ["ignore", "ignore", "pipe", "pipe", "pipe"] },
  );
  chrome.stderr.on("data", (chunk) => {
    stderr = (stderr + chunk).slice(-5000);
  });
  client = new CDP(chrome);
  console.log("Chrome:", (await client.send("Browser.getVersion")).product);
  const loaded = await client.send("Extensions.loadUnpacked", {
    path: extension,
  });
  const identity = JSON.parse(
    await readFile(join(project, "integrations/browser/identity.json"), "utf8"),
  );
  assert.equal(loaded.id, identity.id, "extension ID drifted");
  const background = await waitFor(
    async () =>
      (await client.send("Target.getTargets")).targetInfos.find(
        (t) =>
          t.type === "service_worker" &&
          t.url.startsWith("chrome-extension://" + identity.id + "/"),
      ),
    "extension service worker",
  );
  const worker = await sessionFor(background.targetId);
  await waitFor(
    () => client.evaluate(worker, `typeof native === 'function'`),
    "extension service worker initialization",
  );
  const status = await client.evaluate(worker, `native({type:'status'})`);
  assert(status.ok && status.ready, JSON.stringify(status));
  const tab = await client.send("Target.createTarget", { url: origin });
  const page = await sessionFor(tab.targetId);
  await client.send("Page.enable", {}, page);
  await waitFor(
    () => client.evaluate(page, `!!document.querySelector('#text')`),
    "fixture load",
  );
  // Persist the first opt-in without a follow-up configure message, as happens
  // if Chrome closes the popup during permission/activation work.
  const browserTabId = await client.evaluate(
    worker,
    `(async()=>(await chrome.tabs.query({url:${JSON.stringify(origin + "/*")}}))[0].id)()`,
  );
  await client.evaluate(
    worker,
    `chrome.storage.local.set({sites:[${JSON.stringify(origin)}],paused:false})`,
  );
  await waitFor(
    () =>
      client.evaluate(
        worker,
        `(async()=>{const results=await chrome.scripting.executeScript({target:{tabId:${browserTabId}},func:()=>typeof globalThis.__clipragBrowserLoaded==='function'});return results[0]?.result})()`,
      ),
    "first opt-in activates existing page without another popup message",
  );
  const focus = async (selector) => {
    await key(page, "Escape");
    await client.evaluate(
      page,
      `document.querySelector(${JSON.stringify(selector)}).scrollIntoView({block:'center'});document.querySelector(${JSON.stringify(selector)}).focus();document.querySelector(${JSON.stringify(selector)}).value=''`,
    );
  };
  const type = async (text) => client.send("Input.insertText", { text }, page);
  const value = async (selector) =>
    client.evaluate(
      page,
      `document.querySelector(${JSON.stringify(selector)}).value`,
    );
  // Reading the closed shadow root is done only through Chrome's debugger in
  // this private test; ordinary website JavaScript cannot access the previews.
  async function visible() {
    const document = await client.send(
      "DOM.getDocument",
      { depth: -1, pierce: true },
      page,
    );
    function find(node, hidden = false) {
      hidden ||= node.attributes?.includes("hidden");
      if (
        !hidden &&
        node.nodeType === 1 &&
        node.attributes?.includes("aria-label") &&
        node.attributes.includes("Saved clipboard suggestions")
      )
        return true;
      return [...(node.children || []), ...(node.shadowRoots || [])].some(
        (child) => find(child, hidden),
      );
    }
    return find(document.root);
  }
  await focus("#text");
  await type("Hel");
  await waitFor(visible, "automatic text suggestions");
  assert.equal(uses(records[0][0]), 0, "display counted");
  await key(page, "Tab");
  await waitFor(
    async () => (await value("#text")) === records[0][0],
    "text acceptance",
  );
  await waitFor(() => uses(records[0][0]) === 1, "acceptance count");
  assert.equal(
    await client.evaluate(page, "window.submits"),
    0,
    "acceptance submitted form",
  );
  console.log(
    "PASS Chrome: native messaging, automatic normal-text suggestions, insertion, usage count, no form submission",
  );
  // Re-injection must refresh preferences and await readiness, with one set of
  // listeners. Deliberately deliver an older disabled read after a newer one.
  await client.evaluate(
    worker,
    `chrome.scripting.executeScript({target:{tabId:${browserTabId}},func:()=>{const original=chrome.storage.local.get.bind(chrome.storage.local);let delayed=true;chrome.storage.local.get=(keys)=>{if(delayed){delayed=false;return new Promise(resolve=>setTimeout(()=>resolve({sites:[],paused:false}),250));}return original(keys);};globalThis.restoreClipragStorage=()=>{chrome.storage.local.get=original;};}})`,
  );
  const readiness = await client.evaluate(
    worker,
    `Promise.all([1,2].map(()=>chrome.scripting.executeScript({target:{tabId:${browserTabId}},files:['content.js']})))`,
  );
  assert(
    readiness.every(
      (results) => results[0]?.result?.ready && results[0]?.result?.enabled,
    ),
    "re-injection did not await the latest preferences",
  );
  await client.evaluate(
    worker,
    `chrome.scripting.executeScript({target:{tabId:${browserTabId}},func:()=>globalThis.restoreClipragStorage()})`,
  );
  await focus("#text");
  await type("Hel");
  await waitFor(visible, "enabled state survives an older disabled read");
  await key(page, "Escape");
  await client.send("Page.reload", {}, page);
  await waitFor(
    () =>
      client.evaluate(
        page,
        `document.readyState==='complete' && document.querySelector('#text')?.value===''`,
      ),
    "new document loaded",
  );
  await waitFor(
    () =>
      client.evaluate(
        worker,
        `(async()=>{const results=await chrome.scripting.executeScript({target:{tabId:${browserTabId}},func:()=>typeof globalThis.__clipragBrowserLoaded==='function'});return results[0]?.result})()`,
      ),
    "saved opt-in automatically injects on navigation",
  );
  await focus("#text");
  await type("Hel");
  await waitFor(visible, "suggestions after navigation without opening popup");
  await key(page, "Escape");
  console.log(
    "PASS Chrome: first enable activates immediately; repeated injection waits for current preferences",
  );

  for (const [prefix, expected] of [
    [origin + "/sav", urlClip],
    ["sel", records[3][0]],
    ['{"n', records[4][0]],
    ["Mar", records[6][0]],
  ]) {
    await focus("#text");
    await type(prefix);
    await waitFor(visible, "suggestions for " + prefix);
    await key(page, "Enter");
    await waitFor(
      async () => (await value("#text")) === expected,
      "insert " + prefix,
    );
    await waitFor(() => uses(expected) === 1, "count " + prefix);
  }
  assert.equal(
    await client.evaluate(page, "window.xss"),
    0,
    "clipboard HTML was evaluated",
  );
  assert.equal(
    await client.evaluate(page, "window.submits"),
    0,
    "Enter acceptance submitted form",
  );
  console.log(
    "PASS Chrome: URLs, SQL, JSON and code as plain text; case-insensitive matching; Enter isolation",
  );

  await focus("#text");
  await type("Sni");
  await new Promise((r) => setTimeout(r, 600));
  assert.equal(
    await visible(),
    false,
    "multiline clip offered in single-line field",
  );
  await focus("#area");
  await type("Sni");
  await waitFor(visible, "multiline suggestion");
  await key(page, "Tab");
  assert.equal(await value("#area"), records[5][0]);
  await waitFor(() => uses(records[5][0]) === 1, "multiline count");
  await focus("#rich");
  await client.evaluate(
    page,
    `const editor=document.querySelector('#rich'),range=document.createRange();range.selectNodeContents(editor);range.collapse(false);const selection=getSelection();selection.removeAllRanges();selection.addRange(range);`,
  );
  await type(" Hel");
  await waitFor(visible, "rich editor fallback word");
  await key(page, "Tab");
  assert.equal(
    await client.evaluate(page, `document.querySelector('#rich').textContent`),
    "Existing " + records[0][0],
  );
  assert(
    (
      await client.evaluate(
        page,
        `document.querySelector('#rich b').textContent`,
      )
    ).startsWith("Existing"),
  );
  console.log(
    "PASS Chrome: exact multiline insertion and rich editor formatting retained",
  );

  for (const [editorId, caretId] of [
    ["gmail", "gmail-line"],
    ["gmail-br", "gmail-caret"],
  ]) {
    await key(page, "Escape");
    await client.evaluate(
      page,
      `(()=>{const editor=document.getElementById(${JSON.stringify(editorId)}),line=document.getElementById(${JSON.stringify(caretId)});editor.scrollIntoView({block:'center'});editor.focus();line.replaceChildren(document.createTextNode('H'));const range=document.createRange();range.setStart(line.firstChild,1);range.collapse(true);const selection=getSelection();selection.removeAllRanges();selection.addRange(range);})()`,
    );
    await type("el");
    await waitFor(visible, "Gmail-style composer " + editorId);
    await key(page, "Tab");
    assert.equal(
      await client.evaluate(
        page,
        `document.getElementById(${JSON.stringify(editorId)}).textContent`,
      ),
      "Earlier paragraph" +
        records[0][0] +
        "Signature stays unchanged" +
        (editorId === "gmail" ? "Quoted mail stays unchanged" : ""),
    );
    assert.equal(
      await client.evaluate(
        page,
        `document.getElementById(${JSON.stringify(editorId)}).querySelector('.gmail_signature').textContent`,
      ),
      "Signature stays unchanged",
    );
  }
  assert.equal(
    await client.evaluate(
      page,
      `document.querySelector('#gmail blockquote').textContent`,
    ),
    "Quoted mail stays unchanged",
  );
  console.log(
    "PASS Chrome: Gmail-style editable overrides, multiline/BR boundaries, signatures and quoted mail",
  );

  await focus("#controlled");
  await type("Hel");
  await waitFor(visible, "React-controlled input");
  await key(page, "Tab");
  await waitFor(
    () =>
      client.evaluate(
        page,
        `window.controlledText===${JSON.stringify(records[0][0])}`,
      ),
    "React state receives accepted text",
  );
  await client.evaluate(page, `document.querySelector('#rerender').click()`);
  assert.equal(
    await value("#controlled"),
    records[0][0],
    "React rerender lost insertion",
  );
  console.log("PASS Chrome: controlled React input retains accepted content");

  await focus("#text");
  await type("Hel");
  await waitFor(visible, "mouse selection");
  const dom = await client.send(
    "DOM.getDocument",
    { depth: -1, pierce: true },
    page,
  );
  const optionNodes = [];
  function collectOptions(node, hidden = false) {
    hidden ||= node.attributes?.includes("hidden");
    if (
      !hidden &&
      node.attributes?.includes("option") &&
      node.attributes.includes("role")
    )
      optionNodes.push(node.nodeId);
    for (const child of [...(node.children || []), ...(node.shadowRoots || [])])
      collectOptions(child, hidden);
  }
  collectOptions(dom.root);
  const box = (
    await client.send("DOM.getBoxModel", { nodeId: optionNodes[1] }, page)
  ).model.border;
  const point = {
    x: (box[0] + box[4]) / 2,
    y: (box[1] + box[5]) / 2,
    button: "left",
    clickCount: 1,
  };
  await client.send(
    "Input.dispatchMouseEvent",
    { type: "mousePressed", ...point },
    page,
  );
  await client.send(
    "Input.dispatchMouseEvent",
    { type: "mouseReleased", ...point },
    page,
  );
  await waitFor(
    async () => (await value("#text")) === records[1][0],
    "mouse acceptance",
  );
  await waitFor(() => uses(records[1][0]) === 1, "mouse acceptance count");
  assert.equal(
    await client.evaluate(
      worker,
      `allowed(${JSON.stringify("http://127.0.0.1:1")})`,
    ),
    false,
    "website opt-in leaked to another origin",
  );
  assert.equal(
    await client.evaluate(
      page,
      `Array.from(document.querySelectorAll('div')).some(el=>el.shadowRoot?.querySelector('[role=listbox]'))`,
    ),
    false,
    "previews exposed through an open shadow root",
  );
  console.log("PASS Chrome: click selection and website-origin isolation");

  for (const selector of ["#password", "#payment", "#limited"]) {
    await focus(selector);
    await type("Hel");
    await new Promise((r) => setTimeout(r, 400));
    assert.equal(
      await visible(),
      false,
      "suggestion shown in excluded field " + selector,
    );
  }
  await focus("#text");
  await type("Hel");
  await waitFor(visible, "dismiss test");
  const countBefore = uses(records[0][0]);
  await key(page, "Escape");
  assert.equal(await value("#text"), "Hel");
  assert.equal(uses(records[0][0]), countBefore);
  // A delayed old response must never appear after the user has changed input.
  await client.evaluate(
    worker,
    `globalThis.nativeOriginal=native;globalThis.native=async request=>{const value=await nativeOriginal(request);if(request.prefix==='Hel')await new Promise(r=>setTimeout(r,450));return value;}`,
  );
  await focus("#text");
  await type("Hel");
  await new Promise((r) => setTimeout(r, 230));
  await type("p");
  await new Promise((r) => setTimeout(r, 850));
  assert.equal(await visible(), false, "stale suggestion appeared");
  assert.equal(await value("#text"), "Help");
  await client.evaluate(worker, `globalThis.native=nativeOriginal`);
  await client.evaluate(worker, `chrome.storage.local.set({paused:true})`);
  await focus("#text");
  await type("Hel");
  await new Promise((r) => setTimeout(r, 450));
  assert.equal(await visible(), false, "pause ignored");
  await client.evaluate(worker, `chrome.storage.local.set({paused:false})`);
  console.log(
    "PASS Chrome: field exclusions, cancellation, stale-response protection and pause",
  );

  await focus("#text");
  await type("Hel");
  await waitFor(visible, "screenshot");
  const shot = await client.send(
    "Page.captureScreenshot",
    { format: "png", captureBeyondViewport: false },
    page,
  );
  await writeFile(
    join(project, "bin/cliprag-browser-demo.png"),
    Buffer.from(shot.data, "base64"),
  );
  await key(page, "Escape");
  const domainPrefix = origin.replace("http://", "") + "/sav";
  const urlMatches = await client.evaluate(
    worker,
    `new Promise(resolve=>suggestURLs(${JSON.stringify(domainPrefix)},resolve))`,
  );
  assert.equal(urlMatches[0].content, urlClip);
  await client.evaluate(
    worker,
    `acceptURL(${JSON.stringify(domainPrefix)},'currentTab')`,
  );
  await waitFor(() => uses(urlClip) === 2, "URL address-bar acceptance count");
  assert.equal(await client.evaluate(page, "location.href"), urlClip);
  console.log(
    "PASS Chrome: keyword URL suggestions navigate only an issued saved URL and record acceptance",
  );
  // Browser preferences contain no copied text or prefixes.
  const prefs = await client.evaluate(worker, `chrome.storage.local.get(null)`);
  assert.deepEqual(Object.keys(prefs).sort(), ["paused", "sites"]);
  console.log(
    "PASS browser integration: no prefix/history cache or cloud service; normal browser profile untouched",
  );
} finally {
  if (chrome) {
    await client?.send("Browser.close").catch(() => {});
    chrome.kill("SIGTERM");
  }
  await new Promise((done) => server.close(done));
  await rm(work, {
    recursive: true,
    force: true,
    maxRetries: 4,
    retryDelay: 100,
  });
}
