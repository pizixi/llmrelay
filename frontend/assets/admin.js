/* ============================================
   LLM Relay 管理面板
   ============================================ */

/* ===== SVG 图标 ===== */
const ICONS = {
  refresh:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>',
  trash:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>',
  sync: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 2v6h-6"/><path d="M3 12a9 9 0 0 1 15-6.7L21 8"/><path d="M3 22v-6h6"/><path d="M21 12a9 9 0 0 1-15 6.7L3 16"/></svg>',
  chart:
    '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>',
  edit: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>',
  plus: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>',
  save: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z"/><polyline points="17 21 17 13 7 13 7 21"/><polyline points="7 3 7 8 15 8"/></svg>',
  logout:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>',
  key: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"/></svg>',
  layers:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 17 12 22 22 17"/><polyline points="2 12 12 17 22 12"/></svg>',
  search:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>',
  globe:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>',
  copy: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>',
  check:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>',
  star:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>',
  close:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>',
  play:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="5 3 19 12 5 21 5 3"/></svg>',
  gauge:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4.93 19a10 10 0 1 1 14.14 0"/><path d="M12 12l4-4"/><circle cx="12" cy="12" r="1.5"/></svg>',
  stop:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="6" y="6" width="12" height="12" rx="1"/></svg>',
  arrowUp:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="19" x2="12" y2="5"/><polyline points="5 12 12 5 19 12"/></svg>',
  arrowRight:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="5" y1="12" x2="19" y2="12"/><polyline points="13 6 19 12 13 18"/></svg>',
  chevron:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>',
  sparkles:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3l1.25 3.75L17 8l-3.75 1.25L12 13l-1.25-3.75L7 8l3.75-1.25L12 3z"/><path d="M19 14l.8 2.2L22 17l-2.2.8L19 20l-.8-2.2L16 17l2.2-.8L19 14z"/><path d="M5 13l.65 1.85L7.5 15.5l-1.85.65L5 18l-.65-1.85-1.85-.65 1.85-.65L5 13z"/></svg>',
  alert:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>',
  inbox:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 16 12 14 15 10 15 8 12 2 12"/><path d="M5.45 5.11L2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/></svg>',
  server:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="2" width="20" height="8" rx="2" ry="2"/><rect x="2" y="14" width="20" height="8" rx="2" ry="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>',
  grip: '<svg viewBox="0 0 24 24" fill="currentColor" stroke="none"><circle cx="9" cy="6" r="1.6"/><circle cx="15" cy="6" r="1.6"/><circle cx="9" cy="12" r="1.6"/><circle cx="15" cy="12" r="1.6"/><circle cx="9" cy="18" r="1.6"/><circle cx="15" cy="18" r="1.6"/></svg>',
  moon: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>',
  sun: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>',
};

/* ===== 主题 ===== */
function getThemeIcon(theme) {
  return theme === "dark" ? ICONS.moon : ICONS.sun;
}
function updateThemeIcon(theme) {
  const b = document.querySelector(".theme-toggle");
  if (b) b.innerHTML = getThemeIcon(theme);
}
function toggleTheme() {
  const d = document.documentElement;
  const cur = d.getAttribute("data-theme");
  const next = cur === "dark" ? null : "dark";
  if (next) d.setAttribute("data-theme", next);
  else d.removeAttribute("data-theme");
  localStorage.setItem("theme", next || "light");
  updateThemeIcon(next || "light");
}
(function () {
  const t = localStorage.getItem("theme");
  if (t === "dark") document.documentElement.setAttribute("data-theme", "dark");
  function init() {
    updateThemeIcon(localStorage.getItem("theme") || "light");
    injectButtonIcons();
  }
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();

/* 为带 data-icon 的按钮注入 SVG 图标 */
function injectButtonIcons() {
  document.querySelectorAll("[data-icon]").forEach((el) => {
    const name = el.getAttribute("data-icon");
    if (ICONS[name]) {
      el.classList.add("btn-icon-text");
      el.innerHTML = ICONS[name] + el.innerHTML;
      el.removeAttribute("data-icon");
    }
  });
}

/* ===== 标题栏标签页 ===== */
const ADMIN_TAB_STORAGE_KEY = "llmrelay.admin.activeTab";

function scrollAdminTabIntoView(tab, smooth) {
  const tabList = tab && tab.closest(".header-tabs");
  if (!tabList) return;
  const listRect = tabList.getBoundingClientRect();
  const tabRect = tab.getBoundingClientRect();
  const visibleLeft = listRect.left + 10;
  const visibleRight = listRect.right - 24;
  let nextLeft = tabList.scrollLeft;
  if (tabRect.left < visibleLeft) {
    nextLeft += tabRect.left - visibleLeft;
  } else if (tabRect.right > visibleRight) {
    nextLeft += tabRect.right - visibleRight;
  } else {
    return;
  }
  tabList.scrollTo({ left: nextLeft, behavior: smooth ? "smooth" : "auto" });
}

function activateAdminTab(tabName, options) {
  const tabs = Array.from(document.querySelectorAll(".header-tab[data-tab]"));
  const panels = Array.from(document.querySelectorAll(".tab-panel[data-tab-panel]"));
  const selected = tabs.find((tab) => tab.dataset.tab === tabName) || tabs[0];
  if (!selected) return;

  const activeName = selected.dataset.tab;
  tabs.forEach((tab) => {
    const active = tab === selected;
    tab.classList.toggle("is-active", active);
    tab.setAttribute("aria-selected", String(active));
    tab.tabIndex = active ? 0 : -1;
  });
  panels.forEach((panel) => {
    const active = panel.dataset.tabPanel === activeName;
    panel.hidden = !active;
    panel.classList.toggle("is-active", active);
  });
	if (activeName === "usage" && typeof loadUsageRecords === "function") {
		loadUsageRecords();
	}

  try {
    localStorage.setItem(ADMIN_TAB_STORAGE_KEY, activeName);
  } catch (e) {}
  if (!options || options.updateURL !== false) {
    history.replaceState(null, "", "#" + activeName);
  }
  requestAnimationFrame(() => scrollAdminTabIntoView(selected, Boolean(options && options.focus)));
  if (options && options.focus) selected.focus();
}

function initAdminTabs() {
  const tabs = Array.from(document.querySelectorAll(".header-tab[data-tab]"));
  if (!tabs.length) return;

  const validNames = new Set(tabs.map((tab) => tab.dataset.tab));
  const hashName = window.location.hash.replace(/^#/, "");
  let savedName = "";
  try {
    savedName = localStorage.getItem(ADMIN_TAB_STORAGE_KEY) || "";
  } catch (e) {}
  const initialName = validNames.has(hashName)
    ? hashName
    : validNames.has(savedName)
      ? savedName
      : tabs[0].dataset.tab;
  activateAdminTab(initialName, { updateURL: Boolean(hashName) });

  tabs.forEach((tab, index) => {
    tab.addEventListener("click", () => activateAdminTab(tab.dataset.tab));
    tab.addEventListener("keydown", (event) => {
      let nextIndex = index;
      if (event.key === "ArrowRight") nextIndex = (index + 1) % tabs.length;
      else if (event.key === "ArrowLeft") nextIndex = (index - 1 + tabs.length) % tabs.length;
      else if (event.key === "Home") nextIndex = 0;
      else if (event.key === "End") nextIndex = tabs.length - 1;
      else return;
      event.preventDefault();
      activateAdminTab(tabs[nextIndex].dataset.tab, { focus: true });
    });
  });
}

/* ===== 全局状态 ===== */
let aliasData = {},
  effortData = {},
  globalEffortData = {},
  selectedAliasKey = "",
  modelListByUpstream = {},
  upstreamData = {},
  savedUpstreamModelsByUpstream = {},
  pendingUpstreamRenames = {},
  upstreamOrder = [],
  socks5Data = [],
  webSearchData = {},
  searxngInstances = [],
  apiKeysData = [];

let usagePageOffset = 0;
const USAGE_PAGE_SIZE = 50;
let usageLoadSequence = 0;
let usageRecords = [];

/* ===== 登录态处理 ===== */
function redirectToLogin() {
  if (window.location.pathname !== "/login") {
    window.location.href = "/login";
  }
}

function isLoginResponse(r) {
  if (!r) return false;
  if (r.headers && r.headers.get("X-Login-Required") === "1") return true;
  try {
    if (r.redirected && r.url && r.url.indexOf("/login") !== -1) return true;
    if (r.url && /\/login(?:\?|$)/.test(r.url)) return true;
  } catch (e) {}
  return false;
}

function isLoginHTML(text) {
  return (
    /<form\b[^>]*\baction=["']\/login(?:\?[^"']*)?["']/i.test(text || "") ||
    /<link\b[^>]*\bhref=["']\/assets\/login\.css["']/i.test(text || "")
  );
}

async function apiFetch(url, options) {
  const r = await fetch(url, options);
  if (isLoginResponse(r)) {
    redirectToLogin();
    throw new Error("登录已失效，正在跳转登录页");
  }
  return r;
}

async function apiJSON(url, options) {
  const r = await apiFetch(url, options);
  const ct = (r.headers.get("content-type") || "").toLowerCase();
  if (ct.indexOf("application/json") === -1) {
    const text = await r.text();
    if (isLoginHTML(text)) {
      redirectToLogin();
      throw new Error("登录已失效，正在跳转登录页");
    }
    throw new Error(text || "HTTP " + r.status);
  }
  if (!r.ok) {
    let msg = "HTTP " + r.status;
    try {
      const errBody = await r.json();
      if (errBody && (errBody.error || errBody.message)) {
        msg =
          typeof errBody.error === "string"
            ? errBody.error
            : (errBody.error && errBody.error.message) ||
              errBody.message ||
              msg;
      }
    } catch (e) {}
    throw new Error(msg);
  }
  return r.json();
}

/* ===== 弹层系统 ===== */
let activeOverlay = null;
let activePopover = null;
let popoverCloseCallback = null;
let bodyOverflowBeforePopover = "";
let rootOverflowBeforePopover = "";

function closePopover() {
  if (activePopover) {
    if (ssActiveWrapper && activePopover.contains(ssActiveWrapper)) ssClose();
    const cb = popoverCloseCallback;
    popoverCloseCallback = null;
    const trigger = activePopover._triggerEl;
    if (trigger) trigger.classList.remove("active");
    activeOverlay.remove();
    activeOverlay = null;
    activePopover = null;
    document.removeEventListener("keydown", onModalEsc);
    document.body.style.overflow = bodyOverflowBeforePopover;
    document.documentElement.style.overflow = rootOverflowBeforePopover;
    if (cb) cb();
  }
}

function onModalEsc(e) {
  if (e.key === "Escape" && !ssActiveDropdown) closePopover();
}

function onOverlayClick(e) {
  if (e.target === activeOverlay) closePopover();
}

function openPopover(triggerEl, contentHtml, onClose) {
  closePopover();
  const overlay = document.createElement("div");
  overlay.className = "popover-overlay";
  const pop = document.createElement("div");
  pop.className = "popover";
  pop.innerHTML = contentHtml;
  pop._triggerEl = triggerEl;
  overlay.appendChild(pop);
  document.body.appendChild(overlay);
  bodyOverflowBeforePopover = document.body.style.overflow;
  rootOverflowBeforePopover = document.documentElement.style.overflow;
  document.body.style.overflow = "hidden";
  document.documentElement.style.overflow = "hidden";
  triggerEl.classList.add("active");
  activeOverlay = overlay;
  activePopover = pop;
  enhanceFilterInputs(pop);
  popoverCloseCallback = onClose || null;
  overlay.addEventListener("mousedown", onOverlayClick);
  document.addEventListener("keydown", onModalEsc);
  return pop;
}

/* ===== 配置加载 ===== */
function reloadConfig() {
  const sy = window.scrollY;
  loadConfig();
  loadAPIKeys();
  loadStats();
  loadUsageRecords();
  showToast("页面数据已刷新", "success");
  setTimeout(() => window.scrollTo(0, sy), 100);
}

function buildModelMap(list) {
  const grouped = {};
  (list || []).forEach((m) => {
    const owner =
      (m && m.owned_by ? String(m.owned_by) : "default").trim() || "default";
    const id = m && m.id ? String(m.id) : "";
    if (!id) return;
    if (!grouped[owner]) grouped[owner] = [];
    grouped[owner].push(id);
  });
  Object.keys(grouped).forEach((k) => {
    grouped[k] = Array.from(new Set(grouped[k])).sort();
  });
  return grouped;
}

function normalizeAliasData() {
  const next = {};
  Object.keys(aliasData || {}).forEach((k) => {
    const raw = aliasData[k];
    if (typeof raw === "object" && raw) {
      let targets = normalizeAliasTargets(raw.targets || []);
      if (!targets.length && (raw.target_model || raw.upstream)) {
        targets = normalizeAliasTargets([
          {
            target_model: raw.target_model || k,
            upstream: raw.upstream || "",
            weight: 1,
          },
        ]);
      }
      next[k] = {
        targets: targets,
        with_reasoning: !!raw.with_reasoning,
        reasoning_effort_map: { ...(raw.reasoning_effort_map || {}) },
      };
    } else {
      const targetModel = typeof raw === "string" ? raw : "";
      next[k] = {
        targets: targetModel
          ? [
              {
                target_model: targetModel,
                upstream: "",
                weight: 1,
              },
            ]
          : [],
        with_reasoning: false,
        reasoning_effort_map: {},
      };
    }
  });
  aliasData = next;
}

function normalizeUpstreamData(cfg) {
  upstreamData = cfg.upstreams || {};
  upstreamOrder = Array.isArray(cfg.upstream_order)
    ? cfg.upstream_order.slice()
    : [];
  orderedUpstreamNames();
}

function orderedUpstreamNames() {
  const names = [];
  const seen = new Set();
  (upstreamOrder || []).forEach((rawName) => {
    const name = String(rawName || "").trim();
    if (!name || seen.has(name) || !upstreamData[name]) return;
    seen.add(name);
    names.push(name);
  });
  Object.keys(upstreamData)
    .sort()
    .forEach((name) => {
      if (seen.has(name)) return;
      seen.add(name);
      names.push(name);
    });
  upstreamOrder = names.slice();
  return names;
}

function modelMapFromConfig(cfg) {
  let next = {};
  if (
    cfg.available_models_by_upstream &&
    Object.keys(cfg.available_models_by_upstream).length
  ) {
    Object.keys(cfg.available_models_by_upstream).forEach((k) => {
      next[k] = (cfg.available_models_by_upstream[k] || [])
        .map((m) => (typeof m === "string" ? m : m && m.id ? m.id : ""))
        .filter(Boolean);
      next[k] = Array.from(new Set(next[k])).sort();
    });
  } else if (cfg.available_models && cfg.available_models.length) {
    next = buildModelMap(cfg.available_models);
  }
  return next;
}

function applyModelList(cfg, mergeUpstreamName) {
  const next = modelMapFromConfig(cfg);
  if (mergeUpstreamName) {
    modelListByUpstream[mergeUpstreamName] = next[mergeUpstreamName] || [];
    return;
  }
  if (Object.keys(next).length) modelListByUpstream = next;
}

function upstreamModelsSnapshot(source) {
  const snapshot = {};
  Object.keys(source || {}).forEach((name) => {
    const upstream = source[name] || {};
    snapshot[name] = Array.from(
      new Set(
        (Array.isArray(upstream.custom_models) ? upstream.custom_models : [])
          .map((model) => String(model || "").trim())
          .filter(Boolean),
      ),
    );
  });
  return snapshot;
}

function rememberSavedUpstreamModels() {
  savedUpstreamModelsByUpstream = upstreamModelsSnapshot(upstreamData);
  pendingUpstreamRenames = {};
  document.querySelectorAll("#upstreamTable tbody tr[data-upstream-row]").forEach(
    (row) => {
      const name = String(
        row.querySelector('[data-field="name"]')?.value || "",
      ).trim();
      if (!name) return;
      row.dataset.upstreamRow = name;
      row.dataset.upstreamOriginal = name;
    },
  );
}

function resolvedPendingUpstreamName(name) {
  let current = String(name || "").trim();
  const seen = new Set();
  while (current && pendingUpstreamRenames[current] && !seen.has(current)) {
    seen.add(current);
    current = String(pendingUpstreamRenames[current] || "").trim();
  }
  return current;
}

function syncPendingUpstreamRenames() {
  if (!Object.keys(pendingUpstreamRenames).length) return;

  document
    .querySelectorAll("#aliasTable [data-field=\"targets\"]")
    .forEach((field) => {
      const before = parseAliasTargets(field.dataset.targets || "[]");
      const next = normalizeAliasTargets(
        before.map((target) => ({
          ...target,
          upstream: resolvedPendingUpstreamName(target.upstream),
        })),
      );
      if (JSON.stringify(before) === JSON.stringify(next)) return;
      updateAliasTargetsField(field, next);
    });

  Object.keys(pendingUpstreamRenames).forEach((oldName) => {
    const nextName = resolvedPendingUpstreamName(oldName);
    if (!nextName || oldName === nextName) return;
    const oldModels = modelListByUpstream[oldName] || [];
    const nextModels = modelListByUpstream[nextName] || [];
    if (oldModels.length || nextModels.length) {
      modelListByUpstream[nextName] = Array.from(
        new Set(nextModels.concat(oldModels)),
      ).sort((a, b) => a.localeCompare(b));
    }
    delete modelListByUpstream[oldName];
  });
}

function removedUpstreamModelsSinceSave() {
  const removed = {};
  Object.keys(savedUpstreamModelsByUpstream || {}).forEach((upstreamName) => {
    const currentName = resolvedPendingUpstreamName(upstreamName);
    if (!upstreamData[currentName]) return;
    const current = new Set(
      Array.isArray(upstreamData[currentName].custom_models)
        ? upstreamData[currentName].custom_models
        : [],
    );
    const missing = (savedUpstreamModelsByUpstream[upstreamName] || []).filter(
      (model) => !current.has(model),
    );
    if (missing.length) removed[currentName] = new Set(missing);
  });
  return removed;
}

function upstreamModelAliasDeleteImpact(removedByUpstream) {
  const aliases = [];
  let targetCount = 0;
  Object.keys(aliasData || {}).forEach((aliasName) => {
    const entry = aliasData[aliasName] || {};
    const targets = normalizeAliasTargets(entry.targets || []);
    const removedTargets = targets.filter((target) => {
      const models = removedByUpstream[target.upstream];
      return models && models.has(target.target_model);
    });
    if (!removedTargets.length) return;
    const removedIdentities = new Set(
      removedTargets.map((target) =>
        aliasTargetIdentity(target.upstream, target.target_model),
      ),
    );
    const remainingTargets = targets.filter(
      (target) =>
        !removedIdentities.has(
          aliasTargetIdentity(target.upstream, target.target_model),
        ),
    );
    targetCount += removedTargets.length;
    aliases.push({
      aliasName: aliasName,
      removedTargets: removedTargets,
      remainingTargets: remainingTargets,
      removeAlias: remainingTargets.length === 0,
    });
  });
  return {
    aliases: aliases,
    targetCount: targetCount,
    removedAliasCount: aliases.filter((item) => item.removeAlias).length,
  };
}

function reconcileRemovedUpstreamModelMappings() {
  const removedByUpstream = removedUpstreamModelsSinceSave();
  const impact = upstreamModelAliasDeleteImpact(removedByUpstream);
  if (!impact.targetCount) return impact;
  const selectedAliasRemoved = applyUpstreamAliasDelete(impact);
  renderAliasTable();
  if (selectedAliasRemoved) showSelectedEffortMap();
  return impact;
}

async function loadConfig() {
  const sy = window.scrollY;
  ssClose();
  try {
    const cfg = await apiJSON("/api/config");
    aliasData = cfg.model_alias || {};
    normalizeUpstreamData(cfg);
    rememberSavedUpstreamModels();
    normalizeAliasData();
    globalEffortData = { ...(cfg.reasoning_effort_map || {}) };
    selectedAliasKey = "";
    effortData = { ...globalEffortData };
    socks5Data = cfg.socks5_proxies || [];
    webSearchData = { ...(cfg.web_search || {}) };
    applyModelList(cfg);
    if (Object.keys(modelListByUpstream).length === 0) {
      try {
        const md = await apiJSON("/v1/models");
        modelListByUpstream = buildModelMap(md.data || []);
      } catch (e) {
        modelListByUpstream = {};
      }
    }
    renderUpstreamTable();
    renderAliasTable();
    renderEffortTable();
    renderSocks5Table();
    renderWebSearchConfig();
    setTimeout(() => window.scrollTo(0, sy), 0);
  } catch (e) {
    if (String(e.message || "").indexOf("登录已失效") !== -1) return;
    showToast("失败: " + e.message, "error");
  }
}

/* ===== API 密钥 ===== */
function apiKeyShortValue(value) {
  const key = String(value || "");
  if (!key) return "-";
  if (key.length <= 18) return key;
  return key.slice(0, 11) + "..." + key.slice(-4);
}

function formatAPIKeyDate(value) {
  if (!value) return "-";
  return formatUsageTime(value);
}

function renderAPIKeys() {
  const tbody = document.querySelector("#apiKeyTable tbody");
  if (!tbody) return;
  if (!apiKeysData.length) {
    tbody.innerHTML = emptyRowHtml(
      5,
      ICONS.key,
      "暂无 API 密钥",
      "创建一个密钥后，客户端即可使用它调用网关接口",
    );
    return;
  }
  tbody.innerHTML = apiKeysData
    .map((item) => {
      const id = String(item.id || "");
      const disabled = !!item.disabled;
      const statusClass = disabled ? "is-disabled" : "is-enabled";
      const statusText = disabled ? "已停用" : "已启用";
      return (
        "<tr>" +
        '<td><div class="api-key-name">' +
        esc(item.name || "未命名密钥") +
        "</div></td>" +
        '<td><div class="api-key-value"><code title="点击复制完整密钥">' +
        esc(apiKeyShortValue(item.key)) +
        '</code><button class="btn-icon btn-icon-success" type="button" title="复制 API 密钥" aria-label="复制 API 密钥" onclick="copyAPIKey(\'' +
        escAttr(id) +
        "')\">" +
        ICONS.copy +
        "</button></div></td>" +
        '<td><button class="api-key-status ' +
        statusClass +
        '" type="button" title="点击' +
        (disabled ? "启用" : "停用") +
        '" onclick="toggleAPIKey(\'' +
        escAttr(id) +
        "'," +
        (!disabled ? "true" : "false") +
        ')"><i></i>' +
        statusText +
        "</button></td>" +
        '<td class="api-key-date">' +
        esc(formatAPIKeyDate(item.created_at)) +
        "</td>" +
        '<td><div class="api-key-actions"><button class="btn-icon" type="button" title="编辑名称" aria-label="编辑名称" onclick="editAPIKey(\'' +
        escAttr(id) +
        "',this)\">" +
        ICONS.edit +
        '<\/button><button class="btn-icon btn-icon-danger" type="button" title="删除密钥" aria-label="删除密钥" onclick="deleteAPIKey(\'' +
        escAttr(id) +
        "',this)\">" +
        ICONS.trash +
        "</button></div></td>" +
        "</tr>"
      );
    })
    .join("");
}

function refreshAPIKeyFilter() {
  setUsageFilterOptions(
    "usageAPIKeyFilter",
    apiKeysData.map((item) => item.name || ""),
    "全部密钥",
  );
  ssSyncLabel(document.getElementById("usageAPIKeyFilter"));
}

async function loadAPIKeys() {
  try {
    const data = await apiJSON("/api/api-keys");
    apiKeysData = Array.isArray(data.keys) ? data.keys : [];
    renderAPIKeys();
    refreshAPIKeyFilter();
  } catch (e) {
    if (String(e.message || "").indexOf("登录已失效") !== -1) return;
    const tbody = document.querySelector("#apiKeyTable tbody");
    if (tbody) tbody.innerHTML = emptyRowHtml(5, ICONS.alert, "加载失败", e.message || "请稍后重试");
  }
}

function apiKeyByID(id) {
  return apiKeysData.find((item) => String(item.id || "") === String(id || ""));
}

async function copyAPIKey(id) {
  const item = apiKeyByID(id);
  if (!item || !item.key) {
    showToast("密钥内容不可用", "error");
    return;
  }
  const copied = await copyText(item.key);
  showToast(copied ? "API 密钥已复制" : "复制失败，请手动复制", copied ? "success" : "error");
}

function apiKeyEditorHtml(title, name, submitLabel) {
  return (
    '<div class="popover-header"><span class="popover-title">' +
    ICONS.key +
    esc(title) +
    '</span><button type="button" class="btn-icon" title="关闭" aria-label="关闭" onclick="closePopover()">' +
    ICONS.close +
    "</button></div>" +
    '<label class="popover-label" for="apiKeyNameInput">密钥名称</label>' +
    '<input id="apiKeyNameInput" class="popover-input" type="text" maxlength="128" placeholder="例如：生产环境" value="' +
    escAttr(name || "") +
    '">' +
    '<div class="popover-hint">名称用于在使用记录中识别和筛选该密钥。</div>' +
    '<div class="danger-confirm-actions api-key-editor-actions"><button type="button" class="btn api-key-editor-cancel" onclick="closePopover()">取消</button><button type="button" class="btn btn-primary api-key-editor-submit">' +
    esc(submitLabel || "保存") +
    "</button></div>"
  );
}

function createAPIKey() {
  const trigger = document.querySelector("#tab-api-keys .btn-primary");
  let pop;
  pop = openPopover(trigger, apiKeyEditorHtml("创建 API 密钥", "", "创建并保存"));
  pop.classList.add("api-key-editor-popover");
  const submit = pop.querySelector(".api-key-editor-submit");
  submit?.addEventListener("click", async () => {
    const name = pop.querySelector("#apiKeyNameInput")?.value.trim() || "";
    if (!name) {
      showToast("请输入密钥名称", "error");
      pop.querySelector("#apiKeyNameInput")?.focus();
      return;
    }
    submit.disabled = true;
    try {
      const response = await apiFetch("/api/api-keys", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: name }),
      });
      if (!response.ok) throw new Error(await response.text());
      closePopover();
      await loadAPIKeys();
      showToast("API 密钥已创建，可点击复制", "success");
    } catch (e) {
      if (String(e.message || "").indexOf("登录已失效") !== -1) return;
      showToast("创建失败: " + e.message, "error");
      submit.disabled = false;
    }
  });
  setTimeout(() => pop.querySelector("#apiKeyNameInput")?.focus(), 0);
}

function editAPIKey(id, trigger) {
  const item = apiKeyByID(id);
  if (!item) return;
  let pop;
  pop = openPopover(trigger, apiKeyEditorHtml("编辑 API 密钥名称", item.name, "保存"));
  pop.classList.add("api-key-editor-popover");
  pop.querySelector(".api-key-editor-submit")?.addEventListener("click", async () => {
    const submit = pop.querySelector(".api-key-editor-submit");
    const name = pop.querySelector("#apiKeyNameInput")?.value.trim() || "";
    if (!name) {
      showToast("请输入密钥名称", "error");
      return;
    }
    submit.disabled = true;
    try {
      const response = await apiFetch("/api/api-keys/" + encodeURIComponent(id), {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: name }),
      });
      if (!response.ok) throw new Error(await response.text());
      closePopover();
      await loadAPIKeys();
      showToast("密钥名称已更新", "success");
    } catch (e) {
      if (String(e.message || "").indexOf("登录已失效") !== -1) return;
      showToast("保存失败: " + e.message, "error");
      submit.disabled = false;
    }
  });
  setTimeout(() => {
    const input = pop.querySelector("#apiKeyNameInput");
    input?.focus();
    input?.select();
  }, 0);
}

async function toggleAPIKey(id, disabled) {
  try {
    const response = await apiFetch("/api/api-keys/" + encodeURIComponent(id), {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ disabled: !!disabled }),
    });
    if (!response.ok) throw new Error(await response.text());
    await loadAPIKeys();
    showToast(disabled ? "密钥已停用" : "密钥已启用", "success");
  } catch (e) {
    if (String(e.message || "").indexOf("登录已失效") !== -1) return;
    showToast("更新密钥状态失败: " + e.message, "error");
  }
}

async function deleteAPIKey(id, trigger) {
  const item = apiKeyByID(id);
  if (!item) return;
  const confirmed = await showDangerConfirm(trigger, {
    title: "确认删除 API 密钥？",
    subject: item.name || "未命名密钥",
    description: "删除后，使用该密钥的客户端将立即无法访问网关。",
    note: "删除操作不可撤销；如只是临时停用，建议使用状态按钮。",
    confirmLabel: "确认删除",
  });
  if (!confirmed) return;
  try {
    const response = await apiFetch("/api/api-keys/" + encodeURIComponent(id), { method: "DELETE" });
    if (!response.ok) throw new Error(await response.text());
    await loadAPIKeys();
    showToast("API 密钥已删除", "success");
  } catch (e) {
    if (String(e.message || "").indexOf("登录已失效") !== -1) return;
    showToast("删除失败: " + e.message, "error");
  }
}

async function saveConfigSilent(options) {
  collectUpstreams();
  collectAliases();
  const cleanup = reconcileRemovedUpstreamModelMappings();
  collectEfforts();
  collectSocks5();
  collectWebSearchConfig();
  const cfg = {
    model_alias: aliasData,
    reasoning_effort_map: globalEffortData,
    web_search: webSearchData,
    socks5_proxies: socks5Data,
    upstreams: upstreamData,
    upstream_order: upstreamOrder,
  };
  const qs = options && options.skipModelSync ? "?skip_model_sync=1" : "";
  const r = await apiFetch("/api/config" + qs, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(cfg),
  });
  if (!r.ok) throw new Error(await r.text());
  rememberSavedUpstreamModels();
  return cleanup;
}

async function refreshModelList(upstreamName) {
  try {
    upstreamName = (upstreamName || "").trim();
    if (!upstreamName) throw new Error("upstream name is required");
    // 先保存配置，确保服务端使用最新上游设置
    await saveConfigSilent({ skipModelSync: true });
    // 仅触发当前上游的模型拉取
    const reloadUrl =
      "/api/reload?upstream=" + encodeURIComponent(upstreamName);
    const reloadResp = await apiFetch(reloadUrl, { method: "POST" });
    if (!reloadResp.ok) throw new Error(await reloadResp.text());
    // 重新获取配置，拿到更新后的模型列表
    const cfg = await apiJSON("/api/config");
    applyModelList(cfg, upstreamName || "");
    renderUpstreamModelFilter();
    filterUpstreamRows();
    return true;
  } catch (e) {
    if (String(e.message || "").indexOf("登录已失效") !== -1) return false;
    return false;
  }
}

/* ===== 可搜索下拉框 ===== */
let ssActiveWrapper = null;
let ssActiveDropdown = null;
let ssFilterFrame = 0;

function ssClose() {
  if (ssActiveWrapper) {
    const trigger = ssActiveWrapper.querySelector(".ss-trigger");
    if (trigger) trigger.setAttribute("aria-expanded", "false");
    ssActiveWrapper.classList.remove("ss-open");
    ssActiveWrapper = null;
  }
  if (ssActiveDropdown) ssActiveDropdown.remove();
  ssActiveDropdown = null;
  if (ssFilterFrame) {
    cancelAnimationFrame(ssFilterFrame);
    ssFilterFrame = 0;
  }
  document.removeEventListener("mousedown", ssOnOutsideClick, true);
  document.removeEventListener("keydown", ssOnKeydown);
  document.removeEventListener("scroll", ssOnViewportChange, true);
  window.removeEventListener("resize", ssOnViewportChange);
}

function ssOnOutsideClick(e) {
  const insideTrigger = ssActiveWrapper && ssActiveWrapper.contains(e.target);
  const insideDropdown =
    ssActiveDropdown && ssActiveDropdown.contains(e.target);
  if (!insideTrigger && !insideDropdown) {
    ssClose();
  }
}

function ssAllOptions(dropdown) {
  if (!dropdown) return [];
  if (!dropdown._options) {
    dropdown._options = Array.from(
      dropdown.querySelectorAll(".ss-option:not(.ss-no-match)"),
    );
  }
  return dropdown._options;
}

function ssVisibleOptions(dropdown) {
  return ssAllOptions(dropdown).filter((opt) => opt.style.display !== "none");
}

function ssActiveIndex(dropdown) {
  const opts = ssVisibleOptions(dropdown);
  return opts.findIndex((opt) => opt.classList.contains("ss-active"));
}

function ssHighlight(dropdown, index) {
  ssAllOptions(dropdown).forEach((opt) => opt.classList.remove("ss-active"));
  const opts = ssVisibleOptions(dropdown);
  if (!opts.length) return;
  const clamped = ((index % opts.length) + opts.length) % opts.length;
  const opt = opts[clamped];
  opt.classList.add("ss-active");
  const container =
    opt.closest(".ss-options") || opt.parentElement;
  if (container) {
    const cRect = container.getBoundingClientRect();
    const oRect = opt.getBoundingClientRect();
    if (oRect.top < cRect.top + 1) {
      container.scrollTop -= cRect.top - oRect.top + 1;
    } else if (oRect.bottom > cRect.bottom - 1) {
      container.scrollTop += oRect.bottom - cRect.bottom + 1;
    }
  }
}

function ssOnKeydown(e) {
  if (!ssActiveDropdown) return;
  if (e.key === "Escape" || e.key === "Tab") {
    const trigger = ssActiveWrapper?.querySelector(".ss-trigger");
    ssClose();
    if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
    }
    trigger?.focus();
    return;
  }
  const opts = ssVisibleOptions(ssActiveDropdown);
  if (!opts.length) {
    if (e.key === "Escape") ssClose();
    return;
  }
  let index = ssActiveIndex(ssActiveDropdown);
  if (index === -1 && opts.length) {
    const selected = opts.find((opt) => opt.classList.contains("ss-selected"));
    index = selected ? opts.indexOf(selected) : 0;
  }
  if (e.key === "ArrowDown") {
    e.preventDefault();
    ssHighlight(ssActiveDropdown, index === -1 ? 0 : index + 1);
  } else if (e.key === "ArrowUp") {
    e.preventDefault();
    ssHighlight(ssActiveDropdown, index === -1 ? opts.length - 1 : index - 1);
  } else if (e.key === "Enter") {
    e.preventDefault();
    const active = opts[index === -1 ? 0 : index];
    if (active) ssSelect(active);
  } else if (e.key === "Escape") {
    ssClose();
  }
}

function ssOnViewportChange(e) {
  if (ssActiveDropdown && e && ssActiveDropdown.contains(e.target)) return;
  ssClose();
}

function ssPositionDropdown(btn, dropdown) {
  const margin = 8;
  const gap = 4;
  const rect = btn.getBoundingClientRect();
  const viewportWidth = document.documentElement.clientWidth;
  const viewportHeight = document.documentElement.clientHeight;
  // Keep compact triggers in table cells, but let the open menu grow to fit
  // complete option text (notably proxy host/port) when the viewport allows.
  const borderWidth = Math.max(0, dropdown.offsetWidth - dropdown.clientWidth);
  const contentWidth = Math.ceil((dropdown.scrollWidth || 0) + borderWidth);
  const width = Math.min(
    Math.max(rect.width, contentWidth, 220),
    viewportWidth - margin * 2,
  );
  const left = Math.min(
    Math.max(rect.left, margin),
    viewportWidth - width - margin,
  );
  const spaceBelow = viewportHeight - rect.bottom - gap - margin;
  const spaceAbove = rect.top - gap - margin;
  const desiredHeight = Math.min(dropdown.scrollHeight, 330);
  const openUp =
    spaceBelow < Math.min(desiredHeight, 240) && spaceAbove > spaceBelow;
  const availableHeight = Math.max(0, openUp ? spaceAbove : spaceBelow);
  const height = Math.min(desiredHeight, availableHeight);

  dropdown.classList.toggle("ss-up", openUp);
  dropdown.style.left = left + "px";
  dropdown.style.width = width + "px";
  dropdown.style.maxHeight = height + "px";
  dropdown.style.top =
    (openUp ? Math.max(margin, rect.top - gap - height) : rect.bottom + gap) +
    "px";
}

function ssToggle(btn) {
  if (!btn || btn.disabled) return;
  const wrapper = btn.closest(".ss-wrapper");
  if (!wrapper) return;
  if (wrapper.classList.contains("ss-open")) {
    ssClose();
    return;
  }
  ssClose();
  const sel = wrapper.querySelector(".ss-hidden");
  if (!sel) return;

  const dropdown = document.createElement("div");
  dropdown.className = "ss-dropdown";

  const searchPlaceholder =
    sel.dataset.searchPlaceholder || "搜索...";
  let html =
    '<div class="ss-search">' +
    ICONS.search +
    '<input type="search" placeholder="' +
    escAttr(searchPlaceholder) +
    '" aria-label="' +
    escAttr(searchPlaceholder) +
    '" oninput="ssFilter(this)"></div>';
  html += '<div class="ss-options" role="listbox">';
  for (let i = 0; i < sel.options.length; i++) {
    const opt = sel.options[i];
    const tooltip = opt.title ? ' title="' + escAttr(opt.title) + '"' : "";
    html +=
      '<div class="ss-option' +
      (opt.selected ? " ss-selected" : "") +
      '" data-value="' +
      escAttr(opt.value) +
      '"' +
      tooltip +
      ' role="option" aria-selected="' +
      String(opt.selected) +
      '"' +
      ' onclick="ssSelect(this)">' +
      esc(opt.textContent) +
      "</div>";
  }
  html += "</div>";
  dropdown.innerHTML = html;
  enhanceFilterInputs(dropdown);

  dropdown._select = sel;
  dropdown._wrapper = wrapper;
  dropdown._options = Array.from(dropdown.querySelectorAll(".ss-option"));
  document.body.appendChild(dropdown);
  wrapper.classList.add("ss-open");
  btn.setAttribute("aria-expanded", "true");
  ssActiveWrapper = wrapper;
  ssActiveDropdown = dropdown;
  ssPositionDropdown(btn, dropdown);

  document.addEventListener("mousedown", ssOnOutsideClick, true);
  document.addEventListener("keydown", ssOnKeydown);
  document.addEventListener("scroll", ssOnViewportChange, true);
  window.addEventListener("resize", ssOnViewportChange);

  setTimeout(() => {
    if (ssActiveDropdown !== dropdown || !dropdown.isConnected) return;
    const searchInput = dropdown.querySelector(".ss-search input");
    if (searchInput) searchInput.focus();
    const opts = ssVisibleOptions(dropdown);
    const selected = opts.find((opt) => opt.classList.contains("ss-selected"));
    ssHighlight(dropdown, selected ? opts.indexOf(selected) : 0);
  }, 0);
}

function ssSelect(option) {
  const dropdown = option.closest(".ss-dropdown");
  const wrapper = dropdown && dropdown._wrapper;
  const sel = dropdown && dropdown._select;
  if (!wrapper || !sel) return;
  const value = sel.hasAttribute("data-clearable") && sel.value === option.dataset.value
    ? "" : option.dataset.value;
  sel.value = value;
  ssSyncLabel(sel);

  ssClose();
  sel.dispatchEvent(new Event("change", { bubbles: true }));
  wrapper.querySelector(".ss-trigger")?.focus();
}

function ssFilter(input) {
  if (ssFilterFrame) cancelAnimationFrame(ssFilterFrame);
  ssFilterFrame = requestAnimationFrame(() => {
    ssFilterFrame = 0;
    ssApplyFilter(input);
  });
}

function ssApplyFilter(input) {
  if (!input || !input.isConnected) return;
  const filter = input.value.toLocaleLowerCase();
  const dropdown = input.closest(".ss-dropdown");
  if (!dropdown) return;
  let visibleCount = 0;
  ssAllOptions(dropdown).forEach((opt) => {
    const text =
      opt._searchText ||
      (opt._searchText = opt.textContent.toLocaleLowerCase());
    const visible = text.includes(filter);
    opt.style.display = visible ? "" : "none";
    if (visible) visibleCount++;
  });
  let noMatch = dropdown.querySelector(".ss-no-match");
  if (visibleCount === 0) {
    if (!noMatch) {
      const optionsContainer = dropdown.querySelector(".ss-options");
      if (optionsContainer) {
        noMatch = document.createElement("div");
        noMatch.className = "ss-option ss-no-match";
        noMatch.textContent = "无匹配结果";
        optionsContainer.appendChild(noMatch);
      }
    }
  } else if (noMatch) {
    noMatch.remove();
  }
  const currentIndex = ssActiveIndex(dropdown);
  if (currentIndex === -1) {
    const opts = ssVisibleOptions(dropdown);
    const selected = opts.find((opt) =>
      opt.classList.contains("ss-selected"),
    );
    ssHighlight(dropdown, selected ? opts.indexOf(selected) : 0);
  }
}

function ssSyncLabel(sel) {
  if (!sel) return;
  const wrapper = sel.closest(".ss-wrapper");
  if (!wrapper) return;
  const label = wrapper.querySelector(".ss-label");
  if (!label) return;
  const selectedOpt = sel.options[sel.selectedIndex];
  label.textContent = selectedOpt
    ? selectedOpt.dataset.displayLabel || selectedOpt.textContent
    : "";
  const trigger = wrapper.querySelector(".ss-trigger");
  if (trigger) {
    trigger.title = selectedOpt ? selectedOpt.title || selectedOpt.textContent : "";
    const name = sel.getAttribute("aria-label");
    if (name) trigger.setAttribute("aria-label", name + "：" + label.textContent);
  }
  if (sel.hasAttribute("data-clearable")) {
    wrapper.classList.add("ss-clearable");
    wrapper.classList.toggle("has-value", !!sel.value);
    let clear = wrapper.querySelector(".ss-clear");
    if (!clear) {
      clear = document.createElement("button");
      clear.type = "button";
      clear.className = "ss-clear";
      clear.textContent = "×";
      clear.title = "清除此筛选";
      clear.setAttribute("aria-label", "清除" + (sel.getAttribute("aria-label") || "筛选"));
      clear.onclick = () => clearFilterField(sel);
      wrapper.appendChild(clear);
    }
    clear.hidden = !sel.value;
    clear.disabled = sel.disabled;
  }
}

function ssSyncDisabled(sel) {
  if (!sel) return;
  const wrapper = sel.closest(".ss-wrapper");
  const trigger = wrapper?.querySelector(".ss-trigger");
  if (!wrapper || !trigger) return;
  trigger.disabled = sel.disabled;
  wrapper.classList.toggle("ss-disabled", sel.disabled);
  if (sel.disabled && ssActiveWrapper === wrapper) ssClose();
}

function ssEnhanceSelect(sel) {
  if (!sel || sel.closest(".ss-wrapper") || sel.tagName !== "SELECT") return;
  const wrapper = document.createElement("div");
  wrapper.className = "ss-wrapper";
  sel.parentNode.insertBefore(wrapper, sel);
  sel.classList.add("ss-hidden");
  sel.tabIndex = -1;
  wrapper.appendChild(sel);
  const trigger = document.createElement("button");
  trigger.type = "button";
  trigger.className = "ss-trigger";
  trigger.setAttribute("aria-haspopup", "listbox");
  trigger.setAttribute("aria-expanded", "false");
  trigger.innerHTML =
    '<span class="ss-label"></span><svg class="ss-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>';
  trigger.onclick = function () {
    ssToggle(this);
  };
  wrapper.appendChild(trigger);
  ssSyncLabel(sel);
  ssSyncDisabled(sel);
  sel.addEventListener("change", () => ssSyncLabel(sel));
}

/* ===== 筛选交互 ===== */
const PAGE_FILTERS = {
  usage: [["usageDateFilter", "日期"], ["usageModelFilter", "请求模型"],
    ["usageUpstreamFilter", "上游"], ["usageAPIKeyFilter", "密钥"]],
  upstream: [["upstreamKeywordFilter", "关键词"], ["upstreamModelFilter", "模型"]],
};

function clearFilterField(field) {
  if (!field) return;
  if (!field.closest(".ss-dropdown")) ssClose();
  field.value = "";
  ssSyncLabel(field);
  field.dispatchEvent(new Event(field.tagName === "SELECT" || field.type === "date" ? "change" : "input", { bubbles: true }));
  (field.closest(".ss-wrapper")?.querySelector(".ss-trigger") || field).focus();
}

function updatePageFilters(scope, resultCount) {
  const container = document.getElementById(scope + "ActiveFilters");
  if (!container) return;
  const selected = (PAGE_FILTERS[scope] || []).map(([id, label]) => {
    const field = document.getElementById(id);
    return { id, label, value: String(field?.value || "").trim() };
  }).filter((item) => item.value);
  const reset = document.querySelector('[data-filter-reset="' + scope + '"]');
  if (reset) reset.disabled = !selected.length;
  container.hidden = !selected.length;
  const state = JSON.stringify(selected.map(({ id, value }) => [id, value]));
  if (container.dataset.filterState === state && resultCount == null) return;
  container.dataset.filterState = state;
  const chips = selected.map(({ id, label, value }) =>
    '<button type="button" class="filter-chip" onclick="clearFilterField(document.getElementById(\'' + id +
    '\'))" title="清除此条件" aria-label="清除' + escAttr(label + "：" + value) + '">' +
    esc(label + "：" + value) + '<span aria-hidden="true">×</span></button>').join("");
  const count = '<span class="filter-result-count" role="status">' + (resultCount == null
    ? "正在筛选…" : "匹配 " + fmt(resultCount) + (scope === "upstream" ? " 个上游" : " 条记录")) + '</span>';
  // 自动刷新时保留条件按钮，避免打断键盘焦点。
  const html = chips + count;
  if (container.innerHTML !== html) container.innerHTML = html;
}

function resetPageFilters(scope) {
  ssClose();
  (PAGE_FILTERS[scope] || []).forEach(([id]) => {
    const field = document.getElementById(id);
    if (!field) return;
    field.value = "";
    ssSyncLabel(field);
    syncInputClearButton(field);
  });
  if (scope === "usage") loadUsageRecords(0);
  else filterUpstreamRows();
}

function syncInputClearButton(input) {
  const button = input?._clearButton;
  if (button) button.hidden = !input.value;
}

function enhanceFilterInputs(root) {
  root.querySelectorAll('input[type="search"], input[data-clearable-input], .model-search input').forEach((input) => {
    if (input._clearButton) return;
    const parent = input.parentElement;
    parent.classList.add("input-clearable");
    const clear = document.createElement("button");
    clear.type = "button";
    clear.className = "input-clear";
    clear.textContent = "×";
    clear.title = "清除";
    clear.setAttribute("aria-label", "清除" + (input.getAttribute("aria-label") || input.placeholder || "搜索"));
    clear.onclick = () => clearFilterField(input);
    parent.appendChild(clear);
    input._clearButton = clear;
    input.addEventListener("input", () => syncInputClearButton(input));
    input.addEventListener("change", () => syncInputClearButton(input));
    syncInputClearButton(input);
  });
}

function searchableSelectHtml(field, options, selected, attrs) {
  const selOpt = options.find(function (o) {
    return o.value === selected;
  });
  const label = selOpt
    ? selOpt.displayLabel || selOpt.label
    : options[0]
      ? options[0].displayLabel || options[0].label
      : "";
  const selectedTooltip = selOpt
    ? selOpt.tooltip ||
      (selOpt.displayLabel && selOpt.displayLabel !== selOpt.label
        ? selOpt.label
        : "")
    : "";
  let h = '<div class="ss-wrapper">';
  h +=
    '<select data-field="' +
    field +
    '" class="ss-hidden m-select"' +
    (attrs ? " " + attrs : "") +
    ">";
  for (const opt of options) {
    const tooltip = opt.tooltip ? ' title="' + escAttr(opt.tooltip) + '"' : "";
    const displayLabel = opt.displayLabel || opt.label;
    const displayLabelAttr =
      displayLabel !== opt.label
        ? ' data-display-label="' + escAttr(displayLabel) + '"'
        : "";
    h +=
      '<option value="' +
      escAttr(opt.value) +
      '"' +
      tooltip +
      displayLabelAttr +
      (opt.value === selected ? " selected" : "") +
      ">" +
      esc(opt.label) +
      "</option>";
  }
  h += "</select>";
  h +=
    '<button type="button" class="ss-trigger" aria-haspopup="listbox" aria-expanded="false"' +
    (selectedTooltip ? ' title="' + escAttr(selectedTooltip) + '"' : "") +
    ' onclick="ssToggle(this)">';
  h += '<span class="ss-label">' + esc(label) + "</span>";
  h +=
    '<svg class="ss-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>';
  h += "</button>";
  h += "</div>";
  return h;
}

/* ===== 上游表格 ===== */
function apiTypeSelectHtml(selected) {
  const v = selected || "openai";
  return searchableSelectHtml(
    "api_type",
    [
      { value: "openai", label: "OpenAI" },
      { value: "anthropic", label: "Anthropic" },
      { value: "openai-responses", label: "Responses" },
    ],
    v,
  );
}

function bridgeModeSelectHtml(selected) {
  const v = selected || "compatible";
  return searchableSelectHtml(
    "bridge_mode",
    [
      {
        value: "compatible",
        label: "兼容(推荐)",
        tooltip:
          "兼容模式（推荐）\n客户端与上游协议不同时，网关会尽可能完成协议转换并继续请求。\n无法完整转换的字段或能力会被降级或忽略，同时通过响应头和日志给出桥接警告。\n适合优先保证请求可用性的场景。",
      },
      {
        value: "strict",
        label: "严格",
        tooltip:
          "严格模式\n跨协议转换可能丢失字段、能力或语义时，网关会在请求上游前直接拒绝。\n同协议请求不受影响，仍使用原生透传路径。\n适合要求行为一致、不能接受有损降级的场景。",
      },
    ],
    v,
  );
}

function proxySelectHtml(selected) {
  const value = String(selected || "").trim();
  const options = [{ value: "", label: "直连" }];
  const seen = new Set([""]);
  (socks5Data || []).forEach((proxy) => {
    const addr = String(proxy?.addr || "").trim();
    if (!addr || seen.has(addr)) return;
    seen.add(addr);
    const name = String(proxy?.name || "").trim();
    const fullLabel = name ? name + " (" + addr + ")" : addr;
    options.push({
      value: addr,
      label: fullLabel,
      displayLabel: name || addr,
      tooltip: fullLabel,
    });
  });
  if (value && !seen.has(value)) {
    options.push({ value: value, label: value + "（配置中不存在）" });
  }
  return searchableSelectHtml(
    "proxy",
    options,
    value,
    'data-search-placeholder="搜索代理名称或地址/端口"',
  );
}

function upstreamSearchModels(name, row) {
  const models = new Set(modelListByUpstream[name] || []);
  const configured =
    upstreamData[name] && Array.isArray(upstreamData[name].custom_models)
      ? upstreamData[name].custom_models
      : [];
  configured.forEach((model) => models.add(model));
  if (row) {
    const field = row.querySelector('[data-field="custom_models"]');
    String(field?.dataset.value || "")
      .split(",")
      .map((model) => model.trim())
      .filter(Boolean)
      .forEach((model) => models.add(model));
  }
  return Array.from(models);
}

function renderUpstreamModelFilter() {
  const select = document.getElementById("upstreamModelFilter");
  if (!select) return;
  const selected = select.value;
  const models = new Set();
  orderedUpstreamNames().forEach((name) =>
    upstreamSearchModels(name).forEach((model) => models.add(model)),
  );
  const options = Array.from(models).sort((a, b) => a.localeCompare(b));
  select.innerHTML =
    '<option value="">全部模型</option>' +
    options
      .map(
        (model) =>
          '<option value="' + escAttr(model) + '">' + esc(model) + "</option>",
      )
      .join("");
  select.value = options.includes(selected) ? selected : "";
  ssSyncLabel(select);
}

let upstreamFilterFrame = 0;

function scheduleFilterUpstreamRows() {
  if (upstreamFilterFrame) cancelAnimationFrame(upstreamFilterFrame);
  upstreamFilterFrame = requestAnimationFrame(() => {
    upstreamFilterFrame = 0;
    filterUpstreamRows();
  });
}

function filterUpstreamRows() {
  if (upstreamFilterFrame) {
    cancelAnimationFrame(upstreamFilterFrame);
    upstreamFilterFrame = 0;
  }
  const tbody = document.querySelector("#upstreamTable tbody");
  if (!tbody) return;
  const keywordInput = document.getElementById("upstreamKeywordFilter");
  const modelSelect = document.getElementById("upstreamModelFilter");
  const keyword = String(keywordInput?.value || "")
    .trim()
    .toLocaleLowerCase();
  const model = String(modelSelect?.value || "").trim();
  const rows = Array.from(tbody.querySelectorAll("tr[data-upstream-row]"));
  let visible = 0;

  tbody.querySelector(".upstream-filter-empty")?.remove();
  rows.forEach((row) => {
    const name = String(
      row.querySelector('[data-field="name"]')?.value || "",
    ).trim();
    let keywordMatched = true;
    if (keyword) {
      const values = [name];
      row.querySelectorAll("input, select, [data-value]").forEach((field) => {
        if (field instanceof HTMLSelectElement) {
          values.push(
            field.value,
            field.options[field.selectedIndex]?.textContent || "",
          );
        } else if (field.dataset && field.dataset.value !== undefined) {
          values.push(field.dataset.value);
        } else {
          values.push(field.value || "");
        }
      });
      keywordMatched = values
        .join(" ")
        .toLocaleLowerCase()
        .includes(keyword);
    }
    const modelMatched =
      !model || upstreamSearchModels(name, row).includes(model);
    const hidden = !(keywordMatched && modelMatched);
    if (row.hidden !== hidden) row.hidden = hidden;
    if (!row.hidden) visible++;
  });
  updatePageFilters("upstream", visible);

  if (rows.length && visible === 0) {
    tbody.insertAdjacentHTML(
      "beforeend",
      '<tr class="upstream-filter-empty"><td colspan="9">' +
        '<div class="upstream-filter-empty-content">' +
        ICONS.search +
        '<span>没有匹配的上游</span><button class="btn btn-default" type="button" onclick="resetPageFilters(\'upstream\')">清空筛选</button></div>' +
        "</td></tr>",
    );
  }
}

function renderUpstreamTable() {
  const tb = document.querySelector("#upstreamTable tbody");
  // The persisted order remains the routing priority; the admin list presents
  // the most recently added/dragged item first for quicker editing.
  const ks = orderedUpstreamNames().slice().reverse();
  if (!ks.length) {
    tb.innerHTML = emptyRowHtml(
      9,
      ICONS.server,
      "暂无上游配置",
      "添加第一个上游后即可开始路由请求",
    );
    renderUpstreamModelFilter();
    filterUpstreamRows();
    return;
  }
  tb.innerHTML = ks.map((name) => upstreamRowHtml(name, upstreamData[name] || {})).join("");
  renderUpstreamModelFilter();
  filterUpstreamRows();
}

function upstreamRowHtml(name, up) {
  return (
    '<tr data-upstream-row="' +
    escAttr(name) +
    '" data-upstream-original="' +
    escAttr(name) +
    '" data-upstream-id="' +
    escAttr(up.id || "") +
    '"' +
    ">" +
    '<td class="col-drag"><span class="drag-handle" draggable="true" title="拖动排序" aria-label="拖动排序">' +
    ICONS.grip +
    "</span></td>" +
    '<td><input value="' +
    escAttr(name) +
    '" data-field="name" placeholder="例如: main（留空自动获取）"></td>' +
    '<td><input value="' +
    escAttr(up.base_url || "") +
    '" data-field="base_url" placeholder="https://example.com/v1"></td>' +
    "<td>" +
    apiKeyFieldHtml(up.api_key || "") +
    "</td>" +
    "<td>" +
    apiTypeSelectHtml(up.api_type) +
    "</td>" +
    "<td>" +
    bridgeModeSelectHtml(up.bridge_mode) +
    "</td>" +
    "<td>" +
    proxySelectHtml(up.proxy) +
    "</td>" +
    "<td>" +
    customModelsFieldHtml((up.custom_models || []).join(",")) +
    "</td>" +
    '<td class="action-cell">' +
    '<button class="btn-icon model-test-btn" onclick="testUpstreamModel(this)" title="测试模型" aria-label="测试 ' +
    escAttr(name) +
    ' 的模型">' +
    ICONS.gauge +
    "</button>" +
    '<button class="btn-icon btn-icon-success" onclick="syncModels(this)" title="同步模型">' +
    ICONS.sync +
    "</button>" +
    '<button class="btn-icon btn-icon-danger" onclick="delUpstream(this)" title="删除">' +
    ICONS.trash +
    "</button>" +
    "</td>" +
    "</tr>"
  );
}

function addUpstreamRow() {
  collectUpstreams();
  resetPageFilters("upstream");
  const tb = document.querySelector("#upstreamTable tbody");
  if (tb.querySelector(".empty-hint, .empty-state")) tb.innerHTML = "";
  tb.insertAdjacentHTML(
    "afterbegin",
    upstreamRowHtml("", {}),
  );
  tb.querySelector('[data-field="base_url"]')?.focus();
}

/* ===== 批量添加上游 ===== */
function bulkUpstreamIdentity(up) {
  const parsed = parseUpstreamBaseURL(up.base_url);
  const url = parsed ? parsed.href.replace(/\/+$/, "") : String(up.base_url || "");
  return JSON.stringify([url, String(up.api_key || "").trim(), up.api_type || "openai"]);
}

function parseBulkUpstreams(value, delimiter, apiType, existing, usedNames) {
  const rows = [];
  const errors = [];
  let duplicates = 0;
  const names = new Set(usedNames || []);
  const seen = new Set((existing || []).map(bulkUpstreamIdentity));
  const lines = String(value || "").split(/\r?\n/);
  if (lines.filter((line) => line.trim() && !line.trim().startsWith("#")).length > 500) {
    return { rows, errors: ["每次最多添加 500 个上游，请分批粘贴"], duplicates };
  }
  lines.forEach((line, index) => {
    const text = line.trim();
    if (!text || text.startsWith("#")) return;
    let separator = delimiter;
    if (!separator) separator = ["|", "\t", ",", "，"].find((item) => text.includes(item)) || /\s+/;
    const parts = text.split(separator).map((part) => part.trim());
    const fail = (message) => errors.push("第 " + (index + 1) + " 行：" + message);
    if (parts.length < 2 || parts.length > 3) {
      fail("请填写 URL、Key，可选第三列名称；检查分隔符");
      return;
    }
    const [url, key, customName] = parts;
    const parsed = parseUpstreamBaseURL(url);
    if (!parsed || /\s/.test(url) || parsed.username || parsed.password) {
      fail("URL 无效，请使用 HTTP 或 HTTPS 地址");
      return;
    }
    if (!key || /\s/.test(key)) {
      fail("Key 不能为空或包含空白字符");
      return;
    }
    const up = { base_url: parsed.href.replace(/\/+$/, ""), api_key: key,
      api_type: apiType || "openai", bridge_mode: "compatible" };
    const identity = bulkUpstreamIdentity(up);
    if (seen.has(identity)) {
      duplicates++;
      return;
    }
    seen.add(identity);
    const name = uniqueUpstreamDisplayName(customName || upstreamNameFromBaseURL(up.base_url), names);
    names.add(name);
    rows.push({ name, up, line: index + 1 });
  });
  return { rows, errors, duplicates };
}

function openBulkUpstreamEditor(trigger) {
  const pop = openPopover(trigger,
    '<div class="popover-header"><div class="popover-title">' + ICONS.plus +
    '批量添加上游</div><button class="btn-icon" type="button" aria-label="关闭" onclick="closePopover()">×</button></div>' +
    '<div class="bulk-upstream-options"><label>分隔符<input class="popover-input" data-bulk-delimiter placeholder="自动（|、逗号、Tab、空格）" maxlength="10" oninput="updateBulkUpstreamPreview(this)"></label>' +
    '<label>接口类型<select class="m-select" data-bulk-type onchange="updateBulkUpstreamPreview(this)"><option value="openai">OpenAI Chat</option><option value="openai-responses">OpenAI Responses</option><option value="anthropic">Anthropic Messages</option></select></label></div>' +
    '<div class="bulk-upstream-input-head"><label class="popover-label" for="bulkUpstreamText">上游列表</label><span class="popover-hint" id="bulkUpstreamFormat">每行 URL | Key，可选第三列名称 · 最多 500 个</span></div>' +
    '<textarea id="bulkUpstreamText" class="popover-textarea bulk-upstream-text" aria-describedby="bulkUpstreamFormat" spellcheck="false" autocomplete="off" autocapitalize="off" placeholder="https://api.example.com/v1 | sk-example\nhttps://api.other.com/v1 | sk-other | 备用上游" oninput="updateBulkUpstreamPreview(this)"></textarea>' +
    '<div class="bulk-upstream-errors" role="alert" hidden></div><details class="bulk-upstream-review"><summary><span>导入预览</span><span class="bulk-upstream-summary" role="status"></span></summary><div class="bulk-upstream-preview"></div></details>' +
    '<div class="bulk-upstream-footer"><p class="popover-hint">加入后点击「保存上游」生效</p><button class="btn btn-default" type="button" onclick="closePopover()">取消</button><button class="btn btn-primary" type="button" data-bulk-submit onclick="applyBulkUpstreams(this)" disabled>加入列表</button></div>');
  pop.classList.add("bulk-upstream-popover");
  pop.parentElement.classList.add("bulk-upstream-overlay");
  ssEnhanceSelect(pop.querySelector("[data-bulk-type]"));
  updateBulkUpstreamPreview(pop);
  pop.querySelector("textarea").focus();
}

function readBulkUpstreamPlan(pop) {
  const existing = Array.from(document.querySelectorAll('#upstreamTable tr[data-upstream-row]')).map((row) => ({
    base_url: row.querySelector('[data-field="base_url"]')?.value || "",
    api_key: row.querySelector('[data-field="api_key"]')?.dataset.value || "",
    api_type: row.querySelector('[data-field="api_type"]')?.value || "openai",
  }));
  return parseBulkUpstreams(pop.querySelector("textarea").value,
    pop.querySelector("[data-bulk-delimiter]").value,
    pop.querySelector("[data-bulk-type]").value, existing, upstreamRowsUsedNames());
}

function updateBulkUpstreamPreview(source) {
  const pop = source.closest(".bulk-upstream-popover") || source;
  const plan = readBulkUpstreamPlan(pop);
  pop.querySelector(".bulk-upstream-summary").textContent =
    "可添加 " + plan.rows.length + " 个 · 跳过重复 " + plan.duplicates + " 个 · 错误 " + plan.errors.length + " 行";
  const errors = pop.querySelector(".bulk-upstream-errors");
  errors.hidden = !plan.errors.length;
  errors.textContent = plan.errors.slice(0, 20).join("\n") + (plan.errors.length > 20 ? "\n…请先修正以上错误" : "");
  const preview = pop.querySelector(".bulk-upstream-preview");
  preview.innerHTML = plan.rows.length ? '<table class="tbl"><thead><tr><th>行</th><th>名称</th><th>URL</th><th>Key</th></tr></thead><tbody>' +
    plan.rows.slice(0, 50).map(({ name, up, line }) => '<tr><td>' + line + '</td><td>' + esc(name) +
    '</td><td>' + esc(up.base_url) + '</td><td>••••' + esc(up.api_key.length > 8 ? up.api_key.slice(-4) : "") + '</td></tr>').join("") +
    '</tbody></table>' + (plan.rows.length > 50 ? '<p class="popover-hint">仅预览前 50 个，加入时包含全部有效上游。</p>' : "") : '<p class="popover-hint">暂无可预览的上游</p>';
  pop.querySelector("[data-bulk-submit]").disabled = !plan.rows.length || !!plan.errors.length;
}

function applyBulkUpstreams(button) {
  const pop = button.closest(".bulk-upstream-popover");
  const plan = readBulkUpstreamPlan(pop);
  if (plan.errors.length || !plan.rows.length) {
    updateBulkUpstreamPreview(pop);
    return;
  }
  collectUpstreams();
  const tbody = document.querySelector("#upstreamTable tbody");
  tbody.querySelectorAll("tr:not([data-upstream-row])").forEach((row) => row.remove());
  tbody.insertAdjacentHTML("afterbegin", plan.rows.map(({ name, up }) => upstreamRowHtml(name, up)).join(""));
  collectUpstreams();
  closePopover();
  resetPageFilters("upstream");
  renderUpstreamModelFilter();
  showToast("已加入 " + plan.rows.length + " 个上游，请保存上游配置", "success");
  tbody.querySelector('[data-field="name"]')?.focus();
}

/* ===== 上游拖动排序 ===== */
let upstreamDragRow = null;
let upstreamDropRow = null;
let upstreamDropAfter = false;

function clearUpstreamDropIndicator() {
  if (!upstreamDropRow) return;
  upstreamDropRow.classList.remove("drag-drop-before", "drag-drop-after");
  upstreamDropRow = null;
  upstreamDropAfter = false;
}

function setUpstreamDropIndicator(row, after) {
  if (row === upstreamDropRow && after === upstreamDropAfter) return;
  clearUpstreamDropIndicator();
  if (!row || row === upstreamDragRow) return;
  upstreamDropRow = row;
  upstreamDropAfter = after;
  row.classList.add(after ? "drag-drop-after" : "drag-drop-before");
}

function finishUpstreamDrag(commit) {
  const source = upstreamDragRow;
  const target = upstreamDropRow;
  const after = upstreamDropAfter;
  if (!source) return;

  if (commit && target && target.parentNode === source.parentNode) {
    source.parentNode.insertBefore(source, after ? target.nextSibling : target);
  }

  source.classList.remove("row-dragging");
  source.closest("tbody")?.classList.remove("is-sorting");
  clearUpstreamDropIndicator();
  upstreamDragRow = null;

  if (!commit) return;
  collectUpstreams();
  collectAliases();
  renderAliasTable();
}

function initUpstreamDragSort() {
  const tbody = document.querySelector("#upstreamTable tbody");
  if (!tbody || tbody._dragBound) return;
  tbody._dragBound = true;

  tbody.addEventListener("dragstart", (e) => {
    const handle = e.target.closest(".drag-handle");
    if (!handle) return;
    upstreamDragRow = handle.closest("tr");
    if (!upstreamDragRow) return;
    const source = upstreamDragRow;
    requestAnimationFrame(() => {
      if (upstreamDragRow === source) source.classList.add("row-dragging");
    });
    tbody.classList.add("is-sorting");
    e.dataTransfer.effectAllowed = "move";
    // Firefox 需要 setData 才会触发拖动
    try {
      e.dataTransfer.setData("text/plain", "");
    } catch (err) {}
  });

  tbody.addEventListener("dragover", (e) => {
    if (!upstreamDragRow) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    const target = e.target.closest("tr");
    if (
      !target ||
      target === upstreamDragRow ||
      target.parentNode !== tbody ||
      !target.matches("[data-upstream-row]")
    ) {
      clearUpstreamDropIndicator();
      return;
    }
    const rect = target.getBoundingClientRect();
    const after = e.clientY > rect.top + rect.height / 2;
    setUpstreamDropIndicator(target, after);
  });

  tbody.addEventListener("drop", (e) => {
    if (!upstreamDragRow) return;
    e.preventDefault();
    finishUpstreamDrag(true);
  });

  tbody.addEventListener("dragleave", (e) => {
    if (!e.relatedTarget || !tbody.contains(e.relatedTarget))
      clearUpstreamDropIndicator();
  });

  tbody.addEventListener("dragend", () => {
    finishUpstreamDrag(false);
  });
}

function showDangerConfirm(triggerEl, options) {
  const settings = options || {};
  const title = settings.title || "确认执行此操作？";
  const subject = String(settings.subject || "").trim();
  const description = settings.description || "请确认是否继续。";
  const note = settings.note || "此操作可能无法撤销。";
  const confirmLabel = settings.confirmLabel || "确认删除";
  const details = Array.isArray(settings.details)
    ? settings.details.map((item) => String(item || "").trim()).filter(Boolean)
    : [];
  const detailsTitle = settings.detailsTitle || "受影响的配置";
  return new Promise((resolve) => {
    let settled = false;
    const pop = openPopover(
      triggerEl,
      '<div class="danger-confirm-head"><span class="danger-confirm-icon">' +
        ICONS.trash +
        '</span><div><h2 id="dangerConfirmTitle">' +
        esc(title) +
        '</h2><p>' +
        esc(description) +
        "</p></div></div>" +
        (subject
          ? '<div class="danger-confirm-subject" title="' +
            escAttr(subject) +
            '"><span>' +
            ICONS.alert +
            "</span><strong>" +
            esc(subject) +
            "</strong></div>"
          : "") +
        (details.length
          ? '<div class="danger-confirm-details"><div class="danger-confirm-details-title">' +
            esc(detailsTitle) +
            "</div><ul>" +
            details.map((item) => "<li>" + esc(item) + "</li>").join("") +
            "</ul></div>"
          : "") +
        '<div class="danger-confirm-note"><span>' +
        ICONS.alert +
        "</span><p>" +
        esc(note) +
        "</p></div>" +
        '<div class="danger-confirm-actions"><button type="button" class="btn danger-confirm-cancel">取消</button>' +
        '<button type="button" class="btn danger-confirm-submit">' +
        ICONS.trash +
        "<span>" +
        esc(confirmLabel) +
        "</span></button></div>",
      () => {
        if (settled) return;
        settled = true;
        resolve(false);
      },
    );
    pop.classList.add("danger-confirm-popover");
    pop.setAttribute("role", "alertdialog");
    pop.setAttribute("aria-modal", "true");
    pop.setAttribute("aria-labelledby", "dangerConfirmTitle");

    const finish = (confirmed) => {
      if (settled) return;
      settled = true;
      closePopover();
      resolve(confirmed);
    };
    pop
      .querySelector(".danger-confirm-cancel")
      ?.addEventListener("click", () => finish(false));
    pop
      .querySelector(".danger-confirm-submit")
      ?.addEventListener("click", () => finish(true));
    setTimeout(() => pop.querySelector(".danger-confirm-cancel")?.focus(), 0);
  });
}

function confirmConfigDelete(triggerEl, itemType, itemName) {
  return showDangerConfirm(triggerEl, {
    title: "确认删除" + itemType + "？",
    subject: itemName,
    description: "该项目将从当前配置列表中移除。",
    note: "删除后需保存配置才会生效；保存前可通过刷新页面恢复。",
    confirmLabel: "确认删除",
  });
}

function upstreamAliasDeleteImpact(upstreamName) {
  const upstreamNames = new Set(
    (Array.isArray(upstreamName) ? upstreamName : [upstreamName]).map((name) =>
      String(name || "").trim(),
    ),
  );
  const aliases = [];
  let targetCount = 0;
  Object.keys(aliasData || {}).forEach((aliasName) => {
    const entry = aliasData[aliasName] || {};
    const targets = normalizeAliasTargets(entry.targets || []);
    const removedTargets = targets.filter(
      (target) => upstreamNames.has(target.upstream),
    );
    if (!removedTargets.length) return;
    const remainingTargets = targets.filter(
      (target) => !upstreamNames.has(target.upstream),
    );
    const unroutable =
      remainingTargets.length > 0 &&
      !remainingTargets.some((target) => target.weight > 0);
    targetCount += removedTargets.length;
    aliases.push({
      aliasName: aliasName,
      removedTargets: removedTargets,
      remainingTargets: remainingTargets,
      removeAlias: remainingTargets.length === 0,
      unroutable: unroutable,
    });
  });
  return {
    aliases: aliases,
    targetCount: targetCount,
    removedAliasCount: aliases.filter((item) => item.removeAlias).length,
    unroutableAliasCount: aliases.filter((item) => item.unroutable).length,
  };
}

function upstreamDeleteConfirm(triggerEl, upstreamName, impact) {
  if (!impact.targetCount) {
    return showDangerConfirm(triggerEl, {
      title: "确认删除上游？",
      subject: upstreamName,
      description: "该上游将从当前配置列表中移除，没有关联的模型映射需要删除。",
      note: "删除后需保存配置才会生效；保存前可通过刷新页面恢复。",
      confirmLabel: "确认删除",
    });
  }

  const aliasCount = impact.aliases.length;
  const removedAliasCount = impact.removedAliasCount;
  const unroutableAliasCount = impact.unroutableAliasCount || 0;
  let description =
    "该上游将从当前配置列表中移除，同时删除 " +
    impact.targetCount +
    " 个关联的上游模型映射，涉及 " +
    aliasCount +
    " 个模型别名。";
  if (removedAliasCount) {
    description +=
      "其中 " +
      removedAliasCount +
      " 个别名删除这些映射后已无可用目标，也会一并删除。";
  }
  if (unroutableAliasCount) {
    description +=
      "另有 " +
      unroutableAliasCount +
      " 个别名的剩余模型权重均为 0，将保留配置并在模型映射列表中提醒。";
  }
  return showDangerConfirm(triggerEl, {
    title: "确认删除上游及关联模型映射？",
    subject: upstreamName,
    description: description,
    detailsTitle: "将删除的模型映射（" + impact.targetCount + " 个）",
    details: impact.aliases.map((item) => {
      const models = item.removedTargets
        .map((target) => target.target_model)
        .join("、");
      return (
        item.aliasName +
        " → " +
        models +
        (item.removeAlias
          ? "（模型别名也将删除）"
          : item.unroutable
            ? "（删除后权重均为 0）"
            : "")
      );
    }),
    note: "删除后需保存配置才会生效；保存前可通过刷新页面恢复。",
    confirmLabel: "删除上游及映射",
  });
}

function applyUpstreamAliasDelete(impact) {
  let selectedAliasRemoved = false;
  impact.aliases.forEach((item) => {
    const entry = aliasData[item.aliasName];
    if (!entry) return;
    if (item.removeAlias) {
      delete aliasData[item.aliasName];
      if (selectedAliasKey === item.aliasName) selectedAliasRemoved = true;
      return;
    }
    aliasData[item.aliasName] = {
      ...entry,
      targets: item.remainingTargets,
    };
  });
  if (selectedAliasRemoved) selectedAliasKey = "";
  return selectedAliasRemoved;
}

async function delUpstream(btn) {
  const row = btn.closest("tr");
  const ni = row.querySelector('[data-field="name"]');
  const upstreamName = ni?.value?.trim() || "";
  const originalUpstreamName = String(
    row.dataset.upstreamOriginal || row.dataset.upstreamRow || "",
  ).trim();
  collectAliases();
  const impact = upstreamAliasDeleteImpact([
    upstreamName,
    originalUpstreamName,
  ]);
  if (!(await upstreamDeleteConfirm(btn, upstreamName, impact))) return;
  const selectedAliasRemoved = applyUpstreamAliasDelete(impact);
  const deletedBaseURL = row.querySelector('[data-field="base_url"]');
  if (deletedBaseURL?._upstreamNameTimer) clearTimeout(deletedBaseURL._upstreamNameTimer);
  cancelUpstreamSiteInfoFetch(deletedBaseURL);
  row.remove();
  collectUpstreams();
  if (upstreamName) delete modelListByUpstream[upstreamName];
  if (!Object.keys(upstreamData).length)
    document.querySelector("#upstreamTable tbody").innerHTML = emptyRowHtml(
      9,
      ICONS.server,
      "暂无上游配置",
      "添加第一个上游后即可开始路由请求",
    );
  renderUpstreamModelFilter();
  filterUpstreamRows();
  renderAliasTable();
  if (selectedAliasRemoved) showSelectedEffortMap();
}

// Parse an upstream URL once and keep the URL used for site-level metadata
// separate from the configured API path. An upstream is commonly entered as
// `https://provider.example/v1`; the `/v1` suffix belongs to the API and must
// not affect the display name or the site-info lookup.
function parseUpstreamBaseURL(baseURL) {
  const raw = String(baseURL || "").trim();
  if (!raw) return null;

  const candidates = [raw];
  // The form accepts host-only values for convenience (for example
  // `api.example.com/v1`). URL requires a scheme, so retry with https.
  if (!/^[a-z][a-z\d+.-]*:\/\//i.test(raw)) {
    candidates.push("https://" + raw.replace(/^\/+/, ""));
  }
  for (const candidate of candidates) {
    try {
      const parsed = new URL(candidate);
      if (!/^https?:$/.test(parsed.protocol) || !parsed.hostname) continue;
      return parsed;
    } catch (e) {}
  }
  return null;
}

// Return the site origin for metadata requests. Keeping this helper small
// also makes it safe for callers to use when a Base URL contains `/v1` or a
// deeper API path.
function upstreamSiteInfoURL(baseURL) {
  const parsed = parseUpstreamBaseURL(baseURL);
  return parsed ? parsed.origin : "";
}

function upstreamNameFromBaseURL(baseURL) {
  const parsed = parseUpstreamBaseURL(baseURL);
  const hostname = parsed ? parsed.hostname : "";
  const port = parsed ? parsed.port : "";

  const domain = String(hostname || "")
    .toLowerCase()
    .replace(/^\[|\]$/g, "")
    .replace(/[^a-z0-9.\u00a0-\uffff-]+/g, "-")
    .replace(/^[.-]+|[.-]+$/g, "")
    .replace(/-{2,}/g, "-");
  const address = [domain || "upstream", port].filter(Boolean).join("-");
  return address + "-upstream";
}

function upstreamRowsUsedNames(exceptRow) {
  const names = new Set();
  document
    .querySelectorAll("#upstreamTable tbody tr[data-upstream-row]")
    .forEach((row) => {
      if (row === exceptRow) return;
      const name = String(
        row.querySelector('[data-field="name"]')?.value || "",
      ).trim();
      if (name) names.add(name);
    });
  return names;
}

function autoFillUpstreamName(baseURLEl) {
  const row = baseURLEl?.closest("tr[data-upstream-row]");
  const nameEl = row?.querySelector('[data-field="name"]');
  if (!row || !nameEl) return;

  // Do not overwrite a name the administrator entered. A generated name is
  // tagged so changing the URL can update it to the new host; clearing the
  // field also opts back into automatic naming.
  const currentName = String(nameEl.value || "").trim();
  if (currentName && row.dataset.upstreamNameGenerated !== "1") return;
  if (!parseUpstreamBaseURL(baseURLEl.value)) return;

  const usedNames = upstreamRowsUsedNames(row);
  const nextName = uniqueUpstreamName(baseURLEl.value, usedNames);
  if (!nextName || nextName === "upstream-upstream") return;
  if (nameEl.value !== nextName) nameEl.value = nextName;
  row.dataset.upstreamNameGenerated = "1";
  row.dataset.upstreamRow = nextName;
}

function scheduleUpstreamNameAutofill(baseURLEl, immediate) {
  if (!baseURLEl) return;
  if (baseURLEl._upstreamNameTimer) {
    clearTimeout(baseURLEl._upstreamNameTimer);
    baseURLEl._upstreamNameTimer = null;
  }
  if (immediate) {
    autoFillUpstreamName(baseURLEl);
    return;
  }
  // Debounce typing so transient, incomplete values are less likely to become
  // a generated name before the user finishes entering the host.
  baseURLEl._upstreamNameTimer = setTimeout(() => {
    baseURLEl._upstreamNameTimer = null;
    autoFillUpstreamName(baseURLEl);
  }, 350);
}

function uniqueUpstreamDisplayName(baseName, usedNames) {
  const clean = String(baseName || "").trim();
  if (!clean) return "";
  let name = clean;
  let suffix = 2;
  while (usedNames.has(name)) {
    name = clean + "-" + suffix;
    suffix += 1;
  }
  return name;
}

async function fetchUpstreamSiteName(baseURLEl) {
  const row = baseURLEl?.closest("tr[data-upstream-row]");
  const nameEl = row?.querySelector('[data-field="name"]');
  const siteURL = upstreamSiteInfoURL(baseURLEl?.value || "");
  if (!row || !nameEl || !siteURL) return;

  // Names entered by an administrator are authoritative. Generated names can
  // be replaced by the site's title when the metadata request succeeds.
  if (String(nameEl.value || "").trim() && row.dataset.upstreamNameGenerated !== "1") {
    return;
  }
  if (baseURLEl._upstreamSiteInfoOrigin === siteURL && baseURLEl._upstreamSiteInfoPending) {
    return;
  }
  baseURLEl._upstreamSiteInfoOrigin = siteURL;
  baseURLEl._upstreamSiteInfoPending = true;
  baseURLEl._upstreamSiteInfoSequence = (baseURLEl._upstreamSiteInfoSequence || 0) + 1;
  const sequence = baseURLEl._upstreamSiteInfoSequence;
  baseURLEl._upstreamSiteInfoController?.abort();
  const controller = new AbortController();
  baseURLEl._upstreamSiteInfoController = controller;
  try {
    const result = await apiJSON(
      "/api/site-info?url=" + encodeURIComponent(siteURL),
      { signal: controller.signal },
    );
    if (
      sequence !== baseURLEl._upstreamSiteInfoSequence ||
      controller.signal.aborted ||
      String(nameEl.value || "").trim() && row.dataset.upstreamNameGenerated !== "1"
    ) {
      return;
    }
    const siteName = String(result?.name || "").trim();
    if (!siteName) return;
    const nextName = uniqueUpstreamDisplayName(
      siteName,
      upstreamRowsUsedNames(row),
    );
    if (!nextName) return;
    nameEl.value = nextName;
    row.dataset.upstreamNameGenerated = "1";
    row.dataset.upstreamRow = nextName;
  } catch (e) {
    // Metadata is an enhancement only. Keep the domain-based generated name
    // when the site is unavailable, does not allow requests, or times out.
    if (e?.name === "AbortError") return;
    if (String(e.message || "").indexOf("登录已失效") !== -1) return;
  } finally {
    if (sequence === baseURLEl._upstreamSiteInfoSequence) {
      baseURLEl._upstreamSiteInfoPending = false;
    }
  }
}

function cancelUpstreamSiteInfoFetch(baseURLEl) {
  if (!baseURLEl) return;
  baseURLEl._upstreamSiteInfoController?.abort();
  baseURLEl._upstreamSiteInfoController = null;
  baseURLEl._upstreamSiteInfoPending = false;
  baseURLEl._upstreamSiteInfoOrigin = "";
  baseURLEl._upstreamSiteInfoSequence = (baseURLEl._upstreamSiteInfoSequence || 0) + 1;
}

function scheduleUpstreamSiteInfoFetch(baseURLEl, immediate) {
  if (!baseURLEl) return;
  if (baseURLEl._upstreamSiteInfoTimer) {
    clearTimeout(baseURLEl._upstreamSiteInfoTimer);
    baseURLEl._upstreamSiteInfoTimer = null;
  }
  const run = () => {
    baseURLEl._upstreamSiteInfoTimer = null;
    fetchUpstreamSiteName(baseURLEl);
  };
  if (!upstreamSiteInfoURL(baseURLEl.value || "")) {
    cancelUpstreamSiteInfoFetch(baseURLEl);
    return;
  }
  if (immediate) {
    run();
  } else {
    baseURLEl._upstreamSiteInfoTimer = setTimeout(run, 650);
  }
}

function initUpstreamNameAutofill() {
  const tbody = document.querySelector("#upstreamTable tbody");
  if (!tbody || tbody._upstreamNameAutofillBound) return;
  tbody._upstreamNameAutofillBound = true;
  tbody.addEventListener("input", (event) => {
    const field = event.target;
    if (!(field instanceof HTMLInputElement)) return;
    if (field.matches('[data-field="base_url"]')) {
      scheduleUpstreamNameAutofill(field, false);
      scheduleUpstreamSiteInfoFetch(field, false);
    } else if (field.matches('[data-field="name"]')) {
      const row = field.closest("tr[data-upstream-row]");
      if (!row) return;
      if (String(field.value || "").trim()) {
        delete row.dataset.upstreamNameGenerated;
        cancelUpstreamSiteInfoFetch(row.querySelector('[data-field="base_url"]'));
      }
    }
  });
  tbody.addEventListener("change", (event) => {
    const field = event.target;
    if (field instanceof HTMLInputElement && field.matches('[data-field="base_url"]')) {
      scheduleUpstreamNameAutofill(field, true);
      scheduleUpstreamSiteInfoFetch(field, true);
    }
  });
  tbody.addEventListener(
    "blur",
    (event) => {
      const field = event.target;
      if (field instanceof HTMLInputElement && field.matches('[data-field="base_url"]')) {
        scheduleUpstreamNameAutofill(field, true);
        scheduleUpstreamSiteInfoFetch(field, true);
      }
    },
    true,
  );
}

function uniqueUpstreamName(baseURL, usedNames) {
  const baseName = upstreamNameFromBaseURL(baseURL);
  let name = baseName;
  let suffix = 2;
  while (usedNames.has(name)) {
    name = baseName + "-" + suffix;
    suffix += 1;
  }
  return name;
}

function collectUpstreams() {
  const r = Object.create(null);
  const order = [];
  const rows = Array.from(
    document.querySelectorAll("#upstreamTable tbody tr"),
  );
  const usedNames = new Set(
    rows
      .map((tr) =>
        String(tr.querySelector('[data-field="name"]')?.value || "").trim(),
      )
      .filter(Boolean),
  );
  rows.forEach((tr) => {
    const nameEl = tr.querySelector('[data-field="name"]');
    const baseURLEl = tr.querySelector('[data-field="base_url"]');
    const baseURL = baseURLEl ? baseURLEl.value.trim() : "";
    if (!baseURL) return;
    const originalName = String(
      tr.dataset.upstreamOriginal || tr.dataset.upstreamRow || "",
    ).trim();
    let name = nameEl ? nameEl.value.trim() : "";
    if (!name) {
      name = uniqueUpstreamName(baseURL, usedNames);
      usedNames.add(name);
      if (nameEl) {
        nameEl.value = name;
        tr.dataset.upstreamNameGenerated = "1";
      }
      tr.dataset.upstreamRow = name;
    }
    if (originalName && originalName !== name) {
      pendingUpstreamRenames[originalName] = name;
    }
    const apiKeyEl = tr.querySelector('[data-field="api_key"]');
    const apiKey = apiKeyEl ? (apiKeyEl.dataset.value || "").trim() : "";
    const apiTypeEl = tr.querySelector('[data-field="api_type"]');
    const apiType = apiTypeEl ? apiTypeEl.value : "openai";
    const bridgeModeEl = tr.querySelector('[data-field="bridge_mode"]');
    const bridgeMode = bridgeModeEl ? bridgeModeEl.value : "compatible";
    const proxyEl = tr.querySelector('[data-field="proxy"]');
    const proxy = proxyEl ? proxyEl.value : "";
    const customEl = tr.querySelector('[data-field="custom_models"]');
    const customRaw = customEl ? (customEl.dataset.value || "").trim() : "";
    // The table does not expose provider capability declarations yet. Keep
    // the persisted value intact when the form rebuilds an upstream object.
    const previousUpstream =
      (originalName && upstreamData[originalName]) || upstreamData[name] || {};
    // Start from the persisted object so fields without an editor (retry
    // policy, Responses reasoning format, future provider flags, etc.) are
    // not erased by an ordinary admin save.
    const up = {
      ...previousUpstream,
      base_url: baseURL,
      api_type: apiType,
      bridge_mode: bridgeMode,
      proxy: proxy,
    };
    delete up.api_key;
    delete up.custom_models;
    if (
      Object.prototype.hasOwnProperty.call(previousUpstream, "capabilities") &&
      previousUpstream.capabilities &&
      typeof previousUpstream.capabilities === "object" &&
      !Array.isArray(previousUpstream.capabilities)
    ) {
      up.capabilities = { ...previousUpstream.capabilities };
    }
    const upstreamID = Number.parseInt(tr.dataset.upstreamId || "", 10);
    if (Number.isSafeInteger(upstreamID) && upstreamID > 0) up.id = upstreamID;
    if (apiKey) up.api_key = apiKey;
    if (customRaw)
      up.custom_models = customRaw
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean);
    r[name] = up;
    if (!order.includes(name)) order.push(name);
  });
  upstreamData = r;
  upstreamOrder = order.slice().reverse();
  syncPendingUpstreamRenames();
  return r;
}

/* ===== API Key 字段（点击弹层编辑） ===== */
function apiKeyFieldHtml(value) {
  return (
    '<div class="field-display" data-field="api_key" data-value="' +
    escAttr(value) +
    '" onclick="openApiKeyEditor(this)">' +
    apiKeyDisplay(value) +
    '<span class="field-edit-icon">' +
    ICONS.key +
    "</span>" +
    "</div>"
  );
}

function apiKeyDisplay(value) {
  if (!value || !value.trim())
    return '<span class="field-placeholder">点击编辑 API Key</span>';
  const lines = value
    .trim()
    .split("\n")
    .filter((l) => l.trim());
  if (lines.length === 0)
    return '<span class="field-placeholder">点击编辑 API Key</span>';
  const first = lines[0].substring(0, 14);
  if (lines.length === 1) {
    return (
      '<span class="field-value"><span class="field-text">' +
      esc(first) +
      "...</span></span>"
    );
  }
  return (
    '<span class="field-value"><span class="field-text">' +
    esc(first) +
    '...</span><span class="field-count">' +
    lines.length +
    "</span></span>"
  );
}

function openApiKeyEditor(el) {
  const value = el.dataset.value || "";
  const pop = openPopover(
    el,
    '<div class="popover-header"><span class="popover-title">' +
      ICONS.key +
      "API Key 编辑</span></div>" +
      '<textarea class="popover-textarea" placeholder="每行一个 key" rows="6">' +
      esc(value) +
      "</textarea>" +
      '<div class="popover-hint">每行输入一个 API Key，支持多个 Key 轮换使用</div>',
    function () {
      const ta = pop.querySelector("textarea");
      el.dataset.value = ta.value;
      el.innerHTML =
        apiKeyDisplay(ta.value) +
        '<span class="field-edit-icon">' +
        ICONS.key +
        "</span>";
    },
  );
  pop.classList.add("api-key-popover");
  setTimeout(() => {
    const ta = pop.querySelector("textarea");
    ta.focus();
    ta.setSelectionRange(ta.value.length, ta.value.length);
  }, 0);
}

/* ===== 自定义模型字段（点击弹层选择） ===== */
function parseModelIDs(value) {
  return Array.from(
    new Set(
      String(value || "")
        .split(/[\n,]+/)
        .map((model) => model.trim())
        .filter(Boolean),
    ),
  );
}

function customModelsTitle(models) {
  return models.length
    ? models.length + " 个模型\n" + models.join("\n")
    : "编辑模型";
}

function customModelsFieldHtml(value) {
  const models = parseModelIDs(value);
  const title = customModelsTitle(models);
  return (
    '<div class="field-display" data-field="custom_models" data-value="' +
    escAttr(value) +
    '" title="' +
    escAttr(title) +
    '" aria-label="' +
    escAttr(title) +
    '" onclick="openCustomModelsEditor(this)">' +
    customModelsDisplay(value) +
    '<span class="field-edit-icon">' +
    ICONS.layers +
    "</span>" +
    "</div>"
  );
}

function customModelsDisplay(value) {
  const models = parseModelIDs(value);
  if (models.length === 0)
    return '<span class="field-placeholder">点击编辑模型</span>';
  const remaining = models.length - 1;
  return (
    '<span class="field-value"><span class="field-text">' +
    esc(models[0]) +
    "</span>" +
    (remaining > 0
      ? '<span class="field-count" title="另有 ' +
        remaining +
        ' 个模型">+' +
        remaining +
        "</span>"
      : "") +
    "</span>"
  );
}

function reasoningSwitchHtml(checked) {
  return (
    '<label class="switch" title="启用思维链">' +
    '<input type="checkbox" data-field="with_reasoning"' +
    (checked ? " checked" : "") +
    ">" +
    '<span class="switch-slider"></span>' +
    "</label>"
  );
}

function getUpstreamNameForRow(el) {
  const row = el.closest("tr");
  if (!row) return "";
  const nameInput = row.querySelector('[data-field="name"]');
  return nameInput ? nameInput.value.trim() : "";
}

function openCustomModelsEditor(el) {
  const value = el.dataset.value || "";
  const upstreamName = getUpstreamNameForRow(el);
  const selected = parseModelIDs(value);
  const available = modelsForUpstream(upstreamName);

  const pop = openPopover(
    el,
    buildCustomModelsPopoverHtml(available, selected, "", upstreamName),
    function () {
      const checked = Array.from(
        pop.querySelectorAll(".model-check input:checked"),
      ).map((c) => c.value);
      const manualVal = pop.querySelector(".popover-input").value;
      const manualModels = parseModelIDs(manualVal);
      const combined = Array.from(new Set([...checked, ...manualModels]));
      el.dataset.value = combined.join(",");
      el.title = customModelsTitle(combined);
      el.setAttribute("aria-label", customModelsTitle(combined));
      el.innerHTML =
        customModelsDisplay(combined.join(",")) +
        '<span class="field-edit-icon">' +
        ICONS.layers +
        "</span>";
      collectUpstreams();
      collectAliases();
      renderUpstreamModelFilter();
      filterUpstreamRows();
      renderAliasTable();
    },
  );

  pop.classList.add("model-picker-popover");
  pop._upstreamName = upstreamName;
  setTimeout(() => {
    updateModelSelectionSummary(pop);
    const search = pop.querySelector(".model-search input");
    if (search) search.focus();
  }, 0);
}

function buildCustomModelsPopoverHtml(
  available,
  selected,
  manualText,
  upstreamName,
) {
  const displayModels = mergeModelOptions(available, selected);
  let modelsHtml = "";
  if (displayModels.length === 0) {
    modelsHtml =
      '<div class="model-empty">暂无可用模型，请手动输入或点击刷新</div>';
  } else {
    modelsHtml = buildModelChecksHtml(displayModels, selected, available);
  }
  return (
    '<div class="popover-header">' +
    '<span class="popover-title">' +
    ICONS.layers +
    '<span class="model-popover-heading"><span>模型</span><strong title="' +
    escAttr(upstreamName || "未命名上游") +
    '">' +
    esc(upstreamName || "未命名上游") +
    "</strong></span>" +
    '<span class="model-selection-summary" data-total="' +
    displayModels.length +
    '">已选 ' +
    selected.length +
    " · 共 " +
    displayModels.length +
    "</span></span>" +
    '<button class="btn-icon" onclick="refreshModelsInPopover(this)" title="刷新模型列表">' +
    ICONS.refresh +
    "</button>" +
    "</div>" +
    '<div class="model-search-row">' +
    '<div class="model-search">' +
    ICONS.search +
    '<input type="text" placeholder="搜索模型..." oninput="filterModelList(this)"></div>' +
    '<div class="model-tools">' +
    '<button type="button" class="model-tool-btn" onclick="selectVisibleModels(this)" title="全选当前搜索结果">' +
    '<span class="model-select-visible-icon">' +
    ICONS.check +
    "</span>" +
    '<span class="model-select-visible-label">全选当前</span></button>' +
    '<button type="button" class="model-tool-btn model-clear-btn" onclick="clearSelectedModels(this)" title="取消所有已选模型">' +
    ICONS.close +
    "<span>取消已选</span></button>" +
    '<button type="button" class="model-tool-btn" onclick="copyVisibleModelIds(this)" title="复制当前搜索结果的模型 ID">' +
    ICONS.copy +
    "<span>复制当前ID</span></button>" +
    "</div>" +
    "</div>" +
    '<div class="model-list">' +
    modelsHtml +
    "</div>" +
    '<div class="popover-label">手动输入（逗号分隔）</div>' +
    '<input class="popover-input" placeholder="model-a, model-b" value="' +
    escAttr(manualText) +
    '" oninput="updateModelSelectionSummary(this)">' +
    '<div class="popover-hint">列表包含上游最新模型；已勾选但上游未返回的模型会保留显示</div>'
  );
}

function mergeModelOptions(available, selected) {
  const merged = [];
  const seen = new Set();
  (available || []).forEach((m) => {
    if (!seen.has(m)) {
      seen.add(m);
      merged.push(m);
    }
  });
  (selected || []).forEach((m) => {
    if (!seen.has(m)) {
      seen.add(m);
      merged.push(m);
    }
  });
  return merged;
}

function buildModelChecksHtml(models, selected, available) {
  const availableSet = new Set(available || []);
  return models
    .map((m) => {
      const checked = selected.includes(m) ? " checked" : "";
      const missing = !availableSet.has(m);
      const missingClass = missing ? " model-check-missing" : "";
      const missingBadge = missing
        ? '<span class="model-missing-badge">未在上游返回</span>'
        : "";
      return (
        '<div class="model-check' +
        missingClass +
        '">' +
        '<label class="model-check-label"><input type="checkbox" onchange="updateModelSelectionSummary(this)" value="' +
        escAttr(m) +
        '"' +
        checked +
        '><span class="model-id" title="' +
        escAttr(m) +
        '">' +
        esc(m) +
        "</span>" +
        missingBadge +
        "</label>" +
        '<button type="button" class="btn-icon model-copy-btn" onclick="copyModelId(this)" data-model-id="' +
        escAttr(m) +
        '" title="复制模型 ID">' +
        ICONS.copy +
        "</button>" +
        "</div>"
      );
    })
    .join("");
}

function getCheckedModelsFromPopover(pop) {
  return Array.from(pop.querySelectorAll(".model-check input:checked")).map(
    (c) => c.value,
  );
}

function updateModelSelectionSummary(source) {
  const pop = source?.classList?.contains("popover")
    ? source
    : source?.closest?.(".popover");
  if (!pop) return;
  const checked = getCheckedModelsFromPopover(pop);
  const manual = parseModelIDs(pop.querySelector(".popover-input")?.value || "");
  const selectedCount = new Set([...checked, ...manual]).size;
  const visibleRows = getVisibleModelRows(pop);
  const summary = pop.querySelector(".model-selection-summary");
  const total = Number(summary?.dataset.total || pop.querySelectorAll(".model-check").length);
  if (summary) summary.textContent = "已选 " + selectedCount + " · 共 " + total;

  const selectButton = pop.querySelector(".model-select-visible-label")?.closest("button");
  if (selectButton) {
    const allVisibleSelected =
      visibleRows.length > 0 &&
      visibleRows.every((row) => row.querySelector('input[type="checkbox"]')?.checked);
    selectButton.disabled = visibleRows.length === 0;
    selectButton.title = allVisibleSelected
      ? "取消当前搜索结果的选择"
      : "全选当前搜索结果";
    const label = selectButton.querySelector(".model-select-visible-label");
    if (label) label.textContent = allVisibleSelected ? "取消当前" : "全选当前";
    const icon = selectButton.querySelector(".model-select-visible-icon");
    if (icon) icon.innerHTML = allVisibleSelected ? ICONS.close : ICONS.check;
  }

  const clearButton = pop.querySelector(".model-clear-btn");
  if (clearButton) clearButton.disabled = selectedCount === 0;
}

function filterModelList(input) {
  const filter = input.value.toLowerCase();
  const pop = input.closest(".popover");
  if (!pop) return;
  let visibleCount = 0;
  pop.querySelectorAll(".model-check").forEach((label) => {
    const text = label.textContent.toLowerCase();
    const visible = text.includes(filter);
    label.style.display = visible ? "" : "none";
    if (visible) visibleCount++;
  });
  let empty = pop.querySelector(".model-filter-empty");
  if (filter && visibleCount === 0 && pop.querySelector(".model-check")) {
    if (!empty) {
      empty = document.createElement("div");
      empty.className = "model-filter-empty";
      empty.textContent = "没有匹配的模型";
      pop.querySelector(".model-list")?.appendChild(empty);
    }
  } else if (empty) {
    empty.remove();
  }
  updateModelSelectionSummary(pop);
}

function getVisibleModelRows(pop) {
  return Array.from(pop.querySelectorAll(".model-check")).filter(
    (row) => row.style.display !== "none",
  );
}

function selectVisibleModels(btn) {
  const pop = btn.closest(".popover");
  if (!pop) return;
  const rows = getVisibleModelRows(pop);
  const allSelected =
    rows.length > 0 &&
    rows.every((row) => row.querySelector('input[type="checkbox"]')?.checked);
  let count = 0;
  rows.forEach((row) => {
    const check = row.querySelector('input[type="checkbox"]');
    if (check) {
      check.checked = !allSelected;
      count++;
    }
  });
  updateModelSelectionSummary(pop);
  if (count)
    showToast(
      (allSelected ? "已取消当前筛选的 " : "已选中当前筛选的 ") +
        count +
        " 个模型",
      "success",
    );
  else showToast("当前没有可选模型", "error");
}

function clearSelectedModels(btn) {
  const pop = btn.closest(".popover");
  if (!pop) return;
  const selected = new Set([
    ...getCheckedModelsFromPopover(pop),
    ...parseModelIDs(pop.querySelector(".popover-input")?.value || ""),
  ]);
  pop.querySelectorAll(".model-check input:checked").forEach((checkbox) => {
    checkbox.checked = false;
  });
  const manualInput = pop.querySelector(".popover-input");
  if (manualInput) manualInput.value = "";
  updateModelSelectionSummary(pop);
  showToast("已取消 " + selected.size + " 个已选模型", "success");
}

async function copyVisibleModelIds(btn) {
  const pop = btn.closest(".popover");
  if (!pop) return;
  const ids = getVisibleModelRows(pop)
    .map((row) => {
      const check = row.querySelector('input[type="checkbox"]');
      return check ? check.value : "";
    })
    .filter(Boolean);
  if (!ids.length) {
    showToast("当前没有可复制的模型 ID", "error");
    return;
  }
  const ok = await copyText(ids.join("\n"));
  showToast(
    ok ? "已复制 " + ids.length + " 个模型 ID" : "复制失败，请手动复制",
    ok ? "success" : "error",
  );
}

async function copyModelId(btn) {
  const id = btn.dataset.modelId || "";
  if (!id) return;
  const ok = await copyText(id);
  showToast(
    ok ? "模型 ID 已复制" : "复制失败，请手动复制",
    ok ? "success" : "error",
  );
}

function upstreamProtocolLabel(apiType) {
  switch (apiType) {
    case "anthropic":
      return "Anthropic Messages";
    case "openai-responses":
      return "OpenAI Responses";
    default:
      return "OpenAI Chat";
  }
}

function testModelsForUpstream(upstreamName) {
  return mergeModelOptions(
    selectedModelsForUpstream(upstreamName),
    modelsForUpstream(upstreamName),
  );
}

function buildModelTestOptionsHtml(models, selected) {
  if (!models.length) {
    return '<option value="" selected>暂无模型，请先同步</option>';
  }
  return models
    .map(
      (model) =>
        '<option value="' +
        escAttr(model) +
        '"' +
        (model === selected ? " selected" : "") +
        ">" +
        esc(model) +
        "</option>",
    )
    .join("");
}

const MODEL_TEST_PROMPTS = [
  { title: "快速检查", description: "了解模型能力与回答风格", icon: "sparkles",
    prompt: "请用简洁的中文介绍你的能力，并列出三个你擅长处理的任务。" },
  { title: "上下文记忆", description: "试试连续对话与信息记忆", icon: "inbox",
    prompt: "请记住：我的项目叫「星河」，正在开发一个 AI 网关，我偏好简洁的中文回答。请确认这些信息，等待我接下来的问题。" },
  { title: "逻辑推理", description: "观察分析过程与推理能力", icon: "gauge",
    prompt: "有三个盒子，分别贴着「苹果」「橙子」「混合」的标签，但标签全部贴错。只能从一个盒子取出一个水果，如何确定三个盒子的内容？请解释你的推理。" },
  { title: "代码生成", description: "检查代码质量与格式展示", icon: "edit",
    prompt: "用 Python 实现一个带过期时间的简单缓存，给出可运行的使用示例，并简要解释设计。" },
];
const MODEL_TEST_FOLLOWUPS = [
  { title: "回顾上下文", prompt: "请根据前面的对话，回顾我提到的项目名称、需求与偏好；未提到的信息请明确说明。" },
  { title: "给个例子", prompt: "请结合刚才的回答，给出一个具体、容易理解的例子。" },
  { title: "更简洁一些", prompt: "请把刚才的回答压缩为三个要点，保留核心信息。" },
  { title: "结构化输出", prompt: "请将刚才的回答整理成合法 JSON，只输出 JSON，不要使用 Markdown 代码块。" },
];

function buildModelTestMessages(history, prompt) {
  const messages = [{ role: "user", content: String(prompt).trim() }];
  let characters = Array.from(messages[0].content).length;
  const previous = history || [];
  for (let index = previous.length - 2; index >= 0 && messages.length < 41; index -= 2) {
    const user = previous[index];
    const assistant = previous[index + 1];
    if (user.role !== "user" || assistant.role !== "assistant") break;
    const size = Array.from(user.content).length + Array.from(assistant.content).length;
    if (characters + size > 32768) break;
    messages.unshift({ role: user.role, content: user.content }, { role: assistant.role, content: assistant.content });
    characters += size;
  }
  return messages;
}

function buildModelTestTurnHtml(model) {
  return '<section class="model-test-turn model-test-user-turn"><span class="model-test-user-avatar">你</span><div class="model-test-user-content"></div></section>' +
    '<section class="model-test-turn model-test-assistant-turn"><div class="model-test-assistant-label"><span>' + ICONS.sparkles +
    '</span><strong title="' + escAttr(model) + '">' + esc(model) + '</strong></div>' +
    '<div class="model-test-thinking" data-state="idle" hidden><button type="button" class="model-test-thinking-toggle" onclick="toggleModelTestThinking(this)" aria-expanded="false">' +
    '<span class="model-test-thinking-icon">' + ICONS.sparkles + '</span><span class="model-test-thinking-label">思考过程</span>' +
    '<svg class="model-test-thinking-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg></button>' +
    '<div class="model-test-thinking-body" hidden><div class="model-test-thinking-content"></div></div></div>' +
    '<div class="model-test-output" data-empty="true" aria-live="polite"></div>' +
    '<div class="model-test-response-footer" hidden><span class="model-test-turn-status"></span><button type="button" class="model-test-answer-copy" onclick="copyModelTestAnswer(this)" title="复制回答" aria-label="复制回答">' +
    ICONS.copy + '</button></div></section>';
}

function buildModelTestPopoverHtml(options) {
  const models = options.models || [];
  const mode = options.mode || "upstream";
  const model = models[0] || "";
  const modelLabel = model || (mode === "alias" ? "暂无可测试模型映射" : "暂无可测试模型");
  const readyLabel = model
    ? "就绪"
    : mode === "alias"
      ? "请先添加模型映射"
      : "请先同步模型";
  return (
    '<div class="model-test-header">' +
    '<div class="model-test-heading"><span class="model-test-mark">' +
    ICONS.sparkles +
    '</span><div><h2 id="modelTestTitle">模型对话</h2><div class="model-test-meta"><span data-model-test-source title="' +
    escAttr(options.sourceTitle || options.sourceLabel || "") + '">' + esc(options.sourceLabel || "") +
    '</span><i aria-hidden="true"></i><span data-model-test-route title="' +
    escAttr(options.sourceMeta || "") + '">' + esc(options.sourceMeta || "") +
    '</span></div></div></div>' +
    '<div class="model-test-header-actions"><div class="model-test-model-picker"><span class="model-test-model-icon" aria-hidden="true">' + ICONS.layers +
    '</span><select class="model-test-model-select m-select" aria-label="选择测试模型" onchange="changeModelTestModel(this)">' + buildModelTestOptionsHtml(models, model) + '</select></div>' +
    '<button type="button" class="model-test-new-chat" onclick="startNewModelTestChat(this)" title="新对话，清空当前上下文">' + ICONS.plus + '<span>新对话</span></button>' +
    '<button type="button" class="btn-icon model-test-fullscreen" onclick="toggleModelTestFullscreen(this)" title="全屏查看" aria-label="全屏查看" aria-pressed="false"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M8 3H3v5m13-5h5v5M3 16v5h5m13-5v5h-5"/></svg></button>' +
    '<button type="button" class="btn-icon model-test-close" onclick="closePopover()" title="关闭" aria-label="关闭">' +
    ICONS.close +
    '</button></div></div>' +
    '<div class="model-test-body"><div class="model-test-conversation" onscroll="updateModelTestScroll(this)">' +
    '<div class="model-test-empty"><div class="model-test-welcome"><span class="model-test-empty-mark">' + ICONS.sparkles +
    '</span><span class="model-test-eyebrow">MODEL PLAYGROUND</span><h3>从一个问题，开始探索</h3><p>验证模型能力，也让灵感在对话中延续。</p>' +
    '<strong data-model-test-current-model title="' + escAttr(modelLabel) + '">' + ICONS.layers + esc(modelLabel) + '</strong></div>' +
    '<div class="model-test-prompt-grid">' + MODEL_TEST_PROMPTS.map((item, index) =>
      '<button type="button" class="model-test-prompt-card" data-model-test-prompt onclick="fillModelTestPrompt(this, ' + index + ')"><span class="model-test-prompt-icon">' + ICONS[item.icon] +
      '</span><span><b>' + esc(item.title) + '</b><small>' + esc(item.description) + '</small></span>' + ICONS.arrowRight + '</button>').join('') +
    '</div></div><div class="model-test-thread" hidden></div></div>' +
    '<button class="model-test-scroll-latest" type="button" onclick="scrollModelTestToLatest(this)" aria-label="回到最新消息" hidden>' + ICONS.arrowUp + '<span>回到最新</span></button></div>' +
    '<div class="model-test-composer-wrap"><div class="model-test-followups" hidden>' + MODEL_TEST_FOLLOWUPS.map((item, index) =>
      '<button type="button" data-model-test-prompt onclick="fillModelTestPrompt(this, ' + index + ', true)">' + esc(item.title) + '</button>').join('') + '</div><div class="model-test-composer">' +
    '<textarea class="model-test-input" rows="2" maxlength="32768" aria-label="对话消息" oninput="updateModelTestControls(this); resizeModelTestInput(this)" onkeydown="handleModelTestKeydown(event)" placeholder="输入问题，和模型开始对话…"></textarea>' +
    '<div class="model-test-composer-bar"><span class="model-test-context"><i></i><span data-model-test-context>上下文已开启</span></span>' +
    '<div class="model-test-actions"><span class="model-test-status" data-state="idle" role="status"><i></i><span>' +
    readyLabel +
    '</span></span>' +
    '<button type="button" class="model-test-control model-test-stop" onclick="stopModelTestGeneration(this)" title="停止生成" aria-label="停止生成" disabled hidden>' +
    ICONS.stop +
    '</button><button type="button" class="model-test-control model-test-send" onclick="runModelTest(this)" title="发送" aria-label="发送" disabled>' +
    ICONS.arrowUp +
    '</button></div></div></div><div class="model-test-composer-note"><span>仅在本次窗口中保留对话</span><span><kbd>Enter</kbd> 发送 · <kbd>Shift + Enter</kbd> 换行</span></div></div>'
  );
}

function testUpstreamModel(btn) {
  const row = btn.closest("tr");
  const upstreamName = String(
    row?.querySelector('[data-field="name"]')?.value || "",
  ).trim();
  const baseURL = String(
    row?.querySelector('[data-field="base_url"]')?.value || "",
  ).trim();
  if (!upstreamName || !baseURL) {
    showToast("请先填写上游名称和 Base URL", "error");
    return;
  }

  collectUpstreams();
  const apiType = String(upstreamData[upstreamName]?.api_type || "openai");
  const models = testModelsForUpstream(upstreamName);
  openModelTestPopover(btn, {
    mode: "upstream",
    models,
    sourceLabel: upstreamName,
    sourceMeta: upstreamProtocolLabel(apiType),
  });
}

// 模型映射测试复用上游模型测试的对话窗口，但请求通过模型别名解析，
// 从而验证真实的路由目标，而不是直接指定某个上游模型。
function testModelsForAliases() {
  collectAliases();
  return Object.keys(aliasData || {})
    .filter(
      (name) =>
        normalizeAliasTargets(aliasData[name]?.targets || []).length > 0,
    )
    .sort((a, b) => a.localeCompare(b));
}

function testAliasModel(btn) {
  const row = btn.closest("tr");
  const aliasName = String(
    row?.querySelector('[data-field="key"]')?.value || "",
  ).trim();
  if (!aliasName) {
    showToast("请先填写映射模型名", "error");
    return;
  }
  const models = testModelsForAliases();
  if (!models.includes(aliasName)) {
    showToast("请为 " + aliasName + " 添加至少一个上游模型", "error");
    return;
  }
  // 让当前行的映射默认出现在下拉框首位，同时仍可切换到其他映射。
  const ordered = [aliasName, ...models.filter((name) => name !== aliasName)];
  openModelTestPopover(btn, {
    mode: "alias",
    models: ordered,
    sourceLabel: aliasName,
    sourceTitle: aliasName,
    sourceMeta: "模型映射 · 按权重选择上游",
  });
}

function openModelTestPopover(btn, options) {
  const models = options.models || [];
  const testPop = openPopover(
    btn,
    buildModelTestPopoverHtml(options),
    () => disposeModelTest(testPop),
  );
  testPop.classList.add("model-test-popover");
  testPop.parentElement.classList.add("model-test-overlay");
  testPop.setAttribute("role", "dialog");
  testPop.setAttribute("aria-modal", "true");
  testPop.setAttribute("aria-labelledby", "modelTestTitle");
  testPop._modelTestMode = options.mode || "upstream";
  testPop._modelTestUpstream =
    testPop._modelTestMode === "upstream" ? options.sourceLabel || "" : "";
  testPop._modelTestModel = models[0] || "";
  testPop._modelTestDefaultMeta = options.sourceMeta || "";
  testPop._modelTestRunning = false;
  testPop._modelTestMessages = [];
  testPop._modelTestStickToBottom = true;
  const conversation = testPop.querySelector(".model-test-conversation");
  testPop._modelTestScrollObserver = new ResizeObserver(() => {
    if (testPop._modelTestStickToBottom && !testPop.querySelector(".model-test-thread").hidden) {
      conversation.scrollTop = conversation.scrollHeight;
    }
  });
  testPop._modelTestScrollObserver.observe(conversation);
  testPop.addEventListener("keydown", (event) => {
    if (event.key !== "Tab" || ssActiveDropdown) return;
    const controls = Array.from(testPop.querySelectorAll('button:not(:disabled), textarea:not(:disabled), [tabindex="0"]'))
      .filter((element) => element.getClientRects().length && !element.closest("[hidden]"));
    const first = controls[0];
    const last = controls[controls.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last?.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first?.focus();
    }
  });
  setTimeout(() => {
    if (!testPop.isConnected) return;
    const modelSelect = testPop.querySelector(".model-test-model-select");
    ssEnhanceSelect(modelSelect);
    fitModelTestSelectWidth(modelSelect);
    const input = testPop.querySelector(".model-test-input");
    input?.focus();
    resizeModelTestInput(input);
    updateModelTestControls(testPop);
    updateModelTestContext(testPop);
  }, 0);
}

function disposeModelTest(pop) {
  pop?._modelTestController?.abort();
  pop?._modelTestScrollObserver?.disconnect();
  if (pop?._modelTestThinkingTimer) {
    clearInterval(pop._modelTestThinkingTimer);
    pop._modelTestThinkingTimer = null;
  }
  if (pop?._modelTestRenderFrame) {
    cancelAnimationFrame(pop._modelTestRenderFrame);
    pop._modelTestRenderFrame = null;
  }
}

function fitModelTestSelectWidth(select) {
  const wrapper = select?.closest(".ss-wrapper");
  const trigger = wrapper?.querySelector(".ss-trigger");
  if (!select || !wrapper || !trigger) return;
  const selected = select.options[select.selectedIndex];
  const label = selected?.textContent || "选择模型";
  const canvas = fitModelTestSelectWidth._canvas || document.createElement("canvas");
  fitModelTestSelectWidth._canvas = canvas;
  const context = canvas.getContext("2d");
  const style = getComputedStyle(trigger);
  if (context) context.font = style.font || style.fontSize + " " + style.fontFamily;
  const measured = context ? context.measureText(label).width : label.length * 7.5;
  const width = Math.min(310, Math.max(132, Math.ceil(measured + 46)));
  wrapper.style.setProperty("--model-test-select-width", width + "px");
}

function activeModelTestTurn(pop) {
  return pop?._modelTestActiveTurn || pop;
}

function fillModelTestPrompt(button, index, followup) {
  const pop = button.closest(".model-test-popover");
  const item = (followup ? MODEL_TEST_FOLLOWUPS : MODEL_TEST_PROMPTS)[index];
  if (!pop || !item || pop._modelTestRunning) return;
  const input = pop.querySelector(".model-test-input");
  input.value = item.prompt;
  resizeModelTestInput(input);
  updateModelTestControls(pop);
  input.focus();
}

function startNewModelTestChat(button) {
  const pop = button.closest(".model-test-popover");
  if (!pop || pop._modelTestRunning) return;
  resetModelTestConversation(pop);
  const input = pop.querySelector(".model-test-input");
  input.value = "";
  resizeModelTestInput(input);
  updateModelTestControls(pop);
  input.focus();
}

function toggleModelTestFullscreen(button) {
  const pop = button.closest(".model-test-popover");
  const expanded = pop.classList.toggle("is-fullscreen");
  pop.parentElement.classList.toggle("is-fullscreen", expanded);
  button.setAttribute("aria-pressed", String(expanded));
  button.setAttribute("aria-label", expanded ? "退出全屏" : "全屏查看");
  button.title = expanded ? "退出全屏" : "全屏查看";
  resizeModelTestInput(pop.querySelector(".model-test-input"));
}

function updateModelTestScroll(conversation) {
  const pop = conversation.closest(".model-test-popover");
  const button = pop?.querySelector(".model-test-scroll-latest");
  if (!pop || !button) return;
  const nearBottom = conversation.scrollHeight - conversation.scrollTop - conversation.clientHeight < 120;
  pop._modelTestStickToBottom = nearBottom;
  button.hidden = pop.querySelector(".model-test-thread").hidden || nearBottom;
}

function scrollModelTestToLatest(button) {
  const conversation = button.closest(".model-test-popover").querySelector(".model-test-conversation");
  conversation.scrollTop = conversation.scrollHeight;
  conversation.closest(".model-test-popover")._modelTestStickToBottom = true;
  button.hidden = true;
}

async function copyModelTestAnswer(button) {
  const turn = button.closest(".model-test-assistant-turn");
  const copied = await copyText(turn?._modelTestAnswer || "");
  button.innerHTML = copied ? ICONS.check : ICONS.copy;
  button.title = copied ? "已复制" : "复制失败，请重试";
  setTimeout(() => { button.innerHTML = ICONS.copy; button.title = "复制回答"; }, 1800);
}

function updateModelTestContext(pop) {
  const label = pop.querySelector("[data-model-test-context]");
  const turns = (buildModelTestMessages(pop._modelTestMessages, "继续").length - 1) / 2;
  if (label) {
    label.textContent = turns ? "上下文 · 最近 " + turns + " 轮" : "上下文已开启";
    label.title = "自动携带最近 20 轮完整对话，总计最多 32768 字；失败或停止的回答不加入上下文";
  }
}

function setModelTestThinkingExpanded(pop, expanded, userInitiated) {
  const thinking = activeModelTestTurn(pop)?.querySelector(".model-test-thinking");
  const toggle = thinking?.querySelector(".model-test-thinking-toggle");
  const body = thinking?.querySelector(".model-test-thinking-body");
  if (!thinking || !toggle || !body) return;
  if (userInitiated) pop._modelTestThinkingUserToggled = true;
  thinking.classList.toggle("is-expanded", expanded);
  toggle.setAttribute("aria-expanded", expanded ? "true" : "false");
  body.hidden = !expanded;
  if (expanded) body.scrollTop = body.scrollHeight;
}

function toggleModelTestThinking(button) {
  const pop = button.closest(".model-test-popover");
  const turn = button.closest(".model-test-assistant-turn");
  const expanded = button.getAttribute("aria-expanded") === "true";
  if (turn === pop._modelTestActiveTurn) pop._modelTestThinkingUserToggled = true;
  setModelTestThinkingExpanded(turn, !expanded, false);
}

function modelTestThinkingElapsed(pop) {
  if (!pop?._modelTestThinkingStartedAt) return 0;
  return Math.max(0, performance.now() - pop._modelTestThinkingStartedAt);
}

function formatModelTestThinkingDuration(milliseconds) {
  if (milliseconds < 1000) return "少于 1 秒";
  return Math.max(1, Math.round(milliseconds / 1000)) + " 秒";
}

function updateModelTestThinkingLabel(pop) {
  const thinking = activeModelTestTurn(pop)?.querySelector(".model-test-thinking");
  const label = thinking?.querySelector(".model-test-thinking-label");
  if (!thinking || !label || thinking.dataset.state !== "running") return;
  const elapsed = modelTestThinkingElapsed(pop);
  label.textContent =
    elapsed < 1000
      ? "正在思考…"
      : "正在思考 · " + formatModelTestThinkingDuration(elapsed);
}

function resetModelTestThinking(pop) {
  if (!pop) return;
  if (pop._modelTestThinkingTimer) clearInterval(pop._modelTestThinkingTimer);
  pop._modelTestThinkingTimer = null;
  pop._modelTestThinkingStartedAt = 0;
  pop._modelTestThinkingText = "";
  pop._modelTestThinkingUserToggled = false;
  const thinking = activeModelTestTurn(pop).querySelector(".model-test-thinking");
  const content = thinking?.querySelector(".model-test-thinking-content");
  const label = thinking?.querySelector(".model-test-thinking-label");
  if (thinking) {
    thinking.hidden = true;
    thinking.dataset.state = "idle";
  }
  if (content) content.replaceChildren();
  if (label) label.textContent = "思考过程";
  setModelTestThinkingExpanded(pop, false, false);
}

function appendModelTestThinking(pop, text) {
  if (!pop || !text) return;
  const thinking = activeModelTestTurn(pop).querySelector(".model-test-thinking");
  const body = thinking?.querySelector(".model-test-thinking-body");
  const content = thinking?.querySelector(".model-test-thinking-content");
  if (!thinking || !body || !content) return;
  const followThinking =
    body.hidden || body.scrollHeight - body.scrollTop - body.clientHeight < 48;
  if (!pop._modelTestThinkingStartedAt) {
    pop._modelTestThinkingStartedAt = performance.now();
  }
  if (thinking.dataset.state !== "running") {
    thinking.dataset.state = "running";
    thinking.hidden = false;
    if (!pop._modelTestThinkingUserToggled) {
      setModelTestThinkingExpanded(pop, true, false);
    }
    if (pop._modelTestThinkingTimer) clearInterval(pop._modelTestThinkingTimer);
    pop._modelTestThinkingTimer = setInterval(
      () => updateModelTestThinkingLabel(pop),
      500,
    );
  }
  pop._modelTestThinkingText =
    String(pop._modelTestThinkingText || "") + String(text);
  const lastNode = content.lastChild;
  if (lastNode?.nodeType === Node.TEXT_NODE) lastNode.appendData(String(text));
  else content.appendChild(document.createTextNode(String(text)));
  updateModelTestThinkingLabel(pop);
  if (followThinking && !body.hidden) body.scrollTop = body.scrollHeight;
}

function finishModelTestThinking(pop, state) {
  const thinking = activeModelTestTurn(pop)?.querySelector(".model-test-thinking");
  const label = thinking?.querySelector(".model-test-thinking-label");
  if (
    !thinking ||
    thinking.hidden ||
    !pop._modelTestThinkingText ||
    thinking.dataset.state !== "running"
  ) {
    return;
  }
  if (pop._modelTestThinkingTimer) clearInterval(pop._modelTestThinkingTimer);
  pop._modelTestThinkingTimer = null;
  const finalState = state || "complete";
  const duration = formatModelTestThinkingDuration(modelTestThinkingElapsed(pop));
  thinking.dataset.state = finalState;
  if (label) {
    if (finalState === "stopped") label.textContent = "思考已停止 · " + duration;
    else if (finalState === "error") label.textContent = "思考中断 · " + duration;
    else label.textContent = "已思考 " + duration;
  }
  if (!pop._modelTestThinkingUserToggled) {
    setModelTestThinkingExpanded(pop, false, false);
  }
}

function resetModelTestConversation(pop) {
  if (!pop || pop._modelTestRunning) return;
  if (pop._modelTestRenderFrame) cancelAnimationFrame(pop._modelTestRenderFrame);
  pop._modelTestRenderFrame = null;
  pop._modelTestOutputMarkdown = "";
  resetModelTestThinking(pop);
  pop._modelTestActiveTurn = null;
  pop._modelTestMessages = [];
  const empty = pop.querySelector(".model-test-empty");
  const thread = pop.querySelector(".model-test-thread");
  if (empty) empty.hidden = false;
  if (thread) {
    thread.replaceChildren();
    thread.hidden = true;
  }
  pop.querySelector(".model-test-followups").hidden = true;
  pop.querySelector(".model-test-scroll-latest").hidden = true;
  pop.querySelector(".model-test-conversation").scrollTop = 0;
  resetModelTestRoute(pop);
  updateModelTestContext(pop);
  setModelTestStatus(
    pop,
    pop._modelTestModel ? "就绪" : modelTestEmptyStatus(pop),
    "idle",
  );
}

function changeModelTestModel(select) {
  const pop = select.closest(".model-test-popover");
  if (!pop || pop._modelTestRunning) return;
  const model = String(select.value || "").trim();
  const modelLabel = model || "暂无可测试模型";
  pop._modelTestModel = model;
  ssSyncLabel(select);
  fitModelTestSelectWidth(select);
  pop
    .querySelectorAll("[data-model-test-current-model]")
    .forEach((label) => {
      label.innerHTML = ICONS.layers + esc(modelLabel);
      label.title = modelLabel;
    });
  if (pop._modelTestMode === "alias") {
    const source = pop.querySelector("[data-model-test-source]");
    if (source) {
      const label = model || "模型映射";
      source.textContent = label;
      source.title = label;
    }
  }
  resetModelTestRoute(pop);
  setModelTestStatus(pop, model ? "就绪" : modelTestEmptyStatus(pop), "idle");
  updateModelTestControls(pop);
  pop.querySelector(".model-test-input")?.focus();
}

function resizeModelTestInput(input) {
  if (!input) return;
  input.style.height = "auto";
  const limit = Math.min(160, Math.max(56, window.innerHeight * 0.2));
  input.style.height = Math.min(input.scrollHeight, limit) + "px";
}

function handleModelTestKeydown(event) {
  if (event.key !== "Enter" || event.shiftKey || event.isComposing) return;
  event.preventDefault();
  const send = event.currentTarget
    .closest(".model-test-popover")
    ?.querySelector(".model-test-send");
  if (send && !send.disabled) runModelTest(send);
}

function updateModelTestControls(source) {
  const pop = source?.closest?.(".model-test-popover") || source;
  if (!pop?.classList?.contains("model-test-popover")) return;
  const input = pop.querySelector(".model-test-input");
  const running = Boolean(pop._modelTestRunning);
  const send = pop.querySelector(".model-test-send");
  const stop = pop.querySelector(".model-test-stop");
  const modelSelect = pop.querySelector(".model-test-model-select");
  const hasModel = Boolean(String(modelSelect?.value || "").trim());
  if (input) {
    input.disabled = !hasModel;
    input.placeholder = !hasModel ? (pop._modelTestMode === "alias" ? "没有可测试的模型映射" : "请先同步上游模型") : running ? "可先编辑下一条消息…" : pop._modelTestMessages?.length ? "继续提问，模型会结合前文回答…" : "输入问题，和模型开始对话…";
  }
  pop.querySelectorAll("[data-model-test-prompt]").forEach((button) => { button.disabled = running || !hasModel; });
  const newChat = pop.querySelector(".model-test-new-chat");
  if (newChat) newChat.disabled = running;
  if (modelSelect) {
    modelSelect.disabled = running || !hasModel;
    ssSyncDisabled(modelSelect);
  }
  if (send)
    send.disabled =
      running || !hasModel || !String(input?.value || "").trim();
  if (send) send.hidden = running;
  if (stop) {
    stop.disabled = !running;
    stop.hidden = !running;
  }
}

function setModelTestStatus(pop, text, state) {
  const status = pop?.querySelector(".model-test-status");
  if (!status) return;
  status.dataset.state = state || "idle";
  const label = status.querySelector("span");
  if (label) label.textContent = text;
}

const MODEL_TEST_CODE_ALIASES = {
  bash: "shell",
  cjs: "javascript",
  console: "shell",
  cs: "csharp",
  golang: "go",
  html: "markup",
  js: "javascript",
  jsx: "javascript",
  md: "markdown",
  mjs: "javascript",
  py: "python",
  rb: "ruby",
  sh: "shell",
  ts: "typescript",
  tsx: "typescript",
  yml: "yaml",
  xml: "markup",
};

function modelTestKeywordSet(words) {
  return new Set(words.split(/\s+/).filter(Boolean));
}

const MODEL_TEST_CODE_PROFILES = {
  javascript: {
    keywords: modelTestKeywordSet(
      "as async await break case catch class const continue debugger default delete do else export extends finally for from function get if import in instanceof let new of return set static super switch throw try typeof var void while with yield",
    ),
    literals: modelTestKeywordSet("false null true undefined NaN Infinity"),
    lineComments: ["//"],
    blockComments: [["/*", "*/"]],
  },
  typescript: {
    keywords: modelTestKeywordSet(
      "abstract any as asserts async await bigint boolean break case catch class const constructor continue debugger declare default delete do else enum export extends finally for from function get if implements import in infer instanceof interface is keyof let module namespace never new number object of override private protected public readonly require return set static string super switch symbol this throw try type typeof undefined unique unknown var void while with yield",
    ),
    literals: modelTestKeywordSet("false null true"),
    lineComments: ["//"],
    blockComments: [["/*", "*/"]],
  },
  go: {
    keywords: modelTestKeywordSet(
      "break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var",
    ),
    literals: modelTestKeywordSet("false iota nil true"),
    lineComments: ["//"],
    blockComments: [["/*", "*/"]],
  },
  python: {
    keywords: modelTestKeywordSet(
      "and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield",
    ),
    literals: modelTestKeywordSet("False None True"),
    lineComments: ["#"],
    blockComments: [],
  },
  ruby: {
    keywords: modelTestKeywordSet(
      "alias and begin break case class def defined do else elsif end ensure false for if in module next nil not or redo rescue retry return self super then true undef unless until when while yield",
    ),
    literals: modelTestKeywordSet("false nil true"),
    lineComments: ["#"],
    blockComments: [],
  },
  shell: {
    keywords: modelTestKeywordSet(
      "case do done elif else esac export fi for function if in local readonly return select set source then time trap unset until while",
    ),
    literals: modelTestKeywordSet("false true"),
    lineComments: ["#"],
    blockComments: [],
  },
  sql: {
    keywords: modelTestKeywordSet(
      "add all alter and any as asc begin between by case check column commit constraint create cross database default delete desc distinct drop else end exists foreign from full grant group having in index inner insert intersect into is join key left like limit not null on or order outer primary references right rollback row select set table then union unique update values view when where with",
    ),
    literals: modelTestKeywordSet("false null true"),
    lineComments: ["--"],
    blockComments: [["/*", "*/"]],
    caseInsensitive: true,
  },
  json: {
    keywords: new Set(),
    literals: modelTestKeywordSet("false null true"),
    lineComments: [],
    blockComments: [],
    attributeKeys: true,
  },
  markup: {
    keywords: new Set(),
    literals: new Set(),
    lineComments: [],
    blockComments: [["<!--", "-->"]],
    markup: true,
  },
  css: {
    keywords: modelTestKeywordSet("and important not only or"),
    literals: new Set(),
    lineComments: [],
    blockComments: [["/*", "*/"]],
    attributeKeys: true,
  },
  yaml: {
    keywords: new Set(),
    literals: modelTestKeywordSet("false null true yes no on off"),
    lineComments: ["#"],
    blockComments: [],
    attributeKeys: true,
  },
  java: {
    keywords: modelTestKeywordSet(
      "abstract assert boolean break byte case catch char class const continue default do double else enum extends final finally float for goto if implements import instanceof int interface long native new package private protected public return short static strictfp super switch synchronized this throw throws transient try void volatile while",
    ),
    literals: modelTestKeywordSet("false null true"),
    lineComments: ["//"],
    blockComments: [["/*", "*/"]],
  },
};

function modelTestCodeLanguage(code) {
  const languageClass = Array.from(code.classList).find((name) =>
    /^(?:lang|language)-/i.test(name),
  );
  const raw = String(languageClass || "")
    .replace(/^(?:lang|language)-/i, "")
    .toLowerCase();
  return MODEL_TEST_CODE_ALIASES[raw] || raw || "text";
}

function modelTestCodeProfile(language) {
  if (MODEL_TEST_CODE_PROFILES[language]) return MODEL_TEST_CODE_PROFILES[language];
  if (["c", "cpp", "csharp", "kotlin", "rust", "swift"].includes(language)) {
    return {
      keywords: modelTestKeywordSet(
        "abstract as async await break case catch class const continue default defer do else enum extern final finally fn for foreach from func if impl import in interface internal let match namespace new operator override package private protected public readonly return static struct switch throw trait try type typeof unsafe using var virtual void volatile where while",
      ),
      literals: modelTestKeywordSet("false nil null nullptr true"),
      lineComments: ["//"],
      blockComments: [["/*", "*/"]],
    };
  }
  return null;
}

function appendModelTestCodeToken(fragment, text, kind) {
  if (!text) return;
  if (!kind) {
    fragment.appendChild(document.createTextNode(text));
    return;
  }
  const span = document.createElement("span");
  span.className = "code-token code-token-" + kind;
  span.textContent = text;
  fragment.appendChild(span);
}

function highlightModelTestCode(code, language) {
  const source = code.textContent || "";
  const profile = modelTestCodeProfile(language);
  if (!profile || !source) return source;

  const fragment = document.createDocumentFragment();
  let index = 0;
  let inMarkupTag = false;
  while (index < source.length) {
    const blockComment = (profile.blockComments || []).find(([start]) =>
      source.startsWith(start, index),
    );
    if (blockComment) {
      const endAt = source.indexOf(blockComment[1], index + blockComment[0].length);
      const next = endAt < 0 ? source.length : endAt + blockComment[1].length;
      appendModelTestCodeToken(fragment, source.slice(index, next), "comment");
      index = next;
      continue;
    }

    const lineComment = (profile.lineComments || []).find((start) =>
      source.startsWith(start, index),
    );
    if (lineComment) {
      const endAt = source.indexOf("\n", index + lineComment.length);
      const next = endAt < 0 ? source.length : endAt;
      appendModelTestCodeToken(fragment, source.slice(index, next), "comment");
      index = next;
      continue;
    }

    const char = source[index];
    if (char === '"' || char === "'" || char === "`") {
      const triple = source.startsWith(char.repeat(3), index);
      const delimiter = triple ? char.repeat(3) : char;
      let next = index + delimiter.length;
      while (next < source.length) {
        if (source[next] === "\\") {
          next += 2;
          continue;
        }
        if (source.startsWith(delimiter, next)) {
          next += delimiter.length;
          break;
        }
        next++;
      }
      let kind = "string";
      if (profile.attributeKeys) {
        let lookahead = next;
        while (/\s/.test(source[lookahead] || "")) lookahead++;
        if (source[lookahead] === ":") kind = "attribute";
      }
      appendModelTestCodeToken(fragment, source.slice(index, next), kind);
      index = next;
      continue;
    }

    if (profile.markup && char === "<") {
      const length = source[index + 1] === "/" ? 2 : 1;
      appendModelTestCodeToken(fragment, source.slice(index, index + length), "operator");
      index += length;
      inMarkupTag = true;
      continue;
    }
    if (profile.markup && char === ">") {
      appendModelTestCodeToken(fragment, char, "operator");
      index++;
      inMarkupTag = false;
      continue;
    }

    if (/[A-Za-z_$]/.test(char)) {
      let next = index + 1;
      while (/[\w$-]/.test(source[next] || "")) next++;
      const word = source.slice(index, next);
      const lookup = profile.caseInsensitive ? word.toLowerCase() : word;
      let kind = "";
      if (profile.keywords.has(lookup)) kind = "keyword";
      else if (profile.literals.has(lookup)) kind = "literal";
      else if (profile.markup && inMarkupTag) {
        kind = source[index - 1] === "<" || source[index - 1] === "/" ? "title" : "attribute";
      } else {
        let lookahead = next;
        while (/\s/.test(source[lookahead] || "")) lookahead++;
        if (profile.attributeKeys && source[lookahead] === ":") kind = "attribute";
        else if (source[lookahead] === "(") kind = "title";
      }
      appendModelTestCodeToken(fragment, word, kind);
      index = next;
      continue;
    }

    if (/\d/.test(char) && !/[\w$]/.test(source[index - 1] || "")) {
      const number = source.slice(index).match(/^(?:0[xob])?[\da-f]+(?:\.\d+)?(?:e[+-]?\d+)?/i)?.[0];
      appendModelTestCodeToken(fragment, number || char, "number");
      index += (number || char).length;
      continue;
    }

    if (/[{}[\](),.;:+*/%=&|!<>?#~-]/.test(char)) {
      appendModelTestCodeToken(fragment, char, "operator");
      index++;
      continue;
    }

    let next = index + 1;
    while (
      next < source.length &&
      !/[A-Za-z_$\d"'`{}[\](),.;:+*/%=&|!<>?#~-]/.test(source[next])
    ) {
      next++;
    }
    appendModelTestCodeToken(fragment, source.slice(index, next), "");
    index = next;
  }
  code.replaceChildren(fragment);
  return source;
}

function modelTestLanguageLabel(language) {
  const labels = {
    csharp: "C#",
    cpp: "C++",
    javascript: "JavaScript",
    markup: "HTML / XML",
    shell: "Shell",
    typescript: "TypeScript",
  };
  return labels[language] || (language === "text" ? "纯文本" : language.toUpperCase());
}

function enhanceModelTestCodeBlocks(output) {
  output.querySelectorAll("pre > code").forEach((code) => {
    const pre = code.parentElement;
    if (!pre || pre.closest(".model-test-code-block")) return;
    const language = modelTestCodeLanguage(code);
    const source = highlightModelTestCode(code, language);
    const block = document.createElement("div");
    block.className = "model-test-code-block";
    const toolbar = document.createElement("div");
    toolbar.className = "model-test-code-toolbar";
    const label = document.createElement("span");
    label.className = "model-test-code-language";
    label.textContent = modelTestLanguageLabel(language);
    const copyButton = document.createElement("button");
    copyButton.type = "button";
    copyButton.className = "model-test-code-copy";
    copyButton.title = "复制代码";
    copyButton.setAttribute("aria-label", "复制代码");
    copyButton.innerHTML = ICONS.copy + "<span>复制</span>";
    copyButton.addEventListener("click", async () => {
      const copied = await copyText(source);
      copyButton.classList.toggle("is-copied", copied);
      copyButton.innerHTML = (copied ? ICONS.check : ICONS.copy) +
        "<span>" + (copied ? "已复制" : "复制失败") + "</span>";
      setTimeout(() => {
        if (!copyButton.isConnected) return;
        copyButton.classList.remove("is-copied");
        copyButton.innerHTML = ICONS.copy + "<span>复制</span>";
      }, 1600);
    });
    toolbar.append(label, copyButton);
    pre.replaceWith(block);
    block.append(toolbar, pre);
  });
}

function renderModelTestMarkdown(markdown) {
  const source = String(markdown || "");
  if (!window.marked?.parse || !window.DOMPurify?.sanitize) return null;
  const rendered = window.marked.parse(source, {
    gfm: true,
    breaks: true,
  });
  return window.DOMPurify.sanitize(rendered, {
    USE_PROFILES: { html: true },
    FORBID_TAGS: [
      "button",
      "embed",
      "form",
      "iframe",
      "link",
      "meta",
      "object",
      "option",
      "select",
      "style",
      "textarea",
    ],
    FORBID_ATTR: ["style"],
  });
}

function renderModelTestOutput(pop, followOutput) {
  const output = activeModelTestTurn(pop)?.querySelector(".model-test-output");
  if (!output) return;
  const markdown = String(pop._modelTestOutputMarkdown || "");
  if (!markdown) {
    output.replaceChildren();
    output.dataset.empty = "true";
    return;
  }

  let html = null;
  try {
    html = renderModelTestMarkdown(markdown);
  } catch (e) {
    console.warn("Markdown rendering failed, falling back to plain text", e);
  }
  if (html === null) output.textContent = markdown;
  else output.innerHTML = html;
  output.dataset.empty = "false";

  output.querySelectorAll("a[href]").forEach((link) => {
    link.target = "_blank";
    link.rel = "noopener noreferrer";
  });
  output.querySelectorAll("img").forEach((image) => {
    image.loading = "lazy";
    image.referrerPolicy = "no-referrer";
  });
  output.querySelectorAll("input").forEach((input) => {
    const item = input.closest("li");
    if (input.type === "checkbox" && input.disabled && item) {
      item.classList.add("task-list-item");
      item.closest("ul, ol")?.classList.add("contains-task-list");
      input.tabIndex = -1;
      return;
    }
    input.remove();
  });
  enhanceModelTestCodeBlocks(output);

  const conversation = pop.querySelector(".model-test-conversation");
  if (followOutput && conversation) conversation.scrollTop = conversation.scrollHeight;
}

function scheduleModelTestOutputRender(pop, followOutput) {
  pop._modelTestFollowOutput = followOutput;
  if (pop._modelTestRenderFrame) return;
  pop._modelTestRenderFrame = requestAnimationFrame(() => {
    pop._modelTestRenderFrame = null;
    renderModelTestOutput(pop, pop._modelTestFollowOutput);
  });
}

function flushModelTestOutput(pop) {
  if (!pop) return;
  if (pop._modelTestRenderFrame) {
    cancelAnimationFrame(pop._modelTestRenderFrame);
    pop._modelTestRenderFrame = null;
  }
  renderModelTestOutput(pop, pop._modelTestFollowOutput);
}

function appendModelTestOutput(pop, text) {
  if (!text) return;
  finishModelTestThinking(pop, "complete");
  const output = activeModelTestTurn(pop).querySelector(".model-test-output");
  if (!output) return;
  const conversation = pop.querySelector(".model-test-conversation");
  const followOutput =
    !conversation ||
    conversation.scrollHeight - conversation.scrollTop - conversation.clientHeight < 72;
  output.dataset.empty = "false";
  pop._modelTestOutputMarkdown =
    String(pop._modelTestOutputMarkdown || "") + String(text);
  scheduleModelTestOutputRender(pop, followOutput);
}

function modelTestTextValue(value) {
  if (typeof value === "string") return value;
  if (!value || typeof value !== "object") return "";
  return typeof value.text === "string"
    ? value.text
    : typeof value.content === "string"
      ? value.content
      : typeof value.value === "string"
        ? value.value
        : "";
}

function modelTestContentParts(value) {
  if (typeof value === "string") return { text: value, reasoning: "" };
  if (!Array.isArray(value)) return { text: modelTestTextValue(value), reasoning: "" };
  let text = "";
  let reasoning = "";
  value.forEach((part) => {
    const partText = modelTestTextValue(part);
    const type = String(part?.type || "").toLowerCase();
    if (type.includes("reasoning") || type.includes("thinking")) {
      reasoning += partText;
    } else {
      text += partText;
    }
  });
  return { text, reasoning };
}

function modelTestReasoningValue(value) {
  if (typeof value === "string") return value;
  if (Array.isArray(value)) {
    return value.map((item) => modelTestReasoningValue(item)).join("");
  }
  if (!value || typeof value !== "object") return "";
  const direct = modelTestTextValue(value);
  if (direct) return direct;
  if (Array.isArray(value.summary)) {
    return value.summary.map((item) => modelTestReasoningValue(item)).join("");
  }
  return "";
}

function modelTestIsCompletePayload(value) {
  const trimmed = String(value || "").trim();
  if (!trimmed) return false;
  if (trimmed === "[DONE]") return true;
  if (trimmed.charAt(0) !== "{" && trimmed.charAt(0) !== "[") return false;
  try {
    JSON.parse(trimmed);
    return true;
  } catch (e) {
    return false;
  }
}

// 按完整 SSE 事件解析：支持多行 data、\r\n 与 \r 换行，也兼容
// “每个事件只用单个换行分隔”的非标准上游。之前逐行解析只认单行 data，
// 遇到多行载荷时会丢掉正文。
function createModelTestSSEParser() {
  let pending = "";
  let eventName = "";
  let buffer = "";
  const flush = (out) => {
    if (!pending) return;
    out.push({ data: pending, name: eventName });
    pending = "";
    eventName = "";
  };
  const appendData = (value, out) => {
    pending = pending ? pending + "\n" + value : value;
    if (modelTestIsCompletePayload(pending)) flush(out);
  };
  const consumeLine = (line, out) => {
    if (!line) {
      flush(out);
      return;
    }
    if (line.charAt(0) === ":") return;
    const colon = line.indexOf(":");
    const field = colon === -1 ? line : line.slice(0, colon);
    let value = colon === -1 ? "" : line.slice(colon + 1);
    if (value.charAt(0) === " ") value = value.slice(1);
    if (field === "event") {
      eventName = value;
      return;
    }
    if (field === "data") appendData(value, out);
  };
  return {
    push(text, final) {
      const out = [];
      buffer += String(text || "");
      let index = 0;
      while (true) {
        const newline = buffer.indexOf("\n", index);
        if (newline === -1) break;
        let line = buffer.slice(index, newline);
        index = newline + 1;
        if (line.charAt(line.length - 1) === "\r") line = line.slice(0, -1);
        consumeLine(line, out);
      }
      buffer = buffer.slice(index);
      if (final) {
        const tail = buffer.replace(/\r$/, "");
        buffer = "";
        if (tail) consumeLine(tail, out);
        flush(out);
      }
      return out;
    },
  };
}

function modelTestFinishReason(stopReason) {
  switch (String(stopReason || "")) {
    case "max_tokens":
    case "model_context_window_exceeded":
    case "length":
      return "length";
    case "tool_use":
    case "tool_calls":
      return "tool_calls";
    case "refusal":
    case "content_filter":
      return "content_filter";
    default:
      return String(stopReason || "");
  }
}

function modelTestPayloadEvent(payload, eventName) {
  if (!payload || typeof payload !== "object") return null;
  const payloadType = String(payload.type || eventName || "").toLowerCase();
  if (payload.error) {
    const error = payload.error;
    return {
      error:
        typeof error === "string"
          ? error
          : error.message || error.type || "模型流返回错误",
    };
  }
  if (payloadType === "error") {
    return { error: payload.message || payload.code || "模型流返回错误" };
  }

  const choice = Array.isArray(payload.choices) && payload.choices.length
    ? payload.choices[0] || {}
    : {};
  const delta = choice.delta && typeof choice.delta === "object" ? choice.delta : null;
  const message = choice.message && typeof choice.message === "object" ? choice.message : null;
  // 不是所有上游都真的流式返回：有些在 stream=true 时仍给出完整的
  // choices[0].message，此时必须回退到 message 才能拿到正文。
  const part = delta || message || {};
  const content = modelTestContentParts(part.content ?? message?.content);
  let text = content.text;
  let reasoning = content.reasoning;
  if (typeof choice.text === "string") text += choice.text;
  // 少数聚合网关不使用 choices 结构，直接在顶层给出 content/text/delta。
  // 已知的 Responses / Anthropic 事件已由下方分支处理，避免重复追加。
  const knownProtocolEvent =
    payloadType.startsWith("response.") ||
    payloadType === "message_start" ||
    payloadType === "message_delta" ||
    payloadType === "message_stop" ||
    payloadType === "content_block_start" ||
    payloadType === "content_block_delta" ||
    payloadType === "ping";
  [
    part.reasoning_content,
    part.reasoning,
    part.reasoning_text,
    part.thinking,
  ].forEach((value) => {
    reasoning += modelTestReasoningValue(value);
  });
  if (Array.isArray(part.reasoning_details)) {
    part.reasoning_details.forEach((detail) => {
      reasoning += modelTestReasoningValue(detail);
    });
  }
  // 拒答内容是模型的实际输出，之前被忽略，会表现为“未返回文本”。
  if (typeof part.refusal === "string") {
    text += part.refusal;
  } else if (part.refusal && typeof part.refusal === "object") {
    text += modelTestTextValue(part.refusal);
  }

  let finishReason = modelTestFinishReason(choice.finish_reason);
  let finished = Boolean(finishReason);
  let done = false;
  let hasToolCalls =
    (Array.isArray(part.tool_calls) && part.tool_calls.length > 0) ||
    Boolean(part.function_call);

  if (payloadType === "content_block_delta") {
    const anthropicDelta = payload.delta && typeof payload.delta === "object" ? payload.delta : {};
    if (anthropicDelta.type === "thinking_delta") {
      reasoning += modelTestTextValue({ text: anthropicDelta.thinking });
    } else if (anthropicDelta.type === "text_delta") {
      text += modelTestTextValue({ text: anthropicDelta.text });
    }
  } else if (payloadType === "content_block_start") {
    const block = payload.content_block;
    if (block && typeof block === "object" && block.type === "text" && typeof block.text === "string") {
      text += block.text;
    }
  } else if (payloadType === "message_delta") {
    const stopReason = payload?.delta?.stop_reason;
    if (stopReason) {
      finishReason = modelTestFinishReason(stopReason);
      finished = true;
    }
  } else if (payloadType === "message_stop") {
    finished = true;
    done = true;
  } else if (payloadType.includes("refusal") && typeof payload.delta === "string") {
    text += payload.delta;
  } else if (payloadType.includes("reasoning") && typeof payload.delta === "string") {
    reasoning += payload.delta;
  } else if (payloadType.includes("output_text") && typeof payload.delta === "string") {
    text += payload.delta;
  } else if (payloadType === "response.output_item.added" || payloadType === "response.output_item.done") {
    const itemType = String(payload?.item?.type || "");
    if (itemType === "function_call" || itemType === "custom_tool_call") hasToolCalls = true;
  } else if (payloadType === "response.completed" || payloadType === "response.incomplete" || payloadType === "response.done") {
    finished = true;
    done = true;
    if (payloadType === "response.incomplete" && !finishReason) finishReason = "length";
  } else if (payloadType === "response.failed") {
    return { error: payload?.response?.error?.message || "上游响应失败" };
  } else if (!Array.isArray(payload.choices) && !knownProtocolEvent) {
    if (typeof payload.content === "string") text += payload.content;
    else if (typeof payload.text === "string") text += payload.text;
    else if (typeof payload.delta === "string") text += payload.delta;
  }

  return {
    text,
    reasoning,
    usage: payload.usage || payload.response?.usage,
    done,
    finished,
    finishReason,
    hasToolCalls,
  };
}

function modelTestStreamEvent(frame) {
  let data = "";
  let name = "";
  if (frame && typeof frame === "object" && typeof frame.data === "string") {
    data = frame.data;
    name = frame.name || "";
  } else {
    const events = createModelTestSSEParser().push(String(frame || ""), true);
    if (!events.length) return null;
    data = events[0].data;
    name = events[0].name || "";
  }
  const raw = String(data || "").trim();
  if (!raw) return null;
  if (raw === "[DONE]") return { done: true };
  let payload;
  try {
    payload = JSON.parse(raw);
  } catch (e) {
    return null;
  }
  return modelTestPayloadEvent(payload, name);
}

// 有些上游忽略 stream=true，直接返回完整 JSON。此时没有任何 data: 行，
// 需要把整个响应体当作一次完成的响应来解析。
function modelTestNonStreamEvent(rawBody) {
  const trimmed = String(rawBody || "").trim();
  if (!trimmed || (trimmed.charAt(0) !== "{" && trimmed.charAt(0) !== "[")) return null;
  let payload;
  try {
    payload = JSON.parse(trimmed);
  } catch (e) {
    return null;
  }
  const event = modelTestPayloadEvent(payload, "");
  if (!event || event.error) return event;
  return { ...event, done: true, finished: true };
}

function modelTestEmptyTextReason(finishReason, hasReasoning, hasToolCalls) {
  if (finishReason === "length") {
    return hasReasoning
      ? "模型在思考阶段用尽了输出额度，未生成正文；请重试或换用非推理模型。"
      : "模型因输出额度不足（finish_reason=length）未返回正文，请重试或换用其他模型。";
  }
  if (finishReason === "content_filter") {
    return "模型输出被上游内容策略拦截，未返回正文。";
  }
  if (hasToolCalls || finishReason === "tool_calls") return "模型只返回了工具调用，没有可展示的正文。";
  if (hasReasoning) return "模型只返回了思考内容，没有正文；请重试或换用其他模型。";
  return "模型未返回文本，请重试或切换模型";
}

function modelTestHTTPError(raw, status, response) {
  let payload;
  try {
    payload = JSON.parse(raw);
  } catch (e) {}
  const error = payload?.error;
  const message =
    (typeof error === "string" ? error : error?.message) || raw || "HTTP " + status;
  const upstreamStatus = payload?.upstream_status;
  const detail = upstreamStatus
    ? "上游 HTTP " + upstreamStatus + "：" + message
    : message;
  const route = modelTestResponseRoute(response);
  return route ? route.upstream + " / " + route.model + "：" + detail : detail;
}

function decodeModelTestHeader(value) {
  const raw = String(value || "");
  try {
    return decodeURIComponent(raw);
  } catch (e) {
    return raw;
  }
}

function modelTestResponseRoute(response) {
  const upstream = decodeModelTestHeader(
    response?.headers?.get?.("X-Model-Test-Upstream"),
  );
  const model = decodeModelTestHeader(
    response?.headers?.get?.("X-Model-Test-Model"),
  );
  return upstream && model ? { upstream, model } : null;
}

function resetModelTestRoute(pop) {
  const meta = pop?.querySelector("[data-model-test-route]");
  if (!meta) return;
  const label = pop._modelTestDefaultMeta || "";
  meta.textContent = label;
  meta.title = label;
}

function showModelTestRoute(pop, route) {
  if (!pop || !route) return;
  const meta = pop.querySelector("[data-model-test-route]");
  if (!meta) return;
  const target = route.upstream + " / " + route.model;
  meta.textContent = "→ " + target;
  meta.title = "本次请求路由到 " + target;
}

function modelTestEmptyStatus(pop) {
  return pop?._modelTestMode === "alias" ? "请先添加模型映射" : "请先同步模型";
}

async function runModelTest(btn) {
  const pop = btn.closest(".model-test-popover");
  const input = pop?.querySelector(".model-test-input");
  const prompt = String(input?.value || "").trim();
  const model = String(
    pop?.querySelector(".model-test-model-select")?.value || "",
  ).trim();
  if (!pop || !prompt || !model || pop._modelTestRunning) return;
  pop._modelTestModel = model;
  const messages = buildModelTestMessages(pop._modelTestMessages, prompt);
  const empty = pop.querySelector(".model-test-empty");
  const thread = pop.querySelector(".model-test-thread");
  if (empty) empty.hidden = true;
  if (thread) thread.hidden = false;
  const exchange = document.createElement("div");
  exchange.className = "model-test-exchange";
  exchange.innerHTML = buildModelTestTurnHtml(model);
  exchange.querySelector(".model-test-user-content").textContent = prompt;
  thread.appendChild(exchange);
  const turn = exchange.querySelector(".model-test-assistant-turn");
  pop._modelTestActiveTurn = turn;
  pop._modelTestOutputMarkdown = "";
  pop._modelTestFollowOutput = true;
  pop._modelTestStickToBottom = true;
  resetModelTestThinking(pop);
  pop.querySelector(".model-test-followups").hidden = false;
  const conversation = pop.querySelector(".model-test-conversation");
  conversation.scrollTop = conversation.scrollHeight;
  input.value = "";
  resizeModelTestInput(input);
  pop._modelTestRunning = true;
  pop._modelTestStopped = false;
  const controller = new AbortController();
  pop._modelTestController = controller;
  updateModelTestControls(pop);
  setModelTestStatus(pop, "连接中", "running");
  const started = performance.now();
  let reader = null;
  let state = "complete";
  let firstResponse = null;
  let usage = null;
  let completed = false;
  let sawReasoning = false;
  let sawToolCalls = false;
  let finishReason = "";
  const consume = (event) => {
    if (!event) return false;
    if (event.error) throw new Error(event.error);
    if (firstResponse === null && (event.text || event.reasoning)) firstResponse = performance.now() - started;
    if (event.usage) usage = event.usage;
    if (event.finishReason) finishReason = event.finishReason;
    if (event.hasToolCalls) sawToolCalls = true;
    if (event.reasoning) sawReasoning = true;
    if (event.done || event.finished) completed = true;
    appendModelTestThinking(pop, event.reasoning);
    appendModelTestOutput(pop, event.text);
    return Boolean(event.done);
  };

  const attemptStream = async (attempt) => {
    completed = false;
    usage = null;
    sawReasoning = false;
    sawToolCalls = false;
    finishReason = "";
    if (attempt > 0) setModelTestStatus(pop, "重新生成", "running");
    const response = await apiFetch("/api/test-model", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(
        pop._modelTestMode === "alias"
          ? { alias: model, messages }
          : { upstream: pop._modelTestUpstream, model, messages },
      ),
      signal: controller.signal,
    });
    if (!response.ok) {
      throw new Error(modelTestHTTPError(await response.text(), response.status, response));
    }
    showModelTestRoute(pop, modelTestResponseRoute(response));
    if (!response.body) throw new Error("当前浏览器不支持流式响应");

    setModelTestStatus(pop, "生成中", "running");
    reader = response.body.getReader();
    const decoder = new TextDecoder();
    const parser = createModelTestSSEParser();
    let rawBody = "";
    let sawEvent = false;
    let streamDone = false;
    while (!streamDone) {
      const chunk = await reader.read();
      let piece = chunk.value ? decoder.decode(chunk.value, { stream: true }) : "";
      if (chunk.done) piece += decoder.decode();
      rawBody += piece;
      const events = parser.push(piece, Boolean(chunk.done));
      for (const frame of events) {
        sawEvent = true;
        if (consume(modelTestStreamEvent(frame))) {
          streamDone = true;
          break;
        }
      }
      if (chunk.done) streamDone = true;
    }
    if (!sawEvent && !pop._modelTestStopped) {
      const fallback = modelTestNonStreamEvent(rawBody);
      if (fallback) {
        sawEvent = true;
        consume(fallback);
      }
    }
    if (reader) {
      await reader.cancel().catch(() => {});
      reader = null;
    }
  };

  try {
    await saveConfigSilent({ skipModelSync: true });
    if (controller.signal.aborted) throw new DOMException("已停止", "AbortError");
    // 上游偶发返回空流（只有 role 或只有 [DONE]）时静默重试一次；
    // 有明确结束原因、思考内容或工具调用时保留本次结果，不重复请求。
    for (let attempt = 0; ; attempt += 1) {
      await attemptStream(attempt);
      if (pop._modelTestStopped || controller.signal.aborted) break;
      if (String(pop._modelTestOutputMarkdown || "").trim()) break;
      if (finishReason || sawReasoning || sawToolCalls || attempt >= 1) break;
      await new Promise((resolve) => setTimeout(resolve, 600));
      if (controller.signal.aborted) throw new DOMException("已停止", "AbortError");
    }
    if (!completed && !pop._modelTestStopped) throw new Error("连接提前结束，请重试");
    if (!String(pop._modelTestOutputMarkdown || "").trim() && !pop._modelTestStopped) {
      throw new Error(modelTestEmptyTextReason(finishReason, sawReasoning, sawToolCalls));
    }
    flushModelTestOutput(pop);
    finishModelTestThinking(pop, "complete");
    if (!pop._modelTestStopped) {
      pop._modelTestMessages = messages.concat({ role: "assistant", content: pop._modelTestOutputMarkdown });
      updateModelTestContext(pop);
      const elapsed = Math.round(performance.now() - started);
      setModelTestStatus(pop, "已完成 · " + elapsed + "ms", "complete");
    }
  } catch (e) {
    if (e.name === "AbortError" || pop._modelTestStopped) {
      state = "stopped";
      finishModelTestThinking(pop, "stopped");
      setModelTestStatus(pop, "已停止", "stopped");
    } else {
      state = "error";
      finishModelTestThinking(pop, "error");
      setModelTestStatus(pop, "请求失败", "error");
      const error = document.createElement("p");
      error.className = "model-test-error";
      error.textContent = "请求失败：" + (e.message || "未知错误");
      turn.appendChild(error);
    }
  } finally {
    flushModelTestOutput(pop);
    if (pop._modelTestStopped) state = "stopped";
    turn._modelTestAnswer = pop._modelTestOutputMarkdown || "";
    turn.dataset.state = state;
    const output = turn.querySelector(".model-test-output");
    if (!turn._modelTestAnswer) output.dataset.empty = "false";
    const footer = turn.querySelector(".model-test-response-footer");
    footer.hidden = false;
    footer.querySelector(".model-test-answer-copy").disabled = !turn._modelTestAnswer;
    const elapsed = ((performance.now() - started) / 1000).toFixed(1) + " 秒";
    const tokens = Number(usage?.total_tokens) || Number(usage?.prompt_tokens || usage?.input_tokens || 0) + Number(usage?.completion_tokens || usage?.output_tokens || 0);
    const metrics = [elapsed];
    if (firstResponse !== null) metrics.push("首响 " + (firstResponse / 1000).toFixed(1) + " 秒");
    if (tokens > 0) metrics.push(tokens.toLocaleString() + " Tokens");
    turn.querySelector(".model-test-turn-status").textContent = state === "complete" ? metrics.join(" · ") : (state === "stopped" ? "已停止" : "请求失败") + " · 本轮未加入上下文";
    if (reader) await reader.cancel().catch(() => {});
    if (pop._modelTestController === controller) {
      pop._modelTestController = null;
      pop._modelTestRunning = false;
      updateModelTestControls(pop);
      if (pop._modelTestFollowOutput) conversation.scrollTop = conversation.scrollHeight;
      if (pop.isConnected) input.focus({ preventScroll: true });
    }
    updateModelTestScroll(conversation);
  }
}

function stopModelTestGeneration(btn) {
  const pop = btn.closest(".model-test-popover");
  if (!pop?._modelTestRunning) return;
  pop._modelTestStopped = true;
  setModelTestStatus(pop, "正在停止", "stopped");
  pop._modelTestController?.abort();
}

function refreshModelsInPopover(btn) {
  const pop = btn.closest(".popover");
  if (!pop) return;
  const trigger = pop._triggerEl;
  if (!trigger) return;
  const upstreamName = pop._upstreamName || "";
  const listEl = pop.querySelector(".model-list");
  const currentChecked = getCheckedModelsFromPopover(pop);
  const manualInput = pop.querySelector(".popover-input");
  const manualModels = parseModelIDs(manualInput?.value || "");
  const selectedBeforeRefresh = Array.from(
    new Set([...currentChecked, ...manualModels]),
  );
  btn.disabled = true;
  btn.classList.add("is-syncing");
  if (listEl) {
    listEl.classList.add("is-syncing");
    listEl.insertAdjacentHTML(
      "beforeend",
      '<div class="model-loading-overlay"><span class="model-loading-spinner">' +
        ICONS.sync +
        "</span><span>正在从上游同步模型...</span></div>",
    );
  }

  refreshModelList(upstreamName).then((ok) => {
    if (!ok) {
      listEl?.querySelector(".model-loading-overlay")?.remove();
      listEl?.classList.remove("is-syncing");
      const searchInput = pop.querySelector(".model-search input");
      if (searchInput?.value) filterModelList(searchInput);
      else updateModelSelectionSummary(pop);
      showToast("同步失败，已保留原模型列表和选择", "error");
      return;
    }
    const available = modelsForUpstream(upstreamName);
    const value = trigger.dataset.value || "";
    const savedSelected = parseModelIDs(value);
    const allSelected = Array.from(
      new Set([...savedSelected, ...selectedBeforeRefresh]),
    );
    const displayModels = mergeModelOptions(available, allSelected);

    let modelsHtml = "";
    if (displayModels.length === 0) {
      modelsHtml =
        '<div class="model-empty">该上游暂无可用模型，请手动输入</div>';
    } else {
      modelsHtml = buildModelChecksHtml(displayModels, allSelected, available);
    }
    if (listEl) {
      listEl.classList.remove("is-syncing");
      listEl.innerHTML = modelsHtml;
    }
    const summary = pop.querySelector(".model-selection-summary");
    if (summary) summary.dataset.total = String(displayModels.length);
    if (manualInput) manualInput.value = "";
    const searchInput = pop.querySelector(".model-search input");
    if (searchInput && searchInput.value) filterModelList(searchInput);
    else updateModelSelectionSummary(pop);
    const missingCount = allSelected.filter(
      (m) => !available.includes(m),
    ).length;
    const suffix = missingCount
      ? "，保留 " + missingCount + " 个已选缺失模型"
      : "";
    showToast(
      "已从上游同步 " + available.length + " 个模型" + suffix,
      "success",
    );
  }).finally(() => {
    btn.disabled = false;
    btn.classList.remove("is-syncing");
  });
}

async function syncModels(btn) {
  showToast("正在保存配置并同步模型...", "success");
  const row = btn.closest("tr");
  const el = row.querySelector('[data-field="custom_models"]');
  const upstreamName = el ? getUpstreamNameForRow(el) : "";
  if (!upstreamName) {
    showToast("请先填写上游名称", "error");
    return;
  }
  const ok = await refreshModelList(upstreamName);
  if (!ok) {
    showToast("同步失败，请检查当前上游配置", "error");
    return;
  }
  if (el) openCustomModelsEditor(el);
}

function modelAliasesForUpstreamModel(upstreamName, model) {
  const aliases = [];
  Object.keys(aliasData || {}).forEach((aliasName) => {
    const targets = normalizeAliasTargets(aliasData[aliasName]?.targets || []);
    if (
      targets.some(
        (target) =>
          target.upstream === upstreamName && target.target_model === model,
      )
    ) {
      aliases.push(aliasName);
    }
  });
  return aliases.sort((a, b) => a.localeCompare(b));
}

function buildAllModelSyncPlan(payload) {
  const results = Array.isArray(payload?.upstreams) ? payload.upstreams : [];
  const resultByUpstream = {};
  results.forEach((result) => {
    const name = String(result?.upstream || "").trim();
    if (name) resultByUpstream[name] = result || {};
  });
  return orderedUpstreamNames().map((upstreamName) => {
    const result = resultByUpstream[upstreamName] || {
      upstream: upstreamName,
      error: "同步接口未返回该上游的结果",
    };
    const configured = Array.from(
      new Set(
        (Array.isArray(upstreamData[upstreamName]?.custom_models)
          ? upstreamData[upstreamName].custom_models
          : []
        )
          .map((model) => String(model || "").trim())
          .filter(Boolean),
      ),
    );
    const discovered = Array.from(
      new Set(
        (Array.isArray(result.models) ? result.models : [])
          .map((model) => String(model || "").trim())
          .filter(Boolean),
      ),
    ).sort((a, b) => a.localeCompare(b));
    const configuredSet = new Set(configured);
    const discoveredSet = new Set(discovered);
    const error = String(result.error || "").trim();
    const emptyCatalog = !error && discovered.length === 0;
    return {
      upstream: upstreamName,
      configured: configured,
      discovered: discovered,
      error: error,
      emptyCatalog: emptyCatalog,
      additions: error
        ? []
        : discovered.filter((model) => !configuredSet.has(model)),
      missing: error
        ? []
        : configured.filter((model) => !discoveredSet.has(model)),
      deleted: [],
    };
  });
}

function modelSyncPopoverHeaderHtml(phase) {
  return (
    '<div class="popover-header model-sync-popover-header"><span class="popover-title">' +
    ICONS.sync +
    '<span class="model-popover-heading"><span>同步所有上游模型</span><strong class="model-sync-phase">' +
    esc(phase || "") +
    "</strong></span></span>" +
    '<button type="button" class="btn-icon model-sync-review-close" onclick="closePopover()" title="关闭">' +
    ICONS.close +
    "</button></div>"
  );
}

function openModelSyncLoading(triggerEl, upstreamCount, onClose) {
  const pop = openPopover(
    triggerEl,
    modelSyncPopoverHeaderHtml("准备同步 " + upstreamCount + " 个渠道") +
      '<div class="model-search-row model-sync-search-row"><div class="model-search">' +
      ICONS.search +
      '<input type="search" placeholder="搜索模型或渠道..." disabled></div></div>' +
      '<div class="model-list model-sync-list is-syncing"><div class="model-loading-overlay"><span class="model-loading-spinner">' +
      ICONS.sync +
      '</span><span class="model-sync-loading-text">正在保存当前配置...</span></div></div>' +
      '<div class="popover-hint model-sync-loading-hint">弹窗已打开，正在并行读取所有上游模型；同步失败的渠道不会删除任何现有模型。</div>',
    onClose,
  );
  pop.classList.add("model-sync-review-popover", "model-picker-popover");
  pop.setAttribute("role", "dialog");
  pop.setAttribute("aria-modal", "true");
  return pop;
}

function setModelSyncLoadingPhase(pop, phase) {
  if (!pop) return;
  const phaseEl = pop.querySelector(".model-sync-phase");
  if (phaseEl) phaseEl.textContent = phase;
  const loadingEl = pop.querySelector(".model-sync-loading-text");
  if (loadingEl) loadingEl.textContent = phase;
}

function modelSyncAdditionRowHtml(item, model) {
  const searchValue = (item.upstream + " " + model + " 新增").toLowerCase();
  return (
    '<label class="model-check model-sync-row is-addition model-sync-search-item" data-sync-kind="addition" data-sync-search="' +
    escAttr(searchValue) +
    '"><span class="model-check-label"><input type="checkbox" checked data-sync-action="add" data-upstream="' +
    escAttr(item.upstream) +
    '" data-model="' +
    escAttr(model) +
    '" onchange="updateModelSyncReviewSummary(this)"><span class="alias-target-route-combo model-sync-route"><span class="alias-target-route-upstream" title="' +
    escAttr(item.upstream) +
    '">' +
    esc(item.upstream) +
    '</span><span class="alias-target-route-arrow" aria-hidden="true">' +
    ICONS.arrowRight +
    '</span><span class="alias-target-route-model" title="' +
    escAttr(model) +
    '">' +
    esc(model) +
    '</span></span><span class="model-sync-status is-addition">新增</span></span></label>'
  );
}

function modelSyncDeletedRowHtml(item, detail) {
  const aliases = Array.isArray(detail.aliases) ? detail.aliases : [];
  const aliasHint = aliases.length
    ? '<small>已同步清理映射：' + esc(aliases.join("、")) + "</small>"
    : "";
  const searchValue = (
    item.upstream +
    " " +
    detail.model +
    " 已删除 " +
    aliases.join(" ")
  ).toLowerCase();
  return (
    '<div class="model-check model-sync-row is-deleted model-sync-search-item" data-sync-kind="deleted" data-sync-search="' +
    escAttr(searchValue) +
    '"><div class="model-check-label"><span class="model-sync-row-icon">' +
    ICONS.trash +
    '</span><span class="alias-target-upstream" title="渠道 ' +
    escAttr(item.upstream) +
    '">' +
    esc(item.upstream) +
    '</span><span class="model-sync-model-cell"><span class="model-id" title="' +
    escAttr(detail.model) +
    '">' +
    esc(detail.model) +
    "</span>" +
    aliasHint +
    '</span><span class="model-sync-status is-deleted">已自动删除</span></div></div>'
  );
}

function modelSyncMessageHtml(item, type, title, description) {
  const searchValue = (
    item.upstream +
    " " +
    title +
    " " +
    description
  ).toLowerCase();
  const kind =
    type === "is-error" ? "error" : type === "is-warning" ? "deleted" : "noop";
  return (
    '<div class="model-sync-message ' +
    type +
    ' model-sync-search-item" data-sync-kind="' +
    kind +
    '" data-sync-search="' +
    escAttr(searchValue) +
    '"><span>' +
    (type === "is-current" ? ICONS.check : ICONS.alert) +
    "</span><div><strong>" +
    esc(title) +
    "</strong><p>" +
    esc(description) +
    "</p></div></div>"
  );
}

function modelSyncChannelStat(item, additions, deleted) {
  if (item.error) return { text: "同步失败", cls: "is-error" };
  if (item.emptyCatalog && deleted.length)
    return { text: "已清空 " + deleted.length, cls: "is-warning" };
  const add = additions.length;
  const del = deleted.length;
  if (!add && !del) return { text: "无需修改", cls: "is-noop" };
  return { text: "新增 " + add + " · 删除 " + del, cls: "is-addition" };
}

function modelSyncChannelAvatarColor(name) {
  const palette = [
    "var(--accent)",
    "var(--green)",
    "var(--orange)",
    "var(--blue)",
    "var(--red)",
  ];
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = (hash * 31 + name.charCodeAt(i)) | 0;
  return palette[Math.abs(hash) % palette.length];
}

function modelSyncChannelHtml(item) {
  const additions = item.additions || [];
  const deleted = item.deleted || [];
  let rows = additions
    .map((model) => modelSyncAdditionRowHtml(item, model))
    .join("");
  rows += deleted
    .map((detail) => modelSyncDeletedRowHtml(item, detail))
    .join("");
  if (item.error) {
    rows += modelSyncMessageHtml(
      item,
      "is-error",
      "同步失败，已保留现有模型",
      item.error,
    );
  } else if (item.emptyCatalog && deleted.length) {
    rows += modelSyncMessageHtml(
      item,
      "is-warning",
      "上游返回空模型列表",
      "本渠道同步成功，原有模型均已按结果删除。",
    );
  } else if (!additions.length && !deleted.length) {
    rows += modelSyncMessageHtml(
      item,
      "is-current",
      "无需修改",
      "当前配置与上游返回的模型一致。",
    );
  }
  const stat = modelSyncChannelStat(item, additions, deleted);
  const avatarLetter = String(item.upstream || "?").trim().charAt(0) || "?";
  const channelSearch = (item.upstream + " " + stat.text).toLowerCase();
  return (
    '<section class="model-sync-channel is-collapsed" data-sync-channel="' +
    escAttr(item.upstream) +
    '" data-sync-deleted-count="' +
    deleted.length +
    '" data-sync-search="' +
    escAttr(channelSearch) +
    '"><div class="model-sync-channel-top"><button type="button" class="model-sync-channel-header" onclick="toggleModelSyncChannel(this)" aria-expanded="false"><span class="model-sync-channel-caret" aria-hidden="true">' +
    ICONS.chevron +
    '</span><span class="model-sync-channel-avatar" style="--avatar-color: ' +
    escAttr(modelSyncChannelAvatarColor(String(item.upstream || ""))) +
    '">' +
    esc(avatarLetter) +
    '</span><span class="model-sync-channel-title"><span class="model-sync-channel-name" title="渠道 ' +
    escAttr(item.upstream) +
    '">' +
    esc(item.upstream) +
    '</span><span class="model-sync-channel-count ' +
    stat.cls +
    '" data-sync-channel-count="' +
    escAttr(item.upstream) +
    '">' +
    stat.text +
    "</span></span></button>" +
    '<div class="model-sync-channel-side">' +
    (additions.length
      ? '<span class="model-sync-channel-actions"><button type="button" class="model-sync-channel-tool" onclick="setModelSyncChannelSelection(this,true)">全选新增</button><button type="button" class="model-sync-channel-tool" onclick="setModelSyncChannelSelection(this,false)">取消全选</button></span>'
      : "") +
    '<button type="button" class="model-sync-channel-collapse" onclick="toggleModelSyncChannel(this)" title="折叠 / 展开" aria-expanded="false" aria-label="折叠 / 展开渠道">' +
    ICONS.chevron +
    "</button></div></div>" +
    '<div class="model-sync-channel-models" hidden>' +
    rows +
    "</div></section>"
  );
}

function toggleModelSyncChannel(button) {
  const section =
    button?.closest?.(".model-sync-channel") || button?.closest?.("section");
  if (!section) return;
  const collapsed = section.classList.toggle("is-collapsed");
  const models = section.querySelector(".model-sync-channel-models");
  if (models) models.hidden = collapsed;
  const headers = section.querySelectorAll(
    ".model-sync-channel-header, .model-sync-channel-collapse",
  );
  headers.forEach((el) =>
    el.setAttribute("aria-expanded", collapsed ? "false" : "true"),
  );
  section._userToggled = true;
}

function updateModelSyncReviewSummary(source) {
  const pop = source?.closest?.(".model-sync-review-popover") || activePopover;
  if (!pop) return;
  const selectedAdditions = pop.querySelectorAll(
    '[data-sync-action="add"]:checked',
  ).length;
  const totalAdditions = pop.querySelectorAll('[data-sync-action="add"]').length;
  const summary = pop.querySelector(".model-sync-selection-summary");
  if (summary) {
    summary.innerHTML =
      "已选择 <strong>" +
      selectedAdditions +
      "</strong> / " +
      totalAdditions +
      " 个新增模型";
  }
  pop.querySelectorAll(".model-sync-channel").forEach((section) => {
    const channel = section.dataset.syncChannel || "";
    const total = section.querySelectorAll('[data-sync-action="add"]').length;
    const selected = section.querySelectorAll(
      '[data-sync-action="add"]:checked',
    ).length;
    const count = section.querySelector("[data-sync-channel-count]");
    const deleted = Number(section.dataset.syncDeletedCount || 0);
    if (count && total)
      count.textContent =
        "新增 " +
        selected +
        "/" +
        total +
        " 已选" +
        (deleted ? " · 已删除 " + deleted : "");
    if (count && !total && !count.dataset.originalText)
      count.dataset.originalText = count.textContent;
    if (count && !total && count.dataset.originalText)
      count.textContent = count.dataset.originalText;
    if (count) count.dataset.channel = channel;
  });
  const applyButton = pop.querySelector(".model-sync-apply");
  if (applyButton) applyButton.disabled = selectedAdditions === 0;
}

function setModelSyncChannelSelection(button, checked) {
  const section = button.closest(".model-sync-channel");
  const pop = button.closest(".model-sync-review-popover");
  if (!section || !pop) return;
  section.querySelectorAll('[data-sync-action="add"]').forEach((input) => {
    input.checked = checked;
  });
  updateModelSyncReviewSummary(pop);
}

function applyModelSyncListFilter(pop) {
  if (!pop) return;
  const activeKind = pop._modelSyncKind || "addition";
  const searchInput = pop.querySelector(".model-search input");
  const term = String(searchInput?.value || "").trim().toLowerCase();
  const filtering = !!(term || (activeKind && activeKind !== "addition"));
  const isChannelScopedKind = activeKind === "all";
  pop
    .querySelectorAll(".model-sync-result-summary [data-sync-kind]")
    .forEach((btn) => {
      btn.classList.toggle("is-active", btn.dataset.syncKind === activeKind);
      btn.setAttribute("aria-pressed", btn.dataset.syncKind === activeKind);
    });
  let visibleSections = 0;
  pop.querySelectorAll(".model-sync-channel").forEach((section) => {
    const channelMatch = !term || (section.dataset.syncSearch || "").includes(term);
    let visibleItems = 0;
    section.querySelectorAll(".model-sync-search-item").forEach((row) => {
      const rowKind = row.dataset.syncKind || "addition";
      const kindVisible = activeKind === "all" || rowKind === activeKind;
      const searchVisible = !term || channelMatch || (row.dataset.syncSearch || "").includes(term);
      const visible = kindVisible && searchVisible;
      row.hidden = !visible;
      if (visible) visibleItems++;
    });
    section.hidden = visibleItems === 0;
    if (!section.hidden) {
      visibleSections++;
      if (filtering && !section._userToggled) {
        section.classList.remove("is-collapsed");
        const models = section.querySelector(".model-sync-channel-models");
        if (models) models.hidden = false;
        section
          .querySelectorAll(".model-sync-channel-header, .model-sync-channel-collapse")
          .forEach((el) => el.setAttribute("aria-expanded", "true"));
      }
    }
  });
  const empty = pop.querySelector(".model-sync-filter-empty");
  if (empty) empty.hidden = visibleSections > 0;
}

function filterModelSyncReview(input) {
  const pop = input?.closest?.(".model-sync-review-popover") || activePopover;
  if (!pop) return;
  applyModelSyncListFilter(pop);
}

function filterModelSyncKind(button) {
  const pop = button?.closest?.(".model-sync-review-popover") || activePopover;
  if (!pop || button.disabled) return;
  const kind = button.dataset.syncKind || "addition";
  pop._modelSyncKind = (pop._modelSyncKind || "addition") === kind ? "all" : kind;
  applyModelSyncListFilter(pop);
  updateModelSyncReviewSummary(pop);
  const searchInput = pop.querySelector(".model-search input");
  if (searchInput) searchInput.focus();
}

function renderModelSyncReview(pop, plan, payload, cleanup) {
  const additionCount = plan.reduce(
    (total, item) => total + item.additions.length,
    0,
  );
  const deletedCount = plan.reduce(
    (total, item) => total + item.deleted.length,
    0,
  );
  const noopCount = plan.reduce((total, item) => {
    if (item.error) return total;
    if (item.emptyCatalog && item.deleted.length) return total;
    if (item.additions.length || item.deleted.length) return total;
    return total + 1;
  }, 0);
  const failedCount = Number(payload?.failed || 0);
  const mappingText = cleanup?.targetCount
    ? "；同步清理 " +
      cleanup.targetCount +
      " 个映射目标" +
      (cleanup.removedAliasCount
        ? "，删除 " + cleanup.removedAliasCount + " 个空别名"
        : "")
    : "";
  pop.innerHTML =
    modelSyncPopoverHeaderHtml(
      "成功 " + Number(payload?.succeeded || 0) + " · 失败 " + failedCount,
    ) +
    '<div class="model-search-row model-sync-search-row"><div class="model-search">' +
    ICONS.search +
    '<input type="search" placeholder="搜索模型或渠道..." oninput="filterModelSyncReview(this)"></div><span class="model-sync-selection-summary"></span></div>' +
    '<div class="model-sync-result-summary"><button type="button" class="is-addition is-active" data-sync-kind="addition" aria-pressed="true" onclick="filterModelSyncKind(this)">待新增 <strong>' +
    additionCount +
    '</strong></button><button type="button" class="is-deleted" data-sync-kind="deleted" aria-pressed="false"' +
    (deletedCount ? "" : " disabled") +
    ' onclick="filterModelSyncKind(this)">已自动删除 <strong>' +
    deletedCount +
    '</strong></button><button type="button" class="' +
    (failedCount ? "is-error" : "is-error") +
    '" data-sync-kind="error" aria-pressed="false"' +
    (failedCount ? "" : " disabled") +
    ' onclick="filterModelSyncKind(this)">同步失败 <strong>' +
    failedCount +
    '</strong></button><button type="button" class="is-noop" data-sync-kind="noop" aria-pressed="false"' +
    (noopCount ? "" : " disabled") +
    ' onclick="filterModelSyncKind(this)">无需修改 <strong>' +
    noopCount +
    "</strong></button></div>" +
    '<div class="model-list model-sync-list">' +
    plan.map(modelSyncChannelHtml).join("") +
    '<div class="model-sync-filter-empty" hidden>' +
    ICONS.search +
    "<span>没有匹配的模型或渠道</span></div></div>" +
    '<div class="model-sync-review-footer"><p><span>' +
    ICONS.alert +
    "</span>成功渠道中上游已不存在的模型已自动删除，失败渠道保持不变" +
    mappingText +
    '。</p><div><button type="button" class="btn btn-default" onclick="closePopover()">关闭</button><button type="button" class="btn btn-success model-sync-apply" onclick="applyModelSyncReview(this)">' +
    ICONS.save +
    "<span>保存所选新增</span></button></div></div>";
  pop._modelSyncPlan = plan;
  pop._modelSyncKind = "addition";
  updateModelSyncReviewSummary(pop);
  applyModelSyncListFilter(pop);
  setTimeout(() => pop.querySelector(".model-search input")?.focus(), 0);
}

function cloneAdminState(value) {
  return JSON.parse(JSON.stringify(value || {}));
}

async function applySynchronizedModelDeletions(plan) {
  const removedByUpstream = {};
  const deletedDetails = {};
  plan.forEach((item) => {
    if (item.error || !item.missing.length) return;
    removedByUpstream[item.upstream] = new Set(item.missing);
    deletedDetails[item.upstream] = item.missing.map((model) => ({
      model: model,
      aliases: modelAliasesForUpstreamModel(item.upstream, model),
    }));
  });
  const deletedCount = Object.values(deletedDetails).reduce(
    (total, models) => total + models.length,
    0,
  );
  if (!deletedCount) {
    return { targetCount: 0, removedAliasCount: 0, aliases: [] };
  }

  const beforeUpstreams = cloneAdminState(upstreamData);
  const beforeAliases = cloneAdminState(aliasData);
  const beforeSelectedAlias = selectedAliasKey;
  Object.keys(removedByUpstream).forEach((upstreamName) => {
    const removed = removedByUpstream[upstreamName];
    const current = Array.isArray(upstreamData[upstreamName]?.custom_models)
      ? upstreamData[upstreamName].custom_models
      : [];
    upstreamData[upstreamName] = {
      ...upstreamData[upstreamName],
      custom_models: current.filter((model) => !removed.has(model)),
    };
  });
  const impact = upstreamModelAliasDeleteImpact(removedByUpstream);
  const selectedAliasRemoved = applyUpstreamAliasDelete(impact);
  renderUpstreamTable();
  renderAliasTable();
  if (selectedAliasRemoved) showSelectedEffortMap();
  renderUpstreamModelFilter();
  filterUpstreamRows();

  try {
    await saveConfigSilent({ skipModelSync: true });
    plan.forEach((item) => {
      item.deleted = deletedDetails[item.upstream] || [];
      item.missing = [];
    });
    return impact;
  } catch (e) {
    upstreamData = beforeUpstreams;
    aliasData = beforeAliases;
    selectedAliasKey = beforeSelectedAlias;
    renderUpstreamTable();
    renderAliasTable();
    showSelectedEffortMap();
    renderUpstreamModelFilter();
    filterUpstreamRows();
    throw e;
  }
}

async function applyModelSyncReview(button) {
  const pop = button.closest(".model-sync-review-popover");
  if (!pop) return;
  const changes = Array.from(
    pop.querySelectorAll('[data-sync-action="add"]:checked'),
  ).map((input) => ({
    upstream: input.dataset.upstream,
    model: input.dataset.model,
  }));
  if (!changes.length) return;

  const beforeUpstreams = cloneAdminState(upstreamData);
  const modelsByUpstream = upstreamModelsSnapshot(upstreamData);
  changes.forEach((change) => {
    const models = new Set(modelsByUpstream[change.upstream] || []);
    models.add(change.model);
    modelsByUpstream[change.upstream] = Array.from(models);
  });

  Object.keys(modelsByUpstream).forEach((name) => {
    if (!upstreamData[name]) return;
    upstreamData[name] = {
      ...upstreamData[name],
      custom_models: modelsByUpstream[name].sort((a, b) => a.localeCompare(b)),
    };
  });
  renderUpstreamTable();
  renderUpstreamModelFilter();
  filterUpstreamRows();

  button.disabled = true;
  button.classList.add("is-loading");
  try {
    await saveConfigSilent({ skipModelSync: true });
    closePopover();
    showToast(
      "已保存 " + changes.length + " 个新增上游模型",
      "success",
    );
    loadConfig();
  } catch (e) {
    upstreamData = beforeUpstreams;
    renderUpstreamTable();
    renderUpstreamModelFilter();
    filterUpstreamRows();
    if (String(e.message || "").indexOf("登录已失效") === -1)
      showToast("同步结果保存失败：" + e.message, "error");
  } finally {
    button.disabled = false;
    button.classList.remove("is-loading");
  }
}

async function syncAllUpstreamModels(button) {
  if (!button || button.disabled) return;
  collectUpstreams();
  if (!Object.keys(upstreamData).length) {
    showToast("请先添加并保存至少一个上游", "error");
    return;
  }
  const aliasError = validateAliasRows();
  if (aliasError) {
    showToast(aliasError.message, "error");
    aliasError.element?.scrollIntoView({ behavior: "smooth", block: "center" });
    aliasError.element?.focus?.();
    return;
  }

  const controller = new AbortController();
  const pop = openModelSyncLoading(
    button,
    Object.keys(upstreamData).length,
    () => controller.abort(),
  );
  const originalHTML = button.innerHTML;
  button.disabled = true;
  button.classList.add("is-loading");
  button.innerHTML = ICONS.sync + "<span>正在同步...</span>";
  try {
    setModelSyncLoadingPhase(pop, "正在保存当前配置...");
    await saveConfigSilent({ skipModelSync: true });
    if (activePopover !== pop) return;
    setModelSyncLoadingPhase(pop, "正在并行读取所有上游模型...");
    const payload = await apiJSON("/api/sync-models", {
      method: "POST",
      signal: controller.signal,
    });
    if (activePopover !== pop) return;
    const plan = buildAllModelSyncPlan(payload);
    plan.forEach((item) => {
      if (!item.error) modelListByUpstream[item.upstream] = item.discovered;
    });
    setModelSyncLoadingPhase(pop, "正在自动清理上游已删除的模型...");
    const cleanup = await applySynchronizedModelDeletions(plan);
    if (activePopover !== pop) return;
    renderUpstreamModelFilter();
    filterUpstreamRows();
    renderModelSyncReview(pop, plan, payload, cleanup);
  } catch (e) {
    if (e?.name === "AbortError") return;
    if (String(e.message || "").indexOf("登录已失效") !== -1) return;
    if (activePopover === pop) {
      pop.innerHTML =
        modelSyncPopoverHeaderHtml("同步中断") +
        '<div class="model-sync-fatal">' +
        ICONS.alert +
        "<div><strong>同步所有上游模型失败</strong><p>" +
        esc(e.message || "未知错误") +
        '</p></div></div><div class="model-sync-review-footer"><p>现有模型配置保持在最近一次成功保存的状态。</p><div><button type="button" class="btn btn-default" onclick="closePopover()">关闭</button></div></div>';
    }
    showToast("同步所有上游模型失败：" + e.message, "error");
  } finally {
    button.disabled = false;
    button.classList.remove("is-loading");
    button.innerHTML = originalHTML;
  }
}

/* ===== 别名表格 ===== */
function modelsForUpstream(name) {
  const resolved = String(name || "").trim();
  return modelListByUpstream[resolved] || [];
}

function selectedModelsForUpstream(name) {
  const resolved = String(name || "").trim();
  const upstream = upstreamData[resolved] || {};
  return Array.isArray(upstream.custom_models) ? upstream.custom_models : [];
}

function aliasTargetIdentity(upstream, model) {
  return String(upstream || "").trim() + "\u0000" + String(model || "").trim();
}

function normalizeAliasTargets(value) {
  const source = Array.isArray(value) ? value : [];
  const targets = [];
  const seen = new Set();
  source.forEach((raw) => {
    if (!raw || typeof raw !== "object") return;
    const upstream = String(raw.upstream || "").trim();
    const targetModel = String(raw.target_model || "").trim();
    if (!upstream || !targetModel) return;
    const identity = aliasTargetIdentity(upstream, targetModel);
    if (seen.has(identity)) return;
    seen.add(identity);
    const parsedWeight = Number.parseInt(raw.weight, 10);
    const hasEnabled = typeof raw.enabled === "boolean";
    targets.push({
      upstream: upstream,
      target_model: targetModel,
      weight:
        Number.isFinite(parsedWeight)
          ? Math.min(Math.max(parsedWeight, 1), 1000000)
          : 1,
      enabled: hasEnabled
        ? raw.enabled
        : !Number.isFinite(parsedWeight) || parsedWeight !== 0,
    });
  });
  return targets;
}

function parseAliasTargets(value) {
  if (Array.isArray(value)) return normalizeAliasTargets(value);
  try {
    return normalizeAliasTargets(JSON.parse(String(value || "[]")));
  } catch (_) {
    return [];
  }
}

function aliasTargetsTitle(targets) {
  if (!targets.length) return "编辑上游模型";
  const upstreamCount = new Set(targets.map((target) => target.upstream)).size;
  return (
    targets.length +
    " 个模型 · " +
    upstreamCount +
    " 个上游\n" +
    targets
      .map(
        (target) =>
          target.upstream +
          " / " +
          target.target_model +
          (target.enabled ? "" : "（已禁用）"),
      )
      .join("\n")
  );
}

function aliasTargetsDisplay(targets) {
  if (!targets.length)
    return '<span class="field-placeholder">点击添加上游模型</span>';
  const sortedTargets = sortAliasTargetsForDisplay(targets);
  const first = sortedTargets[0];
  const remaining = targets.length - 1;
  const upstreamCount = new Set(targets.map((target) => target.upstream)).size;
  return (
    '<span class="alias-target-summary">' +
    '<span class="alias-target-upstream-total" title="共 ' +
    upstreamCount +
    ' 个上游">' +
    ICONS.server +
    "<strong>" +
    upstreamCount +
    "</strong><span>上游</span></span>" +
    '<span class="alias-target-route-combo">' +
    '<span class="alias-target-route-upstream" title="' +
    escAttr(first.upstream) +
    '">' +
    esc(first.upstream) +
    '</span><span class="alias-target-route-arrow" aria-hidden="true">' +
    ICONS.arrowRight +
    '</span><span class="alias-target-route-model" title="' +
    escAttr(first.target_model) +
    '">' +
    esc(first.target_model) +
    "</span></span>" +
    (remaining > 0
      ? '<span class="alias-target-more" title="另有 ' +
        remaining +
        ' 个模型">+' +
        remaining +
        "</span>"
      : "") +
    "</span>"
  );
}

function aliasTargetsHaveRoutableTarget(targets) {
  const normalized = normalizeAliasTargets(targets);
  return normalized.some((target) => target.enabled && target.weight > 0);
}

function aliasTargetsWeightWarningHtml(targets) {
  const normalized = normalizeAliasTargets(targets);
  if (!normalized.length || aliasTargetsHaveRoutableTarget(normalized)) return "";
  return (
    '<span class="alias-target-weight-warning" title="当前映射不会参与路由，请启用至少一个上游模型">' +
    ICONS.alert +
    "<span>无可用模型</span></span>"
  );
}

function updateAliasTargetsField(field, value) {
  if (!field) return;
  const targets = normalizeAliasTargets(value);
  const unroutable = targets.length > 0 && !aliasTargetsHaveRoutableTarget(targets);
  const title =
    aliasTargetsTitle(targets) +
    (unroutable ? "\n提醒：当前映射没有已启用的上游模型，不会参与路由" : "");
  field.dataset.targets = JSON.stringify(targets);
  field.title = title;
  field.setAttribute("aria-label", title);
  field.classList.toggle("is-unroutable", unroutable);
  field.innerHTML =
    aliasTargetsDisplay(targets) +
    aliasTargetsWeightWarningHtml(targets) +
    '<span class="field-edit-icon">' +
    ICONS.layers +
    "</span>";
}

function aliasTargetsFieldHtml(value) {
  const targets = normalizeAliasTargets(value);
  const unroutable = targets.length > 0 && !aliasTargetsHaveRoutableTarget(targets);
  const title =
    aliasTargetsTitle(targets) +
    (unroutable ? "\n提醒：当前映射没有已启用的上游模型，不会参与路由" : "");
  return (
    '<div class="field-display alias-targets-field' +
    (unroutable ? " is-unroutable" : "") +
    '" data-field="targets" data-targets="' +
    escAttr(JSON.stringify(targets)) +
    '" title="' +
    escAttr(title) +
    '" aria-label="' +
    escAttr(title) +
    '" onclick="openAliasTargetsEditor(this)">' +
    aliasTargetsDisplay(targets) +
    aliasTargetsWeightWarningHtml(targets) +
    '<span class="field-edit-icon">' +
    ICONS.layers +
    "</span></div>"
  );
}

function aliasTargetCandidates(selected) {
  const candidates = [];
  const seen = new Set();
  orderedUpstreamNames().forEach((upstream) => {
    upstreamSearchModels(upstream)
      .sort((a, b) => a.localeCompare(b))
      .forEach((model) => {
        const identity = aliasTargetIdentity(upstream, model);
        if (seen.has(identity)) return;
        seen.add(identity);
        candidates.push({ upstream: upstream, target_model: model });
      });
  });
  normalizeAliasTargets(selected).forEach((target) => {
    const identity = aliasTargetIdentity(target.upstream, target.target_model);
    if (seen.has(identity)) return;
    seen.add(identity);
    candidates.push({
      upstream: target.upstream,
      target_model: target.target_model,
      missing: true,
    });
  });
  return candidates;
}

function buildAliasTargetOptionsHtml(candidates, selected) {
  if (!candidates.length) {
    return '<div class="alias-target-options-empty">暂无可用模型，请先在多上游配置中同步或添加模型</div>';
  }
  const selectedSet = new Set(
    normalizeAliasTargets(selected).map((target) =>
      aliasTargetIdentity(target.upstream, target.target_model),
    ),
  );
  return candidates
    .map((candidate) => {
      const identity = aliasTargetIdentity(
        candidate.upstream,
        candidate.target_model,
      );
      const isSelected = selectedSet.has(identity);
      return (
        '<button type="button" class="alias-target-option' +
        (isSelected ? " is-selected" : "") +
        '" data-upstream="' +
        escAttr(candidate.upstream) +
        '" data-model="' +
        escAttr(candidate.target_model) +
        '" role="option" aria-selected="' +
        (isSelected ? "true" : "false") +
        '" onclick="addAliasTargetOption(this)"' +
        (isSelected ? " disabled" : "") +
        ">" +
        '<span class="alias-target-route-combo alias-target-option-route"><span class="alias-target-route-upstream" title="' +
        escAttr(candidate.upstream) +
        '">' +
        esc(candidate.upstream) +
        '</span><span class="alias-target-route-arrow" aria-hidden="true">' +
        ICONS.arrowRight +
        '</span><span class="alias-target-route-model" title="' +
        escAttr(candidate.target_model) +
        '">' +
        esc(candidate.target_model) +
        "</span></span>" +
        (candidate.missing
          ? '<span class="model-missing-badge">未在上游返回</span>'
          : "") +
        '<span class="alias-target-option-action">' +
        (isSelected ? ICONS.check + "<span>已添加</span>" : ICONS.plus + "<span>添加</span>") +
        "</span></button>"
      );
    })
    .join("");
}

function sortAliasTargetsForDisplay(targets) {
  return normalizeAliasTargets(targets)
    .map((target, index) => ({ target: target, index: index }))
    .sort(
      (a, b) =>
        Number(b.target.enabled) - Number(a.target.enabled) ||
        b.target.weight - a.target.weight ||
        a.index - b.index,
    )
    .map((item) => item.target);
}

function buildAliasTargetSelectedRows(targets) {
  const normalizedTargets = normalizeAliasTargets(targets);
  if (!normalizedTargets.length) {
    return (
      '<div class="alias-target-selected-empty">' +
      ICONS.layers +
      "<span>尚未添加上游模型</span></div>"
    );
  }
  return normalizedTargets
    .map(
      (target) =>
        '<div class="alias-target-selected-row" data-upstream="' +
        escAttr(target.upstream) +
        '" data-model="' +
        escAttr(target.target_model) +
        '">' +
        '<span class="alias-target-route-combo alias-target-selected-route">' +
        '<span class="alias-target-route-upstream" title="' +
        escAttr(target.upstream) +
        '">' +
        esc(target.upstream) +
        '</span><span class="alias-target-route-arrow" aria-hidden="true">' +
        ICONS.arrowRight +
        '</span><span class="alias-target-route-model" title="' +
        escAttr(target.target_model) +
        '">' +
        esc(target.target_model) +
        "</span></span>" +
        '<label class="alias-target-enabled" title="启用后参与路由，禁用时保留当前权重"><span>启用</span><span class="switch"><input type="checkbox" data-field="enabled" aria-label="启用 ' +
        escAttr(target.upstream + " / " + target.target_model) +
        '" onchange="toggleAliasTargetEnabled(this)"' +
        (target.enabled ? " checked" : "") +
        '><span class="switch-slider"></span></span></label>' +
        '<label class="alias-target-weight"><span>权重</span><input type="number" min="1" max="1000000" step="1" value="' +
        target.weight +
        '" oninput="updateAliasTargetEditorState(this)" onkeydown="handleAliasTargetWeightKeydown(event)" onblur="commitAliasTargetWeightInput(this)"></label>' +
        '<span class="alias-target-percent">0%</span>' +
        '<button type="button" class="btn-icon btn-icon-danger" onclick="removeAliasTarget(this)" title="删除此上游模型" aria-label="删除 ' +
        escAttr(target.upstream + " / " + target.target_model) +
        '">' +
        ICONS.trash +
        "</button></div>",
    )
    .join("");
}

function readAliasTargetsFromPopover(pop) {
  return normalizeAliasTargets(
    Array.from(pop?.querySelectorAll(".alias-target-selected-row") || []).map(
      (row) => ({
        upstream: row.dataset.upstream || "",
        target_model: row.dataset.model || "",
        weight: row.querySelector('input[type="number"]')?.value || 1,
        enabled: row.querySelector('[data-field="enabled"]')?.checked !== false,
      }),
    ),
  );
}

function buildAliasTargetsPopoverHtml(targets) {
  return (
    '<div class="popover-header"><span class="popover-title">' +
    ICONS.layers +
    '<span>上游模型</span><span class="model-selection-summary">已选 ' +
    targets.length +
    '</span></span><button type="button" class="btn-icon" onclick="closePopover()" title="完成" aria-label="完成">' +
    ICONS.close +
    "</button></div>" +
    '<div class="alias-target-picker">' +
    '<label class="model-search alias-target-search">' +
    ICONS.search +
    '<input type="search" role="combobox" aria-expanded="false" aria-controls="aliasTargetOptions" aria-autocomplete="list" autocomplete="off" placeholder="搜索上游名称或模型 ID" onfocus="openAliasTargetOptions(this)" onclick="openAliasTargetOptions(this)" oninput="filterAliasTargetOptions(this)" onkeydown="handleAliasTargetSearchKeydown(event)"></label>' +
    '<div class="alias-target-options" id="aliasTargetOptions" role="listbox" hidden></div></div>' +
    '<div class="alias-target-selected-head"><span>已添加模型</span><span>加载时按启用状态、权重从高到低排序</span></div>' +
    '<div class="alias-target-selected-list">' +
    buildAliasTargetSelectedRows(sortAliasTargetsForDisplay(targets)) +
    "</div>"
  );
}

function openAliasTargetsEditor(el) {
  collectUpstreams();
  const targets = parseAliasTargets(el.dataset.targets || "[]");
  const pop = openPopover(
    el,
    buildAliasTargetsPopoverHtml(targets),
    function () {
      const nextTargets = readAliasTargetsFromPopover(pop);
      updateAliasTargetsField(el, nextTargets);
      collectAliases();
    },
  );
  pop.classList.add("alias-target-popover");
  pop._aliasTargetCandidates = aliasTargetCandidates(targets);
  const searchInput = pop.querySelector(".alias-target-search input");
  if (searchInput) searchInput.placeholder = "搜索未添加的上游名称或模型 ID";
  pop.addEventListener("mousedown", (event) => {
    if (!event.target.closest(".alias-target-picker")) {
      closeAliasTargetOptions(pop);
    }
  });
  updateAliasTargetEditorState(pop);
}

function normalizeAliasTargetSearchText(value) {
  return String(value || "")
    .normalize("NFKC")
    .toLocaleLowerCase()
    .trim();
}

function aliasTargetMatchesSearch(upstream, model, query) {
  return Number.isFinite(aliasTargetSearchScore(upstream, model, query));
}

function aliasTargetSearchScore(upstream, model, query) {
  const normalizedQuery = normalizeAliasTargetSearchText(query);
  if (!normalizedQuery) return 100;
  const normalizedUpstream = normalizeAliasTargetSearchText(upstream);
  const normalizedModel = normalizeAliasTargetSearchText(model);
  if (normalizedModel === normalizedQuery) return 0;
  if (normalizedModel.startsWith(normalizedQuery)) return 1;
  if (normalizedModel.includes(normalizedQuery)) return 2;
  if (normalizedUpstream === normalizedQuery) return 3;
  if (normalizedUpstream.startsWith(normalizedQuery)) return 4;
  if (normalizedUpstream.includes(normalizedQuery)) return 5;
  return Number.POSITIVE_INFINITY;
}

function renderAliasTargetOptions(pop, query) {
  const options = pop?.querySelector(".alias-target-options");
  if (!options) return;
  const candidates = Array.isArray(pop._aliasTargetCandidates)
    ? pop._aliasTargetCandidates
    : [];
  const selected = readAliasTargetsFromPopover(pop);
  const selectedSet = new Set(
    selected.map((target) =>
      aliasTargetIdentity(target.upstream, target.target_model),
    ),
  );
  const availableCandidates = candidates.filter(
    (candidate) =>
      !selectedSet.has(
        aliasTargetIdentity(candidate.upstream, candidate.target_model),
      ),
  );
  const ranked = availableCandidates
    .map((candidate, index) => ({
      candidate: candidate,
      index: index,
      score: aliasTargetSearchScore(
        candidate.upstream,
        candidate.target_model,
        query,
      ),
    }))
    .filter((item) => Number.isFinite(item.score))
    .sort((a, b) => a.score - b.score || a.index - b.index);
  const visible = ranked.map((item) => item.candidate);
  const resultHtml = visible.length
    ? buildAliasTargetOptionsHtml(visible, selected)
    : availableCandidates.length
      ? '<div class="alias-target-filter-empty">没有匹配的上游模型</div>'
      : candidates.length
        ? '<div class="alias-target-options-empty">所有可用上游模型均已添加</div>'
        : '<div class="alias-target-options-empty">暂无可用模型，请先在多上游配置中同步或添加模型</div>';
  options.innerHTML =
    '<div class="alias-target-options-list">' +
    resultHtml +
    '</div><div class="alias-target-results-summary" role="status" aria-live="polite">搜索结果 <strong>' +
    ranked.length +
    "</strong> 条，共 <strong>" +
    availableCandidates.length +
    "</strong> 条</div>";
  options.querySelector(".alias-target-options-list")?.scrollTo({ top: 0 });
  pop._aliasTargetKeysActiveOpt = null;
}

function openAliasTargetOptions(input) {
  const pop = input.closest(".alias-target-popover");
  const picker = input.closest(".alias-target-picker");
  const options = pop?.querySelector(".alias-target-options");
  if (!picker || !options) return;
  picker.classList.add("is-open");
  options.hidden = false;
  input.setAttribute("aria-expanded", "true");
  renderAliasTargetOptions(pop, input.value);
}

function closeAliasTargetOptions(source) {
  const pop = source?.classList?.contains("alias-target-popover")
    ? source
    : source?.closest?.(".alias-target-popover");
  if (!pop) return;
  const picker = pop.querySelector(".alias-target-picker");
  const input = pop.querySelector(".alias-target-search input");
  const options = pop.querySelector(".alias-target-options");
  picker?.classList.remove("is-open");
  if (options) options.hidden = true;
  input?.setAttribute("aria-expanded", "false");
  pop._aliasTargetKeysActiveOpt = null;
}

function filterAliasTargetOptions(input) {
  const pop = input.closest(".alias-target-popover");
  if (!pop) return;
  const options = pop.querySelector(".alias-target-options");
  if (options?.hidden) openAliasTargetOptions(input);
  renderAliasTargetOptions(pop, input.value);
}

function aliasTargetVisibleOptions(pop) {
  if (!pop) return [];
  return Array.from(
    pop.querySelectorAll(".alias-target-options-list .alias-target-option:not([disabled])"),
  ).filter((opt) => !opt.hidden);
}

function aliasTargetHighlightIndex(pop) {
  const opts = aliasTargetVisibleOptions(pop);
  const cur = pop._aliasTargetKeysActiveOpt;
  if (cur && opts.includes(cur)) return opts.indexOf(cur);
  return -1;
}

function aliasTargetHighlight(pop, index) {
  const opts = aliasTargetVisibleOptions(pop);
  opts.forEach((opt) => opt.classList.remove("is-key-active"));
  if (!opts.length) return;
  const clamped = ((index % opts.length) + opts.length) % opts.length;
  const opt = opts[clamped];
  opt.classList.add("is-key-active");
  pop._aliasTargetKeysActiveOpt = opt;
  const list = pop.querySelector(".alias-target-options-list");
  if (list) {
    const lRect = list.getBoundingClientRect();
    const oRect = opt.getBoundingClientRect();
    if (oRect.top < lRect.top + 1) {
      list.scrollTop -= lRect.top - oRect.top + 1;
    } else if (oRect.bottom > lRect.bottom - 1) {
      list.scrollTop += oRect.bottom - lRect.bottom + 1;
    }
  }
}

function handleAliasTargetSearchKeydown(event) {
  const input = event.currentTarget;
  const pop = input.closest(".alias-target-popover");
  if (!pop) return;
  if (event.key === "Escape") {
    event.preventDefault();
    event.stopPropagation();
    closeAliasTargetOptions(pop);
    return;
  }
  const opts = aliasTargetVisibleOptions(pop);
  if (!opts.length) return;
  let index = aliasTargetHighlightIndex(pop);
  if (event.key === "ArrowDown") {
    event.preventDefault();
    openAliasTargetOptions(input);
    aliasTargetHighlight(pop, index === -1 ? 0 : index + 1);
  } else if (event.key === "ArrowUp") {
    event.preventDefault();
    openAliasTargetOptions(input);
    aliasTargetHighlight(pop, index === -1 ? opts.length - 1 : index - 1);
  } else if (event.key === "Enter") {
    event.preventDefault();
    event.stopPropagation();
    if (index === -1) {
      // 没有键盘高亮时回退到点击推荐项功能
      return;
    }
    const active = opts[index];
    if (active && !active.disabled) addAliasTargetOption(active);
  }
}

function renderAliasTargetSelectedList(pop, targets) {
  const list = pop?.querySelector(".alias-target-selected-list");
  if (!list) return;
  list.innerHTML = buildAliasTargetSelectedRows(targets);
  updateAliasTargetEditorState(pop);
  const options = pop.querySelector(".alias-target-options");
  const input = pop.querySelector(".alias-target-search input");
  if (options && !options.hidden) renderAliasTargetOptions(pop, input?.value || "");
}

function addAliasTargetOption(button) {
  const pop = button.closest(".alias-target-popover");
  if (!pop || button.disabled) return;
  const targets = readAliasTargetsFromPopover(pop);
  targets.push({
    upstream: button.dataset.upstream || "",
    target_model: button.dataset.model || "",
    weight: 1,
  });
  renderAliasTargetSelectedList(pop, targets);
  const input = pop.querySelector(".alias-target-search input");
  if (input) {
    filterAliasTargetOptions(input);
    requestAnimationFrame(() => input.focus());
  }
}

function removeAliasTarget(button) {
  const pop = button.closest(".alias-target-popover");
  const row = button.closest(".alias-target-selected-row");
  if (!pop || !row) return;
  row.remove();
  renderAliasTargetSelectedList(pop, readAliasTargetsFromPopover(pop));
}

function sanitizeAliasTargetWeightInput(input) {
  if (!input || input.value === "") return;
  const weight = Number(input.value);
  input.value = String(
    Number.isFinite(weight)
      ? Math.min(Math.max(Math.trunc(weight), 1), 1000000)
      : 1,
  );
}

function handleAliasTargetWeightKeydown(event) {
  if (event.key !== "Enter") return;
  event.preventDefault();
  event.stopPropagation();
  commitAliasTargetWeightInput(event.currentTarget || event.target);
}

function commitAliasTargetWeightInput(input) {
  if (!input) return;
  if (input.value === "") input.value = "1";
  sanitizeAliasTargetWeightInput(input);
  updateAliasTargetEditorState(input);
}

function aliasTargetWeightFromRow(row) {
  const parsedWeight = Number.parseInt(
    row?.querySelector('input[type="number"]')?.value,
    10,
  );
  return Number.isFinite(parsedWeight)
    ? Math.min(Math.max(parsedWeight, 1), 1000000)
    : 1;
}

function aliasTargetEnabledFromRow(row) {
  return row?.querySelector('[data-field="enabled"]')?.checked !== false;
}

function toggleAliasTargetEnabled(input) {
  const row = input?.closest?.(".alias-target-selected-row");
  const weightInput = row?.querySelector('input[type="number"]');
  if (input?.checked && weightInput && Number(weightInput.value) < 1) {
    weightInput.value = "1";
  }
  updateAliasTargetEditorState(input);
}

function updateAliasTargetEditorState(source) {
  const pop = source?.classList?.contains("alias-target-popover")
    ? source
    : source?.closest?.(".alias-target-popover");
  if (!pop) return;
  const targets = readAliasTargetsFromPopover(pop);
  const selected = new Set(
    targets.map((target) =>
      aliasTargetIdentity(target.upstream, target.target_model),
    ),
  );
  const enabledTargets = targets.filter((target) => target.enabled);
  const totalWeight = enabledTargets.reduce((sum, target) => sum + target.weight, 0);
  pop.querySelectorAll(".alias-target-selected-row").forEach((row) => {
    const weight = aliasTargetWeightFromRow(row);
    const enabled = aliasTargetEnabledFromRow(row);
    const percent = enabled && totalWeight ? (weight / totalWeight) * 100 : 0;
    const label = row.querySelector(".alias-target-percent");
    row.classList.toggle("is-disabled", !enabled);
    if (label)
      label.textContent =
        !enabled
          ? "已禁用"
          : percent.toFixed(percent >= 10 ? 0 : 1) + "%";
  });
  pop.querySelectorAll(".alias-target-option").forEach((option) => {
    const isSelected = selected.has(
      aliasTargetIdentity(option.dataset.upstream, option.dataset.model),
    );
    option.disabled = isSelected;
    option.classList.toggle("is-selected", isSelected);
    option.setAttribute("aria-selected", isSelected ? "true" : "false");
    const action = option.querySelector(".alias-target-option-action");
    if (action)
      action.innerHTML = isSelected
        ? ICONS.check + "<span>已添加</span>"
        : ICONS.plus + "<span>添加</span>";
  });
  const summary = pop.querySelector(".model-selection-summary");
  if (summary)
    summary.textContent =
      "已选 " +
      targets.length +
      (targets.length
        ? " · 已启用 " + enabledTargets.length + " · 路由权重 " + totalWeight
        : "");
}

function renderAliasTable() {
  const tb = document.querySelector("#aliasTable tbody");
  const ks = Object.keys(aliasData);
  if (!ks.length) {
    tb.innerHTML = emptyRowHtml(
      5,
      ICONS.layers,
      "暂无别名配置",
      "可用别名把请求模型名映射到实际上游模型",
    );
    return;
  }
  tb.innerHTML = ks
    .map((k) => {
      const entry = aliasData[k] || {
        targets: [],
        with_reasoning: false,
        reasoning_effort_map: {},
      };
      const selected = selectedAliasKey === k;
      return (
        '<tr data-alias-key="' +
        escAttr(k) +
        '"' +
        (selected ? ' class="alias-selected"' : "") +
        ">" +
        '<td><input class="alias-effort-select" type="checkbox" aria-label="编辑 ' +
        escAttr(k) +
        ' 的推理力度映射"' +
        (selected ? " checked" : "") +
        ' onchange="selectAliasEffortMap(this)"></td>' +
        '<td><input value="' +
        escAttr(k) +
        '" data-field="key"></td>' +
        "<td>" +
        aliasTargetsFieldHtml(entry.targets || []) +
        "</td>" +
        '<td class="col-reasoning">' +
        reasoningSwitchHtml(!!entry.with_reasoning) +
        "</td>" +
        '<td class="action-cell">' +
        aliasTestButtonHtml(k) +
        '<button class="btn-icon btn-icon-danger" onclick="delAlias(this)" title="删除">' +
        ICONS.trash +
        "</button></td>" +
        "</tr>"
      );
    })
    .join("");
}

function aliasTestButtonHtml(aliasName) {
  const label = aliasName ? "测试 " + aliasName + " 映射的模型" : "测试该映射的模型";
  return (
    '<button class="btn-icon model-test-btn" onclick="testAliasModel(this)" title="测试模型" aria-label="' +
    escAttr(label) +
    '">' +
    ICONS.gauge +
    "</button>"
  );
}

function addAliasRow() {
  collectUpstreams();
  const tb = document.querySelector("#aliasTable tbody");
  if (tb.querySelector(".empty-hint, .empty-state")) tb.innerHTML = "";
  tb.insertAdjacentHTML(
    "beforeend",
    "<tr>" +
      '<td><input class="alias-effort-select" type="checkbox" aria-label="编辑该模型的推理力度映射" onchange="selectAliasEffortMap(this)"></td>' +
      '<td><input value="" placeholder="例如: gpt-5.5" data-field="key"></td>' +
      "<td>" +
      aliasTargetsFieldHtml([]) +
      "</td>" +
      '<td class="col-reasoning">' +
      reasoningSwitchHtml(false) +
      "</td>" +
      '<td class="action-cell">' +
      aliasTestButtonHtml("") +
      '<button class="btn-icon btn-icon-danger" onclick="delAlias(this)" title="删除">' +
      ICONS.trash +
      "</button></td>" +
      "</tr>",
  );
}

async function delAlias(btn) {
  const row = btn.closest("tr");
  const ki = row.querySelector('[data-field="key"]');
  const rowKey = row.dataset.aliasKey || (ki ? ki.value.trim() : "");
  if (!(await confirmConfigDelete(btn, "模型别名", rowKey))) return;
  const deletingSelected =
    selectedAliasKey === rowKey || (ki && selectedAliasKey === ki.value.trim());
  if (rowKey) delete aliasData[rowKey];
  if (ki && ki.value.trim()) delete aliasData[ki.value.trim()];
  row.remove();
  collectAliases();
  if (deletingSelected) {
    selectedAliasKey = "";
    showSelectedEffortMap();
  }
  if (!Object.keys(aliasData).length)
    document.querySelector("#aliasTable tbody").innerHTML = emptyRowHtml(
      5,
      ICONS.layers,
      "暂无别名配置",
      "可用别名把请求模型名映射到实际上游模型",
    );
}

function collectAliases() {
  const r = {};
  document.querySelectorAll("#aliasTable tbody tr").forEach((tr) => {
    const k = tr.querySelector('[data-field="key"]'),
      targetsField = tr.querySelector('[data-field="targets"]'),
      w = tr.querySelector('[data-field="with_reasoning"]');
    if (k && k.value.trim()) {
      const aliasKey = k.value.trim();
      const previousKey = tr.dataset.aliasKey || aliasKey;
      const previous = aliasData[previousKey] || aliasData[aliasKey] || {};
      const reasoningEffortMap = { ...(previous.reasoning_effort_map || {}) };
      const targets = parseAliasTargets(targetsField?.dataset.targets || "[]");
      const withReasoning = w ? w.checked : false;
      if (
        targets.length ||
        withReasoning ||
        Object.keys(reasoningEffortMap).length
      ) {
        r[aliasKey] = {
          targets: targets,
          with_reasoning: withReasoning,
          reasoning_effort_map: reasoningEffortMap,
        };
        if (selectedAliasKey === previousKey) selectedAliasKey = aliasKey;
        tr.dataset.aliasKey = aliasKey;
      }
    }
  });
  aliasData = r;
  return r;
}

function validateAliasRows() {
  const seen = new Set();
  for (const row of document.querySelectorAll("#aliasTable tbody tr")) {
    const keyInput = row.querySelector('[data-field="key"]');
    const targetsField = row.querySelector('[data-field="targets"]');
    const key = keyInput?.value.trim() || "";
    const targets = parseAliasTargets(targetsField?.dataset.targets || "[]");
    if (!key && !targets.length) continue;
    if (!key) {
      return { message: "请先填写映射模型名", element: keyInput };
    }
    if (!targets.length) {
      return { message: "请为 " + key + " 添加至少一个上游模型", element: targetsField };
    }
    if (seen.has(key)) {
      return { message: "映射模型名不能重复：" + key, element: keyInput };
    }
    seen.add(key);
    const missing = targets.find((target) => !upstreamData[target.upstream]);
    if (missing) {
      return {
        message: "上游不存在：" + missing.upstream,
        element: targetsField,
      };
    }
  }
  return null;
}

function selectAliasEffortMap(checkbox) {
  collectEfforts();
  collectAliases();
  const row = checkbox.closest("tr");
  const keyInput = row ? row.querySelector('[data-field="key"]') : null;
  const key = keyInput ? keyInput.value.trim() : "";
  if (checkbox.checked && !key) {
    checkbox.checked = false;
    showToast("请先填写映射模型名", "error");
    return;
  }
  selectedAliasKey = checkbox.checked ? key : "";
  document
    .querySelectorAll("#aliasTable .alias-effort-select")
    .forEach((item) => {
      item.checked = item === checkbox && checkbox.checked;
      const itemRow = item.closest("tr");
      if (itemRow) itemRow.classList.toggle("alias-selected", item.checked);
    });
  showSelectedEffortMap();
}

function showSelectedEffortMap() {
  if (selectedAliasKey && aliasData[selectedAliasKey]) {
    effortData = {
      ...(aliasData[selectedAliasKey].reasoning_effort_map || {}),
    };
  } else {
    selectedAliasKey = "";
    effortData = { ...globalEffortData };
  }
  const title = document.getElementById("effortMapTitle");
  if (title)
    title.textContent = selectedAliasKey
      ? "推理力度映射（" + selectedAliasKey + "）"
      : "推理力度映射（全局）";
  renderEffortTable();
}

/* ===== Effort 映射表 ===== */
function renderEffortTable() {
  const tb = document.querySelector("#effortTable tbody");
  const ks = Object.keys(effortData);
  if (!ks.length) {
    tb.innerHTML = emptyRowHtml(
      3,
      ICONS.edit,
      "暂无映射配置",
      selectedAliasKey
        ? "为当前模型添加推理力度映射"
        : "可配置全局或按模型的推理力度映射",
    );
    return;
  }
  tb.innerHTML = ks
    .map(
      (k) =>
        "<tr>" +
        '<td><input value="' +
        escAttr(k) +
        '" data-field="key"></td>' +
        '<td><input value="' +
        escAttr(effortData[k]) +
        '" data-field="val"></td>' +
        '<td class="action-cell"><button class="btn-icon btn-icon-danger" onclick="delEffort(this)" title="删除">' +
        ICONS.trash +
        "</button></td>" +
        "</tr>",
    )
    .join("");
}

function addEffortRow() {
  const tb = document.querySelector("#effortTable tbody");
  if (tb.querySelector(".empty-hint, .empty-state")) tb.innerHTML = "";
  tb.insertAdjacentHTML(
    "beforeend",
    "<tr>" +
      '<td><input value="" placeholder="例如: low" data-field="key"></td>' +
      '<td><input value="" placeholder="例如: high" data-field="val"></td>' +
      '<td class="action-cell"><button class="btn-icon btn-icon-danger" onclick="delEffort(this)" title="删除">' +
      ICONS.trash +
      "</button></td>" +
      "</tr>",
  );
}

async function delEffort(btn) {
  const row = btn.closest("tr");
  const ki = row.querySelector('[data-field="key"]');
  if (
    !(await confirmConfigDelete(
      btn,
      "推理力度映射",
      ki?.value?.trim() || "",
    ))
  )
    return;
  if (ki && ki.value && effortData[ki.value]) delete effortData[ki.value];
  row.remove();
  if (!Object.keys(effortData).length)
    document.querySelector("#effortTable tbody").innerHTML = emptyRowHtml(
      3,
      ICONS.edit,
      "暂无映射配置",
      selectedAliasKey
        ? "为当前模型添加推理力度映射"
        : "可配置全局或按模型的推理力度映射",
    );
}

function collectEfforts() {
  const r = {};
  document.querySelectorAll("#effortTable tbody tr").forEach((tr) => {
    const k = tr.querySelector('[data-field="key"]'),
      v = tr.querySelector('[data-field="val"]');
    if (k && k.value.trim()) r[k.value.trim()] = v ? v.value.trim() : "";
  });
  effortData = r;
  if (selectedAliasKey && aliasData[selectedAliasKey]) {
    aliasData[selectedAliasKey].reasoning_effort_map = { ...r };
  } else {
    globalEffortData = { ...r };
  }
  return r;
}

/* ===== SOCKS5 表格 ===== */
function renderSocks5Table() {
  const tb = document.querySelector("#socks5Table tbody");
  if (!socks5Data.length) {
    tb.innerHTML = emptyRowHtml(
      5,
      ICONS.server,
      "暂无代理配置",
      "可选配置 SOCKS5 出口，然后在上游配置中选择",
    );
    refreshUpstreamProxySelectors();
    return;
  }
  tb.innerHTML = socks5Data
    .map(
      (p, i) =>
        "<tr>" +
        '<td><input value="' +
        escAttr(p.name || "") +
        '" data-field="name"></td>' +
        '<td><input value="' +
        escAttr(p.addr) +
        '" data-field="addr" placeholder="例如: 127.0.0.1:1080"></td>' +
        '<td><input value="' +
        escAttr(p.username || "") +
        '" data-field="username"></td>' +
        '<td><input value="' +
        escAttr(p.password || "") +
        '" data-field="password" type="password"></td>' +
        '<td class="action-cell"><button class="btn-icon btn-icon-danger" onclick="delSocks5(this, ' +
        i +
        ')" title="删除">' +
        ICONS.trash +
        "</button></td>" +
        "</tr>",
    )
    .join("");
  refreshUpstreamProxySelectors();
}

function addSocks5Row() {
  const tb = document.querySelector("#socks5Table tbody");
  if (tb.querySelector(".empty-hint, .empty-state")) tb.innerHTML = "";
  socks5Data.push({ addr: "", name: "" });
  renderSocks5Table();
}

async function delSocks5(btn, i) {
  const proxy = socks5Data[i] || {};
  const label = proxy.name || proxy.addr || "";
  if (!(await confirmConfigDelete(btn, "SOCKS5 代理", label))) return;
  socks5Data.splice(i, 1);
  renderSocks5Table();
}

function collectSocks5() {
  const r = [];
  document.querySelectorAll("#socks5Table tbody tr").forEach((tr) => {
    const a = tr.querySelector('[data-field="addr"]');
    if (a && a.value.trim()) {
      r.push({
        addr: a.value.trim(),
        name:
          (tr.querySelector('[data-field="name"]') || {}).value?.trim() || "",
        username:
          (tr.querySelector('[data-field="username"]') || {}).value?.trim() ||
          "",
        password:
          (tr.querySelector('[data-field="password"]') || {}).value?.trim() ||
          "",
      });
    }
  });
  socks5Data = r;
  return r;
}

function refreshUpstreamProxySelectors() {
  document
    .querySelectorAll("#upstreamTable tbody tr[data-upstream-row]")
    .forEach((row) => {
      const cell = row.querySelector('[data-field="proxy"]')?.closest("td");
      const select = row.querySelector('[data-field="proxy"]');
      if (!cell || !select) return;
      cell.innerHTML = proxySelectHtml(select.value);
    });
}

/* ===== 保存配置 ===== */
function renderWebSearchConfig() {
  const cfg = webSearchData || {};
  document.getElementById("webSearchEnabled").checked = !!cfg.enabled;
  document.getElementById("webSearchProvider").value =
    cfg.provider || "searxng";
  document.getElementById("webSearchSearxMode").value =
    cfg.searxng_mode || (cfg.base_url ? "custom" : "auto");
  document.getElementById("webSearchFallbackProvider").value =
    cfg.fallback_provider || "duckduckgo";
  document.getElementById("webSearchBaseURL").value = cfg.base_url || "";
  document.getElementById("webSearchAPIKey").value = cfg.api_key || "";
  document.getElementById("webSearchMaxResults").value = cfg.max_results || 6;
  document.getElementById("webSearchTimeout").value = cfg.timeout_seconds || 10;
  document.getElementById("webSearchMaxRounds").value =
    cfg.max_tool_rounds || 2;
  document.getElementById("webSearchMaxBytes").value =
    cfg.max_result_bytes || 65536;
  syncWebSearchForm();
}

function fitWebSearchBaseURLWidth() {
  const base = document.getElementById("webSearchBaseURL");
  if (!base) return;
  const sample =
    (base.value || base.placeholder || "").trim() || "https://example.com";
  const ch = Math.min(Math.max(sample.length + 2, 18), 52);
  base.style.width = ch + "ch";
  base.style.maxWidth = "100%";
}

function syncWebSearchForm() {
  const enabled = document.getElementById("webSearchEnabled").checked;
  const provider = document.getElementById("webSearchProvider").value;
  const base = document.getElementById("webSearchBaseURL");
  const key = document.getElementById("webSearchAPIKey");
  const searxMode = document.getElementById("webSearchSearxMode").value;
  const isSearx = provider === "searxng";
  const isDuckDuckGo = provider === "duckduckgo";
  document.getElementById("webSearchSearxModeGroup").style.display = isSearx
    ? ""
    : "none";
  document.getElementById("webSearchFallbackProviderGroup").style.display =
    isSearx && searxMode === "auto" ? "" : "none";
  document.getElementById("webSearchSearxInstanceGroup").style.display =
    isSearx && searxMode === "selected" ? "" : "none";
  document.getElementById("webSearchBaseURLGroup").style.display =
    !isSearx || searxMode !== "selected" ? "" : "none";
  document.getElementById("webSearchBaseURLLabel").textContent =
    isSearx && searxMode === "auto"
      ? "备用实例 URL（可选）"
      : isDuckDuckGo
        ? "DuckDuckGo Lite URL（可选）"
        : "Base URL";
  const hint = document.getElementById("webSearchBaseURLHint");
  // DuckDuckGo 不显示下方说明；默认 URL 已体现在 placeholder
  hint.textContent =
    isSearx && searxMode === "auto"
      ? "官方目录不可用时尝试此固定实例；留空则完全自动。"
      : "";
  hint.style.display = hint.textContent ? "" : "none";
  base.placeholder =
    provider === "tavily"
      ? "https://api.tavily.com/search"
      : isDuckDuckGo
        ? "https://lite.duckduckgo.com/lite/"
        : searxMode === "auto"
          ? "可留空"
          : "https://search.example.com";
  fitWebSearchBaseURLWidth();
  document.getElementById("webSearchAPIKeyGroup").style.display =
    provider === "tavily" ? "" : "none";
  key.disabled = !enabled || provider !== "tavily";
  document
    .querySelectorAll(
      "#webSearchProvider,#webSearchSearxMode,#webSearchFallbackProvider,#webSearchSearxInstance,#webSearchBaseURL,#webSearchMaxResults,#webSearchTimeout,#webSearchMaxRounds,#webSearchMaxBytes",
    )
    .forEach((el) => {
      el.disabled = !enabled;
    });
  if (
    enabled &&
    isSearx &&
    searxMode === "selected" &&
    searxngInstances.length === 0
  )
    loadSearXNGInstances(false);
}

function renderSearXNGInstances(selectedURL) {
  const select = document.getElementById("webSearchSearxInstance");
  const selected =
    selectedURL || document.getElementById("webSearchBaseURL").value.trim();
  const options = searxngInstances
    .map((instance) => {
      const latency = Math.round((instance.search_median_seconds || 0) * 1000);
      const uptime = Number(instance.uptime_month || 0).toFixed(1);
      const grade = instance.tls_grade || instance.http_grade || "-";
      const eligibility = instance.auto_eligible ? "自动候选" : "仅手动";
      return (
        '<option value="' +
        escAttr(instance.url) +
        '"' +
        (instance.url === selected ? " selected" : "") +
        ">" +
        esc(
          instance.url +
            " · " +
            latency +
            "ms · 月可用 " +
            uptime +
            "% · " +
            grade +
            " · " +
            eligibility,
        ) +
        "</option>"
      );
    })
    .join("");
  select.innerHTML =
    '<option value="">-- 选择可用公共实例 --</option>' + options;
  if (
    selected &&
    !searxngInstances.some((instance) => instance.url === selected)
  ) {
    select.insertAdjacentHTML(
      "beforeend",
      '<option value="' +
        escAttr(selected) +
        '" selected>' +
        esc(selected + "（已保存，当前目录未列出）") +
        "</option>",
    );
  }
}

async function loadSearXNGInstances(force) {
  const status = document.getElementById("webSearchSearxStatus");
  status.textContent = "正在读取 searx.space 健康实例…";
  try {
    const suffix = force ? "?refresh=1" : "";
    const data = await apiJSON("/api/searxng/instances" + suffix);
    searxngInstances = data.instances || [];
    renderSearXNGInstances();
    status.textContent =
      "已加载 " +
      searxngInstances.length +
      " 个可用实例 · 更新时间 " +
      (data.fetched_at || "-");
  } catch (e) {
    status.textContent = "实例列表加载失败：" + e.message;
    if (force) showToast("SearXNG 实例列表刷新失败: " + e.message, "error");
  }
}

function selectSearXNGInstance() {
  const selected = document.getElementById("webSearchSearxInstance").value;
  if (selected) {
    document.getElementById("webSearchBaseURL").value = selected;
    fitWebSearchBaseURLWidth();
  }
}

function collectWebSearchConfig() {
  const number = (id, fallback) =>
    parseInt(document.getElementById(id).value, 10) || fallback;
  const provider = document.getElementById("webSearchProvider").value;
  const searxMode = document.getElementById("webSearchSearxMode").value;
  webSearchData = {
    enabled: document.getElementById("webSearchEnabled").checked,
    provider: provider,
    fallback_provider:
      provider === "searxng" && searxMode === "auto"
        ? document.getElementById("webSearchFallbackProvider").value
        : "none",
    base_url: document.getElementById("webSearchBaseURL").value.trim(),
    api_key: document.getElementById("webSearchAPIKey").value.trim(),
    searxng_mode: searxMode,
    max_results: number("webSearchMaxResults", 6),
    timeout_seconds: number("webSearchTimeout", 10),
    max_tool_rounds: number("webSearchMaxRounds", 2),
    max_result_bytes: number("webSearchMaxBytes", 65536),
  };
}

async function saveConfig(section) {
  collectUpstreams();
  const aliasError = validateAliasRows();
  if (aliasError) {
    showToast(aliasError.message, "error");
    aliasError.element?.scrollIntoView({ behavior: "smooth", block: "center" });
    aliasError.element?.focus?.();
    return;
  }
  collectAliases();
  const cleanup = reconcileRemovedUpstreamModelMappings();
  collectEfforts();
  collectSocks5();
  collectWebSearchConfig();
  const cfg = {
    model_alias: aliasData,
    reasoning_effort_map: globalEffortData,
    web_search: webSearchData,
    socks5_proxies: socks5Data,
    upstreams: upstreamData,
    upstream_order: upstreamOrder,
  };
  const label = section || "配置";
  const saveBtns = Array.from(
    document.querySelectorAll(".btn-success[data-saving-group], .btn-success"),
  ).filter((btn) => /保存/.test(btn.textContent || ""));
  saveBtns.forEach((btn) => {
    if (!btn.dataset.originalHtml) btn.dataset.originalHtml = btn.innerHTML;
    btn.disabled = true;
    btn.classList.add("is-loading");
  });
  try {
    const r = await apiFetch("/api/config", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(cfg),
    });
    if (!r.ok) throw new Error(await r.text());
    rememberSavedUpstreamModels();
    const cleanupSuffix = cleanup.targetCount
      ? "，已清理 " +
        cleanup.targetCount +
        " 个关联映射目标" +
        (cleanup.removedAliasCount
          ? "并删除 " + cleanup.removedAliasCount + " 个空别名"
          : "")
      : "";
    showToast(label + "已保存" + cleanupSuffix, "success");
    loadConfig();
  } catch (e) {
    if (String(e.message || "").indexOf("登录已失效") !== -1) return;
    showToast(label + "保存失败: " + e.message, "error");
  } finally {
    saveBtns.forEach((btn) => {
      btn.disabled = false;
      btn.classList.remove("is-loading");
      if (btn.dataset.originalHtml) btn.innerHTML = btn.dataset.originalHtml;
    });
  }
}

/* ===== 工具函数 ===== */
function esc(s) {
  const d = document.createElement("div");
  d.textContent = s == null ? "" : String(s);
  return d.innerHTML;
}

function escAttr(s) {
  return String(s == null ? "" : s)
    .replace(/&/g, "&amp;")
    .replace(/"/g, "&quot;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

async function copyText(text) {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.left = "-9999px";
    ta.style.top = "0";
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch (e) {
    return false;
  }
}

function showToast(msg, t) {
  const e = document.getElementById("toast");
  const icon = t === "error" ? ICONS.alert : ICONS.check;
  e.innerHTML = icon + "<span>" + esc(msg) + "</span>";
  e.className = t + " show";
  clearTimeout(e._tid);
  e._tid = setTimeout(() => e.classList.remove("show"), 2500);
}

/* ===== 统计 ===== */
function emptyStateHtml(icon, title, desc) {
  return (
    '<div class="empty-state">' +
    '<div class="empty-state-icon">' +
    (icon || ICONS.inbox) +
    "</div>" +
    '<div class="empty-state-title">' +
    esc(title || "暂无数据") +
    "</div>" +
    (desc ? '<div class="empty-state-desc">' + esc(desc) + "</div>" : "") +
    "</div>"
  );
}

function emptyRowHtml(colspan, icon, title, desc) {
  return (
    '<tr><td colspan="' +
    colspan +
    '">' +
    emptyStateHtml(icon, title, desc) +
    "</td></tr>"
  );
}

function statsSkeletonHtml() {
  return (
    '<div class="stats-top-grid stats-top-grid-skeleton" aria-hidden="true">' +
    '<div class="stats-overview-skeleton"></div>' +
    '<div class="stats-heatmap-skeleton"></div>' +
    '</div>'
  );
}

const STATS_HEATMAP_DAY_COUNT = 365;
const STATS_HEATMAP_DAY_MS = 24 * 60 * 60 * 1000;

/* 单日 / 累计通过两个页签切换，刷新页面后仍默认展示单日 */
let statsPeriod = "today";
let statsSnapshot = null;

function statsDateFromKey(value) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(String(value || ""));
  if (!match) return null;
  const date = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
  if (
    date.getFullYear() !== Number(match[1]) ||
    date.getMonth() !== Number(match[2]) - 1 ||
    date.getDate() !== Number(match[3])
  ) {
    return null;
  }
  return date;
}

function statsDateKey(date) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return year + "-" + month + "-" + day;
}

function shiftStatsDate(date, days) {
  const next = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  next.setDate(next.getDate() + days);
  return next;
}

function formatStatsDate(date) {
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "long",
    day: "numeric",
    weekday: "short",
  }).format(date);
}

function statsHeatmapRow(row) {
  const requests = Math.max(0, Number(row && row.request_count) || 0);
  const prompt = Math.max(0, Number(row && row.prompt_tokens) || 0);
  const cached = Math.max(0, Number(row && row.cached_tokens) || 0);
  const completion = Math.max(0, Number(row && row.completion_tokens) || 0);
  const reportedTotal = Math.max(0, Number(row && row.total_tokens) || 0);
  const total = Math.max(reportedTotal, prompt + completion);
  return {
    requests,
    prompt,
    cached,
    completion,
    total,
    intensity: total || requests,
  };
}

function statsHeatmapLevel(value, scale) {
  if (!value || !scale.length) return 0;
  const rank = scale.reduce((count, item) => count + (item <= value ? 1 : 0), 0);
  return Math.min(4, Math.max(1, Math.ceil((rank / scale.length) * 4)));
}

function statsHeatmapCellLabel(date, value) {
  return (
    formatStatsDate(date) +
    "，调用 " +
    fmt(value.requests) +
    " 次，输入 " +
    fmt(value.prompt) +
    " Token，缓存命中 " +
    fmt(value.cached) +
    " Token，输出 " +
    fmt(value.completion) +
    " Token"
  );
}

function statsHeatmapSummaryHtml(summary) {
  return (
    '<div class="stats-heatmap-summary" aria-label="最近一年汇总">' +
    '<span><b>' + fmt(summary.requests) + '</b><em>调用</em></span>' +
    '<span><b>' + fmt(summary.prompt) + '</b><em>输入</em></span>' +
    '<span class="is-cached"><b>' + fmt(summary.cached) + '</b><em>缓存命中</em></span>' +
    '<span><b>' + fmt(summary.completion) + '</b><em>输出</em></span>' +
    "</div>"
  );
}

function statsDayMap(days) {
  const values = new Map();
  (days || []).forEach((row) => {
    const date = statsDateFromKey(row && row.date);
    if (date) values.set(statsDateKey(date), statsHeatmapRow(row));
  });
  return values;
}

function statsAxisDateLabel(date) {
  return date.getMonth() + 1 + "/" + date.getDate();
}

function statsTrendHtml(days, dateLabel) {
  const endDate = statsDateFromKey(dateLabel) || new Date();
  const startDate = shiftStatsDate(endDate, -29);
  const valuesByDate = statsDayMap(days);
  const points = [];
  const summary = { requests: 0, total: 0 };
  for (let index = 0; index < 30; index++) {
    const date = shiftStatsDate(startDate, index);
    const value = valuesByDate.get(statsDateKey(date)) || {
      requests: 0,
      prompt: 0,
      cached: 0,
      completion: 0,
      total: 0,
      intensity: 0,
    };
    points.push({ date, value });
    summary.requests += value.requests;
    summary.total += value.total;
  }
  const scale = points
    .map((point) => point.value.intensity)
    .filter((value) => value > 0)
    .sort((a, b) => a - b);
  const maxValue = Math.max(1, ...points.map((point) => point.value.intensity));
  const bars = points
    .map((point) => {
      const value = point.value;
      const height = value.intensity
        ? Math.max(8, Math.round((value.intensity / maxValue) * 100))
        : 3;
      const label = statsHeatmapCellLabel(point.date, value);
      return (
        '<span class="stats-trend-bar-wrap" title="' +
        escAttr(label) +
        '"><i class="stats-trend-bar level-' +
        statsHeatmapLevel(value.intensity, scale) +
        '" style="--bar-height:' +
        height +
        '%"></i></span>'
      );
    })
    .join("");
  return (
    '<section class="stats-chart-module stats-trend-module" aria-label="近 30 日用量趋势">' +
    '<div class="stats-chart-head"><div><h3>近 30 日趋势</h3><p>按每日 Token 总量</p></div>' +
    '<div class="stats-chart-metric"><b>' +
    fmt(summary.total) +
    '</b><span>Token</span><em>' +
    fmt(summary.requests) +
    " 次调用</em></div></div>" +
    '<div class="stats-trend-chart" role="img" aria-label="近 30 日每日 Token 用量柱状图">' +
    bars +
    "</div>" +
    '<div class="stats-chart-axis"><span>' +
    statsAxisDateLabel(startDate) +
    "</span><span>今天</span></div>" +
    "</section>"
  );
}

function statsOverviewHtml(days, dateLabel) {
  return statsTrendHtml(days, dateLabel);
}

function statsPeriodTabsHtml(dimension) {
  if (dimension === "day") return "";
  const tab = (value, label) => {
    const active = statsPeriod === value;
    return (
      '<button type="button" class="stats-period-tab' +
      (active ? " is-active" : "") +
      '" aria-pressed="' +
      String(active) +
      '" data-stats-period="' +
      value +
      '" onclick="setStatsPeriod(\'' +
      value +
      "')\">" +
      label +
      "</button>"
    );
  };
  return (
    '<div class="stats-period-tabs" role="group" aria-label="统计周期">' +
    tab("today", "单日") +
    tab("total", "累计") +
    "</div>"
  );
}

function setStatsPeriod(period) {
  if (period !== "today" && period !== "total") return;
  if (period === statsPeriod) return;
  statsPeriod = period;
  if (statsSnapshot) {
    renderStats(statsSnapshot);
  } else {
    loadStats();
  }
}

function statsControlsHtml(dimension) {
  const selected = (value) => (value === dimension ? " selected" : "");
  return (
    '<div class="stats-heading-actions" aria-label="统计操作">' +
    statsPeriodTabsHtml(dimension) +
    '<button class="btn btn-success" data-icon="refresh" onclick="reloadConfig()" title="刷新" aria-label="刷新"><span class="btn-label">刷新</span></button>' +
    '<button class="btn btn-danger" data-icon="trash" onclick="resetStats(this)" title="清空统计" aria-label="清空统计"><span class="btn-label">清空统计</span></button>' +
    '<select id="statsDimension" class="m-select stats-dimension" aria-label="统计维度" onchange="changeStatsDimension()">' +
    '<option value="model"' + selected("model") + ">按模型</option>" +
    '<option value="upstream"' + selected("upstream") + ">按上游</option>" +
    '<option value="model_upstream"' + selected("model_upstream") + ">模型 × 上游</option>" +
    '<option value="model_day"' + selected("model_day") + ">按日期 + 模型</option>" +
    '<option value="model_upstream_day"' + selected("model_upstream_day") + ">按日期 + 上游 + 模型</option>" +
    '<option value="day"' + selected("day") + ">按日期</option>" +
    "</select>" +
    '<span id="resetStatus" class="stats-reset-status" aria-live="polite"></span>' +
    "</div>"
  );
}

function statsHeadingHtml(heading, dateLabel, dimension) {
  return (
    '<div class="stats-heading"><h3>' +
    esc(heading) +
    (dateLabel && dimension !== "day" ? " · " + esc(dateLabel) : "") +
    "</h3>" +
    statsControlsHtml(dimension) +
    "</div>"
  );
}

function statsHeatmapHtml(days, dateLabel) {
  const valuesByDate = statsDayMap(days);

  const endDate = statsDateFromKey(dateLabel) || new Date();
  const startDate = shiftStatsDate(endDate, -(STATS_HEATMAP_DAY_COUNT - 1));
  const gridStart = shiftStatsDate(startDate, -startDate.getDay());
  const gridEnd = shiftStatsDate(endDate, 6 - endDate.getDay());
  const gridStartUTC = Date.UTC(gridStart.getFullYear(), gridStart.getMonth(), gridStart.getDate());
  const gridEndUTC = Date.UTC(gridEnd.getFullYear(), gridEnd.getMonth(), gridEnd.getDate());
  const weekCount = Math.floor((gridEndUTC - gridStartUTC) / STATS_HEATMAP_DAY_MS / 7) + 1;
  const scale = Array.from(valuesByDate.entries())
    .filter(([key, value]) => {
      const date = statsDateFromKey(key);
      return date && date >= startDate && date <= endDate && value.intensity > 0;
    })
    .map(([, value]) => value.intensity)
    .sort((a, b) => a - b);
  const periodSummary = { requests: 0, prompt: 0, cached: 0, completion: 0 };
  const monthLabels = [];
  let lastMonthColumn = 0;
  let monthCursor = new Date(startDate.getFullYear(), startDate.getMonth(), 1);
  while (monthCursor <= endDate) {
    const monthUTC = Date.UTC(monthCursor.getFullYear(), monthCursor.getMonth(), monthCursor.getDate());
    const monthColumn = Math.max(
      1,
      Math.floor((monthUTC - gridStartUTC) / STATS_HEATMAP_DAY_MS / 7) + 1,
    );
    if (monthColumn !== lastMonthColumn) {
      monthLabels.push(
        '<span class="stats-heatmap-month" style="--month-column:' +
          monthColumn +
          '">' +
          (monthCursor.getMonth() + 1) +
          "月</span>",
      );
      lastMonthColumn = monthColumn;
    }
    monthCursor = new Date(monthCursor.getFullYear(), monthCursor.getMonth() + 1, 1);
  }

  let cells = "";
  for (let week = 0; week < weekCount; week++) {
    for (let day = 0; day < 7; day++) {
      const date = shiftStatsDate(gridStart, week * 7 + day);
      if (date < startDate || date > endDate) {
        cells += '<span class="stats-heatmap-cell is-outside" aria-hidden="true"></span>';
        continue;
      }
      const value = valuesByDate.get(statsDateKey(date)) || {
        requests: 0,
        prompt: 0,
        cached: 0,
        completion: 0,
        total: 0,
        intensity: 0,
      };
      periodSummary.requests += value.requests;
      periodSummary.prompt += value.prompt;
      periodSummary.cached += value.cached;
      periodSummary.completion += value.completion;
      const label = statsHeatmapCellLabel(date, value);
      cells +=
        '<button class="stats-heatmap-cell level-' +
        statsHeatmapLevel(value.intensity, scale) +
        '" type="button" data-stats-day="1" data-date="' +
        escAttr(statsDateKey(date)) +
        '" data-requests="' +
        escAttr(value.requests) +
        '" data-prompt="' +
        escAttr(value.prompt) +
        '" data-cached="' +
        escAttr(value.cached) +
        '" data-completion="' +
        escAttr(value.completion) +
        '" aria-label="' +
        escAttr(label) +
        '"></button>';
    }
  }

  const weekdayLabels = ["", "一", "", "三", "", "五", ""]
    .map((label) => '<span>' + label + "</span>")
    .join("");
  return (
    '<section class="stats-heatmap" aria-label="每日用量热力图">' +
    '<div class="stats-heatmap-head">' +
    '<div class="stats-heatmap-title-group"><h3 class="stats-heatmap-title">每日用量</h3><div class="stats-heatmap-caption">最近一年 · 按每日总量分级</div></div>' +
    statsHeatmapSummaryHtml(periodSummary) +
    "</div>" +
    '<div class="stats-heatmap-scroll">' +
    '<div class="stats-heatmap-body"><div class="stats-heatmap-track">' +
    '<div class="stats-heatmap-weekdays" aria-hidden="true">' +
    weekdayLabels +
    "</div>" +
    '<div class="stats-heatmap-chart" style="--heatmap-weeks:' +
    weekCount +
    '">' +
    '<div class="stats-heatmap-months" aria-hidden="true">' +
    monthLabels.join("") +
    "</div>" +
    '<div class="stats-heatmap-grid" role="group" aria-label="最近一年每日 Token 用量">' +
    cells +
    "</div>" +
    "</div>" +
    "</div>" +
    "</div>" +
    "</div>" +
    '<div class="stats-heatmap-legend"><span>少</span><i class="level-0"></i><i class="level-1"></i><i class="level-2"></i><i class="level-3"></i><i class="level-4"></i><span>多</span></div>' +
    '<div class="stats-heatmap-tooltip" role="tooltip" hidden></div>' +
    "</section>"
  );
}

function fitStatsHeatmap(root) {
  const scroll = root.querySelector(".stats-heatmap-scroll");
  const chart = root.querySelector(".stats-heatmap-chart");
  const weekdays = root.querySelector(".stats-heatmap-weekdays");
  const track = root.querySelector(".stats-heatmap-track");
  if (!scroll || !chart || !weekdays || !track || !scroll.clientWidth) return;
  const weeks = Number.parseInt(chart.style.getPropertyValue("--heatmap-weeks"), 10);
  if (!Number.isFinite(weeks) || weeks <= 0) return;
  const gap = Number.parseFloat(getComputedStyle(root).getPropertyValue("--heat-gap")) || 3;
  const maxCell = Number.parseFloat(getComputedStyle(root).getPropertyValue("--heat-cell-max")) || 16;
  const trackGap = Number.parseFloat(getComputedStyle(track).columnGap) || gap;
  const available = Math.max(0, scroll.clientWidth - weekdays.offsetWidth - trackGap);
  const widthFitCell = (available - (weeks - 1) * gap) / weeks;
  // The calendar has a fixed 53-column shape. Size it from the available width
  // and let the module grow vertically instead of compressing every cell.
  const cell = Math.floor(Math.min(maxCell, Math.max(1, widthFitCell)) * 10) / 10;
  const chartWidth = weeks * cell + (weeks - 1) * gap;
  chart.style.width = Math.ceil(chartWidth * 10) / 10 + "px";
  root.style.setProperty("--heat-cell", cell + "px");
}

function bindStatsHeatmap() {
  const root = document.querySelector(".stats-heatmap");
  if (!root) return;
  const tooltip = root.querySelector(".stats-heatmap-tooltip");
  if (!tooltip) return;
  fitStatsHeatmap(root);
  const scroll = root.querySelector(".stats-heatmap-scroll");
  if (scroll && typeof ResizeObserver === "function") {
    const observer = new ResizeObserver(() => fitStatsHeatmap(root));
    observer.observe(scroll);
  }
  let activeCell = null;

  const hideTooltip = () => {
    activeCell = null;
    tooltip.hidden = true;
    tooltip.style.visibility = "";
  };

  const showTooltip = (cell) => {
    activeCell = cell;
    const date = statsDateFromKey(cell.dataset.date);
    if (!date) return;
    const value = {
      requests: Number(cell.dataset.requests) || 0,
      prompt: Number(cell.dataset.prompt) || 0,
      cached: Number(cell.dataset.cached) || 0,
      completion: Number(cell.dataset.completion) || 0,
    };
    tooltip.innerHTML =
      '<div class="stats-heatmap-tooltip-date">' +
      esc(formatStatsDate(date)) +
      "</div>" +
      '<div class="stats-heatmap-tooltip-row"><span>调用</span><b>' +
      fmt(value.requests) +
      " 次</b></div>" +
      '<div class="stats-heatmap-tooltip-row"><span>输入</span><b>' +
      fmt(value.prompt) +
      " Token</b></div>" +
      '<div class="stats-heatmap-tooltip-row is-cached"><span>缓存命中</span><b>' +
      fmt(value.cached) +
      " Token</b></div>" +
      '<div class="stats-heatmap-tooltip-row"><span>输出</span><b>' +
      fmt(value.completion) +
      " Token</b></div>";
    tooltip.hidden = false;
    tooltip.style.visibility = "hidden";
    requestAnimationFrame(() => {
      if (activeCell !== cell) return;
      const cellRect = cell.getBoundingClientRect();
      const tooltipRect = tooltip.getBoundingClientRect();
      const edge = 10;
      const gap = 10;
      const left = Math.min(
        Math.max(edge, cellRect.left + cellRect.width / 2 - tooltipRect.width / 2),
        window.innerWidth - tooltipRect.width - edge,
      );
      const above = cellRect.top - tooltipRect.height - gap;
      const top = above >= edge ? above : cellRect.bottom + gap;
      tooltip.style.left = Math.round(left) + "px";
      tooltip.style.top = Math.round(Math.max(edge, top)) + "px";
      tooltip.style.visibility = "visible";
    });
  };

  root.querySelectorAll("[data-stats-day]").forEach((cell) => {
    cell.addEventListener("mouseenter", () => showTooltip(cell));
    cell.addEventListener("mouseleave", hideTooltip);
    cell.addEventListener("focus", () => showTooltip(cell));
    cell.addEventListener("blur", hideTooltip);
  });
  const scrollContainer = root.closest(".stats-content");
  if (scrollContainer) scrollContainer.addEventListener("scroll", hideTooltip, { passive: true });
}

function sumModelStats(map, keys) {
  let requests = 0,
    prompt = 0,
    cached = 0,
    completion = 0,
    total = 0;
  for (const k of keys) {
    const m = map[k];
    if (!m) continue;
    requests += m.request_count || 0;
    prompt += m.prompt_tokens || 0;
    cached += m.cached_tokens || 0;
    completion += m.completion_tokens || 0;
    total += m.total_tokens || 0;
  }
  return { requests, prompt, cached, completion, total };
}

function emptyModelStat() {
  return {
    request_count: 0,
    prompt_tokens: 0,
    cached_tokens: 0,
    completion_tokens: 0,
    total_tokens: 0,
  };
}

function modelStatCells(m) {
  const s = m || emptyModelStat();
  return (
    '<td class="num-cell stats-request-cell">' +
    fmt(s.request_count) +
    "</td>" +
    '<td class="num-cell stats-prompt-cell">' +
    fmt(s.prompt_tokens) +
    "</td>" +
    '<td class="num-cell stats-cache-cell">' +
    fmt(s.cached_tokens) +
    "</td>" +
    '<td class="num-cell stats-completion-cell">' +
    fmt(s.completion_tokens) +
    "</td>" +
    '<td class="num-cell stats-token-total-cell">' +
    fmt(s.total_tokens) +
    "</td>"
  );
}

function statsRouteCells(row, includeDate) {
  const labels = includeDate ? [row.date || "-"] : [];
  labels.push(row.upstream || "-", row.model || "-", row.upstream_model || row.model || "-");
  return labels.map((label) => '<td class="stats-route-cell" title="' + escAttr(label) + '">' + esc(label) + '</td>').join("");
}

function statsTableHtml(rowsHtml, dateLabel, dimensionLabel, period) {
  const isToday = period !== "total";
  const title = isToday
    ? dateLabel
      ? "单日用量 · " + esc(dateLabel)
      : "单日用量"
    : "累计用量";
  const labels = Array.isArray(dimensionLabel) ? dimensionLabel : [dimensionLabel];
  const tableClass = "tbl stats-period-table " + (labels.length > 1 ? "stats-route-table " : "") + (isToday ? "is-today" : "is-total");
  return (
    '<div class="stats-table-wrap"><table class="' +
    tableClass +
    '" id="statsTable">' +
    "<thead>" +
    "<tr>" +
    labels.map((label) => '<th class="stats-model-col" rowspan="2">' + esc(label) + '</th>').join("") +
    '<th colspan="5" class="stats-group-head">' +
    title +
    "</th>" +
    "</tr>" +
    '<tr class="stats-subhead">' +
    "<th>请求</th><th>输入</th><th class=\"stats-cache-head\">缓存命中</th><th>输出</th><th>合计</th>" +
    "</tr>" +
    "</thead>" +
    "<tbody>" +
    rowsHtml +
    "</tbody></table></div>"
  );
}

function statsRowMap(rows) {
  const result = {};
  (rows || []).forEach((row) => {
    const key = JSON.stringify([row.model || "", row.upstream || "", row.upstream_model || row.model || ""]);
    result[key] = row;
  });
  return result;
}

function statsModelDayMap(rows, dateFilter) {
  const result = {};
  (rows || []).forEach((row) => {
    const date = row.date || "";
    const model = row.model || "";
    if (!date || !model) return;
    if (dateFilter && date !== dateFilter) return;
    result[date + "\u0000" + model] = row;
  });
  return result;
}

function statsModelUpstreamDayMap(rows, dateFilter) {
  const result = {};
  (rows || []).forEach((row) => {
    const date = row.date || "";
    const upstream = row.upstream || "";
    const model = row.model || "";
    if (!date || !upstream || !model) return;
    if (dateFilter && date !== dateFilter) return;
    result[date + "\u0000" + upstream + "\u0000" + model + "\u0000" + (row.upstream_model || model)] = row;
  });
  return result;
}

function setUsageFilterOptions(id, values, emptyLabel) {
  const select = document.getElementById(id);
  if (!select) return;
  const selected = select.value;
  const options = Array.from(new Set([...values, selected].filter(Boolean))).sort((a, b) =>
    a.localeCompare(b),
  );
  select.innerHTML =
    '<option value="">' + esc(emptyLabel) + "</option>" +
    options
      .map((value) => '<option value="' + escAttr(value) + '">' + esc(value) + "</option>")
      .join("");
  select.value = options.includes(selected) ? selected : "";
	ssSyncLabel(select);
}

async function resetStats(btn) {
  const confirmed = await showDangerConfirm(btn, {
    title: "确认清空 Token 统计？",
    description: "所有累计与每日 Token 统计都将被永久清空。",
    note: "此操作立即生效且不可撤销，请谨慎操作。",
    confirmLabel: "确认清空",
  });
  if (!confirmed) return;
  const s = document.getElementById("resetStatus");
  s.textContent = "清空中...";
  try {
    const r = await apiFetch("/api/stats", { method: "DELETE" });
    if (!r.ok) throw new Error(await r.text());
    await loadStats();
    const nextStatus = document.getElementById("resetStatus");
    if (nextStatus) {
      nextStatus.textContent = "已清空";
      setTimeout(() => (nextStatus.textContent = ""), 2000);
    }
  } catch (e) {
    if (String(e.message || "").indexOf("登录已失效") !== -1) return;
    s.textContent = "失败: " + e.message;
  }
}

let statsRenderKey = "";

function changeStatsDimension() {
  if (statsSnapshot) renderStats(statsSnapshot);
  else loadStats();
}

function statsSignature(d, dimension) {
  return dimension + "|" + statsPeriod + "|" + JSON.stringify(d);
}

// 统计区每 5 秒自动刷新。重绘时保留已增强的维度下拉框节点，
// 避免下拉菜单打开期间被整体替换，导致点击选项后选中值丢失。
function mountStatsContent(html) {
  const content = document.getElementById("statsContent");
  if (!content) return;
  const prevWrapper = content.querySelector(
    ".stats-heading-actions > .ss-wrapper",
  );
  content.innerHTML = html;
  const nextSelect = document.getElementById("statsDimension");
  ssEnhanceSelect(nextSelect);
  const nextWrapper = nextSelect && nextSelect.closest(".ss-wrapper");
  if (!prevWrapper || prevWrapper === nextWrapper) return;
  if (!nextWrapper) {
    const actions = content.querySelector(".stats-heading-actions");
    if (actions) actions.appendChild(prevWrapper);
    return;
  }
  const prevSelect = prevWrapper.querySelector("select");
  if (prevSelect && nextSelect) {
    prevSelect.value = nextSelect.value;
    ssSyncLabel(prevSelect);
  }
  nextWrapper.replaceWith(prevWrapper);
  const trigger = prevWrapper.querySelector(".ss-trigger");
  if (trigger && ssActiveDropdown && ssActiveWrapper === prevWrapper) {
    ssPositionDropdown(trigger, ssActiveDropdown);
  }
}

async function loadStats() {
  try {
    const d = await apiJSON("/api/stats");
    statsSnapshot = d;
    const dimension =
      document.getElementById("statsDimension")?.value || "model";
    // 数据与视图状态都没变时不重绘，减少无谓的 DOM 重建。
    if (statsSignature(d, dimension) === statsRenderKey) return;
    renderStats(d);
  } catch (e) {
    if (String(e.message || "").indexOf("登录已失效") !== -1) return;
    statsRenderKey = "";
    const currentDimension = document.getElementById("statsDimension")?.value || "model";
    mountStatsContent(
      statsHeadingHtml("加载失败", "", currentDimension) +
        emptyStateHtml(ICONS.alert, "加载失败", e.message || "请稍后重试"),
    );
    injectButtonIcons();
  }
}

function renderStats(d) {
  const ms = d.models || {};
  const dm = d.daily ? d.daily.models || {} : {};
  const dateLabel = d.daily && d.daily.date ? d.daily.date : "";
  const dimension = document.getElementById("statsDimension")?.value || "model";
  statsRenderKey = statsSignature(d, dimension);
  setUsageFilterOptions("usageModelFilter", Object.keys(ms), "全部模型");
  setUsageFilterOptions("usageUpstreamFilter", Object.keys(d.upstreams || {}), "全部上游");

  let h = "";
  h +=
    '<div class="stats-top-grid">' +
    statsOverviewHtml(d.days || [], dateLabel) +
    statsHeatmapHtml(d.days || [], dateLabel) +
    "</div>";

  let cumulative = ms;
  let todayValues = dm;
  let dimensionLabel = "模型";
  let heading = "模型用量";
  const displayLabels = {};
  if (dimension === "upstream") {
    cumulative = d.upstreams || {};
    todayValues = d.daily_upstreams || {};
    dimensionLabel = "上游";
    heading = "上游用量";
  } else if (dimension === "model_upstream") {
    cumulative = statsRowMap(d.model_upstreams);
    todayValues = statsRowMap(d.daily_model_upstreams);
    dimensionLabel = ["上游", "请求模型（下游）", "实际模型（上游）"];
    heading = "模型与上游用量";
  } else if (dimension === "model_day") {
    cumulative = statsModelDayMap(d.model_days);
    todayValues = statsModelDayMap(d.model_days, dateLabel);
    Object.keys(cumulative).forEach((key) => {
      const parts = key.split("\u0000");
      displayLabels[key] = (parts[0] || "-") + " / " + (parts[1] || "-");
    });
    dimensionLabel = "日期 / 模型";
    heading = "日期与模型用量";
  } else if (dimension === "model_upstream_day") {
    cumulative = statsModelUpstreamDayMap(d.model_upstream_days);
    todayValues = statsModelUpstreamDayMap(d.model_upstream_days, dateLabel);
    dimensionLabel = ["日期", "上游", "请求模型（下游）", "实际模型（上游）"];
    heading = "日期、上游与模型用量";
  } else if (dimension === "day") {
    heading = "每日用量明细";
  }

  h += statsHeadingHtml(
    heading,
    statsPeriod === "today" && dimension !== "day" ? dateLabel : "",
    dimension,
  );
  if (dimension === "day") {
    const days = d.days || [];
    if (!days.length) {
      h += emptyStateHtml(ICONS.chart, "暂无统计数据", "网关处理请求后会在此汇总每日 Token 用量");
    } else {
      const rows = days
        .map(
          (row) =>
            '<tr><td><span class="stats-dimension-label is-date">' +
            esc(row.date || "-") +
            "</span></td>" +
            modelStatCells(row) +
            "</tr>",
        )
        .join("");
      h += '<div class="stats-table-wrap"><table class="tbl stats-daily-table"><thead><tr><th>日期</th><th>请求</th><th>输入</th><th class="stats-cache-head">缓存命中</th><th>输出</th><th>合计</th></tr></thead><tbody>' + rows + "</tbody></table></div>";
    }
    mountStatsContent(h);
    injectButtonIcons();
    bindStatsHeatmap();
    return;
  }

  const showCumulative = statsPeriod === "total";
  const values = showCumulative ? cumulative : todayValues;
  const keys = Object.keys(values);
  if (dimension === "model_day" || dimension === "model_upstream_day") {
    keys.sort((a, b) => {
      const dateA = a.split("\u0000")[0] || "";
      const dateB = b.split("\u0000")[0] || "";
      if (dateA !== dateB) return dateB.localeCompare(dateA);
      const totalDiff = ((values[b] || {}).total_tokens || 0) - ((values[a] || {}).total_tokens || 0);
      return totalDiff || a.localeCompare(b);
    });
  } else {
    keys.sort((a, b) => {
      const totalDiff = ((values[b] || {}).total_tokens || 0) - ((values[a] || {}).total_tokens || 0);
      return totalDiff || a.localeCompare(b);
    });
  }
  if (!keys.length) {
    h += emptyStateHtml(
      ICONS.chart,
      "暂无统计数据",
      "网关处理请求后会在此汇总 Token 用量",
    );
  } else {
    let rows = "";
    const isDateDimension =
      dimension === "model_day" || dimension === "model_upstream_day";
    const labelClass = isDateDimension
      ? "stats-dimension-label is-date-model"
      : "stats-dimension-label";
    for (const k of keys) {
      const routeCells = Array.isArray(dimensionLabel)
        ? statsRouteCells(values[k], dimension === "model_upstream_day")
        : '<td><span class="' + labelClass + '">' + esc(displayLabels[k] || k) + '</span></td>';
      rows +=
        '<tr>' + routeCells +
        modelStatCells(values[k]) +
        "</tr>";
    }
    const totals = sumModelStats(values, Object.keys(values));
    rows +=
      '<tr class="stats-total-row"><td colspan="' + (Array.isArray(dimensionLabel) ? dimensionLabel.length : 1) + '"><span class="stats-dimension-label is-total">合计</span></td>' +
      modelStatCells({
        request_count: totals.requests,
        prompt_tokens: totals.prompt,
        cached_tokens: totals.cached,
        completion_tokens: totals.completion,
        total_tokens: totals.total,
      }) +
      "</tr>";
    h += statsTableHtml(rows, dateLabel, dimensionLabel, statsPeriod);
  }
  mountStatsContent(h);
  injectButtonIcons();
  bindStatsHeatmap();
}

let usageTotal = 0;

function getUsageTotalPages() {
  return Math.max(1, Math.ceil(usageTotal / USAGE_PAGE_SIZE));
}

function updateUsagePagination() {
  const totalPages = getUsageTotalPages();
  const currentPage = usageTotal
    ? Math.min(Math.floor(usagePageOffset / USAGE_PAGE_SIZE) + 1, totalPages)
    : 1;
  const pageInput = document.getElementById("usagePageInput");
  const totalPagesElement = document.getElementById("usageTotalPages");
  const pageIndicator = document.getElementById("usagePageIndicator");
  if (pageInput) {
    pageInput.value = String(currentPage);
    pageInput.max = String(totalPages);
  }
  if (totalPagesElement) totalPagesElement.textContent = String(totalPages);
  if (pageIndicator) pageIndicator.setAttribute("aria-label", "第 " + currentPage + " 页，共 " + totalPages + " 页");
  const prev = document.getElementById("usagePrev");
  const next = document.getElementById("usageNext");
  if (prev) prev.disabled = usagePageOffset <= 0;
  if (next) next.disabled = !usageTotal || currentPage >= totalPages;
}

function jumpToUsagePage() {
  const pageInput = document.getElementById("usagePageInput");
  if (!pageInput) return;
  const totalPages = getUsageTotalPages();
  const requestedPage = Number.parseInt(pageInput.value, 10);
  if (!Number.isFinite(requestedPage)) {
    updateUsagePagination();
    return;
  }
  const targetPage = Math.min(Math.max(requestedPage, 1), totalPages);
  const nextOffset = (targetPage - 1) * USAGE_PAGE_SIZE;
  pageInput.value = String(targetPage);
  if (nextOffset === usagePageOffset) {
    updateUsagePagination();
    return;
  }
  loadUsageRecords(nextOffset);
}

function handleUsagePageInputKeydown(event) {
  if (event.key !== "Enter") return;
  event.preventDefault();
  jumpToUsagePage();
  event.currentTarget.blur();
}

function formatUsageTime(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value || "-";
  return date.toLocaleString("zh-CN", { hour12: false });
}

function formatUsageDuration(value) {
  const milliseconds = Number(value);
  if (!Number.isFinite(milliseconds) || milliseconds <= 0) return "-";
  if (milliseconds < 1000) return Math.round(milliseconds) + " ms";
  if (milliseconds < 60000) {
    const digits = milliseconds < 10000 ? 2 : 1;
    return (milliseconds / 1000).toFixed(digits) + " s";
  }
  const minutes = Math.floor(milliseconds / 60000);
  const seconds = Math.round((milliseconds % 60000) / 1000);
  return minutes + " m " + seconds + " s";
}

function usageTimeParts(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return { date: value || "-", time: "" };
  return {
    date: date.toLocaleDateString("zh-CN", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
    }),
    time: date.toLocaleTimeString("zh-CN", {
      hour12: false,
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    }),
  };
}

function usageLatencyTone(value, kind) {
  const milliseconds = Number(value) || 0;
  if (!milliseconds) return "is-empty";
  const fastLimit = kind === "first" ? 800 : 3000;
  const slowLimit = kind === "first" ? 2500 : 10000;
  if (milliseconds <= fastLimit) return "is-fast";
  if (milliseconds >= slowLimit) return "is-slow";
  return "is-normal";
}

function usageRecordRowHtml(item) {
  const time = usageTimeParts(item.called_at);
  const requestCount = Math.max(1, Number(item.request_count) || 1);
  const aggregate =
    requestCount > 1
      ? '<span class="usage-aggregate">历史聚合 × ' + fmt(requestCount) + "</span>"
      : "";
  const keyName = item.api_key_name || "未记录密钥";
  const requestModel = item.request_model || "-";
  const upstreamName = item.upstream_name || "-";
  const upstreamModel = item.upstream_model || "-";
  const prompt = Math.max(0, Number(item.prompt_tokens) || 0);
  const cached = Math.max(0, Number(item.cached_tokens) || 0);
  const completion = Math.max(0, Number(item.completion_tokens) || 0);
  const total = Math.max(0, Number(item.total_tokens) || 0, prompt + completion);
  const firstByte = formatUsageDuration(item.first_byte_ms);
  const duration = formatUsageDuration(item.duration_ms);

  return (
    '<tr class="usage-record-row">' +
    '<td class="usage-call-cell"><div class="usage-call-line"><span class="usage-call-time"><strong>' +
    esc(time.date) +
    "</strong><span>" +
    esc(time.time) +
    '</span></span><span class="usage-key-chip" title="' +
    escAttr(keyName) +
    '"><i></i>' +
    "<span>" +
    esc(keyName) +
    "</span></span>" +
    aggregate +
    "</div></td>" +
    '<td class="usage-route-cell"><div class="usage-route-flow"><span class="usage-route-model is-request" title="' +
    escAttr(requestModel) +
    '">' +
    esc(requestModel) +
    '</span><span class="usage-route-arrow" aria-hidden="true">' +
    ICONS.arrowRight +
    '</span><span class="usage-route-model is-upstream" title="' +
    escAttr(upstreamModel) +
    '">' +
    esc(upstreamModel) +
    '</span><span class="usage-route-provider">' +
    ICONS.server +
    '<span title="' +
    escAttr(upstreamName) +
    '">' +
    esc(upstreamName) +
    "</span></span></div></td>" +
    '<td class="usage-performance-cell"><div class="usage-performance-grid"><span><em>首字</em><b class="' +
    usageLatencyTone(item.first_byte_ms, "first") +
    '">' +
    esc(firstByte) +
    '</b></span><span><em>总耗时</em><b class="' +
    usageLatencyTone(item.duration_ms, "total") +
    '">' +
    esc(duration) +
    "</b></span></div></td>" +
    '<td class="usage-token-cell"><div class="usage-token-line"><span class="usage-token-metric is-total"><em>合计</em><b>' +
    fmt(total) +
    '</b></span><span class="usage-token-metric is-prompt"><i></i><em>输入</em><b>' +
    fmt(prompt) +
    '</b></span><span class="usage-token-metric is-cached' +
    (cached > 0 ? " has-value" : "") +
    '"><i></i><em>缓存命中</em><b>' +
    fmt(cached) +
    '</b></span><span class="usage-token-metric is-completion"><i></i><em>输出</em><b>' +
    fmt(completion) +
    "</b></span></div></td>" +
    '<td class="usage-action"><button class="btn-icon btn-icon-danger" type="button" title="删除使用记录" aria-label="删除使用记录" onclick="deleteUsageRecord(' +
    Number(item.id) +
    ',this)">' +
    ICONS.trash +
    "</button></td></tr>"
  );
}

async function loadUsageRecords(offset) {
  if (typeof offset === "number") usagePageOffset = Math.max(0, offset);
  const loadSequence = ++usageLoadSequence;
  const params = new URLSearchParams({ limit: String(USAGE_PAGE_SIZE), offset: String(usagePageOffset) });
  const model = document.getElementById("usageModelFilter")?.value || "";
  const upstream = document.getElementById("usageUpstreamFilter")?.value || "";
  const apiKeyName = document.getElementById("usageAPIKeyFilter")?.value || "";
  const date = document.getElementById("usageDateFilter")?.value || "";
  const hasFilters = !!(model || upstream || apiKeyName || date);
  updatePageFilters("usage");
  if (model) params.set("model", model);
  if (upstream) params.set("upstream", upstream);
  if (apiKeyName) params.set("key_name", apiKeyName);
  if (date) params.set("date", date);
  const tbody = document.querySelector("#usageTable tbody");
  if (!tbody) return;
  document.getElementById("usageTable").setAttribute("aria-busy", "true");
  try {
    const page = await apiJSON("/api/usage?" + params.toString());
    if (loadSequence !== usageLoadSequence) return;
    usageTotal = page.total || 0;
    const items = page.items || [];
    usageRecords = items;
    if (!items.length && usageTotal > 0 && usagePageOffset >= usageTotal) {
      const lastOffset = Math.floor((usageTotal - 1) / USAGE_PAGE_SIZE) * USAGE_PAGE_SIZE;
      loadUsageRecords(lastOffset);
      return;
    }
    if (!items.length) {
      tbody.innerHTML = hasFilters
        ? '<tr><td colspan="5">' + emptyStateHtml(ICONS.search, "没有匹配的使用记录", "试试减少筛选条件，或清空筛选查看全部记录") + '<div class="filter-empty-actions"><button class="btn btn-default" type="button" onclick="resetPageFilters(\'usage\')">清空筛选</button></div></td></tr>'
        : emptyRowHtml(5, ICONS.inbox, "暂无使用记录", "成功调用模型后会在这里显示详细用量");
    } else {
      tbody.innerHTML = items.map(usageRecordRowHtml).join("");
    }
    const keyNames = Array.from(
      new Set([
        ...apiKeysData.map((item) => item.name || ""),
        ...(page.key_names || []),
      ]),
    ).filter(Boolean);
    setUsageFilterOptions("usageAPIKeyFilter", keyNames, "全部密钥");
    const keyFilter = document.getElementById("usageAPIKeyFilter");
    if (apiKeyName && keyFilter && keyFilter.value !== apiKeyName) {
      loadUsageRecords(0);
      return;
    }
    const summary = page.summary || {};
    document.getElementById("usageSummaryRequests").textContent = fmt(summary.request_count);
    document.getElementById("usageSummaryPrompt").textContent = fmt(summary.prompt_tokens);
    document.getElementById("usageSummaryCached").textContent = fmt(summary.cached_tokens);
    document.getElementById("usageSummaryCompletion").textContent = fmt(summary.completion_tokens);
    document.getElementById("usageSummaryTotal").textContent = fmt(summary.total_tokens);
    const start = usageTotal ? usagePageOffset + 1 : 0;
    const end = Math.min(usagePageOffset + items.length, usageTotal);
    document.getElementById("usageSummary").textContent = "共 " + fmt(usageTotal) + " 条 · " + fmt(start) + "–" + fmt(end);
    updateUsagePagination();
    updatePageFilters("usage", usageTotal);
  } catch (e) {
    if (loadSequence !== usageLoadSequence) return;
    if (String(e.message || "").indexOf("登录已失效") !== -1) return;
    usageRecords = [];
    tbody.innerHTML = emptyRowHtml(5, ICONS.alert, "加载失败", e.message || "请稍后重试");
    document.getElementById("usageSummaryRequests").textContent = "0";
    document.getElementById("usageSummaryPrompt").textContent = "0";
    document.getElementById("usageSummaryCached").textContent = "0";
    document.getElementById("usageSummaryCompletion").textContent = "0";
    document.getElementById("usageSummaryTotal").textContent = "0";
    updateUsagePagination();
  } finally {
    if (loadSequence === usageLoadSequence) document.getElementById("usageTable").setAttribute("aria-busy", "false");
  }
}

async function deleteUsageRecord(id, trigger) {
  const item = usageRecords.find((record) => Number(record.id) === Number(id));
  if (!item) {
    showToast("使用记录已更新，请刷新后重试", "error");
    return;
  }
  const confirmed = await showDangerConfirm(trigger, {
    title: "确认删除使用记录？",
    subject: (item.request_model || "未命名模型") + " · " + formatUsageTime(item.called_at),
    description: "该次调用记录及其 Token 用量将从统计中移除。",
    note: "删除操作立即生效且不可撤销。",
    confirmLabel: "确认删除",
  });
  if (!confirmed) return;
  if (trigger) trigger.disabled = true;
  try {
    await apiJSON("/api/usage?id=" + encodeURIComponent(id), { method: "DELETE" });
    const remainingTotal = Math.max(0, usageTotal - 1);
    const nextOffset =
      usagePageOffset > 0 && usagePageOffset >= remainingTotal
        ? Math.max(0, usagePageOffset - USAGE_PAGE_SIZE)
        : usagePageOffset;
    await loadStats();
    await loadUsageRecords(nextOffset);
    showToast("使用记录已删除", "success");
  } catch (e) {
    if (String(e.message || "").indexOf("登录已失效") !== -1) return;
    if (trigger) trigger.disabled = false;
    showToast("删除失败: " + e.message, "error");
  }
}

function changeUsagePage(direction) {
  const next = usagePageOffset + direction * USAGE_PAGE_SIZE;
  if (next < 0 || next >= usageTotal && direction > 0) return;
  loadUsageRecords(next);
}

function fmt(n) {
  return (n == null ? 0 : n).toString().replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

/* ===== 初始化 ===== */
function initAdminPage() {
  initAdminTabs();
  enhanceFilterInputs(document);
  new MutationObserver((mutations) => {
    mutations.forEach((mutation) => mutation.addedNodes.forEach((node) => {
      if (!(node instanceof Element)) return;
      enhanceFilterInputs(node);
      if (node.matches('input[type="search"], input[data-clearable-input]')) enhanceFilterInputs(node.parentElement);
    }));
  }).observe(document.body, { childList: true, subtree: true });
  ssEnhanceSelect(document.getElementById("upstreamModelFilter"));
  ssEnhanceSelect(document.getElementById("usageModelFilter"));
  ssEnhanceSelect(document.getElementById("usageUpstreamFilter"));
  ssEnhanceSelect(document.getElementById("usageAPIKeyFilter"));
  initUpstreamDragSort();
  initUpstreamNameAutofill();
  const baseURL = document.getElementById("webSearchBaseURL");
  if (baseURL) {
    baseURL.addEventListener("input", fitWebSearchBaseURLWidth);
    baseURL.addEventListener("change", fitWebSearchBaseURLWidth);
  }
  loadConfig();
  loadAPIKeys();
  loadStats();
}
if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", initAdminPage, { once: true });
} else {
  initAdminPage();
}
setInterval(function () {
  loadStats();
  if (document.querySelector('[data-tab-panel="usage"]')?.classList.contains("is-active")) {
    loadUsageRecords();
  }
}, 5000);
document.addEventListener("visibilitychange", function () {
  if (!document.hidden) {
    loadStats();
    if (document.querySelector('[data-tab-panel="usage"]')?.classList.contains("is-active")) loadUsageRecords();
  }
});
