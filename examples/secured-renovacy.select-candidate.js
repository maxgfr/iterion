// secured-renovacy `select_candidate` — deterministic survivor pick.
//
// This script is what `examples/secured-renovacy.iter`'s
// `select_candidate` tool runs inside the sandbox. The .iter embeds
// it as a base64 string in the command body (recipe DSL doesn't
// support multi-line strings or raw quotes, and escape-juggling
// every `"` through nested DSL+shell+jq+printf layers is a recurring
// source of fragility). The base64 wrapper sidesteps every escape
// concern: the source file lives here for review/edit, and a tiny
// `node scripts/encode-select-candidate.sh` (or equivalent one-liner)
// re-encodes it into the .iter.
//
// Inputs (env vars set by the .iter shell wrapper):
//   PKGS_FILE  — path to JSON file with `packages` (object keyed by
//                name OR array of {name, current, target, risk} OR
//                a `{list|items|packages|data: [...]}` wrapper).
//   ATT_FILE   — path to JSON file with `attempted` (same shape
//                tolerance as packages, but values are usually just
//                "yes I tried this" markers).
//   SCOPE      — comma-separated risk levels, or "all".
//   MAX        — max packages per run as integer string.
//
// Output: one-line JSON to stdout matching the `candidate` schema.

const fs = require("fs");

function readJSON(path) {
  return JSON.parse(fs.readFileSync(path, "utf8"));
}

function unwrap(value, wrapperKeys) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return value;
  for (const key of wrapperKeys) {
    if (Array.isArray(value[key])) return value[key];
  }
  return value;
}

function normaliseEntries(pkgs) {
  if (Array.isArray(pkgs)) {
    return pkgs.map((p) => ({
      name: p.name || "",
      current: p.current || "",
      target: p.target || "",
      risk: p.risk || "unknown",
    }));
  }
  if (pkgs && typeof pkgs === "object") {
    return Object.entries(pkgs).map(([name, v]) => ({
      name,
      current: (v && v.current) || "",
      target: (v && v.target) || "",
      risk: (v && v.risk) || "unknown",
    }));
  }
  return [];
}

function attemptedNames(atts) {
  if (Array.isArray(atts)) return atts.map((a) => (a && a.name) || a).filter((n) => typeof n === "string" && n);
  if (atts && typeof atts === "object") return Object.keys(atts);
  return [];
}

const packages = unwrap(readJSON(process.env.PKGS_FILE), ["list", "items", "packages", "data"]);
const attempted = unwrap(readJSON(process.env.ATT_FILE), ["list"]) || {};
const scope = (process.env.SCOPE || "").split(",").map((s) => s.trim()).filter(Boolean);
const max = parseInt(process.env.MAX || "0", 10) || 0;

const allowed = scope.includes("all") ? ["patch", "minor", "major", "unknown"] : scope;
const rank = { patch: 1, minor: 2, major: 3, unknown: 4 };

const all = normaliseEntries(packages);
const attemptedList = attemptedNames(attempted);
const attemptedCount = attemptedList.length;

const survivors = all
  .filter((p) => !attemptedList.includes(p.name))
  .filter((p) => allowed.includes(p.risk))
  .sort((a, b) => (rank[a.risk] || 4) - (rank[b.risk] || 4));

let result;
if (attemptedCount >= max || survivors.length === 0) {
  result = {
    selected_package: "",
    current_version: "",
    target_version: "",
    risk: "unknown",
    has_more: false,
    attempted_count: attemptedCount,
    cumulative_attempted: attempted,
    fix_loop_max: 0,
  };
} else {
  const p = survivors[0];
  const cumulative = { ...attempted, [p.name]: { current: p.current, target: p.target, risk: p.risk } };
  result = {
    selected_package: p.name,
    current_version: p.current,
    target_version: p.target,
    risk: p.risk,
    has_more: true,
    attempted_count: attemptedCount,
    cumulative_attempted: cumulative,
    fix_loop_max: p.risk === "major" ? 5 : 3,
  };
}

process.stdout.write(JSON.stringify(result));
