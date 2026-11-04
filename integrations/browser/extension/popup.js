const status = document.getElementById("status");
const button = document.getElementById("site-toggle");
const pause = document.getElementById("pause");
let origin,
  tabId,
  enabledOnSite = false;
function permissionPattern() {
  const url = new URL(origin);
  return url.protocol + "//" + url.hostname + "/*";
}
async function render() {
  const prefs = await chrome.storage.local.get({ sites: [], paused: false });
  pause.checked = prefs.paused;
  enabledOnSite =
    prefs.sites.includes(origin) &&
    (await chrome.permissions.contains({ origins: [permissionPattern()] }));
  button.textContent = enabledOnSite
    ? "Disable on this website"
    : "Enable on this website";
  button.disabled = !origin;
}
async function start() {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  if (tab?.url) {
    const url = new URL(tab.url);
    if (["http:", "https:"].includes(url.protocol)) {
      origin = url.origin;
      tabId = tab.id;
    }
  }
  document.getElementById("site").textContent =
    origin || "Open a website to enable autocomplete.";
  await render();
  if (enabledOnSite) {
    const activated = await chrome.runtime.sendMessage({
      type: "configure",
      tabId,
    });
    if (!activated?.ok)
      throw new Error(activated?.error || "Could not activate this website.");
  }
  const result = await chrome.runtime.sendMessage({ type: "status" });
  status.textContent = result?.ok
    ? result.ready
      ? "Connected · Local clipboard history"
      : "Connected · Open ClipRAG and copy some text"
    : result?.error || "Local bridge unavailable";
  if (result?.ok) document.getElementById("indicator").classList.add("ready");
}
button.addEventListener("click", async () => {
  if (!origin) return;
  button.disabled = true;
  button.textContent = enabledOnSite ? "Disabling…" : "Enabling…";
  try {
    // Ask directly from the click handler, before awaiting any other API.
    if (
      !enabledOnSite &&
      !(await chrome.permissions.request({ origins: [permissionPattern()] }))
    )
      return;
    const prefs = await chrome.storage.local.get({ sites: [] });
    let sites = prefs.sites;
    if (enabledOnSite) {
      sites = sites.filter((site) => site !== origin);
      await chrome.storage.local.set({ sites });
      const pattern = permissionPattern();
      const sameHost = sites.some((site) => {
        const url = new URL(site);
        return url.protocol + "//" + url.hostname + "/*" === pattern;
      });
      if (!sameHost) await chrome.permissions.remove({ origins: [pattern] });
    } else {
      await chrome.storage.local.set({
        sites: [...new Set([...sites, origin])],
      });
    }
    const response = await chrome.runtime.sendMessage({
      type: "configure",
      tabId,
    });
    if (!response?.ok)
      throw new Error(response?.error || "Could not activate this website.");
  } catch (error) {
    status.textContent = error.message;
  } finally {
    await render();
  }
});
pause.addEventListener("change", async () => {
  await chrome.storage.local.set({ paused: pause.checked });
  await chrome.runtime.sendMessage({ type: "configure", tabId });
});
void start().catch((error) => {
  status.textContent = error.message || "ClipRAG unavailable";
});
