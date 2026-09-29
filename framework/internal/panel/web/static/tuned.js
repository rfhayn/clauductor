"use strict";
// The Tuned theme (PANEL-11), after Linear's generated themes: every colour token is
// computed in CIE LCh from three inputs, a base hue, an accent hue and a contrast
// (0 to 1), for light and for dark. Each text colour is walked in lightness until it
// meets its floor against every surface it sits on, so any input meets the contrast
// floors, and contrast 1 holds text to AAA (7:1). Separators sit 6 L from the ground.
// A pure function: theme.js calls it before the first paint, and themes_test.go runs
// it in node over a grid of inputs. themes.css carries its output for the defaults.
(function (root) {
  const DEFAULTS = { hue: 265, accent: 255, contrast: 0.35 };
  function lch(L, C, H) {
    const h = (H * Math.PI) / 180, a = C * Math.cos(h), b = C * Math.sin(h);
    const fy = (L + 16) / 116, fx = fy + a / 500, fz = fy - b / 200;
    const inv = (t) => (t ** 3 > 0.008856 ? t ** 3 : (t - 16 / 116) / 7.787);
    const X = 0.95047 * inv(fx), Y = L > 8 ? fy ** 3 : L / 903.3, Z = 1.08883 * inv(fz);
    const lin = [3.2406 * X - 1.5372 * Y - 0.4986 * Z, -0.9689 * X + 1.8758 * Y + 0.0415 * Z, 0.0557 * X - 0.204 * Y + 1.057 * Z];
    const gam = (v) => { v = Math.max(0, Math.min(1, v)); return v <= 0.0031308 ? 12.92 * v : 1.055 * v ** (1 / 2.4) - 0.055; };
    return "#" + lin.map((v) => Math.round(gam(v) * 255).toString(16).padStart(2, "0")).join("");
  }
  function lum(hex) {
    const c = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((v) => (v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
    return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
  }
  function ratio(a, b) { const x = lum(a), y = lum(b); return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05); }
  // fit walks L from L0 in dir (-1 darker, +1 lighter) until the colour meets target
  // on every surface in bgs.
  function fit(L0, C, H, bgs, target, dir) {
    for (let L = L0; L >= 0 && L <= 100; L += dir * 0.5) {
      const c = lch(L, C, H);
      if (bgs.every((b) => ratio(c, b) >= target)) return c;
    }
    return dir < 0 ? "#000000" : "#ffffff";
  }
  function tokens(inp, mode) {
    const o = Object.assign({}, DEFAULTS, inp || {});
    const hue = +o.hue, acc = +o.accent, k = Math.max(0, Math.min(1, +o.contrast));
    const dark = mode === "dark", d = dark ? 1 : -1; // text moves away from the ground
    const gL = dark ? 13 : 97;
    const ground = lch(gL, 2, hue), pane = lch(gL + (dark ? 3 : 0), 2, hue), pane2 = lch(gL + (dark ? -3 : -3), 2.5, hue);
    const rail = lch(gL + (dark ? 0 : -2), 2, hue);
    const surf = [ground, pane, pane2, rail];
    const text = (t, C) => fit(dark ? 55 : 45, C, hue, surf, t, d);
    const t2 = 4.5 + 2.5 * k;
    const x = {
      ground, pane, "pane-2": pane2, zebra: pane, rail, bar: pane,
      rule: lch(gL + (dark ? 9 : -6), 3, hue), "rule-strong": fit(dark ? 40 : 60, 4, hue, [pane], 3, d),
      "text-1": text(12 + 6 * k, 3), "text-2": text(Math.max(t2, 6.5), 4), "text-3": text(t2, 3),
      act: fit(dark ? 60 : 40, 45, acc, surf, t2, d), info: fit(dark ? 60 : 40, 55, 330, surf, 3, d),
      warn: fit(dark ? 70 : 40, 60, 70, surf, t2, d), crit: fit(dark ? 60 : 40, 65, 28, surf, t2, d),
      "warn-bg": lch(dark ? 24 : 90, 22, 80), "crit-bg": lch(dark ? 22 : 90, 20, 25),
      sel: lch(dark ? 28 : 86, 16, acc),
      scrim: dark ? "rgba(0,0,0,.6)" : "rgba(0,0,0,.35)",
    };
    x.ok = x["text-2"];
    x["bar-text"] = x["text-1"]; x["bar-text-2"] = x["text-2"];
    x.focus = x.act;
    x["act-ink"] = ratio("#ffffff", x.act) >= ratio("#000000", x.act) ? "#ffffff" : "#000000";
    x["warn-ink"] = fit(dark ? 85 : 20, 10, 70, [x["warn-bg"]], t2, d);
    x["crit-ink"] = fit(dark ? 85 : 20, 10, 25, [x["crit-bg"]], t2, d);
    x["sel-text"] = fit(dark ? 85 : 15, 3, hue, [x.sel], t2, d);
    // At full contrast the terminal goes AAA too: white on black, every ANSI colour 7:1
    // on it, and xterm lifts anything a program draws to 7:1.
    if (k >= 1) {
      Object.assign(x, { "term-bg": "#000000", "term-fg": "#ffffff", "term-cursor": "#ffff00", "term-selection": "rgba(255,255,0,.3)", "term-min-contrast": "7" });
      ["#5a5a5a", "#ffc7c7", "#32f453", "#edd600", "#bdd6ff", "#ffc0ff", "#28eaff", "#e6e6e6", "#989898", "#ffdddd", "#99faa9", "#ffe70a", "#d7e6ff", "#ffd9ff", "#92f4ff", "#ffffff"]
        .forEach((c, i) => { x["ansi-" + i] = c; });
    }
    return x;
  }
  root.PanelTuned = { defaults: DEFAULTS, tokens, ratio };
})(typeof window !== "undefined" ? window : globalThis);
