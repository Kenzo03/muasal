import assert from "node:assert/strict";
import test from "node:test";
import { problemMessage, type Problem } from "./problem.ts";

const messages: Record<string, string> = {
  generic: "Something went wrong.",
  invalid: "Check this field",
  invalidField: "Check the {field} field",
  "fields.key": "Key",
  project_key_taken: "This key is already taken",
};
const t = Object.assign((key: string, values?: Record<string, string>) => messages[key].replace("{field}", values?.field ?? ""), {
  has: (key: string) => key in messages,
});
const fieldError = (field: string, code: string) => ({ code: "validation", errors: [{ field, code, message: "" }] }) as unknown as Problem;

test("a vague field error names its field", () => {
  assert.equal(problemMessage(fieldError("key", "invalid"), t), "Check the Key field");
});

test("a field without a label, or a specific code, keeps its own sentence", () => {
  assert.equal(problemMessage(fieldError("colour", "invalid"), t), "Check this field");
  assert.equal(problemMessage(fieldError("key", "project_key_taken"), t), "This key is already taken");
  assert.equal(problemMessage(undefined, t), "Something went wrong.");
});
