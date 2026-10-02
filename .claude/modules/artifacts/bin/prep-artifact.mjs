#!/usr/bin/env node
// prep-artifact.mjs: the artifacts module's publishing half (ARTIFACT_PUBLISH="claude.ai"). The shared
// claude.ai copies of the docs/*.html pages: prepare one for publishing, report which are stale, and
// record what was published. Registry: ARTIFACT_REGISTRY (default docs/artifacts.json), "pages".
// Run through publish.sh, which passes --root and checks that Node is there; Standing Tee's
// infra/prep-artifact.mjs, with the registry path and the output directory made settings, so the
// hashes it records and compares are byte-for-byte the same.
//
// WHY NODE. The copy is an HTML rewrite (comments dropped outside script, style and textarea; links
// to registered pages rewritten to their artifact URLs; relative images inlined as data URIs), and
// its hash IS the record. A second implementation in sh would be a second hash. Only this half
// needs Node; the currency check, the guard rule and the opener are sh and jq.
//
// It DETECTS; it cannot publish, because the Artifact tool is not reachable from a shell. Skill
// steps are the executor: session-start publishes each STALE page the session owns, the close only
// `--record`s the hash in its PR, and merge-pr publishes after that PR merges, from
// `--ref origin/main`, so a shared copy never runs ahead of `main`.
//
// WHAT "STALE" MEANS. The registry records, per page, the git blob hash of the PUBLISH-READY copy
// as last published, not the hash of the file on disk. The two differ exactly when a page links to
// another registered page: the published copy has those links rewritten to artifact URLs. So a
// change to the page, to the rewrite, or to a linked page's URL all read as STALE.
//
//   prep-artifact.mjs docs/erd.html [--ref origin/main] [--out <path>]
//       Write the publish-ready copy (default <tmpdir>/artifact-publish/<name>); print its path.
//   prep-artifact.mjs --status [--ref <rev> | --worktree]
//       One line per page: OK / STALE / UNREGISTERED / MISSING, each naming its artifact URL, plus
//       a WARN line per link that will be dead in the copy.
//   prep-artifact.mjs --record docs/erd.html [...]
//       Write the hash of the WORKING-TREE copy into the registry (the close, before its merge).
//   prep-artifact.mjs --recorded-in <commit>
//       The pages whose published hash that commit changed (publish.sh answers this in sh).
//
// `--root <dir>` is the repository (publish.sh passes it). Pages are enumerated from the tree
// itself (docs/*.html), never from the registry: the registry is what is checked.

import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { lstatSync, mkdirSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { basename, dirname, join, posix } from "node:path";
import { tmpdir } from "node:os";

const REGISTRY = process.env.ARTIFACT_REGISTRY || "docs/artifacts.json";
const PAGE = /^docs\/[^/]+\.html$/;

function fail(msg) {
  process.stderr.write(`prep-artifact: ${msg}\n`);
  process.exit(2);
}

const args = process.argv.slice(2);
function take(flag) {
  const i = args.indexOf(flag);
  if (i === -1) return undefined;
  const v = args[i + 1];
  if (v === undefined || v.startsWith("--")) fail(`${flag} needs a value`);
  args.splice(i, 2);
  return v;
}
function has(flag) {
  const i = args.indexOf(flag);
  if (i === -1) return false;
  args.splice(i, 1);
  return true;
}

const root = take("--root") ?? process.cwd();
const refArg = take("--ref");
const out = take("--out");
const worktree = has("--worktree");
const status = has("--status");
const record = has("--record");
const recordedIn = take("--recorded-in");
if (recordedIn !== undefined && (status || record || refArg || out || worktree || args.length > 0))
  fail("--recorded-in takes a commit and nothing else");
// Refuse rather than guess: a typo'd `--wortree` falling through to the default subject would
// report on the wrong tree and look exactly like a real answer (review of #304).
if (worktree && refArg) fail("--worktree and --ref are exclusive");
if (status && record) fail("--status and --record are exclusive");
if (status && (out || args.length > 0))
  fail(`--status takes no other arguments: ${args.join(" ")}`);
if (record && (refArg || out || worktree))
  fail("--record reads the working tree only; it takes no --ref, --out or --worktree");
if (!status && !record && worktree) fail("--worktree is a --status mode; prep reads it by default");
const unknown = args.filter((a) => a.startsWith("--"));
if (unknown.length > 0) fail(`unknown option ${unknown.join(" ")}`);

// A SOURCE is where page bytes and the registry are read from: a git revision, or the files on
// disk. latin1 round-trips every byte, so a page with nothing to rewrite hashes to its git blob.
function gitSource(ref) {
  const git = (a) =>
    execFileSync("git", ["-C", root, ...a], {
      stdio: ["ignore", "pipe", "pipe"],
      maxBuffer: 1 << 28,
    });
  let sha;
  try {
    sha = git(["rev-parse", "--verify", "--quiet", `${ref}^{commit}`])
      .toString()
      .trim();
  } catch {
    fail(`cannot resolve ${ref} in ${root}`);
  }
  return {
    subject: `${ref} @ ${sha.slice(0, 9)}`,
    pages: () =>
      git(["ls-tree", "--name-only", sha, "docs/"])
        .toString()
        .split("\n")
        .filter((p) => PAGE.test(p)),
    read: (p) => {
      try {
        return git(["show", `${sha}:${p}`]).toString("latin1");
      } catch {
        return null;
      }
    },
    // A regular file only (mode 100644/100755), like the disk source: `git show` on a symlink
    // returns its target TEXT, which would be inlined as a bogus image.
    readBytes: (p) => {
      try {
        const mode = git(["ls-tree", sha, "--", p]).toString().split(" ")[0];
        return mode === "100644" || mode === "100755" ? git(["show", `${sha}:${p}`]) : null;
      } catch {
        return null;
      }
    },
    // The day the commit that wrote this hash into the registry landed. A shared copy last updated
    // BEFORE that day was recorded and never published; only the Artifact tool's listing can say
    // when it was updated, so the session-start step makes the comparison.
    recordedOn: (hash) => {
      try {
        return git(["log", "-1", "--format=%cs", `-S${hash}`, sha, "--", REGISTRY])
          .toString()
          .trim();
      } catch {
        return "";
      }
    },
  };
}
function diskSource() {
  let head = "";
  try {
    head = execFileSync("git", ["-C", root, "rev-parse", "--short=9", "HEAD"], {
      stdio: ["ignore", "pipe", "ignore"],
    })
      .toString()
      .trim();
  } catch {}
  return {
    subject: `the working tree${head ? ` (HEAD ${head}, uncommitted edits included)` : ""}`,
    pages: () =>
      readdirSync(join(root, "docs"))
        .map((f) => `docs/${f}`)
        .filter((p) => PAGE.test(p))
        .sort(),
    read: (p) => {
      try {
        return readFileSync(join(root, p), "latin1");
      } catch {
        return null;
      }
    },
    // A regular file only: git stores a symlink as its target TEXT, so following it here would
    // hash differently from --ref and read outside docs/.
    readBytes: (p) => {
      try {
        return lstatSync(join(root, p)).isFile() ? readFileSync(join(root, p)) : null;
      } catch {
        return null;
      }
    },
  };
}

function loadRegistry(src) {
  const text = src.read(REGISTRY);
  if (text === null) fail(`${REGISTRY} not found in ${src.subject}`);
  let reg;
  try {
    reg = JSON.parse(Buffer.from(text, "latin1").toString("utf8"));
  } catch (e) {
    fail(`${REGISTRY} is not valid JSON: ${e.message}`);
  }
  if (!reg || typeof reg.pages !== "object") fail(`${REGISTRY} has no "pages" object`);
  return reg;
}

// Rewrite every href that resolves (relative to the page) to ANOTHER registered page into that
// page's artifact URL, keeping any #fragment, and open it in a new tab: the artifact renders in a
// frame that a claude.ai page will not load inside. A link to the page itself becomes its bare
// fragment. Anything else relative is left alone and WARNED about, because it will not resolve in
// the artifact either.
//
// Tags are tokenised attribute by attribute (quoted values may hold `>`), and only an attribute
// NAMED `href` is touched: replacing by string search rewrote the first matching attribute, which
// could be a `data-*` value, and left the href relative with no warning (review of #304).
const TAG =
  /<([a-zA-Z][\w:-]*)((?:\s+[^\s=<>"'/]+(?:\s*=\s*(?:"[^"]*"|'[^']*'|[^\s"'<>]+))?)*)(\s*\/?)>/g;
// `<` is excluded from names and unquoted values in both, so a failed match (`i<n` in a script)
// stops at the next `<` instead of scanning to the end of the page: that was 21 s on 400 KB.
const ATTR = /(\s+)([^\s=<>"'/]+)(?:(\s*=\s*)("[^"]*"|'[^']*'|[^\s"'<>]+))?/g;
const IMAGE_TYPES = {
  avif: "image/avif",
  gif: "image/gif",
  ico: "image/x-icon",
  jpeg: "image/jpeg",
  jpg: "image/jpeg",
  png: "image/png",
  svg: "image/svg+xml",
  webp: "image/webp",
};
// HTML COMMENTS ARE FOR THE REPO, NOT THE READER (2026-09-27). The pages carry long source comments
// (where each date came from, which test asserts what, what a region is generated from) that
// nobody reading the shared copy should have to download, and that went stale in the copy where
// nobody could see them: roadmap.html's said no 2C gate had started, three merges after one had.
// The copy drops them; the repo file keeps every one. One left-to-right scan, so a comment is never
// looked for INSIDE a <script>, <style> or <textarea> (where `<!--` can be ordinary text), and a
// `<script>` mentioned inside a comment is part of the comment. An unterminated `<!--` matches
// nothing and is left as it is.
// `<!-->` and `<!--->` are complete, empty comments in HTML (the parser closes them abruptly); the
// first alternative takes them, or the lazy one would run on to the NEXT `-->` and eat what lies
// between (review of #371).
const COMMENT_OR_RAW = /<!---?>|<!--[\s\S]*?-->|<(script|style|textarea)\b[\s\S]*?<\/\1\s*>/gi;
const stripHtmlComments = (html) =>
  html.replace(COMMENT_OR_RAW, (m) => (m.startsWith("<!--") ? "" : m));

function prep(page, raw, reg, src) {
  const warnings = [];
  const html = stripHtmlComments(raw);
  const body = html.replace(TAG, (tag, name, attrs, close) => {
    let rewrote = false;
    let hasTarget = false;
    const next = attrs.replace(ATTR, (a, sp, key, eq, raw) => {
      if (/^target$/i.test(key)) hasTarget = true;
      const isSrc = /^src$/i.test(key);
      if ((!isSrc && !/^href$/i.test(key)) || raw === undefined) return a;
      const q = /^["']/.test(raw) ? raw[0] : "";
      const href = q ? raw.slice(1, -1) : raw;
      if (/^([a-z][a-z0-9+.-]*:|#|\/\/)/i.test(href) || href === "") return a;
      if (href.startsWith("/")) {
        warnings.push(`${page}: root-relative link "${href}" will not resolve in the artifact`);
        return a;
      }
      const [, path, frag] = href.match(/^([^#?]*)(.*)$/);
      const target = posix.normalize(posix.join(posix.dirname(page), path));
      // A relative image is inlined: the artifact has no files beside it for the path to reach.
      // Only as an `src` or a `<link href>` (an icon): an `<a href>` to a data: URI is a link
      // browsers refuse to navigate to, so it falls through to the dead-link warning below.
      const type = IMAGE_TYPES[path.match(/\.([a-z0-9]+)$/i)?.[1].toLowerCase() ?? ""];
      if (type && (isSrc || /^link$/i.test(name))) {
        const bytes = target.startsWith("docs/") ? src.readBytes(target) : null;
        if (!bytes) {
          warnings.push(
            `${page}: image "${href}" is not a file under docs/ — it will not resolve in the artifact`,
          );
          return a;
        }
        return `${sp}${key}${eq}${q || '"'}data:${type};base64,${bytes.toString("base64")}${q || '"'}`;
      }
      if (isSrc) {
        warnings.push(
          `${page}: relative src "${href}" is not an image it can inline — it will not resolve in the artifact`,
        );
        return a;
      }
      // A self-link keeps only its fragment: `?q=1` would not stay on the page inside the artifact.
      const hash = frag.includes("#") ? frag.slice(frag.indexOf("#")) : "#";
      if (target === page) return `${sp}${key}${eq}${q || '"'}${hash}${q || '"'}`;
      const entry = reg.pages[target];
      if (!entry) {
        warnings.push(
          `${page}: relative link "${href}" is not a registered page — it will not resolve in the artifact`,
        );
        return a;
      }
      rewrote = /^a$/i.test(name);
      return `${sp}${key}${eq}${q || '"'}${entry.url}${frag}${q || '"'}`;
    });
    const extra = rewrote && !hasTarget ? ' target="_blank" rel="noopener"' : "";
    return `<${name}${extra}${next}${close}>`;
  });
  // Safety net for any shape the tokeniser missed: a registered page's file name still sitting in
  // an href is a dead link in the artifact, and it is named rather than shipped quietly.
  for (const p of Object.keys(reg.pages)) {
    const esc = posix.basename(p).replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    if (
      new RegExp(`(?<![\\w-])href\\s*=\\s*["']?(\\.{0,2}/)*(docs/)?${esc}(?=["'#?\\s>])`, "i").test(
        body,
      )
    )
      warnings.push(
        `${page}: a link to ${p} survived the rewrite — it will not resolve in the artifact`,
      );
  }
  return { body, warnings };
}

const blob = (latin1) => {
  const buf = Buffer.from(latin1, "latin1");
  return createHash("sha1").update(`blob ${buf.length}\0`).update(buf).digest("hex");
};

if (status) {
  const src = worktree ? diskSource() : gitSource(refArg ?? "origin/main");
  const reg = loadRegistry(src);
  const onDisk = src.pages();
  console.log(`Shared copies vs ${src.subject}; registry read from the same place:`);
  let stale = 0;
  let warned = 0;
  for (const p of onDisk) {
    const e = reg.pages[p];
    if (!e) {
      console.log(
        `UNREGISTERED ${p}: no shared copy. Publish a NEW artifact from the owner's session and add its URL + hash`,
      );
      stale++;
      continue;
    }
    const html = src.read(p);
    if (html === null) fail(`cannot read ${p} from ${src.subject}`);
    const { body, warnings } = prep(p, html, reg, src);
    for (const w of warnings) console.log(`WARN  ${w}`);
    warned += warnings.length;
    const now = blob(body);
    const on = now === e.published && src.recordedOn ? src.recordedOn(e.published) : "";
    if (now === e.published) console.log(`OK    ${p} → ${e.url}${on ? ` (recorded ${on})` : ""}`);
    else {
      console.log(
        `STALE ${p} → ${e.url} (published ${String(e.published).slice(0, 9)}, now ${now.slice(0, 9)})`,
      );
      stale++;
    }
  }
  for (const p of Object.keys(reg.pages)
    .filter((k) => !onDisk.includes(k))
    .sort()) {
    console.log(
      `MISSING ${p} is registered but absent from ${src.subject}: drop its entry → ${reg.pages[p].url}`,
    );
    stale++;
  }
  console.log(
    stale === 0
      ? `All ${onDisk.length} shared copies are current${warned ? `, with ${warned} WARN line(s) above` : ""}.`
      : `${stale} of the lines above need action; session-start and merge-pr publish (session-close only records).`,
  );
  process.exit(0);
}

// Which pages did this commit record? The post-merge publish derives its list from the merge
// commit, never from a session's memory: a list kept in memory is gone after a compaction or a
// merge made from another session, and then main claims a copy that was never published.
if (recordedIn !== undefined) {
  // No registry before this commit means this is the commit that introduced it, and its entries are
  // SEEDS describing copies already live when it was written, not records awaiting a publish.
  let hasParent = true;
  try {
    execFileSync(
      "git",
      ["-C", root, "rev-parse", "--verify", "--quiet", `${recordedIn}^{commit}`],
      {
        stdio: "ignore",
      },
    );
    execFileSync("git", ["-C", root, "rev-parse", "--verify", "--quiet", `${recordedIn}^`], {
      stdio: "ignore",
    });
  } catch {
    hasParent = false;
  }
  if (!hasParent) gitSource(recordedIn); // an unresolvable commit fails loudly here
  const parent = hasParent ? gitSource(`${recordedIn}^`) : { read: () => null };
  if (parent.read(REGISTRY) === null) {
    process.stderr.write(
      `${REGISTRY} does not exist before ${recordedIn}: its entries are seeds, nothing to publish\n`,
    );
    process.exit(0);
  }
  const before = loadRegistry(parent);
  const after = loadRegistry(gitSource(recordedIn));
  for (const [p, e] of Object.entries(after.pages).sort(([a], [b]) => a.localeCompare(b)))
    if (before.pages[p]?.published !== e.published) console.log(`${p} ${e.url}`);
  process.exit(0);
}

if (record) {
  if (args.length === 0) fail("--record needs at least one page");
  const src = diskSource();
  const reg = loadRegistry(src);
  for (const p of args) {
    if (!reg.pages[p]) fail(`${p} has no registry entry`);
    const html = src.read(p);
    if (html === null) fail(`${p} not found`);
    reg.pages[p].published = blob(prep(p, html, reg, src).body);
    console.log(`recorded ${p} = ${reg.pages[p].published.slice(0, 9)}`);
  }
  writeFileSync(join(root, REGISTRY), `${JSON.stringify(reg, null, 2)}\n`);
  process.exit(0);
}

if (args.length !== 1 || !PAGE.test(args[0]))
  fail("usage: prep-artifact.mjs docs/<page>.html [--ref <rev>] [--out <path>]");
const page = args[0];
const src = refArg ? gitSource(refArg) : diskSource();
const reg = loadRegistry(src);
if (!reg.pages[page]) fail(`${page} has no entry in ${REGISTRY}`);
const html = src.read(page);
if (html === null) fail(`${page} not found in ${src.subject}`);
const { body, warnings } = prep(page, html, reg, src);
for (const w of warnings) process.stderr.write(`warning: ${w}\n`);
const dest = out ?? join(tmpdir(), "artifact-publish", basename(page));
mkdirSync(dirname(dest), { recursive: true });
writeFileSync(dest, body, "latin1");
console.log(dest);
