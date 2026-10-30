"use strict";
// PANEL-42: what the page says about a project whose config is outside its repository
// (an attached project's sidecar, design.md D6 and D7). panel.js renders these lines;
// project_config_test.go runs them in node.
(function (root) {
  // What Remove from panel… says today, for a project reading its own config: adding
  // it again from the page finds that config in the repository.
  const KEPT = "This only unregisters it. Nothing on disk is deleted: the repository, its config, its worktrees and its lane registry stay where they are, and adding it again brings it all back.";

  // menuLine is the project menu row's line about its config, from the menu entry
  // (web.ProjectSummary): "" unless the config is outside the repository.
  function menuLine(p) {
    return p && p.config ? "reads " + p.config + ", outside the repository" : "";
  }

  // shellWord is s as one POSIX shell word: bare when it holds only characters no shell
  // reads, else single-quoted, as install.ShellWord quotes `panel remove`'s line.
  function shellWord(s) {
    s = String(s);
    return /^[A-Za-z0-9_@%+=:,.\/-]+$/.test(s) ? s : "'" + s.replace(/'/g, "'\\''") + "'";
  }

  // removeNote is the Remove from panel… confirmation's opening, from the remove plan
  // (live.go, removePlan). For a config outside the repository, adding the project
  // from the page would not find it (it reads only <root>/.stridal/panel.json), so
  // the note names the file and the command that serves it again, its paths quoted so
  // it runs as printed.
  function removeNote(plan) {
    if (!plan || !plan.configOutside) return KEPT;
    return "This only unregisters it. Nothing on disk is deleted: the repository, its worktrees and its lane registry stay where they are, " +
      "and so does its config, " + plan.configPath + ", which is outside the repository. To serve it again from that config, run: " +
      "stridal panel add --project " + shellWord(plan.root) + " --config " + shellWord(plan.configPath);
  }

  root.ProjectConfig = { menuLine, removeNote };
})(window);
