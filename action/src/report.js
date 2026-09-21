"use strict";

const core = require("@actions/core");

const MAX_ANNOTATIONS = 50;
const MAX_TABLE_ROWS = 30;

function parseReport(stdout, expectedVersion) {
  let report;
  try {
    report = JSON.parse(stdout);
  } catch (error) {
    throw new Error(`Sanad did not produce valid JSON: ${error.message}`);
  }
  if (report.version !== expectedVersion) {
    throw new Error(`unsupported Sanad report version ${report.version}; expected ${expectedVersion}`);
  }
  return report;
}

function annotateCheck(report) {
  const violations = Array.isArray(report.violations) ? report.violations : [];
  for (const violation of violations.slice(0, MAX_ANNOTATIONS)) {
    core.error(violation.reason || violation.reason_code || "Sanad policy violation", {
      title: `Sanad: ${violation.action || violation.decision}`,
      file: violation.file,
      startLine: violation.line,
      startColumn: violation.column,
    });
  }
  if (violations.length > MAX_ANNOTATIONS) {
    core.warning(`${violations.length - MAX_ANNOTATIONS} additional Sanad violations are available in the JSON report`);
  }
}

function reportMetrics(mode, report) {
  if (mode === "check") {
    return { violations: report.summary.violations, updates: report.summary.updates, pending: report.summary.pending_cooldown };
  }
  if (mode === "plan" || mode === "apply") {
    return { violations: report.summary.policy_violations, updates: report.summary.updates_available, pending: report.summary.pending_cooldown };
  }
  return { violations: report.summary.blocked, updates: report.summary.updates, pending: report.summary.pending_cooldown };
}

function annotateDecisions(mode, report) {
  if (mode === "check") return annotateCheck(report);
  const entries = mode === "upgrade" ? report.actions : report.files.flatMap((file) => file.actions.map((action) => ({ ...action, file: file.path })));
  const problems = entries.filter((entry) => String(entry.decision).startsWith("error"));
  for (const entry of problems.slice(0, MAX_ANNOTATIONS)) {
    const properties = { title: `Sanad: ${entry.action || entry.raw || entry.decision}`, file: entry.file, startLine: entry.line };
    const message = entry.reason || entry.reason_code || entry.decision;
    if (mode === "plan") core.warning(message, properties);
    else core.error(message, properties);
  }
  if (problems.length > MAX_ANNOTATIONS) core.warning(`${problems.length - MAX_ANNOTATIONS} additional Sanad findings are available in the JSON report`);
}

function shortSHA(sha) {
  return sha ? String(sha).slice(0, 12) : "-";
}

async function summarizeCheck(report, version, passed) {
  const summary = report.summary;
  const statusLine = passed
    ? `✅ Sanad ${version} — all actions comply with policy.`
    : `❌ Sanad ${version} — ${summary.violations} policy violation(s) found.`;

  let s = core.summary.addHeading("Sanad check", 2).addRaw(statusLine).addEOL();

  s = s.addTable([
    [{ data: "Checked", header: true }, { data: "Violations", header: true }, { data: "Updates available", header: true }, { data: "Pending cooldown", header: true }, { data: "Skipped", header: true }],
    [String(summary.checked), String(summary.violations), String(summary.updates), String(summary.pending_cooldown), String(summary.skipped)],
  ]);

  const violations = Array.isArray(report.violations) ? report.violations : [];
  if (violations.length > 0) {
    s = s.addHeading("Policy violations", 3).addTable([
      [{ data: "File", header: true }, { data: "Line", header: true }, { data: "Action", header: true }, { data: "Decision", header: true }, { data: "Reason", header: true }],
      ...violations.slice(0, MAX_TABLE_ROWS).map((v) => [v.file || "-", String(v.line || "-"), v.action || "-", v.decision || "-", v.reason || v.reason_code || "-"]),
    ]);
    if (violations.length > MAX_TABLE_ROWS) s = s.addRaw(`_…and ${violations.length - MAX_TABLE_ROWS} more. See the JSON report for the full list._`).addEOL();
  }

  await s.write();
}

async function summarizePlanApply(mode, report, version, passed) {
  const metrics = reportMetrics(mode, report);
  const verb = mode === "apply" ? "applied" : "planned";
  const statusLine = passed
    ? `✅ Sanad ${version} ${verb} ${metrics.updates} update(s).`
    : `❌ Sanad ${version} ${mode} failed — ${metrics.violations} violation(s).`;

  let s = core.summary.addHeading(`Sanad ${mode}`, 2).addRaw(statusLine).addEOL();

  s = s.addTable([
    [{ data: "Updates", header: true }, { data: "Pending cooldown", header: true }, { data: "Policy violations", header: true }],
    [String(metrics.updates), String(metrics.pending), String(metrics.violations)],
  ]);

  // List the actual updates in a table
  const allActions = (report.files || []).flatMap((file) => (file.actions || []).map((a) => ({ ...a, filePath: file.path })));
  const updates = allActions.filter((a) => a.decision === "update" || a.decision === "pin");
  if (updates.length > 0) {
    s = s.addHeading("Updates", 3).addTable([
      [{ data: "File", header: true }, { data: "Line", header: true }, { data: "Action", header: true }, { data: "Ref", header: true }, { data: "Current SHA", header: true }, { data: "Candidate SHA", header: true }],
      ...updates.slice(0, MAX_TABLE_ROWS).map((a) => [
        a.filePath || "-", String(a.line || "-"), a.action || "-", a.logical_ref || a.ref || "-",
        shortSHA(a.current_sha), shortSHA(a.candidate_sha),
      ]),
    ]);
    if (updates.length > MAX_TABLE_ROWS) s = s.addRaw(`_…and ${updates.length - MAX_TABLE_ROWS} more._`).addEOL();
  }

  const violations = allActions.filter((a) => String(a.decision).startsWith("error"));
  if (violations.length > 0) {
    s = s.addHeading("Policy violations", 3).addTable([
      [{ data: "File", header: true }, { data: "Line", header: true }, { data: "Action", header: true }, { data: "Decision", header: true }, { data: "Reason", header: true }],
      ...violations.slice(0, MAX_TABLE_ROWS).map((a) => [a.filePath || "-", String(a.line || "-"), a.action || "-", a.decision || "-", a.reason || a.reason_code || "-"]),
    ]);
  }

  await s.write();
}

async function summarizeUpgrade(report, version, passed) {
  const metrics = reportMetrics("upgrade", report);
  const statusLine = passed
    ? `✅ Sanad ${version} upgrade — ${metrics.updates} ref(s) upgraded.`
    : `❌ Sanad ${version} upgrade — ${metrics.violations} blocked.`;

  let s = core.summary.addHeading("Sanad upgrade", 2).addRaw(statusLine).addEOL();

  s = s.addTable([
    [{ data: "Upgraded", header: true }, { data: "Pending cooldown", header: true }, { data: "Blocked", header: true }],
    [String(metrics.updates), String(metrics.pending), String(metrics.violations)],
  ]);

  const actions = Array.isArray(report.actions) ? report.actions : [];
  const upgraded = actions.filter((a) => a.decision === "update");
  if (upgraded.length > 0) {
    s = s.addHeading("Upgraded refs", 3).addTable([
      [{ data: "Action", header: true }, { data: "From ref", header: true }, { data: "To ref", header: true }, { data: "Candidate SHA", header: true }],
      ...upgraded.slice(0, MAX_TABLE_ROWS).map((a) => [a.action || "-", a.current_ref || "-", a.logical_ref || a.ref || "-", shortSHA(a.candidate_sha)]),
    ]);
  }

  const blocked = actions.filter((a) => String(a.decision).startsWith("error"));
  if (blocked.length > 0) {
    s = s.addHeading("Blocked", 3).addTable([
      [{ data: "Action", header: true }, { data: "Decision", header: true }, { data: "Reason", header: true }],
      ...blocked.slice(0, MAX_TABLE_ROWS).map((a) => [a.action || "-", a.decision || "-", a.reason || a.reason_code || "-"]),
    ]);
  }

  await s.write();
}

async function summarizeReport(mode, report, version, passed) {
  if (mode === "check") return summarizeCheck(report, version, passed);
  if (mode === "plan" || mode === "apply") return summarizePlanApply(mode, report, version, passed);
  if (mode === "upgrade") return summarizeUpgrade(report, version, passed);
  // fallback for unknown modes
  const metrics = reportMetrics(mode, report);
  core.summary
    .addHeading(`Sanad ${mode}`, 2)
    .addRaw(`Sanad ${version} ${passed ? "completed" : "failed"}.`)
    .addTable([
      [{ data: "Updates", header: true }, { data: "Pending cooldown", header: true }, { data: "Violations", header: true }],
      [String(metrics.updates), String(metrics.pending), String(metrics.violations)],
    ]);
  await core.summary.write();
}

module.exports = { MAX_ANNOTATIONS, annotateCheck, annotateDecisions, parseReport, reportMetrics, summarizeCheck, summarizePlanApply, summarizeUpgrade, summarizeReport };
