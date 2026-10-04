import assert from "node:assert/strict";
import test from "node:test";
import { safeNext } from "./next.ts";

test("safeNext follows only paths on this site", () => {
  assert.equal(safeNext("/oauth/authorize?client_id=X"), "/oauth/authorize?client_id=X");
  assert.equal(safeNext(undefined), "/");
  assert.equal(safeNext("https://evil.example"), "/");
  assert.equal(safeNext("//evil.example"), "/");
  assert.equal(safeNext("/\\evil.example"), "/");
  // Browsers drop tabs and newlines while parsing, which turns these into //evil.example.
  assert.equal(safeNext("/\t/evil.example"), "/");
  assert.equal(safeNext("/\n/evil.example"), "/");
  assert.equal(safeNext("\t//evil.example"), "/");
});
