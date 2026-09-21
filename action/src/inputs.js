"use strict";

const fs = require("node:fs");
const path = require("node:path");
const core = require("@actions/core");

const MODES = new Set(["check", "plan", "apply", "upgrade", "setup"]);

function parseBoolean(value, name) {
  const normalized = String(value).trim().toLowerCase();
  if (normalized === "true") return true;
  if (normalized === "false") return false;
  throw new Error(`${name} must be "true" or "false"`);
}

function resolveWorkingDirectory(workspace, requested) {
  if (!workspace) throw new Error("GITHUB_WORKSPACE is not set; check out the repository before running Sanad");
  const workspaceRoot = fs.realpathSync(workspace);
  if (!fs.existsSync(path.join(workspaceRoot, ".git"))) {
    throw new Error("GITHUB_WORKSPACE does not contain a checked-out repository; run actions/checkout before Sanad");
  }
  const candidate = fs.realpathSync(path.resolve(workspaceRoot, requested || "."));
  const relative = path.relative(workspaceRoot, candidate);
  if (relative === ".." || relative.startsWith(`..${path.sep}`) || path.isAbsolute(relative)) {
    throw new Error("working-directory must stay inside GITHUB_WORKSPACE");
  }
  const stat = fs.statSync(candidate);
  if (!stat.isDirectory()) throw new Error("working-directory must name a directory");
  return candidate;
}

function resolveConfigPath(workingDirectory, requested) {
  if (!requested) throw new Error("config must not be empty");
  if (path.isAbsolute(requested)) throw new Error("config must be relative to working-directory");
  const resolved = path.resolve(workingDirectory, requested);
  const relative = path.relative(workingDirectory, resolved);
  if (relative === ".." || relative.startsWith(`..${path.sep}`) || path.isAbsolute(relative)) {
    throw new Error("config must stay inside working-directory");
  }
  return requested;
}

// Resolve the effective mode:
//   If the user set mode explicitly, use it.
//   If write=true and mode is blank, default to "apply" (PR-ready write).
//   Otherwise default to "check".
function resolveMode(rawMode, write) {
  const trimmed = String(rawMode || "").trim().toLowerCase();
  if (trimmed) return trimmed;
  return write ? "apply" : "check";
}

function validateInputs(values) {
  if (!MODES.has(values.mode)) {
    throw new Error(`mode must be one of: ${Array.from(MODES).join(", ")}`);
  }
  // fresh and strict only do anything on check; warn rather than error for other modes.
  if (values.mode !== "check") {
    if (values.fresh) core.warning("fresh is only meaningful with mode=check; ignoring");
    if (values.strict) core.warning("strict is only meaningful with mode=check; ignoring");
  }
  if (values.mode !== "apply" && values.mode !== "upgrade" && values.write) {
    throw new Error("write is valid only with mode=apply or mode=upgrade");
  }
  return values;
}

module.exports = { MODES, parseBoolean, resolveConfigPath, resolveMode, resolveWorkingDirectory, validateInputs };
