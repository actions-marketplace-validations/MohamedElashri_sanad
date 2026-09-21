"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const packageMetadata = require("../package.json");
const { REPORT_VERSIONS, SANAD_VERSION } = require("../src/constants");

test("root and self-reference action metadata stay equivalent", () => {
  const root = fs.readFileSync(path.join(__dirname, "..", "..", "action.yml"), "utf8");
  const nested = fs.readFileSync(path.join(__dirname, "..", "action.yml"), "utf8");
  assert.equal(nested, root.replace("main: action/dist/index.js", "main: dist/index.js"));
});

test("runtime, release, and report contract versions stay aligned", () => {
  const repositoryRoot = path.join(__dirname, "..", "..");
  const metadata = fs.readFileSync(path.join(repositoryRoot, "action.yml"), "utf8");
  const changelog = fs.readFileSync(path.join(repositoryRoot, "CHANGELOG.md"), "utf8");
  const contracts = fs.readFileSync(path.join(repositoryRoot, "internal", "cli", "report_contract.go"), "utf8");

  assert.equal(SANAD_VERSION, packageMetadata.version);
  assert.equal(packageMetadata.engines.node, ">=24");
  assert.match(metadata, /using: node24/);
  assert.match(changelog, new RegExp(`^## ${packageMetadata.version.replaceAll(".", "\\.")} - \\d{4}-\\d{2}-\\d{2}$`, "m"));
  assert.match(contracts, new RegExp(`checkReportVersion\\s*=\\s*${REPORT_VERSIONS.check}`));
  assert.match(contracts, new RegExp(`planReportVersion\\s*=\\s*${REPORT_VERSIONS.plan}`));
  assert.match(contracts, new RegExp(`upgradeReportVersion\\s*=\\s*${REPORT_VERSIONS.upgrade}`));
});
