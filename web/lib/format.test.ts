import assert from "node:assert/strict";
import test from "node:test";
import { dateIn, dateTime, day, dayOf, itemDay } from "./format.ts";

test("timestamps show on the clock of the user's timezone", () => {
  // 20:30 UTC is 03:30 the next day in Jakarta and 16:30 the same day in New York.
  const at = "2026-09-27T20:30:00Z";
  assert.equal(dateTime(at, "id", "Asia/Jakarta"), "28 Sep 2026, 03:30");
  assert.equal(dateTime(at, "en", "America/New_York"), "27 Sep 2026, 16:30");
  assert.equal(dateTime("2026-09-27T00:05:00Z", "en", "UTC"), "27 Sep 2026, 00:05");
  assert.equal(dayOf(at, "id", "Asia/Jakarta"), "28 Sep 2026");
  assert.equal(dayOf(at, "id", "Asia/Jakarta", false), "28 Sep");
  assert.equal(dateIn(at, "Asia/Jakarta"), "2026-09-28");
  assert.equal(dateIn(new Date(at), "America/New_York"), "2026-09-27");
});

test("calendar dates never shift", () => {
  assert.equal(day("2026-12-01", "id"), "1 Des 2026");
  // A note's decision day arrives as midnight UTC; west of UTC it must stay that day.
  const note = { kind: "note", date: "2026-09-27T00:00:00Z" };
  assert.equal(itemDay(note, "en", "America/New_York"), "27 Sep 2026");
  assert.equal(itemDay({ ...note, kind: "ticket" }, "en", "America/New_York"), "26 Sep 2026");
});
