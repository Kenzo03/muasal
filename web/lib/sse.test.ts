import assert from "node:assert/strict";
import { test } from "node:test";
import { SSEParser } from "./sse.ts";

test("events split across chunks come out whole, in order", () => {
  const p = new SSEParser();
  assert.deepEqual(p.push('event: scope\ndata: {"a":'), []);
  assert.deepEqual(p.push('1}\n\nevent: claim\ndata: {"text":"x"}\n'), [{ event: "scope", data: '{"a":1}' }]);
  assert.deepEqual(p.push("\n"), [{ event: "claim", data: '{"text":"x"}' }]);
});

test("keep-alive comments and blank blocks are skipped", () => {
  const p = new SSEParser();
  assert.deepEqual(p.push(": keep-alive\n\n: again\n\nevent: result\ndata: {}\n\n"), [{ event: "result", data: "{}" }]);
});

test("CRLF line ends, data without an event name and multi-line data", () => {
  const p = new SSEParser();
  assert.deepEqual(p.push("data: one\r\ndata: two\r\n\r\n"), [{ event: "message", data: "one\ntwo" }]);
});
