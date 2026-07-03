(() => {
  if (window.__labbitInitialized) return;
  window.__labbitInitialized = true;

  const qs = (s, r = document) => r.querySelector(s);
  const qsa = (s, r = document) => Array.from(r.querySelectorAll(s));
  const minRem = 13.75, maxRem = 32.5, snapRem = 11.25, defaultRem = 18.75;
  const shell = () => qs(".labbit-shell");
  const searchItems = () => qsa("[data-search-result]");
  const searchOpen = () => !qs("[data-search-modal]")?.classList.contains("hidden");
  const rem = (px) => px / (parseFloat(getComputedStyle(document.documentElement).fontSize) || 16);
  const typing = (el) => el?.matches?.("input, textarea, select, [contenteditable='true']");

  function modal(name, open) {
    const el = qs(`[data-${name}-modal]`);
    if (!el) return;
    el.classList.toggle("hidden", !open);
    if (open && name === "search") {
      const input = qs("[data-search-input]", el);
      selectSearch(0);
      input?.focus();
      input?.select();
    }
  }

  function selectSearch(index) {
    const items = searchItems();
    const box = qs("[data-search-results]");
    qsa(".search-result.active").forEach((el) => el.classList.remove("active"));
    if (box) box.dataset.selectedIndex = "-1";
    if (!items.length) return;
    const next = (index + items.length) % items.length;
    if (box) box.dataset.selectedIndex = String(next);
    items[next].classList.add("active");
    items[next].scrollIntoView({ block: "nearest" });
  }

  function moveSearch(delta) {
    const current = Number(qs("[data-search-results]")?.dataset.selectedIndex);
    selectSearch(Number.isFinite(current) && current >= 0 ? current + delta : 0);
  }

  function activeSearch() {
    const items = searchItems(), index = Number(qs("[data-search-results]")?.dataset.selectedIndex);
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
    items.forEach((el, i) => {
      if (el.getBoundingClientRect().top + scrollY <= point) current = i;
    });
    return current;
  }

  function moveBlock(delta) {
    const items = blocks();
    if (!items.length) return;
    const next = Math.max(0, Math.min(items.length - 1, currentBlockIndex(items) + delta));
    const block = items[next];
    const link = qs("[data-block-link]", block);
    if (link) {
      markPending(link);
      link.click();
    } else {
      selectBlock(block, true, true);
    }
  }

  function moveSection(delta) {
    const links = qsa(".nav-link[data-section-id]");
    const current = qs(".nav-link.active")?.dataset.sectionId || qs("#content [data-section]")?.dataset.section || "overview";
    const index = Math.max(0, links.findIndex((el) => el.dataset.sectionId === current));
    const link = links[Math.max(0, Math.min(links.length - 1, index + delta))];
    if (link) {
      markPending(link);
      link.click();
    }
  }

  function actOnBlock() {
    const items = blocks(), block = qs("[data-action-block].selected") || items[currentBlockIndex(items)];
    if (!block) return;
    const solution = qs("[data-solution-toggle]", block);
    if (solution) return solution.click();
    const inline = qs("[data-inline-hint-toggle]", block), quiz = qs("[data-quiz-submit]:not(:disabled)", block);
    (inline || quiz)?.click();
  }

  function updateQuiz(form) {
    const button = qs("[data-quiz-submit]", form);
    if (button) button.disabled = !qsa("[data-quiz-option]", form).some((el) => el.checked);
  }

  function updateSolutionToggle(button, open) {
    if (!button) return;
    const label = open ? "Hide Solution" : "Show Solution";
    button.dataset.tooltip = label;
    button.setAttribute("aria-label", label);
    button.setAttribute("aria-expanded", String(open));
    button.setAttribute("aria-pressed", String(open));
  }

  function applySidebar() {
    const root = shell();
    if (!root) return;
    const width = Math.max(minRem, Math.min(maxRem, Number(localStorage.getItem("labbit.sidebar.width.rem")) || defaultRem));
    root.style.setProperty("--sidebar-expanded-width", `${width}rem`);
    root.classList.toggle("sidebar-collapsed", localStorage.getItem("labbit.sidebar.collapsed") === "true");
    root.style.setProperty("--sidebar-width", root.classList.contains("sidebar-collapsed") ? "var(--sidebar-rail-width)" : `${width}rem`);
  }

  function toggleSidebar(trigger) {
    const root = shell();
    if (!root) return;
    root.classList.toggle("sidebar-collapsed");
    localStorage.setItem("labbit.sidebar.collapsed", String(root.classList.contains("sidebar-collapsed")));
    trigger?.blur?.();
    applySidebar();
  }

  function resizeSidebar(event) {
    const root = shell();
    if (!root) return;
    event.preventDefault();
    root.classList.remove("sidebar-collapsed");
    localStorage.setItem("labbit.sidebar.collapsed", "false");
    const move = (e) => {
      const width = rem(e.clientX);
      if (width < snapRem) {
        root.classList.add("sidebar-collapsed");
        localStorage.setItem("labbit.sidebar.collapsed", "true");
        return;
      }
      const clamped = Math.max(minRem, Math.min(maxRem, width));
      root.classList.remove("sidebar-collapsed");
      root.style.setProperty("--sidebar-width", `${clamped}rem`);
      root.style.setProperty("--sidebar-expanded-width", `${clamped}rem`);
      localStorage.setItem("labbit.sidebar.width.rem", String(clamped));
    };
    const up = () => {
      document.removeEventListener("mousemove", move);
      document.removeEventListener("mouseup", up);
    };
    document.addEventListener("mousemove", move);
    document.addEventListener("mouseup", up);
  }

  function markPending(el) {
    const root = shell();
    if (root) root.dataset.pendingScrollTarget = el.dataset.scrollTarget || el.dataset.shareTarget || "";
  }

  function swapTarget(event) {
    return event.detail?.target || event.detail?.ctx?.target || event.target;
  }

  document.addEventListener("keydown", (event) => {
    const key = event.key.toLowerCase();
    if (searchOpen()) {
      if ((event.ctrlKey || event.metaKey) && key === "j") return event.preventDefault(), moveSearch(1);
      if ((event.ctrlKey || event.metaKey) && key === "k") return event.preventDefault(), moveSearch(-1);
      if (event.key === "ArrowDown") return event.preventDefault(), moveSearch(1);
      if (event.key === "ArrowUp") return event.preventDefault(), moveSearch(-1);
      if (event.key === "Enter") return event.preventDefault(), activeSearch()?.click();
    }
    if ((event.ctrlKey || event.metaKey) && key === "k") return event.preventDefault(), modal("search", true);
    if (event.key === "Escape") return modal("search", false), modal("keybindings", false);
    if (typing(event.target) || event.ctrlKey || event.metaKey || event.altKey) return;
    if (event.key === "?") return event.preventDefault(), modal("keybindings", true);
    if (event.key === "A") return event.preventDefault(), actOnBlock();
    if (event.key === "J") return event.preventDefault(), moveSection(1);
    if (event.key === "H" || event.key === "K") return event.preventDefault(), moveSection(-1);
    if (event.key === "j") return event.preventDefault(), moveBlock(1);
    if (event.key === "h" || event.key === "k") return event.preventDefault(), moveBlock(-1);
  });

  document.addEventListener("click", async (event) => {
    if (event.target.matches("[data-search-modal]")) return modal("search", false);
    if (event.target.matches("[data-keybindings-modal]") || event.target.closest("[data-close-keybindings]")) return modal("keybindings", false);
    const sidebar = event.target.closest("[data-toggle-sidebar]");
    if (sidebar) return toggleSidebar(sidebar);
    const opener = event.target.closest("[data-open-search]");
    if (opener) return modal("search", true);
    const hxLink = event.target.closest("[hx-get][data-section-id], [hx-get][data-share-target], [data-search-result]");
    if (hxLink) markPending(hxLink);
    if (event.target.closest("[data-close-search]")) modal("search", false);

    const selected = event.target.closest("[data-select-block], [data-inline-hint-toggle], [data-solution-toggle]");
    if (selected) selectBlock(selected.closest("[data-action-block]"));

    const share = event.target.closest("[data-share-target]:not([hx-get]):not([data-solution-toggle])");
    if (share) {
      const target = document.getElementById(share.dataset.shareTarget);
      if (target?.matches?.("[data-action-block]")) {
        event.preventDefault();
        selectBlock(target);
      }
    }

    const solutionButton = event.target.closest("[data-solution-toggle]");
    if (solutionButton) {
      const slot = qs(solutionButton.dataset.solutionTarget);
      if (slot?.dataset.solutionLoaded === "true") {
        event.preventDefault();
        const hidden = slot.classList.toggle("hidden");
        solutionButton.classList.toggle("open", !hidden);
        updateSolutionToggle(solutionButton, !hidden);
      }
    }

    const copy = event.target.closest("[data-copy]");
    if (!copy) return;
    const code = copy.closest(".code-shell")?.querySelector("[data-code]")?.dataset.code;
    if (!code) return;
    await navigator.clipboard.writeText(code);
    copy.textContent = "Copied";
    setTimeout(() => (copy.textContent = "Copy"), 900);
  });

  document.addEventListener("mousedown", (event) => event.target.closest("[data-sidebar-resizer]") && resizeSidebar(event));
  document.addEventListener("change", (event) => {
    const form = event.target.closest("[data-quiz-card]");
    if (form) updateQuiz(form);
  });

  document.body.addEventListener("htmx:after:swap", (event) => {
    const target = swapTarget(event);
    if (target?.tagName === "BODY") applySidebar();
    if (target?.matches?.("[data-search-results]")) return selectSearch(0);
    if (target?.classList?.contains("solution-slot")) {
      const slot = target, button = qs(`[data-solution-target="#${CSS.escape(slot.id)}"]`);
      slot.dataset.solutionLoaded = "true";
      slot.classList.remove("hidden");
      button?.classList.add("open");
      updateSolutionToggle(button, true);
    }
    qsa("[data-quiz-card]").forEach(updateQuiz);
    if (target?.id === "content") {
      setTimeout(() => {
        const root = shell();
        const pending = root?.dataset.pendingScrollTarget || "";
        const scrollTarget = document.getElementById(pending) || qs("[data-section]", target);
        if (root) delete root.dataset.pendingScrollTarget;
        scrollTarget?.scrollIntoView({ behavior: "smooth", block: "start" });
      }, 0);
    }
  });

  document.body.addEventListener("labbitThemeChanged", (event) => {
    const theme = event.detail?.theme === "light" ? "light" : "dark";
    document.documentElement.dataset.theme = theme;
  });

  document.addEventListener("DOMContentLoaded", () => {
    applySidebar();
    qsa("[data-quiz-card]").forEach(updateQuiz);
    qs("[data-action-block].selected")?.scrollIntoView({ block: "start" });
  });
})();
