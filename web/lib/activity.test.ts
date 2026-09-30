import assert from "node:assert/strict";
import test from "node:test";
import { describeChange } from "./activity.ts";

const messages: Record<string, string> = {
  you: "You",
  system: "System",
  imported: "{actor} imported the ticket from {ref}",
  importUpdated: "{actor} updated the ticket from import {ref}",
};
const t = Object.assign(
  (key: string, values: Record<string, string> = {}) => messages[key].replace(/\{(\w+)\}/g, (_, k: string) => values[k]),
  { has: (key: string) => key in messages },
);

// MSL-41: an imported ticket reads as imported, not as the raw action name.
test("describes imports", () => {
  const at = "2026-09-29T15:00:00Z";
  assert.equal(
    describeChange(t, { kind: "event", action: "import_create", at, actor: { id: 1, name: "Rina" }, changes: { external_ref: "ARU-101" } }, 1),
    "You imported the ticket from ARU-101",
  );
  assert.equal(
    describeChange(t, { kind: "event", action: "import_update", at, actor: { id: 2, name: "Bayu" }, changes: { external_ref: "ARU-101" } }, 1),
    "Bayu updated the ticket from import ARU-101",
  );
});
