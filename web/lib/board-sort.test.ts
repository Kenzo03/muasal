import assert from "node:assert/strict";
import test from "node:test";
import { boardSorts, sortCards } from "./board-sort.ts";

type C = { key: string; priority: "low" | "medium" | "high" | "urgent"; due_date: string | null };
const cards: C[] = [
  { key: "CTR-9", priority: "medium", due_date: null },
  { key: "CTR-10", priority: "urgent", due_date: "2026-10-20" },
  { key: "CTR-2", priority: "low", due_date: "2026-10-08" },
  { key: "CTR-11", priority: "urgent", due_date: "2026-10-09" },
  { key: "CTR-3", priority: "medium", due_date: "2026-10-08" },
];
const keys = (xs: C[]) => xs.map((c) => c.key);

// The default matches the board's server order: priority, due date (none
// last), then ticket number.
test("highest priority first, then due date, then number", () => {
  assert.deepEqual(keys(sortCards(cards, "priority")), ["CTR-11", "CTR-10", "CTR-3", "CTR-9", "CTR-2"]);
});

test("lowest priority first keeps the due date and number tie-breaks", () => {
  assert.deepEqual(keys(sortCards(cards, "priority-low")), ["CTR-2", "CTR-3", "CTR-9", "CTR-11", "CTR-10"]);
});

// Ticket numbers are given in creation order, so they stand for age; CTR-10
// is newer than CTR-9 (numbers, not text).
test("newest and oldest follow the ticket number", () => {
  assert.deepEqual(keys(sortCards(cards, "newest")), ["CTR-11", "CTR-10", "CTR-9", "CTR-3", "CTR-2"]);
  assert.deepEqual(keys(sortCards(cards, "oldest")), ["CTR-2", "CTR-3", "CTR-9", "CTR-10", "CTR-11"]);
});

test("sorting leaves the input alone, and an unknown choice falls back to the default", () => {
  const before = keys(cards);
  sortCards(cards, "newest");
  assert.deepEqual(keys(cards), before);
  assert.deepEqual(keys(sortCards(cards, "nonsense" as never)), keys(sortCards(cards, "priority")));
  assert.deepEqual(boardSorts, ["priority", "priority-low", "newest", "oldest"]);
});
