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
// - Type: the faces and type sizes (types.css). "Match theme" (no choice stored) uses
//   the type system the theme pairs with; any theme works with any type system.
// - Size: the page's text size (--ui-scale, below) and the terminal's (panel.js).
//
// THEMES and TYPES are the authority for which exist: themes_test.go reads both and
// fails if themes.css or types.css lacks a block for any id here (or has one for an
// id not here). aaa marks a theme held to WCAG AAA (7:1) text contrast instead of AA.
// type is the type system a theme pairs with; wide is its text size on a window at
// least 1440 px wide when you have not chosen one (110% unless it says otherwise):
// High contrast is already the largest type, so it stays at 100%.
const THEMES = [
  { id: "console", name: "Console", note: "Instrument panel at night", type: "instrument" },
  { id: "chartroom", name: "Chart room", note: "Nautical chart; dimmed night mode", type: "chart" },
  { id: "ward", name: "Ward monitor", note: "A central monitoring station", type: "clinical" },
  { id: "duplicator", name: "Duplicator", note: "Typed dispatch forms, violet ink", type: "typewriter" },
  { id: "contrast", name: "High contrast", note: "AAA text, heavy rules, largest type", aaa: true, type: "hyperlegible", wide: 100 },
  { id: "shopfloor", name: "Shop floor", note: "Safety signage, hazard edge", type: "stencil" },
];
// Placeholders: the faces the six themes shipped with, until the type systems are chosen.
const TYPES = [
  { id: "instrument", name: "Instrument", note: "Chakra Petch · IBM Plex Sans · JetBrains Mono" },
  { id: "chart", name: "Chart", note: "Newsreader · DM Mono" },
  { id: "clinical", name: "Clinical", note: "Barlow · Red Hat Mono" },
  { id: "typewriter", name: "Typewriter", note: "Courier Prime" },
  { id: "hyperlegible", name: "Hyperlegible", note: "Atkinson Hyperlegible Next · Mono" },
  { id: "stencil", name: "Stencil", note: "Big Shoulders · Archivo · Martian Mono" },
];
const MODES = ["system", "light", "dark"];

window.PanelTheme = (function () {
  const root = document.documentElement;
  const KEY_THEME = "clauductor-panel-theme", KEY_MODE = "clauductor-panel-mode", KEY_TYPE = "clauductor-panel-type";
  const dark = window.matchMedia ? window.matchMedia("(prefers-color-scheme: dark)") : null;
  let theme = "console", mode = "system", type = null; // type null: match the theme
  // Storage can be missing or throw (a private window, blocked site data): the page
  // then renders the default and the menu still works for this visit.
  try {
    const t = localStorage.getItem(KEY_THEME), m = localStorage.getItem(KEY_MODE), y = localStorage.getItem(KEY_TYPE);
    if (THEMES.some((x) => x.id === t)) theme = t;
    if (MODES.includes(m)) mode = m;
    if (TYPES.some((x) => x.id === y)) type = y;
  } catch (e) {}

  function resolved() {
    if (mode !== "system") return mode;
    // The console was designed dark-first, so with no OS preference it stays dark.
    return dark && !dark.matches && window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
  }
  function typeOf() { return type || THEMES.find((x) => x.id === theme).type; }
  function state() { return { theme, mode, resolved: resolved(), type: typeOf(), typeChoice: type }; }
  function apply() {
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
    // null matches the theme.
    setType(id) { if (id === null || TYPES.some((x) => x.id === id)) { type = id; save(); apply(); } },
  };
})();

// The page's text size: one factor, --ui-scale, multiplies the root size, and every
// size in panel.css is in rem, so the whole page scales with it. It is set here, before
// the first paint, through CSSOM (the CSP refuses style attributes, not CSSOM). Stored
// per browser; with nothing stored, the theme's `wide` on a window at least 1440 px
// wide, 100% below. panel.js draws the controls and the keys (Ctrl+Alt+= and
// Ctrl+Alt+−; the browser's own zoom keys stay the browser's).
window.PanelScale = (function () {
  const root = document.documentElement;
  const KEY = "clauductor-panel-scale";
  const STEPS = [85, 90, 100, 110, 120, 130, 145, 160];
  const auto = () => {
    const th = THEMES.find((x) => x.id === window.PanelTheme.get().theme);
    return window.innerWidth >= 1440 ? (th && th.wide) || 110 : 100;
  };
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
