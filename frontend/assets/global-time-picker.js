/**
 * Global Time Picker — Phase 2 UI only.
 * Updates RiskLensGlobalTime store. Pages must not subscribe or filter yet.
 */
(function (root, factory) {
  const api = factory(root);
  if (typeof module === "object" && module.exports) {
    module.exports = api;
  }
  root.RiskLensGlobalTimePicker = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function (root) {
  const PRESET_UI_LABELS = Object.freeze({
    today: "Today",
    yesterday: "Yesterday",
    last_15_minutes: "Last 15 Minutes",
    last_30_minutes: "Last 30 Minutes",
    last_1_hour: "Last 1 Hour",
    last_24_hours: "Last 24 Hours",
    last_7_days: "Last 7 Days",
    last_30_days: "Last 30 Days",
    last_90_days: "Last 90 Days",
    last_year: "Last Year",
    custom: "Custom Range",
  });

  const REFRESH_UI_LABELS = Object.freeze({
    off: "Off",
    "30_seconds": "30 Seconds",
    "1_minute": "1 Minute",
    "5_minutes": "5 Minutes",
    "10_minutes": "10 Minutes",
    "30_minutes": "30 Minutes",
  });

  function displayPresetLabel(presetId) {
    return PRESET_UI_LABELS[presetId] || presetId;
  }

  function displayRefreshLabel(intervalId) {
    return REFRESH_UI_LABELS[intervalId] || intervalId;
  }

  function pad(value) {
    return String(value).padStart(2, "0");
  }

  function isoToLocalInputs(iso) {
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) {
      return { date: "", time: "" };
    }
    return {
      date: `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`,
      time: `${pad(date.getHours())}:${pad(date.getMinutes())}`,
    };
  }

  function localInputsToDate(dateValue, timeValue) {
    if (!dateValue || !timeValue) return null;
    const parsed = new Date(`${dateValue}T${timeValue}:00`);
    if (Number.isNaN(parsed.getTime())) return null;
    return parsed;
  }

  function validateCustomRange(startDate, startTime, endDate, endTime) {
    const start = localInputsToDate(startDate, startTime);
    const end = localInputsToDate(endDate, endTime);
    if (!start || !end) {
      return { ok: false, error: "Start Date, Start Time, End Date, and End Time are required." };
    }
    if (start.getTime() > end.getTime()) {
      return { ok: false, error: "Start must be before End." };
    }
    return { ok: true, start, end, error: "" };
  }

  function headerLabel(state) {
    if (!state) return displayPresetLabel("last_15_minutes");
    if (state.preset === "custom") return displayPresetLabel("custom");
    return displayPresetLabel(state.preset);
  }

  function formatRecentLabel(item) {
    if (!item) return "";
    if (item.preset !== "custom") return displayPresetLabel(item.preset);
    const start = isoToLocalInputs(item.startTime);
    const end = isoToLocalInputs(item.endTime);
    if (!start.date || !end.date) return displayPresetLabel("custom");
    return `${start.date} ${start.time} – ${end.date} ${end.time}`;
  }

  function shouldClosePicker(key) {
    return key === "Escape";
  }

  function isPickerNavKey(key) {
    return key === "ArrowDown" || key === "ArrowUp" || key === "ArrowLeft" || key === "ArrowRight" || key === "Home" || key === "End";
  }

  function nextFocusIndex(current, key, count) {
    if (!count) return 0;
    if (key === "Home") return 0;
    if (key === "End") return count - 1;
    if (key === "ArrowDown" || key === "ArrowRight") return (current + 1) % count;
    if (key === "ArrowUp" || key === "ArrowLeft") return (current - 1 + count) % count;
    return current;
  }

  function panelFocusables(rootEl) {
    return Array.from(
      rootEl.querySelectorAll("[data-preset], [data-recent], .gtp-input, #gtp-cancel, #gtp-apply, [data-refresh]")
    ).filter((node) => !node.disabled && !node.closest(".hidden"));
  }

  function createTimePickerController(store) {
    if (!store) {
      throw new Error("Global time store is required");
    }

    let customOpen = false;
    let customError = "";
    let customInputs = { startDate: "", startTime: "", endDate: "", endTime: "" };

    function syncCustomFromStore() {
      const state = store.getState();
      const start = isoToLocalInputs(state.startTime);
      const end = isoToLocalInputs(state.endTime);
      customInputs = {
        startDate: start.date,
        startTime: start.time,
        endDate: end.date,
        endTime: end.time,
      };
    }

    syncCustomFromStore();

    return {
      displayPresetLabel,
      displayRefreshLabel,
      headerLabel,
      validateCustomRange,
      getView() {
        const state = store.getState();
        return {
          state,
          headerText: headerLabel(state),
          presets: store.getPresets().map((preset) => ({
            id: preset.id,
            label: displayPresetLabel(preset.id),
            active: customOpen ? preset.id === "custom" : preset.id === state.preset,
          })),
          refreshIntervals: store.getRefreshIntervals().map((item) => ({
            id: item.id,
            label: displayRefreshLabel(item.id),
            active: item.id === state.refreshInterval,
          })),
          recentlyUsed: store.getRecentlyUsed().map((item) => ({
            ...item,
            label: formatRecentLabel(item),
            active: customOpen ? item.preset === "custom" : item.preset === state.preset,
          })),
          customOpen,
          customInputs: { ...customInputs },
          customError,
        };
      },
      selectPreset(presetId) {
        customError = "";
        if (presetId === "custom") {
          customOpen = true;
          syncCustomFromStore();
          return { applied: false, view: this.getView() };
        }
        customOpen = false;
        const state = store.setPreset(presetId);
        return { applied: true, state, view: this.getView() };
      },
      selectRefresh(intervalId) {
        const state = store.setRefreshInterval(intervalId);
        return { applied: true, state, view: this.getView() };
      },
      selectRecent(item) {
        customError = "";
        customOpen = item && item.preset === "custom";
        const state = store.applyRecent(item);
        if (customOpen) syncCustomFromStore();
        return { applied: true, state, view: this.getView() };
      },
      setCustomInputs(next) {
        customInputs = { ...customInputs, ...next };
        customError = "";
      },
      applyCustom() {
        const result = validateCustomRange(
          customInputs.startDate,
          customInputs.startTime,
          customInputs.endDate,
          customInputs.endTime
        );
        if (!result.ok) {
          customError = result.error;
          return { applied: false, error: customError, view: this.getView() };
        }
        const state = store.setCustomRange(result.start, result.end);
        customOpen = false;
        customError = "";
        return { applied: true, state, view: this.getView() };
      },
      cancelCustom() {
        customOpen = false;
        customError = "";
        syncCustomFromStore();
        return { applied: false, view: this.getView() };
      },
    };
  }

  function bindTimePicker(rootEl, store) {
    const controller = createTimePickerController(store);
    const toggle = rootEl.querySelector("#gtp-toggle");
    const current = rootEl.querySelector("#gtp-current");
    const presetsEl = rootEl.querySelector("#gtp-presets");
    const recentEl = rootEl.querySelector("#gtp-recent");
    const customEl = rootEl.querySelector("#gtp-custom");
    const errorEl = rootEl.querySelector("#gtp-custom-error");
    const refreshEl = rootEl.querySelector("#gtp-refresh-options");
    const startDate = rootEl.querySelector("#gtp-start-date");
    const startTime = rootEl.querySelector("#gtp-start-time");
    const endDate = rootEl.querySelector("#gtp-end-date");
    const endTime = rootEl.querySelector("#gtp-end-time");

    function isOpen() {
      return rootEl.classList.contains("is-open");
    }

    function setOpen(open) {
      rootEl.classList.toggle("is-open", open);
      toggle.setAttribute("aria-expanded", open ? "true" : "false");
      if (open) {
        paint();
      }
    }

    function moveFocus(key) {
      const items = panelFocusables(rootEl);
      if (!items.length) return;
      const active = document.activeElement;
      let currentIndex = items.indexOf(active);
      if (currentIndex < 0) {
        currentIndex = key === "ArrowUp" || key === "ArrowLeft" || key === "End" ? 0 : -1;
      }
      const next = items[nextFocusIndex(currentIndex, key, items.length)];
      if (next) next.focus();
    }

    function paint() {
      const view = controller.getView();
      current.textContent = view.headerText;
      presetsEl.innerHTML = view.presets
        .map(
          (preset) =>
            `<button type="button" class="gtp-chip${preset.active ? " gtp-chip-active" : ""}" data-preset="${preset.id}">${preset.label}</button>`
        )
        .join("");
      if (!view.recentlyUsed.length) {
        recentEl.innerHTML = `<p class="gtp-recent-empty">No recently used ranges</p>`;
      } else {
        recentEl.innerHTML = view.recentlyUsed
          .map(
            (item, index) =>
              `<button type="button" class="gtp-recent-item${item.active ? " gtp-chip-active" : ""}" data-recent="${index}">${item.label}</button>`
          )
          .join("");
      }
      refreshEl.innerHTML = view.refreshIntervals
        .map(
          (item) =>
            `<button type="button" class="gtp-chip${item.active ? " gtp-chip-active" : ""}" data-refresh="${item.id}">${item.label}</button>`
        )
        .join("");
      customEl.classList.toggle("hidden", !view.customOpen);
      startDate.value = view.customInputs.startDate;
      startTime.value = view.customInputs.startTime;
      endDate.value = view.customInputs.endDate;
      endTime.value = view.customInputs.endTime;
      if (view.customError) {
        errorEl.hidden = false;
        errorEl.textContent = view.customError;
      } else {
        errorEl.hidden = true;
        errorEl.textContent = "";
      }
    }

    toggle.addEventListener("click", (event) => {
      event.stopPropagation();
      setOpen(!isOpen());
      paint();
    });
    toggle.addEventListener("keydown", (event) => {
      if (event.key !== "ArrowDown" || isOpen()) return;
      event.preventDefault();
      setOpen(true);
      const first = panelFocusables(rootEl)[0];
      if (first) first.focus();
    });

    presetsEl.addEventListener("click", (event) => {
      const button = event.target.closest("[data-preset]");
      if (!button) return;
      const result = controller.selectPreset(button.dataset.preset);
      paint();
      if (result.applied) setOpen(false);
    });

    recentEl.addEventListener("click", (event) => {
      const button = event.target.closest("[data-recent]");
      if (!button) return;
      const item = store.getRecentlyUsed()[Number(button.dataset.recent)];
      if (!item) return;
      controller.selectRecent(item);
      paint();
      setOpen(false);
    });

    refreshEl.addEventListener("click", (event) => {
      const button = event.target.closest("[data-refresh]");
      if (!button) return;
      controller.selectRefresh(button.dataset.refresh);
      paint();
    });

    function readCustomInputs() {
      controller.setCustomInputs({
        startDate: startDate.value,
        startTime: startTime.value,
        endDate: endDate.value,
        endTime: endTime.value,
      });
    }

    rootEl.querySelector("#gtp-apply").addEventListener("click", () => {
      readCustomInputs();
      const result = controller.applyCustom();
      paint();
      if (result.applied) setOpen(false);
    });

    rootEl.querySelector("#gtp-cancel").addEventListener("click", () => {
      controller.cancelCustom();
      paint();
      setOpen(false);
    });

    document.addEventListener("click", (event) => {
      if (!rootEl.contains(event.target)) setOpen(false);
    });

    document.addEventListener("keydown", (event) => {
      if (shouldClosePicker(event.key) && isOpen()) {
        event.preventDefault();
        setOpen(false);
        toggle.focus();
        return;
      }
      if (!isOpen() || !isPickerNavKey(event.key)) return;
      if (event.target && event.target.classList && event.target.classList.contains("gtp-input")) {
        return;
      }
      event.preventDefault();
      moveFocus(event.key);
    });

    paint();
    return { controller, paint, setOpen };
  }

  function initGlobalTimePicker(options) {
    const opts = options || {};
    const timeApi = opts.timeApi || root.RiskLensGlobalTime;
    if (!timeApi) return null;
    const store = opts.store || timeApi.getGlobalTimeStore();
    const mount = opts.root || (typeof document !== "undefined" ? document.getElementById("global-time-picker") : null);
    if (!mount) return { store, controller: createTimePickerController(store) };
    return bindTimePicker(mount, store);
  }

  if (typeof document !== "undefined") {
    const start = function start() {
      if (!document.getElementById("global-time-picker")) return;
      initGlobalTimePicker();
    };
    if (document.readyState === "loading") {
      document.addEventListener("DOMContentLoaded", start);
    } else {
      start();
    }
  }

  return {
    PRESET_UI_LABELS,
    REFRESH_UI_LABELS,
    displayPresetLabel,
    displayRefreshLabel,
    headerLabel,
    validateCustomRange,
    isoToLocalInputs,
    formatRecentLabel,
    shouldClosePicker,
    isPickerNavKey,
    nextFocusIndex,
    createTimePickerController,
    initGlobalTimePicker,
  };
});
