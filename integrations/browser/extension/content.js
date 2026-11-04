(() => {
  if (typeof globalThis.__clipragBrowserLoaded === "function")
    return globalThis.__clipragBrowserLoaded();
  if (globalThis.__clipragBrowserLoaded) return;
  globalThis.__clipragBrowserLoaded = updateEnabled;
  let enabled = false,
    composing = false,
    accepting = false;
  let timer,
    generation = 0,
    current,
    items = [],
    selected = 0;
  let host, root, panel, list, footer;
  let preferenceGeneration = 0,
    preferenceRead;
  const sensitive =
    /password|passwd|secret|token|one.?time|otp|credit.?card|card.?number|card.?code|cc-(?:number|csc|exp)|cvv|cvc|social.?security|\bssn\b/i;

  function updateEnabled() {
    const ticket = ++preferenceGeneration;
    preferenceRead = chrome.storage.local
      .get({ sites: [], paused: false })
      .then((prefs) => {
        // Concurrent injections and preference events must all wait for the
        // newest read, rather than allowing an older disabled state to win.
        if (ticket !== preferenceGeneration) return preferenceRead;
        enabled = !prefs.paused && prefs.sites.includes(location.origin);
        if (!enabled) hide();
        return { ready: true, enabled };
      });
    return preferenceRead;
  }
  chrome.storage.onChanged.addListener((changes, area) => {
    if (area === "local" && (changes.sites || changes.paused))
      void updateEnabled().catch(() => {
        enabled = false;
        hide();
      });
  });

  function editable(element) {
    if (!(element instanceof HTMLElement)) return null;
    const field = element.matches("input,textarea")
      ? element
      : element.closest("[contenteditable]");
    // An explicit editable descendant overrides a non-editable ancestor.
    // Gmail and other rich editors use this nesting for their compose UI.
    if (
      field &&
      !field.matches("input,textarea") &&
      (!field.isContentEditable ||
        field.getAttribute("contenteditable") === "false")
    )
      return null;
    if (
      !field ||
      field.disabled ||
      field.readOnly ||
      field.getAttribute("aria-disabled") === "true"
    )
      return null;
    if (
      field instanceof HTMLInputElement &&
      !["text", "search", "url", "email", "tel"].includes(field.type)
    )
      return null;
    const metadata = ["type", "name", "id", "autocomplete", "aria-label"]
      .map((name) => field.getAttribute(name) || "")
      .join(" ");
    if (sensitive.test(metadata)) return null;
    return field;
  }
  function activeField() {
    let active = document.activeElement;
    while (active?.shadowRoot?.activeElement)
      active = active.shadowRoot.activeElement;
    return editable(active);
  }
  function richContext(field, range) {
    const segments = [];
    const pieces = [];
    let length = 0,
      cursor = null,
      visited = 0,
      exceeded = false;
    const blocks = new Set([
      "DIV",
      "P",
      "LI",
      "PRE",
      "BLOCKQUOTE",
      "H1",
      "H2",
      "H3",
      "H4",
      "H5",
      "H6",
      "TR",
      "TD",
      "TH",
      "SECTION",
      "ARTICLE",
    ]);
    let last = "";
    function append(text) {
      if (!text) return;
      pieces.push(text);
      length += text.length;
      last = text.at(-1);
      if (length > 256 * 1024) exceeded = true;
    }
    function boundary() {
      if (length && last !== "\n") append("\n");
    }
    function walk(node, outer = false) {
      if (exceeded || ++visited > 12000) {
        exceeded = true;
        return;
      }
      if (node.nodeType === Node.TEXT_NODE) {
        segments.push({ node, start: length, length: node.length });
        if (node === range.endContainer) cursor = length + range.endOffset;
        append(node.data);
        return;
      }
      if (!(node instanceof Element)) return;
      if (!outer && node.getAttribute("contenteditable") === "false") {
        boundary();
        return;
      }
      if (["SCRIPT", "STYLE", "TEMPLATE", "NOSCRIPT"].includes(node.tagName))
        return;
      const block = !outer && blocks.has(node.tagName);
      if (block) boundary();
      if (node.tagName === "BR") {
        append("\n");
        return;
      }
      for (let i = 0; i <= node.childNodes.length; i++) {
        if (node === range.endContainer && i === range.endOffset)
          cursor = length;
        if (i < node.childNodes.length) walk(node.childNodes[i]);
        if (exceeded) return;
      }
      if (block) boundary();
    }
    walk(field, true);
    if (exceeded || cursor === null) return null;
    const text = pieces.join("");
    // Text on a later line (including Gmail signatures and quoted messages)
    // does not prevent suggestions at the end of the current line.
    if (text.slice(cursor).split("\n", 1)[0].trim()) return null;
    return {
      before: text.slice(0, cursor),
      signature: text + "\0" + cursor,
      cursor,
      segments,
    };
  }
  function context(field) {
    if (!editable(field)) return null;
    let before, signature, range, cursor, segments;
    const plain =
      field instanceof HTMLInputElement || field instanceof HTMLTextAreaElement;
    if (plain) {
      cursor = field.selectionStart ?? field.value.length;
      if (
        field.selectionEnd !== null &&
        field.selectionStart !== field.selectionEnd
      )
        return null;
      if (cursor < field.value.length && field.value[cursor] !== "\n")
        return null;
      before = field.value.slice(0, cursor);
      signature = field.value + "\0" + cursor;
    } else {
      const selection = window.getSelection();
      if (
        !selection?.isCollapsed ||
        !selection.rangeCount ||
        !field.contains(selection.anchorNode)
      )
        return null;
      range = selection.getRangeAt(0).cloneRange();
      const rich = richContext(field, range);
      if (!rich) return null;
      ({ before, signature, cursor, segments } = rich);
    }
    const line = before.slice(before.lastIndexOf("\n") + 1);
    if (line.length > 2048 || [...line].length < 3 || !line.trim()) return null;
    return {
      field,
      plain,
      multiline: !(field instanceof HTMLInputElement),
      before,
      prefix: line,
      signature,
      cursor,
      range,
      segments,
    };
  }
  function hide() {
    clearTimeout(timer);
    ++generation;
    items = [];
    current = undefined;
    if (host) host.hidden = true;
  }
  function createPanel() {
    host = document.createElement("div");
    host.style.cssText =
      "all:initial!important;position:fixed!important;z-index:2147483647!important;pointer-events:none!important;";
    root = host.attachShadow({ mode: "closed" });
    const style = document.createElement("style");
    style.textContent = `:host([hidden]){display:none!important}*{box-sizing:border-box}.panel{pointer-events:auto;background:#19161f;color:#f1edf7;border:1px solid #75618e;border-radius:12px;box-shadow:0 12px 38px #0006;font:14px/1.45 system-ui,sans-serif;overflow:hidden;width:min(440px,calc(100vw - 24px))}.heading{padding:10px 13px;color:#c4a9e9;font-size:12px;font-weight:650;display:flex;justify-content:space-between}.list{max-height:250px;overflow:auto;padding:0 5px}.option{display:block;width:100%;border:0;text-align:left;background:none;color:inherit;padding:9px 10px;border-radius:7px;font:inherit;cursor:pointer}.option[aria-selected=true]{background:#3c2d50;outline:1px solid #8f6bb6;outline-offset:-1px}.category{color:#c5b3d7;font-size:11px;display:block;margin-bottom:3px}.preview{white-space:pre-wrap;overflow-wrap:anywhere;display:-webkit-box;-webkit-line-clamp:3;-webkit-box-orient:vertical;overflow:hidden}.footer{padding:9px 13px;color:#b1a7bf;font-size:11px}`;
    panel = document.createElement("div");
    panel.className = "panel";
    const heading = document.createElement("div");
    heading.className = "heading";
    heading.textContent = "ClipRAG · Saved on this device";
    list = document.createElement("div");
    list.className = "list";
    list.setAttribute("role", "listbox");
    list.setAttribute("aria-label", "Saved clipboard suggestions");
    footer = document.createElement("div");
    footer.className = "footer";
    footer.setAttribute("aria-live", "polite");
    panel.append(heading, list, footer);
    root.append(style, panel);
    document.documentElement.append(host);
    panel.addEventListener("pointerdown", (event) => {
      event.preventDefault();
    });
    panel.addEventListener("click", (event) => {
      if (!event.isTrusted) return;
      const button = event.target.closest("button[data-index]");
      if (button) {
        selected = Number(button.dataset.index);
        accept();
      }
    });
  }
  function position() {
    if (!host || host.hidden || !current) return;
    let rect = current.range?.getBoundingClientRect();
    if (!rect?.height) rect = current.field.getBoundingClientRect();
    const width = panel.getBoundingClientRect().width;
    const height = panel.getBoundingClientRect().height;
    host.style.setProperty(
      "left",
      Math.max(12, Math.min(rect.left, innerWidth - width - 12)) + "px",
      "important",
    );
    const top =
      rect.bottom + 7 + height < innerHeight
        ? rect.bottom + 7
        : Math.max(8, rect.top - height - 7);
    host.style.setProperty("top", top + "px", "important");
  }
  function render() {
    if (!host) createPanel();
    list.replaceChildren();
    items.forEach((match, index) => {
      const option = document.createElement("button");
      option.type = "button";
      option.className = "option";
      option.dataset.index = String(index);
      option.setAttribute("role", "option");
      option.setAttribute("aria-selected", String(index === selected));
      const category = document.createElement("span");
      category.className = "category";
      category.textContent = match.category;
      const preview = document.createElement("span");
      preview.className = "preview";
      preview.textContent = match.text.slice(0, 260);
      option.append(category, preview);
      list.append(option);
    });
    footer.textContent = `Match ${selected + 1} of ${items.length} · ↑↓ choose · Tab / Enter insert · Esc close`;
    host.hidden = false;
    position();
    list.children[selected]?.scrollIntoView({
      block: "nearest",
      inline: "nearest",
    });
  }
  async function lookup(field, ticket) {
    let ctx = context(field);
    if (!ctx || !enabled || composing || activeField() !== field) return;
    try {
      let response = await chrome.runtime.sendMessage({
        type: "suggest",
        prefix: ctx.prefix,
        multiline: ctx.multiline,
      });
      if (!response?.ok) {
        if (ticket === generation) hide();
        return;
      }
      if (!response.matches?.length) {
        const word = ctx.prefix.match(/\S+$/)?.[0];
        if (word && word !== ctx.prefix && [...word].length >= 3) {
          ctx = { ...ctx, prefix: word };
          response = await chrome.runtime.sendMessage({
            type: "suggest",
            prefix: word,
            multiline: ctx.multiline,
          });
        }
      }
      if (
        ticket !== generation ||
        !enabled ||
        activeField() !== field ||
        context(field)?.signature !== ctx.signature
      )
        return;
      const matches = (response.matches || []).filter(
        (match) =>
          typeof match.text === "string" &&
          match.text.toLowerCase().startsWith(ctx.prefix.toLowerCase()) &&
          typeof match.token === "string",
      );
      items = matches.filter(
        (match) =>
          !ctx.plain ||
          field.maxLength < 0 ||
          field.value.length - ctx.prefix.length + match.text.length <=
            field.maxLength,
      );
      if (!items.length) {
        hide();
        return;
      }
      current = { ...context(field), prefix: ctx.prefix };
      selected = 0;
      render();
    } catch {
      if (ticket === generation) hide();
    }
  }
  function richRange(ctx) {
    const result = ctx.range.cloneRange();
    const start = ctx.cursor - ctx.prefix.length;
    for (const segment of ctx.segments) {
      if (start >= segment.start && start <= segment.start + segment.length) {
        result.setStart(segment.node, start - segment.start);
        return result;
      }
    }
    return null;
  }
  function accept() {
    const match = items[selected],
      previous = current;
    const fresh = previous && context(previous.field);
    const ctx =
      fresh && previous ? { ...fresh, prefix: previous.prefix } : null;
    if (
      !enabled ||
      !match ||
      !ctx ||
      activeField() !== ctx.field ||
      ctx.signature !== previous.signature
    ) {
      hide();
      return;
    }
    const field = ctx.field;
    accepting = true;
    let inserted = false;
    try {
      if (ctx.plain) {
        if (typeof field.selectionStart === "number")
          field.setSelectionRange(ctx.cursor - ctx.prefix.length, ctx.cursor);
        else field.select();
      } else {
        const range = richRange(ctx);
        if (!range) return;
        const selection = window.getSelection();
        selection.removeAllRanges();
        selection.addRange(range);
      }
      // Native editing preserves undo and lets controlled website inputs receive
      // the browser's normal input event. Text is never parsed as HTML.
      inserted = document.execCommand("insertText", false, match.text);
      if (!inserted) {
        const before = new InputEvent("beforeinput", {
          bubbles: true,
          cancelable: true,
          inputType: "insertReplacementText",
          data: match.text,
        });
        if (!field.dispatchEvent(before)) return;
        if (ctx.plain) {
          const value =
            field.value.slice(0, ctx.cursor - ctx.prefix.length) +
            match.text +
            field.value.slice(ctx.cursor);
          const prototype =
            field instanceof HTMLTextAreaElement
              ? HTMLTextAreaElement.prototype
              : HTMLInputElement.prototype;
          Object.getOwnPropertyDescriptor(prototype, "value").set.call(
            field,
            value,
          );
          if (typeof field.selectionStart === "number")
            field.setSelectionRange(
              ctx.cursor - ctx.prefix.length + match.text.length,
              ctx.cursor - ctx.prefix.length + match.text.length,
            );
        } else {
          const range = window.getSelection().getRangeAt(0);
          range.deleteContents();
          const text = document.createTextNode(match.text);
          range.insertNode(text);
          range.setStartAfter(text);
          range.collapse(true);
          const selection = window.getSelection();
          selection.removeAllRanges();
          selection.addRange(range);
        }
        field.dispatchEvent(
          new InputEvent("input", {
            bubbles: true,
            inputType: "insertReplacementText",
            data: match.text,
          }),
        );
        inserted = true;
      }
    } finally {
      accepting = false;
      hide();
    }
    if (inserted)
      void chrome.runtime
        .sendMessage({ type: "accept", token: match.token })
        .catch(() => {});
  }
  document.addEventListener(
    "input",
    (event) => {
      if (!event.isTrusted || accepting || !enabled || composing) return;
      const field = editable(event.composedPath()[0]);
      hide();
      if (!field) return;
      const ticket = generation;
      timer = setTimeout(() => {
        void lookup(field, ticket);
      }, 170);
    },
    true,
  );
  document.addEventListener(
    "keydown",
    (event) => {
      if (
        !event.isTrusted ||
        composing ||
        !current ||
        !items.length ||
        !enabled
      )
        return;
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopImmediatePropagation();
        hide();
        return;
      }
      if (["ArrowDown", "ArrowUp"].includes(event.key)) {
        event.preventDefault();
        event.stopImmediatePropagation();
        selected =
          (selected + (event.key === "ArrowDown" ? 1 : -1) + items.length) %
          items.length;
        render();
        return;
      }
      if (
        ["Tab", "Enter", "ArrowRight"].includes(event.key) &&
        !event.shiftKey &&
        !event.ctrlKey &&
        !event.metaKey &&
        !event.altKey
      ) {
        event.preventDefault();
        event.stopImmediatePropagation();
        accept();
        return;
      }
      if (
        ["ArrowLeft", "Home", "End", "PageUp", "PageDown"].includes(event.key)
      )
        hide();
    },
    true,
  );
  document.addEventListener(
    "focusin",
    () => {
      if (!accepting) hide();
    },
    true,
  );
  document.addEventListener(
    "pointerdown",
    (event) => {
      if (event.composedPath()[0] !== host) hide();
    },
    true,
  );
  document.addEventListener(
    "compositionstart",
    () => {
      composing = true;
      hide();
    },
    true,
  );
  document.addEventListener(
    "compositionend",
    (event) => {
      composing = false;
      if (!event.isTrusted || !enabled) return;
      const field = activeField(),
        ticket = generation;
      if (field)
        timer = setTimeout(() => {
          void lookup(field, ticket);
        }, 170);
    },
    true,
  );
  addEventListener("blur", hide);
  addEventListener("resize", position);
  addEventListener("scroll", position, true);
  // executeScript waits for this promise; the popup only confirms activation
  // once this document has read its current preferences.
  return updateEnabled();
})();
