import assert from "node:assert/strict";
import test from "node:test";
import { bursts } from "./notifications.ts";

const n = (id: number, at: string, key: string | undefined, actor: number) => ({
  id, type: "status" as const, created_at: `2026-09-30T${at}:00Z`, ticket_key: key, actor: { id: actor, name: "Rina" }, payload: {}, read: false,
});

// MSL-28: one person's changes to one ticket within minutes read as one item.
test("groups bursts", () => {
  const items = [
    n(6, "10:09", "DMS-6", 1),
    n(5, "10:07", "DMS-6", 1), // two minutes before: the same burst
    n(4, "10:05", "DMS-6", 2), // another person
    n(3, "10:04", "DMS-7", 2), // another ticket
    n(2, "10:00", "DMS-7", 2), // four minutes before: the same burst
    n(1, "09:50", "DMS-7", 2), // ten minutes before: a new one
    n(0, "09:49", undefined, 2), // no ticket
  ];
  assert.deepEqual(bursts(items).map((g) => g.map((x) => x.id)), [[6, 5], [4], [3, 2], [1], [0]]);
});
