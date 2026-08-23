const PLATFORM_API_BASE = String(window.RISKLENS_PLATFORM_API || "http://127.0.0.1:8000").replace(
  /\/$/,
  ""
);
const API_TIMEOUT_MS = 10000;
const LIST_CACHE_TTL_MS = 30000;

const state = {
  view: "fleet",
  fleet: {
    items: [],
    total: 0,
    limit: 25,
    offset: 0,
    sort: "hostname",
    q: "",
    status: "",
    tag: "",
    loading: false,
    error: null,
    summary: {
      total_agents: 0,
      online: 0,
      offline: 0,
      warning: 0,
      never_seen: 0,
      healthy: 0,
    },
    selectedId: null,
    tab: "overview",
    healthRefreshTimer: null,
    detail: null,
    detailLoading: false,
    detailError: null,
    users: null,
    software: null,
    sessions: null,
    applicationActivity: null,
    browserActivity: null,
    browserSort: "-duration_seconds",
    browserTimelineFilter: null,
    health: null,
    healthLoading: false,
    healthError: null,
    extraLoading: false,
    extraError: null,
    extraTab: null,
    extraLoadGen: 0,
    fetchedAt: 0,
    queryKey: "",
    loadGen: 0,
  },
  users: {
    items: [],
    total: 0,
    limit: 25,
    offset: 0,
    sort: "username",
    username: "",
    account_type: "",
    enabled: "",
    loading: false,
    error: null,
    fetchedAt: 0,
    queryKey: "",
    loadGen: 0,
  },
  software: {
    items: [],
    total: 0,
    limit: 25,
    offset: 0,
    sort: "application_name",
    application_name: "",
    publisher: "",
    version: "",
    architecture: "",
    loading: false,
    error: null,
    fetchedAt: 0,
    queryKey: "",
    loadGen: 0,
  },
  productivity: {
    agents: [],
    agentId: "",
    report: null,
    loading: false,
    agentsLoading: false,
    error: null,
    agentsFetchedAt: 0,
    reportQueryKey: "",
    loadGen: 0,
    reportGen: 0,
  },
  geolocation: {
    items: [],
    summary: {
      managed_devices: 0,
      online_devices: 0,
      countries: 0,
      cities: 0,
    },
    q: "",
    status: "",
    country: "",
    city: "",
    isp: "",
    loading: false,
    error: null,
    selectedId: null,
    selected: null,
    refreshTimer: null,
    lastFingerprint: "",
    fetchedAt: 0,
    queryKey: "",
    loadGen: 0,
    map: null,
    markers: [],
  },
};

const views = document.querySelectorAll(".view");
const navLinks = document.querySelectorAll(".nav-link");
let routingFromHash = false;
let applicationActivityTimeDebounce = null;

function isCacheFresh(fetchedAt, ttl = LIST_CACHE_TTL_MS) {
  return Boolean(fetchedAt) && Date.now() - fetchedAt < ttl;
}

function stableQueryKey(value) {
  return JSON.stringify(value);
}

function fleetQueryParams() {
  const f = state.fleet;
  return {
    q: f.q,
    status: f.status,
    tag: f.tag,
    limit: f.limit,
    offset: f.offset,
    sort: f.sort,
  };
}

function fleetQueryKey() {
  return stableQueryKey(fleetQueryParams());
}

function renderFleetFromCache() {
  renderFleetSummary();
  renderFleetStatus();
  renderFleetTable();
  renderFleetPagination();
  updateFleetSortIndicators();
}

function manageSessionDurationTicker() {
  const active =
    state.view === "fleet-detail" &&
    state.fleet.tab === "sessions" &&
    Boolean(document.querySelector("[data-session-duration-live]"));
  if (active) startSessionDurationTicker();
  else stopSessionDurationTicker();
}

function hashPath() {
  const hash = String(window.location.hash || "");
  const path = hash.split("?")[0];
  return path || "#/fleet";
}

const REMOVED_NAV_VIEWS = new Set(["alerts", "incidents", "policies", "administration", "groups"]);
const REMOVED_FLEET_TABS = new Set([
  "alerts",
  "detections",
  "response",
  "threat-timeline",
  "health",
  "timeline",
  "processes",
  "services",
  "network",
  "policies",
]);

function normalizeFleetTab(tab) {
  if (REMOVED_FLEET_TABS.has(tab)) return "overview";
  return tab || "overview";
}

function hashForView(view) {
  if (view === "fleet-detail") return null;
  if (REMOVED_NAV_VIEWS.has(view)) return "#/fleet";
  if (view === "reports") return "#/productivity";
  if (view === "fleet") return "#/fleet";
  return `#/${view}`;
}

function primaryNavView(view) {
  if (view === "fleet-detail") return "fleet";
  if (view === "users" || view === "software" || view === "productivity" || view === "reports") {
    return "reports";
  }
  return view;
}

function setView(view, options = {}) {
  if (REMOVED_NAV_VIEWS.has(view)) {
    view = "fleet";
  }
  if (view === "reports") {
    view = "productivity";
  }
  if (!document.getElementById(`view-${view}`)) return;
  const force = options.force === true;
  const viewChanged = state.view !== view;
  state.view = view;
  views.forEach((section) => {
    section.classList.toggle("hidden", section.id !== `view-${view}`);
  });
  const navView = primaryNavView(view);
  navLinks.forEach((link) => {
    link.classList.toggle("nav-link-active", link.dataset.view === navView);
  });
  if (view === "fleet" || view === "dashboard") {
    loadFleet({ force, silent: !viewChanged && !force });
  }
  if (view === "users") {
    loadUsers({ force, silent: !viewChanged && !force });
  }
  if (view === "software") {
    loadSoftware({ force, silent: !viewChanged && !force });
  }
  if (view === "productivity") {
    loadProductivityPage({ force, silent: !viewChanged && !force });
  }
  if (view === "geolocation") {
    loadGeolocationPage({ force, silent: !viewChanged && !force });
  } else {
    stopGeolocationAutoRefresh();
  }
  if (view !== "fleet-detail" || state.fleet.tab !== "overview") {
    stopHealthAutoRefresh();
  }
  manageSessionDurationTicker();
}

function formatRelativeLastSeen(value) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  const seconds = Math.max(0, Math.floor((Date.now() - date.getTime()) / 1000));
  if (seconds < 45) return "Just now";
  if (seconds < 3600) {
    const minutes = Math.max(1, Math.floor(seconds / 60));
    return `${minutes} minute${minutes === 1 ? "" : "s"} ago`;
  }
  if (seconds < 86400) {
    const hours = Math.max(1, Math.floor(seconds / 3600));
    return `${hours} hour${hours === 1 ? "" : "s"} ago`;
  }
  return formatTime(value);
}

function formatTime(value) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return date.toLocaleString();
}

function formatNumber(value) {
  if (value === null || value === undefined) return "—";
  return Number(value).toLocaleString();
}

function formatBytes(value) {
  const n = Number(value);
  if (!Number.isFinite(n) || n < 0) return "—";
  if (n === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  let size = n;
  let idx = 0;
  while (size >= 1024 && idx < units.length - 1) {
    size /= 1024;
    idx += 1;
  }
  return `${size.toFixed(size >= 10 || idx === 0 ? 0 : 1)} ${units[idx]}`;
}

function formatPercent(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return "—";
  return `${n.toFixed(n >= 10 ? 0 : 1)}%`;
}

function healthUsageBar(percent) {
  const width = Math.max(0, Math.min(100, Number(percent) || 0));
  return `<div class="health-usage-track" aria-hidden="true">
    <div class="health-usage-fill" style="width:${width}%"></div>
  </div>`;
}

function formatSystemUptime(health, item) {
  const seconds =
    health?.uptime_seconds ?? health?.system_uptime_seconds ?? item?.uptime_seconds ?? null;
  if (seconds != null && Number.isFinite(Number(seconds))) {
    return formatDurationSeconds(Number(seconds));
  }
  const boot = health?.boot_time || item?.boot_time;
  if (!boot) return null;
  const start = new Date(boot);
  if (Number.isNaN(start.getTime())) return null;
  return formatDurationSeconds(Math.max(0, Math.floor((Date.now() - start.getTime()) / 1000)));
}

function renderCpuUsageCard(health) {
  if (health?.cpu_percent == null) {
    return `<p class="text-sm text-mist-400">Waiting for the next agent heartbeat with CPU metrics.</p>`;
  }
  return `<div class="health-usage-row">
      ${healthUsageBar(health.cpu_percent)}
      <div class="health-usage-caption">${escapeHtml(formatPercent(health.cpu_percent))}</div>
    </div>`;
}

function renderMemoryUsageCard(health) {
  if (!health?.memory) {
    return `<p class="text-sm text-mist-400">Memory metrics not reported yet.</p>`;
  }
  const m = health.memory;
  return `<div class="health-usage-row">
      ${healthUsageBar(m.percent)}
      <div class="health-usage-caption">
        ${escapeHtml(formatBytes(m.used))} / ${escapeHtml(formatBytes(m.total))}
        (${escapeHtml(formatPercent(m.percent))})
      </div>
    </div>`;
}

function renderDiskUsageCard(health, item) {
  const disks = Array.isArray(health?.disks) ? health.disks : [];
  if (disks.length) {
    return disks
      .map((disk) => {
        const name = disk.name || "Disk";
        return `<article class="health-disk-card">
          <h3 class="health-section-title">Disk ${escapeHtml(name)}</h3>
          <div class="health-usage-row">
            ${healthUsageBar(disk.percent)}
            <div class="health-usage-caption">
              ${escapeHtml(formatBytes(disk.used))} / ${escapeHtml(formatBytes(disk.total))}
              (${escapeHtml(formatPercent(disk.percent))})
            </div>
          </div>
        </article>`;
      })
      .join("");
  }
  if (item?.disk_total_bytes != null) {
    const total = Number(item.disk_total_bytes);
    const free = Number(item.disk_free_bytes);
    const used = Number.isFinite(total) && Number.isFinite(free) ? Math.max(0, total - free) : null;
    const percent =
      Number.isFinite(total) && total > 0 && used != null ? (used / total) * 100 : null;
    return `<div class="health-usage-row">
        ${healthUsageBar(percent)}
        <div class="health-usage-caption">
          ${escapeHtml(formatBytes(used))} / ${escapeHtml(formatBytes(total))}
          (${escapeHtml(formatPercent(percent))})
        </div>
      </div>`;
  }
  return `<p class="text-sm text-mist-400">Disk metrics not reported yet.</p>`;
}

/** Health cards reused on Computer Overview (formerly the Health tab). */
function renderOverviewHealthSection(health, item, { loading = false, error = null } = {}) {
  if (error && !health) {
    return `<p class="device-error">${escapeHtml(error)}</p>`;
  }
  if (loading && !health) {
    return `<p class="text-mist-400">Loading health…</p>`;
  }
  if (!health) {
    return `<p class="text-mist-400">No health data.</p>`;
  }

  const uptime = formatSystemUptime(health, item);
  const metaRows = [
    ["Agent Status", presenceLabel(health.status)],
    ["Last Heartbeat", item?.last_seen ? formatRelativeLastSeen(item.last_seen) : formatHeartbeatAge(health.heartbeat_age_seconds)],
    ["Last Communication", formatTime(health.last_communication_at)],
    ["Last Inventory", formatTime(health.last_inventory_at)],
  ];
  if (uptime) {
    metaRows.push(["System Uptime", uptime]);
  }

  return `<div class="health-panel space-y-4">
    <div class="overview-health-meta">${definitionListHtml(metaRows)}</div>
    <div class="overview-health-grid">
      <section class="health-card">
        <h2 class="health-section-heading">CPU Usage</h2>
        ${renderCpuUsageCard(health)}
      </section>
      <section class="health-card">
        <h2 class="health-section-heading">Memory Usage</h2>
        ${renderMemoryUsageCard(health)}
      </section>
      <section class="health-card overview-health-disk">
        <h2 class="health-section-heading">Disk Usage</h2>
        <div class="space-y-3">${renderDiskUsageCard(health, item)}</div>
      </section>
    </div>
  </div>`;
}

function formatHeartbeatAge(seconds) {
  const n = Number(seconds);
  if (!Number.isFinite(n) || n < 0) return "—";
  if (n < 45) return "Just now";
  if (n < 3600) {
    const minutes = Math.max(1, Math.floor(n / 60));
    return `${minutes} minute${minutes === 1 ? "" : "s"} ago`;
  }
  return formatDurationSeconds(n);
}

function stopHealthAutoRefresh() {
  const f = state.fleet;
  if (f.healthRefreshTimer) {
    clearInterval(f.healthRefreshTimer);
    f.healthRefreshTimer = null;
  }
}

function manageHealthAutoRefresh() {
  if (state.view !== "fleet-detail" || state.fleet.tab !== "overview" || !state.fleet.selectedId) {
    stopHealthAutoRefresh();
    return;
  }
  if (state.fleet.healthRefreshTimer) return;
  state.fleet.healthRefreshTimer = setInterval(() => {
    if (state.view === "fleet-detail" && state.fleet.tab === "overview" && state.fleet.selectedId) {
      refreshOverviewHealth({ silent: true });
    } else {
      stopHealthAutoRefresh();
    }
  }, 8000);
}

async function refreshOverviewHealth({ silent = false } = {}) {
  const f = state.fleet;
  if (!f.selectedId || f.tab !== "overview" || state.view !== "fleet-detail") return;
  if (f.healthLoading) return;
  const requestAgentId = f.selectedId;
  const gen = ++f.extraLoadGen;
  f.healthLoading = true;
  if (!silent || !f.health) {
    renderFleetDetail();
  }
  try {
    const health = await fetchJson(
      platformUrl(`/api/v1/fleet/${encodeURIComponent(requestAgentId)}/health`)
    );
    if (gen !== f.extraLoadGen || f.selectedId !== requestAgentId || f.tab !== "overview") return;
    f.health = health;
    f.healthError = null;
  } catch (error) {
    if (gen !== f.extraLoadGen || f.selectedId !== requestAgentId || f.tab !== "overview") return;
    f.healthError = error.message || "Failed to load health.";
  } finally {
    if (gen !== f.extraLoadGen) return;
    f.healthLoading = false;
    if (f.selectedId !== requestAgentId || f.tab !== "overview") return;
    renderFleetDetail();
    manageHealthAutoRefresh();
  }
}

function escapeHtml(value) {
  return String(value ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function displayValue(value) {
  if (value === null || value === undefined || value === "") return "—";
  return escapeHtml(value);
}

function formatPresenceStatus(status) {
  const key = String(status || "offline").toLowerCase();
  if (key === "online") {
    return `<span class="device-presence device-presence-online" title="Online">🟢 Online</span>`;
  }
  if (key === "warning" || key === "stale") {
    return `<span class="device-presence device-presence-warning" title="Warning">🟡 Warning</span>`;
  }
  if (key === "never_seen") {
    return `<span class="device-presence device-presence-never-seen" title="Never seen">Never seen</span>`;
  }
  return `<span class="device-presence device-presence-offline" title="Offline">🔴 Offline</span>`;
}

function presenceLabel(status) {
  const key = String(status || "offline").toLowerCase();
  if (key === "online") return "🟢 Online";
  if (key === "warning" || key === "stale") return "🟡 Warning";
  if (key === "never_seen") return "Never seen";
  return "🔴 Offline";
}

function parseApiError(payload, status, timedOut) {
  if (timedOut) return "Request timed out waiting for the platform API.";
  if (status === 0) return "Backend unavailable. Could not reach the RiskLens platform API.";
  const err = payload?.error;
  if (typeof err === "string") return err;
  if (err && typeof err === "object") {
    return err.message || err.code || `Request failed (${status})`;
  }
  const detail = payload?.detail;
  if (typeof detail === "string") return detail;
  if (detail?.error) return typeof detail.error === "string" ? detail.error : detail.error.message || detail.hint;
  if (payload?.message) return payload.message;
  return `Request failed (${status})`;
}

async function fetchJson(url, { timeoutMs = API_TIMEOUT_MS } = {}) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  let timedOut = false;
  try {
    const response = await fetch(url, {
      signal: controller.signal,
      headers: { Accept: "application/json" },
    });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      throw Object.assign(new Error(parseApiError(payload, response.status, false)), {
        status: response.status,
        payload,
      });
    }
    return payload;
  } catch (error) {
    if (error.name === "AbortError") {
      timedOut = true;
      throw Object.assign(new Error(parseApiError({}, 0, true)), { timedOut: true, status: 0 });
    }
    if (error.status !== undefined) throw error;
    throw Object.assign(
      new Error(parseApiError({}, 0, timedOut) || "Backend unavailable. Could not reach the RiskLens platform API."),
      { status: 0, cause: error }
    );
  } finally {
    clearTimeout(timer);
  }
}

function platformUrl(path, params) {
  const url = new URL(`${PLATFORM_API_BASE}${path}`);
  if (params) {
    Object.entries(params).forEach(([key, value]) => {
      if (value === null || value === undefined || value === "") return;
      url.searchParams.set(key, String(value));
    });
  }
  return url.toString();
}

// ---------------------------------------------------------------------------
// Fleet (Sprint 13) — GET /api/v1/fleet  (Sprint 12 Users/Software pages unchanged)
// ---------------------------------------------------------------------------

function fleetSortParam(field) {
  const current = state.fleet.sort;
  if (current === field) return `-${field}`;
  if (current === `-${field}`) return field;
  return field;
}

function updateFleetSortIndicators() {
  const sort = state.fleet.sort || "hostname";
  const field = sort.startsWith("-") ? sort.slice(1) : sort;
  const descending = sort.startsWith("-");
  const labels = {
    hostname: "Hostname",
    current_user: "Current User",
    operating_system: "Operating System",
    online_status: "Status",
    last_seen: "Last Seen",
    policy_name: "Policy",
  };
  document.querySelectorAll(".fleet-sort").forEach((button) => {
    const key = button.dataset.sort;
    const base = labels[key] || key;
    const active = key === field;
    button.textContent = active ? `${base} ${descending ? "↓" : "↑"}` : base;
    button.classList.toggle("device-sort-active", active);
  });
}

function renderFleetSummary() {
  const summary = state.fleet.summary || {};
  const cards = {
    "fleet-card-total": summary.total_agents,
    "fleet-card-online": summary.online,
    "fleet-card-offline": summary.offline,
    "fleet-card-warning": summary.warning,
    "fleet-card-never-seen": summary.never_seen,
    "fleet-card-healthy": summary.healthy,
  };
  Object.entries(cards).forEach(([id, value]) => {
    const node = document.getElementById(id);
    if (!node) return;
    node.textContent = state.fleet.loading ? "—" : formatNumber(value ?? 0);
  });
  const dash = {
    "dash-card-total": state.fleet.summary.total_agents,
    "dash-card-online": state.fleet.summary.online,
    "dash-card-offline": state.fleet.summary.offline,
    "dash-card-warning": state.fleet.summary.warning,
  };
  Object.entries(dash).forEach(([id, value]) => {
    const node = document.getElementById(id);
    if (!node) return;
    node.textContent = state.fleet.loading ? "—" : formatNumber(value ?? 0);
  });
}

function renderFleetStatus() {
  const node = document.getElementById("fleet-status");
  const f = state.fleet;
  if (f.loading) {
    node.textContent = "Loading fleet…";
    return;
  }
  if (f.error) {
    node.textContent = f.error;
    return;
  }
  node.textContent = `Showing ${f.items.length} of ${f.total} agent(s) · sort ${f.sort}`;
}

function formatTags(tags) {
  if (!Array.isArray(tags) || tags.length === 0) return "—";
  return escapeHtml(tags.join(", "));
}

function renderFleetTable() {
  const body = document.getElementById("fleet-body");
  const f = state.fleet;

  if (f.loading) {
    body.innerHTML = `<tr><td colspan="8" class="py-8 text-center text-mist-400">Loading fleet…</td></tr>`;
    return;
  }
  if (f.error) {
    body.innerHTML = `<tr><td colspan="8" class="device-error">${escapeHtml(f.error)}</td></tr>`;
    return;
  }
  if (!f.items.length) {
    body.innerHTML = `<tr><td colspan="8" class="device-empty">No agents found. Register an agent, or clear filters.</td></tr>`;
    return;
  }

  body.innerHTML = f.items
    .map((item) => {
      const selected = item.agent_id === f.selectedId ? "device-row-selected" : "";
      return `<tr class="device-row ${selected}" data-agent-id="${escapeHtml(item.agent_id)}" tabindex="0">
        <td class="py-2.5 pr-3">${displayValue(item.hostname || item.machine_name)}</td>
        <td class="py-2.5 pr-3 font-mono text-xs">${displayValue(item.current_user)}</td>
        <td class="py-2.5 pr-3">${displayValue(item.operating_system)}</td>
        <td class="py-2.5 pr-3">${formatPresenceStatus(item.online_status)}</td>
        <td class="py-2.5 pr-3 whitespace-nowrap font-mono text-xs">${formatTime(item.last_seen)}</td>
        <td class="py-2.5 pr-3">${displayValue(item.policy_name)}</td>
        <td class="py-2.5 pr-3">—</td>
        <td class="py-2.5">${formatTags(item.tags)}</td>
      </tr>`;
    })
    .join("");
}

function renderFleetPagination() {
  const f = state.fleet;
  const info = document.getElementById("fleet-page-info");
  const prev = document.getElementById("fleet-prev");
  const next = document.getElementById("fleet-next");
  const start = f.total === 0 ? 0 : f.offset + 1;
  const end = Math.min(f.offset + f.limit, f.total);
  info.textContent = f.total ? `Rows ${start}–${end} of ${f.total}` : "No rows";
  prev.disabled = f.loading || f.offset <= 0;
  next.disabled = f.loading || f.offset + f.limit >= f.total;
}

function fleetAgentIdFromHash() {
  const match = hashPath().match(/^#\/fleet\/([0-9a-fA-F-]{36})$/i);
  return match ? match[1] : null;
}

function appViewFromHash() {
  const path = hashPath();
  const match = path.match(/^#\/([^/]+)$/);
  return match ? match[1] : null;
}

function navigateToView(view) {
  const next = hashForView(view);
  if (!next) {
    setView(view);
    return;
  }
  if (hashPath() !== next) {
    window.location.hash = next;
    return;
  }
  if (state.view !== view) {
    setView(view);
  }
}

function applyAppHash() {
  if (routingFromHash) return false;
  routingFromHash = true;
  try {
    const agentId = fleetAgentIdFromHash();
    if (agentId) {
      loadFleetDetail(agentId);
      return true;
    }
    const view = appViewFromHash();
    if (view && REMOVED_NAV_VIEWS.has(view)) {
      window.location.hash = "#/fleet";
      return true;
    }
    if (view === "reports") {
      window.location.hash = "#/productivity";
      return true;
    }
    if (view && document.getElementById(`view-${view}`)) {
      if (state.view !== view) {
        setView(view);
      }
      return true;
    }
    if (hashPath() === "#/fleet" || !window.location.hash) {
      if (state.view !== "fleet") {
        setView("fleet");
      }
      return true;
    }
    return false;
  } finally {
    routingFromHash = false;
  }
}

function renderEndpointOverview(item) {
  const f = state.fleet;
  const health = f.health;
  const storageLabel = (() => {
    const disks = Array.isArray(health?.disks) ? health.disks : [];
    if (disks.length) {
      return disks
        .map((disk) => {
          const name = disk.name ? `${disk.name} ` : "";
          return `${name}${formatBytes(disk.total)}`.trim();
        })
        .join(", ");
    }
    return formatBytes(item.disk_total_bytes);
  })();

  const memoryLabel =
    health?.memory?.total != null ? formatBytes(health.memory.total) : formatBytes(item.memory_bytes);

  const cpuModel = health?.cpu_model || item.cpu_model || "";

  const computerInfo = definitionListHtml([
    ["Hostname", item.hostname || item.machine_name],
    ["User", item.current_user],
    ["Operating System", item.operating_system || item.os_version],
    ["Agent Version", item.agent_version],
    ["Last Seen", formatTime(item.last_seen)],
    ["Online Status", { html: formatPresenceStatus(item.online_status) }],
  ]);

  const hardware = definitionListHtml([
    ["CPU Model", cpuModel],
    ["Memory", memoryLabel],
    ["Storage", storageLabel],
    ["Manufacturer", item.manufacturer],
    ["Model", item.model],
  ]);

  const network = definitionListHtml([
    ["Public IP", item.ip_address || item.public_ip],
    ["Private IP", item.private_ip],
    ["MAC Address", item.mac_address],
    ["Geo Location", item.geo_location || item.location || formatGeoSummary(item)],
  ]);

  return `<div class="fleet-overview space-y-6">
    <section class="fleet-overview-section">
      <h2 class="fleet-overview-heading">Computer Information</h2>
      ${computerInfo}
    </section>
    <section class="fleet-overview-section">
      <h2 class="fleet-overview-heading">Health</h2>
      ${renderOverviewHealthSection(health, item, {
        loading: f.healthLoading,
        error: f.healthError,
      })}
    </section>
    <section class="fleet-overview-section">
      <h2 class="fleet-overview-heading">Hardware</h2>
      ${hardware}
    </section>
    <section class="fleet-overview-section">
      <h2 class="fleet-overview-heading">Network</h2>
      ${network}
    </section>
  </div>`;
}

function formatGeoSummary(item) {
  const parts = [item.city, item.region, item.country].filter(Boolean);
  return parts.length ? parts.join(", ") : "";
}

function formatBrowserTimeSpent(seconds) {
  const n = Number(seconds);
  if (!Number.isFinite(n) || n < 0) return "—";
  const total = Math.floor(n);
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const secs = total % 60;
  if (hours) return `${hours}h ${String(minutes).padStart(2, "0")}m`;
  if (minutes) return `${minutes}m`;
  return `${secs}s`;
}

function formatBrowserStatus(status) {
  const key = String(status || "").toLowerCase();
  if (key === "active") return "Active";
  if (key === "inactive") return "Inactive";
  return displayValue(status);
}

function formatDurationSeconds(seconds) {
  const n = Number(seconds);
  if (!Number.isFinite(n) || n < 0) return "—";
  const total = Math.floor(n);
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const secs = total % 60;
  if (hours > 0) return `${hours}h ${minutes}m`;
  if (minutes > 0) return `${minutes}m ${secs}s`;
  return `${secs}s`;
}

function formatSessionStatus(status) {
  const key = String(status || "").toLowerCase();
  if (key === "active") return "Active";
  if (key === "closed") return "Closed";
  return displayValue(status);
}

function startSessionDurationTicker() {
  if (window.__risklensSessionTicker) return;
  window.__risklensSessionTicker = setInterval(() => {
    if (state.view !== "fleet-detail" || state.fleet.tab !== "sessions") return;
    document.querySelectorAll("[data-session-duration-live]").forEach((el) => {
      const logon = el.getAttribute("data-logon-at");
      el.textContent = formatDurationSeconds(liveDurationSeconds(logon, null, 0));
    });
  }, 1000);
}

function stopSessionDurationTicker() {
  if (!window.__risklensSessionTicker) return;
  clearInterval(window.__risklensSessionTicker);
  window.__risklensSessionTicker = null;
}

function liveDurationSeconds(startedAt, stoppedAt, storedSeconds) {
  if (stoppedAt) {
    const start = new Date(startedAt);
    const end = new Date(stoppedAt);
    if (!Number.isNaN(start.getTime()) && !Number.isNaN(end.getTime())) {
      return Math.max(0, Math.floor((end.getTime() - start.getTime()) / 1000));
    }
    return Number(storedSeconds) || 0;
  }
  const start = new Date(startedAt);
  if (Number.isNaN(start.getTime())) return Number(storedSeconds) || 0;
  return Math.max(0, Math.floor((Date.now() - start.getTime()) / 1000));
}

function renderFleetSessionsTable(page) {
  const items = page?.items || [];
  const activeCount = items.filter((item) => String(item.status || "").toLowerCase() === "active").length;
  const online = state.fleet.detail?.online_status === "online";
  let notice =
    '<p class="text-xs text-mist-400 mb-3">Sessions are not filtered by the Global Time Filter. They show all logon history reported by the agent.</p>';
  if (online && !activeCount) {
    notice +=
      '<p class="text-xs text-amber-300/90 mb-3">This computer is online but has no active session record. Historical sessions below may still appear. Restart the agent if a current session is missing.</p>';
  }
  if (!items.length) {
    return `${notice}<p class="device-empty">No sessions reported for this computer yet.</p>`;
  }
  const rows = items
    .map((item) => {
      const closed = Boolean(item.logoff_at) || String(item.status || "").toLowerCase() === "closed";
      const duration = liveDurationSeconds(item.logon_at, closed ? item.logoff_at : null, item.duration_seconds);
      const durationAttr = closed
        ? ""
        : ` data-session-duration-live data-logon-at="${escapeHtml(item.logon_at || "")}"`;
      return `<tr>
        <td class="py-2 pr-3">${displayValue(item.machine_name)}</td>
        <td class="py-2 pr-3 font-mono text-xs">${displayValue(item.username)}</td>
        <td class="py-2 pr-3 whitespace-nowrap font-mono text-xs">${formatTime(item.logon_at)}</td>
        <td class="py-2 pr-3 whitespace-nowrap font-mono text-xs">${closed ? formatTime(item.logoff_at) : "—"}</td>
        <td class="py-2 pr-3"${durationAttr}>${escapeHtml(formatDurationSeconds(duration))}</td>
        <td class="py-2">${formatSessionStatus(item.status)}</td>
      </tr>`;
    })
    .join("");
  startSessionDurationTicker();
  return `${notice}<p class="text-mist-400 mb-3">Computer → Session → Account → Application Activity</p>
    <table class="w-full text-sm"><thead class="text-left text-mist-400 text-xs uppercase tracking-wide"><tr>
    <th class="py-2 pr-3">Computer</th><th class="py-2 pr-3">Account</th>
    <th class="py-2 pr-3">Login Time</th><th class="py-2 pr-3">Logout Time</th>
    <th class="py-2 pr-3">Duration</th><th class="py-2">Status</th>
    </tr></thead><tbody class="divide-y divide-ink-600">${rows}</tbody></table>
    <p class="text-xs text-mist-400 mt-3">${items.length} of ${page.total} session(s)</p>`;
}

function renderFleetApplicationActivityTable(page) {
  const items = page?.items || [];
  if (!items.length) {
    return `${activityTimeRangeNotice()}<p class="device-empty">No application activity during ${escapeHtml(
      globalTimeFilterLabel()
    )}. This computer may still have older activity — try Last 24 Hours or Last 7 Days.</p>`;
  }
  const rows = items
    .map((item) => {
      const duration = liveDurationSeconds(item.started_at, item.stopped_at, item.duration_seconds);
      return `<tr>
        <td class="py-2 pr-3">${displayValue(item.application_name)}</td>
        <td class="py-2 pr-3 font-mono text-xs">${displayValue(item.executable_name)}</td>
        <td class="py-2 pr-3">${displayValue(item.window_title)}</td>
        <td class="py-2 pr-3 whitespace-nowrap font-mono text-xs">${formatTime(item.started_at)}</td>
        <td class="py-2 pr-3 whitespace-nowrap font-mono text-xs">${formatTime(item.stopped_at)}</td>
        <td class="py-2 pr-3">${escapeHtml(formatDurationSeconds(duration))}</td>
        <td class="py-2 font-mono text-xs">${displayValue(item.username)}</td>
      </tr>`;
    })
    .join("");
  return `${activityTimeRangeNotice()}<table class="w-full text-sm"><thead class="text-left text-mist-400 text-xs uppercase tracking-wide"><tr>
    <th class="py-2 pr-3">Application</th><th class="py-2 pr-3">Executable</th>
    <th class="py-2 pr-3">Window Title</th><th class="py-2 pr-3">Started</th>
    <th class="py-2 pr-3">Stopped</th><th class="py-2 pr-3">Duration</th>
    <th class="py-2">Session account</th></tr></thead><tbody class="divide-y divide-ink-600">${rows}</tbody></table>
    <p class="text-xs text-mist-400 mt-3">${items.length} of ${page.total} activity row(s)</p>`;
}

function formatBrowserClock(value) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hour12: false });
}

function browserWebsite(value) {
  const text = String(value || "").trim();
  return text || "Unknown Website";
}

function firstBrowserTitle(...values) {
  for (const value of values) {
    const text = String(value || "").trim();
    if (text && text !== "—" && text !== "-") return text;
  }
  return "";
}

function flattenBrowserSessions(items) {
  const sessions = [];
  for (const item of items || []) {
    const parentWebsite = browserWebsite(item.website);
    const timeline = Array.isArray(item.timeline) ? item.timeline : [];
    if (timeline.length) {
      for (const visit of timeline) {
        sessions.push({
          website: browserWebsite(visit.website || parentWebsite),
          page_title: firstBrowserTitle(
            visit.page_title,
            visit.window_title,
            visit.title,
            item.page_title,
            item.window_title
          ),
          started_at: visit.started_at,
          stopped_at: visit.stopped_at,
          duration_seconds: visit.duration_seconds,
        });
      }
    } else if (item.started_at) {
      sessions.push({
        website: parentWebsite,
        page_title: firstBrowserTitle(item.page_title, item.window_title, item.title),
        started_at: item.started_at,
        stopped_at: item.stopped_at,
        duration_seconds: item.duration_seconds,
      });
    }
  }
  return sessions;
}

function filterBrowserSessions(sessions, selectedWebsite) {
  if (!selectedWebsite) return [];
  const selected = browserWebsite(selectedWebsite).toLowerCase();
  return sessions.filter((row) => browserWebsite(row.website).toLowerCase() === selected);
}

function groupBrowserSummary(sessions) {
  const groups = new Map();
  for (const row of sessions) {
    const website = browserWebsite(row.website);
    let group = groups.get(website);
    if (!group) {
      group = { website, duration_seconds: 0, visits: 0 };
      groups.set(website, group);
    }
    group.duration_seconds += liveDurationSeconds(row.started_at, row.stopped_at, row.duration_seconds);
    group.visits += 1;
  }
  return [...groups.values()]
    .map((group) => ({ ...group, total_time: formatBrowserTimeSpent(group.duration_seconds) }))
    .sort((a, b) => b.duration_seconds - a.duration_seconds);
}

function browserVisitsForWebsite(items, selectedWebsite) {
  if (!selectedWebsite) return [];
  const selected = browserWebsite(selectedWebsite).toLowerCase();
  const visits = [];
  for (const item of items || []) {
    if (browserWebsite(item.website).toLowerCase() !== selected) continue;
    const timeline = Array.isArray(item.timeline) ? item.timeline : [];
    if (timeline.length) {
      visits.push(...timeline);
      continue;
    }
    if (item.started_at) {
      visits.push({
        page_title: firstBrowserTitle(item.page_title, item.window_title, item.title),
        started_at: item.started_at,
        stopped_at: item.stopped_at,
        duration_seconds: item.duration_seconds,
      });
    }
  }
  return visits;
}

function renderBrowserSessionTable(visits) {
  const rows = [...visits]
    .sort((a, b) => new Date(a.started_at) - new Date(b.started_at))
    .map((visit) => {
      const duration = formatBrowserTimeSpent(
        liveDurationSeconds(visit.started_at, visit.stopped_at, visit.duration_seconds)
      );
      const title = firstBrowserTitle(visit.page_title, visit.window_title, visit.title);
      return `<tr>
        <td class="py-2 pr-3 whitespace-nowrap font-mono text-xs">${escapeHtml(formatBrowserClock(visit.started_at))}</td>
        <td class="py-2 pr-3 whitespace-nowrap font-mono text-xs">${escapeHtml(
          visit.stopped_at ? formatBrowserClock(visit.stopped_at) : ""
        )}</td>
        <td class="py-2 pr-3">${title ? `<span title="${escapeHtml(title)}">${escapeHtml(title)}</span>` : "—"}</td>
        <td class="py-2">${escapeHtml(duration)}</td>
      </tr>`;
    })
    .join("");
  if (!rows) {
    return `<p class="browser-detail-empty">No browsing sessions for this website during the selected time range.</p>`;
  }
  return `<table class="browser-sessions-table w-full text-sm">
    <thead class="text-left text-mist-400 text-xs uppercase tracking-wide">
      <tr>
        <th class="py-2 pr-3">Start</th>
        <th class="py-2 pr-3">End</th>
        <th class="py-2 pr-3">Page Title</th>
        <th class="py-2">Duration</th>
      </tr>
    </thead>
    <tbody class="divide-y divide-ink-600">${rows}</tbody>
  </table>`;
}

function renderBrowserActivityDetail(selectedWebsite, items) {
  if (!selectedWebsite) {
    return `<section class="browser-detail-section">
      <p class="browser-detail-empty">Select a website to view browsing sessions.</p>
    </section>`;
  }
  const visits = browserVisitsForWebsite(items, selectedWebsite);
  return `<section class="browser-detail-section">
    <h3 class="browser-section-heading">Browsing Sessions — ${escapeHtml(browserWebsite(selectedWebsite))}</h3>
    ${renderBrowserSessionTable(visits)}
  </section>`;
}

function renderBrowserActivityTable(page) {
  const items = page?.items || [];
  const sessions = flattenBrowserSessions(items);
  const grouped = groupBrowserSummary(sessions);
  if (!grouped.length) {
    return `<div class="browser-activity-panel">
      <p class="device-empty">No browser activity during the selected time range. Try widening the Global Time Filter in the header.</p>
    </div>`;
  }
  const selectedWebsite = state.fleet.browserTimelineFilter;
  const summaryRows = grouped
    .map((item) => {
      const active =
        selectedWebsite &&
        browserWebsite(item.website).toLowerCase() === browserWebsite(selectedWebsite).toLowerCase();
      return `<tr class="browser-summary-row${active ? " browser-summary-row-selected" : ""}" data-browser-summary-key="${escapeHtml(
        item.website
      )}" tabindex="0">
        <td class="py-2 pr-3">${displayValue(item.website)}</td>
        <td class="py-2 pr-3">${escapeHtml(item.total_time)}</td>
        <td class="py-2">${escapeHtml(String(item.visits ?? 0))}</td>
      </tr>`;
    })
    .join("");
  return `<div class="browser-activity-panel">
    <section class="browser-summary-section">
      <h3 class="browser-section-heading">Website Summary</h3>
      <table class="browser-summary-table w-full text-sm">
        <thead class="text-left text-mist-400 text-xs uppercase tracking-wide">
          <tr>
            <th class="py-2 pr-3">Website</th>
            <th class="py-2 pr-3">Total Time</th>
            <th class="py-2">Visits</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-ink-600">${summaryRows}</tbody>
      </table>
    </section>
    ${renderBrowserActivityDetail(selectedWebsite, items)}
  </div>`;
}

function renderActivityArchitecture() {
  const rows = [
    ["Current logged in user", "session", "Not collected yet"],
    ["Logon time", "session", "Not collected yet"],
    ["Logoff time", "session", "Not collected yet"],
    ["Current session duration", "session", "Not collected yet"],
    ["Daily active time", "usage", "Not collected yet"],
    ["Application usage", "usage", "Not collected yet"],
    ["Most used applications", "usage", "Not collected yet"],
    ["Foreground application", "usage", "Not collected yet"],
    ["Idle time", "presence", "Not collected yet"],
    ["Lock", "presence", "Not collected yet"],
    ["Unlock", "presence", "Not collected yet"],
    ["Sleep", "power", "Not collected yet"],
    ["Wake", "power", "Not collected yet"],
    ["Shutdown", "power", "Not collected yet"],
    ["Restart", "power", "Not collected yet"],
  ];
  return `<p class="text-mist-400 mb-3">Activity is the future monitoring page for this computer only. Collectors are not implemented in this sprint.</p>
    <table class="w-full text-sm"><thead class="text-left text-mist-400 text-xs uppercase tracking-wide"><tr>
    <th class="py-2 pr-3">Signal</th><th class="py-2 pr-3">Family</th><th class="py-2">Status</th>
    </tr></thead><tbody class="divide-y divide-ink-600">${rows
      .map(
        ([signal, family, status]) =>
          `<tr><td class="py-2 pr-3">${escapeHtml(signal)}</td><td class="py-2 pr-3">${escapeHtml(family)}</td><td class="py-2">${escapeHtml(status)}</td></tr>`
      )
      .join("")}</tbody></table>`;
}

function definitionList(rows) {
  return `<dl class="device-detail-grid">${rows
    .map(([label, value]) => {
      const rendered =
        typeof value === "number" ? formatNumber(value) : displayValue(value);
      return `<dt>${escapeHtml(label)}</dt><dd>${rendered}</dd>`;
    })
    .join("")}</dl>`;
}

function definitionListHtml(rows) {
  return `<dl class="device-detail-grid">${rows
    .map(([label, value]) => {
      let rendered;
      if (value && typeof value === "object" && Object.prototype.hasOwnProperty.call(value, "html")) {
        rendered = value.html;
      } else if (typeof value === "number") {
        rendered = formatNumber(value);
      } else {
        rendered = displayValue(value);
      }
      return `<dt>${escapeHtml(label)}</dt><dd>${rendered}</dd>`;
    })
    .join("")}</dl>`;
}

function renderFleetUsersTable(page) {
  const items = page?.items || [];
  if (!items.length) {
    return `<p class="device-empty">No accounts reported for this agent.</p>`;
  }
  const rows = items
    .map(
      (item) => `<tr>
        <td class="py-2 pr-3 font-mono text-xs">${displayValue(item.username)}</td>
        <td class="py-2 pr-3">${displayValue(item.display_name)}</td>
        <td class="py-2 pr-3">${displayValue(item.account_type)}</td>
        <td class="py-2 pr-3">${item.enabled === true ? "Yes" : item.enabled === false ? "No" : "—"}</td>
        <td class="py-2 pr-3 whitespace-nowrap font-mono text-xs">${formatTime(item.last_logon)}</td>
      </tr>`
    )
    .join("");
  return `<table class="w-full text-sm"><thead class="text-left text-mist-400 text-xs uppercase tracking-wide"><tr>
    <th class="py-2 pr-3">Username</th><th class="py-2 pr-3">Display Name</th>
    <th class="py-2 pr-3">Account Type</th><th class="py-2 pr-3">Enabled</th>
    <th class="py-2">Last Logon</th></tr></thead><tbody class="divide-y divide-ink-600">${rows}</tbody></table>
    <p class="text-xs text-mist-400 mt-3">${items.length} of ${page.total} account(s)</p>`;
}

function renderFleetSoftwareTable(page) {
  const items = page?.items || [];
  if (!items.length) {
    return `<p class="device-empty">No software reported for this agent.</p>`;
  }
  const rows = items
    .map(
      (item) => `<tr>
        <td class="py-2 pr-3">${displayValue(item.application_name)}</td>
        <td class="py-2 pr-3">${displayValue(item.version)}</td>
        <td class="py-2 pr-3">${displayValue(item.publisher)}</td>
        <td class="py-2 pr-3">${displayValue(item.architecture)}</td>
      </tr>`
    )
    .join("");
  return `<table class="w-full text-sm"><thead class="text-left text-mist-400 text-xs uppercase tracking-wide"><tr>
    <th class="py-2 pr-3">Application</th><th class="py-2 pr-3">Version</th>
    <th class="py-2 pr-3">Publisher</th><th class="py-2">Architecture</th>
    </tr></thead><tbody class="divide-y divide-ink-600">${rows}</tbody></table>
    <p class="text-xs text-mist-400 mt-3">${items.length} of ${page.total} application(s)</p>`;
}

function renderFleetDetail() {
  try {
  const f = state.fleet;
  f.tab = normalizeFleetTab(f.tab);
  const title = document.getElementById("fleet-detail-title");
  const subtitle = document.getElementById("fleet-detail-subtitle");
  const errorNode = document.getElementById("fleet-detail-error");
  const body = document.getElementById("fleet-detail-body");
  const item = f.detail;

  title.textContent = item?.hostname || item?.machine_name || "Computer";
  subtitle.textContent = f.selectedId || "";

  document.querySelectorAll(".fleet-nav-item").forEach((button) => {
    button.classList.toggle("fleet-nav-item-active", button.dataset.tab === f.tab);
  });

  if (f.detailError) {
    errorNode.textContent = f.detailError;
    errorNode.classList.remove("hidden");
  } else {
    errorNode.textContent = "";
    errorNode.classList.add("hidden");
  }

  if (f.detailLoading) {
    body.innerHTML = `<p class="text-mist-400">Loading agent…</p>`;
    return;
  }
  if (!item && f.detailError) {
    body.innerHTML = `<p class="device-error">${escapeHtml(f.detailError)}</p>`;
    return;
  }
  if (!item) {
    body.innerHTML = `<p class="text-mist-400">Select an endpoint.</p>`;
    return;
  }

  if (f.tab === "overview") {
    body.innerHTML = renderEndpointOverview(item);
    manageHealthAutoRefresh();
    return;
  }

  if (f.tab === "activity") {
    body.innerHTML = renderActivityArchitecture();
    return;
  }

  if (f.extraLoading && f.extraTab === f.tab) {
    body.innerHTML = `<p class="text-mist-400">Loading…</p>`;
    return;
  }
  if (f.extraError) {
    body.innerHTML = `<p class="device-error">${escapeHtml(f.extraError)}</p>`;
    return;
  }

  if (f.tab === "accounts" || f.tab === "users") {
    body.innerHTML = renderFleetUsersTable(f.users);
    return;
  }
  if (f.tab === "sessions") {
    body.innerHTML = renderFleetSessionsTable(f.sessions);
    return;
  }
  if (f.tab === "applications") {
    body.innerHTML = renderFleetApplicationActivityTable(f.applicationActivity);
    return;
  }
  if (f.tab === "browser") {
    body.innerHTML = renderBrowserActivityTable(f.browserActivity);
    return;
  }
  if (f.tab === "software") {
    body.innerHTML = renderFleetSoftwareTable(f.software);
    return;
  }
  } finally {
    manageSessionDurationTicker();
  }
}

async function loadFleet({ force = false, silent = false } = {}) {
  const f = state.fleet;
  const queryKey = fleetQueryKey();
  if (
    !force &&
    !f.loading &&
    queryKey === f.queryKey &&
    isCacheFresh(f.fetchedAt) &&
    (state.view === "fleet" || state.view === "dashboard")
  ) {
    renderFleetFromCache();
    return;
  }
  if (f.loading && queryKey === f.queryKey) return;

  const gen = ++f.loadGen;
  f.loading = true;
  f.error = null;
  if (!silent) {
    renderFleetFromCache();
  }

  try {
    const payload = await fetchJson(platformUrl("/api/v1/fleet", fleetQueryParams()));
    if (gen !== f.loadGen) return;
    f.items = payload.items || [];
    f.total = Number(payload.total || 0);
    f.limit = Number(payload.limit || f.limit);
    f.offset = Number(payload.offset || 0);
    f.summary = payload.summary || f.summary;
    f.error = null;
    f.queryKey = queryKey;
    f.fetchedAt = Date.now();
  } catch (error) {
    if (gen !== f.loadGen) return;
    f.items = [];
    f.total = 0;
    f.error = error.message || "Failed to load fleet.";
  } finally {
    if (gen !== f.loadGen) return;
    f.loading = false;
    if (state.view === "fleet" || state.view === "dashboard") {
      renderFleetFromCache();
    }
  }
}

function applicationActivityTimeQuery() {
  const api = window.RiskLensGlobalTime;
  if (!api || typeof api.getGlobalTimeStore !== "function") return {};
  const range = api.getGlobalTimeStore().getState();
  return {
    started_at_from: range.startTime,
    started_at_to: range.endTime,
  };
}

function globalTimeFilterLabel() {
  const api = window.RiskLensGlobalTime;
  if (!api || typeof api.getGlobalTimeStore !== "function") return "the selected time range";
  const range = api.getGlobalTimeStore().getState();
  const picker = window.RiskLensGlobalTimePicker;
  if (picker && typeof picker.displayPresetLabel === "function") {
    if (range.preset === "custom") return "Custom Range";
    return picker.displayPresetLabel(range.preset);
  }
  return range.preset || "the selected time range";
}

function activityTimeRangeNotice() {
  return `<p class="text-xs text-mist-400 mb-3">Filtered by Global Time: <strong>${escapeHtml(
    globalTimeFilterLabel()
  )}</strong>. Use the time picker in the header to widen the range if activity looks missing.</p>`;
}

function onApplicationActivityTimeRangeChange() {
  clearTimeout(applicationActivityTimeDebounce);
  applicationActivityTimeDebounce = setTimeout(() => {
    const f = state.fleet;
    f.applicationActivity = null;
    f.browserActivity = null;
    f.browserTimelineFilter = null;
    state.productivity.reportQueryKey = "";
    if (
      state.view === "fleet-detail" &&
      f.selectedId &&
      (f.tab === "applications" || f.tab === "browser")
    ) {
      loadFleetTabData();
    }
    if (state.view === "productivity" && state.productivity.agentId) {
      loadProductivityReport({ force: true });
    }
  }, 250);
}

function bindApplicationActivityTimeFilter() {
  const api = window.RiskLensGlobalTime;
  if (!api || typeof api.getGlobalTimeStore !== "function") return;
  api.getGlobalTimeStore().subscribe(onApplicationActivityTimeRangeChange);
}

async function loadFleetTabData() {
  const f = state.fleet;
  if (!f.selectedId) return;
  f.tab = normalizeFleetTab(f.tab);
  const tab = f.tab;
  if (tab === "activity") {
    renderFleetDetail();
    return;
  }
  if (tab === "overview") {
    renderFleetDetail();
    await refreshOverviewHealth({ silent: Boolean(f.health) });
    return;
  }
  const requestAgentId = f.selectedId;
  const requestTab = tab;
  const gen = ++f.extraLoadGen;
  f.extraTab = tab;
  f.extraLoading = true;
  f.extraError = null;
  renderFleetDetail();
  try {
    if ((tab === "accounts" || tab === "users") && !f.users) {
      f.users = await fetchJson(
        platformUrl(`/api/v1/fleet/${encodeURIComponent(f.selectedId)}/users`, { limit: 200 })
      );
    } else if (tab === "software" && !f.software) {
      f.software = await fetchJson(
        platformUrl(`/api/v1/fleet/${encodeURIComponent(f.selectedId)}/software`, { limit: 200 })
      );
    } else if (tab === "sessions") {
      f.sessions = await fetchJson(
        platformUrl(`/api/v1/fleet/${encodeURIComponent(f.selectedId)}/sessions`, { limit: 200 })
      );
    } else if (tab === "applications") {
      f.applicationActivity = await fetchJson(
        platformUrl(`/api/v1/fleet/${encodeURIComponent(f.selectedId)}/applications`, {
          limit: 200,
          ...applicationActivityTimeQuery(),
        })
      );
    } else if (tab === "browser") {
      f.browserActivity = await fetchJson(
        platformUrl(`/api/v1/fleet/${encodeURIComponent(f.selectedId)}/browser-activity`, {
          limit: 200,
          sort: f.browserSort || "-duration_seconds",
          ...applicationActivityTimeQuery(),
        })
      );
    }
    if (gen !== f.extraLoadGen || f.selectedId !== requestAgentId || f.tab !== requestTab) return;
    f.extraError = null;
  } catch (error) {
    if (gen !== f.extraLoadGen || f.selectedId !== requestAgentId || f.tab !== requestTab) return;
    f.extraError = error.message || "Failed to load tab.";
  } finally {
    if (gen !== f.extraLoadGen) return;
    f.extraLoading = false;
    f.extraTab = null;
    if (f.selectedId !== requestAgentId || f.tab !== requestTab) return;
    renderFleetDetail();
  }
}

async function loadFleetDetail(agentId) {
  const f = state.fleet;
  const requestAgentId = agentId;
  if (
    f.selectedId === agentId &&
    f.detail &&
    !f.detailLoading &&
    !f.detailError &&
    fleetAgentIdFromHash() === requestAgentId
  ) {
    if (state.view !== "fleet-detail") {
      setView("fleet-detail", { force: false });
    }
    loadFleetTabData();
    return;
  }
  const switched = f.selectedId !== agentId;
  f.selectedId = agentId;
  if (switched) {
    f.tab = "overview";
    f.users = null;
    f.software = null;
    f.sessions = null;
    f.applicationActivity = null;
    f.browserActivity = null;
    f.browserTimelineFilter = null;
    f.health = null;
    f.healthError = null;
    f.healthLoading = false;
  }
  f.detailError = null;
  f.detailLoading = true;
  f.extraError = null;
  if (fleetAgentIdFromHash() === requestAgentId) {
    setView("fleet-detail", { force: false });
  }
  renderFleetDetail();

  try {
    f.detail = await fetchJson(platformUrl(`/api/v1/fleet/${encodeURIComponent(agentId)}`));
    if (fleetAgentIdFromHash() !== requestAgentId) return;
    f.detailError = null;
  } catch (error) {
    if (fleetAgentIdFromHash() !== requestAgentId) return;
    f.detail = null;
    f.detailError = error.message || "Failed to load agent.";
  } finally {
    if (fleetAgentIdFromHash() !== requestAgentId) return;
    f.detailLoading = false;
    renderFleetDetail();
    if (!f.detailError) loadFleetTabData();
  }
}

function openFleetAgent(agentId) {
  const next = `#/fleet/${agentId}`;
  if (window.location.hash === next) {
    loadFleetDetail(agentId);
    return;
  }
  window.location.hash = next;
}

function backToFleet() {
  if (hashPath() === "#/fleet") {
    setView("fleet");
    return;
  }
  window.location.hash = "#/fleet";
}

function applyFleetHash() {
  return applyAppHash();
}

navLinks.forEach((link) => {
  link.addEventListener("click", () => {
    navigateToView(link.dataset.view);
  });
});

document.querySelectorAll("[data-open-view]").forEach((button) => {
  button.addEventListener("click", () => {
    const view = button.dataset.openView;
    if (view) navigateToView(view);
  });
});

document.getElementById("global-search-form").addEventListener("submit", (event) => {
  event.preventDefault();
});

document.getElementById("fleet-filters").addEventListener("submit", (event) => {
  event.preventDefault();
  const f = state.fleet;
  f.q = document.getElementById("filter-fleet-q").value.trim();
  f.status = document.getElementById("filter-fleet-status").value;
  f.tag = document.getElementById("filter-fleet-tag").value;
  f.offset = 0;
  loadFleet();
});

document.getElementById("fleet-clear-filters").addEventListener("click", () => {
  document.getElementById("filter-fleet-q").value = "";
  document.getElementById("filter-fleet-status").value = "";
  document.getElementById("filter-fleet-tag").value = "";
  const f = state.fleet;
  f.q = "";
  f.status = "";
  f.tag = "";
  f.offset = 0;
  loadFleet();
});

document.getElementById("fleet-refresh").addEventListener("click", () => {
  loadFleet({ force: true });
});

document.getElementById("fleet-prev").addEventListener("click", () => {
  const f = state.fleet;
  f.offset = Math.max(0, f.offset - f.limit);
  loadFleet();
});

document.getElementById("fleet-next").addEventListener("click", () => {
  const f = state.fleet;
  if (f.offset + f.limit < f.total) {
    f.offset += f.limit;
    loadFleet();
  }
});

document.querySelectorAll(".fleet-sort").forEach((button) => {
  button.addEventListener("click", () => {
    state.fleet.sort = fleetSortParam(button.dataset.sort);
    state.fleet.offset = 0;
    loadFleet();
  });
});

document.getElementById("fleet-body").addEventListener("click", (event) => {
  const row = event.target.closest("tr[data-agent-id]");
  if (!row) return;
  openFleetAgent(row.dataset.agentId);
});

document.getElementById("fleet-body").addEventListener("keydown", (event) => {
  if (event.key !== "Enter" && event.key !== " ") return;
  const row = event.target.closest("tr[data-agent-id]");
  if (!row) return;
  event.preventDefault();
  openFleetAgent(row.dataset.agentId);
});

document.getElementById("fleet-detail-back").addEventListener("click", backToFleet);

document.getElementById("fleet-detail-nav").addEventListener("click", (event) => {
  const button = event.target.closest(".fleet-nav-item");
  if (!button) return;
  state.fleet.tab = button.dataset.tab;
  loadFleetTabData();
});

document.getElementById("fleet-detail-body").addEventListener("click", (event) => {
  const row = event.target.closest("[data-browser-summary-key]");
  if (!row || state.fleet.tab !== "browser") return;
  const key = (row.getAttribute("data-browser-summary-key") || "").trim();
  if (!key) return;
  state.fleet.browserTimelineFilter = key;
  renderFleetDetail();
});

document.getElementById("fleet-detail-body").addEventListener("keydown", (event) => {
  if (event.key !== "Enter" && event.key !== " ") return;
  const row = event.target.closest("[data-browser-summary-key]");
  if (!row || state.fleet.tab !== "browser") return;
  event.preventDefault();
  const key = (row.getAttribute("data-browser-summary-key") || "").trim();
  if (!key) return;
  state.fleet.browserTimelineFilter = key;
  renderFleetDetail();
});

window.addEventListener("hashchange", () => {
  applyAppHash();
});

// ---------------------------------------------------------------------------
// Users (Sprint 12) — unchanged, uses GET /api/v1/users
// ---------------------------------------------------------------------------

function userSortParam(field) {
  const current = state.users.sort;
  if (current === field) return `-${field}`;
  if (current === `-${field}`) return field;
  return field;
}

function updateUserSortIndicators() {
  const sort = state.users.sort || "username";
  const field = sort.startsWith("-") ? sort.slice(1) : sort;
  const descending = sort.startsWith("-");
  const labels = {
    username: "Username",
    machine_name: "Computer",
    enabled: "Status",
    last_logon: "Last Seen",
  };
  document.querySelectorAll(".user-sort").forEach((button) => {
    const key = button.dataset.sort;
    const base = labels[key] || key;
    const active = key === field;
    button.textContent = active ? `${base} ${descending ? "↓" : "↑"}` : base;
    button.classList.toggle("device-sort-active", active);
  });
}

function renderUsersStatus() {
  const node = document.getElementById("users-status");
  const u = state.users;
  if (u.loading) {
    node.textContent = "Loading user inventory…";
    return;
  }
  if (u.error) {
    node.textContent = u.error;
    return;
  }
  node.textContent = `Showing ${u.items.length} of ${u.total} account(s) · sort ${u.sort}`;
}

function renderUsersTable() {
  const body = document.getElementById("users-body");
  const u = state.users;
  if (u.loading) {
    body.innerHTML = `<tr><td colspan="7" class="py-8 text-center text-mist-400">Loading user inventory…</td></tr>`;
    return;
  }
  if (u.error) {
    body.innerHTML = `<tr><td colspan="7" class="device-error">${escapeHtml(u.error)}</td></tr>`;
    return;
  }
  if (!u.items.length) {
    body.innerHTML = `<tr><td colspan="7" class="device-empty">No user accounts found. Enable the agent users module, or clear filters.</td></tr>`;
    return;
  }
  body.innerHTML = u.items
    .map(
      (item) => `<tr class="device-row" data-sid="${escapeHtml(item.sid)}">
        <td class="py-2.5 pr-3">${displayValue(item.username)}</td>
        <td class="py-2.5 pr-3">${displayValue(item.machine_name)}</td>
        <td class="py-2.5 pr-3">—</td>
        <td class="py-2.5 pr-3">—</td>
        <td class="py-2.5 pr-3">—</td>
        <td class="py-2.5 pr-3">${item.enabled ? "Enabled" : "Disabled"}</td>
        <td class="py-2.5 whitespace-nowrap font-mono text-xs">${formatTime(item.last_logon)}</td>
      </tr>`
    )
    .join("");
}

function renderUsersPagination() {
  const u = state.users;
  const info = document.getElementById("users-page-info");
  const prev = document.getElementById("users-prev");
  const next = document.getElementById("users-next");
  const start = u.total === 0 ? 0 : u.offset + 1;
  const end = Math.min(u.offset + u.limit, u.total);
  info.textContent = u.total ? `Rows ${start}–${end} of ${u.total}` : "No rows";
  prev.disabled = u.loading || u.offset <= 0;
  next.disabled = u.loading || u.offset + u.limit >= u.total;
}

async function loadUsers({ force = false, silent = false } = {}) {
  const u = state.users;
  const params = {
    username: u.username,
    account_type: u.account_type,
    limit: u.limit,
    offset: u.offset,
    sort: u.sort,
  };
  if (u.enabled === "true" || u.enabled === "false") {
    params.enabled = u.enabled;
  }
  const queryKey = stableQueryKey(params);
  if (!force && !u.loading && queryKey === u.queryKey && isCacheFresh(u.fetchedAt) && state.view === "users") {
    renderUsersStatus();
    renderUsersTable();
    renderUsersPagination();
    updateUserSortIndicators();
    return;
  }
  if (u.loading && queryKey === u.queryKey) return;

  const gen = ++u.loadGen;
  u.loading = true;
  u.error = null;
  if (!silent) {
    renderUsersStatus();
    renderUsersTable();
    renderUsersPagination();
    updateUserSortIndicators();
  }

  try {
    const payload = await fetchJson(platformUrl("/api/v1/users", params));
    if (gen !== u.loadGen) return;
    u.items = payload.items || [];
    u.total = Number(payload.total || 0);
    u.limit = Number(payload.limit || u.limit);
    u.offset = Number(payload.offset || 0);
    u.error = null;
    u.queryKey = queryKey;
    u.fetchedAt = Date.now();
  } catch (error) {
    if (gen !== u.loadGen) return;
    u.items = [];
    u.total = 0;
    u.error = error.message || "Failed to load users.";
  } finally {
    if (gen !== u.loadGen) return;
    u.loading = false;
    if (state.view === "users") {
      renderUsersStatus();
      renderUsersTable();
      renderUsersPagination();
      updateUserSortIndicators();
    }
  }
}

document.getElementById("users-filters").addEventListener("submit", (event) => {
  event.preventDefault();
  const u = state.users;
  u.username = document.getElementById("filter-user-username").value.trim();
  u.account_type = document.getElementById("filter-user-account-type").value.trim();
  u.enabled = document.getElementById("filter-user-enabled").value;
  u.offset = 0;
  loadUsers();
});

document.getElementById("users-clear-filters").addEventListener("click", () => {
  document.getElementById("filter-user-username").value = "";
  document.getElementById("filter-user-account-type").value = "";
  document.getElementById("filter-user-enabled").value = "";
  const u = state.users;
  u.username = "";
  u.account_type = "";
  u.enabled = "";
  u.offset = 0;
  loadUsers();
});

document.getElementById("users-refresh").addEventListener("click", () => loadUsers({ force: true }));
document.getElementById("users-prev").addEventListener("click", () => {
  const u = state.users;
  u.offset = Math.max(0, u.offset - u.limit);
  loadUsers();
});
document.getElementById("users-next").addEventListener("click", () => {
  const u = state.users;
  if (u.offset + u.limit < u.total) {
    u.offset += u.limit;
    loadUsers();
  }
});
document.querySelectorAll(".user-sort").forEach((button) => {
  button.addEventListener("click", () => {
    state.users.sort = userSortParam(button.dataset.sort);
    state.users.offset = 0;
    loadUsers();
  });
});

// ---------------------------------------------------------------------------
// Software (Sprint 13) — unchanged, uses GET /api/v1/software
// ---------------------------------------------------------------------------

function softwareSortParam(field) {
  const current = state.software.sort;
  if (current === field) return `-${field}`;
  if (current === `-${field}`) return field;
  return field;
}

function updateSoftwareSortIndicators() {
  const sort = state.software.sort || "application_name";
  const field = sort.startsWith("-") ? sort.slice(1) : sort;
  const descending = sort.startsWith("-");
  const labels = {
    application_name: "Application",
    version: "Version",
    publisher: "Publisher",
    machine_name: "Computer",
    collected_at: "Last Seen",
  };
  document.querySelectorAll(".software-sort").forEach((button) => {
    const key = button.dataset.sort;
    const base = labels[key] || key;
    const active = key === field;
    button.textContent = active ? `${base} ${descending ? "↓" : "↑"}` : base;
    button.classList.toggle("device-sort-active", active);
  });
}

function renderSoftwareStatus() {
  const node = document.getElementById("software-status");
  const s = state.software;
  if (s.loading) {
    node.textContent = "Loading installed software…";
    return;
  }
  if (s.error) {
    node.textContent = s.error;
    return;
  }
  node.textContent = `Showing ${s.items.length} of ${s.total} application(s) · sort ${s.sort}`;
}

function renderSoftwareTable() {
  const body = document.getElementById("software-body");
  const s = state.software;
  if (s.loading) {
    body.innerHTML = `<tr><td colspan="6" class="py-8 text-center text-mist-400">Loading installed software…</td></tr>`;
    return;
  }
  if (s.error) {
    body.innerHTML = `<tr><td colspan="6" class="device-error">${escapeHtml(s.error)}</td></tr>`;
    return;
  }
  if (!s.items.length) {
    body.innerHTML = `<tr><td colspan="6" class="device-empty">No installed applications found. Enable the agent software module, or clear filters.</td></tr>`;
    return;
  }
  body.innerHTML = s.items
    .map(
      (item) => `<tr class="device-row" data-software-id="${escapeHtml(item.id)}">
        <td class="py-2.5 pr-3">${displayValue(item.application_name)}</td>
        <td class="py-2.5 pr-3 font-mono text-xs">${displayValue(item.version)}</td>
        <td class="py-2.5 pr-3">${displayValue(item.publisher)}</td>
        <td class="py-2.5 pr-3">${displayValue(item.machine_name)}</td>
        <td class="py-2.5 pr-3">—</td>
        <td class="py-2.5 whitespace-nowrap font-mono text-xs">${formatTime(item.received_at || item.collected_at)}</td>
      </tr>`
    )
    .join("");
}

function renderSoftwarePagination() {
  const s = state.software;
  const info = document.getElementById("software-page-info");
  const prev = document.getElementById("software-prev");
  const next = document.getElementById("software-next");
  const start = s.total === 0 ? 0 : s.offset + 1;
  const end = Math.min(s.offset + s.limit, s.total);
  info.textContent = s.total ? `Rows ${start}–${end} of ${s.total}` : "No rows";
  prev.disabled = s.loading || s.offset <= 0;
  next.disabled = s.loading || s.offset + s.limit >= s.total;
}

async function loadSoftware({ force = false, silent = false } = {}) {
  const s = state.software;
  const params = {
    application_name: s.application_name,
    publisher: s.publisher,
    version: s.version,
    architecture: s.architecture,
    limit: s.limit,
    offset: s.offset,
    sort: s.sort,
  };
  const queryKey = stableQueryKey(params);
  if (!force && !s.loading && queryKey === s.queryKey && isCacheFresh(s.fetchedAt) && state.view === "software") {
    renderSoftwareStatus();
    renderSoftwareTable();
    renderSoftwarePagination();
    updateSoftwareSortIndicators();
    return;
  }
  if (s.loading && queryKey === s.queryKey) return;

  const gen = ++s.loadGen;
  s.loading = true;
  s.error = null;
  if (!silent) {
    renderSoftwareStatus();
    renderSoftwareTable();
    renderSoftwarePagination();
    updateSoftwareSortIndicators();
  }

  try {
    const payload = await fetchJson(platformUrl("/api/v1/software", params));
    if (gen !== s.loadGen) return;
    s.items = payload.items || [];
    s.total = Number(payload.total || 0);
    s.limit = Number(payload.limit || s.limit);
    s.offset = Number(payload.offset || 0);
    s.error = null;
    s.queryKey = queryKey;
    s.fetchedAt = Date.now();
  } catch (error) {
    if (gen !== s.loadGen) return;
    s.items = [];
    s.total = 0;
    s.error = error.message || "Failed to load installed software.";
  } finally {
    if (gen !== s.loadGen) return;
    s.loading = false;
    if (state.view === "software") {
      renderSoftwareStatus();
      renderSoftwareTable();
      renderSoftwarePagination();
      updateSoftwareSortIndicators();
    }
  }
}

document.getElementById("software-filters").addEventListener("submit", (event) => {
  event.preventDefault();
  const s = state.software;
  s.application_name = document.getElementById("filter-software-name").value.trim();
  s.publisher = document.getElementById("filter-software-publisher").value.trim();
  s.version = document.getElementById("filter-software-version").value.trim();
  s.architecture = document.getElementById("filter-software-arch").value;
  s.offset = 0;
  loadSoftware();
});

document.getElementById("software-clear-filters").addEventListener("click", () => {
  document.getElementById("filter-software-name").value = "";
  document.getElementById("filter-software-publisher").value = "";
  document.getElementById("filter-software-version").value = "";
  document.getElementById("filter-software-arch").value = "";
  const s = state.software;
  s.application_name = "";
  s.publisher = "";
  s.version = "";
  s.architecture = "";
  s.offset = 0;
  loadSoftware();
});

document.getElementById("software-refresh").addEventListener("click", () => loadSoftware({ force: true }));
document.getElementById("software-prev").addEventListener("click", () => {
  const s = state.software;
  s.offset = Math.max(0, s.offset - s.limit);
  loadSoftware();
});
document.getElementById("software-next").addEventListener("click", () => {
  const s = state.software;
  if (s.offset + s.limit < s.total) {
    s.offset += s.limit;
    loadSoftware();
  }
});
document.querySelectorAll(".software-sort").forEach((button) => {
  button.addEventListener("click", () => {
    state.software.sort = softwareSortParam(button.dataset.sort);
    state.software.offset = 0;
    loadSoftware();
  });
});

// ---------------------------------------------------------------------------
// Productivity Report (Reports) — uses Global Time Filter + Application Activity
// ---------------------------------------------------------------------------

function renderProductivityAgentOptions() {
  const select = document.getElementById("productivity-agent");
  if (!select) return;
  const p = state.productivity;
  const options = ['<option value="">Select a computer…</option>']
    .concat(
      (p.agents || []).map((agent) => {
        const id = agent.agent_id;
        const label = agent.hostname || agent.machine_name || id;
        const user = agent.current_user ? ` · ${agent.current_user}` : "";
        const selected = id === p.agentId ? " selected" : "";
        return `<option value="${escapeHtml(id)}"${selected}>${escapeHtml(label)}${escapeHtml(user)}</option>`;
      })
    )
    .join("");
  select.innerHTML = options;
}

function renderProductivityStatus() {
  const node = document.getElementById("productivity-status");
  if (!node) return;
  const p = state.productivity;
  if (p.agentsLoading) {
    node.textContent = "Loading computers…";
    return;
  }
  if (p.loading) {
    node.textContent = "Generating productivity report…";
    return;
  }
  if (p.error) {
    node.textContent = p.error;
    return;
  }
  if (!p.agentId) {
    node.textContent = "Choose a computer to generate the report.";
    return;
  }
  const range = applicationActivityTimeQuery();
  const from = range.started_at_from ? formatTime(range.started_at_from) : "—";
  const to = range.started_at_to ? formatTime(range.started_at_to) : "—";
  node.textContent = `Report period ${from} → ${to}`;
}

function ensureProductivityChartStyles() {
  if (document.getElementById("productivity-chart-styles")) return;
  const style = document.createElement("style");
  style.id = "productivity-chart-styles";
  style.textContent = `
    .productivity-bar-row{display:grid;grid-template-columns:minmax(5rem,8rem) 1fr auto;gap:.65rem;align-items:center;margin-bottom:.55rem}
    .productivity-bar-label{font-size:.8rem;color:#e8eef4;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
    .productivity-bar-track{height:.75rem;border-radius:999px;background:#0c1117;border:1px solid #243140;overflow:hidden}
    .productivity-bar-fill{height:100%;border-radius:inherit;background:#3ddc97;min-width:2px}
    .productivity-bar-value{font-size:.75rem;font-family:"IBM Plex Mono",ui-monospace,monospace;color:#9aa8b7;white-space:nowrap}
    .productivity-stack-track{display:flex;height:1rem;border-radius:999px;overflow:hidden;border:1px solid #243140;background:#0c1117}
    .productivity-stack-active{background:#3ddc97;height:100%}
    .productivity-stack-idle{background:#314456;height:100%}
    .productivity-stack-legend{display:flex;flex-wrap:wrap;gap:.85rem 1.25rem;margin-top:.75rem;font-size:.8rem;color:#9aa8b7}
    .productivity-dot{display:inline-block;width:.55rem;height:.55rem;border-radius:999px;margin-right:.4rem;vertical-align:middle}
    .productivity-dot-active{background:#3ddc97}
    .productivity-dot-idle{background:#314456;border:1px solid #4a5d70}
    .productivity-metric-grid{display:grid;grid-template-columns:1fr 1fr;gap:.85rem}
    .productivity-donut-wrap{display:grid;grid-template-columns:auto 1fr;gap:1rem;align-items:center}
    .productivity-donut-chart{position:relative;width:8.5rem;height:8.5rem}
    .productivity-donut-svg{width:100%;height:100%;display:block}
    .productivity-donut-center{position:absolute;inset:0;display:flex;flex-direction:column;align-items:center;justify-content:center;pointer-events:none}
    .productivity-donut-center-label{font-size:.65rem;text-transform:uppercase;letter-spacing:.06em;color:#6f7f90}
    .productivity-donut-center-value{margin-top:.15rem;font-size:1.15rem;font-weight:600;font-family:"IBM Plex Mono",ui-monospace,monospace;color:#e8eef4}
    .productivity-legend{display:flex;flex-direction:column;gap:.4rem;min-width:0}
    .productivity-legend-row{display:grid;grid-template-columns:auto 1fr auto;gap:.45rem;align-items:center;font-size:.75rem}
    .productivity-legend-swatch{width:.55rem;height:.55rem;border-radius:999px;display:inline-block}
    .productivity-legend-label{color:#e8eef4;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
    .productivity-legend-value{color:#9aa8b7;font-family:"IBM Plex Mono",ui-monospace,monospace;white-space:nowrap}
    .productivity-cols{display:grid;grid-template-columns:repeat(6,minmax(0,1fr));gap:.75rem;align-items:end;min-height:10rem}
    .productivity-col{display:flex;flex-direction:column;align-items:center;gap:.35rem;min-width:0}
    .productivity-col-track{width:100%;max-width:2.4rem;height:7rem;border-radius:.4rem .4rem .2rem .2rem;background:#0c1117;border:1px solid #243140;display:flex;align-items:flex-end;overflow:hidden}
    .productivity-col-fill{width:100%;min-height:4px;border-radius:.25rem .25rem 0 0}
    .productivity-col-value{font-size:.7rem;font-family:"IBM Plex Mono",ui-monospace,monospace;color:#9aa8b7}
    .productivity-col-label{width:100%;text-align:center;font-size:.68rem;color:#e8eef4;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
    .productivity-kpi-meta{margin-top:.45rem;font-size:.7rem;color:#6f7f90}
    @media (max-width:900px){
      .productivity-donut-wrap{grid-template-columns:1fr;justify-items:center}
      .productivity-cols{grid-template-columns:repeat(3,minmax(0,1fr))}
    }
  `;
  document.head.appendChild(style);
}

function productivityChartColors() {
  return ["#3ddc97", "#4aa8d8", "#e6b35a", "#7aa2b8", "#d67a5a", "#5ec4a8", "#8b9bb4", "#c4a35a"];
}

function productivityPercent(part, whole) {
  const total = Number(whole) || 0;
  if (total <= 0) return 0;
  return Math.round((Math.max(0, Number(part) || 0) / total) * 1000) / 10;
}

function productivityDonutChart(segments, centerLabel, centerValue) {
  const colors = productivityChartColors();
  const total = segments.reduce((sum, item) => sum + Math.max(0, Number(item.value) || 0), 0);
  if (!total) {
    return `<div class="productivity-donut-empty text-sm text-mist-400">No data for this period.</div>`;
  }
  const radius = 42;
  const circumference = 2 * Math.PI * radius;
  let offset = 0;
  const rings = segments
    .filter((item) => Number(item.value) > 0)
    .slice(0, 8)
    .map((item, index) => {
      const value = Math.max(0, Number(item.value) || 0);
      const length = (value / total) * circumference;
      const dasharray = `${length} ${circumference - length}`;
      const dashoffset = -offset;
      offset += length;
      return `<circle class="productivity-donut-segment" cx="54" cy="54" r="${radius}" fill="transparent"
        stroke="${colors[index % colors.length]}" stroke-width="12"
        stroke-dasharray="${dasharray}" stroke-dashoffset="${dashoffset}"
        transform="rotate(-90 54 54)">
        <title>${escapeHtml(item.label)}: ${escapeHtml(item.display || String(value))}</title>
      </circle>`;
    })
    .join("");
  const legend = segments
    .filter((item) => Number(item.value) > 0)
    .slice(0, 8)
    .map((item, index) => {
      const pct = productivityPercent(item.value, total);
      return `<div class="productivity-legend-row">
        <span class="productivity-legend-swatch" style="background:${colors[index % colors.length]}"></span>
        <span class="productivity-legend-label" title="${escapeHtml(item.label)}">${escapeHtml(item.label)}</span>
        <span class="productivity-legend-value">${escapeHtml(item.display || "")} · ${pct}%</span>
      </div>`;
    })
    .join("");
  return `<div class="productivity-donut-wrap" style="display:grid;grid-template-columns:auto 1fr;gap:1rem;align-items:center">
    <div class="productivity-donut-chart" style="position:relative;width:8.5rem;height:8.5rem">
      <svg viewBox="0 0 108 108" class="productivity-donut-svg" style="width:100%;height:100%;display:block" aria-hidden="true">
        <circle cx="54" cy="54" r="${radius}" fill="transparent" stroke="#0c1117" stroke-width="12"></circle>
        ${rings}
      </svg>
      <div class="productivity-donut-center" style="position:absolute;inset:0;display:flex;flex-direction:column;align-items:center;justify-content:center;pointer-events:none">
        <div class="productivity-donut-center-label">${escapeHtml(centerLabel || "")}</div>
        <div class="productivity-donut-center-value">${escapeHtml(centerValue || "")}</div>
      </div>
    </div>
    <div class="productivity-legend">${legend}</div>
  </div>`;
}

function productivityStackedTimeBar(summary) {
  const active = Math.max(0, Number(summary.active_time_seconds) || 0);
  const idle = Math.max(0, Number(summary.idle_time_seconds) || 0);
  const working = Math.max(active + idle, Number(summary.working_time_seconds) || 0);
  if (!working) {
    return `<p class="text-sm text-mist-400">No time composition for this period.</p>`;
  }
  const activePct = productivityPercent(active, working);
  const idlePct = Math.max(0, Math.round((1000 - activePct * 10)) / 10);
  return `<div class="productivity-stack">
    <div class="productivity-stack-track" style="display:flex;height:1rem;border-radius:999px;overflow:hidden;border:1px solid #243140;background:#0c1117" title="Active ${activePct}% · Idle ${idlePct}%">
      <div class="productivity-stack-active" style="width:${activePct}%;height:100%;background:#3ddc97"></div>
      <div class="productivity-stack-idle" style="width:${idlePct}%;height:100%;background:#314456"></div>
    </div>
    <div class="productivity-stack-legend">
      <span><i class="productivity-dot productivity-dot-active"></i>Active ${escapeHtml(
        summary.active_time || "0s"
      )} (${activePct}%)</span>
      <span><i class="productivity-dot productivity-dot-idle"></i>Idle ${escapeHtml(
        summary.idle_time || "0s"
      )} (${idlePct}%)</span>
    </div>
  </div>`;
}

function productivityBarRows(items, labelKey, valueKey) {
  const colors = productivityChartColors();
  const max = Math.max(0, ...items.map((item) => Number(item[valueKey] || 0)));
  if (!items.length) {
    return `<p class="text-sm text-mist-400">No data for this period.</p>`;
  }
  return items
    .slice(0, 8)
    .map((item, index) => {
      const value = Number(item[valueKey] || 0);
      const width = max > 0 ? Math.max(6, Math.round((value / max) * 100)) : 0;
      return `<div class="productivity-bar-row" style="display:grid;grid-template-columns:minmax(5rem,8rem) 1fr auto;gap:.65rem;align-items:center;margin-bottom:.55rem">
        <div class="productivity-bar-label" title="${escapeHtml(item[labelKey] || "")}">${escapeHtml(
        item[labelKey] || "—"
      )}</div>
        <div class="productivity-bar-track" style="height:.75rem;border-radius:999px;background:#0c1117;border:1px solid #243140;overflow:hidden">
          <div class="productivity-bar-fill" style="width:${width}%;height:100%;background:${
        colors[index % colors.length]
      }"></div>
        </div>
        <div class="productivity-bar-value">${escapeHtml(item.total_time || formatBrowserTimeSpent(value))}</div>
      </div>`;
    })
    .join("");
}

function productivityLaunchBars(items) {
  const colors = productivityChartColors();
  const rows = (items || []).slice(0, 6);
  const max = Math.max(0, ...rows.map((item) => Number(item.launch_count || 0)));
  if (!rows.length || max <= 0) {
    return `<p class="text-sm text-mist-400">No launch data for this period.</p>`;
  }
  return rows
    .map((item, index) => {
      const value = Number(item.launch_count || 0);
      const height = Math.max(8, Math.round((value / max) * 100));
      return `<div class="productivity-col" style="display:flex;flex-direction:column;align-items:center;gap:.35rem;min-width:0">
        <div class="productivity-col-value">${escapeHtml(String(value))}</div>
        <div class="productivity-col-track" style="width:100%;max-width:2.4rem;height:7rem;border-radius:.4rem .4rem .2rem .2rem;background:#0c1117;border:1px solid #243140;display:flex;align-items:flex-end;overflow:hidden">
          <div class="productivity-col-fill" style="width:100%;height:${height}%;min-height:4px;background:${
        colors[index % colors.length]
      }"></div>
        </div>
        <div class="productivity-col-label" title="${escapeHtml(item.application || "")}">${escapeHtml(
        item.application || "—"
      )}</div>
      </div>`;
    })
    .join("");
}

function renderProductivityReport() {
  ensureProductivityChartStyles();
  const root = document.getElementById("productivity-report");
  if (!root) return;
  const p = state.productivity;
  if (p.loading) {
    root.innerHTML = `<div class="panel text-sm text-mist-400 py-6 text-center">Loading…</div>`;
    return;
  }
  if (p.error) {
    root.innerHTML = `<div class="panel device-error py-6 text-center">${escapeHtml(p.error)}</div>`;
    return;
  }
  if (!p.report) {
    root.innerHTML = `<div class="panel text-sm text-mist-400 py-6 text-center">Select a computer and generate a report for the current Global Time Filter range.</div>`;
    return;
  }
  const report = p.report;
  if (report.empty) {
    root.innerHTML = `<div class="panel text-sm text-mist-400 py-6 text-center">No productivity data available for the selected period.</div>`;
    return;
  }

  const employee = report.employee || {};
  const summary = report.summary || {};
  const activity = report.activity || {};
  const applications = report.applications || [];
  const websites = report.websites || [];
  const periodStart = employee.report_period_start ? formatTime(employee.report_period_start) : "—";
  const periodEnd = employee.report_period_end ? formatTime(employee.report_period_end) : "—";
  const activeRatio = productivityPercent(summary.active_time_seconds, summary.working_time_seconds);

  const appRows = applications
    .map(
      (row) => `<tr>
        <td class="py-2 pr-3">${escapeHtml(row.application)}</td>
        <td class="py-2 pr-3 whitespace-nowrap">${escapeHtml(row.total_time)}</td>
        <td class="py-2 pr-3">${escapeHtml(String(row.percentage))}%</td>
        <td class="py-2">${escapeHtml(String(row.launch_count))}</td>
      </tr>`
    )
    .join("");
  const websiteRows = websites
    .map(
      (row) => `<tr>
        <td class="py-2 pr-3">${escapeHtml(row.website)}</td>
        <td class="py-2 pr-3 whitespace-nowrap">${escapeHtml(row.total_time)}</td>
        <td class="py-2">${escapeHtml(String(row.visits))}</td>
      </tr>`
    )
    .join("");

  const appSegments = applications.slice(0, 6).map((row) => ({
    label: row.application,
    value: row.duration_seconds,
    display: row.total_time,
  }));
  const websiteSegments = websites.slice(0, 6).map((row) => ({
    label: row.website,
    value: row.duration_seconds,
    display: row.total_time,
  }));

  root.innerHTML = `
    <section class="panel">
      <div class="panel-header"><h2>Employee Information</h2></div>
      <dl class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-3 text-sm">
        <div><dt class="text-[11px] uppercase tracking-wide text-mist-400">Computer</dt><dd class="mt-1">${escapeHtml(
          employee.computer || "—"
        )}</dd></div>
        <div><dt class="text-[11px] uppercase tracking-wide text-mist-400">User</dt><dd class="mt-1">${escapeHtml(
          employee.user || "—"
        )}</dd></div>
        <div><dt class="text-[11px] uppercase tracking-wide text-mist-400">Department</dt><dd class="mt-1">${escapeHtml(
          employee.department || "—"
        )}</dd></div>
        <div><dt class="text-[11px] uppercase tracking-wide text-mist-400">Report Period</dt><dd class="mt-1 text-xs font-mono">${escapeHtml(
          periodStart
        )} → ${escapeHtml(periodEnd)}</dd></div>
      </dl>
    </section>

    <section class="grid grid-cols-2 md:grid-cols-3 xl:grid-cols-6 gap-3">
      <article class="stat-card productivity-kpi">
        <div class="stat-label">Working Time</div>
        <div class="stat-value text-lg">${escapeHtml(summary.working_time || "0s")}</div>
        <div class="productivity-kpi-meta">First → last activity</div>
      </article>
      <article class="stat-card productivity-kpi">
        <div class="stat-label">Active Time</div>
        <div class="stat-value text-lg">${escapeHtml(summary.active_time || "0s")}</div>
        <div class="productivity-kpi-meta">${activeRatio}% of working time</div>
      </article>
      <article class="stat-card productivity-kpi">
        <div class="stat-label">Idle Time</div>
        <div class="stat-value text-lg">${escapeHtml(summary.idle_time || "0s")}</div>
        <div class="productivity-kpi-meta">Working − active</div>
      </article>
      <article class="stat-card productivity-kpi">
        <div class="stat-label">Applications Used</div>
        <div class="stat-value text-lg">${escapeHtml(String(summary.applications_used ?? 0))}</div>
        <div class="productivity-kpi-meta">Unique apps</div>
      </article>
      <article class="stat-card productivity-kpi">
        <div class="stat-label">Websites Visited</div>
        <div class="stat-value text-lg">${escapeHtml(String(summary.websites_visited ?? 0))}</div>
        <div class="productivity-kpi-meta">Unique domains</div>
      </article>
      <article class="stat-card productivity-kpi">
        <div class="stat-label">Avg Session Length</div>
        <div class="stat-value text-lg">${escapeHtml(summary.average_session_length || "0s")}</div>
        <div class="productivity-kpi-meta">Per activity segment</div>
      </article>
    </section>

    <section class="grid grid-cols-1 xl:grid-cols-3 gap-4">
      <article class="panel">
        <div class="panel-header"><h2>Time Composition</h2><span class="text-xs text-mist-400">Active vs idle</span></div>
        ${productivityStackedTimeBar(summary)}
        <div class="productivity-metric-grid mt-4">
          <div>
            <div class="text-[11px] uppercase tracking-wide text-mist-400">Longest focus</div>
            <div class="mt-1 text-sm">${escapeHtml(activity.longest_continuous_working_session || "0s")}</div>
          </div>
          <div>
            <div class="text-[11px] uppercase tracking-wide text-mist-400">Longest idle</div>
            <div class="mt-1 text-sm">${escapeHtml(activity.longest_idle_period || "0s")}</div>
          </div>
        </div>
      </article>
      <article class="panel">
        <div class="panel-header"><h2>Application Share</h2><span class="text-xs text-mist-400">By active time</span></div>
        ${productivityDonutChart(appSegments, "Apps", String(summary.applications_used ?? 0))}
      </article>
      <article class="panel">
        <div class="panel-header"><h2>Website Share</h2><span class="text-xs text-mist-400">By browse time</span></div>
        ${productivityDonutChart(websiteSegments, "Sites", String(summary.websites_visited ?? 0))}
      </article>
    </section>

    <section class="grid grid-cols-1 xl:grid-cols-2 gap-4">
      <article class="panel">
        <div class="panel-header"><h2>Most Used Applications</h2></div>
        ${productivityBarRows(applications, "application", "duration_seconds")}
      </article>
      <article class="panel">
        <div class="panel-header"><h2>Most Used Websites</h2></div>
        ${productivityBarRows(websites, "website", "duration_seconds")}
      </article>
    </section>

    <section class="panel">
      <div class="panel-header"><h2>Application Launches</h2><span class="text-xs text-mist-400">Top apps by launch count</span></div>
      <div class="productivity-cols" style="display:grid;grid-template-columns:repeat(6,minmax(0,1fr));gap:.75rem;align-items:end;min-height:10rem">${productivityLaunchBars(applications)}</div>
    </section>

    <div class="grid grid-cols-1 xl:grid-cols-2 gap-4">
      <section class="panel overflow-x-auto">
        <div class="panel-header"><h2>Application Usage</h2></div>
        <table class="w-full text-sm">
          <thead class="text-left text-mist-400 text-xs uppercase tracking-wide">
            <tr>
              <th class="py-2 pr-3">Application</th>
              <th class="py-2 pr-3">Total Time</th>
              <th class="py-2 pr-3">Percentage</th>
              <th class="py-2">Launch Count</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-ink-600">${
            appRows ||
            `<tr><td colspan="4" class="py-6 text-center text-mist-400">No application usage for this period.</td></tr>`
          }</tbody>
        </table>
      </section>
      <section class="panel overflow-x-auto">
        <div class="panel-header"><h2>Website Usage</h2></div>
        <table class="w-full text-sm">
          <thead class="text-left text-mist-400 text-xs uppercase tracking-wide">
            <tr>
              <th class="py-2 pr-3">Website</th>
              <th class="py-2 pr-3">Total Time</th>
              <th class="py-2">Visits</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-ink-600">${
            websiteRows ||
            `<tr><td colspan="3" class="py-6 text-center text-mist-400">No website usage for this period.</td></tr>`
          }</tbody>
        </table>
      </section>
    </div>

    <section class="panel">
      <div class="panel-header"><h2>Activity Summary</h2></div>
      <dl class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3 text-sm">
        <div><dt class="text-[11px] uppercase tracking-wide text-mist-400">First Activity</dt><dd class="mt-1 font-mono text-xs">${escapeHtml(
          formatTime(activity.first_activity)
        )}</dd></div>
        <div><dt class="text-[11px] uppercase tracking-wide text-mist-400">Last Activity</dt><dd class="mt-1 font-mono text-xs">${escapeHtml(
          formatTime(activity.last_activity)
        )}</dd></div>
        <div><dt class="text-[11px] uppercase tracking-wide text-mist-400">Longest Continuous Working Session</dt><dd class="mt-1">${escapeHtml(
          activity.longest_continuous_working_session || "0s"
        )}</dd></div>
        <div><dt class="text-[11px] uppercase tracking-wide text-mist-400">Longest Idle Period</dt><dd class="mt-1">${escapeHtml(
          activity.longest_idle_period || "0s"
        )}</dd></div>
        <div><dt class="text-[11px] uppercase tracking-wide text-mist-400">Most Used Application</dt><dd class="mt-1">${escapeHtml(
          activity.most_used_application || "—"
        )}</dd></div>
        <div><dt class="text-[11px] uppercase tracking-wide text-mist-400">Most Used Website</dt><dd class="mt-1">${escapeHtml(
          activity.most_used_website || "—"
        )}</dd></div>
      </dl>
    </section>`;
}

async function loadProductivityAgents({ force = false } = {}) {
  const p = state.productivity;
  if (!force && isCacheFresh(p.agentsFetchedAt) && p.agents.length) {
    renderProductivityAgentOptions();
    renderProductivityStatus();
    return;
  }
  if (!force && isCacheFresh(state.fleet.fetchedAt) && state.fleet.items.length) {
    p.agents = state.fleet.items;
    if (!p.agentId && p.agents.length === 1) {
      p.agentId = p.agents[0].agent_id;
    }
    p.agentsFetchedAt = state.fleet.fetchedAt;
    p.error = null;
    renderProductivityAgentOptions();
    renderProductivityStatus();
    return;
  }

  const gen = ++p.loadGen;
  p.agentsLoading = true;
  p.error = null;
  const select = document.getElementById("productivity-agent");
  if (select && !p.agents.length) {
    select.innerHTML = `<option value="">Loading computers…</option>`;
  }
  renderProductivityStatus();
  try {
    const payload = await fetchJson(platformUrl("/api/v1/fleet", { limit: 200, offset: 0, sort: "hostname" }));
    if (gen !== p.loadGen) return;
    p.agents = payload.items || [];
    if (!p.agentId && p.agents.length === 1) {
      p.agentId = p.agents[0].agent_id;
    }
    p.error = null;
    p.agentsFetchedAt = Date.now();
  } catch (error) {
    if (gen !== p.loadGen) return;
    p.agents = [];
    p.error = error.message || "Failed to load computers.";
  } finally {
    if (gen !== p.loadGen) return;
    p.agentsLoading = false;
    renderProductivityAgentOptions();
    renderProductivityStatus();
  }
}

function productivityReportQueryKey() {
  const p = state.productivity;
  return stableQueryKey({
    agent_id: p.agentId,
    ...applicationActivityTimeQuery(),
  });
}

async function loadProductivityReport({ force = false, silent = false } = {}) {
  const p = state.productivity;
  if (!p.agentId) {
    p.report = null;
    renderProductivityReport();
    renderProductivityStatus();
    return;
  }
  const queryKey = productivityReportQueryKey();
  if (!force && !p.loading && queryKey === p.reportQueryKey && p.report && state.view === "productivity") {
    renderProductivityReport();
    renderProductivityStatus();
    return;
  }
  if (p.loading && queryKey === p.reportQueryKey) return;

  const gen = ++p.reportGen;
  p.loading = true;
  p.error = null;
  if (!silent) {
    renderProductivityStatus();
    renderProductivityReport();
  }
  try {
    const payload = await fetchJson(
      platformUrl("/api/v1/reports/productivity", {
        agent_id: p.agentId,
        ...applicationActivityTimeQuery(),
      })
    );
    if (gen !== p.reportGen) return;
    p.report = payload;
    p.error = null;
    p.reportQueryKey = queryKey;
  } catch (error) {
    if (gen !== p.reportGen) return;
    p.report = null;
    p.error = error.message || "Failed to load productivity report.";
  } finally {
    if (gen !== p.reportGen) return;
    p.loading = false;
    if (state.view === "productivity") {
      renderProductivityStatus();
      renderProductivityReport();
    }
  }
}

async function loadProductivityPage({ force = false, silent = false } = {}) {
  await loadProductivityAgents({ force });
  if (state.productivity.agentId) {
    await loadProductivityReport({ force, silent });
  } else {
    renderProductivityReport();
  }
}

function productivityCsvEscape(value) {
  const text = String(value ?? "");
  if (/[",\n]/.test(text)) return `"${text.replace(/"/g, '""')}"`;
  return text;
}

function exportProductivityExcel() {
  const report = state.productivity.report;
  if (!report || report.empty) return;
  const lines = [];
  lines.push("Section,Field,Value");
  lines.push(`Employee,Computer,${productivityCsvEscape(report.employee?.computer)}`);
  lines.push(`Employee,User,${productivityCsvEscape(report.employee?.user)}`);
  lines.push(`Employee,Department,${productivityCsvEscape(report.employee?.department || "")}`);
  lines.push(`Summary,Working Time,${productivityCsvEscape(report.summary?.working_time)}`);
  lines.push(`Summary,Active Time,${productivityCsvEscape(report.summary?.active_time)}`);
  lines.push(`Summary,Idle Time,${productivityCsvEscape(report.summary?.idle_time)}`);
  lines.push(`Summary,Applications Used,${productivityCsvEscape(report.summary?.applications_used)}`);
  lines.push(`Summary,Websites Visited,${productivityCsvEscape(report.summary?.websites_visited)}`);
  lines.push(
    `Summary,Average Session Length,${productivityCsvEscape(report.summary?.average_session_length)}`
  );
  lines.push("");
  lines.push("Application,Total Time,Percentage,Launch Count");
  for (const row of report.applications || []) {
    lines.push(
      [
        productivityCsvEscape(row.application),
        productivityCsvEscape(row.total_time),
        productivityCsvEscape(row.percentage),
        productivityCsvEscape(row.launch_count),
      ].join(",")
    );
  }
  lines.push("");
  lines.push("Website,Total Time,Visits");
  for (const row of report.websites || []) {
    lines.push(
      [
        productivityCsvEscape(row.website),
        productivityCsvEscape(row.total_time),
        productivityCsvEscape(row.visits),
      ].join(",")
    );
  }
  const blob = new Blob([lines.join("\n")], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  const computer = (report.employee?.computer || "report").replace(/[^\w.-]+/g, "_");
  link.href = url;
  link.download = `productivity-report-${computer}.csv`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function printProductivityReport() {
  window.print();
}

document.getElementById("productivity-filters")?.addEventListener("submit", (event) => {
  event.preventDefault();
  state.productivity.agentId = document.getElementById("productivity-agent").value.trim();
  state.productivity.reportQueryKey = "";
  loadProductivityReport({ force: true });
});
document.getElementById("productivity-refresh")?.addEventListener("click", () => loadProductivityPage({ force: true }));
document.getElementById("productivity-export-excel")?.addEventListener("click", () => exportProductivityExcel());
document.getElementById("productivity-export-pdf")?.addEventListener("click", () => printProductivityReport());
document.getElementById("productivity-print")?.addEventListener("click", () => printProductivityReport());

function geolocationQueryKey() {
  return stableQueryKey(geolocationQueryParams());
}

function renderGeolocationFromCache({ refreshMap = true } = {}) {
  renderGeolocationSummary();
  renderGeolocationStatus();
  renderGeolocationTable();
  renderGeolocationDetail();
  if (refreshMap) {
    ensureGeolocationMap();
    renderGeolocationMap();
  } else {
    ensureGeolocationMap();
    window.requestAnimationFrame(() => state.geolocation.map?.invalidateSize());
  }
}

function geolocationQueryParams() {
  const g = state.geolocation;
  return {
    q: g.q,
    status: g.status,
    country: g.country,
    city: g.city,
    isp: g.isp,
  };
}

function geolocationFingerprint(payload) {
  return JSON.stringify(payload?.items || []);
}

function renderGeolocationSummary() {
  const summary = state.geolocation.summary || {};
  document.getElementById("geo-card-managed").textContent = formatNumber(summary.managed_devices ?? 0);
  document.getElementById("geo-card-online").textContent = formatNumber(summary.online_devices ?? 0);
  document.getElementById("geo-card-countries").textContent = formatNumber(summary.countries ?? 0);
  document.getElementById("geo-card-cities").textContent = formatNumber(summary.cities ?? 0);
}

function renderGeolocationStatus() {
  const el = document.getElementById("geolocation-status");
  if (!el) return;
  const g = state.geolocation;
  if (g.loading) {
    el.textContent = "Loading endpoint locations…";
    return;
  }
  if (g.error) {
    el.textContent = g.error;
    return;
  }
  const total = g.items.length;
  el.textContent = total
    ? `${total.toLocaleString()} device${total === 1 ? "" : "s"} with location information. Auto-refresh every 30 seconds.`
    : "No managed devices with location information.";
}

function formatGeoStatus(status, online) {
  if (online || status === "online") {
    return '<span class="geo-status geo-status-online">Online</span>';
  }
  return '<span class="geo-status geo-status-offline">Offline</span>';
}

function renderGeolocationTable() {
  const body = document.getElementById("geolocation-body");
  if (!body) return;
  const g = state.geolocation;
  if (g.loading && !g.items.length) {
    body.innerHTML =
      '<tr><td colspan="10" class="py-8 text-center text-mist-400">Loading endpoint locations…</td></tr>';
    return;
  }
  if (!g.items.length) {
    body.innerHTML =
      '<tr><td colspan="10" class="py-8 text-center text-mist-400">No managed devices with location information.</td></tr>';
    return;
  }
  body.innerHTML = g.items
    .map(
      (item) => `
      <tr class="geo-row${g.selectedId === item.agent_id ? " geo-row-selected" : ""}" data-agent-id="${escapeHtml(item.agent_id)}">
        <td class="py-2.5 pr-3 font-medium">${displayValue(item.computer)}</td>
        <td class="py-2.5 pr-3">${displayValue(item.user)}</td>
        <td class="py-2.5 pr-3">${formatGeoStatus(item.status, item.online)}</td>
        <td class="py-2.5 pr-3">${displayValue(item.city)}</td>
        <td class="py-2.5 pr-3">${displayValue(item.state)}</td>
        <td class="py-2.5 pr-3">${displayValue(item.country)}</td>
        <td class="py-2.5 pr-3 font-mono text-xs">${displayValue(item.public_ip)}</td>
        <td class="py-2.5 pr-3">${displayValue(item.isp)}</td>
        <td class="py-2.5 pr-3 font-mono text-xs">${displayValue(item.timezone)}</td>
        <td class="py-2.5 whitespace-nowrap">${formatRelativeLastSeen(item.last_seen)}</td>
      </tr>`
    )
    .join("");
}

function geolocationPopupHtml(item) {
  return `
    <div class="geo-popup">
      <div class="geo-popup-title">${displayValue(item.computer)}</div>
      <dl class="geo-popup-list">
        <div><dt>User</dt><dd>${displayValue(item.user)}</dd></div>
        <div><dt>City</dt><dd>${displayValue(item.city)}</dd></div>
        <div><dt>Country</dt><dd>${displayValue(item.country)}</dd></div>
        <div><dt>ISP</dt><dd>${displayValue(item.isp)}</dd></div>
        <div><dt>Public IP</dt><dd>${displayValue(item.public_ip)}</dd></div>
        <div><dt>Latitude</dt><dd>${displayValue(item.latitude)}</dd></div>
        <div><dt>Longitude</dt><dd>${displayValue(item.longitude)}</dd></div>
        <div><dt>Last Seen</dt><dd>${formatRelativeLastSeen(item.last_seen)}</dd></div>
      </dl>
    </div>`;
}

function ensureGeolocationMap() {
  const g = state.geolocation;
  if (g.map || typeof window.L === "undefined") return g.map;
  const container = document.getElementById("geolocation-map");
  if (!container) return null;
  const map = window.L.map(container, {
    center: [20, 0],
    zoom: 2,
    minZoom: 2,
    worldCopyJump: true,
  });
  window.L.tileLayer("https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png", {
    attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors &copy; CARTO',
    subdomains: "abcd",
    maxZoom: 19,
  }).addTo(map);
  g.map = map;
  return map;
}

function renderGeolocationMap() {
  const g = state.geolocation;
  const map = ensureGeolocationMap();
  if (!map) return;

  for (const marker of g.markers) {
    map.removeLayer(marker);
  }
  g.markers = [];

  const online = g.items.filter(
    (item) =>
      item.online &&
      Number.isFinite(Number(item.latitude)) &&
      Number.isFinite(Number(item.longitude))
  );
  for (const item of online) {
    const marker = window.L.circleMarker([Number(item.latitude), Number(item.longitude)], {
      radius: 10,
      color: "#e8eef4",
      fillColor: "#3ddc97",
      fillOpacity: 0.9,
      weight: 2,
    });
    marker.bindPopup(geolocationPopupHtml(item));
    marker.on("click", () => selectGeolocationDevice(item.agent_id));
    marker.addTo(map);
    g.markers.push(marker);
  }

  if (online.length === 1) {
    map.setView([Number(online[0].latitude), Number(online[0].longitude)], 8, { animate: false });
  } else if (online.length > 1) {
    const bounds = window.L.latLngBounds(
      online.map((item) => [Number(item.latitude), Number(item.longitude)])
    );
    map.fitBounds(bounds.pad(0.25), { animate: false, maxZoom: 6 });
  } else {
    map.setView([20, 0], 2, { animate: false });
  }

  window.requestAnimationFrame(() => map.invalidateSize());
}

function renderGeolocationDetail() {
  const panel = document.getElementById("geolocation-detail");
  const body = document.getElementById("geolocation-detail-body");
  if (!panel || !body) return;
  const item = state.geolocation.selected;
  if (!item) {
    panel.classList.add("hidden");
    body.innerHTML = "";
    return;
  }
  panel.classList.remove("hidden");
  const rows = [
    ["User", item.user],
    ["Hostname", item.hostname || item.computer],
    ["Public IP", item.public_ip],
    ["Private IP", item.private_ip],
    ["Country", item.country],
    ["State", item.state],
    ["City", item.city],
    ["Latitude", item.latitude],
    ["Longitude", item.longitude],
    ["Timezone", item.timezone],
    ["ISP", item.isp],
    ["ASN", item.asn],
    ["Last Seen", formatRelativeLastSeen(item.last_seen)],
    ["Operating System", item.operating_system],
  ];
  body.innerHTML = rows
    .map(
      ([label, value]) =>
        `<div class="geo-detail-row"><dt>${escapeHtml(label)}</dt><dd>${displayValue(value)}</dd></div>`
    )
    .join("");
}

function selectGeolocationDevice(agentId) {
  const g = state.geolocation;
  g.selectedId = agentId;
  g.selected = g.items.find((item) => item.agent_id === agentId) || null;
  renderGeolocationTable();
  renderGeolocationDetail();
}

async function loadGeolocation({ silent = false, force = false } = {}) {
  const g = state.geolocation;
  const queryKey = geolocationQueryKey();
  if (!force && g.loading && queryKey === g.queryKey) return;

  const gen = ++g.loadGen;
  if (!silent) {
    g.loading = true;
    g.error = null;
    renderGeolocationStatus();
    if (!g.items.length) {
      renderGeolocationTable();
    }
  }
  try {
    const payload = await fetchJson(platformUrl("/api/v1/geolocation", geolocationQueryParams()));
    if (gen !== g.loadGen) return;
    const fingerprint = geolocationFingerprint(payload);
    if (!force && silent && fingerprint === g.lastFingerprint) {
      g.fetchedAt = Date.now();
      g.error = null;
      return;
    }
    if (fingerprint !== g.lastFingerprint || !silent || force) {
      g.summary = payload.summary || g.summary;
      g.items = payload.items || [];
      g.lastFingerprint = fingerprint;
      if (g.selectedId) {
        g.selected = g.items.find((item) => item.agent_id === g.selectedId) || null;
        if (!g.selected) g.selectedId = null;
      }
      if (state.view === "geolocation") {
        renderGeolocationFromCache({ refreshMap: true });
      }
    }
    g.error = null;
    g.queryKey = queryKey;
    g.fetchedAt = Date.now();
  } catch (error) {
    if (gen !== g.loadGen) return;
    if (!silent) {
      g.error = error.message || "Failed to load geolocation data.";
      g.items = [];
      renderGeolocationTable();
      renderGeolocationMap();
    }
  } finally {
    if (gen !== g.loadGen) return;
    if (!silent) g.loading = false;
    if (state.view === "geolocation") {
      renderGeolocationStatus();
    }
  }
}

function stopGeolocationAutoRefresh() {
  const g = state.geolocation;
  if (g.refreshTimer) {
    clearInterval(g.refreshTimer);
    g.refreshTimer = null;
  }
}

function startGeolocationAutoRefresh() {
  const g = state.geolocation;
  stopGeolocationAutoRefresh();
  g.refreshTimer = setInterval(() => {
    if (state.view === "geolocation") {
      loadGeolocation({ silent: true });
    }
  }, 30000);
}

async function loadGeolocationPage({ force = false, silent = false } = {}) {
  syncGeolocationFilterFields();
  const g = state.geolocation;
  const queryKey = geolocationQueryKey();
  if (
    !force &&
    queryKey === g.queryKey &&
    isCacheFresh(g.fetchedAt) &&
    state.view === "geolocation"
  ) {
    renderGeolocationFromCache({ refreshMap: false });
    startGeolocationAutoRefresh();
    return;
  }
  await loadGeolocation({ silent, force });
  startGeolocationAutoRefresh();
}

function syncGeolocationFilterFields() {
  const g = state.geolocation;
  const search = document.getElementById("geo-search");
  const status = document.getElementById("geo-status");
  const country = document.getElementById("geo-country");
  const city = document.getElementById("geo-city");
  const isp = document.getElementById("geo-isp");
  if (search) search.value = g.q;
  if (status) status.value = g.status;
  if (country) country.value = g.country;
  if (city) city.value = g.city;
  if (isp) isp.value = g.isp;
}

document.getElementById("geolocation-filters")?.addEventListener("submit", (event) => {
  event.preventDefault();
  const g = state.geolocation;
  g.q = document.getElementById("geo-search").value.trim();
  g.status = document.getElementById("geo-status").value.trim();
  g.country = document.getElementById("geo-country").value.trim();
  g.city = document.getElementById("geo-city").value.trim();
  g.isp = document.getElementById("geo-isp").value.trim();
  loadGeolocation({ force: true });
});
document.getElementById("geolocation-clear-filters")?.addEventListener("click", () => {
  const g = state.geolocation;
  g.q = "";
  g.status = "";
  g.country = "";
  g.city = "";
  g.isp = "";
  document.getElementById("geo-search").value = "";
  document.getElementById("geo-status").value = "";
  document.getElementById("geo-country").value = "";
  document.getElementById("geo-city").value = "";
  document.getElementById("geo-isp").value = "";
  loadGeolocation({ force: true });
});
document.getElementById("geolocation-refresh")?.addEventListener("click", () => loadGeolocation({ force: true }));
document.getElementById("geolocation-body")?.addEventListener("click", (event) => {
  const row = event.target.closest(".geo-row");
  if (!row) return;
  selectGeolocationDevice(row.dataset.agentId);
});
document.getElementById("geolocation-detail-close")?.addEventListener("click", () => {
  state.geolocation.selectedId = null;
  state.geolocation.selected = null;
  renderGeolocationTable();
  renderGeolocationDetail();
});

applyAppHash();
bindApplicationActivityTimeFilter();
