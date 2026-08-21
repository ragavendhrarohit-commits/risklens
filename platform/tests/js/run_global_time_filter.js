"use strict";

const assert = require("assert");
const path = require("path");

const time = require(
  path.resolve(__dirname, "../../../frontend/assets/global-time-filter.js")
);

const NOW = new Date("2026-08-21T04:34:00.000Z");
const TZ = "Australia/Sydney";

function store(overrides) {
  return time.createTimeStore({
    storage: time.createMemoryStorage(),
    timezone: TZ,
    now: () => NOW,
    ...overrides,
  });
}

assert.strictEqual(time.PRESETS.length, 11);
assert.deepStrictEqual(
  time.PRESETS.map((item) => item.label),
  [
    "Today",
    "Yesterday",
    "Last 15 minutes",
    "Last 30 minutes",
    "Last 1 hour",
    "Last 24 hours",
    "Last 7 days",
    "Last 30 days",
    "Last 90 days",
    "Last Year",
    "Custom",
  ]
);

assert.deepStrictEqual(
  time.REFRESH_INTERVALS.map((item) => item.label),
  ["Off", "30 seconds", "1 minute", "5 minutes", "10 minutes", "30 minutes"]
);

const fifteen = time.resolveTimeRange(
  { preset: "last_15_minutes", timezone: TZ, refreshInterval: "off" },
  NOW
);
assert.strictEqual(fifteen.startTime, "2026-08-21T04:19:00.000Z");
assert.strictEqual(fifteen.endTime, NOW.toISOString());
assert.strictEqual(fifteen.preset, "last_15_minutes");
assert.strictEqual(fifteen.timezone, TZ);
assert.strictEqual(fifteen.refreshInterval, "off");

const today = time.resolveTimeRange(
  { preset: "today", timezone: TZ, refreshInterval: "off" },
  NOW
);
assert.strictEqual(today.startTime, "2026-08-20T14:00:00.000Z");
assert.strictEqual(today.endTime, NOW.toISOString());

const yesterday = time.resolveTimeRange(
  { preset: "yesterday", timezone: TZ, refreshInterval: "off" },
  NOW
);
assert.strictEqual(yesterday.startTime, "2026-08-19T14:00:00.000Z");
assert.strictEqual(yesterday.endTime, "2026-08-20T13:59:59.999Z");

const lastYear = time.resolveTimeRange(
  { preset: "last_year", timezone: TZ, refreshInterval: "off" },
  NOW
);
assert.strictEqual(lastYear.startTime, "2025-08-21T04:34:00.000Z");

const custom = time.createCustomRange("2026-08-01T00:00:00.000Z", "2026-08-02T00:00:00.000Z");
assert.strictEqual(custom.startTime, "2026-08-01T00:00:00.000Z");
assert.strictEqual(custom.endTime, "2026-08-02T00:00:00.000Z");
assert.throws(() => time.createCustomRange("2026-08-02T00:00:00.000Z", "2026-08-01T00:00:00.000Z"));

const refreshOff = time.createAutoRefresh("off");
assert.strictEqual(refreshOff.enabled, false);
assert.strictEqual(refreshOff.milliseconds, 0);
assert.strictEqual(time.createAutoRefresh("1_minute").milliseconds, 60000);
assert.strictEqual(time.createAutoRefresh("30_minutes").milliseconds, 30 * 60 * 1000);

const created = store();
const state = created.getState();
assert.strictEqual(state.preset, "last_15_minutes");
assert.strictEqual(state.timezone, TZ);
assert.strictEqual(state.refreshInterval, "off");
assert.ok(state.startTime);
assert.ok(state.endTime);
assert.strictEqual(state.refresh.enabled, false);

created.setPreset("last_1_hour");
created.setPreset("today");
created.setPreset("yesterday");
created.setPreset("last_7_days");
created.setCustomRange("2026-01-01T00:00:00.000Z", "2026-01-31T23:59:59.000Z");
created.setPreset("last_24_hours");
assert.strictEqual(created.getRecentlyUsed().length, 5);
assert.strictEqual(created.getRecentlyUsed()[0].preset, "last_24_hours");
assert.strictEqual(created.getRecentlyUsed()[1].preset, "custom");
assert.strictEqual(created.getRecentlyUsed()[1].label, "Custom");

const duplicate = store();
duplicate.setPreset("today");
duplicate.setPreset("today");
assert.strictEqual(duplicate.getRecentlyUsed().length, 1);

const refreshed = store();
refreshed.setRefreshInterval("5_minutes");
assert.strictEqual(refreshed.getState().refreshInterval, "5_minutes");
assert.strictEqual(refreshed.getState().refresh.milliseconds, 5 * 60 * 1000);
assert.strictEqual(refreshed.getState().refresh.enabled, true);

let notified = 0;
const subscribed = store();
const unsubscribe = subscribed.subscribe(() => {
  notified += 1;
});
subscribed.setPreset("last_30_minutes");
unsubscribe();
subscribed.setPreset("last_90_days");
assert.strictEqual(notified, 1);

const storage = time.createMemoryStorage();
const first = time.createTimeStore({ storage, timezone: TZ, now: () => NOW });
first.setPreset("last_30_days");
first.setRefreshInterval("10_minutes");
const restored = time.createTimeStore({ storage, timezone: "UTC", now: () => NOW });
assert.strictEqual(restored.getState().preset, "last_30_days");
assert.strictEqual(restored.getState().refreshInterval, "10_minutes");
assert.strictEqual(restored.getState().timezone, TZ);
assert.strictEqual(restored.getRecentlyUsed()[0].preset, "last_30_days");

console.log("global time filter tests passed");
