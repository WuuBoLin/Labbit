// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

(() => {
  if (window.__labbitInitialized) return;
  window.__labbitInitialized = true;

  const qs = (selector, root = document) => root.querySelector(selector);
  const qsa = (selector, root = document) => Array.from(root.querySelectorAll(selector));
  const clamp = (value, min, max) => Math.max(min, Math.min(max, value));
  const shell = () => qs(".labbit-shell");
  const minRem = 13.75, maxRem = 32.5, snapRem = 11.25, defaultRem = 18.75;

  const rem = (px) => px / (parseFloat(getComputedStyle(document.documentElement).fontSize) || 16);
  const mobile = () => !matchMedia("(min-width: 64rem)").matches;
  const typing = (el) => el?.matches?.("input, textarea, select, [contenteditable='true']");
  const prevent = (event, action) => (event.preventDefault(), action?.());
  const popoverOpen = (el) => el?.matches?.(":popover-open");
  const panel = (name) => qs(`[data-${name}-modal]`);
  const selectedIndexBox = () => qs("[data-search-results]");
  const searchItems = () => qsa("[data-search-result]");
  const accountLongPressMs = 1234;
  let accountLongPress = null;

  const b64ToBuffer = (value) => {
    const base64 = String(value).replace(/-/g, "+").replace(/_/g, "/");
    return Uint8Array.from(atob(base64 + "=".repeat((4 - (base64.length % 4)) % 4)), (c) => c.charCodeAt(0)).buffer;
  };

  const bufferToB64 = (value) =>
    btoa(Array.from(new Uint8Array(value || new ArrayBuffer(0)), (byte) => String.fromCharCode(byte)).join(""))
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=+$/g, "");

  function openPanel(name) {
    const el = panel(name);
    if (el && !popoverOpen(el)) el.showPopover?.();
    if (name === "search") focusSearch();
  }

  function closePanel(name) {
    const el = panel(name);
    if (popoverOpen(el)) el.hidePopover?.();
  }

  function focusSearch() {
    const input = qs("[data-search-input]", panel("search"));
    selectSearch(0);
    input?.focus();
    input?.select();
  }

  function selectSearch(index) {
    const items = searchItems();
    const box = selectedIndexBox();
    qsa(".search-result.active").forEach((el) => el.classList.remove("active"));
    if (box) box.dataset.selectedIndex = "-1";
    if (!items.length) return;
    const next = (index + items.length) % items.length;
    if (box) box.dataset.selectedIndex = String(next);
    items[next].classList.add("active");
    items[next].scrollIntoView({ block: "nearest" });
  }

  function moveSearch(delta) {
    const current = Number(selectedIndexBox()?.dataset.selectedIndex);
    selectSearch(
      Number.isFinite(current) && current >= 0 ? current + delta : 0,
    );
  }

  function activeSearch() {
    const items = searchItems();
    const index = Number(selectedIndexBox()?.dataset.selectedIndex);
    return items[index] || items[0];
  }

  function selectBlock(block, push = true, scroll = false) {
    if (!block) return;
    qsa("[data-action-block].selected").forEach((el) => el.classList.remove("selected"));
    block.classList.add("selected");
    if (push && block.dataset.blockUrl) history.pushState(null, "", block.dataset.blockUrl);
    if (scroll) block.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  const blocks = () => qsa("[data-action-block]").filter((el) => el.offsetParent !== null);

  function currentBlockIndex(items) {
    const selected = qs("[data-action-block].selected");
    if (selected) return Math.max(0, items.indexOf(selected));
    const point = scrollY + innerHeight * 0.3;
    let current = 0;
    items.forEach((el, index) => {
      if (el.getBoundingClientRect().top + scrollY <= point) current = index;
    });
    return current;
  }

  function moveBlock(delta) {
    const items = blocks();
    if (!items.length) return;
    const next = clamp(currentBlockIndex(items) + delta, 0, items.length - 1);
    const block = items[next];
    const link = qs("[data-block-link]", block);
    if (link) {
      markPending(link);
      link.click();
      return;
    }
    selectBlock(block, true, true);
  }

  function moveSection(delta) {
    const links = qsa(".nav-link[data-section-id]");
    const current =
      qs(".nav-link.active")?.dataset.sectionId ||
      qs("#content [data-section]")?.dataset.section ||
      "overview";
    const index = Math.max(0, links.findIndex((el) => el.dataset.sectionId === current));
    const link = links[clamp(index + delta, 0, links.length - 1)];
    if (link) {
      markPending(link);
      link.click();
    }
  }

  function actOnBlock() {
    const items = blocks();
    const block =
      qs("[data-action-block].selected") || items[currentBlockIndex(items)];
    if (!block) return;
    const solution = qs("[data-solution-toggle]", block);
    if (solution) return solution.click();
    const inline = qs("[data-inline-hint-toggle]", block);
    const quiz = qs("[data-quiz-submit]:not(:disabled)", block);
    (inline || quiz)?.click();
  }

  function updateQuiz(form) {
    const button = qs("[data-quiz-submit]", form);
    if (button) button.disabled = !qsa("[data-quiz-option]", form).some((el) => el.checked);
  }

  function applySidebar() {
    const root = shell();
    if (!root) return;
    const width = clamp(Number(localStorage.getItem("labbit.sidebar.width.rem")) || defaultRem, minRem, maxRem);
    root.style.setProperty("--sidebar-expanded-width", `${width}rem`);
    root.classList.toggle(
      "sidebar-collapsed",
      mobile()
        ? root.dataset.mobileSidebarOpen !== "true"
        : localStorage.getItem("labbit.sidebar.collapsed") === "true",
    );
    root.style.setProperty(
      "--sidebar-width",
      root.classList.contains("sidebar-collapsed")
        ? "var(--sidebar-rail-width)"
        : `${width}rem`,
    );
  }

  function toggleSidebar(trigger) {
    const root = shell();
    if (!root) return;
    if (mobile()) {
      root.dataset.mobileSidebarOpen = root.classList.contains(
        "sidebar-collapsed",
      )
        ? "true"
        : "false";
      trigger?.blur?.();
      applySidebar();
      return;
    }
    root.classList.toggle("sidebar-collapsed");
    localStorage.setItem(
      "labbit.sidebar.collapsed",
      String(root.classList.contains("sidebar-collapsed")),
    );
    trigger?.blur?.();
    applySidebar();
  }

  function closeMobileSidebar() {
    const root = shell();
    if (!root || !mobile()) return;
    root.dataset.mobileSidebarOpen = "false";
    applySidebar();
  }

  function resizeSidebar(event, handle) {
    const root = shell();
    if (!root) return;
    event.preventDefault();
    handle?.setPointerCapture?.(event.pointerId);
    root.classList.remove("sidebar-collapsed");
    localStorage.setItem("labbit.sidebar.collapsed", "false");

    const move = (e) => {
      const width = rem(e.clientX);
      if (width < snapRem) {
        root.classList.add("sidebar-collapsed");
        localStorage.setItem("labbit.sidebar.collapsed", "true");
        return;
      }
      const clamped = clamp(width, minRem, maxRem);
      root.classList.remove("sidebar-collapsed");
      root.style.setProperty("--sidebar-width", `${clamped}rem`);
      root.style.setProperty("--sidebar-expanded-width", `${clamped}rem`);
      localStorage.setItem("labbit.sidebar.width.rem", String(clamped));
    };

    const up = () => {
      handle?.removeEventListener("pointermove", move);
      handle?.removeEventListener("pointerup", up);
      handle?.removeEventListener("pointercancel", up);
    };

    move(event);
    handle?.addEventListener("pointermove", move);
    handle?.addEventListener("pointerup", up, { once: true });
    handle?.addEventListener("pointercancel", up, { once: true });
  }

  function markPending(el) {
    const root = shell();
    if (root)
      root.dataset.pendingScrollTarget =
        el.dataset.scrollTarget || el.dataset.shareTarget || "";
  }

  function setIDStatus(panel, text) {
    const status = qs("[data-id-status]", panel);
    if (status) status.textContent = text || "";
  }

  function setPasskeyBusy(panel, busy) {
    if (!panel) return;
    panel.dataset.passkeyBusy = busy ? "true" : "false";
    qsa("[data-passkey-signin], [data-passkey-register]", panel).forEach((button) => (button.disabled = busy));
    if (!busy) setIDStatus(panel, "");
  }

  function clearAccountLongPress() {
    clearTimeout(accountLongPress?.timer);
    accountLongPress = null;
  }

  function startAccountLongPress(event) {
    const chip = event.target.closest("[data-account-chip]");
    if (!chip || event.button > 0) return;
    clearAccountLongPress();
    accountLongPress = { chip };
    accountLongPress.timer = setTimeout(() => {
      accountLongPress = null;
      location.assign("/id");
    }, accountLongPressMs);
  }

  function trackAccountLongPress(event) {
    const chip = accountLongPress?.chip;
    if (!chip) return;
    const { left, right, top, bottom } = chip.getBoundingClientRect();
    if (
      event.clientX < left ||
      event.clientX > right ||
      event.clientY < top ||
      event.clientY > bottom
    ) clearAccountLongPress();
  }

  function passkeyFailureMessage(error, mode) {
    if (["AbortError", "NotAllowedError"].includes(error?.name)) {
      return "Passkey prompt was cancelled.";
    }
    return mode === "register"
      ? "Passkey could not be created."
      : "Passkey sign-in failed.";
  }

  function decodeCredentialOptions(options) {
    const publicKey = options.publicKey;
    publicKey.challenge = b64ToBuffer(publicKey.challenge);
    if (publicKey.user?.id) publicKey.user.id = b64ToBuffer(publicKey.user.id);
    (publicKey.excludeCredentials || []).forEach((credential) => (credential.id = b64ToBuffer(credential.id)));
    (publicKey.allowCredentials || []).forEach((credential) => (credential.id = b64ToBuffer(credential.id)));
    return publicKey;
  }

  function encodeCredential(credential) {
    const response = credential.response;
    const out = {
      id: credential.id,
      type: credential.type,
      rawId: bufferToB64(credential.rawId),
      response: {
        clientDataJSON: bufferToB64(response.clientDataJSON),
      },
      clientExtensionResults: credential.getClientExtensionResults?.() || {},
      authenticatorAttachment: credential.authenticatorAttachment || undefined,
    };
    ["attestationObject", "authenticatorData", "signature", "userHandle"].forEach((key) => {
      if (response[key]) out.response[key] = bufferToB64(response[key]);
    });
    return out;
  }

  function idEndpoint(path, params) {
    return `${path}?${new URLSearchParams(params)}`;
  }

  async function copyText(button) {
    const originalHTML = button.innerHTML;
    let label = "Copied";
    try {
      const code = button.dataset.copyUrl
        ? await fetch(button.dataset.copyUrl).then((response) => response.ok ? response.text() : "")
        : button.closest(".code-shell")?.querySelector("[data-code]")?.dataset.code;
      if (!code) return;
      await navigator.clipboard.writeText(code);
    } catch {
      label = "Copy failed";
    }
    button.textContent = label;
    setTimeout(() => (button.innerHTML = originalHTML), 900);
  }

  async function passkey(panel, mode) {
    if (!window.PublicKeyCredential)
      return setIDStatus(panel, "Passkeys are not available in this browser.");
    if (!panel || panel.dataset.passkeyBusy === "true") return;
    setPasskeyBusy(panel, true);
    const next = panel.dataset.next || "/";
    const endpoint = mode === "register" ? "/id/register" : "/id/authenticate";

    try {
      setIDStatus(panel, "Check your browser prompt.");
      const begin = await fetch(idEndpoint(endpoint, { step: "begin", next }), {
        method: "POST",
        credentials: "same-origin",
      });
      if (!begin.ok) {
        setIDStatus(panel, "Passkey setup failed.");
        return;
      }

      const payload = await begin.json();
      const publicKey = decodeCredentialOptions(payload.options);
      const credential =
        mode === "register"
          ? await navigator.credentials.create({ publicKey })
          : await navigator.credentials.get({ publicKey });
      if (!credential) {
        setIDStatus(panel, "Passkey prompt was cancelled.");
        return;
      }

      const finish = await fetch(
        idEndpoint(endpoint, { step: "finish", state: payload.state, next }),
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          credentials: "same-origin",
          body: JSON.stringify(encodeCredential(credential)),
        },
      );
      if (!finish.ok) {
        setIDStatus(
          panel,
          mode === "register"
            ? "Passkey could not be created."
            : "Passkey sign-in failed.",
        );
        return;
      }

      const result = await finish.json();
      location.assign(result.next || "/");
    } catch (error) {
      setIDStatus(panel, passkeyFailureMessage(error, mode));
    } finally {
      setPasskeyBusy(panel, false);
    }
  }

  document.addEventListener("keydown", (event) => {
    const key = event.key.toLowerCase();

    if (popoverOpen(panel("search"))) {
      if ((event.ctrlKey || event.metaKey) && key === "j")
        return prevent(event, () => moveSearch(1));
      if ((event.ctrlKey || event.metaKey) && key === "k")
        return prevent(event, () => moveSearch(-1));
      if (event.key === "ArrowDown")
        return prevent(event, () => moveSearch(1));
      if (event.key === "ArrowUp")
        return prevent(event, () => moveSearch(-1));
      if (event.key === "Enter")
        return prevent(event, () => activeSearch()?.click());
    }

    if ((event.ctrlKey || event.metaKey) && key === "k")
      return prevent(event, () => openPanel("search"));
    if (event.key === "Escape")
      return (closePanel("keybindings"), closePanel("search"));
    if (typing(event.target) || event.ctrlKey || event.metaKey || event.altKey)
      return;
    if (event.key === "?")
      return prevent(event, () => openPanel("keybindings"));
    if (event.key === "A") return prevent(event, actOnBlock);
    if (event.key === "J") return prevent(event, () => moveSection(1));
    if (event.key === "H" || event.key === "K")
      return prevent(event, () => moveSection(-1));
    if (event.key === "j") return prevent(event, () => moveBlock(1));
    if (event.key === "h" || event.key === "k")
      return prevent(event, () => moveBlock(-1));
  });

  document.addEventListener("click", (event) => {
    const passkeySignin = event.target.closest("[data-passkey-signin]");
    if (passkeySignin)
      return (
        event.preventDefault(),
        passkey(passkeySignin.closest("[data-id-panel]"), "signin")
      );
    const passkeyRegister = event.target.closest("[data-passkey-register]");
    if (passkeyRegister)
      return (
        event.preventDefault(),
        passkey(passkeyRegister.closest("[data-id-panel]"), "register")
      );

    if (event.target.matches?.("[data-search-modal]")) return closePanel("search");
    if (event.target.matches?.("[data-keybindings-modal]"))
      return closePanel("keybindings");

    const sidebar = event.target.closest("[data-toggle-sidebar]");
    if (sidebar) return toggleSidebar(sidebar);

    const hxLink = event.target.closest(
      "[hx-get][data-section-id], [hx-get][data-share-target], [data-search-result]",
    );
    if (hxLink) {
      markPending(hxLink);
      closeMobileSidebar();
    }
    if (event.target.closest("[data-close-search]")) closePanel("search");

    const selected = event.target.closest(
      "[data-select-block], [data-inline-hint-toggle], [data-solution-toggle]",
    );
    if (selected) selectBlock(selected.closest("[data-action-block]"));

    const share = event.target.closest(
      "[data-share-target]:not([hx-get]):not([data-solution-toggle])",
    );
    if (share) {
      const target = document.getElementById(share.dataset.shareTarget);
      if (target?.matches?.("[data-action-block]")) {
        event.preventDefault();
        selectBlock(target);
      }
    }

    const copy = event.target.closest("[data-copy]");
    if (copy) copyText(copy);
  });

  document.addEventListener("pointerdown", (event) => {
    const resizer = event.target.closest("[data-sidebar-resizer]");
    if (resizer) resizeSidebar(event, resizer);
  });

  document.addEventListener("pointerdown", startAccountLongPress);
  document.addEventListener("pointermove", trackAccountLongPress);
  document.addEventListener("pointerup", clearAccountLongPress);
  document.addEventListener("pointercancel", clearAccountLongPress);
  document.addEventListener("pointerleave", clearAccountLongPress);
  document.addEventListener("contextmenu", clearAccountLongPress);

  document.addEventListener("change", (event) => {
    const form = event.target.closest("[data-quiz-card]");
    if (form) updateQuiz(form);
  });

  document.addEventListener(
    "toggle",
    (event) => {
      if (
        event.target.matches?.("[data-search-modal]") &&
        event.target.matches(":popover-open")
      ) {
        focusSearch();
      }
    },
    true,
  );

  document.body.addEventListener("htmx:after:swap", (event) => {
    const target = event.detail?.target || event.detail?.ctx?.target || event.target;
    if (target?.tagName === "BODY") applySidebar();
    if (target?.matches?.("[data-search-results]")) return selectSearch(0);
    qsa("[data-quiz-card]").forEach(updateQuiz);
    if (target?.id === "content") {
      setTimeout(() => {
        const root = shell();
        const pending = root?.dataset.pendingScrollTarget || "";
        const scrollTarget =
          document.getElementById(pending) || qs("[data-section]", target);
        if (root) delete root.dataset.pendingScrollTarget;
        scrollTarget?.scrollIntoView({ behavior: "smooth", block: "start" });
      }, 0);
    }
  });

  document.body.addEventListener("labbitThemeChanged", (event) => {
    document.documentElement.dataset.theme =
      event.detail?.theme === "light" ? "light" : "dark";
  });

  document.addEventListener("DOMContentLoaded", () => {
    applySidebar();
    qsa("[data-quiz-card]").forEach(updateQuiz);
    qs("[data-action-block].selected")?.scrollIntoView({ block: "start" });
  });
})();
