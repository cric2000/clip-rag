const HOST = "io.github.cliprag.browser";
let port;
let sequence = 0;
let idle;
const pending = new Map();
let omniboxGeneration = 0;
let omniboxMatches = new Map();
let omniboxPrefix = "";

function native(request) {
  return new Promise((resolve, reject) => {
    if (!port) {
      port = chrome.runtime.connectNative(HOST);
      port.onMessage.addListener((message) => {
        const entry = pending.get(message.requestId);
        if (!entry) return;
        clearTimeout(entry.timeout);
        pending.delete(message.requestId);
        if (message.ok) entry.resolve(message);
        else
          entry.reject(
            new Error(message.error || "ClipRAG lookup unavailable."),
          );
      });
      port.onDisconnect.addListener(() => {
        // Consume lastError without logging clipboard or query data.
        void chrome.runtime.lastError;
        port = undefined;
        for (const entry of pending.values()) {
          clearTimeout(entry.timeout);
          entry.reject(
            new Error(
              "Local bridge unavailable. Run cliprag-browser install for this browser.",
            ),
          );
        }
        pending.clear();
      });
    }
    clearTimeout(idle);
    idle = setTimeout(() => {
      port?.disconnect();
    }, 90000);
    const requestId = String(++sequence);
    const timeout = setTimeout(() => {
      pending.delete(requestId);
      reject(new Error("Local lookup timed out. Try again."));
    }, 2000);
    pending.set(requestId, { resolve, reject, timeout });
    try {
      port.postMessage({ ...request, requestId });
    } catch (error) {
      clearTimeout(timeout);
      pending.delete(requestId);
      reject(error);
    }
  });
}

function originOf(url) {
  try {
    const u = new URL(url);
    return ["http:", "https:"].includes(u.protocol) ? u.origin : null;
  } catch {
    return null;
  }
}
function permissionPattern(origin) {
  const url = new URL(origin);
  return url.protocol + "//" + url.hostname + "/*";
}
async function preferences() {
  const value = await chrome.storage.local.get({ sites: [], paused: false });
  return {
    sites: value.sites.filter((s) => originOf(s) === s),
    paused: !!value.paused,
  };
}
async function allowed(origin) {
  const prefs = await preferences();
  return (
    !!origin &&
    !prefs.paused &&
    prefs.sites.includes(origin) &&
    (await chrome.permissions.contains({
      origins: [permissionPattern(origin)],
    }))
  );
}
async function syncScripts() {
  const prefs = await preferences();
  const registered = await chrome.scripting.getRegisteredContentScripts();
  const own = registered.filter((entry) =>
    entry.id.startsWith("cliprag-site-"),
  );
  if (own.length)
    await chrome.scripting.unregisterContentScripts({
      ids: own.map((entry) => entry.id),
    });
  if (prefs.paused) return;
  let index = 0;
  for (const origin of prefs.sites) {
    if (
      await chrome.permissions.contains({
        origins: [permissionPattern(origin)],
      })
    ) {
      await chrome.scripting.registerContentScripts([
        {
          id: "cliprag-site-" + index++,
          matches: [permissionPattern(origin)],
          js: ["content.js"],
          allFrames: true,
          runAt: "document_idle",
          persistAcrossSessions: true,
        },
      ]);
    }
  }
}
// Serialize registration changes from popup, startup and permission revocation.
let syncing = Promise.resolve();
function synchronize() {
  syncing = syncing.catch(() => {}).then(syncScripts);
  return syncing;
}
async function activateTab(tab) {
  if (tab.incognito || !(await allowed(originOf(tab.url)))) return;
  const results = await chrome.scripting.executeScript({
    target: { tabId: tab.id },
    files: ["content.js"],
  });
  if (!results.some((entry) => entry.frameId === 0 && entry.result?.ready))
    throw new Error(
      "Save your work and refresh this website to activate ClipRAG.",
    );
  // Main-frame activation succeeds even when a foreign frame is inaccessible.
  await chrome.scripting
    .executeScript({
      target: { tabId: tab.id, allFrames: true },
      files: ["content.js"],
    })
    .catch(() => {});
}
async function refreshSites() {
  await synchronize();
  // Recover saved opt-ins even if the popup closed before sending configure.
  const tabs = await chrome.tabs.query({});
  await Promise.allSettled(tabs.map(activateTab));
}
function recoverSites() {
  void refreshSites().catch(() => {});
}
chrome.runtime.onInstalled.addListener(() => {
  recoverSites();
});
chrome.runtime.onStartup.addListener(() => {
  recoverSites();
});
chrome.permissions.onRemoved.addListener(() => {
  recoverSites();
});
chrome.permissions.onAdded.addListener(() => {
  recoverSites();
});
chrome.storage.onChanged.addListener((changes, area) => {
  if (area === "local" && (changes.sites || changes.paused)) recoverSites();
});

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (
    sender.id !== chrome.runtime.id ||
    !message ||
    typeof message.type !== "string"
  )
    return false;
  (async () => {
    const extensionPage =
      !sender.tab && sender.url?.startsWith(chrome.runtime.getURL(""));
    if (extensionPage) {
      if (message.type === "status") return native({ type: "status" });
      if (message.type === "configure") {
        await synchronize();
        if (Number.isInteger(message.tabId)) {
          const tab = await chrome.tabs.get(message.tabId);
          await activateTab(tab);
        }
        return { ok: true };
      }
      throw new Error("Unsupported extension action.");
    }
    const origin = originOf(sender.url);
    if (!sender.tab || sender.tab.incognito || !(await allowed(origin)))
      throw new Error("Enable ClipRAG on this website first.");
    if (message.type === "suggest") {
      if (typeof message.prefix !== "string" || message.prefix.length > 2048)
        throw new Error("Invalid prefix.");
      return native({
        type: "suggest",
        prefix: message.prefix,
        multiline: !!message.multiline,
      });
    }
    if (
      message.type === "accept" &&
      typeof message.token === "string" &&
      /^[a-f0-9]{48}$/.test(message.token)
    ) {
      return native({ type: "accept", token: message.token });
    }
    throw new Error("Unsupported request.");
  })().then(sendResponse, (error) =>
    sendResponse({ ok: false, error: error.message || "ClipRAG unavailable." }),
  );
  return true;
});

function xml(text) {
  return text.replace(
    /[&<>"']/g,
    (c) =>
      ({
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        '"': "&quot;",
        "'": "&apos;",
      })[c],
  );
}
chrome.omnibox.setDefaultSuggestion({
  description: "Search saved ClipRAG URLs — type at least 3 characters",
});
async function suggestURLs(prefix, suggest) {
  const generation = ++omniboxGeneration;
  omniboxMatches.clear();
  omniboxPrefix = prefix;
  if ([...prefix].length < 3 || (await preferences()).paused) {
    if (generation !== omniboxGeneration) return;
    await chrome.omnibox.setDefaultSuggestion({
      description: "Search saved ClipRAG URLs — type at least 3 characters",
    });
    if (generation !== omniboxGeneration) return;
    suggest([]);
    return;
  }
  try {
    const result = await native({ type: "suggest", prefix, urlsOnly: true });
    if (generation !== omniboxGeneration) return;
    const matches = result.matches || [];
    omniboxMatches = new Map(matches.map((m) => [m.text, m.token]));
    await chrome.omnibox.setDefaultSuggestion({
      description: matches.length
        ? "Open <url>" +
          xml(matches[0].text) +
          "</url> <dim>· ClipRAG · Enter to open, ↓ for more</dim>"
        : "No saved URL matches. Try its domain, such as github.com.",
    });
    if (generation !== omniboxGeneration) return;
    suggest(
      matches.map((m) => ({
        content: m.text,
        description: "<url>" + xml(m.text) + "</url> <dim>ClipRAG</dim>",
      })),
    );
  } catch {
    if (generation === omniboxGeneration) {
      chrome.omnibox.setDefaultSuggestion({
        description:
          "Local history unavailable. Check the ClipRAG toolbar popup.",
      });
      suggest([]);
    }
  }
}
chrome.omnibox.onInputChanged.addListener(suggestURLs);
chrome.omnibox.onInputCancelled.addListener(() => {
  ++omniboxGeneration;
  omniboxMatches.clear();
  omniboxPrefix = "";
});
async function acceptURL(text, disposition) {
  if (!omniboxMatches.has(text) && text === omniboxPrefix)
    text = omniboxMatches.keys().next().value;
  const token = omniboxMatches.get(text);
  omniboxMatches.clear();
  omniboxPrefix = "";
  ++omniboxGeneration;
  if (!token || (await preferences()).paused) return;
  let url;
  try {
    url = new URL(text);
  } catch {
    return;
  }
  if (
    !["http:", "https:"].includes(url.protocol) ||
    url.username ||
    url.password
  )
    return;
  if (disposition === "currentTab") {
    const [tab] = await chrome.tabs.query({
      active: true,
      currentWindow: true,
    });
    if (tab) await chrome.tabs.update(tab.id, { url: text });
  } else
    await chrome.tabs.create({
      url: text,
      active: disposition !== "newBackgroundTab",
    });
  void native({ type: "accept", token }).catch(() => {});
}
chrome.omnibox.onInputEntered.addListener(acceptURL);
