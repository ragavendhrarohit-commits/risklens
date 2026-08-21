/**
 * Global Time Filter — reusable time store.
 * The header picker may load this module. Pages must not consume it until a later phase.
 */
(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) {
    module.exports = api;
  }
  root.RiskLensGlobalTime = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  const STORAGE_STATE_KEY = "risklens.globalTime.state";
  const STORAGE_RECENT_KEY = "risklens.globalTime.recent";
  const RECENT_LIMIT = 5;
  const CUSTOM_PRESET = "custom";

  const PRESETS = Object.freeze([
    Object.freeze({ id: "today", label: "Today", kind: "calendar" }),
    Object.freeze({ id: "yesterday", label: "Yesterday", kind: "calendar" }),
    Object.freeze({ id: "last_15_minutes", label: "Last 15 minutes", kind: "relative", durationMs: 15 * 60 * 1000 }),
    Object.freeze({ id: "last_30_minutes", label: "Last 30 minutes", kind: "relative", durationMs: 30 * 60 * 1000 }),
    Object.freeze({ id: "last_1_hour", label: "Last 1 hour", kind: "relative", durationMs: 60 * 60 * 1000 }),
    Object.freeze({ id: "last_24_hours", label: "Last 24 hours", kind: "relative", durationMs: 24 * 60 * 60 * 1000 }),
    Object.freeze({ id: "last_7_days", label: "Last 7 days", kind: "relative", durationMs: 7 * 24 * 60 * 60 * 1000 }),
    Object.freeze({ id: "last_30_days", label: "Last 30 days", kind: "relative", durationMs: 30 * 24 * 60 * 60 * 1000 }),
    Object.freeze({ id: "last_90_days", label: "Last 90 days", kind: "relative", durationMs: 90 * 24 * 60 * 60 * 1000 }),
    Object.freeze({ id: "last_year", label: "Last Year", kind: "calendar_year" }),
    Object.freeze({ id: "custom", label: "Custom", kind: "custom" }),
  ]);

  const PRESET_BY_ID = Object.freeze(
    Object.fromEntries(PRESETS.map((preset) => [preset.id, preset]))
  );

  const REFRESH_INTERVALS = Object.freeze([
    Object.freeze({ id: "off", label: "Off", milliseconds: 0 }),
    Object.freeze({ id: "30_seconds", label: "30 seconds", milliseconds: 30 * 1000 }),
    Object.freeze({ id: "1_minute", label: "1 minute", milliseconds: 60 * 1000 }),
    Object.freeze({ id: "5_minutes", label: "5 minutes", milliseconds: 5 * 60 * 1000 }),
    Object.freeze({ id: "10_minutes", label: "10 minutes", milliseconds: 10 * 60 * 1000 }),
    Object.freeze({ id: "30_minutes", label: "30 minutes", milliseconds: 30 * 60 * 1000 }),
  ]);

  const REFRESH_BY_ID = Object.freeze(
    Object.fromEntries(REFRESH_INTERVALS.map((item) => [item.id, item]))
  );

  const DEFAULT_PRESET = "last_15_minutes";
  const DEFAULT_REFRESH = "off";

  function createMemoryStorage() {
    const data = new Map();
    return {
      getItem(key) {
        return data.has(key) ? data.get(key) : null;
      },
      setItem(key, value) {
        data.set(String(key), String(value));
      },
      removeItem(key) {
        data.delete(key);
      },
    };
  }

  function detectTimezone() {
    try {
      const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
      if (zone) return zone;
    } catch (_error) {
      /* fall through */
    }
    return "UTC";
  }

  function toDate(value) {
    if (value instanceof Date) {
      if (Number.isNaN(value.getTime())) {
        throw new Error("Invalid date");
      }
      return value;
    }
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) {
      throw new Error("Invalid date");
    }
    return parsed;
  }

  function toIso(date) {
    return toDate(date).toISOString();
  }

  function zonedParts(date, timeZone) {
    const formatter = new Intl.DateTimeFormat("en-US", {
      timeZone,
      hourCycle: "h23",
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
    const named = {};
    for (const part of formatter.formatToParts(date)) {
      if (part.type !== "literal") named[part.type] = part.value;
    }
    return {
      year: Number(named.year),
      month: Number(named.month),
      day: Number(named.day),
      hour: Number(named.hour),
      minute: Number(named.minute),
      second: Number(named.second),
    };
  }

  function timezoneOffsetMs(date, timeZone) {
    const parts = zonedParts(date, timeZone);
    const asUtc = Date.UTC(parts.year, parts.month - 1, parts.day, parts.hour, parts.minute, parts.second);
    return asUtc - date.getTime();
  }

  function zonedLocalToUtc(parts, timeZone) {
    const utcGuess = Date.UTC(
      parts.year,
      parts.month - 1,
      parts.day,
      parts.hour || 0,
      parts.minute || 0,
      parts.second || 0,
      parts.millisecond || 0
    );
    const first = utcGuess - timezoneOffsetMs(new Date(utcGuess), timeZone);
    const second = utcGuess - timezoneOffsetMs(new Date(first), timeZone);
    return new Date(second);
  }

  function startOfZonedDay(date, timeZone) {
    const parts = zonedParts(date, timeZone);
    return zonedLocalToUtc(
      { year: parts.year, month: parts.month, day: parts.day, hour: 0, minute: 0, second: 0, millisecond: 0 },
      timeZone
    );
  }

  function addZonedDays(date, timeZone, days) {
    const start = startOfZonedDay(date, timeZone);
    const parts = zonedParts(start, timeZone);
    const utcDay = Date.UTC(parts.year, parts.month - 1, parts.day) + days * 24 * 60 * 60 * 1000;
    const shifted = new Date(utcDay);
    return zonedLocalToUtc(
      {
        year: shifted.getUTCFullYear(),
        month: shifted.getUTCMonth() + 1,
        day: shifted.getUTCDate(),
        hour: 0,
        minute: 0,
        second: 0,
        millisecond: 0,
      },
      timeZone
    );
  }

  function subtractCalendarYear(date, timeZone) {
    const parts = zonedParts(date, timeZone);
    return zonedLocalToUtc(
      {
        year: parts.year - 1,
        month: parts.month,
        day: parts.day,
        hour: parts.hour,
        minute: parts.minute,
        second: parts.second,
        millisecond: date.getMilliseconds(),
      },
      timeZone
    );
  }

  function requirePreset(presetId) {
    const preset = PRESET_BY_ID[presetId];
    if (!preset) {
      throw new Error(`Unknown time preset: ${presetId}`);
    }
    return preset;
  }

  function requireRefresh(intervalId) {
    const interval = REFRESH_BY_ID[intervalId];
    if (!interval) {
      throw new Error(`Unknown refresh interval: ${intervalId}`);
    }
    return interval;
  }

  function createCustomRange(startTime, endTime) {
    const start = toDate(startTime);
    const end = toDate(endTime);
    if (start.getTime() > end.getTime()) {
      throw new Error("Custom range startTime must be before endTime");
    }
    return Object.freeze({
      startTime: toIso(start),
      endTime: toIso(end),
    });
  }

  function createAutoRefresh(intervalId) {
    const interval = requireRefresh(intervalId);
    return Object.freeze({
      intervalId: interval.id,
      label: interval.label,
      milliseconds: interval.milliseconds,
      enabled: interval.milliseconds > 0,
    });
  }

  function resolveTimeRange(input, now) {
    const timezone = input.timezone || detectTimezone();
    const preset = requirePreset(input.preset);
    const clock = toDate(now || new Date());
    let start;
    let end = clock;

    if (preset.kind === "relative") {
      start = new Date(clock.getTime() - preset.durationMs);
    } else if (preset.id === "today") {
      start = startOfZonedDay(clock, timezone);
    } else if (preset.id === "yesterday") {
      const todayStart = startOfZonedDay(clock, timezone);
      start = addZonedDays(todayStart, timezone, -1);
      end = new Date(todayStart.getTime() - 1);
    } else if (preset.id === "last_year") {
      start = subtractCalendarYear(clock, timezone);
    } else if (preset.id === CUSTOM_PRESET) {
      const custom = createCustomRange(input.startTime, input.endTime);
      start = toDate(custom.startTime);
      end = toDate(custom.endTime);
    } else {
      throw new Error(`Unsupported time preset: ${preset.id}`);
    }

    return Object.freeze({
      startTime: toIso(start),
      endTime: toIso(end),
      preset: preset.id,
      timezone,
      refreshInterval: requireRefresh(input.refreshInterval || DEFAULT_REFRESH).id,
    });
  }

  function presetLabel(presetId) {
    return requirePreset(presetId).label;
  }

  function recentKey(item) {
    return [item.preset, item.startTime, item.endTime, item.timezone].join("|");
  }

  function pushRecent(existing, item) {
    const next = [item, ...existing.filter((row) => recentKey(row) !== recentKey(item))];
    return next.slice(0, RECENT_LIMIT);
  }

  function readJson(storage, key, fallback) {
    try {
      const raw = storage.getItem(key);
      if (!raw) return fallback;
      return JSON.parse(raw);
    } catch (_error) {
      return fallback;
    }
  }

  function writeJson(storage, key, value) {
    storage.setItem(key, JSON.stringify(value));
  }

  function defaultSelection(timezone) {
    return {
      preset: DEFAULT_PRESET,
      timezone: timezone || detectTimezone(),
      refreshInterval: DEFAULT_REFRESH,
      startTime: null,
      endTime: null,
    };
  }

  function createTimeStore(options) {
    const opts = options || {};
    const storage = opts.storage || (typeof localStorage !== "undefined" ? localStorage : createMemoryStorage());
    const clock = opts.now || (() => new Date());
    const timezone = opts.timezone || detectTimezone();

    let selection = readJson(storage, STORAGE_STATE_KEY, null) || defaultSelection(timezone);
    if (!PRESET_BY_ID[selection.preset]) selection = defaultSelection(timezone);
    if (!REFRESH_BY_ID[selection.refreshInterval]) selection.refreshInterval = DEFAULT_REFRESH;
    if (!selection.timezone) selection.timezone = timezone;

    const loadedRecent = readJson(storage, STORAGE_RECENT_KEY, []);
    let recent = Array.isArray(loadedRecent) ? loadedRecent.slice(0, RECENT_LIMIT) : [];
    const listeners = new Set();

    function persist() {
      writeJson(storage, STORAGE_STATE_KEY, {
        preset: selection.preset,
        timezone: selection.timezone,
        refreshInterval: selection.refreshInterval,
        startTime: selection.preset === CUSTOM_PRESET ? selection.startTime : null,
        endTime: selection.preset === CUSTOM_PRESET ? selection.endTime : null,
      });
      writeJson(storage, STORAGE_RECENT_KEY, recent);
    }

    function resolved(at) {
      return resolveTimeRange(
        {
          preset: selection.preset,
          timezone: selection.timezone,
          refreshInterval: selection.refreshInterval,
          startTime: selection.startTime,
          endTime: selection.endTime,
        },
        at || clock()
      );
    }

    function emit() {
      const snapshot = getState();
      listeners.forEach((listener) => listener(snapshot));
    }

    function rememberCurrent() {
      const range = resolved();
      const item = Object.freeze({
        preset: range.preset,
        startTime: range.startTime,
        endTime: range.endTime,
        timezone: range.timezone,
        label: presetLabel(range.preset),
      });
      recent = pushRecent(recent, item);
    }

    function applySelection(next, remember) {
      selection = next;
      if (remember) rememberCurrent();
      persist();
      emit();
      return getState();
    }

    function getState() {
      const range = resolved();
      const refresh = createAutoRefresh(range.refreshInterval);
      return Object.freeze({
        startTime: range.startTime,
        endTime: range.endTime,
        preset: range.preset,
        timezone: range.timezone,
        refreshInterval: range.refreshInterval,
        refresh,
      });
    }

    return {
      getState,
      getPresets() {
        return PRESETS.slice();
      },
      getRefreshIntervals() {
        return REFRESH_INTERVALS.slice();
      },
      getRecentlyUsed() {
        return recent.slice();
      },
      setPreset(presetId) {
        requirePreset(presetId);
        if (presetId === CUSTOM_PRESET) {
          throw new Error("Use setCustomRange for a custom time range");
        }
        return applySelection(
          {
            preset: presetId,
            timezone: selection.timezone,
            refreshInterval: selection.refreshInterval,
            startTime: null,
            endTime: null,
          },
          true
        );
      },
      setCustomRange(startTime, endTime) {
        const custom = createCustomRange(startTime, endTime);
        return applySelection(
          {
            preset: CUSTOM_PRESET,
            timezone: selection.timezone,
            refreshInterval: selection.refreshInterval,
            startTime: custom.startTime,
            endTime: custom.endTime,
          },
          true
        );
      },
      setTimezone(nextTimezone) {
        if (!nextTimezone || typeof nextTimezone !== "string") {
          throw new Error("Timezone is required");
        }
        zonedParts(new Date(), nextTimezone);
        return applySelection({ ...selection, timezone: nextTimezone }, false);
      },
      setRefreshInterval(intervalId) {
        requireRefresh(intervalId);
        return applySelection({ ...selection, refreshInterval: intervalId }, false);
      },
      applyRecent(item) {
        if (!item || !item.preset) {
          throw new Error("Recently used item is required");
        }
        if (item.preset === CUSTOM_PRESET) {
          return this.setCustomRange(item.startTime, item.endTime);
        }
        if (item.timezone && item.timezone !== selection.timezone) {
          selection = { ...selection, timezone: item.timezone };
        }
        return this.setPreset(item.preset);
      },
      subscribe(listener) {
        if (typeof listener !== "function") {
          throw new Error("Listener must be a function");
        }
        listeners.add(listener);
        return function unsubscribe() {
          listeners.delete(listener);
        };
      },
      resolve(at) {
        return resolved(at);
      },
    };
  }

  let singleton = null;

  function getGlobalTimeStore(options) {
    if (!singleton) singleton = createTimeStore(options);
    return singleton;
  }

  function resetGlobalTimeStore() {
    singleton = null;
  }

  return {
    STORAGE_STATE_KEY,
    STORAGE_RECENT_KEY,
    RECENT_LIMIT,
    PRESETS,
    REFRESH_INTERVALS,
    createMemoryStorage,
    createCustomRange,
    createAutoRefresh,
    resolveTimeRange,
    createTimeStore,
    getGlobalTimeStore,
    resetGlobalTimeStore,
  };
});
