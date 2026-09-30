"use strict";
// PANEL-14: links in a lane's terminal. xterm.js links only OSC 8 by itself; this finds
// the plain-text URLs in a line for panel.js's link provider, and says whether a link
// may open without asking. Terminal output is untrusted: only http(s) ever opens.
// TestTermLinks runs it in node.
(function (root) {
  // A URL ends at whitespace, a quote, an angle bracket or a backtick: none is in a URL
  // a program prints, and each often closes one ("<https://…>", "`https://…`").
  const URL_RE = /\bhttps?:\/\/[^\s"'<>`]+/g;
  const PAIRS = { ")": "(", "]": "[", "}": "{" };
  const count = (s, ch) => s.split(ch).length - 1;
  // Trailing punctuation ends the sentence, and a closing bracket belongs to one opened
  // before the URL, unless the URL opened it too (…/wiki/Go_(language)).
  function trim(u) {
    for (;;) {
      const c = u[u.length - 1];
      if (".,;:!?*".includes(c) || (PAIRS[c] && count(u, PAIRS[c]) < count(u, c))) u = u.slice(0, -1);
      else return u;
    }
  }
  // The normalised http(s) URL, or null for anything else.
  function webURL(s) {
    let u;
    try { u = new URL(s); } catch (e) { return null; }
    return u.protocol === "http:" || u.protocol === "https:" ? u.href : null;
  }
  // [{start, end, url}] with end exclusive, as indexes into text.
  function findURLs(text) {
    const out = [];
    for (const m of text.matchAll(URL_RE)) {
      const u = trim(m[0]);
      if (webURL(u)) out.push({ start: m.index, end: m.index + u.length, url: u });
    }
    return out;
  }
  // Whether the text on screen is the link's own target. An OSC 8 link can print any
  // words over any URL, so only one that shows where it goes opens without asking.
  function showsTarget(text, uri) {
    const t = webURL(uri);
    return !!t && webURL(String(text).trim()) === t;
  }
  root.TermLinks = { findURLs, webURL, showsTarget };
})(window);
