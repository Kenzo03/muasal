import assert from "node:assert/strict";
import test from "node:test";
import { addAlias, matches, maxAliases, userStatus } from "./admin.ts";

test("a user is invited until they set a password, and disabled wins", () => {
  assert.equal(userStatus({ disabled: false, has_password: false }), "invited");
  assert.equal(userStatus({ disabled: false, has_password: true }), "active");
  assert.equal(userStatus({ disabled: true, has_password: false }), "disabled");
});

test("search ignores case, accents and surrounding space, and any field may match", () => {
  assert.ok(matches("", "Budi"));
  assert.ok(matches("  budi ", "Budi Santoso", "budi@example.com"));
  assert.ok(matches("jose", "José"));
  assert.ok(matches("sj group", "Sinar Jaya", "SJ", "SJ Group"));
  assert.ok(!matches("arunika", "Sinar Jaya", null, undefined));
});

test("an alias is trimmed, loses a trailing comma, and duplicates or blanks change nothing", () => {
  assert.deepEqual(addAlias([], "  SJ Group, "), ["SJ Group"]);
  assert.deepEqual(addAlias(["SJ Group"], "sj group"), ["SJ Group"]);
  assert.deepEqual(addAlias(["SJ Group"], " , "), ["SJ Group"]);
});

test("aliases stop at the API's limit", () => {
  const full = Array.from({ length: maxAliases }, (_, i) => `A${i}`);
  assert.equal(addAlias(full, "one more"), full);
});
