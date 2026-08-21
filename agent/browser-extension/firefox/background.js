const NATIVE_HOST = "com.risklens.browser";
const POLL_MS = 15000;

const api = typeof browser !== "undefined" ? browser : chrome;

let port = null;

function browserName() {
  const ua = navigator.userAgent || "";
  if (ua.includes("Edg/")) {
    return "edge";
  }
  if (ua.includes("Firefox/")) {
    return "firefox";
  }
  return "chrome";
}

function executableFor(name) {
  if (name === "edge") {
    return "msedge.exe";
  }
  if (name === "firefox") {
    return "firefox.exe";
  }
  return "chrome.exe";
}

function stripUrl(raw) {
  if (!raw || typeof raw !== "string") {
    return { host: "", url: "" };
  }
  try {
    const parsed = new URL(raw);
    const scheme = (parsed.protocol || "").replace(":", "").toLowerCase();
    if (
      [
        "chrome",
        "chrome-extension",
        "edge",
        "extension",
        "about",
        "file",
        "data",
        "blob",
        "javascript",
        "view-source",
        "devtools",
        "moz-extension",
      ].includes(scheme)
    ) {
      return { host: "", url: "" };
    }
    if (scheme !== "http" && scheme !== "https") {
      return { host: "", url: "" };
    }
    let host = (parsed.hostname || "").replace(/\.$/, "").toLowerCase();
    if (host.startsWith("www.")) {
      host = host.slice(4);
    }
    if (!host) {
      return { host: "", url: "" };
    }
    const port = parsed.port;
    const dropPort =
      (scheme === "https" && port === "443") || (scheme === "http" && port === "80");
    const origin = scheme + "://" + host + (port && !dropPort ? ":" + port : "");
    return { host, url: origin };
  } catch (_err) {
    return { host: "", url: "" };
  }
}

function connect() {
  if (!api.runtime || !api.runtime.connectNative) {
    return;
  }
  try {
    port = api.runtime.connectNative(NATIVE_HOST);
    port.onDisconnect.addListener(function () {
      port = null;
    });
  } catch (_err) {
    port = null;
  }
}

function send(payload) {
  if (!port) {
    connect();
  }
  if (!port) {
    return;
  }
  try {
    port.postMessage(payload);
  } catch (_err) {
    port = null;
  }
}

async function capture() {
  const tabs = await api.tabs.query({ active: true, lastFocusedWindow: true });
  if (!tabs || !tabs.length) {
    return;
  }
  const tab = tabs[0];
  if (tab.incognito) {
    return;
  }
  if (!tab.active) {
    return;
  }
  const stripped = stripUrl(tab.url || "");
  if (!stripped.host) {
    return;
  }
  const name = browserName();
  send({
    type: "active_tab",
    browser: name,
    executable: executableFor(name),
    url: stripped.url,
    title: String(tab.title || "").slice(0, 512),
    profile: "",
    incognito: false,
    window_id: tab.windowId,
    tab_id: tab.id,
    at: new Date().toISOString(),
  });
}

function wire() {
  if (api.tabs && api.tabs.onActivated) {
    api.tabs.onActivated.addListener(function () {
      capture();
    });
  }
  if (api.windows && api.windows.onFocusChanged) {
    api.windows.onFocusChanged.addListener(function () {
      capture();
    });
  }
  if (api.tabs && api.tabs.onUpdated) {
    api.tabs.onUpdated.addListener(function (_tabId, info, tab) {
      if (info && info.status === "complete" && tab && tab.active) {
        capture();
      }
    });
  }
  if (api.alarms) {
    api.alarms.create("risklens-active-tab", { periodInMinutes: 0.25 });
    api.alarms.onAlarm.addListener(function (alarm) {
      if (alarm && alarm.name === "risklens-active-tab") {
        capture();
      }
    });
  } else {
    setInterval(capture, POLL_MS);
  }
  capture();
}

wire();
