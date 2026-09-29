import assert from "node:assert/strict";
import test from "node:test";
import { matchProjects, recentFirst } from "./projects.ts";

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
