"use strict";

const assert = require("assert");
const path = require("path");

const time = require(
  path.resolve(__dirname, "../../../frontend/assets/global-time-filter.js")
);
const picker = require(
  path.resolve(__dirname, "../../../frontend/assets/global-time-picker.js")
);

const NOW = new Date("2026-08-21T04:34:00.000Z");

function makeStore() {
  return time.createTimeStore({
    storage: time.createMemoryStorage(),
    timezone: "UTC",
    now: () => NOW,
  });
}

assert.deepStrictEqual(
  Object.values(picker.PRESET_UI_LABELS),
  [
    "Today",
    "Yesterday",
    "Last 15 Minutes",
    "Last 30 Minutes",
    "Last 1 Hour",
    "Last 24 Hours",
    "Last 7 Days",
    "Last 30 Days",
    "Last 90 Days",
    "Last Year",
    "Custom Range",
  ]
);
assert.deepStrictEqual(Object.values(picker.REFRESH_UI_LABELS), [
  "Off",
  "30 Seconds",
  "1 Minute",
  "5 Minutes",
  "10 Minutes",
  "30 Minutes",
]);

const store = makeStore();
const controller = picker.createTimePickerController(store);
const view = controller.getView();
assert.strictEqual(view.headerText, "Last 15 Minutes");
assert.strictEqual(view.presets.length, 11);
assert.deepStrictEqual(
  view.presets.map((item) => item.label),
  Object.values(picker.PRESET_UI_LABELS)
);
assert.strictEqual(view.refreshIntervals.length, 6);
assert.strictEqual(view.customOpen, false);

const selected = controller.selectPreset("last_7_days");
assert.strictEqual(selected.applied, true);
assert.strictEqual(store.getState().preset, "last_7_days");
assert.strictEqual(selected.view.headerText, "Last 7 Days");
assert.strictEqual(store.getRecentlyUsed()[0].preset, "last_7_days");
assert.strictEqual(selected.view.recentlyUsed[0].label, "Last 7 Days");

controller.selectPreset("today");
controller.selectPreset("yesterday");
controller.selectPreset("last_30_days");
controller.selectPreset("last_year");
assert.ok(store.getRecentlyUsed().length <= 5);
assert.strictEqual(store.getRecentlyUsed().length, 5);

const customOpen = controller.selectPreset("custom");
assert.strictEqual(customOpen.applied, false);
assert.strictEqual(store.getState().preset, "last_year");
assert.strictEqual(customOpen.view.customOpen, true);

const invalid = picker.validateCustomRange("", "", "", "");
assert.strictEqual(invalid.ok, false);
controller.setCustomInputs({
  startDate: "2026-08-21",
  startTime: "18:00",
  endDate: "2026-08-21",
  endTime: "10:00",
});
const inverted = controller.applyCustom();
assert.strictEqual(inverted.applied, false);
assert.match(inverted.error, /Start must be before End/);
assert.strictEqual(store.getState().preset, "last_year");

controller.setCustomInputs({
  startDate: "2026-08-01",
  startTime: "00:00",
  endDate: "2026-08-02",
  endTime: "00:00",
});
const applied = controller.applyCustom();
assert.strictEqual(applied.applied, true);
assert.strictEqual(store.getState().preset, "custom");
assert.strictEqual(applied.view.headerText, "Custom Range");

const cancelled = controller.selectPreset("custom");
assert.strictEqual(cancelled.applied, false);
controller.cancelCustom();
assert.strictEqual(controller.getView().customOpen, false);
assert.strictEqual(store.getState().preset, "custom");

const refresh = controller.selectRefresh("5_minutes");
assert.strictEqual(refresh.applied, true);
assert.strictEqual(store.getState().refreshInterval, "5_minutes");
assert.strictEqual(refresh.view.refreshIntervals.find((item) => item.id === "5_minutes").active, true);

const recentApply = controller.selectRecent(store.getRecentlyUsed().find((item) => item.preset === "last_year"));
assert.strictEqual(recentApply.applied, true);
assert.strictEqual(store.getState().preset, "last_year");

assert.strictEqual(picker.shouldClosePicker("Escape"), true);
assert.strictEqual(picker.shouldClosePicker("Enter"), false);
assert.strictEqual(picker.nextFocusIndex(0, "ArrowDown", 4), 1);
assert.strictEqual(picker.nextFocusIndex(3, "ArrowDown", 4), 0);
assert.strictEqual(picker.nextFocusIndex(0, "ArrowUp", 4), 3);
assert.strictEqual(picker.nextFocusIndex(2, "Home", 4), 0);
assert.strictEqual(picker.nextFocusIndex(0, "End", 4), 3);
assert.strictEqual(
  picker.formatRecentLabel({
    preset: "custom",
    startTime: "2026-08-01T00:00:00.000Z",
    endTime: "2026-08-02T00:00:00.000Z",
  }).includes("–"),
  true
);

console.log("global time picker tests passed");
