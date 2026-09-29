"use strict";
// Loaded first, synchronously, from <head>: it sets data-theme, data-mode, data-type and
// the page's text size on <html> before the first paint, so a stored choice never
// flashes the default. The CSP allows no inline script, so this is a file of its own.
// panel.js draws the Appearance menu from PanelTheme and PanelScale, and listens for
// "panel-theme" and "panel-scale" to re-theme and refit the terminals.
//
// Appearance is three independent choices (PANEL-11), each kept per browser:
// - Theme: colour, surface, border and density (themes.css), in a mode (System, Light,
//   Dark).
// - Type: the faces (types.css). "Theme's choice" (nothing stored) uses the type
//   system the theme suggests; any theme works with any type system.
// - Size: the page's text size (--ui-scale, below) and the terminal's (panel.js).
//
// THEMES and TYPES are the authority for which exist: themes_test.go reads both and
// fails if themes.css or types.css lacks a block for any id here (or has one for an
// id not here). aaa marks a theme held to WCAG AAA (7:1) text contrast instead of AA.
// type is the type system a theme suggests; tuned marks the theme generated from your
// inputs (tuned.js, loaded before this file).
const THEMES = [
  { id: "hmi", name: "Grey HMI", note: "Grey at rest; colour only when abnormal", type: "highway" },
  { id: "amber", name: "Terminal Amber", note: "Black on white, amber on black", type: "cockpit" },
  { id: "cockpit", name: "Glass Cockpit", note: "Nominal green, caution amber, cyan to act", type: "cockpit" },
  { id: "tuned", name: "Tuned", note: "Generated from a hue, an accent and a contrast", type: "civic", tuned: true },
  { id: "tui", name: "TUI", note: "ANSI colours, box-drawn panes, key hints", type: "engineer" },
  { id: "native", name: "System Native", note: "System greys, zebra rows, a sidebar", type: "hyperlegible" },
];
const TYPES = [
  { id: "highway", name: "Highway", note: "Overpass and Overpass Mono" },
  { id: "cockpit", name: "Cockpit", note: "B612 and B612 Mono" },
  { id: "civic", name: "Civic", note: "Public Sans and Commit Mono" },
  { id: "hyperlegible", name: "Hyperlegible", note: "Atkinson Hyperlegible Next and Mono" },
  { id: "engineer", name: "Engineer", note: "Iosevka Aile, Iosevka and Iosevka Term" },
  { id: "variable", name: "Variable", note: "From Mona Sans and Monaspace" },
];
const MODES = ["system", "light", "dark"];

window.PanelTheme = (function () {
  const root = document.documentElement;
  const KEY_THEME = "clauductor-panel-theme", KEY_MODE = "clauductor-panel-mode", KEY_TYPE = "clauductor-panel-type";
  const dark = window.matchMedia ? window.matchMedia("(prefers-color-scheme: dark)") : null;
  const KEY_TUNED = "clauductor-panel-tuned";
  let theme = "hmi", mode = "system", type = null; // type null: the theme's suggestion
  let tuned = Object.assign({}, window.PanelTuned ? window.PanelTuned.defaults : {});
  // Storage can be missing or throw (a private window, blocked site data): the page
  // then renders the default and the menu still works for this visit.
  try {
    const t = localStorage.getItem(KEY_THEME), m = localStorage.getItem(KEY_MODE), y = localStorage.getItem(KEY_TYPE);
    if (THEMES.some((x) => x.id === t)) theme = t;
    if (MODES.includes(m)) mode = m;
    if (TYPES.some((x) => x.id === y)) type = y;
    const tu = JSON.parse(localStorage.getItem(KEY_TUNED) || "null");
    if (tu && typeof tu === "object") for (const k of ["hue", "accent", "contrast"]) if (typeof tu[k] === "number") tuned[k] = tu[k];
  } catch (e) {}

  function resolved() {
    if (mode !== "system") return mode;
    // Follow the system; with no preference at all, light.
    return dark && dark.matches ? "dark" : "light";
  }
  function typeOf() { return type || THEMES.find((x) => x.id === theme).type; }
  function state() { return { theme, mode, resolved: resolved(), type: typeOf(), typeChoice: type, tuned: Object.assign({}, tuned) }; }
  // Tuned's colours are generated and set on <html> through CSSOM, over its defaults in
  // themes.css; any other theme clears them.
  let tunedKeys = [];
  function applyTuned() {
    for (const k of tunedKeys) root.style.removeProperty("--" + k);
    tunedKeys = [];
    const th = THEMES.find((x) => x.id === theme);
    if (!th.tuned || !window.PanelTuned) return;
    const t = window.PanelTuned.tokens(tuned, resolved());
    for (const [k, v] of Object.entries(t)) { root.style.setProperty("--" + k, v); tunedKeys.push(k); }
  }
  function apply() {
    applyTuned();
    root.setAttribute("data-theme", theme);
    root.setAttribute("data-mode", resolved());
    root.setAttribute("data-mode-choice", mode);
    root.setAttribute("data-type", typeOf());
    document.dispatchEvent(new CustomEvent("panel-theme", { detail: state() }));
  }
  function save() {
    try {
      localStorage.setItem(KEY_THEME, theme); localStorage.setItem(KEY_MODE, mode);
      if (type) localStorage.setItem(KEY_TYPE, type); else localStorage.removeItem(KEY_TYPE);
      localStorage.setItem(KEY_TUNED, JSON.stringify(tuned));
    } catch (e) {}
  }
  if (dark) {
    const onOS = () => { if (mode === "system") apply(); };
    if (dark.addEventListener) dark.addEventListener("change", onOS); else if (dark.addListener) dark.addListener(onOS);
  }
  apply();
  return {
    themes: THEMES, types: TYPES, modes: MODES,
    get: state,
    setTheme(id) { if (THEMES.some((x) => x.id === id)) { theme = id; save(); apply(); } },
    setMode(m) { if (MODES.includes(m)) { mode = m; save(); apply(); } },
    // null takes the theme's suggestion.
    setType(id) { if (id === null || TYPES.some((x) => x.id === id)) { type = id; save(); apply(); } },
    // Tuned's inputs: hue and accent 0-360, contrast 0-1.
    setTuned(k, v) {
      if (!["hue", "accent", "contrast"].includes(k) || !isFinite(v)) return;
      tuned[k] = k === "contrast" ? Math.max(0, Math.min(1, +v)) : ((+v % 360) + 360) % 360;
      save(); apply();
    },
  };
})();

// The page's text size: one factor, --ui-scale, multiplies the root size, and every
// size in panel.css is in rem, so the whole page scales with it. It is set here, before
// the first paint, through CSSOM (the CSP refuses style attributes, not CSSOM). Stored
// per browser, 85% to 130%; with nothing stored, 110% on a window at least 1440 px
// wide, 100% below. panel.js draws the controls and the keys (Ctrl+Alt+= and
// Ctrl+Alt+−; the browser's own zoom keys stay the browser's).
window.PanelScale = (function () {
  const root = document.documentElement;
  const KEY = "clauductor-panel-scale";
  const STEPS = [85, 90, 100, 110, 120, 130];
  const auto = () => (window.innerWidth >= 1440 ? 110 : 100);
  let stored = null;
  try {
    const v = parseInt(localStorage.getItem(KEY), 10);
    if (STEPS.includes(v)) stored = v;
  } catch (e) {}
  function current() { return stored || auto(); }
  function apply() {
    root.style.setProperty("--ui-scale", String(current() / 100));
    document.dispatchEvent(new CustomEvent("panel-scale", { detail: { pct: current(), stored: stored !== null } }));
  }
  function save() {
    try { if (stored === null) localStorage.removeItem(KEY); else localStorage.setItem(KEY, String(stored)); } catch (e) {}
  }
  const follow = () => { if (stored === null && root.style.getPropertyValue("--ui-scale") !== String(auto() / 100)) apply(); };
  window.addEventListener("resize", follow);
  document.addEventListener("panel-theme", follow);
  apply();
  return {
    steps: STEPS,
    get: current,
    own: () => stored !== null,
    step(d) {
      const cur = current();
      let i = STEPS.indexOf(cur);
      if (i < 0) i = STEPS.findIndex((x) => x > cur) - (d > 0 ? 1 : 0);
      stored = STEPS[Math.max(0, Math.min(STEPS.length - 1, i + d))];
      save(); apply();
    },
    reset() { stored = null; save(); apply(); },
  };
})();
