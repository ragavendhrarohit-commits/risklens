const PLATFORM_API_BASE = String(window.RISKLENS_PLATFORM_API || "http://127.0.0.1:8000").replace(
  /\/$/,
  ""
);
const API_TIMEOUT_MS = 10000;

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
    detail: null,
    detailLoading: false,
    detailError: null,
    users: null,
    software: null,
    sessions: null,
    applicationActivity: null,
    websiteUsage: null,
    health: null,
    policy: null,
    extraLoading: false,
    extraError: null,
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
  },
};

const views = document.querySelectorAll(".view");
const navLinks = document.querySelectorAll(".nav-link");

function primaryNavView(view) {
  if (view === "fleet-detail") return "fleet";
  if (view === "users" || view === "software") return "reports";
  if (view === "groups") return "administration";
  return view;
}

function setView(view) {
  state.view = view;
  views.forEach((section) => {
    section.classList.toggle("hidden", section.id !== `view-${view}`);
  });
  const navView = primaryNavView(view);
  navLinks.forEach((link) => {
    link.classList.toggle("nav-link-active", link.dataset.view === navView);
  });
  if (view === "fleet" || view === "dashboard") {
    loadFleet();
  }
  if (view === "users") {
    loadUsers();
  }
  if (view === "software") {
    loadSoftware();
  }
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
  const match = String(window.location.hash || "").match(/^#\/fleet\/([0-9a-fA-F-]{36})$/i);
  return match ? match[1] : null;
}

function renderEndpointOverview(item) {
  const cards = [
    ["Hostname", item.hostname || item.machine_name],
    ["Machine", item.machine_name],
    ["Current User", item.current_user],
    ["Agent ID", item.agent_id],
    ["Operating System", item.operating_system],
    ["Version", item.agent_version],
    ["Policy", item.policy_name],
    ["Group", ""],
    ["Tags", Array.isArray(item.tags) && item.tags.length ? item.tags.join(", ") : ""],
    ["Status", presenceLabel(item.online_status)],
    ["Last Seen", formatTime(item.last_seen)],
  ];
  const cardHtml = `<div class="grid grid-cols-2 md:grid-cols-3 xl:grid-cols-4 gap-3 mb-5">${cards
    .map(
      ([label, value]) =>
        `<article class="stat-card"><div class="stat-label">${escapeHtml(label)}</div><div class="stat-value text-base">${
          label === "Status" ? value : displayValue(value)
        }</div></article>`
    )
    .join("")}</div>`;
  return `${cardHtml}<h3 class="text-sm font-semibold mb-3">Hardware inventory (this endpoint only)</h3>${definitionList([
    ["OS version", item.os_version],
    ["Windows build", item.windows_build],
    ["Architecture", item.architecture],
    ["Manufacturer", item.manufacturer],
    ["Model", item.model],
    ["Serial number", item.serial_number],
    ["CPU model", item.cpu_model],
    ["CPU cores", item.cpu_cores],
    ["Memory", formatBytes(item.memory_bytes)],
    ["Disk total", formatBytes(item.disk_total_bytes)],
    ["Disk free", formatBytes(item.disk_free_bytes)],
    ["Last inventory", formatTime(item.last_inventory_at)],
  ])}`;
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
    document.querySelectorAll("[data-session-duration-live]").forEach((el) => {
      const logon = el.getAttribute("data-logon-at");
      el.textContent = formatDurationSeconds(liveDurationSeconds(logon, null, 0));
    });
  }, 1000);
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
  if (!items.length) {
    return `<p class="text-mist-400 mb-3">Computer → Session → Account → Application Activity</p>
      <p class="device-empty">No sessions reported for this computer.</p>`;
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
  return `<p class="text-mist-400 mb-3">Computer → Session → Account → Application Activity</p>
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
    return `<p class="device-empty">No application activity during the selected time range.</p>`;
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
  return `<table class="w-full text-sm"><thead class="text-left text-mist-400 text-xs uppercase tracking-wide"><tr>
    <th class="py-2 pr-3">Application</th><th class="py-2 pr-3">Executable</th>
    <th class="py-2 pr-3">Window Title</th><th class="py-2 pr-3">Started</th>
    <th class="py-2 pr-3">Stopped</th><th class="py-2 pr-3">Duration</th>
    <th class="py-2">Session account</th></tr></thead><tbody class="divide-y divide-ink-600">${rows}</tbody></table>
    <p class="text-xs text-mist-400 mt-3">${items.length} of ${page.total} activity row(s)</p>`;
}

function renderWebsiteUsageTable(page) {
  const items = page?.items || [];
  if (!items.length) {
    return `<p class="text-mist-400 mb-3">Website usage is rolled up from Application Activity window titles. No browser rows for this computer.</p>
      <p class="device-empty">No website usage to summarize.</p>`;
  }
  const rows = items
    .map((item) => {
      return `<tr>
        <td class="py-2 pr-3">${displayValue(item.browser)}</td>
        <td class="py-2 pr-3">${displayValue(item.website)}</td>
        <td class="py-2 pr-3 whitespace-nowrap">${escapeHtml(item.total_time || formatDurationSeconds(item.duration_seconds))}</td>
        <td class="py-2">${displayValue(item.category)}</td>
      </tr>`;
    })
    .join("");
  return `<p class="text-mist-400 mb-3">Website usage is rolled up from existing Application Activity window titles. Time is not counted twice.</p>
    <table class="w-full text-sm"><thead class="text-left text-mist-400 text-xs uppercase tracking-wide"><tr>
    <th class="py-2 pr-3">Browser</th><th class="py-2 pr-3">Website</th>
    <th class="py-2 pr-3">Total Time Spent</th><th class="py-2">Category</th>
    </tr></thead><tbody class="divide-y divide-ink-600">${rows}</tbody></table>
    <p class="text-xs text-mist-400 mt-3">${items.length} website(s)</p>`;
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

function renderTimelinePlaceholders() {
  const events = [
    { type: "user.logged_in", title: "User Logged In" },
    { type: "user.logged_out", title: "User Logged Out" },
    { type: "app.started", title: "Chrome Started" },
    { type: "app.closed", title: "Chrome Closed" },
    { type: "app.started", title: "Cursor Started" },
    { type: "app.closed", title: "Cursor Closed" },
    { type: "screen.locked", title: "Screen Locked" },
    { type: "screen.unlocked", title: "Screen Unlocked" },
    { type: "power.sleep", title: "Sleep" },
    { type: "power.wake", title: "Wake" },
    { type: "power.restart", title: "Restart" },
    { type: "power.shutdown", title: "Shutdown" },
  ];
  return `<p class="text-mist-400 mb-3">Structured timeline placeholders for this endpoint. No live events are collected yet.</p>
    <table class="w-full text-sm"><thead class="text-left text-mist-400 text-xs uppercase tracking-wide"><tr>
    <th class="py-2 pr-3">Type</th><th class="py-2 pr-3">Title</th><th class="py-2">Status</th>
    </tr></thead><tbody class="divide-y divide-ink-600">${events
      .map(
        (event) =>
          `<tr><td class="py-2 pr-3 font-mono text-xs">${escapeHtml(event.type)}</td><td class="py-2 pr-3">${escapeHtml(event.title)}</td><td class="py-2">placeholder</td></tr>`
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
  const f = state.fleet;
  const title = document.getElementById("fleet-detail-title");
  const subtitle = document.getElementById("fleet-detail-subtitle");
  const errorNode = document.getElementById("fleet-detail-error");
  const body = document.getElementById("fleet-detail-body");
  const item = f.detail;

  title.textContent = item?.hostname || item?.machine_name || "Computer";
  subtitle.textContent = f.selectedId || "";

  document.querySelectorAll(".fleet-tab").forEach((button) => {
    button.classList.toggle("fleet-tab-active", button.dataset.tab === f.tab);
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
    return;
  }

  if (f.tab === "activity") {
    body.innerHTML = renderActivityArchitecture();
    return;
  }

  if (f.tab === "alerts" || f.tab === "detections" || f.tab === "response" || f.tab === "threat-timeline") {
    body.innerHTML = `<p class="text-mist-400 mb-3">Security signals will attach to this computer. Detections and response are not implemented.</p><p>Not implemented yet.</p>`;
    return;
  }

  if (f.tab === "network") {
    body.innerHTML = `<p class="text-mist-400 mb-3">Network data belongs only to this endpoint. Collectors are not implemented.</p>
      ${definitionList([
        ["Adapters", "Not collected yet"],
        ["Active connections", "Not collected yet"],
        ["Listening ports", "Not collected yet"],
        ["Public IP", "Not collected yet"],
      ])}`;
    return;
  }

  if (f.tab === "timeline") {
    body.innerHTML = renderTimelinePlaceholders();
    return;
  }

  if (f.tab === "processes" || f.tab === "services") {
    body.innerHTML = `<p>Not implemented yet</p>`;
    return;
  }

  if (f.extraLoading) {
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
    body.innerHTML = renderWebsiteUsageTable(f.websiteUsage);
    return;
  }
  if (f.tab === "software") {
    body.innerHTML = renderFleetSoftwareTable(f.software);
    return;
  }
  if (f.tab === "health") {
    const health = f.health;
    if (!health) {
      body.innerHTML = `<p class="text-mist-400">No health data.</p>`;
      return;
    }
    body.innerHTML = definitionList([
      ["Status", presenceLabel(health.status)],
      ["Heartbeat age (s)", health.heartbeat_age_seconds],
      ["Last inventory", formatTime(health.last_inventory_at)],
      ["Last communication", formatTime(health.last_communication_at)],
      ["CPU usage", health.cpu_usage],
      ["Memory usage", health.memory_usage],
      ["Disk usage", health.disk_usage],
      ["Memory (inventory snapshot)", formatBytes(health.memory_bytes)],
      ["Disk total (inventory snapshot)", formatBytes(health.disk_total_bytes)],
      ["Disk free (inventory snapshot)", formatBytes(health.disk_free_bytes)],
    ]);
    return;
  }
  if (f.tab === "policies") {
    const policy = f.policy;
    if (!policy) {
      body.innerHTML = `<p class="text-mist-400">No policy assigned.</p>`;
      return;
    }
    body.innerHTML = definitionList([
      ["Policy ID", policy.id],
      ["Name", policy.name],
      ["Heartbeat interval (s)", policy.heartbeat_interval_seconds],
      ["Collect devices", String(policy.collect_devices)],
      ["Collect users", String(policy.collect_users)],
      ["Collect software", String(policy.collect_software)],
      ["Collect processes", String(policy.collect_processes)],
      ["Collect services", String(policy.collect_services)],
      ["Collect event logs", String(policy.collect_event_logs)],
      ["Created at", formatTime(policy.created_at)],
    ]);
  }
}

async function loadFleet() {
  const f = state.fleet;
  f.loading = true;
  f.error = null;
  renderFleetSummary();
  renderFleetStatus();
  renderFleetTable();
  renderFleetPagination();
  updateFleetSortIndicators();

  try {
    const payload = await fetchJson(
      platformUrl("/api/v1/fleet", {
        q: f.q,
        status: f.status,
        tag: f.tag,
        limit: f.limit,
        offset: f.offset,
        sort: f.sort,
      })
    );
    f.items = payload.items || [];
    f.total = Number(payload.total || 0);
    f.limit = Number(payload.limit || f.limit);
    f.offset = Number(payload.offset || 0);
    f.summary = payload.summary || f.summary;
    f.error = null;
  } catch (error) {
    f.items = [];
    f.total = 0;
    f.error = error.message || "Failed to load fleet.";
  } finally {
    f.loading = false;
    renderFleetSummary();
    renderFleetStatus();
    renderFleetTable();
    renderFleetPagination();
    updateFleetSortIndicators();
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

function onApplicationActivityTimeRangeChange() {
  const f = state.fleet;
  f.applicationActivity = null;
  if (state.view === "fleet-detail" && f.tab === "applications" && f.selectedId) {
    loadFleetTabData();
  }
}

function bindApplicationActivityTimeFilter() {
  const api = window.RiskLensGlobalTime;
  if (!api || typeof api.getGlobalTimeStore !== "function") return;
  api.getGlobalTimeStore().subscribe(onApplicationActivityTimeRangeChange);
}

async function loadFleetTabData() {
  const f = state.fleet;
  if (!f.selectedId) return;
  const tab = f.tab;
  if (
    tab === "overview" ||
    tab === "activity" ||
    tab === "network" ||
    tab === "timeline" ||
    tab === "processes" ||
    tab === "services" ||
    tab === "alerts" ||
    tab === "detections" ||
    tab === "response" ||
    tab === "threat-timeline"
  ) {
    renderFleetDetail();
    return;
  }
  f.extraLoading = true;
  f.extraError = null;
  renderFleetDetail();
  try {
    if ((tab === "accounts" || tab === "users") && !f.users) {
      f.users = await fetchJson(
        platformUrl(`/api/v1/fleet/${encodeURIComponent(f.selectedId)}/users`, { limit: 200 })
      );
    } else if (tab === "software") {
      f.software = await fetchJson(
        platformUrl(`/api/v1/fleet/${encodeURIComponent(f.selectedId)}/software`, { limit: 200 })
      );
    } else if (tab === "sessions" && !f.sessions) {
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
      f.websiteUsage = await fetchJson(
        platformUrl(`/api/v1/fleet/${encodeURIComponent(f.selectedId)}/website-usage`)
      );
    } else if (tab === "health" && !f.health) {
      f.health = await fetchJson(
        platformUrl(`/api/v1/fleet/${encodeURIComponent(f.selectedId)}/health`)
      );
    } else if (tab === "policies" && !f.policy) {
      f.policy = await fetchJson(
        platformUrl(`/api/v1/fleet/${encodeURIComponent(f.selectedId)}/policy`)
      );
    }
    f.extraError = null;
  } catch (error) {
    f.extraError = error.message || "Failed to load tab.";
  } finally {
    f.extraLoading = false;
    renderFleetDetail();
  }
}

async function loadFleetDetail(agentId) {
  const f = state.fleet;
  const switched = f.selectedId !== agentId;
  f.selectedId = agentId;
  if (switched) {
    f.tab = "overview";
    f.users = null;
    f.software = null;
    f.sessions = null;
    f.applicationActivity = null;
    f.websiteUsage = null;
    f.health = null;
    f.policy = null;
  }
  f.detailError = null;
  f.detailLoading = true;
  f.extraError = null;
  setView("fleet-detail");
  renderFleetDetail();

  try {
    f.detail = await fetchJson(platformUrl(`/api/v1/fleet/${encodeURIComponent(agentId)}`));
    f.detailError = null;
  } catch (error) {
    f.detail = null;
    f.detailError = error.message || "Failed to load agent.";
  } finally {
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
  if (window.location.hash === "#/fleet") {
    setView("fleet");
    return;
  }
  window.location.hash = "#/fleet";
}

function applyFleetHash() {
  const agentId = fleetAgentIdFromHash();
  if (agentId) {
    loadFleetDetail(agentId);
    return true;
  }
  if (window.location.hash === "#/fleet" || !window.location.hash) {
    setView("fleet");
    return true;
  }
  return false;
}

navLinks.forEach((link) => {
  link.addEventListener("click", () => {
    const view = link.dataset.view;
    if (view === "fleet") {
      if (window.location.hash !== "#/fleet") {
        window.location.hash = "#/fleet";
      }
      setView("fleet");
      return;
    }
    setView(view);
  });
});

document.querySelectorAll("[data-open-view]").forEach((button) => {
  button.addEventListener("click", () => {
    const view = button.dataset.openView;
    if (view) setView(view);
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
  loadFleet();
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

document.getElementById("fleet-detail-tabs").addEventListener("click", (event) => {
  const button = event.target.closest(".fleet-tab");
  if (!button) return;
  state.fleet.tab = button.dataset.tab;
  loadFleetTabData();
});

window.addEventListener("hashchange", () => {
  applyFleetHash();
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

async function loadUsers() {
  const u = state.users;
  u.loading = true;
  u.error = null;
  renderUsersStatus();
  renderUsersTable();
  renderUsersPagination();
  updateUserSortIndicators();

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

  try {
    const payload = await fetchJson(platformUrl("/api/v1/users", params));
    u.items = payload.items || [];
    u.total = Number(payload.total || 0);
    u.limit = Number(payload.limit || u.limit);
    u.offset = Number(payload.offset || 0);
    u.error = null;
  } catch (error) {
    u.items = [];
    u.total = 0;
    u.error = error.message || "Failed to load users.";
  } finally {
    u.loading = false;
    renderUsersStatus();
    renderUsersTable();
    renderUsersPagination();
    updateUserSortIndicators();
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

document.getElementById("users-refresh").addEventListener("click", () => loadUsers());
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

async function loadSoftware() {
  const s = state.software;
  s.loading = true;
  s.error = null;
  renderSoftwareStatus();
  renderSoftwareTable();
  renderSoftwarePagination();
  updateSoftwareSortIndicators();

  const params = {
    application_name: s.application_name,
    publisher: s.publisher,
    version: s.version,
    architecture: s.architecture,
    limit: s.limit,
    offset: s.offset,
    sort: s.sort,
  };

  try {
    const payload = await fetchJson(platformUrl("/api/v1/software", params));
    s.items = payload.items || [];
    s.total = Number(payload.total || 0);
    s.limit = Number(payload.limit || s.limit);
    s.offset = Number(payload.offset || 0);
    s.error = null;
  } catch (error) {
    s.items = [];
    s.total = 0;
    s.error = error.message || "Failed to load installed software.";
  } finally {
    s.loading = false;
    renderSoftwareStatus();
    renderSoftwareTable();
    renderSoftwarePagination();
    updateSoftwareSortIndicators();
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

document.getElementById("software-refresh").addEventListener("click", () => loadSoftware());
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

applyFleetHash();
bindApplicationActivityTimeFilter();
