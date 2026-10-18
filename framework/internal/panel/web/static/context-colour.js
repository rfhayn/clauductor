"use strict";
// PANEL-48 (D7, D13): a lane's context bar. The state carries each lane's own
// auto-compact tick and context alert threshold, for the window the lane was launched
// with; this decides, from them, the bar's tick and whether its figure is drawn in the
// warning colour. panel.js draws; TestContextColour runs this in node.
(function (root) {
  // Drawn in the warning colour: a figure at or above its lane's alert threshold. A
  // threshold of 0 is the alert turned off (alerts.context_pct 0), so it colours
  // nothing; a missing figure or threshold colours nothing either.
  function warns(ctx, alertPct) {
    return ctx != null && alertPct != null && alertPct > 0 && ctx >= alertPct;
  }
  // The bar's tick: the lane's own. The page-wide one only for state from an older
  // server, whose lanes carry none.
  function tick(laneTick, pageTick) {
    return laneTick != null && laneTick > 0 ? laneTick : pageTick;
  }
  // The lane row's figure and its marks, from one view, as panel.js's laneCtx picks
  // the figure: the worktree lane's when it has one, else the terminal lane's.
  function laneMarks(lv, t) {
    const o = lv && lv.ctxPct != null ? lv : t || {};
    return { ctx: o.ctxPct != null ? o.ctxPct : null, tick: o.ctxTick, alertPct: o.ctxAlertPct };
  }
  root.ContextColour = { warns, tick, laneMarks };
})(window);
