"use strict";
// xterm's DOM renderer colours a cell with a truecolor value, or with a colour lifted to
// the theme's minimumContrastRatio (--term-min-contrast), through
// span.setAttribute("style", …). The CSP blocks style attributes, so those cells would
// silently keep their palette colour. This routes exactly those writes through CSSOM,
// which the CSP does not govern, and leaves every other style attribute to the CSP:
// - only on a <span> that is still detached (xterm builds a row's cells before it
//   attaches them) or that sits inside an .xterm container;
// - only for a value made of color / background-color declarations with a hex or
//   rgb()/rgba() value. xterm writes "color:#rrggbb[aa];" and "background-color:#rrggbb;"
//   and appends to what getAttribute("style") returns, which CSSOM serialises as rgb().
// Anything else (url(), position, any other property) goes to the real setAttribute,
// which the CSP refuses. xterm-style.test.js exercises both sides.
(function (root) {
  const COLOUR_ONLY = /^(\s*(background-)?color:\s*(#[0-9a-fA-F]{3,8}|rgba?\([0-9., ]+\))\s*;?)+\s*$/;
  const allowed = (el, value) => (!el.isConnected || !!el.closest(".xterm")) && COLOUR_ONLY.test(value);
  const setAttr = root.Element.prototype.setAttribute;
  root.HTMLSpanElement.prototype.setAttribute = function (name, value) {
    if (String(name).toLowerCase() === "style" && allowed(this, String(value))) {
      this.style.cssText = String(value);
      return;
    }
    return setAttr.call(this, name, value);
  };
})(window);
