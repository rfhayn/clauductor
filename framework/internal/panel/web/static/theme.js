"use strict";
// Loaded first, synchronously, from <head>: it sets data-theme and data-mode on <html>
// before the first paint, so a stored theme never flashes the default. The CSP allows
// no inline script, so this is a file of its own. panel.js draws the picker from
// PanelTheme and listens for "panel-theme" to re-theme the terminals.
//
// THEMES is the authority for which themes exist: themes_test.go reads it and fails if
// themes.css lacks a block for any id here (or has one for an id not here). aaa marks a
// theme held to WCAG AAA (7:1) text contrast instead of AA.
const THEMES = [
  { id: "console", name: "Console", note: "Instrument panel at night" },
  { id: "chartroom", name: "Chart room", note: "Nautical chart; dimmed night mode" },
  { id: "ward", name: "Ward monitor", note: "A central monitoring station" },
  { id: "duplicator", name: "Duplicator", note: "Typed dispatch forms, violet ink" },
  { id: "contrast", name: "High contrast", note: "AAA text, heavy rules, largest type", aaa: true },
  { id: "shopfloor", name: "Shop floor", note: "Safety signage, stencil, hazard edge" },
];
const MODES = ["system", "light", "dark"];

window.PanelTheme = (function () {
  const root = document.documentElement;
  const KEY_THEME = "clauductor-panel-theme", KEY_MODE = "clauductor-panel-mode";
  const dark = window.matchMedia ? window.matchMedia("(prefers-color-scheme: dark)") : null;
  let theme = "console", mode = "system";
  // Storage can be missing or throw (a private window, blocked site data): the page
  // then renders the default and the picker still works for this visit.
  try {
    const t = localStorage.getItem(KEY_THEME), m = localStorage.getItem(KEY_MODE);
    if (THEMES.some((x) => x.id === t)) theme = t;
    if (MODES.includes(m)) mode = m;
  } catch (e) {}

  function resolved() {
    if (mode !== "system") return mode;
    // The console was designed dark-first, so with no OS preference it stays dark.
    return dark && !dark.matches && window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
  }
  function apply() {
    root.setAttribute("data-theme", theme);
    root.setAttribute("data-mode", resolved());
    root.setAttribute("data-mode-choice", mode);
    document.dispatchEvent(new CustomEvent("panel-theme", { detail: { theme, mode, resolved: resolved() } }));
  }
  function save() {
    try { localStorage.setItem(KEY_THEME, theme); localStorage.setItem(KEY_MODE, mode); } catch (e) {}
  }
  if (dark) {
    const onOS = () => { if (mode === "system") apply(); };
    if (dark.addEventListener) dark.addEventListener("change", onOS); else if (dark.addListener) dark.addListener(onOS);
  }
  apply();
  return {
    themes: THEMES, modes: MODES,
    get: () => ({ theme, mode, resolved: resolved() }),
    setTheme(id) { if (THEMES.some((x) => x.id === id)) { theme = id; save(); apply(); } },
    setMode(m) { if (MODES.includes(m)) { mode = m; save(); apply(); } },
  };
})();
