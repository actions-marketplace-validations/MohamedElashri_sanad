"use strict";

const fs = require("node:fs");
const path = require("node:path");
const core = require("@actions/core");
const exec = require("@actions/exec");
const { SANAD_VERSION, REPORT_VERSIONS } = require("./constants");
const { changedManagedFiles, snapshotManagedFiles } = require("./files");
const { installSanad } = require("./installer");
const { parseBoolean, resolveConfigPath, resolveMode, resolveWorkingDirectory, validateInputs } = require("./inputs");
const { annotateDecisions, parseReport, reportMetrics, summarizeReport } = require("./report");

function setCommonOutputs(values = {}) {
  core.setOutput("passed", String(values.passed ?? false));
  core.setOutput("changed", String(values.changed ?? false));
  core.setOutput("changed-files", JSON.stringify(values.changedFiles ?? []));
  core.setOutput("violations", String(values.violations ?? 0));
  core.setOutput("updates", String(values.updates ?? 0));
  core.setOutput("pending-cooldown", String(values.pending ?? 0));
  core.setOutput("report-path", values.reportPath ?? "");
  core.setOutput("pr-body-path", values.prBodyPath ?? "");
  core.setOutput("sanad-version", SANAD_VERSION);
}

function actionInputs() {
  const rawMode = core.getInput("mode").trim().toLowerCase();
  const write = parseBoolean(core.getInput("write", { required: true }), "write");
  const mode = resolveMode(rawMode, write);
  return validateInputs({
    mode,
    config: core.getInput("config", { required: true }).trim(),
    workingDirectory: core.getInput("working-directory", { required: true }).trim(),
    fresh: parseBoolean(core.getInput("fresh", { required: true }), "fresh"),
    strict: parseBoolean(core.getInput("strict", { required: true }), "strict"),
    write,
    token: core.getInput("token"),
  });
}

function commandArguments(inputs, reportPath, prBodyPath) {
  const args = ["--root", inputs.root, "--config", inputs.config, "--format", "json", inputs.mode];
  if (inputs.mode === "check") {
    if (inputs.strict) args.push("--strict");
    else if (inputs.fresh) args.push("--fresh");
  } else if (inputs.mode === "plan") {
    args.push("--out", reportPath, "--pr-body-out", prBodyPath);
  } else if (inputs.mode === "apply") {
    args.push("--pr-body-out", prBodyPath);
    if (inputs.write) args.push("--write", "--yes");
  } else if (inputs.mode === "upgrade") {
    args.push("--all");
    if (inputs.write) args.push("--write", "--yes");
  }
  return args;
}

async function execute(binary, args, workspace, token) {
  const environment = { ...process.env, NO_COLOR: "1", SANAD_COLOR: "never" };
  if (token) environment.GITHUB_TOKEN = token;
  return exec.getExecOutput(binary, args, {
    cwd: workspace,
    env: environment,
    ignoreReturnCode: true,
    silent: true,
  });
}

async function verifySanadVersion(binary, workspace, token) {
  const result = await execute(binary, ["version"], workspace, token);
  const actual = result.stdout.split(/\r?\n/, 1)[0].trim();
  const expected = `sanad ${SANAD_VERSION}`;
  if (result.exitCode !== 0 || actual !== expected) {
    throw new Error(`installed Sanad version mismatch: expected ${expected}, received ${actual || "no version output"}`);
  }
}

async function workflowPaths(binary, inputs, workspace) {
  const args = ["--root", inputs.root, "--config", inputs.config, "--format", "json", "config", "show"];
  const result = await execute(binary, args, workspace, inputs.token);
  if (result.exitCode !== 0) throw new Error(`Sanad could not load configuration: ${result.stderr.trim() || "unknown error"}`);
  const config = JSON.parse(result.stdout);
  if (!Array.isArray(config.workflow_paths)) throw new Error("Sanad configuration did not contain workflow_paths");
  return config.workflow_paths;
}

function logStepSummary(mode, metrics, passed, write) {
  const icon = passed ? "✅" : "❌";
  const lines = [`${icon} sanad ${mode} — updates: ${metrics.updates}, pending cooldown: ${metrics.pending}, violations: ${metrics.violations}`];
  if ((mode === "apply" || mode === "upgrade") && !write) {
    lines.push("ℹ️  Preview only — set write: \"true\" to apply changes and commit them.");
  }
  core.info(lines.join("\n"));
}

async function run() {
  const inputs = actionInputs();
  const workspace = resolveWorkingDirectory(process.env.GITHUB_WORKSPACE, inputs.workingDirectory);
  inputs.root = workspace;
  inputs.config = resolveConfigPath(workspace, inputs.config);
  const binary = await installSanad(SANAD_VERSION, inputs.token);
  await verifySanadVersion(binary, workspace, inputs.token);
  core.addPath(path.dirname(binary));

  if (inputs.mode === "setup") {
    setCommonOutputs({ passed: true });
    core.info(`✅ Installed Sanad ${SANAD_VERSION}`);
    return;
  }

  const reportRoot = process.env.RUNNER_TEMP || workspace;
  const reportDirectory = fs.mkdtempSync(path.join(reportRoot, "sanad-"));
  const reportPath = path.join(reportDirectory, `${inputs.mode}-report.json`);
  const hasPRBody = inputs.mode === "plan" || inputs.mode === "apply";
  const prBodyPath = hasPRBody ? path.join(reportDirectory, `${inputs.mode}-pr-body.md`) : "";

  let paths = [];
  let before = new Map();
  if (inputs.write) {
    paths = await workflowPaths(binary, inputs, workspace);
    before = snapshotManagedFiles(workspace, paths, inputs.config);
  }

  const result = await execute(binary, commandArguments(inputs, reportPath, prBodyPath), workspace, inputs.token);
  if (!result.stdout.trim()) {
    setCommonOutputs({ passed: false, prBodyPath: prBodyPath && fs.existsSync(prBodyPath) ? prBodyPath : "" });
    core.setFailed(`Sanad exited with code ${result.exitCode}: ${result.stderr.trim() || "no diagnostic output"}`);
    return;
  }

  const report = parseReport(result.stdout, REPORT_VERSIONS[inputs.mode]);
  fs.writeFileSync(reportPath, `${JSON.stringify(report, null, 2)}\n`, { mode: 0o600 });
  const after = inputs.write ? snapshotManagedFiles(workspace, paths, inputs.config) : new Map();
  const changedFiles = inputs.write ? changedManagedFiles(before, after) : [];
  const metrics = reportMetrics(inputs.mode, report);
  const passed = result.exitCode === 0 && (inputs.mode !== "check" || report.passed);

  annotateDecisions(inputs.mode, report);
  logStepSummary(inputs.mode, metrics, passed, inputs.write);
  await summarizeReport(inputs.mode, report, SANAD_VERSION, passed);
  setCommonOutputs({
    passed,
    changed: changedFiles.length > 0,
    changedFiles,
    violations: metrics.violations,
    updates: metrics.updates,
    pending: metrics.pending,
    reportPath,
    prBodyPath: prBodyPath && fs.existsSync(prBodyPath) ? prBodyPath : "",
  });

  if (!passed) {
    core.setFailed(result.stderr.trim() || `Sanad ${inputs.mode} failed with exit code ${result.exitCode}`);
  }
}

if (require.main === module) {
  run().catch((error) => {
    setCommonOutputs();
    core.setFailed(error instanceof Error ? error.message : String(error));
  });
}

module.exports = { actionInputs, commandArguments, execute, run, setCommonOutputs, verifySanadVersion, workflowPaths };
