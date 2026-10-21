"use strict";
// PANEL-51 (D11): a stalled lane on today's page. The state carries each stalled
// lane's `stall` (since, waitedOn, openWork, approx) on its lane view, and a Needs-you
// item for it: kind "stalled", or the lane's own waiting item relabelled "Stalled".
// This decides, from them, what the page draws: the word, the square and the tint of
// the lane's row, tab and tree node, and the Needs-you row's text, quiet time and
// button. panel.js only puts them in place; TestStallView runs this in node.
(function (root) {
  const APPROX = "≈ ";
  const APPROX_TITLE = "approximate: the stall rests on a reading that is not current, or began across the panel's restart; not notified until it is exact";
  function stallOf(lv) { return (lv && lv.stall) || null; }
  // The lane's marks: base ({ state, word, approx, title }, what the lane reads
  // otherwise) while the lane is not stalled, else "stalled" for the square's and the
  // word's class and for the word, approximate as the stall is; with text, the word as
  // drawn ("≈ " first when approximate). A stall reads over every other state, an
  // exited claude's included: it is the owner's move. No base and no stall: null.
  function mark(lv, base) {
    const st = stallOf(lv);
    const m = st ? { state: "stalled", word: "stalled", approx: !!st.approx, title: st.approx ? APPROX_TITLE : "" } : base;
    return m ? Object.assign({}, m, { text: (m.approx ? APPROX : "") + m.word }) : null;
  }
  // The lane row's tint: a blocking row's ("crit") while stalled, else the one it had.
  function tint(lv, cls) { return stallOf(lv) ? "crit" : cls; }
  // The lane's status in words, for its row's and its tab's accessible name and title:
  // "stalled: <what it waited on>" while stalled, else the words it had.
  function status(lv, words) {
    const st = stallOf(lv);
    if (!st) return words;
    return (st.approx ? APPROX : "") + "stalled" + (st.waitedOn ? ": " + st.waitedOn : "");
  }
  // A Needs-you row's parts when the row is a stall's, else null:
  // - sev: blocking, a relabelled waiting row included;
  // - text: the row's words as drawn, its label then its text (what it last waited on,
  //   then the open work, as the item carries them); a relabelled waiting row keeps its
  //   "not a current reading" note, while a stalled item's label already says why it is
  //   approximate;
  // - since and pre: the quiet time, drawn by panel.js's ticking age as "quiet for 34m"
  //   (a stalled lane sends no update, so words would freeze at the render);
  // - action: Open terminal only where the lane has a terminal; a lane started by hand
  //   has none, and its row only shows the lane.
  function need(n, lanes) {
    const lv = (lanes || []).find((l) => l.path === n.lane);
    const st = stallOf(lv);
    if (!st || !(n.kind === "stalled" || /^Stalled\b/.test(n.label || ""))) return null;
    let text = n.label || "Stalled";
    if (n.text) text += /[?!.]$/.test(text) ? " " + n.text.charAt(0).toUpperCase() + n.text.slice(1) : ": " + n.text;
    if (n.approx && n.kind !== "stalled") text += " (≈ not a current reading)";
    return {
      sev: "crit",
      text,
      since: st.since,
      pre: "quiet for ",
      action: n.terminal ? "Open terminal" : "Show lane",
    };
  }
  root.StallView = { mark, tint, status, need };
})(window);
