import assert from "node:assert/strict";
import test from "node:test";
import { remarkMentions } from "./mentions.ts";

// MSL-30: "@bayu" reads as Bayu Santoso; unknown handles, emails and code stay.
test("mentions become names", () => {
  const tree = {
    type: "root",
    children: [
      { type: "paragraph", children: [{ type: "text", value: "Bisa dicek, @bayu. Cc @nobody, mail ani@bayu.id" }] },
      { type: "paragraph", children: [{ type: "inlineCode", value: "@bayu" }] },
    ],
  };
  remarkMentions([{ handle: "bayu", name: "Bayu Santoso" }])()(tree);
  assert.deepEqual(tree.children[0].children, [
    { type: "text", value: "Bisa dicek, " },
    { type: "link", url: "#mention-bayu", children: [{ type: "text", value: "Bayu Santoso" }] },
    { type: "text", value: ". Cc @nobody, mail ani@bayu.id" },
  ]);
  assert.deepEqual(tree.children[1].children, [{ type: "inlineCode", value: "@bayu" }]);
});
