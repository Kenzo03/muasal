import assert from "node:assert/strict";
import test from "node:test";
import { matchProjects, recentFirst, suggestKey } from "./projects.ts";

const projects = [
  { key: "AAA", name: "Alpha" },
  { key: "DEMO", name: "HRIS Demo" },
  { key: "OPT", name: "Optik Employee Self Service" },
  { key: "ZED", name: "Zed" },
];
const keys = (ps: { key: string }[]) => ps.map((p) => p.key);

test("the projects opened last come first, the rest keep their order", () => {
  assert.deepEqual(keys(recentFirst(projects, ["OPT", "DEMO", "GONE"])), ["OPT", "DEMO", "AAA", "ZED"]);
  assert.deepEqual(keys(recentFirst(projects, [])), ["AAA", "DEMO", "OPT", "ZED"]);
});

test("a filter matches the name or the key, ignoring case", () => {
  assert.deepEqual(keys(matchProjects(projects, " self ")), ["OPT"]);
  assert.deepEqual(keys(matchProjects(projects, "demo")), ["DEMO"]);
  assert.equal(matchProjects(projects, "").length, 4);
});

test("a new project's key follows its name", () => {
  assert.equal(suggestKey("Arunika DMS"), "DMS");
  assert.equal(suggestKey("PT Bumi Logistik"), "PBL"); // two capitals are a prefix, not an acronym
  assert.equal(suggestKey("Payroll"), "PAYR");
  assert.equal(suggestKey("Sistem Absensi Karyawan é"), "SAKE");
  assert.equal(suggestKey("2026 rollout"), "");
  assert.equal(suggestKey(""), "");
});
